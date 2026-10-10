# cmd/mapgen — как устроен генератор мира

Офлайн-утилита: генерирует весь мир (тайлы + деревья/валуны + ресурсные споты) и заливает в Postgres.
В рантайме сервер ничего не генерирует. Исследование H&H-первоисточников и философия —
в [docs/features/mapgen.md](../../docs/features/mapgen.md).

## Запуск

```bash
# Полная генерация в БД (читает ./data/items, ./data/objects, конфиг сервера)
go run ./cmd/mapgen -gen-config etc/mapgen/presets/hnh.yaml

# Только обзорный PNG, без записи в БД — основной цикл настройки параметров
go run ./cmd/mapgen -gen-config etc/mapgen/presets/hnh.yaml -png-overview-only

# Переопределения поверх пресета (флаг сильнее YAML)
go run ./cmd/mapgen -gen-config ... -seed 42 -chunks-x 50 -chunks-y 50 -threads 8 \
    -png-export -png-dir map_png -png-scale 1 -png-highlight-rivers
```

- Пресеты: `etc/mapgen/presets/{default,hnh,minecraft_like}.yaml`, формат `version: 1`.
  YAML строгий: неизвестный ключ = ошибка (`KnownFields(true)`, gen_config.go).
- `seed: 0` = текущее время; фиксированный seed воспроизводим побайтово
  (параллельность по строкам/чанкам на результат не влияет).
- Мир: `chunks_x/y` × 128 тайлов; пресет hnh — 50×50 чанков = 6400² тайлов.

Generated trees and boulders store `object.hp = NULL`. The server initializes HP
from the matching object definition on load, without backfilling the database.
Explicitly saved HP, including zero, is restored unchanged.

## Resource spots

Full database generation automatically runs the spot stage after chunks,
objects, and `last_used_id` have been saved. Configure it separately:

```bash
go run ./cmd/mapgen -gen-config etc/mapgen/presets/hnh.yaml -spots-config etc/mapgen/spots.yaml
```

Apply [the spot migration](../../migrations/20261010_spots.sql) to an existing
database before running mapgen. A fresh database uses `migrations/schema.sql`.
The migration creates an empty table; spots are populated by full map generation.

The default spot config declares five types: `www`, `clay`, `soil`, `sand`, and
`water`. Each 512-tile district gets at most one spot per type, with a 64-tile
radius and a peak quality in the inclusive range 20–50. `water` represents an
underground well source and has its center on grass or forest. A district with
no eligible center tile skips that type. Centers cannot be on water; circles
may overlap water, other districts, other spots, and the world boundary.

Each type can set `spawn_chance` from 0 to 1 (omitted defaults to 1). The default
config uses 0.25 for `www` and 1 for the other types. This is one deterministic
roll per district/type using the map seed, independent of eligible tile count:
0 disables that type, 1 always places it when terrain allows, and 0.25 gives a
25% chance in each eligible district rather than an exact 25% quota. Changing
chance does not change the center or quality of retained spots. Logs distinguish
`skipped_no_eligible_tile` from `skipped_by_chance`.

Config dimensions are in tiles. Persisted centers and radii use absolute world
units calculated through `CoordPerTile`; chunk sizes use the shared constants.
Config and coordinate-range checks and a strict runtime read happen before any
region reset. PNG-only and interactive previews do not load spot config or write
spots. PNG export alongside full database generation still generates spots.

Generation streams five candidates per district and inserts batches of at most
1000 spots in one transaction. An insertion error rolls back that stage; the
whole map reset is not atomic. Region reset uses `DELETE` and retains the spot
ID sequence. Spots store geometry, peak quality, `{}` state, revision `1`, and
the saved server-runtime timestamp. No generation metadata is stored.

This stage does not change resource gathering, digging, wells, or runtime
regeneration. Feature details: [spots](../../docs/features/spot.md).

## Интерактивный предпросмотр слоёв (preview_server.go)

