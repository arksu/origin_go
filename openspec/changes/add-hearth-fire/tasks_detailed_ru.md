# Hearth Fire: подробный план реализации

Дата сверки с кодом: 2026-10-16.

Это пояснение задач из [tasks.md](tasks.md), а не новая редакция
игровых требований и не отчёт о выполнении. Нумерация совпадает с основным
списком. Источники правил: [design.md](design.md), delta-specs в `specs/`
и `docs/features/hearth_fire.md`, часть 2. Реализованные контракты отмечены
отдельно; остальные пункты описывают будущие этапы.

**Статус реализации:** готовы задачи 1.1–1.4, рецепт 2.1 и runtime-завершение
стройки 2.2–2.5, проход владельца 3.2, стандартные tile rules 3.3,
структурные проверки ручной постановки и строительства 3.4, подготовка
metadata/нормализация перед activation 3.5 и синхронное сохранение стройки 4.3.
Синхронная очистка при смерти и REST-удалении реализована в 4.1–4.3a;
проверки этого этапа фиксируются отдельно в `tasks.md`.
Фактические SQL и runtime API описаны в
`docs/features/hearth_fire.md`, разделах 2.13–2.23:
`character_hearth.sql`, migration/schema и `internal/game/hearth/index.go`.
Ранее прошли целевые Hearth/construction tests, unit-тесты записи и полный
`GOCACHE=/tmp/origin-hearth-go-cache make test`, включая этап 4.3;
regeneration не изменил generated-файлы. Тесты используют реальные каталоги
и заглушки на границах чтения владения/chunk workers/persistence; PostgreSQL tests
пропущены, поскольку `ORIGIN_OBJECT_HEALTH_TEST_DSN` не задан. Проверка с живой
БД на этом этапе не заявляется.
Обычный рецепт добавлен в `data/builds/basic.jsonc`; catalog/runtime tests
проверяют число циклов из конфигурации рецепта, solo и совместные затраты.
Подключены одна optional capability `CompleteConstruction`, surface-only
`hearth.Runtime` и синхронная подготовка владения/прежнего чанка в том же
вызове завершения; отдельного Hearth apply hook в `Shard.Update` нет.
Задача 3.5 реализована и проверена, включая упрощённый индекс, PostgreSQL
integration и финальный полный `make test`; результаты приведены в 3.5.
Ранее указанное отсутствие DSN относится только к предыдущему этапу.
Return и FX остаются отдельными будущими этапами.

## Базовые гарантии

**Главная гарантия: целостность текущего игрового состояния в памяти.**
У одного персонажа не может быть двух действующих домов. Достроивший получает
свой очаг сразу, старый теряет статус очага, повторный callback не меняет
владельца, чужой handle не затрагивается. БД не принимает эти решения вместо
игрового runtime.

**Сохранение стройки: синхронное после runtime-перехода.**
Итоговые объекты, связь и ID watermark сохраняются одной транзакцией;
успех возвращается после commit. Персонаж, инвентари, материалы, stamina
сохраняются независимо. Hearth terminal-переходы также используют
синхронный SQL на owning surface path, с отдельным five-second context для
каждого чтения/транзакции. После аварии их
несохранённые изменения могут откатиться; общего checkpoint и WAL операций нет.
БД считаем доступной. Любая ошибка Hearth SQL, включая неизвестный
результат commit, требует полной остановки сервера. Ошибка предшествующего
account-scoped DELETE сохраняет HTTP 500. SQL retry, reconnect
и восстановление после ошибки БД не добавляются. Штатное отсутствие строки
не является ошибкой; невалидный результат запроса фатален.

**Восстановление: безопасное использование сохранённого, без угадывания.**
Валидная сохранённая связь восстанавливает дом. Невалидная даёт отсутствие
дома; осиротевший зелёный очаг становится обычным незажжённым костром до
видимости. Не восстанавливаем достроившего по догадкам и не переигрываем
несохранённый финальный цикл. Материалы и очаг также могут сохраниться в
разные моменты: финансовой гарантии «списание и результат всегда вместе» нет.

Допустимый откат после аварии не означает допустимость повреждения живого
runtime, гонок, неограниченных очередей или игнорирования ошибок. Существующие
гарантии других механик, в частности удаления объектов и переноса инвентарей,
не ослабляем ради упрощения Hearth.

## Граница между фичей и ядром

Hearth-модуль будет владеть связью персонажа с домом, её загрузкой и сохранением,
правилами превращения в костёр и обработчиками возврата. Его предполагаемое
место: `internal/game/hearth`; behavior-adapter может находиться в
`internal/game/behaviors`. Сборка зависимостей остаётся в composition root
игры. Фича не импортирует родительский пакет `game`, создавая цикл импортов:
используются небольшие interfaces/функции на границе уже существующих сервисов.

Есть важная граница текущих Go-пакетов: `ActionHandler`/`ActionResult` сейчас
объявлены в пакете `game`. Поэтому тонкий adapter возврата может остаться
в отдельном feature-файле этого пакета, делегируя policy Hearth-модулю.
Это не ветка в `ActionService` и не причина переносить все action contracts
или переписывать существующие handlers ради красивого дерева каталогов.

В общие `BuildBehaviorState`, action/transfer state и ECS-компоненты не добавляем
владельца очага, достроившего, Hearth-operation ID или проверку ключа
`hearth_fire`. `behaviors.hearth_construction` и сохранённый finisher тоже не
нужны: оплату последней ветки и готовый результат не разделяет ожидание БД.

### Когда правка ядра действительно необходима

Для каждого изменения общий код должен иметь понятную самостоятельную
ответственность. Сначала проверяем существующие helper, optional capabilities
registry и lifecycle hooks. Если они уже решают задачу, новый контракт не
добавляем. Если нет, допускается узкая точка вызова политики, но не новая
платформа для будущих гипотетических фич.

| Область | Минимально допустимая правка | Почему без неё нельзя |
| --- | --- | --- |
| Стройка | Одна optional wrapper-capability результата и передача `bool` transform | Wrapper подготавливает дом до финальных затрат и вызывает обычное завершение с прежним порядком link hooks |
| Размещение и коллизии | Стандартные tile rules рецепта, независимые structural checks и callback столкновения с субъектом | Tile rules используют collider результата; проход владельца не разрешает ставить предметы на очаг |
| Raw cache | Точечное чтение raw по ID, `PinPersistenceBlocking` поверх existing gate/pins и `UpsertRawObject` | Nonblocking pin вернул бы busy; блокирующий вариант удерживает чанк, но освобождает mutex для loader до `WaitPreloaded` |
| Активация чанка | Один optional callback после existing gate/state guards до создания raw ECS; синхронные batch read и orphan transaction | Без подготовленного владельца и commit нормализации нельзя публиковать очаг; существующий per-object reconcile не объединяет SQL до создания entities |
| Удаление объекта | Зарегистрированная removal policy до изменения runtime или SQL, только там, где существующего запрета недостаточно | Одной защиты от урона недостаточно для отдельного admin deletion path |
| Смерть и REST | Нейтральное уведомление о принятом terminal transition и синхронный completion после source unlock | Фича должна узнать о смерти и завершить save до возврата owning call; вызов под source lock вложил бы две shard locks |
| Перенос игрока | Source validation, разрешение цели и уведомление об успешном relocation | Admission асинхронного переноса не является успехом, а координаты могли устареть |
| Действия | Optional interruption policy текущего handler до игровых отказов | Отказ другого действия всё равно должен прерывать возврат, не меняя правила остальных действий |
| Протокол FX | Только поле `stream_epoch = 3` в существующем `S2C_Fx` | World-event из прошлого входа не должен отобразиться в новом мире |

Это перечень границ, а не разрешение заранее реализовать девять новых
абстракций. Перед каждой соответствующей задачей проверяется минимальный
конкретный diff. Если для неё неожиданно требуется общий transaction engine,
новый job registry или изменение семантики других фич, реализацию этой части
нужно остановить и отдельно согласовать корректировку плана.

## 1. Данные владения и runtime

### 1.1. Таблица действующей связи

