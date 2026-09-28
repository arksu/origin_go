# Build Objects (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 по Ring of Brodgar wiki (Construction Site, Quality — с формулами loftar), официальному форуму и перекрёстно с ранее собранными документами ([personal_claim.md](personal_claim.md) — decay, [lay_stone.md](lay_stone.md) — тайлы). Особенность документа: **в origin_go build-конвейер уже реализован целиком**, поэтому Часть 2 — это аудит соответствия и список гэпов, а не план с нуля.

## Часть 1. Механика в H&H

### 1.1 Build mode и размещение

- **Меню Build** (кнопка постройки): объекты по категориям (мебель, станции, хранилища, стены/палисады, дома, шахты, сельское хозяйство…). Записи меню **гейтятся скиллами и дисковериями** (например, каменные постройки — Masonry/Stone Working).
- **Ghost-превью** следует за курсором: зелёный/красный по валидности места, hitbox превью равен размеру будущего объекта. Поворот — **Ctrl** (удерживать — можно ходить с зажатым LMB), **точное размещение** — Shift (мелкие шаги позиции и поворота). С 2022-11-04 blueprint **не исчезает по клику**, а живёт до установки строительного знака — можно донастроить позицию.
- Установка создаёт **Construction Site («знак»/signpost)** — плейсхолдер-объект: hitbox 1×1 минимум, обычно равен финальному объекту; **никогда не поднимается**, даже если результат лифтабельный; некоторые животные (Mouflon, Aurochs) не могут ломать знак. Исчезает при завершении стройки.

### 1.2 Стройка: материалы и циклы

- Build-интерфейс открывается при установке знака и по каждому интеракту до конца стройки. На каждый требуемый материал — **три числа**: «осталось подать / лежит на площадке / уже встроено». Кнопка build переводит материал из «на площадке» в «встроено» **сверху вниз по списку**; **встроенное нельзя вынуть** обратно.
- Материалы подаются из инвентаря, а также **из смежных стокпайлов** — возникает конвейер: одна команда встраивает, другая подносит к площадке.
- Сама стройка — циклическое действие с прогрессом; часть больших зданий (**Timber House, Log Cabin, Stone Mansion**) после заполнения материалов требует **дополнительного реального ожидания** (timed wait).
- Прекратить стройку можно с возвратом **невстроенных** материалов.

### 1.3 Качество результата (формула loftar)

- Качества предметов **внутри одного типа** материалов усредняются арифметически; затем **средние по типам усредняются арифметически**, «часто с весами на предметы, которые очевидно важнее». Инструменты в формуле **не участвуют** (в отличие от craft: инструмент с весом 1/4, скиллы — геометрическим средним софткапом).
- Качество Q ∈ [1, ∞), дефолт Q10; эффект через мультипликатор **QM = √(Q/10)** — у результата это HP стен, скорость станций и прочие функции.

### 1.4 Смежные правила

- Готовые объекты **decay на незаклеймленной земле** (клейм подавляет decay; мощёная земля замедляет) — см. [personal_claim.md](personal_claim.md).
- Grand projects (Stone Mansion и т.п.) — большие многотайловые объекты с внутренними полами (в нашем тайл-контракте под них уже есть `TileHouse=80` / `TileHouseCellar=90`).
- Decay незавершённых площадок вики не документирует (не подтверждено).

## Часть 2. Порт — аудит origin_go

### 2.1 Что уже реализовано (совпадает с H&H)

- **Каталог defs**: `data/builds/*.jsonc` (строгий загрузчик internal/builddefs) — `inputs[]` (itemKey/itemTag, count, **qualityWeight**), `staminaCost`, `ticksRequired`, `requiredSkills`, `requiredDiscovery`, `allowedTiles`/`disallowedTiles`, `objectKey` → результат обязан существовать в data/objects.
- **Полный цикл** (build_service.go, ~800 строк + build_progress.go ~600):
  - `C2S_BuildStart` → resolve defs → **валидация tile rules по footprint'у коллайдера** (build_service.go:150,639 — многотайловость поддержана) → проверки движения → **PendingBuildPlacement** с PhantomCollider и TTL → персонаж подходит → `FinalizePendingBuildPlacement` ставит **build-site объект** (BuildObjectTypeID + BuildBehaviorState со слотами требуемых материалов);
  - интеракт с площадкой (behavior `build`, действие `open`) → **S2C_BuildState** — снапшот слотов для UI;
  - подача материалов — inventory/build_put.go (посштучные стеки по слотам);
  - `C2S_BuildProgress` — синтетический cyclic action: `processOneBuildItem` переводит по одному предмету из «подано» в «встроено» со стаминой; есть **rollback при отмене** (rollbackProcessedBuildItem);
  - `isBuildProgressComplete` → `finalizeCompletedBuild` → `TransformObjectToDefInPlace` в финальный объект с **QualityOverride**, закрытие UI, разрыв линков;
  - `C2S_BuildTakeBack` — разбор площадки с возвратом.