```bash
go run ./cmd/mapgen -river-preview [-gen-config etc/mapgen/presets/hnh.yaml] [-preview-port 8099]
# открыть http://127.0.0.1:8099
```

Preset workflow:

- Open `http://127.0.0.1:8099/?preset=hnh.yaml` to load that file's parameters,
  seed, and world dimensions on page load. Without `preset`, the server's
  `-gen-config` file is loaded. No file-list dropdown is used.
- The **Preset file** text input stays synchronized with `?preset=`. Press
  **Enter** or **Load** to read another file. Editing the path alone does not
  discard the current controls, so you can type a new filename for a variant.
- **Save** writes current river, biome, and world parameters, seed, and dimensions to the entered
  YAML file, creating or overwriting it. Comments and other preset sections are
  preserved; hidden river settings come from the loaded source preset.
- Paths are relative to the directory containing the server's `-gen-config`.
  Absolute/repository-relative paths inside that directory also work. Files must
  end in `.yaml` or `.yml`; parent directories must already exist. Files outside
  that directory and symlink save targets are rejected.
- Saving is manual and atomic; it never regenerates the live world or writes to
  the game database. Load errors do not silently fall back to another preset.

API: `GET /api/defaults?preset=...`, `POST /api/render` with an optional `preset`
source, and `POST /api/preset` with the render fields plus target `path`.
The save endpoint requires same-origin, localhost JSON requests.
Regression checks: `go test ./cmd/mapgen/...` and
`node --test cmd/mapgen/preview/index.test.mjs`.

Страница для оперативного подбора параметров: слева панель параметров (строится
из схемы слоёв), справа результат; колесо — зум к курсору, ЛКМ — пан,
двойной клик — вписать в экран. Доступны слои **Biomes** и **Rivers**:
`POST /api/render` принимает `{seed, chunks_x, chunks_y, layers, params}`;
`params` содержит секции `river`, `biomes` и `world`. Общая панель `world`
показывает `terrain_scale` и `perlin_water_enabled`; панель биомов — все 58
параметров `BiomeOptions`, сгруппированных по назначению, с русскими пояснениями.
Числовой ввод сохраняет точность и допустимые значения за пределами обычного
диапазона ползунка. `erosion_scale` и `weirdness_scale` сохраняются, но сейчас
не влияют на выбор биома.

При выборе Biomes карта считается один раз через `BuildTerrainPrecompute`,
с теми же правилами биомов, берегов и защиты фарватера, что и полная генерация.
Затем рисуется земля и вода Перлина в основной палитре, поверх — реки с
подсветкой глубины. Скрытие Rivers оставляет их места цветом фона: маска рек
продолжает защищать русла от биомов, размещение пятен не меняется. Галочки слоёв
управляют только видимостью; YAML-параметры `enabled` управляют генерацией.
Настройки скрытых слоёв также сохраняются кнопкой Save.

Для Rivers без Biomes сохраняется быстрый путь `BuildRiverNetwork` при
`layout_draw: true`; высота сэмплируется только под руслами для правильной
глубины воды Перлина. Режим `layout_draw: false` использует основной пайплайн
и не переписывается при сохранении. Реки всегда отрисовываются после биомов,
независимо от порядка имён в запросе. Рендеры выполняются последовательно;
перед расчётом проверяется бюджет памяти, включая RGBA и PNG-буферы.
Параметры декодируются строго (`KnownFields`) поверх значений пресета и
проходят общий `Validate()`, включая зависимости min/max и суммы плотностей.

Реестр `previewLayers` задаёт порядок и рендеры; `preview_biomes.go` описывает
панели биомов и общих настроек, `preview_terrain.go` — отображение биомов и
проверку памяти preview.

## Option B: bends within bends

The H&H preset uses visually natural drawn paths, not simulated drainage.
Elevation, lake placement, and landmass generation are unchanged; no coastal
frame or center-to-border flow is added.

