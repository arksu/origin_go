# Implementation Checklist: Tree Growth (ECS Resource Scheduler)

Основано на:
- PRD: `/Users/park/projects/origin_go/docs/prd/tree_growth_behavior_runtime.md`
- ADR: `/Users/park/projects/origin_go/docs/adr/0003_tree_growth_scheduler_via_ecs_resource.md`
- JSON Contract: `/Users/park/projects/origin_go/docs/prd/tree_growth_object_defs_json_contract.md`

## 1. Scope Lock

- [ ] В scope: рост стадий, offline catch-up, stage flags, stage-gated chop.
- [ ] Вне scope: jitter, live-tuning, биомы/сезоны, миграции старых данных, telemetry.

## 2. Config Layer

- [ ] Добавить `game.behavior_tick_global_budget_per_tick` (default `200`).
- [ ] Добавить `game.behavior_tick_catchup_limit_ticks` (default `2000`).
- [ ] Значения валидируются (`> 0`).

## 3. Object Def Contract

- [ ] Внедрить новые поля `growthStageMax`, `growthStartStage`, `growthStageDurationsTicks`, `allowedChopStages`.
- [ ] Оставить совместимость с текущими chop-полями tree behavior.
- [ ] Валидация strict (`DisallowUnknownFields`) и ограничения диапазонов/длины массива.
- [ ] `allowedChopStages` обрабатывается как явный список (без диапазонов).

## 4. Runtime State & Persistence

- [ ] Расширить `TreeBehaviorState` полями `stage`, `next_growth_tick`.
- [ ] Обновить serialize/deserialize envelope для tree growth state.
- [ ] Для финальной стадии корректно сохранять состояние без планирования следующего роста.
- [ ] Проверить совместимость с объектами без state (новые спавны/чистые данные).
- [ ] Зафиксировать policy: при restore/activation без persisted tree state объект поднимается в `growthStageMax` и не schedule-ится на рост.

## 5. ECS Resource Scheduler

- [ ] Добавить ресурс планирования роста (`TreeGrowthSchedule`/финальное имя).
- [ ] Реализовать `schedule/reschedule/cancel/popDue`.
- [ ] Дедупликация хендлов обязательна.
- [ ] Unschedule при despawn/transform в не-растущее состояние.

## 6. Growth System

- [ ] Добавить `TreeGrowthSystem`.
- [ ] Система обрабатывает только due-объекты до `tree_growth_budget_per_tick`.
- [ ] При переходе стадии обновляет runtime state и планирует следующий тик.
- [ ] На финальной стадии снимает объект из schedule.
- [ ] При фактическом изменении стадии вызывает `MarkObjectBehaviorDirty(...)`.

## 7. Behavior Integration

- [ ] Runtime behavior выставляет один флаг `tree.stageN` по текущей стадии.
- [ ] Проверка доступности `chop` завязана на `allowedChopStages`.
- [ ] Если стадия неразрешена — `chop` не попадает в context actions.
- [ ] Если разрешена — лут полный (logs), без stage-пенальти.

## 8. Chunk Activation / Offline Catch-up

- [ ] При активации чанка учитывать прошедшее время по `next_growth_tick`.
- [ ] Применять лимит catch-up `behavior_tick_catchup_limit_ticks`.
- [ ] Если догон не завершен — продолжить через scheduler.
- [ ] На активации применять только финальное appearance-состояние (без промежуточных appearance-events).
- [ ] Если persisted state отсутствует (map-generated object) — сразу выставлять финальную стадию и не запускать catch-up/schedule.

## 9. System Ordering

- [ ] Подключить `TreeGrowthSystem` перед `ObjectBehaviorSystem`.
- [ ] Проверить отсутствие конфликтов с `ChunkSystem` и chunk lifecycle.

## 10. Performance & Safety

- [ ] Подтвердить отсутствие full-scan по всем деревьям.
- [ ] Подтвердить бюджетную обработку (`<= 200` объектов/тик).
- [ ] Проверить сценарий высокой загрузки: много чанков, рост только у доли деревьев.
- [ ] Проверить отсутствие неограниченного backlog роста на финальных стадиях.
- [ ] Проверить, что деревья без persisted state не создают ростовой backlog.

## 11. Testing Checklist

### Unit
- [ ] Валидация growth-полей def (границы, длины, дубликаты).
- [ ] Scheduler: due/pop, dedup, reschedule, cancel.
- [ ] Stage transitions: normal path + final stop.
- [ ] Stage flags: только один `tree.stageN` активен.
- [ ] Chop gating по `allowedChopStages`.

### Integration
- [ ] Spawn/restore дерева с growth-state.
- [ ] Chunk activation catch-up с лимитом `2000`.
- [ ] Продолжение недогнанного объекта в следующих тиках.
- [ ] Финальная стадия не апдейтится и не re-schedule-ится.
- [ ] Multiplayer: рубка разрешенной стадии и полный logs-лут.
- [ ] Restore/activation дерева без persisted state: сразу `stage=growthStageMax`, без growth schedule.

## 12. Definition of Done

- [ ] Все пункты PRD acceptance criteria закрыты.
- [ ] JSON contract принят командой backend/gameplay.
- [ ] ADR и PRD синхронизированы с реализацией.
- [ ] Нет деградации tick-time в целевом нагрузочном сценарии.
