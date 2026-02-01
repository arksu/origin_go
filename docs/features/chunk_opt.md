# План (production-grade): убрать фризы при переходе между чанками через LRU-кэш + фоновые ребилды

## 0) Цель и ограничения
- **Цель:** при движении игрока и подгрузке новых чанков не блокировать кадр ребилдом (CPU build + GPU upload), а максимально переиспользовать уже собранное.
- **Наблюдение:** сервер заранее шлёт зону **3x3**, игрок обычно видит **2x2** → периферийные чанки перестраивать **асинхронно/по бюджету**, а не “сразу и все”.
- **Ключевая идея:** добавить **LRU-кэш выгруженных чанков** (или их частей), храним:
    - тайлы (`Uint8Array`)
    - CPU-геометрию (`positions/uv/indices` per subchunk)
    - опционально GPU-ресурсы (MeshGeometry/buffers)
    - `version` чанка → если при повторной загрузке версия другая, кэш-инстанс невалиден

---

## 1) Архитектурное разделение (обязательно для production)
### 1.1 Store vs Render-cache
- **Store (данные):** хранит только POJO/tiles/version и должен корректно отражать “сервер сказал выгрузить” → запись может удаляться из store.
- **Render/cache (визуал):** имеет право **держать скрытый чанк в памяти** (CPU/GPU) даже после `ChunkUnload`, чтобы избежать ребилда при скором возврате.
- Следствие: “unload в store” не равен “destroy в GPU”. Рендер-слой сам решает, уничтожать ли ресурсы, по LRU/TTL/лимитам.

### 1.2 Детерминизм и консистентность
- Любое восстановление из кэша обязано давать **тот же визуальный результат**, что и полный build:
    - версионность
    - учёт соседей для borders/corners (см. §5)
- Если не можем гарантировать корректность (нет соседей/нужен refresh) — чанк может быть показан, но помечен на **фоновой border-refresh**.

---

## 2) Идентификация и версии
### 2.1 Ключи
- `chunkKey = "${x},${y}"`
- `subchunkKey = "${chunkKey}:${cx},${cy}"`

### 2.2 Версионирование
- Сервер в `ChunkLoad` присылает `version`.
- В кэше храним `version`.
- При `ChunkLoad`:
    - `cacheHit` только если `cached.version === incoming.version`
    - иначе: `cacheInvalidate(chunkKey)` и build заново

---

## 3) Состав кэшируемого объекта (уровни) + выбор стратегии
### 3.1 Уровень A (data)
- `tiles + version + meta`
- Низкий риск, но CPU build всё равно будет.

### 3.2 Уровень B (CPU-cache) — рекомендовано как старт
- `tiles + cpuGeometryPerSubchunk + (опц.) hasBordersOrCorners`
- Существенно уменьшает CPU нагрузку при повторном появлении чанка.

### 3.3 Уровень C (GPU-cache) — включать по профилированию
- `tiles + cpu + gpuGeometry/meshes`
- Максимально снижает фризы, но требует:
    - строгого ownership ресурсов
    - корректного destroy при eviction
    - лимитов памяти по байтам

**Рекомендация:** внедрять по этапам: B → C.

---

## 4) LRU-кэш: политика, лимиты, очистка (production требования)
### 4.1 Структура
- `LRUCache<chunkKey, CachedChunk>`
- `CachedChunk`:
    - `{ x, y, key, version }`
    - `tiles`
    - `cpu: Map<subchunkKey, { positions, uvs, indices }>`
    - `gpu?: Map<subchunkKey, { geometryRef | bufferHandles }>`
    - `sizeBytesEstimate` (раздельно `cpuBytes`, `gpuBytes`, `tilesBytes`)
    - `createdAt`, `lastUsedAt`

### 4.2 Лимиты (не только maxEntries)
- Минимальный production-набор:
    - `maxEntries` (например 32–128)
    - `maxBytesTotal` (например 128–512MB, под платформу)
    - (опц.) `maxGpuBytes` отдельно, чтобы GPU не “съел всё”
