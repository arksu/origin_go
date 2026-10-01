# WASD: результаты реализации и проверки

Дата: 2026-10-01. Основа working tree: `c7746ba`; реализация не закоммичена.

## Статус

Реализованы задачи 1–7 и выполнен scope review 8.5. **36/36** задач закрыты. Пункты 8.1–8.4 закрыты по подтверждению пользователя от 2026-10-01: «я провел ручные тесты. все ок». Ручную проверку выполнил пользователь; агент не запускал browser smoke из-за недоступного браузера. Протокол и запись результата: [manual-smoke.md](manual-smoke.md). Численные замеры задержки, параметры среды и вывод harness пользователем не переданы; отчёт не приписывает им конкретных значений.

Реализация и приёмка по плану завершены; ожидание ручной проверки снято. Rollout остаётся отдельным действием по команде пользователя. Агент не развёртывал сервер/клиент и не перезапускал пользовательские процессы. Коммит не создавался. Спецификации синхронизированы; change архивирован в `openspec/changes/archive/2026-10-01-add-wasd-movement/`.

## Реализация

- Additive `MoveDirection` + capability; сервер проверяет wire values, текущую сессию/эпоху, revisions и lease 800 мс от receipt с учётом очереди.
- Направление проходит существующий movement/collision/transform pipeline. Полный упор сохраняет действительный intent, возвращает idle/нулевую velocity, повторяет collision probe и подавляет только неизменившийся blocked update.
- Cleanup использует существующие action/link/craft APIs; не переносит ephemeral input через spawn/transfer. Fresh input отменяет незавершённую попытку, selecting сохраняется.
- Физические WASD → экранное направление → нормализованный мировой float-вектор. Изменения отправляются сразу, подтверждение удержания — раз в 200 мс одним таймером. Handoff/reset подавляет старые зажатые клавиши.
- Клиент использует существующий server movement stream. `MoveController`, `TimeSync` и locomotion durations не изменены.

## Автоматические проверки

| Команда / проверка | Результат |
|---|---|
| `make proto`; `npm --prefix web_new run proto` | PASS; повторная генерация дала идентичные SHA-256 Go/JS/TS bindings |
| Go wire/component/ingress/session/TTL/collision/action/bootstrap tests | PASS |
| `go test ./internal/... ./cmd/load_test/...` | PASS |
| `go test -race ./internal/network ./internal/network/proto ./internal/ecs/systems ./internal/game -run 'Direction\|PlayerActionRoutes\|MapClick\|CommandQueue\|CommandInbox' -count=1` | PASS во всех четырёх пакетах |
| `go build ./cmd/gameserver ./cmd/load_test` | PASS |
| `npm --prefix web_new run test:movement-input` | PASS, 20 tests, включая protocol и movement batches |
| `npm --prefix web_new run test:map-click` | PASS, 3 tests |
| `npm --prefix web_new run test:actions` | PASS |
| `npm --prefix web_new run test:chunks` | PASS, 9 tests |
| `npm --prefix web_new run test:action-animations` | PASS, 9 tests |
| `npm --prefix web_new run test:character-visual` | PASS, 24 tests |
| `npm --prefix web_new run build` | PASS |
| `go test ./internal/game/events -run '^TestPeriodicMovementBatchesDeliverEveryEntry$' -count=1 -v` | PASS, 3 сценария, настоящая WebSocket-доставка |
| `git diff --check`; `openspec validate add-wasd-movement --strict` | PASS |

При одном повторном race-запуске sandbox запретил чтение Go build cache (`operation not permitted`); тот же запуск вне sandbox завершился успешно. Клиентский build сообщает о vendor chunk больше 600 kB; bundling не менялся. PostgreSQL integration tests требуют отдельного `ORIGIN_CHARACTER_SAVE_TEST_DSN`; этот прогон не заявляется как проверка БД. Изменений persistence/DB в change нет.