New controls in `river:` (zero/omitted preserves legacy behavior):

| Setting | H&H | Meaning |
| --- | --- | --- |
| `shape_wavelength_tiles` | 400 | Broad-bend scale before `shape_frequency_scale`; positive enables Option B |
| `fairway_width_tiles` | 3 | Minimum odd-width square of deep tiles that can travel along every accepted route |
| `tributary_ratio` | 0.08 | Maximum extra branches as a fraction of accepted main links, rounded down |
| `tributary_spacing_tiles` | 300 | Minimum separation of tributary junctions from other junctions/inlets |
| `tributary_length_min/max` | 120 / 360 | Tributary endpoint-distance bounds, in tiles |

`shape_octaves` and `shape_octave_gain` control bends within the large bends.
H&H currently uses four octaves, gain 0.5, and amplitude scale 1.5 for review.
`shape_segment_length` limits control spacing; subdivision adapts to fine detail.
`shape_waves_per_link` applies only to legacy mode (wavelength zero).

### Y-соединения

`river.junction_chance` (по умолчанию 0, H&H: 0.25) выбирает связь
**озеро → середина уже существующей основной реки**, без нового озера в месте
слияния. Один seeded roll выполняется на подходящую дополнительную возможность
после регионального каркаса или в коротком этапе, включая его запасной выход
к границе. Источник должен допускать ещё одну связь, а сеть — содержать обычную
основную реку. Это не процент всех рек и не гарантированная частота развилок.
Явный этап основных выходов к границе и региональный каркас не перестраиваются.

До 16 геометрических кандидатов проверяются без изменения карты; при неудаче
используется исходный обычный кандидат с исходным seed. `0` полностью отключает
Y и сохраняет прежний результат. `1` всегда выбирает попытку в подходящих
возможностях, но не отменяет проверок свободного места. Требуются `layout_draw`,
Option B и положительный `fairway_width_tiles`; три рукава сохраняют этот
глубокий квадратный проход, а ширина русла и берегов остаётся переменной.

`river.junction_spacing_tiles` (180 тайлов) разносит развилки и требует
положительного значения при включении. Между Y и поздним притоком применяется
максимум двух настроек расстояния и физического габарита русел. Прямо связанные
с источником реки, другие Y-ветки и притоки не являются родителями нового Y.
На длинной обычной реке допускается несколько разнесённых соединений.
`tributary_ratio` по-прежнему отдельно задаёт ветки с концом на суше; Y занимает
слот основных/коротких связей, а не дополнительный бюджет притоков.
Preview возвращает `junction_eligible/selected/placed/fallbacks/attempts` и
счётчики причин отклонения вместе со статистикой рек.

### Переменная ширина русла и мелководья

В рисуемом режиме с `fairway_width_tiles > 0` значения `river_width_min/max`
задают ширину **глубокой части** основных рек. Она плавно меняется по длине
русла; мелководье добавляется снаружи с независимым профилем каждого берега.
Притоки меняют глубокую ширину от `fairway_width_tiles` до `river_width_min`.
На поворотах, устьях и слияниях растровые контуры могут быть шире из-за
объединения соседних сечений; гарантированный квадратный фарватер сохраняется.

| Параметр `river` | По умолчанию | Значение |
| --- | --- | --- |
| `width_variation_scale` | 160 | Масштаб изменения глубокой ширины вдоль русла, тайлы; 0 оставляет постоянную ширину каждой связи |
| `shallow_width_min` | 1 | Минимальная полоса мелководья с каждой стороны глубокого русла, тайлы |
| `shallow_width_max` | 5 | Максимальная полоса мелководья, тайлы; равные min/max дают постоянную толщину, оба 0 отключают полосу у рек |
| `shallow_variation_scale` | 64 | Масштаб независимых плавных изменений двух берегов, тайлы; > 0 при разных min/max |