**Что сделаю.** Добавлю одну ещё не развёрнутую миграцию
`migrations/20261012_character_hearth.sql` и ту же структуру в
`migrations/schema.sql`. В `character_hearth` будут только `character_id`,
`object_region`, `object_id`. PK по персонажу даёт максимум один сохранённый
дом; UNIQUE по объекту запрещает двух сохранённых владельцев; составной FK
соответствует реальному partitioned ключу `object(region, id)`. Оба FK используют
RESTRICT. Игровая смерть остаётся soft delete через `character.deleted_at`.

**Граница.** Не меняю модель общего владения `object.owner_id`, не дублирую
координаты или тип объекта. Никаких first-entry markers, compatibility/backfill,
автоматического очищения БД и таблиц pending-операций. Ограничения БД защищают
сохранённые строки, но сами не проверяют тип, поверхность или живого владельца.

**Проверки.** PostgreSQL-тесты на вторую связь, второго владельца, несуществующие
FK и физическое удаление связанной строки. Эти проверки не требуют общего
checkpoint мира и не означают, что БД всегда равна памяти.

### 1.2. SQL-запросы и sqlc

**Что сделаю.** Добавлю feature-запросы в
`internal/persistence/queries/character_hearth.sql`: дом по character PK,
владельцы по списку объектных ключей, чтение текущей сохранённой связи для
замены, установка/удаление связи и условное исправление невалидной связи.
Чтение дома возвращает данные объекта вместе с информацией, достаточной
для проверки типа, слоя, `deleted_at` объекта и персонажа. Batch ограничен
объектами загружаемого чанка, не всем списком персонажей.

**Граница.** Все Hearth SQL read/save выполняются синхронно под owning
surface shard lock с timeout 5 секунд на чтение/транзакцию. Это осознанное
исключение из общего запрета I/O под shard lock; worker/save queue не нужны.
Repair удаляет именно проверенную устаревшую связь, а не безусловно любую
связь персонажа: нельзя стереть более свежий дом старым результатом проверки.
Генерирую repository через `make sqlc`; generated-файлы вручную не редактирую.

**Проверки.** Отсутствующий дом, soft-deleted персонаж/объект, неверный тип/слой,
пакет из разных объектных ключей, условный repair после смены сохранённой связи.
Проверяю использование PK/UNIQUE, отсутствие N+1 и отсутствие лишних индексов.

### 1.3. Feature-owned индекс домов

**Контракт.** Индекс хранит copied home identity/позицию по персонажу и обратное
владение по `(region, object_id)`, различая `Unknown`, `NoHome`, `CurrentHome`.
`LoadIfUnknown` устанавливает результат лишь для неизвестного состояния;
любое известное runtime-состояние имеет приоритет над SQL. Owning surface lock
сериализует чтение и изменения связи: loading, optimistic Version, Dirty,
AcknowledgeSaved и протокол принятого/отклонённого lookup удаляются.

**Синхронизация.** Короткий mutex остаётся: другой слой или REST может поставить
`PendingTerminal`, пока surface ждёт SQL. Его нельзя держать во время SQL,
получения shard lock или ECS callback. `ReplaceHome` меняет оба направления
вместе. `PendingTerminal` блокирует новые admission, `ApplyTerminal` на owner
path отзывает последний дом и устанавливает `Terminal`. Связь pending owner
остаётся доступной в copied reverse snapshot для hydration: это не orphan.
`Owner` для прохода/return по-прежнему возвращает только eligible владельца.

**Память.** Записи живут до завершения процесса. Неиспользуемые Reference,
Retain и Evict удаляются вместе с учётом references; отдельного eviction
lifecycle нет. Рост памяти зависит от числа встреченных персонажей и принят
явно; не обещается фиксированный размер индекса.

**Проверки.** Назначение, замена, отзыв, known NoHome против старого SQL,
терминальный признак во время чтения, offline owner, copied snapshot и reverse
lookup с PendingTerminal. Collision lookup остаётся без SQL/аллокаций.

### 1.4. Контракт данных и границы ответственности

**Что сделаю.** Обновлю инженерные примеры в feature-документе по фактически
доставленному коду: что хранит таблица, что хранит runtime, когда загружаем,
как синхронно сохраняем и восстанавливаем частичное сохранение. Зафиксирую,
что обычный CharacterSaver не пишет `character_hearth` и не может перезаписать
текущую связь своей старой копией персонажа.

**Граница.** Не переписываю CharacterSaver, общий JSON codec или ECS storage
ради очага. В документах не выдаю предложенные hooks за готовые API; после
реализации заменю предположения фактическими путями и контрактами.

**Проверки.** Ревью всех общих изменённых типов и точек composition: нет
Hearth-полей, key branches и глобальных mutable зависимостей. Документация
явно отделяет обязательную runtime-целостность от допустимого crash rollback.

## 2. Ручная стройка и немедленная замена

### 2.1. Обычный рецепт

**Выполнено.** Рецепт `hearth_fire` получил свободный recipe ID 6 и создаёт
подготовленный object ID 36 через существующий BuildService. Проверки в
`internal/builddefs/integration_test.go` и
`internal/game/hearth_build_recipe_test.go` используют реальные каталоги
и cyclic action system: 100 solo ticks и 60 cooperative ticks при 10 Hz,
затраты 40 и 16+24 stamina. На этапе одного рецепта ядро не менялось;
личное владение подключено последующей интеграцией 2.2–2.5.

**Что сделаю.** Добавлю рецепт в существующий build catalog, используя образец
`data/builds/basic.jsonc`: пять `branch`, восемь stamina за обработку, длительность
одной ветки две секунды, пустые skill/discovery prerequisites, результат
`hearth_fire`. Номер recipe выбирается среди свободных; object ID 36 не является
номером рецепта. В текущих рецептах 20 ticks дают нужный цикл; соответствие
двум секундам проверю по действующим TickPeriod и loader, не введу другой таймер.

**Граница.** Материалы, build window, помощь, cyclic action и оплата остаются
существующими. Не пишу отдельный Hearth builder и не меняю баланс других
рецептов. Допустимые тайлы задаются рецептом в 3.3 без специальной ветки
в загрузчике; ограничение поверхности остаётся в существующей достройке Hearth.

**Проверки.** Загрузка и видимость рецепта новому персонажу, пять успешных
циклов, 40 stamina одного строителя, обычное распределение затрат помощников.
Десять секунд означают время обработки без отмен и ожидания данных;
SQL error при подготовке приводит к полной остановке сервера.

### 2.2. Подготовка перед финальными затратами

**Что сделаю.** В `internal/game/build_progress.go` перед обработкой последней
нужной единицы вызову одну optional wrapper-capability behavior результата.
Она подготавливает Hearth policy, затем вызывает callback обычного завершения;
общий build path не проверяет `hearth_fire`. Hearth проверяет фактического
рабочего, surface layer, его дом и готовность live/raw старого объекта.
`Unknown` разрешается прямым `GetCharacterHearth` с существующим timeout
5 секунд под surface shard lock. Результат сразу заполняет индекс, отсутствие
строки даёт `NoHome`. Незагруженный старый чанк ожидается прямым `WaitPreloaded`
с отдельным timeout 5 секунд под тем же shard lock. Успех завершает стройку
в этом вызове без лишнего цикла или повторного клика. Wrapper возвращает
обычный `BehaviorResult`; отдельные retry-результат, Hearth preparation
queues/channels и apply hook в `Shard.Update` не нужны.

**Выбранный компромисс.** SQL lookup, ожидание chunk gate и загрузки блокируют
surface shard. `ShardManager` ждёт завершения всех слоёв перед следующим tick,
поэтому задерживается весь game loop. Последовательные ожидания суммируются;
команды движения/отмены во время ожидания остаются в очереди до возврата вызова.

**Граница.** До успешной подготовки последняя ветка, stamina и обработанный
прогресс не меняются. Штатный `ErrChunkNotLoaded` при остановке сервера даёт
обычный отказ; настоящие ошибки БД/I/O останавливают сервер. Существующий
chunk worker только загружает cache; owning вызов ждёт его и завершает стройку.
Нет polling, оплаченного completed-site, специального
pending phase, admission ticket, раздельного prepare/commit протокола или
сохранённого finisher. Нефинальные циклы и результаты без capability идут
по прежнему пути. Это read-side подготовки; синхронная запись описана в 4.3,
pre-activation hydration описана в 3.5, смерть — в 4.1/4.3a; REST cleanup описан в 4.2;
broader restart repair остаётся будущим этапом.

