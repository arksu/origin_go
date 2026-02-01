markdown
### Проблема
Terrain объекты генерируются синхронно целиком на чанк, число спрайтов может доходить до ~2000+ (с учётом слоёв больше).
Это даёт фризы из-за:
- массового `new Sprite()` + `addChild()`
- массового `destroy()` на unload (GC/driver stalls)
- лишних вычислений (`coordGame2Screen` на каждый sprite)
- возможной сортировки (`sortableChildren` + zIndex) на тысячах children
- двойной очистки из culling

### Решение (production-grade)

1) Terrain строить по subchunks
- ключ: `terrainSubchunkKey = "${chunkKey}:${cx},${cy}"`
- хранить состояние subchunk terrain: NotBuilt / BuiltHidden / BuiltVisible
- строить subchunks инкрементально по бюджету кадра (константа `TERRAIN_BUILD_BUDGET_MS` или `MAX_TERRAIN_SUBCHUNKS_PER_FRAME`)
- добавить cancellation/epoch/token на chunk/subchunk, чтобы поздние результаты не применялись после unload/смены зоны

2) Отображать только ближние subchunks в радиусе N
- `TERRAIN_SHOW_RADIUS_SUBCHUNKS = 2` (default)
- `TERRAIN_HIDE_RADIUS_SUBCHUNKS = 3` (hysteresis) чтобы не мигало на границе
- приоритет построения: сначала subchunks в viewport/near camera, потом дальние
- невидимые subchunks: либо hidden (если хотим быстрый show), либо возвращаем в пул (если давит память)

3) Sprite pooling (terrain отдельный пул, game objects отдельный)
- On unload/hide:
  - `removeChild`
  - `sprite.visible=false`
  - `sprite.texture = EMPTY/placeholder` (опционально)
  - положить в пул, НЕ destroy
- On load/show:
  - взять из пула, иначе `new Sprite`
  - назначить texture, обновить x/y/zIndex, visible=true, addChild
- добавить лимиты:
  - `MAX_TERRAIN_SPRITES_IN_POOL`
  - политика очистки пула при resetWorld / смене атласа

4) Убрать лишний CPU расход: coordGame2Screen на каждый sprite
- вместо:
  - `coordGame2Screen(context.tileX, context.tileY)`
- использовать:
  - `context.anchorScreenY`
- единая формула depth:
  - `zIndex = BASE_Z_INDEX + context.anchorScreenY + zOffset`

5) Убрать двойную очистку terrain из culling
- выбрать один контракт:
  - либо unregister каждого terrain и НЕ вызывать `clearTerrainForChunk`
  - либо только bulk `clearTerrainForChunk(prefix)` и не хранить/не дергать individual unregister
- зафиксировать это как инвариант (без двойной работы)

6) Риски sortableChildren/zIndex
- явно контролировать: когда и сколько объектов попадает под сортировку
- не менять zIndex terrain после первичной установки
- (опционально) вынести terrain в отдельный контейнер, если сортировка общего objectsContainer становится bottleneck, но тогда нужно сохранить корректный перекрывающий порядок

7) Метрики и критерии готовности
- метрики:
  - terrainSpritesActive / terrainSpritesPooled / terrainSpritesCreatedTotal
  - terrainBuildSubchunksQueued/Done/Canceled, terrainBuildMsAvg/p95
  - terrainClearMsAvg/p95
- acceptance:
  - при движении p95 frame time < целевого значения
  - отсутствуют single-frame spikes > порога при chunk load/unload после прогрева пула
  - memory: пул и кэши стабилизируются, нет линейного роста