- Eviction:
    - при переполнении: удалять по LRU
    - на eviction: гарантированно `destroy()` GPU и обнуление ссылок

### 4.3 TTL (как страховка)
- `ttlMs = 120s` (константа)
- Очистка:
    - периодический sweep (например раз в 5–10s)
    - и/или при `get/set`

### 4.4 Оценка размеров (важно)
- Для TypedArray: `byteLength`
- Для GPU: хранить “учётные байты” по созданным буферам (приближённо, но стабильно)

---

## 5) Жизненный цикл чанка: load/unload/remove + гонки (production correctness)
### 5.1 События
- `onChunkLoad(x,y,tiles,version)`
- `onChunkUnload(x,y)` (сервер сказал выгрузить)
- `onPlayerLeaveWorld / resetWorld` (жёсткий сброс сцены)

### 5.2 Поведение на ChunkUnload
1. Снять с экрана:
    - `visible=false`, unregister from culling, убрать terrain/объекты чанка (если они привязаны к чанку)
2. Сохранить в кэш:
    - сохранить CPU/GPU snapshot (если есть) и `tiles/version`
3. Не делать тяжелый destroy сразу:
    - destroy только на eviction/TTL/explicit removeWorld

### 5.3 Поведение на ChunkLoad
1. **Антигонки / cancelation:**
    - для каждого `chunkKey` вести `buildToken`/`generationId`
    - если пришёл новый `ChunkLoad` (или unload) — отменять/инвалидировать старые build-задачи
2. Попытка восстановления:
    - если `cacheHit(version)` → attach быстро (минимум работы на main thread)
    - иначе → enqueue build (см. §6)
3. После attach:
    - зарегистрировать subchunks в culling
    - пометить на border-refresh если нужны соседи (см. §7)

### 5.4 Поведение на “жёсткий remove” (leave world/reset)
- Немедленно:
    - остановить/очистить build queue
    - destroy всех живых чанков
    - очистить весь cache (или минимум GPU) — чтобы гарантировать отсутствие утечек и артефактов при заходе в другой мир

---

## 6) Приоритизация и фоновые ребилды: бюджет, backpressure, справедливость
### 6.1 Приоритеты (P0/P1/P2)
- **P0:** реально видимые (2x2 или по viewport/culling) — строить максимально быстро
- **P1:** рядом/скорее всего появятся — строить в фоне
- **P2:** дальние в пределах 3x3 — строить только в idle

### 6.2 Очередь задач (BuildQueue)
- Требования:
    - сортировка по `priority`, затем по расстоянию до камеры
    - дедупликация: на `chunkKey` не может висеть 10 задач (только последняя актуальная)
    - отмена по `buildToken`
    - метрики: длина очереди, среднее время задачи, dropped/canceled count
- Выполнение:
    - `timeBudgetMsPerFrame` (константа, например 1–3ms)
    - если кадр уже “тяжёлый” (dt > threshold) — снижать бюджет (адаптивно)

### 6.3 Backpressure (защита от “всплесков”)
- Ограничить:
    - `maxInFlightBuilds` (CPU) и отдельно `maxInFlightGpuUploads`
- Если очередь растёт:
    - P2 можно вообще не билдить
    - P1 билдить только при простое
    - P0 держать приоритет, но не допускать “стройку всего мира”

---

## 7) Соседи (borders/corners): убрать массовые rebuild
### 7.1 Проблема
- Ребилд соседей при каждом чанке даёт лавину пересборок и фризы.

### 7.2 Production-решение: “deferred border refresh”
- На каждый чанк хранить:
    - `neighborsMask` (какие из 8 соседей известны по tiles+version)
    - `needsBorderRefresh` (bool)
- При `ChunkLoad`:
    - строим с тем, что есть
    - если неполный `neighborsMask` → `needsBorderRefresh=true`
