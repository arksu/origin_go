# PRD Checklist: Tree chopping + Cyclic Action + Object State Foundation

Основано на PRD `/Users/park/projects/origin_go/docs/prd/tree_chop_cyclic_action_state_foundation.md`.

## 1. Scope Lock

- [ ] В scope только: `tree` рубка, `Cyclic Action`, `ObjectInternalState`, persistence.
- [ ] Вне scope: рост деревьев, плодоношение, Telemetry, Rollout.

## 2. ObjectInternalState Contract

- [ ] `ObjectInternalState` содержит только `State` и `IsDirty`.
- [ ] `State` реализован как sparse typed storage.
- [ ] Все мутации идут через единый helper.
- [ ] Любая реальная мутация `State` ставит `IsDirty=true`.
- [ ] Флаги не используются и не хранятся.

## 3. Serialization and DB Persistence

- [ ] Реализован versioned envelope (`v=1`) в `objects.data`.
- [ ] Персистится только `State`.
- [ ] При пустом state в БД сохраняется `NULL`.
- [ ] Save фильтр использует `IsDirty`.
- [ ] Ошибки сериализации/десериализации не проглатываются.

## 4. Object Def Contract (`behaviors`)

- [ ] `behaviors` map является каноничным источником конфигов behavior.
- [ ] Ключи `behaviors` — строковые (`"tree"` и т.д.).
- [ ] Конфиг содержит только параметры, без DSL-логики.
- [ ] Для `tree` есть поля: `chopPointsTotal`, `chopCycleDurationTicks`, `logsSpawnDefKey`, `logsSpawnCount`, `transformToDefKey`.
- [ ] Для `tree` есть поля: `chopPointsTotal`, `chopCycleDurationTicks`, `logsSpawnDefKey`, `logsSpawnCount`, `logsSpawnInitialOffset`, `logsSpawnStepOffset`, `transformToDefKey`.
- [ ] `priority` optional (default `100`), порядок `(priority ASC, behaviorKey ASC)`.
- [ ] behavior config декодируется strict (`DisallowUnknownFields`).
- [ ] Определен `log` def без коллайдера с `resource="log"`.

## 5. Tree Behavior

- [ ] Для дерева доступен `Chop`.
- [ ] На первом `Chop` инициализируется `chop_points`.
- [ ] На completion цикла `chop_points` уменьшается.
- [ ] На `chop_points==0` спавнятся бревна один раз.
- [ ] На `chop_points==0` выполняется in-place transform в stump def (`type_id` меняется, `object id` сохраняется).
- [ ] После transform `Chop` больше не доступен.
- [ ] Направление валки определяется противоположно вектору `tree->player` и привязано строго к оси `X` или `Y`.
- [ ] При `abs(dx) == abs(dy)` направление валки выбирает ось `X`.
- [ ] Позиции логов: первый на отступе `O`, остальные с шагом `P` по той же оси.

## 6. Appearance and Client Update

- [ ] После transform пересчитывается appearance по новому def.
- [ ] Изменение объекта отправляется клиентам (тот же `object id`, новый `type_id/resource`).

## 7. Cyclic Action Core

- [ ] Поддержаны object-target и self-target действия.
- [ ] У игрока строго одно активное длительное действие.
- [ ] `unlink` немедленно завершает действие как `canceled`.
- [ ] На `canceled` прогресс очищается и action-state удаляется.
- [ ] Клиент получает прогресс цикла `elapsedTicks/totalTicks`.
- [ ] По completion вызывается callback behavior (`continue|complete|canceled`).

## 8. Multiplayer Chop Rules

- [ ] Несколько игроков могут рубить одно дерево одновременно.
- [ ] `chop_points` общий для объекта.
- [ ] Если дерево уже transformed в stump, остальные получают `canceled` на callback после текущего цикла.

## 9. Performance Checklist

- [ ] Подтверждено отсутствие глобального per-tick скана всех деревьев.
- [ ] Обновления дерева происходят только на событийных точках.
- [ ] Прогресс длительных действий хранится в action-state, не в object-state.
- [ ] Нагрузочный сценарий с тысячами деревьев не деградирует по тик-циклу.

## 10. Testing Checklist

- [ ] Unit: state helper корректно ставит `IsDirty`.
- [ ] Unit: init-on-first-chop.
- [ ] Unit: `onChopped` спавнит бревна и меняет `type_id` с сохранением `id`.
- [ ] Unit: ось валки выбирается корректно для всех квадрантов.
- [ ] Unit: логи спавнятся по схеме `N/O/P` без отклонения от выбранной оси.
- [ ] Unit: `unlink -> canceled + clear`.
- [ ] Integration: мультиплеерная рубка одного дерева.
- [ ] Integration: serialize/deserialize envelope `v=1`.
- [ ] Integration: empty state persists as `NULL`.

## 11. Definition of Done

- [ ] Все acceptance criteria PRD закрыты.
- [ ] Документация согласована с backend/gameplay.
- [ ] Нет регресса производительности в целевом сценарии.
