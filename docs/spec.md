MMO game - survival game:
- very large 2d map
- world sliced by ground layers (0 surface, -1, -2 underground)
- world split into chunks (128x128 tiles), chunk store tiles BLOB into DB, COORD_PER_TILE=12
- most objects are static (tree, wall, stone): no moves. player can interact with it.
- load objects when chunk marks active
- area of interest around player 3x3 chunks, load when player moving, unload inactive chunks by LRU.
- need to optimize ECS queries: base it on loaded chunks?
- player moving to point, to object, to object and interact with it (combat, open inventory, etc). no pathfinding, move straight line
- player have move modes: walk, run, fast run, swim. stamina consuming depend on move mode.
- AABB swept collisions, Uses Minkowski difference approach for robust swept collision, slide walls. collisions with dynamic objects (that are moving too) use push back.
- player do some actions with object if not near with object - must move to it and then interact.
- player have skills, exp, inventory, paperdoll, health.
- player can lift objects: drag boxes, boat, etc
- objects have available actions depends on it ECS components: pick branch, chop tree, take stone, pick a berry, etc
- when player disconnect - its character stay in game for 15 seconds to prevent leave battle, if player connect - check its character and link them.
- visiblity system: player have perception stat, some objects can have stealth component. perception defines visibility radius, weather also affect it. stealth: the closer the object, the more visible it is
- 3 kinds of exp: nature, industry, combat. actions (dig, chop, attack) give exp.
- combat pve and pvp
- time of day, weather (global)
- tile types. tile speed modifier. walkable tiles. swimmable tiles. water transport (water and deep water).
- craft system: recipe require items in inventory, formula to calc result item quality, quantity, exp (kind and amount)
- build system: recipe require items, objects. all requited must be put inside `build object`. have progress (total build points, provided build points (with the things placed inside), current build points). it's impossible to take it back required.
- farming (plants are growing even offline).

chunk based queries for movement and collision systems. use spatial hash grid into chunks, store entities into static and dynamic lists.

game:
- game loop (ticker)
- call `update` shards in goroutines (async update), worker pool

shard manager:
- shards list (layers)
- one shard = one ECS, with own systems

chunk manager:
- one chunk manager per shard(layer)
- load chunks (tiles, entities) async
- manage active chunks, while player in chunk - keep it active
- preload chunks in background async, beyond the borders of AOI
- unload inactive chunks, LRU eviction
- store chunk state into DB (dirty flag), background

chunk:
- spatial hash for entities (split entities to static and dynamic)
- state: Unloaded, Loading, Preloaded (Config.Game.PreloadRadius), Active (в AOI), Inactive
- main goal: only Active chunks keep entities in ECS
- when chunk deactivates - remove entities from ECS

Event bus system for main game logic

ECS for update game world
components:
- transform (x, y, heading)
- movement (velocity, moveMode(walk, run, fastRun, swim) )
- chunkRef
- Inventory(w, h, slots: []Item)
- Stealth
- Player(stat, skills)
  systems:
- movement
- collision
- chunk migration

it must service about 1000 online real players and about 5000 bots (simulate, moving and other game actions).