**Проверки.** Ставлю preflight именно до `processOneBuildItem` финального цикла,
а не после списания. Проверяю reservation/destruction guards, смену дома между
подготовкой и завершением, удалённый site, reused handle и полную остановку
сервера при ошибке чтения БД. Unknown lookup и загрузка прежнего чанка должны
завершиться в том же вызове до затрат, без дополнительного action cycle.

### 2.3. Финальная ветка и обе трансформации

**Что сделаю.** В одной surface critical section повторно проверю рабочего,
site, progress, ownership revision, старый объект и обе destination definitions.
После preflight wrapper вызывает обычную обработку/оплату последней ветки
и завершение площадки; затем превращает прежний Hearth в ordinary unlit
campfire и переключает индекс на достроившего. ID, handle, heading/позиция
и предусмотренная обычной стройкой quality сохраняются. Игрок не перемещается.

**Необходимая правка ядра.** Сейчас `finalizeCompletedBuild` закрывает links
и возвращает успех, хотя `transformCompletedBuildTarget` игнорирует `bool`
общего `TransformObjectToDefInPlace`. Передам фактический результат вызывающему
коду. Сохраняю существующий порядок: сначала закрытие windows/break links,
потом transform. Синхронные link/station hooks выполняются как раньше;
переносить их после transform ради Hearth нельзя.

**Критическая деталь.** До оплаты проверю конкретные definitions, source
компоненты и существующие guards для Hearth/campfire; подготовлю raw-данные,
если они нужны. Преобразования используют `TransformObjectToDefInPlace`
и обычную инициализацию незажжённого campfire. Универсальный transaction,
rollback engine и новый prepare/apply API трансформаций не нужны.

**Проверки.** Отказ каждой предварительной проверки без финальных затрат;
после успеха нет build state/действий стройки, есть один дом. Проверяю, что
appearance/visibility notifications видят согласованный результат, а callbacks
не запускают новый отказывающий шаг между двумя подготовленными изменениями.

### 2.4. Старый очаг вне активного ECS

**Что сделаю.** Использую подготовленную owned копию строки старого объекта.
Преобразую только необходимые type/state поля в обычный unlit campfire,
сохранив ключ и позицию. Применяю runtime raw update через chunk cache под
owning surface path; не вызываю `SpawnEntityFromDef` и не активирую дальнюю
область ради замены. Если строка отсутствует в памяти, ожидаю загрузку по 2.2
до финальных затрат; итоговая строка сохраняется синхронно вместе с новым
очагом и связью по 4.3, а обычный dirty saver остаётся включённым.

**Механизм удержания.** `PinPersistenceBlocking` ждёт existing `ioMu`, увеличивает
existing pin count и освобождает mutex до `WaitPreloaded`. Nonblocking pin
вернул бы busy, а `WithPersistence` удержал бы mutex, нужный самому loader.
Pin удерживается через ожидание, callback завершения и синхронное сохранение,
после возврата освобождается через defer. `UpsertRawObject` ставит превращённую
строку dirty. Нового gate, polling и правок `cacheRevision` нет.

**Граница.** Использую существующие dirty/save/activation правила чанка.
Не маскирую ещё не сохранённую строку API с семантикой committed replacement
и не добавляю отдельное хранилище конверсий. Связь сначала меняется в runtime,
затем сохраняется по 4.3; общее восстановление после рестарта остаётся в 4.4.

**Проверки.** Готовый raw, отсутствующий/ещё загружаемый чанк, занятый I/O gate,
освобождение pin при отказе callback и после успеха. Activation использует
уже превращённый campfire, сохраняя identity и не активируя дальний AOI
в момент замены. Полное restart repair проверяется отдельно в 4.4.

### 2.5. Совместное завершение и повторы

**Выполнено.** Добавлены сценарии вокруг существующего BuildService, без нового
планировщика строителей. Первый успешный финальный цикл превращает site; второй
рабочий уже видит другой тип и не может назначить себя владельцем. Достроивший
передаётся из выполняемого цикла, не из `builder_id`, initiator или material
provider. Замена затрагивает только текущий дом достроившего.

**Граница.** Незавершённые площадки ничьё владение не резервируют. Офлайн/смерть
инициатора не меняет право живого помощника достроить собственный очаг.
Не храню инициатора ради будущего владельца и не добавляю idempotency journal:
защита от повторного завершения обеспечивается типом объекта и точным handle.

**Проверки.** Совместные затраты, helper-finisher, первый дом, замена live/raw
дома, конкурирующие финальные циклы и отказ повторного завершения готовой
площадки. Проверены stale handles/results, незапуск callback до подготовки
и обычный порядок закрытия links; ordinary build regressions проходят.

## 3. Правила мира и восстановление владельца

### 3.1. Definition и защита от удаления

**Что сделаю.** Дополню существующий `data/objects/hearth_fire.jsonc` behavior
очага. Сохраняю ID 36, статичность, зелёный ресурс и collider 12×12 игровых
координат: это 1×1 тайл. Не добавляю burner, station, топливо или lift behavior.
Неуязвимость от урона уже обеспечивается `indestructible`; не пишу второй
слой damage immunity. Последнее правило запрещает удаление всем, включая
владельца, а не только посторонним.

**Необходимая правка ядра.** Admin destroy сейчас обходит indestructible и
в `ChatAdminCommandHandler.ExecutePendingDestroy` вызывает DB deleter до
runtime lifecycle deletion. Если имеющегося guard недостаточно, добавлю
optional removal policy именно до этих изменений. Hearth реализует запрет,
общая команда лишь спрашивает политику. Unload, штатная трансформация в костёр
и cleanup ECS не являются игровым разрушением и этим запретом не блокируются.
Правила других indestructible объектов и обычного `/destroy` не меняются.

**Wiring.** Текущий `DefaultRegistry` является singleton и используется также
каталогом. Добавлю туда stateless behavior/validator, не состояние shard
или Hearth cache. Composition root устанавливает в каждом World feature-owned
resource со ссылкой на Hearth service через existing `ecs.SetResource`;
behavior получает её через existing `ecs.TryGetResource`. Тип resource
определяется фичей, общий ECS его не знает. Не требуется заменять существующие
registry во всех spawn/corpse/carry путях или добавлять `HearthCache` в общий
`ExecutionDeps`; валидация каталога не зависит от runtime resource.

**Проверки.** Урон, owner/foreign-player removal, admin removal до SQL, lift,
unload и разрешённая конвертация. Отдельно ordinary campfire: после превращения
он действительно получает обычные правила и collider костра, а не продолжает
быть защищённым объектом Hearth.

### 3.2. Столкновение зависит от субъекта

**Реализовано.** Через очаг проходит только владелец; другие игроки и животные
сталкиваются с ним. Для этого достаточно
одного optional callback; behavior capability/context/dispatcher не требуются.
`hearth.Runtime.IgnoreObjectCollision(actorID, targetHandle)` проверяет текущий
behavior и составной ключ объекта через `Index.Owner`: своего владельца
пропускает, для остальных сохраняет обычную коллизию. Для spawn передаётся
character ID: player handle в этой точке
ещё не существует; отдельная классификация субъекта не нужна.

**Необходимая правка ядра.** Одна политика подключена к обеим текущим проверкам:
`CollisionSystem.sweepCollision` и `Shard.trySpawnPlayerWithPolicy`.
Общие systems получают нейтральный callback через setter, не импортируют Hearth-пакет.
Исключение снимает только один collider; terrain и остальные объекты по-прежнему
проверяются. `IgnoreObjectCollision = true` для возврата использовать нельзя:
он игнорировал бы все препятствия.

**Проверки.** Проход владельца, запрет чужому игроку, spawn владельца на доме,
неизвестное владение, смена дома и препятствие поверх destination. Исключения
для животных нет: разрешение получает только ID владельца. Измеряю стоимость на
существующем наборе spatial candidates: никаких SQL, обхода всех домов,
аллокаций metadata и отдельных сканирований мира для каждого движения.

### 3.3. Стандартная проверка тайлов build

