**PRD: Контекстное меню взаимодействий (RMB) на базе behaviors**

## 1. Problem Statement
Текущая механика открытия контейнеров имеет отдельный flow (`PendingAutoOpen` + `LinkCreated`), а интеракции в целом не масштабируются единообразно. Нужен общий серверный pipeline для всех действий объекта через контекстное меню и behaviors.

## 2. Goals
1. Единый вход: ПКМ по объекту всегда запускает вычисление контекстных действий.
2. Единый execute-flow для всех действий, включая `open`.
3. Детерминированная агрегация действий из behaviors.
4. Мобильный-friendly UX ошибок через мини-алерты по центру.

## 3. Non-Goals
1. Локализация текстов мини-алертов.
2. Телеметрия rollout/эксперименты.
3. Исключения из линковки для отдельных действий.

## 4. Core Flow

1. Игрок делает ПКМ по объекту.
2. Сервер выполняет `ComputeContextActions(entity)`:
    - обходит все behaviors в порядке из `def`;
    - каждое behavior смотрит state, валидирует по своему контракту, возвращает actions.
3. Агрегация:
    - действия суммируются;
    - при duplicate `action_id`: `first wins`, пишется `WARN`, инкремент метрики дубликатов.
4. Развилка:
    - `0 actions`: полный игнор (ничего не отправляем клиенту),
    - `1 action`: auto-select, сразу execute pipeline,
    - `2+ actions`: отправка меню, после выбора execute pipeline.
5. Execute pipeline:
    - создается pending intent/action;
    - запускается движение к объекту;
    - исполнение триггерится **только по факту `LinkCreated`**;
    - повторная валидация внутри behavior (`EXECUTE`);
    - вызов `behavior.execute(action_id)`;
    - cleanup pending state.
6. Если действие к моменту исполнения невалидно/пропало: полный игнор (без ответа клиенту).

## 5. Link & Pending Lifecycle

1. Для всех действий link обязателен, без исключений.
2. Таймаут ожидания `LinkCreated` для pending: `15s` (вынести в конфиг).
3. `PendingInteraction` очищается на:
    - новый `MoveTo`,
    - новый `MoveToEntity`,
    - `LinkBroken`,
    - despawn target/player,
    - stop movement.
4. Поведение очистки должно быть эквивалентно текущей механике open containers по полноте cancel-path.

## 6. Validation Contract (Variant B)

### 6.1 Behavior API
1. `ProvideActions(ctx) -> []ActionDescriptor`
2. `Validate(ctx, action_id, phase) -> ValidationResult`, `phase: PREVIEW | EXECUTE`
3. `Execute(ctx, action_id) -> ExecuteResult`

### 6.2 Standard Result Types
`ValidationResult` / `ExecuteResult`:
1. `ok: bool`
2. `reason_code: string/enum`
3. `severity: info | warning | error`
4. `user_visible: bool`

### 6.3 Rules
1. `EXECUTE`-валидация обязательна перед `Execute`.
2. UI-решения принимаются только через общий результат контракта.
3. Behaviors не отправляют UI напрямую в обход контракта.

## 7. UX: Mini Alerts (center-screen)

1. Показываются только при явных ошибках/отказах (`user_visible=true`).
2. Цвета:
    - `error` — красный, TTL `2500ms`
    - `warning` — желтый, TTL `2000ms`
    - `info` — нейтральный/синий, TTL `1500ms`
3. Anti-spam:
    - debounce key: `player_id + reason_code`
    - coalesce identical
    - max одновременно: `3`
4. Текст:
    - клиент получает только `reason_code`
    - локализация не используется (mapping reason_code -> raw message/label на клиенте по фиксированному словарю).

## 8. Data/Config Requirements

1. Конфиг:
    - `interaction_pending_timeout_ms = 15000`
2. Словарь reason codes (единый для behaviors).
3. Канонический порядок behaviors берется из `def`.

## 9. Observability

1. `WARN` лог при duplicate action.
2. Метрика:
    - `context_action_duplicate_total`
    - теги: `entity_def`, `action_id`, `winner_behavior`, `loser_behavior`.

## 10. Migration Requirement

1. Удалить/выключить отдельный auto-open контейнерный путь (`PendingAutoOpen` как execution-механизм).
2. `open` становится обычным context action в общем pipeline.

## 11. Acceptance Criteria

1. ПКМ всегда проходит через compute actions по behaviors.
2. `0/1/2+` действия обрабатываются строго по правилам (ignore/auto/menu).
3. Любое исполнение запускается только после `LinkCreated`.
4. Pending очищается во всех согласованных cancel-path + по timeout 15s.
5. Duplicate actions не ломают flow (`first wins`) и попадают в WARN + метрику.
6. Ошибки/отказы отображаются центровыми mini-alert с заданными severity/TTL/debounce/maxN.
7. `open` работает через новый общий pipeline, без отдельной legacy execution-ветки.

## 12. Current Implementation (as of this refactor)

1. Единый контракт behavior находится в `internal/types/behavior.go`.
2. Все реализации поведений вынесены в `internal/game/behaviors`:
    - `container` (runtime flags + context actions),
    - `tree` (context actions + cyclic + object transform/spawn helpers),
    - `player` (base behavior key).
3. Единый runtime-реестр находится в `internal/game/behaviors/registry.go` и используется как:
    - источник исполнения behavior,
    - источник валидации behavior-ключей при `objectdefs.LoadFromDirectory(...)`.
4. `Fail Fast` включен на уровне реестра:
    - action-capable behavior обязан декларировать action specs,
    - behavior с `Provide/Validate` обязан реализовывать `Execute`,
    - action с `StartsCyclic=true` требует cyclic capability.
5. Lifecycle init hooks поддерживаются и вызываются в трех сценариях:
    - `spawn` (создание новых объектов),
    - `restore` (активация чанка + восстановление из persistence),
    - `transform` (смена типа объекта, например tree -> stump).
6. Legacy behavior split удален:
    - больше нет раздельных `contextActionBehavior` / `RuntimeBehavior` / `cyclicActionBehavior` сервисных контрактов в runtime-системах.
