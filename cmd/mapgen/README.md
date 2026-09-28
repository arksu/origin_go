# cmd/mapgen — как устроен генератор мира

Офлайн-утилита: генерирует весь мир (тайлы + деревья/валуны) и заливает в Postgres.
В рантайме сервер ничего не генерирует. Исследование H&H-первоисточников и философия —
в [docs/features/mapgen.md](../docs/features/mapgen.md).

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

## Полный поток (main.go)

1. `ParseMapgenOptions` — флаги → YAML-пресет → `Validate()` (диапазоны + лимиты памяти 3 GiB).
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
| Масштаб рельефа («размер материков») | `terrainScale` в noise_fields.go (0.002) — единственная частота высот |
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