**Реализовано.** В рецепте Hearth пустой `allowedTiles` заменён на
`disallowedTiles: [1, 3, 50, 53, 56, 255]`: DeepWater, ShallowWater,
Swamp1/2/3 и Void. Остальные известные тайлы разрешены, включая сухой Moor.
Отдельного TileBog в репозитории нет. `HandleStartBuild` уже вызывает
`BuildService.validateTileRules` для всей площади collider результата;
максимальная граница exclusive через существующий `math.Nextafter`.

**Граница.** Сохраняется обычная проверка при запросе установки, без новой
terrain-перепроверки в `FinalizePendingBuildPlacement` и без нового ограничения
слоя в `BuildDef` или placement behavior. Номер слоя при установке не проверяется;
существующая достройка Hearth по-прежнему запрещена вне surface layer.
Ядро, проходимость terrain и остальные параметры рецепта не меняются.

**Проверки.** Реальный рецепт через `HandleStartBuild`: допустимые тайлы,
каждый запрещённый ID, запрещённый сосед при допустимом якоре, точное касание
границы и недостающие данные чанка. Отказ не создаёт pending placement или phantom.

### 3.4. Запрет структурного пересечения

**3.4.1 реализовано: ручная постановка.** `LiftService.placementAllowedAt`
использует общий `colliderPlacementAllowed` на всех слоях без callback в Hearth.
Проверка идёт до создания pending/phantom и повторно перед восстановлением runtime предмета
и снятием carry state. Обычный phantom проверял только подход игрока, поэтому
эти две точки необходимы. Отказ использует `LIFT_PUTDOWN_INVALID` и сохраняет
предмет в руках; очистка текущей попытки использует существующие generation guards.

**Площадь.** Для collider используется `LiftedObjectState.OriginalCollider` с масками,
поскольку live collider при переноске отключён. Для объектов без collider
необязательный `lift.placementSize` хранит конечные положительные размеры:
`log_x` — 24×6, `log_y` — 6×24; без настройки — прежние 2×2, Layer/Mask 1.
Каталог разбирает параметры в `ObjectDef.LiftConfig`; финальная проверка
заново читает актуальные размеры. Проходимость, подъём и phantom не меняются.

**Переиспользование.** Приватный helper `colliderPlacementAllowed` в
`internal/game/placement_collision.go` общий для lift и build. Существующий
`core.WorldColliderSpatial.Index.QueryBoundedInto` отбирает кандидатов; затем
проверяется полная площадь по текущим живым `Transform`/`Collider` и штатному
правилу масок движения. Касание границ разрешено. Игрок, переносимый объект
для lift и кандидаты с phantom исключаются.
Behavior, ownership index и разрешение прохода не используются: очаг и обычный
костёр после превращения проверяются как любые другие препятствия.
У `LiftService` и `BuildService` собственные переиспользуемые буферы по
4096 handles, общие бюджеты — 1024 ячейки / 4096 посещений;
отсутствие spatial index и любая ошибка запроса означают отказ.
Сохраняется проверка активного чанка якоря. Запрос учитывает только загруженные
collider, включая пересекающие границу чанка; дополнительных проверок готовности
соседей или отложенной активации нет. I/O и новых spatial API нет.

**3.4.2 реализовано: строительство.** `HandleStartBuild` вызывает общий helper
до создания новой pending placement и phantom; `FinalizePendingBuildPlacement`
заново разрешает текущее определение результата и проверяет collider до выдачи
ID и создания стройплощадки. В обоих местах используется
`objectdefs.BuildColliderComponent`; исключается сам строитель. Правило общее
для всех рецептов и слоёв, включая обычные стройплощадки. При коллизии возвращается
`BUILD_PENDING_FAILED`: ранний отказ не создаёт новую попытку, поздний очищает
текущие pending/phantom через `CancelPendingBuildPlacement`, не создавая площадку
и не расходуя ID. Старый callback с другими координатами или build key
не применяет и не очищает заменивший его preview.

**Граница.** Общая 3.4 реализована для строительства, `lift_down` и ПКМ.
Принудительное сбрасывание при выходе, смерти и сбоях переноса не меняется.
Tile rules остаются проверкой при запросе, без final terrain recheck.
Новых behavior hooks, core API, I/O, SQL, протокола и правок data не требуется.

**Проверки 3.4.1.** Владелец и чужой игрок, сундук и обе ориентации бревна,
якорь снаружи, касание границ, новый очаг во время подхода, превращение
в костёр, неизвестный владелец/pending terminal, stale generation,
обычные препятствия на разных слоях, маски/phantom, большой collider с якорем
за пределами области, недоступный индекс и переполнение бюджетов. Отказ сохраняет
carry и не оставляет частично поставленный объект. Длинное бревно может пересекать
самого несущего игрока при постановке; его phantom подхода остаётся прежним.

`BenchmarkLiftPlacementCollision` показал 149.0 ns/op, 0 B/op и 0 allocs/op
(Apple M3 Max, 200 ms). Отсутствие аллокаций проверяется также тестом.

**Проверки 3.4.2.** Обычные стройплощадки, владелец и чужой игрок, полный footprint
при якоре снаружи препятствия, препятствие после принятого preview, точное
касание, маски/phantom, разные слои, недоступный индекс и бюджеты запроса.
Проверяются отсутствие новой pending/phantom при раннем отказе, отсутствие
площадки и расхода ID с очисткой текущей попытки при позднем отказе,
а также сохранность нового preview при старом callback.

Целевые тесты build/lift, `GOCACHE=/tmp/origin-go-build-cache make test`,
`openspec validate add-hearth-fire --strict` и `git diff --check` прошли.
Независимое ревью — без замечаний. В отдельном прогоне `internal/game`
существующий тест сетевых уведомлений завершился с panic в `Server.Stop`;
его изолированный повтор и полный повтор пакета прошли, сетевой код не менялся.

### 3.5. Синхронная подготовка metadata до активации

**Порядок.** `ChunkManager.SetActivationPreparer(func(*core.Chunk) error)`
подключает `Runtime.PrepareChunkActivation(*core.Chunk) error` после existing
gate/state guards, до создания raw ECS. Surface runtime получает
текущие raw activation candidates, исключая deleted строки и ID уже живых
entities. Сначала проверяет copied `Index.SnapshotByObject`, включая
`PendingTerminal`, затем одним `GetCharacterHearthOwners` читает unresolved
ключи под `surface.mu` с timeout 5 секунд. Пустой чанк и полностью известные
кандидаты не требуют SQL. Load worker и его generation protocol не меняются.

**Индекс.** Живые offline владельцы гидратируются так же, как online.
`LoadIfUnknown` не заменяет known runtime state старой связью; после SQL
решение снова учитывает текущий индекс. PendingTerminal не даёт прав return,
но не превращает очаг в orphan до owning terminal cleanup. SQL error или
невалидный результат фатален, N+1 и сканирования мира нет.

**Orphan.** Подготавливаются copies через тот же `rawCampfire`, что
использует construction: сохраняются composite identity и позиция, ставятся
campfire type/HP, очищается behavior data и обновляется lastTick. Все copies
сохраняются `ConstructionSaverDB.SaveOrphans` одной синхронной транзакцией
с timeout 5 секунд через существующий `UpsertObject`. Только после commit выполняются `UpsertRawObject` и activation.
Ошибка не публикует частичную нормализацию. Burner/corpse restore composition
получает уже корректные type/data; отдельный deferred-restore framework не нужен.

**Необходимая граница ядра.** Existing per-object reconcile слишком поздний
для одной batch-выборки и commit до создания entities. Нужен только указанный
callback. `UnregisterEntityAOI` для dead observer переносится под существующий
shard lock, поскольку этот вызов способен инициировать activation. Общие
chunk dirty flags, I/O gates, generation guards и workers сохраняются;
новых SQL/schema и Hearth preparation jobs нет.

