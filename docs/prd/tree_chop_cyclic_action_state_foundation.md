# PRD: Tree chopping, Cyclic Action и Object State Foundation

## 1. Problem Statement

Нужен узкий scope на итерацию:
1. Только рубка деревьев.
2. Единая механика длительных действий.
3. Производительное хранение `ObjectInternalState`.
4. Корректный персист state в БД.

## 2. Goals

1. Реализовать `tree` behavior для рубки.
2. Реализовать `Cyclic Action` (объект/`self`, single active action).
3. Зафиксировать `ObjectInternalState` как `State + IsDirty` (без flags).
4. Зафиксировать формат `object def` с `behaviors` map (только конфиг, алгоритмы в коде).
5. При срубании делать in-place transform дерева в отдельный `stump` def:
   - меняется `type_id`,
   - `object id` сохраняется,
   - appearance пересчитывается и отправляется на клиент.

## 3. Non-Goals

1. Рост деревьев.
2. Сбор фруктов.
3. Telemetry и Rollout.
4. DSL-логика механик в `object def`.

## 4. Product Scope

### 4.1 Tree behavior

1. Деревья в этой итерации рубимы.
2. На первом `Chop` инициализируется `chop_points` из `object def`.
3. На completion каждого цикла `chop_points` уменьшается на 1.
4. На `chop_points == 0`:
   - спавнятся `N` объектов `log` (по конфигу),
   - объект трансформируется в `stump` (`type_id` смена с сохранением `id`),
   - appearance пересчитывается,
   - клиентам отправляется обновление объекта.
5. Для `stump` def действие `Chop` отсутствует.
6. Геометрия спавна логов:
   - первый `log` ставится с начальным отступом `O` от центра дерева,
   - каждый следующий `log` ставится с шагом `P` по той же оси,
   - направление валки определяется от позиции игрока относительно дерева:
     - считаем вектор `tree -> player`,
     - берем противоположный вектор,
     - приводим к одной оси (`+X`, `-X`, `+Y`, `-Y`),
     - при `abs(dx) == abs(dy)` выбираем ось `X`,
     - спавн идет строго по выбранной оси.

### 4.2 Cyclic Action

1. Действие может быть над объектом и над `self`.
2. У игрока строго одно активное длительное действие.
3. Для object-target нужен `Linked`.
4. Любой `unlink`:
   - немедленно `canceled`,
   - прогресс очищается,
   - action-state удаляется.
5. Клиент получает `elapsedTicks/totalTicks` по циклу.
6. По завершении цикла вызывается callback behavior (`continue|complete|canceled`).

### 4.3 Multiplayer chopping

1. Несколько игроков могут рубить одно дерево.
2. `chop_points` общий для объекта.
3. Если дерево уже transformed в `stump`, другие игроки завершают текущий цикл и получают `canceled` на callback.

## 5. Object State Architecture

### 5.1 Runtime contract

`ObjectInternalState`:
1. `State` — sparse typed behavior-state.
2. `IsDirty` — marker на персист/репликацию.

Правила:
1. Все мутации state идут через единый helper.
2. Любое реальное изменение state ставит `IsDirty=true`.
3. Flags не используются в этой модели.

### 5.2 Persistence contract

1. Персистим только `State` в `objects.data`.
2. Формат: versioned envelope `v=1` + `behaviors`.
3. Если state пустой, `objects.data = NULL`.
4. Сериализация только для `IsDirty=true`.
5. Ошибки сериализации/десериализации не игнорируются.

## 6. Performance Requirements

1. Нет глобального per-tick скана всех деревьев.
2. Изменения дерева только на событиях:
   - `Chop start`,
   - `cycle complete`,
   - `onChopped transform`.
3. Прогресс длительных действий не хранится в object state.
4. Нагрузка системы: `O(active actions)`, а не `O(all trees)`.

## 7. Object Def Format (Config Only)

### 7.1 Principles

1. Канон: поле `behaviors` (map).
2. Ключ map — string key behavior (`"tree"`).
3. В дефе только параметры; вся логика в коде behavior.
4. `priority` optional, default `100`, порядок `(priority ASC, behaviorKey ASC)`.
5. Strict decode для behavior config (`DisallowUnknownFields`).

### 7.2 Tree config fields

`behaviors.tree`:
1. `priority`
2. `chopPointsTotal`
3. `chopCycleDurationTicks`
4. `logsSpawnDefKey`
5. `logsSpawnCount` (`N`)
6. `logsSpawnInitialOffset` (`O`)
7. `logsSpawnStepOffset` (`P`)
8. `transformToDefKey`

### 7.3 Full JSONC example

```jsonc
{
  "v": 1,
  "source": "objects",
  "objects": [
    {
      "defId": 1,
      "key": "tree_birch",
      "static": true,
      "hp": 100,
      "components": { "collider": { "w": 10, "h": 10, "layer": 1, "mask": 1 } },
      "resource": "trees/birch/tree",
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
    },
    {
      "defId": 2,
      "key": "stump_birch",
      "static": true,
      "hp": 100,
      "components": { "collider": { "w": 10, "h": 10, "layer": 1, "mask": 1 } },
      "resource": "trees/birch/stump"
    },
    {
      "defId": 3,
      "key": "log_birch",
      "static": true,
      "hp": 1,
      "resource": "log"
    }
  ]
}
```

## 8. Acceptance Criteria

1. `Chop` доступен для дерева и недоступен для `stump` def.
2. `chop_points` создается при первом `Chop` и уменьшается на каждом cycle-complete.
3. На `chop_points == 0`:
   - дерево спавнит бревна,
   - объект меняет `type_id` на `stump`,
   - `object id` сохраняется.
4. Спавн логов идет линейно с параметрами `N/O/P` строго по одной оси (`X` или `Y`) в сторону, противоположную игроку.
5. После transform appearance пересчитывается и отправляется клиенту.
5. `Cyclic Action` поддерживает `self`/object target, single active action и `unlink -> canceled + clear`.
6. `ObjectInternalState` содержит только `State + IsDirty`.
7. `State` сохраняется как envelope `v=1`; при пустом state сохраняется `NULL`.
8. Нет полного per-tick скана всех деревьев в нагрузочном сценарии.
