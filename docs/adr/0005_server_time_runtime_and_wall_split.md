# ADR 0005: Server Time Model Refactor (Tick + Runtime + Wall)

- Status: Proposed
- Date: 2026-02-17
- Owners: Game Server

## 1. Context

Текущая модель времени основана на `server_start_time`, поэтому игровое время продолжается во время оффлайна сервера.  
Это противоречит требованиям:

1. Время сервера должно идти только пока сервер запущен.
2. `tick` и server runtime должны храниться отдельно.
3. Сетевое `server_time_ms` должно быть wall-time (для RTT/time sync клиента).
4. `drop_time` для dropped items должен паузиться при оффлайне.

## 2. Decision

Вводится трехдоменная модель времени:

1. `Tick` — дискретное игровое время (`uint64`), накопительное.
2. `Runtime` — накопленное время работы сервера в секундах, только пока процесс запущен.
3. `Wall` — реальное системное время (`time.Now()`), используется для сети.

Persist в `global_var`:

1. `server_tick_total` (`BIGINT`)
2. `server_runtime_seconds_total` (`BIGINT`)

Требования к persist:

1. Периодический flush каждые 20 реальных секунд.
2. Отдельная горутина с `ticker`.
3. Финальный flush при `Stop()`.
4. Запись пары ключей атомарно в одной транзакции.
5. Ошибки записи только логируются (без остановки сервера/блокировки shutdown).

Bootstrap:

1. Чтение обоих ключей из БД.
2. Если один ключ отсутствует — недостающий восстанавливается как `0`.
3. Если оба отсутствуют — инициализация нулями.
4. Ошибка чтения БД — fail-fast.
5. Отрицательные persisted значения — fail-fast.
6. Несовпадение `tick_rate` с прошлым запуском игнорируется.

Сеть:

1. `S2C_Pong.server_time_ms` = wall unix ms.
2. `S2C_ObjectMove.server_time_ms` = wall unix ms.
3. Все producer-места `server_time_ms` используют один домен (`Wall`).

Dropped items:

1. JSON-поле `drop_time` остается без переименования.
2. Семантика `drop_time` = runtime seconds.
3. Decay сравнивается с runtime seconds (пауза во время оффлайна гарантирована).

## 3. Rationale

1. Разделение доменов исключает смешение сетевого и игрового времени.
2. Runtime-based decay корректно моделирует "мир заморожен при оффлайне".
3. Wall-based network time сохраняет корректную работу клиентского `TimeSync` и `MoveController`.
4. Периодический + финальный persist минимизирует потерю прогресса времени при падениях.

## 4. Consequences

Positive:

1. Время сервера перестает "убегать" во время оффлайна.
2. `tick` и runtime продолжаются между рестартами.
3. Клиентская интерполяция и RTT оценка остаются корректными.

Trade-offs:

1. Усложняется контракт `TimeState` (два домена времени вместо одного).
2. Добавляется фоновая горутина persist.
3. Нужна миграция всех систем на явный выбор домена времени.

## 5. Rejected Alternatives

1. Сохранять только tick и пересчитывать runtime через tick_rate.
   - Отклонено: runtime не должен зависеть от изменения tick_rate.
2. Использовать runtime-domain для `server_time_ms` в сети.
   - Отклонено: клиент считает RTT/offset через wall-модель.
3. Не делать periodic persist, только stop persist.
   - Отклонено: высокий риск потери прогресса при crash.

## 6. Acceptance Criteria

1. После рестарта `server_tick_total` и `server_runtime_seconds_total` продолжаются с persisted значений.
2. `server_runtime_seconds_total` увеличивается только при запущенном сервере.
3. Persist происходит каждые 20 секунд и на `Stop()` атомарно по двум ключам.
4. `server_time_ms` в Pong/ObjectMove соответствует wall unix ms.
5. Dropped item decay не идет во время оффлайна и продолжается после старта.