- Когда приезжает сосед:
    - не пересобирать сразу всё
    - поставить задачу `BorderRefresh(chunkKey)` в очередь (P1/P2)
- Оптимизация:
    - refresh только пограничных subchunks/тайлов (по факту зависит от реализации, но цель — не трогать центр)

---

## 8) GPU-ресурсы и ownership (уровень C) — чтобы не словить утечки/краши
### 8.1 Ownership модель (выбрать одну и придерживаться)
**Вариант 1 (безопаснее):** кэш хранит CPU-геометрию, а GPU создаётся при attach
- проще, меньше риск double-free, но чуть дороже на возврате

**Вариант 2 (максимум скорости):** кэш хранит GPU, при attach просто реиспользуем
- нужно чётко определить:
    - кто владелец geometry сейчас (scene vs cache)
    - что происходит при eviction, если объект “в сцене” (нельзя уничтожать)
    - как переносить ownership при hide/unhide

### 8.2 Правила destroy
- Eviction/TTL обязаны:
    - освобождать GPU (если владеем)
    - обнулять ссылки на TypedArray/geometry
- Никаких “частичных” destroy без учёта ownership.

---

## 9) Web Worker (этап 2/3): перенос CPU build
### 9.1 Что переносим
- Сборка `positions/uv/indices` и вычисления borders/corners
- На main thread:
    - создание/обновление MeshGeometry (GPU upload)
    - attach контейнеров и регистрация в culling

### 9.2 Контракт сообщений
- `{ chunkKey, version, buildToken, cpuGeometryPerSubchunk, hasBordersOrCorners }`
- При получении результата:
    - если `buildToken` уже не актуален — результат выбрасываем (иначе будут артефакты)

---

## 10) Метрики, профилирование, диагностика (обязательно)
### 10.1 Метрики
- cache:
    - `hits/misses`, `hitRate(rolling)`
    - `entries`, `bytesTotal`, `bytesCpu`, `bytesGpu`
    - `evictions` по причинам (LRU/TTL/versionMismatch/maxBytes)
- build:
    - `buildQueueLength`, `canceledCount`
    - `cpuBuildMs`, `gpuUploadMs` (среднее/перцентили)
- borders:
    - `borderRefreshCount`, `borderRefreshMs`

### 10.2 Debug overlay (минимум)
- `chunkCacheEntries`, `chunkCacheBytes`, `chunkCacheHitRate`
- `buildQueueLength`, `budgetMs`, `spentMsLastFrame`, `inFlightBuilds`

---

## 11) Этапы внедрения (обновлено)
### Этап 1 — быстрый эффект, минимальный риск
- LRU (TTL + maxEntries + maxBytesTotal) для **B (CPU-cache)**.
- `ChunkUnload` → hide + cache (без destroy).
- BuildQueue с бюджетом по кадру + отмена задач по `buildToken`.
- Deferred border refresh вместо массовых neighbor rebuild.

### Этап 2 — максимальное снижение фризов
- Добавить **GPU-cache (уровень C)** при наличии жёстких лимитов и ownership модели.
- Разделить лимиты CPU/GPU, добавить агрессивный eviction GPU при давлении памяти.

### Этап 3 — масштабирование на слабых CPU
- Web Worker для CPU build.
- Частичный border refresh (только границы/subchunks).

---

## 12) Acceptance criteria (расширено, production-grade)
- Производительность:
    - при движении нет заметных фризов; p95 frame time стабилен (зафиксировать целевые значения под платформу).
- Корректность:
    - при version mismatch чанк гарантированно пересобирается, артефактов нет.
    - отменённые/устаревшие build-результаты не применяются (нет “мигания” старым чанком).
- Память:
    - bytes в кэше стабилизируются; при долгой игре нет линейного роста.
- Соседи:
    - borders/corners корректируются “догоняющим” refresh’ем без лавины rebuild.