**Проверено.** No-query paths, один batch, offline/dead/pending owner,
known state против старой DB-связи, live duplicate/deleted raw, unload/reload,
commit до raw/ECS изменений, campfire identity/HP/data/lastTick и fatal failures.
Regression также покрывает no-op activation при каталоге без Hearth definition.
Focused Hearth/login/activation tests и race checks Index/Hearth/world/game
прошли. Финальный `make test` прошёл с `ORIGIN_OBJECT_HEALTH_TEST_DSN`, указывающим
на изолированный временный PostgreSQL, и
`GOCACHE=/private/tmp/origin-go-build-cache`; generated-файлы не изменились.
Новые PostgreSQL tests реально выполнены: mixed live/deleted/unowned Hearths,
restart restoration и rollback нескольких строк в одной транзакции.
`BenchmarkIgnoreObjectCollision`: 98.16 ns/op, 0 B/op, 0 allocs/op; локальный
microbenchmark не доказывает нагрузку 30 000 CCU. Strict OpenSpec validation
и `git diff --check` прошли. Независимый code review: Approve, нет outstanding
Critical/Important. Отметка 3.5 установлена; результаты death cleanup приведены
отдельно в `tasks.md`. REST cleanup описан ниже; release задачи остаются открытыми.

## 4. Смерть, удаление и обычное сохранение

### 4.1. Перманентная смерть

**Реализация.** `HandlePlayerPermanentDeath` вызывает приватный notification
callback. `Runtime.NotifyTerminal` работает только с синхронизированным индексом:
немедленно ставит `PendingTerminal`, сохраняя association до owning completion.
Исходный shard записывает только character ID текущего вызова.

**Синхронное завершение.** На выходе из `Shard.Update`, `tryReattachPlayer`,
`attachClientToWorld` и немедленного attachment в `spawnTeleportedPlayerWithRuntime`
общий helper отделяет текущий список ID, присваивает полю `nil`, освобождает
source lock и вызывает `CompleteTerminal` под surface lock. Возврат внешнего
вызова ждёт conversion и commit. Новый вызов source shard не может перезаписать
отделённый список. `ShardManager.Update` уже ждёт возврата всех слоёв;
worker pool, game loop, `ServerJobInbox` и набор ECS-систем не меняются.

**Почему нужны эти точки ядра.** Смерть возникает как из ECS death system,
так и при offline progression во время подключения. Прямой surface вызов
из death handler вложил бы две shard locks; перенос работы на следующий tick
нарушил бы выбранную синхронность. Два приватных callback подключаются после
создания всех shard; helper меняет только указанные границы unlock. Очередь
между тиками, coalesced drain и новый dispatcher не нужны.

**Остановка сервера.** Вход и перенос учитываются существующим `Game.wg`:
`Add` до запуска горутины, `Done` после всего owning call с terminal completion.
Существующий `Wait` перенесён после остановки сети, перед остановкой shard.
Auth handlers уже завершены сетевым `Stop`; переносы запускаются командами
shard, возврата которых ждёт отслеживаемый game loop. Поэтому `Add/Wait`
не требуют нового mutex, а chunk workers работают до завершения cleanup.

**Runtime.** Login уже загружает metadata до входа, записи не эвиктятся.
Поэтому Unknown при реальной смерти — Fatal, дополнительного SQL lookup нет.
NoHome становится Terminal без Hearth I/O, повтор Terminal — no-op.
CurrentHome разрешается заново на surface, включая результат уже допущенной
достройки. `PinPersistenceBlocking` и `WaitPreloaded` готовят home chunk;
pin остаётся до окончания сохранения. Общие live/raw transform helpers
сохраняют identity и позицию без activation unloaded дома. Проверяются
регион, слой, текущий тип, deleted/tombstone и destruction/reservation state;
непригодный источник получает только удаление связи. Труп и обычные death
rules не удерживаются до Hearth save; corpse persistence независим.

**Проверки.** NoHome без I/O, live/raw/unloaded дом, смерть на поверхности и
underground, актуальный дом после admitted construction, повтор notification,
missing/deleted/changed/quarantined source, Fatal failures и блокировка возврата
до сохранения. Конкурентные owning calls проверяют отсутствие вложенных locks
и повторного использования отделённого списка. Защита `/destroy` отложена
по решению пользователя и этим этапом не реализуется.

### 4.2. REST-удаление персонажа

**Реализация.** REST Handler получает обязательный callback `Game.DeleteCharacter`.
Authentication, разбор ID и HTTP-ответы остаются в REST; callback получает
`repository.DeleteCharacterParams`. Game берёт существующий `surface.mu`,
проверяет Running/runtime до SQL и удерживает lock через DELETE и весь cleanup.

**Подтверждение удаления.** `DeleteCharacter :execrows` сохраняет прежние
account-scoped predicates. Ноль строк — прежний 204 без изменения индекса;
одна строка — `Runtime.CompleteDeletion`. SQL DELETE использует request context
с пределом пять секунд; ошибка сохраняет HTTP 500. Это согласованное исключение
из Fatal-политики последующих Hearth read/save, а не новая общая DB policy.

**Почему lock нужен до DELETE.** Иначе удаление может опередить save уже
допущенной достройки и нарушить её alive-owner проверку. Один surface lock
упорядочивает всю стройку перед удалением последнего дома либо всё удаление
перед отказом новой стройки. `Shard.Stop` устанавливает Stopping под тем же lock,
поэтому принятый DELETE заканчивает cleanup до остановки chunk manager, а
поздний запрос отказывается до SQL. Новых mutex, Game.wg-регистраций и изменений
network shutdown нет, даже если HTTP shutdown уже исчерпал timeout.

**Feature cleanup.** `CompleteDeletion` ставит PendingTerminal. Known state
передаётся существующему terminal cleanup без lookup. Только Unknown читает
расширенный `GetSavedCharacterHearth`: LEFT JOIN по `(region, id)` сохраняет
ключ связи и nullable metadata объекта после soft delete владельца. Валидный
surface Hearth гидратируется и конвертируется существующим completion; нет
связи — Terminal без conversion; missing/deleted/changed/wrong-layer source —
`ApplyTerminal` и точный `SaveTerminal(..., nil)` без chunk I/O. Reader errors,
malformed results и cleanup failures — Fatal. Unknown при actual death остаётся
Fatal; известные NoHome/Terminal не делают Hearth I/O. Скрытые за известным
NoHome старые невалидные SQL-связи относятся к repair 4.4.

**Граница.** После подтверждённого DELETE Hearth read/save используют независимые
пятисекундные contexts; отмена HTTP не прерывает cleanup. 204 возвращается после
save. REST не знает таблицу/тип очага, не удаляет online-тело, не отключает
сессию, не создаёт corpse/FX. Политика online deletion сохраняется.

**Проверки.** Account scope и no-op responses, online/offline/Unknown/known,
composite object identity, invalid source без upsert, cancellation после DELETE,
синхронный возврат, обе очередности с достройкой и остановкой surface. Результаты
фактических запусков фиксируются в `tasks.md`.

### 4.3. Синхронное сохранение завершённой стройки

**Реализованный порядок.** Под owning surface shard lock сначала обрабатывается
последняя ветка, выполняются transforms и `ReplaceHome`. Затем сериализуются
итоговые объекты и вызывается один существующий `WithTx` с timeout 5 секунд.
Транзакция сохраняет новый очаг, прежний незажжённый костёр при наличии,
`character_hearth` и durable ID watermark через `UpsertGlobalVarLongMax`.
Успешный результат возвращается после commit. Индекс не требует dirty version
или acknowledgment; обычные dirty-флаги объектов и чанков не сбрасываются.

**Ожидание.** Pin прежнего raw-чанка остаётся до возврата сохранения и
освобождается через defer. SQL выполняется под surface shard lock: медленная
запись задерживает весь game loop, поскольку менеджер ждёт все слои.
Задержки чтения, загрузки и записи суммируются; команды движения/отмены
ожидают возврата текущего вызова. Это выбранная граница простого синхронного пути.

**Ошибки и границы.** Ошибка сериализации, SQL или commit, включая неизвестный
результат, останавливает сервер. Число затронутых строк `SetCharacterHearth`,
отличное от одного, также Fatal. Runtime rollback, SQL retry, save job queue
и commit probing не добавляются. Персонаж, инвентари, материалы, stamina
и труп не входят в транзакцию. Schema, protocol и recipe не меняются.
Сохранение смерти/REST реализовано в 4.3a; restart repair остаётся задачей 4.4.

**Предыдущие проверки.** Focused Hearth/construction и writer unit tests,
а также полный `make test` ранее прошли; проверялись ошибки записи/сериализации,
affected-row count, ID watermark и timeout. Эти результаты предшествуют
упрощению индекса в 3.5. В том запуске PostgreSQL fixture был пропущен без DSN;
последующий полный запуск с реальным временным PostgreSQL прошёл и описан в 3.5.

