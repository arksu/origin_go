# ADR 0001: ObjectInternalState для tree chopping и Cyclic Action

- Status: Proposed
- Date: 2026-02-11
- Owners: Game Server

## 1. Context

Текущая итерация:
1. Только `tree` behavior для рубки.
2. Общая механика длительных действий `Cyclic Action`.
3. База для будущих mechanics: оптимизированный `ObjectInternalState`.

Ключевые требования:
1. Не делать массовый per-tick update тысяч объектов.
2. Хранить только нужный runtime-state behavior-ов.
3. В `object def` хранить только конфиг (числа/ссылки), алгоритмы держать в коде.
4. При срубании дерева превращать его в отдельный `def` пня через смену `type_id` с сохранением `object id`.
5. Убрать флаги из этой модели.
6. Спавн логов при рубке должен быть детерминированным по оси и зависеть от позиции игрока.

## 2. Decisions

1. `ObjectInternalState` содержит только `State` и `IsDirty`.
2. `State` хранится как sparse typed storage.
3. `Flags` не используются и не хранятся.
4. Персист состояния идет в `objects.data` как versioned envelope.
5. Пустой state в БД хранится как `NULL`.
6. Канон подключения behavior-ов в `ObjectDef`: `behaviors` map со строковыми ключами (`"tree"`).
7. После `onChopped` выполняется in-place transform объекта:
   - сохраняем `object id`,
   - меняем `type_id` на `transformToDefKey`,
   - пересчитываем `appearance`,
   - отправляем обновление клиентам.
8. `log` — отдельный `def` без коллайдера, `resource = "log"`.

## 3. Runtime Data Model

```go
type ObjectInternalState struct {
    State   BehaviorStateBag
    IsDirty bool
}

type BehaviorID uint16

type StateKey[T any] struct {
    ID BehaviorID
}

type BehaviorSlot struct {
    ID    BehaviorID
    Value any // *T
}

type BehaviorStateBag struct {
    Slots []BehaviorSlot
}
```

Правила:
1. Доступ к `State` только через typed helper API.
2. Любая реальная мутация `State` ставит `IsDirty=true`.
3. Никаких runtime-флагов в `ObjectInternalState`.

## 4. ObjectDef format (config-only)

Канон:
1. `behaviors` = `map[string]config`.
2. `priority` optional, default `100`, runtime порядок: `(priority ASC, behaviorKey ASC)`.
3. В конфиге только параметры, не DSL-логика.
4. Все условия действия, валидации и эффекты описываются в коде behavior.

Пример:

```json
{
  "behaviors": {
    "tree": {
      "priority": 20,
      "chopPointsTotal": 6,
      "chopCycleDurationTicks": 20,
      "logsSpawnDefKey": "log_birch",
      "logsSpawnCount": 3,
      "logsSpawnInitialOffset": 12,
      "logsSpawnStepOffset": 10,
      "transformToDefKey": "stump_birch"
    }
  }
}
```

## 5. Persistent Format

Envelope:

```json
{
  "v": 1,
  "behaviors": {
    "tree": {
      "chop_points": 6
    }
  }
}
```

Правила:
1. В `objects.data` пишем только `State`.
2. Пустой state -> `objects.data = NULL`.
3. Сериализация только для `IsDirty=true`.
4. Ошибки сериализации/десериализации не проглатываются.

## 6. Tree Flow (iteration 1)

1. Все деревья рубимы (`Chop` доступен, пока объект типа дерева).
2. На первом `Chop` инициализируется `chop_points`.
3. На каждом completion цикла `chop_points--`.
4. На `chop_points == 0`:
   - spawn `N` логов по конфигу и направлению валки,
   - in-place transform в `stump` def,
   - `type_id` меняется, `object id` сохраняется,
   - пересчитывается appearance,
   - клиентам отправляется обновление объекта.
5. Для `stump` def behavior `tree` отсутствует, поэтому `Chop` больше не появляется в меню.

Геометрия спавна бревен:
1. Находим вектор `tree -> player`.
2. Берем противоположный вектор (валим от игрока).
3. Приводим к одной оси (`+X`, `-X`, `+Y`, `-Y`).
4. При равенстве `abs(dx)` и `abs(dy)` выбираем ось `X`.
5. Первый `log` спавним на отступе `O`.
6. Остальные `log` спавним с шагом `P` по той же оси.

## 7. Performance

1. Нет `O(N objects)` per-tick обхода деревьев.
2. Изменения state идут только на событиях (`Chop start`, `cycle complete`, `onChopped` transform).
3. Прогресс `Cyclic Action` хранится в action-state игрока, не в object state.
4. Нагрузка определяется `O(active actions)`.

## 8. Rejected Alternatives

1. Большой struct с полем под каждый behavior.
2. DSL условий/эффектов в `object def`.
3. Runtime flags для tree/stump переключения вместо отдельного stump def.
