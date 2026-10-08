# Дробное здоровье персонажа

`EntityHealth.SHP/HHP`, `CharacterSnapshot`, параметры sqlc и поля PostgreSQL
`character.shp/hhp` используют `float64` / `DOUBLE PRECISION` на всём пути
сохранения. Значения `21.4/24.28`, `81/13` и положительное `HHP=0.49`
сохраняются без округления. Поля protobuf остаются целочисленной проекцией
для отображения; из неё авторитетное здоровье не восстанавливается.

## Захват и загрузка

Захват требует живого handle, `EntityHealth`, `Transform` и `EntityStats`.
Здоровье проверяется до сериализации профиля и инвентарей: SHP/HHP должны быть
конечными и неотрицательными. Нет fallback `100/100`, нормализации отрицательных
значений или ограничения `MaxInt32`. Проверка здоровья использует один component
lookup и sentinel-ошибки, не выделяя память.

`Save` / `SaveDetached` возвращают ошибку захвата или enqueue. Успешный возврат
означает принятие snapshot очередью. `SaveSync` использует тот же захват, затем
ожидает транзакционную запись. Отказ захвата не изменяет ECS или предыдущий
валидный pending snapshot. Принятые снимки сохраняются вместе с инвентарями
в прежней транзакции; coalescing, порядок записей, worker count, batch limit
100 и retry после ошибки БД сохранены.

На загрузке выбирается переданное runtime-состояние, затем reconnect-cache,
затем БД. Только выбранные SHP/HHP проверяются до существующего ограничения
`HHP <= MHP`, `SHP <= HHP`; некорректное состояние прерывает spawn. Reconnect-cache
удаляется после attachment. Создание персонажа записывает вычисленный MHP
напрямую. Правила голода, административного урона и регенерации сохраняются.
Поза сохраняется отдельно, runtime-дедлайн KO по-прежнему не записывается в БД.

## Отказ финального захвата

Disconnect и detached expiry удаляют персонажа, owned inventories, tracking
	и пространственную регистрацию только после принятия финального snapshot.
Immediate disconnect при отказе переводит игрока в detached-состояние и
останавливает движение. Detached expiry сохраняет исходный `ExpirationTime`
и использует отдельный `SaveRetryAt`.

Повтор захвата назначается через 5 секунд по `ecs.TimeState.Now`. Periodic save
возвращает consumed-запись `PopDue` в расписание, сохраняя `LastSaveAt` и
`SavesCount`. Для detached-персонажа periodic и финальный захват согласуют
retry-дедлайн, чтобы одна система не повторяла попытку раньше срока другой.
Transfer с ошибкой `SaveSync` прекращается до удаления source
и захвата participant state. Shutdown захватывает все снимки под shard lock,
сообщает агрегированные отказы, освобождает lock и завершает drain валидной
очереди. Невалидные персонажи остаются в ECS до остановки процесса; их состояние
не объявляется сохранённым.

## Развёртывание

1. Завершить финальное сохранение старым сервером и остановить его. Проверить
   отчёт shutdown о сохранении персонажей.
2. Применить `migrations/20261007_fractional_character_health.sql` к нужной БД:

   ```sh
   psql "$ORIGIN_DATABASE_DSN" -v ON_ERROR_STOP=1 -f migrations/20261007_fractional_character_health.sql
   ```

   Миграция транзакционная, использует `lock_timeout=5s` и явный `USING`.
   `NOT NULL` сохраняется; CHECK каждого поля исключает отрицательные значения,
   NaN и обе бесконечности. Старые целые значения сохраняются точно. Повторное
   применение сохраняет уже записанные дробные значения. При ошибке миграция
   откатывается целиком.
3. Запустить новый binary. Смешанный запуск старых integer readers и новых
   fractional writers исключён. Перед rollback необходимо отдельно определить
   судьбу дробных данных; автоматического обратного округления нет.

## Проверки

PostgreSQL suites создают отдельные временные schema и удаляют их после теста.
Запуск без `ORIGIN_CHARACTER_SAVE_TEST_DSN` пропускает эти suites и не считается
приёмкой изменения.

```sh
export CGO_ENABLED=0
export GOCACHE=/private/tmp/origin-combat-go-cache
export ORIGIN_CHARACTER_SAVE_TEST_DSN='postgres://user:password@host/database?sslmode=disable'
go test ./internal/entityhealth ./internal/ecs ./internal/ecs/systems ./internal/game ./internal/restapi
make test
go test ./internal/ecs/systems -run '^$' -bench '^BenchmarkCharacter(HealthSnapshot|SaveCapture|SaveParams)$' -benchmem -count=5
git diff --check
```

Покрыты точные roundtrip, значения выше `MaxInt32`, ноль, минимальное положительное
и максимальное конечное `float64`, migration/replay/CHECK, загрузочный clamp,
отсутствие сериализации при invalid health, сохранение предыдущего pending
snapshot, periodic/disconnect/expiry/transfer/shutdown, coalescing, in-flight
порядок перед `SaveSync`, retry БД и rollback транзакции инвентарей. Временные
пороги benchmarks не являются CI-assertions.

## Производительность: 2026-10-07

Медианы пяти повторов с `-benchmem -count=5`, Go 1.27.1, Apple M3 Max, darwin/arm64,
`GOMAXPROCS=16`, `CGO_ENABLED=0`. Baseline снят до изменения runtime;
дополнительные варианты полного Save измерены в изолированном исходном коде
HEAD `b83894af3c2ae81b9af314d55e5cacd97c5a2ffa` с теми же fixtures.
У Save нет worker/БД; вариант с инвентарями возвращает заранее подготовленные
неизменяемые grid/hand/equipment. Batch построение измеряется отдельно.

| Benchmark | До, ns/op | После, ns/op | B/op до = после | allocs/op до = после |
| --- | ---: | ---: | ---: | ---: |
| Health snapshot | 58.67 | 33.92 | 0 | 0 |
| Save, без инвентарей | 3506 | 3480 | 2666 | 44 |
| Save, 3 инвентаря | 3516 | 3485 | 2666 | 44 |
| Batch 1, без инвентарей | 264.7 | 262.3 | 152 | 14 |
| Batch 1, с инвентарями | 625.8 | 612.4 | 544 | 32 |
| Batch 10, без инвентарей | 1827 | 1787 | 4488 | 67 |
| Batch 10, с инвентарями | 5718 | 5389 | 11280 | 134 |
| Batch 100, без инвентарей | 9380 | 9264 | 38888 | 109 |
| Batch 100, с инвентарями | 41981 | 41365 | 113152 | 471 |

Проверка здоровья быстрее после удаления округления. Рост времени более 10%
не обнаружен; небольшие различия остальных измерений не считаются гарантированным
ускорением. B/op и количество allocations сохранены. Два поля увеличились
с 4 до 8 байт каждое; с учётом выравнивания размер snapshot на этой платформе
увеличился со 168 до 176 байт, оставаясь в прежнем классе heap allocation.
Параметры batch уже использовали 8-байтные значения на этой платформе, поэтому
замена числового типа не увеличила их память.