### 4.3a. Сохранение смерти и удаления

**Реализовано для смерти и REST.** `ConstructionSaverDB.SaveTerminal` под `surface.mu`
исполняет один `WithTx` с timeout 5 секунд: optional `UpsertObject` для
проверенного преобразованного костра, затем `RemoveCharacterHearth` по точному
character/region/object ID. Допустимы 0 или 1 удалённая связь; больше — ошибка.
Нет нового SQL/schema, ID watermark, save queue, retry или acknowledgment.

**Порядок.** Подготовка и проверка source предшествуют runtime conversion и
`ApplyTerminal`; затем выполняется сохранение. Live сериализуется общим factory,
raw использует `rawCampfire` и `UpsertRawObject`. Существующий chunk pin
удерживается до возврата save. `WithPersistence` под surface lock не вызывается.
Nil object означает только удаление связи: отсутствующий, deleted, changed
или quarantined source не восстанавливается через upsert. NoHome обходится
без Hearth SQL/chunk access; Unknown при actual death — Fatal.
Персонаж/инвентари/труп сохраняются независимо. Serialization/SQL/commit/invalid
result — Fatal; синхронное ожидание может задержать весь game loop.

**REST.** Подключение и saved lookup для offline Unknown owner реализованы
по 4.2; общий terminal writer переиспользован без нового persistence API.

**Проверки.** Runtime identity/terminal ordering и идемпотентность, timeout,
ошибки записи/числа строк, PostgreSQL atomic conversion+removal, rollback
при отказе удаления связи, сохранение deleted_at и отсутствие дома после
рестарта. Фактические результаты запуска фиксируются в `tasks.md`.

### 4.4. Восстановление и остановка

**Что сделаю.** Дополню восстановление task 3.5 проверками invalid associations
и независимых character/object snapshots. Принимается связь живого персонажа
с undeleted surface Hearth. Невалидная связь не даёт home; её условный repair
исполняется синхронно owning path по тем же five-second/fatal правилам.
Осиротевший Hearth уже должен быть сохранён как campfire до activation.
Не сканирую мир на каждом tick и не загружаю его целиком при старте.

**Допуск сбоев.** Не угадываю утраченного finisher, не повышаю campfire в Hearth
и не переигрываю финальную ветку. Ошибка процесса до commit может потерять
последний переход; обычные character/material/stamina/corpse snapshots также
не образуют общий atomic checkpoint.

**Shutdown.** Сохраняется ordinary shutdown/final-save ordering. Hearth-specific
flush, dirty drain, новый global coordinator или удержание dead bodies не нужны.

**Проверки.** Частичные saved snapshots, invalid link, повтор загрузки и
остановка с незавершённым owning вызовом. Проверки допускают потерю недавнего
прогресса и не обещают полное сохранение последнего tick.

### 4.5. Стоимость и реальная граница потерь

**Что сделаю.** Явно документирую выбранные цены: синхронные чтения, ожидание
чанка и транзакции под surface lock блокируют весь game loop; их задержки
суммируются. Команды движения/отмены ждут возврата текущего вызова. Индекс
хранит встреченные записи до конца процесса, поэтому его память растёт.

**Граница.** Нет Hearth save-lag queue, dirty-map metrics или flush/drain.
Работа hydration ограничена текущими raw кандидатами, без per-object SQL,
world scan или goroutine на каждого игрока. При оценке 30 000 CCU измеряю
фактическую задержку и размер индекса, не обещаю фиксированную память.

**Проверки.** Стройка возвращает успех после commit; orphan normalization
сохраняется до raw/ECS изменения; death cleanup ждёт commit до возврата
текущего update/attachment, REST cleanup тоже синхронен.
Реальные Hearth SQL errors/invalid results останавливают сервер; ошибка
самого REST DELETE возвращает HTTP 500. Общий transfer
сохраняет собственные DB dependencies и ordinary rollback; не добавляю
per-player metric labels или новый persistence framework.

## 5. Возврат, прерывание и обычный вход

### 5.1. Обычное timed action

**Что сделаю.** Зарегистрирую `Travel to Hearth Fire` в server action catalog
и feature-owned handler через существующие `ActionService`/`ActionHandler`.
Действие без цели, с обычным execution progress на пять секунд, нулевой
stamina cost и cooldown 360000 ms. Длительность переводится в ticks текущим
способом, не отдельным wall-clock таймером и не произвольным fixed tick count.

**Политика фичи.** Проверяю current runtime home, живое тело, combat state,
swim mode и active KO. Post-KO lying допускается, если нокаут уже закончился;
стояние в shallow water не является плаванием. Использую known index после
login/hydration; `Unknown` закрывает admission без async lookup и не выдаётся
за подтверждённое отсутствие дома.
Понятные отказы не начинают progress/кулдаун. Обычный UI получает server catalog.

**Граница.** Использую существующие cooldown persistence/time rules. Не создаю
отдельный Hearth action state, таймер, stamina resource или таблицу cooldown.
Право запуска определяет сервер, не клиентский KO timer или имя действия.

**Проверки.** No home, Unknown fail-closed, combat, swim, active KO, post-KO lying,
shallow standing, progress без затрат, cooldown denial. Проверяю обычный
путь действия и сохранение независимости cooldown других действий.

### 5.2. Асинхронный перенос и действительный успех

**Что сделаю.** После progress использую `PlayerTransferService.RequestTransfer`
с существующим `ActionDeferred`. Нейтральная source policy проверяет exact
source handle, action generation, session/connection и eligibility под source
lock непосредственно до detach. Проверка `CanCommit` используется пока source
существует, не для уже удалённого handle после переноса.

**Цель.** Destination resolver повторно читает current home под surface lock
непосредственно перед spawn. Запрошенные ранее X/Y только hints для подготовки.
Если дом заменился во время AOI/chunk preparation, подготавливаем актуальную
цель и повторяем проверку в пределах existing `SpawnTimeout`. Не отправляем
игрока на старый костёр и не вводим бесконечный retry loop. Смена дома сама
по себе не отменяет progress, но обычный timeout/исчезновение дома может
завершить перенос отказом.

**Cooldown.** Admission сейчас запускает асинхронную работу, а detach отменяет
source action и убирает source handle. Поэтому нельзя вызывать на старом
handle `Complete(success=true)` или считать `RequestTransfer` успехом.
Target-success hook применяет existing cooldown helper к новому target body
один раз после действительного relocation и до attach/initial snapshots.
Текущий `transferInFlight` запрещает параллельный перенос персонажа, но не
содержит отдельной transfer generation. Однократность нового completion hook
проверяется его узким контрактом под owning lock; captured action generation,
точный handle/session и однократный completion guard не допускают чужой или
повторный результат. Новый durable operation ID для этого не нужен.

**Граница.** Три hooks описывают реальные общие стадии переноса, а не
`HearthTransfer` branch в сервисе. Остальные transfer causes сохраняют default
поведение. Disconnect/cancel до detach запрещает успех; после capture остаётся
обычная transfer/disconnect политика, не новый способ rollback по произвольному
позднему input. Не добавляю DB publication barrier или новый transfer journal.

**Проверки.** Два места смены дома: во время progress и после queue admission.
Также late callback, cancel, disconnect, eligibility change, timeout, source
rollback и repeated success. Cooldown виден первому target snapshot, но никогда
не запускается на rejected/rolled-back transfer.

### 5.3. Carry, точное место и лежачее состояние

**Что сделаю.** Использую существующие exact spawn checks и
`LiftCarryTransferParticipant`. Для clearance берётся только player collider,
с owner callback из 3.2: свой Hearth не блокирует, остальные объекты блокируют.
Габариты переносимого предмета не расширяют эту проверку. Ни nearby search,
ни random fallback для добровольного возврата нет.

**Необходимая правка ядра.** Transfer сейчас запрещает любое
`IsIncapacitated`. Узкая source eligibility policy handler-а разрешает только
lying после завершённого KO. Для target создаётся отдельная standing-копия
runtime state и согласованное `Character.IsLying=false`. Source rollback
snapshot остаётся неизменным lying-снимком. Не меняю глобальный predicate и
не разрешаю лежачим пользоваться stairs/admin transfers из-за Hearth.