Масштабы поддерживают 16–8192 тайла (и 0 для отключённых изменений),
береговая полоса — 0–32 тайла. `bank_radius` в этом режиме ограничивается
`shallow_width_min`, чтобы дополнительное расширение не перекрывало настройки
мелководья. Все четыре параметра доступны в **Width & depth** preview и
сохраняются в preset; отсутствующие river-поля наследуют значения по умолчанию.
При `layout_draw: false` или `fairway_width_tiles: 0` действует прежнее
построение речного коридора. Форма озёр задаётся их отдельными настройками.

Tributaries are non-recursive, at most one per parent, and can be fewer than
the budget when routes are crowded. They retain deep water to their ends.

Protected fairways connect across junctions and through lake entrances, and
stay deep where elevation would otherwise create shallow water. Generation
validates the final tiles after shoreline processing and fails with the seed,
route, and position if clearance is broken. At the world edge, clearance is
measured through the last fully in-bounds placement. This is a generator tile
contract; it does not claim that a server boat collision model was tested.

The rivers-only preview shares geometry/carving but does not run final terrain
resolution. Inspect full maps as well as the preview before accepting tuning.
Logs include `river_routes`: main count, branch budget, accepted branches, and
rejected branch candidates. Other presets stay in legacy mode.

To restore the previous H&H output, set all six new controls to zero and
restore `shape_octave_gain: 0.06` and `shape_amplitude_scale: 0.5`.
Existing world/DB content is not regenerated automatically.
Use `-png-overview-only` for safe review without DB writes.

Reproduce before/after maps, rivers-only images, and deep-fairway close-ups
for three full-size seeds without accessing the database:

```bash
go test ./cmd/mapgen -run '^TestRiverBendsReview$' -count=1 -timeout 15m -v -args \
  -bends-review-dir "$PWD/map_png/river-bends-review" -bends-review-full
```

Open `map_png/river-bends-review/README.md` for images and measurements.
Main-route counts and drawn-water components can change when candidate paths
are rejected; a validated fairway does not prove the entire world is one
connected waterway. Review those comparisons before accepting the preset.

## Озёра C: заливы, полуострова и острова

В H&H включён `river.lake_irregular_enabled`. Берег получает крупные вырезы
суши и небольшую неровность; острова выбираются отдельно для каждого озера.
Фарватеры обходят сушу и сохраняют полный квадрат лодки шириной
`fairway_width_tiles` (в H&H — 3 глубоких тайла), включая обход островов.

| Параметр `river` | H&H | Значение |
| --- | --- | --- |
| `lake_irregular_enabled` | true | false возвращает прежнюю генерацию озёр |
| `lake_peninsula_count_max` | 3 | Максимум полуостровов, 0 отключает |
| `lake_peninsula_depth_ratio` | 0.35 | Максимальное проникновение суши относительно локального радиуса; 0 отключает |
| `lake_shore_variation_tiles` | 3 | Максимальное смещение берега в тайлах; 0 отключает мелкую неровность |
| `lake_island_small_chance` | 0 | Вероятность попытки острова на малое озеро |
| `lake_island_medium_chance` | 0.15 | Вероятность попытки на среднее озеро |
| `lake_island_large_chance` | 0.35 | Вероятность выбора большого озера для островов |
| `lake_island_second_chance` | 0.20 | Вероятность второго острова в уже выбранном большом озере |
| `lake_island_radius_min` | 4 | Минимальный номинальный радиус сухой части, тайлы |
| `lake_island_radius_max` | 12 | Максимальный номинальный радиус сухой части, тайлы |
| `lake_shallow_width_min` | 1 | Минимальная полоса мелководья озёр и островов, тайлы |
| `lake_shallow_width_max` | 5 | Максимальная полоса; одинаковые границы фиксируют ширину, обе 0 отключают |

Чтобы отключить только острова, задайте все три `lake_island_*_chance` для
размеров озёр равными 0. `lake_island_second_chance` сам по себе острова
не включает. Эти параметры не связаны с `biomes.blob_islet_*`, которые
рисуют небольшие пятна биомов на суше.

