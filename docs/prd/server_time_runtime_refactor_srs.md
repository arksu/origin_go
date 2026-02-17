# SRS: Server Time Runtime Refactor

Version: 1.0  
Date: 2026-02-17  
Scope: Backend game server + network timestamps + dropped item decay

## 1. Purpose

Задать формальные требования к новой системе времени сервера для реализации coding-агентом без дополнительных уточнений.

## 2. Definitions

1. `Tick` — накопительный игровой счетчик тиков (`uint64`).
2. `RuntimeSeconds` — накопительное время работы сервера в секундах (`int64`), растет только при запущенном процессе.
3. `WallTime` — системное время (`time.Now()`), используется для сетевых timestamp.

## 3. Functional Requirements

### FR-1 Persisted State

Сервер обязан хранить в `global_var`:

1. `server_tick_total`
2. `server_runtime_seconds_total`

Оба значения должны трактоваться как единая пара состояния времени.

### FR-2 Bootstrap

При старте сервера:

1. Загружать оба ключа из БД.
2. Если отсутствует один из ключей — восстановить отсутствующий как `0` и продолжить.
3. Если отсутствуют оба — инициализировать оба нулями.
4. Если чтение БД не удалось — fail-fast.
5. Если значение любого ключа отрицательное — fail-fast.

### FR-3 Tick Continuity

`currentTick` должен стартовать из `server_tick_total` и продолжать инкремент без ресета.

### FR-4 Runtime Continuity

`RuntimeSeconds` должен стартовать из `server_runtime_seconds_total` и продолжать расти только при аптайме сервера.

### FR-5 Runtime Accumulation Rule

Runtime считается по реальному elapsed wall-time между итерациями loop.  
Даже при frame spike и ограничении catch-up по тикам runtime должен вырасти на весь elapsed интервал.

### FR-6 Tick Rate Independence

`RuntimeSeconds` не зависит от `tick_rate`.  
Изменение `tick_rate` между рестартами не требует special handling.

### FR-7 Periodic Persist

Сервер обязан выполнять periodic persist времени:

1. Отдельная горутина.
2. `ticker` каждые 20 реальных секунд.
3. Сохранять `server_tick_total` и `server_runtime_seconds_total` в одной транзакции.
4. Ошибки только логировать.

### FR-8 Final Persist on Stop

При shutdown обязателен финальный persist той же пары ключей в одной транзакции.  
Ошибка только логируется, shutdown продолжается.

### FR-9 Network Timestamp Domain

`server_time_ms` в сетевых пакетах должен быть wall unix milliseconds:

1. `S2C_Pong.server_time_ms`
2. `S2C_ObjectMove.server_time_ms`

### FR-10 Dropped Item Time Semantics

Поле JSON `drop_time` сохраняется без переименования, но семантика:

1. `drop_time` = runtime seconds на момент дропа.
2. Decay/expiration проверяется по runtime seconds.
3. Во время оффлайна decay не продвигается.

## 4. Non-Functional Requirements

### NFR-1 Logging

Periodic/final persist логирует только ошибки; информационные periodic логи запрещены.

### NFR-2 Consistency

Запись `server_tick_total` и `server_runtime_seconds_total` должна быть атомарной (одна транзакция).

### NFR-3 Compatibility

Обратная совместимость со старыми ключами `server_start_time`/`server_tick_rate` не требуется.

## 5. Out of Scope

1. Защита wall-time от rollback (NTP/manual adjustments).
2. Дополнительные админ-метрики/команды диагностики.
3. Миграция данных старой схемы (предполагается чистая БД).

## 6. Acceptance Criteria

1. После рестарта значения tick/runtime продолжаются из БД.
2. При выключенном сервере runtime не увеличивается.
3. Persist происходит каждые 20s и на stop, атомарно по двум ключам.
4. Клиент получает wall-based `server_time_ms` в Pong и ObjectMove.
5. Dropped items не истекают во время оффлайна.