**Граница.** В existing carry participant есть restoration failure/recovery,
включая обычное безопасное падение предмета. Не обещаю невозможности таких
ошибок и не переписываю carry в универсальную атомарную transaction. Если тело
успешно relocated и participant использовал штатный recovery, это body success
с одним cooldown; если rollback тела, успеха нет.

**Проверки.** Same-layer/cross-layer carry, большой предмет при достаточном
месте игроку, независимое препятствие, post-KO успех standing и rollback lying,
participant recovery. Старые carry/transfer health/reservation tests сохраняют
свои гарантии; предмет не дублируется и не исчезает при отказе.

### 5.4. Прерывание до отказа другого действия

**Что сделаю.** Добавлю optional interruption policy активного handler-а и
нейтральное уведомление о валидном игровом намерении. Оно проходит после
проверок session/epoch/revision/формата, но до gameplay eligibility отказов.
Hearth policy вызывает уже существующую отмену progress. Без этой policy
обычные действия сохраняют прежние правила; общая система не проверяет ключ
`Travel to Hearth Fire`.

**Конкретные границы.** WASD сейчас отменяет actions после stamina/incapacity
refusal; map click имеет ранние incapacity/items-locked guards. Уведомление
должно быть раньше этих guards. `ActivateRequest`/`StartTargetedOnce` сейчас
проверяют cooldown до cancel: этот отказ также не сохраняет Hearth progress.
Проверю context/craft/build/carry и queued `Stand Up`, а не только Actions menu.
Свежая ненулевая команда движения прерывает даже без displacement; release,
keepalive, malformed/stale packets не являются новой попыткой двигаться.

**Другие причины.** Any committed damage, начало combat/swim и disconnect
проводятся через существующие lifecycle границы. Одного melee callback мало:
starvation/admin damage изменяют здоровье отдельно. Combat transition может
произойти без положительного damage. Для каждого реального пути проверю вызов
нейтральной interruption policy, без новой событийной платформы.

**Проверки.** Отклонённое действие, cooldown, blocked movement/lying, damage без
combat, combat без damage, swim, disconnect. Chat/личные окна продолжают progress;
invalid/stale ввод не отменяет новое действие и не расходует admin intent.
Урон не вызывает SQL, очереди или аллокацию списка всех действий.

### 5.5. Попытка действия, отсечённая клиентом

**Что сделаю.** Сведу existing Actions/hotbar/keyboard/touch activation к общей
функции отправки намерения. Если другое действие заблокировано клиентским
cooldown во время текущего return, отправляется только existing
`C2S_CancelAction`; заблокированное действие не запускается. Индикатор cooldown
и обычные non-return ограничения остаются.

**Почему это необходимо.** `ActionsMenu.vue` сейчас делает ранний `canActivate`
guard до общего activation пути. Изменить только `requestGameAction`
недостаточно: mouse/touch попытки не дойдут до него. Проверю все эти места
и не оставлю отдельное поведение разных способов управления.

**Граница.** Клиент не решает eligibility/успех возврата. Cancel не получает
новый packet type, cooldown не сбрасывается и stale activation после отмены
не воспроизводится. Действующий серверный guard обязателен независимо от UI.

**Проверки.** Mouse, keyboard, hotbar, touch, cooldown-blocked other action,
normal activation и повторное нажатие. Проверяю, что cancel уйдёт, blocked
action packet не уйдёт и прогресс остановится по server update.

### 5.6. Вход без автосоздания и fallback

**Что сделаю.** Сохраню `spawnAndLogin` и первый `tryReattachPlayer`.
Живое оставленное тело присоединяется в своей позиции; home fallback не
создаёт другое тело. Для обычного spawn кандидаты идут в порядке: saved
position, exact current home если он есть, nearby saved position, existing
random fallback. Первый вход без Hearth проходит обычные проверки игрока.

**Необходимая правка общего пути.** Текущий `spawnPos` содержит X/Y, а loop
привязан к исходному layer/shard. Добавлю нейтральный layer-aware candidate
и согласованный выбор owning shard/setup/attach: saved position может быть
под землёй, home всегда на поверхности. Подготовка ownership/инвентарей
выполняется вне shard lock; SQL в проверку кандидата не добавляется.

**Граница.** No-home просто пропускает этап дома, не создаёт объект, marker
или initialization transaction. Blocked home ищет место около saved position,
не около дома. Capacity/setup/connection errors нельзя выдавать за collision
и обходить бесконечным поиском; неуспешная AOI-подготовка очищается штатно.

**Проверки.** New/no-home персонаж, live-body reconnect, saved blocked,
surface home fallback из cave, обе точки blocked, nearby/random и capacity
failure. Spawn использует то же исключение коллизии только своего Hearth.

## 6. Ассеты, UI и общий черепной дым

### 6.1. Утверждённый огонь и presentation

**Что сделаю.** Сохраню существующий object resource в
`web_new/src/game/objects/structures.json`: семь подготовленных зелёных кадров,
70% размера обычного огня, 4 FPS, сохранённый ground anchor и дрова/тень костра.
Action/build icon подключается стандартным catalog/resource способом, не
отдельным hardcoded объектным UI. Имя `Hearth Fire` и игровое application copy
остаются на английском.

**Граница.** Не генерирую новый огонь или smoke, не перекрашиваю обычный
campfire и не добавляю owner name, online indicator, last-login time или
minimap marker. Фича использует подготовленный ресурс, а не арт-preview
страницу как обход игрового renderer.

**Проверки.** Frame keys/order, размер/anchor, загрузка existing textures,
always-green view, catalog icon и campfire states. Appearance после превращения
действительно ordinary unlit campfire, без зелёного flame слой/ресурса.

### 6.2. Возврат рядом со Stand Up

**Что сделаю.** В существующем `KnockoutWindow.vue` покажу действие рядом
со `Stand Up`, когда server state уже говорит, что active KO завершён, а
персонаж лежит. Использую server action catalog и общую presentation/activation
функцию из 5.5, тот же progress и cooldown, что в Actions menu.

**Граница.** Это не отдельная мгновенная teleport-кнопка и не предварительный
автоматический Stand Up. UI не считает локальное окончание таймера достаточным
правом на возврат. Active KO продолжает блокировать его на сервере, даже если
клиент устарел или отправил пакет вручную.

**Проверки.** Active KO, post-KO lying, no-home/eligibility denial, progress,
cancel, success standing и cooldown. Проверю размеры окна/кнопок и отсутствие
перекрытий на desktop/mobile без новых объясняющих текстов в игре.

### 6.3. FX событие и эпоха входа

**Что сделаю.** В `api/proto/packets.proto` сохраню `S2C_Fx.fx_key = 1`,
`position = 2` и добавлю только `stream_epoch = 3` в согласованном с текущим
world stream формате. Regenerate через `make proto` и `npm run proto`
в `web_new/`; generated files не редактирую вручную.

**Доставка.** В момент настоящей permanent death/успешной live-конвертации
сниму owned копию позиции и текущих observers до удаления source view.
Existing send path отправляет `skull_smoke` с эпохой каждого получателя,
не эпохой владельца объекта. Death position и Hearth position независимы,
в том числе в разных слоях. При конвертации unloaded объекта нет event-time
observers, поэтому поздняя загрузка не должна синтезировать эффект.

**Граница.** Ни SQL persistence path, ни restart repair, ни view rebuild не публикуют FX.
Повтор callback не повторяет runtime transition. REST cleanup не создаёт
character-position death smoke. Legacy `exp_gain` остаётся локальным эффектом
по старому маршруту и не требует epoch у старых сообщений. Никаких event IDs,
server timestamps, exactly-once delivery или retransmission сервиса.

**Проверки.** Observers до despawn, невидимые игроки/другой слой, old session/
epoch, две разные позиции, повтор save ack и новая видимость после события.
Животные смогут использовать renderer позднее; подключение их death gameplay
не входит в этот change.

### 6.4. Detached one-shot renderer