Острова имеют разную вытянутость и поворот, асимметричные выступы и бухты,
а не одинаковый круглый контур. Форма воспроизводится по seed; радиус в
настройках номинальный, без мелководья. Вероятности появления от формы не зависят.
Острова не уменьшаются ниже минимального размера ради размещения. Тесные
кандидаты пропускаются: фактическая частота может быть ниже заданной, а
суммарная сухая площадь островов ограничена 8% исходной площади озера.
Статистика `river_routes` показывает выбор, запрошенные/созданные/отклонённые
острова отдельно по размерам озёр, число полуостровов и возвратов к базовой форме.
Все настройки доступны в группе preview **Берега озёр и острова** и
сохраняются в preset с явными нулями и `false`.

В новых озёрах береговое мелководье строится отдельно от речного.
Суша островов и полуостровов защищена при включённой воде Перлина;
геометрия и глубокие проходы проверяются на окончательных тайлах.
При отсутствии переключателя действует прежняя генерация. Активный режим
требует `layout_draw: true` и положительную ширину фарватера.

Для визуального сравнения без записи в БД:

```bash
go test ./cmd/mapgen -run '^TestLakeGeometryReview$' -count=1 -timeout 15m -v -args \
  -lake-review-dir /private/tmp/origin-lake-review
go test ./cmd/mapgen -run '^TestLakeGeometryReview$' -count=1 -timeout 15m -v -args \
  -lake-review-dir /private/tmp/origin-lake-review-full -lake-review-full
```

Отчёт `README.md` содержит сравнения, крупные планы, статистику и время.

## Полный поток (main.go)

1. `ParseMapgenOptions` — флаги → YAML-пресет → `Validate()` (диапазоны + лимиты памяти 6 GiB).
2. `BuildTerrainPrecompute` — весь мир считается в память (tile_pipeline.go), см. пайплайн ниже.
3. PNG-экспорт (`png_export.go`): `overview.png` + пофрагментные `chunks/`.
4. Если не `overview_only`: truncate чанков/объектов региона → пул воркеров →
   на чанк: тайлы из precompute + спавн деревьев/валунов → UpsertChunk/UpsertObject.
5. `last_used_id` (сквозной счётчик entity ID) читается и сохраняется глобально.

Детерминизм чанка: `deterministicChunkSeed(seed, x, y)` — splitMix64-микс; RNG чанка
управляет только позициями/heading деревьев и валунов.

## Пайплайн BuildTerrainPrecompute (tile_pipeline.go)

Стадии в порядке выполнения; тайминги каждой пишутся в `TerrainTimings` и в лог.

| # | Стадия | Файл | Что делает |
|---|--------|------|------------|
| 1 | **Elevation** | noise_fields.go | Одиноктавный Perlin `Noise2D(world*0.002)` → normalize в [0,1]. float32-массив на весь мир |
| 2 | **Rivers** | river.go `BuildRiverNetwork` | Речная сеть + озёра → `RiverClass` (none/shallow/deep) на тайл; см. «Реки» |
| 3 | **Ground** | biome.go `classifyBiomeGround` | Базовый фон: трава; горы/камень по `Ruggedness ≥ 0.97` (масштаб массива 100); климатический песок (сухо+жарко+континентально). Сигналы (temperature/moisture/continentalness/erosion/weirdness/ruggedness/wetness) считает `NoiseFields.BiomeSignals` с domain warp |
| 4 | **Main blobs** | blob_biomes.go | Пятна лес/heath/moor/swamp поверх травы (порт patch.py); см. «Блобы» |
| 5 | **Cleanup** | biome.go `cleanupBlobBiomes` | 3 сглаживающих прохода (majority-фильтры с locked-маской) + удаление пятен < `hnh_min_patch_tiles` (24) |
| 6 | **Secondary** | blob_biomes.go `paintSecondary` | Мелкая сетка (48): thicket внутри леса (p=0.35), dirt (0.08) и clay (0.12, по мокроте) на траве |
| 7 | **Islets** | blob_biomes.go `paintIslets` | «Островки» (1–15 тайлов) у берега пятен: сэмплирование периметра + BFS-кластер, p=0.25 |
| 8 | **Hydrology/resolve** | tile_pipeline.go + shoreline_sand.go | `resolveTileType`: вода по высоте (deep < 0.25, shallow < 0.35), речные классы перекрывают сушу; затем `applyShorelineSand` |

