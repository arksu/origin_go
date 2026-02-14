# ADR 0003: Tree Growth через ECS Resource Scheduler и budgeted catch-up

- Status: Proposed
- Date: 2026-02-14
- Owners: Game Server

## 1. Context

Нужно внедрить рост деревьев как новую фичу с требованиями:
1. Стадии роста и тайминги задаются в `object defs` (per-def).
2. Рост должен работать online + offline.
3. При активации чанка нужен догон прошедшего времени.
4. Обновлять только растущие объекты; финальная стадия не апдейтится.
5. High-load режим: без полного per-tick обхода.
6. Appearance зависит от flags (`tree.stageN`).
7. `chop` доступен только на разрешенных стадиях.

В системе уже есть паттерн dirty-queue для behavior recompute и budget-per-tick обработка объектов.

## 2. Decision

Принимаем архитектуру централизованной Behavior Tick Platform, где tree growth — первая механика:

1. В behavior contracts добавляется новая tick-capability (рабочее имя: `ScheduledTickBehavior`).
   - capability опциональна;
   - behavior реализует ее только когда нужен периодический апдейт.

2. В `behaviors.Registry` добавляется fail-fast валидация tick-контракта на startup.

3. Вводится единый ECS resource `BehaviorTickSchedule` для всех механик, который:
   - хранит due-объекты;
   - поддерживает dedup/reschedule/cancel;
   - адресует объекты по `EntityID`;
   - отдает due-объекты порционно в рамках общего budget.

4. Вводится `BehaviorTickSystem`, которая на каждом тике:
   - берет due-объекты из `BehaviorTickSchedule`;
   - вызывает tick-логику соответствующего behavior;
   - допускает прямые изменения ECS-компонентов в tick-handler (в рамках shard lock / ECS tick);
   - при фактическом изменении runtime-состояния вызывает `MarkObjectBehaviorDirty(...)`;
   - при ошибке обработки объекта: логирует, пропускает объект и продолжает тик.

5. Частота апдейта определяется механикой через интервал в тиках из behavior def/config.
   - always-tick режим не поддерживается.

6. Порядок систем фиксирован:
   - `BehaviorTickSystem -> MarkObjectBehaviorDirty -> ObjectBehaviorSystem`.

7. Catch-up при активации чанка:
   - учитывается прошедшее время;
   - применяется общий лимит catch-up для tick-платформы;
   - при недогоне объект остается в `BehaviorTickSchedule` для продолжения.
   - если у дерева отсутствует persisted state (объект создан генератором карты), дерево инициализируется сразу в финальной стадии и не добавляется в schedule.

8. Для tree mechanics:
   - runtime формирует stage-flag (`tree.stageN`);
   - доступность `chop` проверяется по стадии и списку разрешенных стадий из def;
   - финальная стадия снимает объект с расписания.

## 3. Rationale

1. **Производительность**: сложность по активным due-объектам, а не по всем деревьям.
2. **Детерминизм**: тик-модель без джиттера.
3. **Совместимость**: интеграция в существующий behavior runtime и dirty-queue pipeline.
4. **Управляемость нагрузки**: budget-per-tick и catch-up limit.
5. **KISS**: единый scheduler и единый tick-system вместо множества специализированных scheduler-ресурсов.

## 4. Consequences

### Positive

1. Нет full-scan деревьев каждый тик.
2. Финальные деревья не потребляют runtime budget.
3. Поведение стабильно при оффлайн догоне и рестартах.
4. Appearance остается в едином канале через behavior recompute.

### Negative / Trade-offs

1. Появляется дополнительное состояние-планировщик, требующее строгой консистентности при despawn/restore.
2. При больших оффлайн разрывах возможен backlog due-объектов.
3. Catch-up логика усложняет path активации чанка.
4. Нужна явная ветка восстановления для объектов без persisted state, чтобы исключить ложный рост с `growthStartStage`.

## 5. Rejected Alternatives

1. **Global per-tick scan всех деревьев**
   - Отклонено: плохо масштабируется и нарушает high-load требования.

2. **Рост только online, без offline catch-up**
   - Отклонено: не соответствует бизнес-требованию «оффлайн тоже».

3. **Хранить growth-таймер как wall-clock без tick-based scheduler**
   - Отклонено: сложнее детерминизм, больше edge-cases в shard loop.

4. **Сразу применять growth в момент chunk activation без лимитов**
   - Отклонено: риск спайков при массовой активации чанков.

## 6. Operational Limits

1. `game.behavior_tick_global_budget_per_tick` (configurable)
2. `game.behavior_tick_catchup_limit_ticks` (configurable)
3. Для tree growth стартовые значения:
   - budget default: `200`
   - catch-up limit default: `2000`

## 7. Follow-up Implementation Notes

1. Добавить tick-capability интерфейс в contracts и registry fail-fast validation.
2. Ввести единый `BehaviorTickSchedule` в ECS resources.
3. Подключить `BehaviorTickSystem` в shard systems order перед `ObjectBehaviorSystem`.
4. Добавить growth-поля в tree behavior config (`object defs`) и interval в тиках.
5. Расширить persistent `TreeBehaviorState` полями стадии/next tick.
6. На chunk activation: если persisted tree state отсутствует, ставить `stage=growthStageMax`, не планировать рост.
7. Обеспечить корректный unschedule при despawn/transform в не-растущее состояние.
8. Покрыть unit + integration тестами (due processing, catch-up, final stop, chop-stage gating, error-continue policy, no-state-final-stage policy).

## 8. Finalized Points

1. JSON contract growth-конфига фиксируется с явным списком стадий для `chop` (`allowedChopStages`).
2. При многоступенчатом catch-up на активации чанка применяется только финальное appearance-состояние (без публикации промежуточных appearance-событий).
3. Tick-capability опциональна, но валидируется fail-fast при регистрации behavior.
4. Единый scheduler: `BehaviorTickSchedule`, ключ — `EntityID`.
5. Бюджет и catch-up лимит — общие для платформы.
6. Порядок систем: `BehaviorTickSystem -> MarkObjectBehaviorDirty -> ObjectBehaviorSystem`.
7. Ошибки отдельных объектов не останавливают тик (log + skip + continue).
8. Для дерева без persisted state применяется политика `final-stage-on-missing-state` (без schedule).