Покрыты: rollover и tombstones; идентичность wire heartbeat; очередь до/после deadline; blocked→idle→resume/expire; восемь направлений и speed modes; ограничения stamina/carry/stun/KO; единственный physical step/stamina charge при нескольких командах; отсутствие point-arrival и target marker; стена/угол/вода/phantom/world bounds; chunk crossing с carry; selecting/approach/последний тик repeating cycle; lift/put-down/tile cancellation и late callback; linkless craft; session replacement/detach/epoch; реальный transfer detach/reattach boundary; bootstrap capability/start/stop. Полный teleport/rollback с UI включён в ручной smoke, принятый по подтверждению пользователя.

## CPU, allocations, очередь и пакеты

Команда:

```sh
go test ./internal/ecs/systems -run '^$' \
  -bench 'Benchmark(MovementPipeline|DirectionalMovementPipeline|ClickMovementPipelineBlocked)' \
  -benchmem -benchtime=500ms -count=3
```

Darwin arm64, Apple M3 Max; 200 movers, существующая геометрия benchmark. Диапазон трёх прогонов:

| Сценарий | µs/op | B/op | allocs/op | Movement entries/tick |
|---|---:|---:|---:|---:|
| Click active | 308.5–313.5 | 28 351–28 370 | 604 | 200 |
| Direction active | 313.4–317.4 | 21 947–21 950 | 4 | 200 |
| Click dense/sliding | 439.6–440.9 | 28 292–28 293 | 604 | 200 |
| Direction dense/sliding | 438.5–445.7 | 21 890–21 891 | 4 | 200 |
| Click blocked, повторная попытка | 423.0–436.3 | 45 898–45 900 | 604 | 200 |
| Direction blocked, устойчивый упор | 310.0–310.7 | 0 | 0 | 0 |

Benchmark включает сброс fixture и Movement → Collision → Transform, но не весь shard tick и не сетевой fanout. Для blocked click исходная цель восстанавливается перед каждой попыткой: это сравнение цены повторного probe, не утверждение, что обычный idle click-клиент постоянно генерирует 200 updates. Direction сохраняет уже установившийся blocked state после прогрева. Активные медианы отличаются примерно на 2.6%, sliding — менее чем на 1%; это локальный микробенчмарк, не capacity/SLA сервера.

`TestDirectionPopulationInputQueueBounded`: 200 игроков, 10 Hz, 2 с модельного времени, стандартные лимиты 40 packets/s, 20 commands/tick/client, queue 500. Обработаны все **2200** команд (start + heartbeat + release), dropped **0**, burst **200**, backlog после каждого drain **0**. Fake timers клиента подтверждают **5 refreshes/s** независимо от FPS, без idle packets/catch-up burst и с очисткой после destroy/reset.

`TestPeriodicMovementBatchesDeliverEveryEntry`: 100 активных directional игроков на одного observer дают **1 WebSocket message / 100 entries / 2762 protobuf bytes за проход**, как существующая active movement fixture. Первый полный stop — **1 message / 100 entries**, повторный неизменившийся упор — **0 messages**. Все entries уникальны, последовательность `move_seq` корректна, `targetPosition` отсутствует. Это отдельный сетевой fixture; entries микробенчмарка не выдаются за число сетевых пакетов.

## Scope review

Production diff ограничен schema/bindings, directional runtime/handler/validator, movement/transform, action adapter, shard wiring, bootstrap capability и клиентским input/lifecycle/handoff. Новых dependencies, DB/schema, atlas/assets, общей очереди или интерполяционного алгоритма нет. JS/TS protobuf генерируются в существующую gitignored директорию; Go binding отслеживается как раньше.

Тестовые изменения переиспользуют имеющиеся fixture/runner; LoadWorkers в lift fixture включён для проверки реального перехода между загруженными чанками. Новые документы фиксируют проверку и ручную приёмку. Изначальные `.zcodeignore`, `art_source/previews/`, `docs/features/minimap.md` не затронуты.

Локальные логи этого прогона: `/tmp/origin-wasd-go-tests.log`, `/tmp/origin-wasd-race.log`, `/tmp/origin-wasd-client-build.log`, `/tmp/origin-wasd-bench.log`. Численные результаты сохранены выше, поскольку `/tmp` не является постоянным хранилищем.
