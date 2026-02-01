## План разработки: Viewport Culling для subchunks, объектов и terrain (margin в тайлах)

### 0) Цели и приоритеты
- **Цель:** уменьшить нагрузку на GPU/CPU за счёт отключения рендера всего, что вне текущего viewport.
- **Приоритет внедрения:**
    1) **Subchunk culling** (основная выгода по GPU)
    2) **Terrain culling** (так как terrain в objects-layer и может быть многочисленным)
    3) **Object culling** (персонажи/ресурсы/строения) + оптимизация интерактивности + оптимизация сортировки

---

### 1) Определения и единицы
- **Margin задаём в тайлах**: `cullMarginTiles` (например 3–6 тайлов).
- Проверка видимости делается по пересечению **AABB** (axis-aligned bounding box) объекта/подчанка с **cullRect**.
- Для стабильности используем **cullRect = viewportRect expanded by margin**.

---

### 2) Архитектурные компоненты

#### 2.1. ViewportCullingController (единый контроллер)
Ответственность:
- вычислять `viewportRectLocal` и `cullRectLocal` в координатах целевого контейнера (map-layer и objects-layer).
- выполнять проход по:
    - subchunks (map-layer)
    - terrain sprites/commands (objects-layer)
    - objects (objects-layer)
- собирать метрики (counts + time).

API/контракт (на уровне плана):
- `update(cameraState, screenSize)` вызывается каждый кадр **после применения камеры** (когда трансформы контейнеров актуальны).
- `setMarginTiles(n: number)`.

#### 2.2. Bounds providers
- **SubchunkBounds**: bounds считаются **один раз** при сборке subchunk и сохраняются.
- **ObjectBounds**: bounds считаются быстро из game-данных (`position`, `size`) или кэшируются.
- **TerrainBounds**: аналогично объектам (terrain тоже живёт в objects-layer и должен куллиться).

---

### 3) Viewport и cullRect (margin в тайлах)

#### 3.1. Как получить viewportRect в локальных координатах контейнера
- Берём экранный прямоугольник: `(0,0) - (screenW, screenH)`.
- Преобразуем 4 угла в **local coords** нужного контейнера (через inverse transform).
- Строим AABB по четырём точкам → это `viewportRectLocal`.

> Делать отдельно для:
> - `mapContainer`/`chunkLayerContainer` (где лежат subchunks)
> - `objectsContainer` (где лежат objects + terrain)

#### 3.2. Как расширить viewport на margin в тайлах
- Нужен перевод `tiles -> local units`.
- Ввести утилиту: `tilesToLocalMargin(tiles: number) -> { dx, dy }`
    - Базовый вариант: оценить “радиус” тайла в screen/local и умножить на `tiles`.
    - Для изометрии достаточно консервативного расширения:
        - `dx ≈ tiles * TILE_WIDTH_HALF * 2` (или ~полная ширина тайла)
        - `dy ≈ tiles * TILE_HEIGHT_HALF * 2`
- `cullRectLocal = expand(viewportRectLocal, dx, dy)`.

> Важно: margin должен быть одинаково применим и к map-layer, и к objects-layer (оба в одной проекции, но в разных контейнерах).

---

### 4) Subchunk culling (приоритет №1)

#### 4.1. Что куллим
- Каждый subchunk — отдельный контейнер/меш. Куллим на уровне subchunk container: `visible = true/false`.

#### 4.2. Данные, которые нужно добавить/хранить
- Для каждого subchunk сохраняем:
    - `bounds` (AABB) в координатах **родительского chunk container** или сразу в координатах **общего map-layer контейнера**.
    - `id`/ключ вида: `chunkKey + cx + cy`.
- Для каждого chunk:
    - `chunkBounds` (опционально, для раннего “whole-chunk reject”).

#### 4.3. Алгоритм кадра (subchunks)
1. Вычислить `cullRectLocal` для map-layer контейнера.
2. Для каждого загруженного chunk:
    - (опционально) если `chunkBounds` не пересекается — скрыть chunk целиком и пропустить subchunks.
    - иначе пройти по subchunks:
        - `subchunk.visible = intersects(subchunkBounds, cullRectLocal)`.

#### 4.4. Риски и решения
- AABB для изометрии может быть “широким” → culling будет менее агрессивным, но безопасным.
- Чтобы избежать мерцания на границе: margin в тайлах + (при необходимости) hysteresis (см. раздел 8).

---

### 5) Terrain culling (приоритет №2)

#### 5.1. Принцип
- Terrain-декор живёт в objects-layer и должен куллиться как объекты.
- Terrain может быть многочисленным → важно иметь дешёвые bounds и эффективный проход.

#### 5.2. Организация данных
- Для terrain обязательно иметь привязку к чанку/сабчанку:
    - `chunkKey -> list of terrain instances`
    - (лучше) `subchunkKey -> list of terrain instances` (для более тонкого culling/cleanup)
