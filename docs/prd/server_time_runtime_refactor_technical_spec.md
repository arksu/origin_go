# Technical Specification: Server Time Runtime Refactor

Version: 1.0  
Date: 2026-02-17  
Audience: Coding agent (clean context)

## 1. Implementation Targets

### 1.1 Files to change

1. `internal/const/const.go`
2. `internal/timeutil/server_time.go`
3. `internal/timeutil/clock.go`
4. `internal/ecs/resources_time_movement.go`
5. `internal/game/game.go`
6. `internal/ecs/systems/drop_decay.go`
7. `internal/game/inventory/operations.go`
8. `internal/game/inventory/give_item.go`
9. `internal/game/world/object_factory.go`
10. Все producer-места `server_time_ms` (в первую очередь `game.go`, `ecs/systems/transform.go`, `game/events/game_events.go` по цепочке данных)
11. Тесты, затрагивающие `TimeState`, bootstrap и dropped item decay

### 1.2 New global_var keys

1. `server_tick_total`
2. `server_runtime_seconds_total`

## 2. Data Model

### 2.1 Persisted server time state

```text
server_tick_total             BIGINT >= 0
server_runtime_seconds_total  BIGINT >= 0
```

Сохраняются и обновляются вместе, транзакционно.

### 2.2 Runtime state in Game

Минимально необходимые поля:

1. `currentTick uint64`
2. `runtimeSecondsTotal int64`
3. `runtimeRemainder time.Duration` (опционально, для накопления sub-second)
4. `lastWallTime time.Time` (для расчета elapsed)

### 2.3 TimeState contract

Разделить домены:

1. Tick domain:
   - `Tick`
   - `TickRate`
   - `TickPeriod`
   - `Delta`
2. Runtime domain:
   - `RuntimeSecondsTotal`
   - `Uptime` (если нужно как duration)
3. Wall domain:
   - `WallNow`
   - `WallUnixMs` (или сохранить имя `UnixMs`, но семантика строго wall)

## 3. Algorithms

### 3.1 Bootstrap algorithm

1. Read `server_tick_total`, `server_runtime_seconds_total`.
2. If DB read error -> return error (fail-fast caller).
3. Missing key handling:
   - If both missing: write both zero in one tx.
   - If one missing: write missing as zero in one tx.
4. Validate both values >= 0.
5. Return loaded/initialized state.

### 3.2 Runtime accumulation algorithm

Per loop iteration:

1. `elapsed = nowWall - lastWallTime`
2. clamp negative elapsed to zero
3. `runtimeAcc += elapsed`
4. `runtimeSecondsAdded = floor(runtimeAcc.Seconds())` via duration arithmetic
5. `runtimeSecondsTotal += runtimeSecondsAdded`
6. keep remainder `< 1s` in accumulator

Важно: делается независимо от того, сколько тиков реально отработано в catch-up цикле.

### 3.3 Periodic persist algorithm

Отдельная goroutine:

1. ticker `20s` real time
2. read current `tick/runtime` snapshot thread-safe
3. write both keys in one DB transaction
4. on error: `logger.Error(...)`, continue

### 3.4 Stop persist algorithm

В `Stop()`:

1. остановить periodic goroutine
2. взять финальный snapshot
3. одна tx на запись двух ключей
4. on error: log only, continue shutdown

### 3.5 Dropped item time algorithm

1. На spawn dropped item: `drop_time = RuntimeSecondsTotal`.
2. Expiration check:
   - `expired if drop_time + DroppedDespawnSeconds <= RuntimeSecondsTotalNow`
3. Нельзя использовать wall-time в drop decay.

### 3.6 Network timestamp algorithm

На отправке `server_time_ms`:

1. `server_time_ms = time.Now().UnixMilli()` (или из текущего tick-local wall value, если already computed)
2. Единый домен для Pong и ObjectMove.

## 4. Concurrency and Safety

1. Persist goroutine не должна блокировать game loop.
2. Snapshot shared state (`tick/runtime`) читать под mutex/atomic-safe способом.
3. DB writes for pair keys only in transaction (`WithTx` + 2 upserts).
4. Ошибка periodic/final persist не меняет state machine сервера.

## 5. Logging Rules

1. periodic success logs: forbidden.
2. periodic/final persist errors: required.
3. bootstrap errors: explicit and actionable (fail-fast path).

## 6. Test Plan (minimum)

1. Bootstrap:
   - both keys missing
   - only one key missing
   - negative values
   - DB read error
2. Continuity:
   - tick/runtime continue after restart
3. Runtime accumulation:
   - frame spike increases runtime by full elapsed
4. Persist:
   - periodic writes every 20s (mock clock/ticker)
   - final write on stop
   - tx writes both keys together
5. Network:
   - Pong/ObjectMove `server_time_ms` wall-based
6. Dropped items:
   - decay by runtime seconds
   - no offline decay progression

## 7. Definition of Done

1. Все FR/NFR из SRS выполнены.
2. Нет runtime-use of `server_start_time`/`server_tick_rate`.
3. Все unit/integration тесты проходят.
4. Документация AGENTS updated to new contract.
