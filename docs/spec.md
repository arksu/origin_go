Build backend for a 2D survival MMO game

**Technical Stack**
- Go v1.24
- async gorutines.
- ECS architecture for entities.

**Objectives**
- High-performance, low-latency simulation server.
- Modular design: simulation, networking, persistence, craft, inventory, bot AI.
- Load chunks on demand. 3x3 area around player. store last player's time to unload old chunks.
- Horizontal scalability: shard/continent architecture.
- service about 1000 online real players and about 5000 bots per shard.
- large 2D world, split into continents and layers (simulation shards).
- Game protocol: binary WebSocket.

**Base mechanics**
- Spawn player into world: try original position, if failed, try near radius 2 tiles, if failed find a random position at 0 layer (teleport to another shard), do it through save position and spawn retry.
- Movement: to specific point, follow the object, to object and interact with it, to object and attack it.
- Store position in database periodically, one time per second, not every tick.
- Collisions: with objects, with other players, with tiles, with world borders (keep buffer 20 tiles).
- Craft: list of craftable items, list of ingredients and it count, list of tools, stamina consume value, ticks. formula to calculate result and it quality.
- Inventory: list of items stack. can be nested (bags, boxes, etc).
- Item: all items has quality (unsigned int)
- Itmes can be thrown to the ground, can be picked up (separate action).
- Player: has inventory, health, hunger, stamina, experience (3 kind of exp: combat, ).
- Map: stored in database, load on demand
- Bot AI simulation: skeleton, no logic
- Plant growth & offline calculation
- Trees: has several stages, can be chopped (make log, require any axe).
- Physics & combat resolution
- Chunk/sector management
- Persistence layer (Postgres + Redis)
- Client interest management: send only relevant chunks/entities.
- Client prediction: server authoritative with delta correction.
- Postgres: players, inventories, map.
- Redis: hot session mapping, chunk metadata.
- Chunk serialization: compress chunks to byte arrays for disk storage.

**Tech**
- use object (grid_x, grid_y) in spatial hash grid, load objects from db by this fields

**Performance & Scaling**
Target: 1k players + 5k bots per shard.
Tick loop CPU budget: <30ms per tick (optimize ECS queries).
Memory: ~16–32 GB per sim server (depends on map size and active chunk count).
Chunk size: 128×128 tiles.
Interest management reduces bandwidth and CPU.