- **Клиент**: BuildGhostController/BuildGhostView (ghost-превью), build-UI в gameStore, каталог S2C_BuildList.
- Семеистика «встроенное нельзя вынуть» соблюдена; multi-player состояние рассылается линкованным игрокам (SendBuildStateSnapshotToLinkedPlayers).

### 2.2 Гэпы vs H&H (приоритизированные)

1. **Качество результата — general-case отсутствует**: `computeCompletedBuildObjectQuality` (build_progress.go:591) — **special-case только для campfire** (среднее арифметическое качеств веток). `qualityWeight` из defs для остальных построек не используется. Порт формулы loftar ложится 1:1 на наш формат: среднее по каждому input'у → арифметическое среднее средних с весами `qualityWeight`.
2. **Серверная проверка requiredSkills/requiredDiscovery не выполняется**: поля грузятся и отдаются клиенту (BuildRecipeEntry), но в `HandleStartBuild` их проверка отсутствует (валидны только tile rules) — клиентский гейт легко обходится.
3. **Стокпайл-стройка** (материалы из смежных стокпайлов) — у нас нет стокпайлов как типа объектов; отложить до введения.
4. **Timed wait** для больших зданий (после заполнения материалов — реальное ожидание) — нет; добавляется полем в builddef + behavior-tick (burner-паттерн catch-up).
5. **Поворот/точное размещение blueprint**: поворот footprint'а в build_service не поддержан (нет rotation); в клиентском BuildGhostController — проверить поворот; Shift-файн-плейс и persist-blueprint-до-установки (2022-11-04) — UX-полировка.
6. **Decay** построек вне клеймов — после появления клеймов (гэп общий с [personal_claim.md](personal_claim.md)); vine-механика мощения — в [lay_stone.md](lay_stone.md).

### 2.3 Фазы

- **F1 (quick wins)**: обобщить формулу качества на все build'ы (расширить computeCompletedBuildObjectQuality по qualityWeight, снять campfire special-case) + серверная проверка requiredSkills/requiredDiscovery в HandleStartBuild.
- **F2**: timed wait для больших объектов; полировка take-back/rollback; поворот footprint'а (если появились поворачиваемые объекты).
- **F3** (в связке с другими системами): стокпайл-стройка, decay построек (клеймы), категории меню/гранд-проекты (дома на TileHouse).

### 2.4 Открытые решения

1. Формула качества: точный порт loftar (средние по типам → взвешенное среднее средних) против плоского взвешенного среднего по всем предметам — для наших форматов результатов разница мала, но точный порт дешевле расширить потом на craft.
2. Скилл-гейты: завести скиллы (Masonry/Carpentry-аналоги) в defs или оставить пустыми до появления скилл-сетки у построек.
3. Timed wait — нужен ли в ближайших постройках ( kiln/crate — нет; большой дом — да).
4. Decay: модель и тайминги — решать вместе с клеймами.

### Тесты (по конвенциям репо)

- Качество: weighted-average формула на мульти-инпутовых builds (расширить существующий build_progress_quality_test), равенство результатов при разном порядке подачи.
- Гейты: start с недоступным скиллом/дисковери → отказ; валидные → pending placement.
- Rollback/take-back: отмена цикла возвращает «встроенное» в «подано»; разбор площадки возвращает материалы.
- Регрессия: существующие build_placement/build_progress/build_service тесты зелёные.

## Источники

- [Construction Site — Ring of Brodgar](https://ringofbrodgar.com/wiki/Construction_site) (помечена «OUT», но дает UI-семантику: три числа, top-to-bottom, стокпайлы, blueprint 2022-11-04)
- [Quality — Ring of Brodgar](https://ringofbrodgar.com/wiki/Quality) (формулы loftar для buildable/craftable, QM=√(Q/10))
- [Quality Matters Cheat Sheet — Ring of Brodgar](https://ringofbrodgar.com/wiki/Quality_Matters_Cheat_Sheet)
- [personal_claim.md](personal_claim.md) — decay на незаклеймленной земле; [lay_stone.md](lay_stone.md) — тайлы и мощение
- Код: internal/game/build_service.go, build_progress.go, behaviors/build_behavior.go, inventory/build_put.go, internal/builddefs, data/builds/README.md, api/proto/packets.proto (C2S 22–24, S2C 35–37)