### Locked-маска (blob_biomes.go `buildBiomeStructuralMask`)
Вода (по высоте+рекам), горы, камень и песок — «залочены»: блобы и сглаживание их не трогают.
Это структурный каркас, приоритетный над биомами.

### Блобы — порт patch.py (blob_geometry.go)
Каждое пятно: сетка кандидатов 230 тайлов с джиттером → выбор типа взвешенно по сигналам
климата (`selectMainBlob`: лес ×(0.5+moisture)×1.6, skip-weight 0.30, moor/heath взаимоисключимы,
swamp по wetness) → геометрия: `genBlobTree` (ветвящееся дерево узлов, max 64 узлов/глубина 16)
→ `genBlobOutline` (перпендикулярные смещения ±20%) → `blobSpline` (сглаживание + рваность
`blob_raggedness`) → scanline-растеризация (supercover-линии, capsule-кисти).
RNG пятна детерминирован: `blobRandom(seed, pass, column, row, purpose)`.
Приоритет отрисовки: лес(1) → heath/moor(2) → swamp(3).

### Берега (shoreline_sand.go)
1) песок, граничащий только с внутренними водоёмами, заменяется на baseTile (убирает кольца вокруг рек);
2) добавочный пляжный песок у воды когерентным шумом: ocean p≈0.17, inland p≈0.11 (+бонусы за deep/оба),
   кластерный масштаб 20/14 тайлов;
3) fallback: если шум не дал ни одного тайла — бюджет 1/140 кандидатов по хешу.

## Реки (river.go, ~3400 строк) — два режима `river.layout_draw`

### layout_draw: true (используется в hnh.yaml) — «рисуемая» сеть
`buildDrawLayoutRiverFlow`: высота игнорируется (`_ = elevation`).
1. `generateDrawLakes` — blue-noise центры (отсев по мин. дистанции, `canPlaceDrawLake`),
   размерные классы small/medium/large (шансы 0.08 large / 0.26 medium в hnh),
   эллиптический контур с несколькими basin'ами и шумовым «берегом».
2. Связи: региональный бэкбон (`connectRegionalLakeBackbone`) + озеро-озеро до `major_count`
   (в hnh 120, из них `lake_border_mix`=0.42 доля — связи к границам карты = «реки в море»)
   + короткие джиттер-связи до `lake_connect_chance` доли связанных озёр (short jitter stage).
   Каждая связь: берег-точки → контрольные точки → Catmull-Rom → растеризация →
   `carveRiverCorridor` (ширина 5–20, случайная на связь) → inlets (расширение устьев).
3. `carveDrawLakeFootprint` — сами озёра; `buildRiverClassMask` — класс из стока (`flow`):
   shallow ≥ `flow_shallow_threshold`(6), deep ≥ `flow_deep_threshold`(20), `bank_radius` мелководье вокруг deep.

### layout_draw: false — сток по рельефу
`buildElevationRoutedRiverFlow`: источники при elevation ≥ 0.90 с шансом 0.0018 (+бонус у границ
Вороного) → `traceDownhillPath` (спуск с меандром и притяжением к рёбрам Вороного) → накопление
стока; sink-озёра в точках остановки; связка существующих озёр (`detectInlandLakes` +
`connectSubsetOfLakes`); магистральные реки (`generateMajorTrunkRivers`, от истоков 0.62 к берегу);
равномерная сетка рек (`generateUniformGridRivers`) с авто-уплотнением, пока покрытие суши
реками < 0.6%/1%. Режим в пресетах сейчас не используется.

