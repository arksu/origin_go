# Tree Growth: Final JSON Contract for Object Defs

## 1. Scope

Документ фиксирует финальный JSON contract для `behaviors.tree` в `object defs`.

Принятые решения:
1. Финальный флаг задается как `tree.stageN` (отдельный `tree.stage_final` не используется).
2. Стадии, где доступен `chop`, задаются явным списком (`allowedChopStages`).
3. При catch-up применяется только финальное appearance-состояние.

## 2. Contract (behaviors.tree)

Поля, уже существующие в tree behavior (рубка):
1. `priority` (`int`, optional, default `100`)
2. `chopPointsTotal` (`int`, required, `> 0`)
3. `chopCycleDurationTicks` (`int`, required, `> 0`)
4. `logsSpawnDefKey` (`string`, required, non-empty)
5. `logsSpawnCount` (`int`, required, `> 0`)
6. `logsSpawnInitialOffset` (`int`, required, `>= 0`)
7. `logsSpawnStepOffset` (`int`, required, `> 0`)
8. `transformToDefKey` (`string`, required, non-empty)
9. `action_sound` (`string`, optional)
10. `finish_sound` (`string`, optional)

Новые поля роста:
1. `growthStageMax` (`int`, required, `>= 1`)
   - максимальный номер стадии;
   - финальная стадия = `growthStageMax`.
2. `growthStartStage` (`int`, optional, default `1`, range `1..growthStageMax`)
   - стартовая стадия для новых объектов в spawn/init-потоке.
   - не применяется к восстановлению объекта без persisted state из базы (для этого действует политика `final-stage-on-missing-state`).
3. `growthStageDurationsTicks` (`array<int>`, required)
   - длина массива строго `growthStageMax - 1`;
   - элемент `i` (0-based) — тики перехода из стадии `i+1` в `i+2`;
   - все значения `> 0`.
4. `allowedChopStages` (`array<int>`, required)
   - явный список стадий, где доступен `chop`;
   - каждое значение в диапазоне `1..growthStageMax`;
   - дубликаты запрещены;
   - список может быть пустым, если рубка на этом дереве должна быть недоступна.

## 3. Runtime Derivation Rules

1. Текущий stage-флаг формируется автоматически как `tree.stage{stage}`.
2. Одновременно активен только один stage-флаг.
3. На финальной стадии (`stage == growthStageMax`) дерево больше не планируется на рост.
4. `chop` доступен только если `currentStage` входит в `allowedChopStages`.
5. Если persisted `behaviors.tree` state отсутствует (объект создан генератором карты), runtime трактует дерево как финальную стадию (`stage = growthStageMax`) без schedule.

## 4. Validation Rules (strict)

1. `DisallowUnknownFields` обязателен.
2. `growthStageMax >= 1`.
3. `growthStartStage` в диапазоне `1..growthStageMax`.
4. `len(growthStageDurationsTicks) == growthStageMax - 1`.
5. Все `growthStageDurationsTicks[i] > 0`.
6. Все `allowedChopStages[i]` уникальны и в диапазоне `1..growthStageMax`.

## 5. Persisted Tree Runtime State (objects.data -> behaviors.tree)

Минимальный state:

```json
{
  "v": 1,
  "behaviors": {
    "tree": {
      "stage": 2,
      "next_growth_tick": 123456
    }
  }
}
```

Правила:
1. `stage` — текущая стадия `1..growthStageMax`.
2. `next_growth_tick` присутствует только если `stage < growthStageMax`.
3. Для финальной стадии допускается отсутствие `next_growth_tick`.
4. Отсутствие блока `behaviors.tree` в persisted state интерпретируется как политика `final-stage-on-missing-state`.

## 6. Full JSONC Example

```jsonc
{
  "v": 1,
  "source": "objects",
  "objects": [
    {
      "defId": 1,
      "key": "tree_birch",
      "name": "Birch Tree",
      "static": true,
      "hp": 100,
      "contextMenuEvenForOneItem": true,
      "components": {
        "collider": {
          "w": 10,
          "h": 10,
          "layer": 1,
          "mask": 1
        }
      },
      "resource": "trees/birch/1",
      "appearance": [
        { "id": "stage1", "when": { "flags": ["tree.stage1"] }, "resource": "trees/birch/1" },
        { "id": "stage2", "when": { "flags": ["tree.stage2"] }, "resource": "trees/birch/2" },
        { "id": "stage3", "when": { "flags": ["tree.stage3"] }, "resource": "trees/birch/3" },
        { "id": "stage4", "when": { "flags": ["tree.stage4"] }, "resource": "trees/birch/4" }
      ],
      "behaviors": {
        "tree": {
          "priority": 20,
          "chopPointsTotal": 6,
          "chopCycleDurationTicks": 20,
          "logsSpawnDefKey": "log",
          "logsSpawnCount": 2,
          "logsSpawnInitialOffset": 16,
          "logsSpawnStepOffset": 20,
          "transformToDefKey": "stump_birch",
          "action_sound": "chop",
          "finish_sound": "tree_fall",

          "growthStageMax": 4,
          "growthStartStage": 1,
          "growthStageDurationsTicks": [600, 900, 1200],
          "allowedChopStages": [2, 3, 4]
        }
      }
    }
  ]
}
```