**Что сделаю.** Добавлю позиционный маршрут в existing FX/render machinery.
Сейчас `handlers.ts` игнорирует packet position, а `Render.playFx` требует
ObjectView и использует character offset/duration. Для skull smoke источник
view не нужен: sprite живёт отдельно в подходящем существующем world container
с обычной проекцией server event position. Новый FX layer не добавляю без
конкретной необходимости корректного порядка отрисовки.
Existing `FxManager`/`ResourceLoader` уже дают shared textures и загрузку,
их переиспользую вместо второй asset-loading системы.

**Анимация.** `web_new/src/game/fx/assets/skullSmoke.ts` задаёт восемь кадров
96×128, anchor `[48,120]`, 250 ms на кадр, 2000 ms, `loop: false`.
Кадр выбирается по локальному monotonic времени от старта; на expiry sprite
удаляется. Нельзя просто суммировать clamped `Ticker.deltaMS`: hidden tab
иначе продлит эффект. Не добавляю character y-offset поверх authored anchor.

**Ресурсы.** Восемь текстур готовятся один раз при инициализации renderer
через имеющийся `ResourceLoader`, без новой системы preload; concurrent
one-shots имеют явный конечный лимит. Если текстуры не готовы или лимит
исчерпан, cosmetic event можно пропустить, но не хранить неограниченную
историю. World/session/
layer reset и destroy очищают transient instances; reset-generation fence
не допускает позднего load callback из старого мира. Shared textures и
continuous campfire emitters не уничтожаются вместе с one-shot sprite.

**Проверки.** Ровно один проход кадров, исчезновение через две секунды,
удалённый source view, hidden tab, old epoch, capacity, reset во время load,
destroy и legacy FX. `test:fx` сейчас отсутствует: новый focused тест добавлю
к existing TS runner, не объявляю несуществующий suite уже готовым.

### 6.5. Визуальная и asset-проверка

**Что сделаю.** Использую существующие `hearth-fire-art-review` и
`skull-smoke-art-review` pages как visual fixtures вместе с реальным игровым
view. Проверю desktop/mobile screenshots: масштаб огня, ground anchor дыма,
полный двухсекундный цикл, видимость над объектом, post-KO controls и отсутствие
перекрытий. Это проверка результата, не замена server-authoritative тестов.

**Граница.** Prepared PNG/descriptor/provenance остаются входными ассетами;
заменяющая артгенерация не требуется. Общий skull renderer не становится
частью campfire fuel behavior. Непрерывный дым burning campfire и finite
conversion smoke сохраняют разные lifecycles.

**Проверки.** Existing asset regression tests и object validation по изменённым
ресурсам. Если atlas/tools не менялись, не объявляю выполненными нерелевантные
pipeline проверки; если менялись, выполняю все три обязательные проверки
из AGENTS.md и просматриваю реальные generated diffs.

## 7. Интеграция и выпуск

### 7.1. Сервер и PostgreSQL

**Что сделаю.** Выполню `make test` в корне, проверив regeneration protobuf/sqlc
и отсутствие случайного diff. Новые PostgreSQL tests используют existing
`internal/persistence/testutil.NewPostgres` с отдельной disposable схемой;
используемый в текущих game fixtures DSN — `ORIGIN_OBJECT_HEALTH_TEST_DSN`.
Отдельный targeted запуск гарантирует, что новая Hearth-группа действительно
была выполнена, а не скрыта среди skipped tests.

**Покрытие.** PK/UNIQUE/FK, alive/type/layer checks, синхронное сохранение стройки
и pre-activation batch read/orphan transaction, синхронные death save и REST cleanup,
PendingTerminal во время SQL, fatal/invalid-result failure, partial recovery
и отсутствие resurrection deleted target. Runtime ordering проверяется
контролируемыми задержками synchronous SQL и owning callbacks. Task 3.5 уже
проверена по перечисленным выше результатам; полный будущий scope 7.1 остаётся
незавершённым.

**Граница.** Если DSN отсутствует, PostgreSQL coverage явно отмечается как
skipped/непроверенное; это не «все проверки прошли». Чистую production БД
не очищаю, tests не запускаю на недиспозабельной БД, новые зависимости не нужны.

### 7.2. Клиентские проверки

**Что сделаю.** В `web_new/` выполню `npm run type-check`, `npm run lint`
и соответствующие existing scripts. Lint использует `--fix`, поэтому его diff
обязательно просматривается и не превращается в unrelated refactor.

**Основной набор.** `test:actions`, `test:action-execution`, `test:knockout`,
`test:movement-input`, `test:map-click`, `test:chunks`, `test:minimap`,
`test:asset-pipeline`. По реально затронутому коду добавляются sound/animation/
другие existing regression suites. Focused skull test предполагается через
`node scripts/test-character-visual.mjs tests/skull-smoke.test.ts`; это будущий
файл, а не уже существующий тест. Проверяю protobuf generation отдельно.

**Граница.** Ни клиентский progress, ни frame renderer не подтверждают gameplay
успех. UI suite не подменяет server tests на запреты, handle safety и cooldown.
Не меняю unrelated assets/protocol presentation ради прохождения lint/tests.

### 7.3. End-to-end и стоимость работы

**Что сделаю.** Пройду связные сценарии: новый персонаж без дома; первая
стройка; helper-finisher; replacement active/unloaded дома; underground return
с предметом; post-KO return; owner death/REST cleanup; login с blocked positions.
Во время сценариев искусственно задерживаю save, read и callbacks, а не только
проверяю happy path при мгновенной локальной БД.

**Инварианты.** В runtime всегда максимум один дом; старый load не возвращает
зелёный объект, blocked action отменяет progress, неудачный transfer не даёт
cooldown; normalization commit предшествует raw/ECS activation. Crash/restart fixtures
допускают прежнее saved состояние и проверяют safe normalization, а не
равенство последнему memory snapshot.

**Производительность.** Проверяю allocation/work budgets collision queries,
batch metadata и transient FX при репрезентативной плотности, задержку game
loop от синхронного SQL и рост process-lifetime индекса.
Ни N+1 SQL, ни world-per-tick scan, ни goroutine на каждого ожидающего игрока.
Локальные benchmarks не выдаю за доказательство 30 000 CCU: цель нагрузки
требует отдельного измерения и честного отчёта о фактическом сценарии.

### 7.4. Итоговое ревью и deployment

**Что сделаю.** Выполню `openspec validate add-hearth-fire --strict --no-interactive`
и `git diff --check`. Просмотрю все handwritten/generated changes: только
утверждённые механики, additive FX epoch, нужные SQL и подготовленные assets.
Обновлю change/feature документы по фактической реализации; tasks отмечаются
выполненными только после своих проверок.

**Архитектурный gate.** Для каждой общей правки сверяю таблицу необходимости
в начале документа: есть конкретный вызывающий путь и regression test,
фича не проникла в shared state, existing behavior сохранён. Нет скрытого
координатора под generic названием, SQL вне согласованных owning Hearth paths, новой
зависимости, изменения чужого баланса или `life_death_factor`.

**Deployment.** Миграция и generated contracts выкатываются согласованно.
Очистка БД перед релизом выполняется владельцем проекта отдельно; application
не очищает мир, не backfill-ит старых персонажей и не создаёт автоматический
очаг при входе. Откат заполненного мира требует совместимого backup/явного
операторского решения, а не удаления таблицы под работающим writer.
Отчёт указывает реально выполненные команды, failures/skips, принятые окна
потерь и commit status, без заявлений о стопроцентной atomic durability.

## Порядок сборки и финальная граница

Нумерация групп не означает, что можно отдельно выпустить рецепт без его
ownership/placement policy. Сначала готовятся schema/индекс и необходимая
runtime интеграция; затем связываются строительство, geometry и hydration;
после этого синхронный terminal cleanup/persistence, return/UI/FX.
Промежуточные изменения могут проверяться отдельно, но production-рецепт
не должен позволять создавать зелёный объект без владельца и world rules.
Проверки каждого слоя добавляются вместе с кодом, а не откладываются все на 7.x.

В результате обычный случай остаётся простым: игрок обработал последнюю ветку,
подготовленная замена произошла под owning lock, итоговые объекты и связь
сохранились в том же вызове. Дополнительные проверки служат конкретным
рискам — unloaded объектам, stale callbacks, границам слоёв и UI отказам.
Они не превращают игровой сервер в финансовый ledger и не обещают, что
любой последний tick обязательно переживёт сбой.