- Для каждого terrain instance хранить:
    - `container/sprite reference`
    - `bounds` (AABB) в координатах objectsContainer (или вычисляемое быстро из позиции)

#### 5.3. Алгоритм кадра (terrain)
1. Вычислить `cullRectLocal` для objectsContainer.
2. Пройти по terrain instances:
    - `visible = intersects(bounds, cullRectLocal)`.

---

### 6) Object culling (приоритет №3)

#### 6.1. Что куллим
- `ObjectView.getContainer().visible = false` если объект вне `cullRectLocal`.

#### 6.2. Bounds объектов (дешёвый способ)
- Использовать `position + size` (из game-координат) → перевести в screen/local через ту же проекцию, либо напрямую хранить “screen footprint”.
- Допустить conservative bounds (слегка больше) — безопаснее.

#### 6.3. Интерактивность (важно для CPU)
- Если объект culled:
    - отключить обработку событий/хиттест (например, `eventMode = 'none'` на корневом интерактивном элементе)
- Если объект visible:
    - вернуть интерактивность (например, `eventMode = 'static'` или прежний режим)

---

### 7) Интеграция в игровой цикл (порядок)
В основном update loop:
1. `updateMovement()`
2. `updateCamera()` (обновляет transforms контейнеров)
3. `cullingController.update(...)`
    - сначала **subchunks**
    - затем **terrain**
    - затем **objects**
4. `objectManager.update()` (z-sort) — желательно учитывать только видимые (см. ниже)
5. `updateDebugOverlay()`

---

### 8) Стабильность на границе: hysteresis (опционально, но желательно)
Чтобы вообще исключить “мигание” на границе:
- Ввести два прямоугольника:
    - `enterRect = viewportRect expanded by marginEnterTiles`
    - `exitRect = viewportRect expanded by marginExitTiles` (меньше)
- Логика:
    - если объект был невидим → показываем при пересечении с `enterRect`
    - если объект был видим → скрываем только когда **не пересекается** с `exitRect`

Рекомендация:
- `marginEnterTiles = N`, `marginExitTiles = N - 1` (или `N - 2`).

---

### 9) Оптимизация z-sort с учётом culling (после базовой реализации)
Текущая сортировка по всем объектам может быть дорогой.
План улучшения:
- поддерживать список `visibleObjects` (и `visibleTerrain` если он в той же сортировке).
- сортировать **только видимые**.
- при смене видимости помечать `needsSort = true`.

---

### 10) Debug и метрики (обязательно для проверки)
Добавить в debug overlay:
- `subchunksTotal`, `subchunksVisible`, `subchunksCulled`
- `terrainTotal`, `terrainVisible`, `terrainCulled`
- `objectsTotal`, `objectsVisible`, `objectsCulled`
- `cullingTimeMs` (замер времени update culling)
- текущие `marginTiles`, (если hysteresis) `enterTiles/exitTiles`

Опционально:
- отрисовка `viewportRectLocal` и `cullRectLocal` (в debug режиме) поверх сцены.

---

### 11) Этапы реализации (пошагово)

#### Этап A — Подготовка данных и утилит
- A1. Утилита для rect:
    - AABB структура, `intersects(a,b)`, `expand(rect, dx, dy)`.
- A2. Viewport transform:
    - `getViewportRectLocal(container)` (по 4 углам экрана).
- A3. Margin в тайлах:
    - `tilesToLocalMargin(tiles)`.

#### Этап B — Subchunk bounds + subchunk culling
- B1. Сохранять bounds для каждого subchunk при build.
- B2. Реализовать `cullSubchunks(cullRectLocalMap)`.
- B3. Метрики + проверка, что видимость корректна на пан/зуме.

#### Этап C — Terrain culling
- C1. В учёте terrain перейти на `subchunkKey -> instances`.
- C2. Добавить bounds для terrain instance.
- C3. `cullTerrain(cullRectLocalObjects)`.

#### Этап D — Object culling + интерактивность
- D1. Bounds для ObjectView (дешёвый AABB).
- D2. `cullObjects(...)` + toggle eventMode.
- D3. Проверка кликов: culled объект не должен ловить pointer events.

#### Этап E — Оптимизация сортировки
- E1. Сортировать только видимые объекты/terrain.
- E2. Переход на инкрементальные обновления (опционально).

#### Этап F — Полировка
- F1. Hysteresis (enter/exit rect).
- F2. Пороги обновления subchunk culling (если камера почти не изменилась).
- F3. Профилирование и настройка `marginTiles`.

---

### 12) Критерии готовности
- Subchunks вне viewport+marginTiles не рендерятся (видимость false), GPU нагрузка падает.
- Terrain-декор вне viewport+marginTiles не рендерится.
- Объекты вне viewport+marginTiles не рендерятся и не интерактивны.
- Нет заметного мерцания на границе (marginTiles, при необходимости hysteresis).
- Debug overlay показывает корректные счётчики и время culling.