## Объекты (main.go)

- Деревья: p=0.05 на forest-тайлах (`treeDefForTile`: и хвойный, и лиственный → `tree_birch`), Q=10.
- Валуны: p=0.0012 на траве/песке (`boulder`).
- Константы `TreeDensity`/`BoulderDensity` — в начале main.go; плотность тайлов — в пресете.

## Карта правок «хочу X → крути Y»

| Хочу изменить | Где |
|---|---|
| Доля/пороги воды | константы `deepWaterThreshold`/`shallowWaterThreshold` в noise_fields.go (пока в коде; вынос в YAML — задача change mapgen-living-water) |
| Масштаб рельефа и озёр Перлина | `world.terrain_scale` (0.002); больше — мельче и чаще детали/озёра, меньше — крупнее и плавнее; это также базовая частота климатических полей |
| Отключить воду из шума высоты | `world.perlin_water_enabled: false`; рисуемые реки и озёра речной сети остаются |
| Климатический песок / горы | пороги в biome.go `classifyBiomeGround` + `biomes.hnh_mountain_rugged_threshold`, `mountain_massif_scale`, `mountain_stone_scale` |
| Частоты климатических полей | `biomes.temperature_scale` … `weirdness_scale`, `domain_warp_strength` (в мировых координатах, 12 ед./тайл) |
| Состав/площадь биомов | `blob_*_size/_weight/_density`, `blob_skip_weight` в пресете; логика выбора — blob_biomes.go `selectMainBlob`/`selectSecondaryBlob` |
| Форма пятен (ветвистость, рваность) | blob_geometry.go (`genBlobTree`/`genBlobOutline`/`blobSpline`), `blob_raggedness`, `blob_max_nodes/depth` |
| Сглаживание, мин. пятно | `hnh_smoothing_passes`, `hnh_min_patch_tiles` |
| Пляжи | shoreline_sand.go (шансы 0.17/0.11, clusterScale, fallback-бюджет) |
| Озёра/реки рисуемой сети | секция `river:` пресета (см. аннотированный hnh.yaml — каждый ключ прокомментирован) |
| Плотность леса/валунов | `TreeDensity`/`BoulderDensity` в main.go, `treeDefForTile`/`boulderDefForTile` |
| Цвета PNG | png_export.go `tileColor` (на тайлы мира не влияет) |

## Мёртвый/зарезервированный код (не влияет на карту)

- `ecology:` в пресете — зарезервирована, нигде не потребляется (только валидация и лог).
- Сигналы `erosion` и `weirdness` считаются в `BiomeSignals`, но в выборе тайлов не участвуют.
- `classifyBaseTile` — легаси-путь при `biomes.enabled: false`; `sandThreshold` и
  `MoistureTemperature` в noise_fields.go не используются.

## Тесты

```bash
go build ./... && go test ./cmd/mapgen/   # сейчас зелёные
```

Покрытие: детерминизм (фикс. seed), геометрия блобов, пайплайн (blob_pipeline_test),
берега, экспорт PNG, валидация опций/конфига.

## Незавершённое (openspec/changes/)

- **mapgen-blob-biomes** — реализован полностью (коммит c480af0), заархивирован
  2026-09-28 (openspec/changes/archive/2026-09-28-mapgen-blob-biomes); главная спека —
  openspec/specs/mapgen-biomes/spec.md.
- **mapgen-living-water** — спланирован, в коде его НЕТ: структурная высота (fBm + континенты
  + percentile-stretch), фильтр мин. водоёма, привязка озёр к рельефу, структурные якоря,
  ширина рек по стоку. Ранее применялся частично и был откат; задачи в tasks.md не отмечены.
  Это главный кандидат на «оживление» карты.
- **s2c-packet-batching** — к mapgen не относится.
