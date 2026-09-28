# Lay Stone (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 (углублённая повторная проходка в тот же день; первичное исследование 2026-09-22) по Ring of Brodgar wiki (Paving, Terraforming, Stone Working, Road/Milestone), Fandom wiki (Terraforming, Paved Ground, Stone Working), Fandom Glossary и форуму; порт размечен по актуальному коду. Смежные документы: [farming.md](farming.md) (plow — прямой прецедент), [personal_claim.md](personal_claim.md) (клеймы гейтят decay павинга).

## Часть 1. Механика в H&H

### 1.1 Действие и семья

- **Adventure → Landscaping → Lay Stone Paving**: превращает годный тайл в мощение по курсу **1 Stone = 1 тайл**, без инструмента; таргетинг — клик по тайлу, drag-покраски нет, но **shift-click при удержании кладёт мощение непрерывно** (дорожка за одно проведение). Требует скилл **Stone Working** (200 LP, пререквизит Foraging; открывает Mining и Metal Working).
- Семья (все — 1 предмет = 1 тайл):
  - **Lay Brick Paving** — Masonry; цвет кирпича определяется типом глины обжига;
  - **Lay Metal** — Metal Working, 1 бар любого металла (реимплементировано 2016-04-06; в Legacy — 1 наггет);
  - **Lay Woodchips** — Carpentry, 1 Block of Wood; woodchip ведёт себя как лес: **нельзя вспахать и нельзя строить структуры**, но creep блокирует;
  - **Silver/Gold paving** — декоративные, **никогда не гниют** (2018-07-02).
- Визуальное разнообразие: 41 тип камня, 17 руд, 11 металлов, 9 цветов кирпича, 9 «специальных камней» (Obsidian, Slag, Lava Rock…).
- **Приоритет перекрытия** соседних мощений: **Metals > Bricks > Special Stones > Stones > Ore** — каждая категория накладывается поверх следующей.

### 1.2 Где можно / нельзя

- **Нельзя**: болота, stoneflats, пляжи, hearth-lands, acre clay, снег (первые три — «никогда»; hearth-lands/acre clay/снег — по Fandom «не мощится вовсе, пока тайл не конвертирован»; в origin_go таких тайлов нет, для референса), вода (**+3-тайловый береговой буфер — подтверждают обе вики**), **любые лесные тайлы** (включая выращенные игроком). Лес можно **сначала вспахать** — plowed мощить можно.
- **Можно**: трава, вспашка, mudflats, горы, вересковые пустоши (плавно проходимые terrain'ы).

### 1.3 Эффекты мощения

- **Скорость**: тип terrain задаёт верхний тир скорости (crawl/walk/run/sprint). Павинг — верхний тир (sprint); лес/заросли режут до walk/crawl (повозки в лесу — crawl). Правило с форума: «sprint-terrain — это grassland-тип».
- **Стамина**: сниженный дрен при беге/спринте (в glossary связь прямая: чем быстрее движение, тем больше расход).
- **Terrain creep**: dirt и вспашка со временем зарастают соседним grass/forest; **павинг общий creep блокирует** (рекомендация вики — окаймлять поля лентой павинга/woodchips). Но **tree-driven терраформинг павинг не блокирует**: растущее дерево спавнит лес до 4 тайлов вокруг независимо от мощения; лес без деревьев в радиусе ~9 тайлов деградирует в grassland.
- **Bats не спавнятся на павинге** (безопасные дороги в шахтах), но и форажаблы там не растут.
- **Посевы под мощением** дорастают и собираются, но пересаживать нельзя; очистка перед мощением не нужна — мощить тайл с растущими посевами можно сразу.
- Постройки на незаклеймленном мощении **decay медленнее**. Сам павинг **не защищает территорию** (клайм — отдельная механика).

### 1.4 Decay — vines (с 2018-05-02, «Hookah Vines»)

- Незаклеймленный павинг, граничащий с natural-тайлом или уже завиненным, может получить decay-hit → на нём появляются **vines**; каждый следующий hit растит их, **4-й hit уничтожает мощение** — тайл становится natural-тайлом соседа. Итог: большие площади гниют **с краёв внутрь**.
- Не гниют: silver/gold; тайлы под bounding box построек (2018-05-21).
- Снятие vines: Adventure → Destroy (shift-click с 2018-07-09 стирает vines **с площади**, не только с одного тайла).
- Удаление мощения: **Stomp to Dirt** (требует скилл Farming; native terrain отрастает за несколько дней), **Plant Grass** (несколько семян → grassland); в пещерах/домах — **Dig** (без инструмента, по тайлу, дорогой по энергии).

### 1.5 Roads — смежная, отдельная система

Сеть fast-travel строится на **Wooden Roadsign / Milestone** (трейлы, «tails», waylaid-дебафф) — от мощения **не зависит**; в порт Lay Stone не входит.

## Часть 2. Порт в origin_go

### 2.1 Готовые субстраты (что уже есть в коде)

- **plow_tile — полный прецедент по-тайлового действия**: def `data/actions/plow_tile.json` (`target.kind: tile`, `approach: tile_center`, `cursor`, `execution: {ticks, stamina}`, `isRepeatable`), обработчик `plowTileActionHandler` (internal/game/plow_tile_action.go) с whitelist-`ValidateTarget` через `GetTileID` и `Start` через `SetTile`, регистрация в карте действий (shard.go:250).
- **Тайл-контракт уже содержит мощение**: серверные ID `TileStonePaving=12` и кирпичи `TileBrickRed…White=5–9` (internal/types/tile.go) из H&H mapgen-контракта; клиентские `tileIds.ts` синхронны, и **`TILE_STONE_PAVING` уже зарегистрирован с тайлсетом `floor_stone`** (web_new/src/game/tiles/tileSetLoader.ts:55, configs/floor_stone.json, арт art_source/tiles/floor_stone*/). Визуальная отрисовка павинга на клиенте работает уже сейчас — достаточно сменить ID тайла.
- **Authoritative смена тайла**: `ChunkManager.SetTile` (internal/game/world/chunk_stream.go:38) — только на active chunk, правка `Tiles []byte`, переадкаст viewer'ам через chunk load events (StreamEpoch/Seq); персистенс чанков с тайлами существует (chunk.sql.go, `RestoreTiles` без бампа версии — CAS-машина plow-реализации).
- **Предмет `stone` есть** (data/items/common.jsonc:66).
- **Движение**: режимы Crawl/Walk/Run/FastRun/Swim с множителями скорости (internal/ecs/components/movement.go:96); режим запрашивает клиент (C2S MovementMode), сервер клампит по стамине (network_command.go:540, `enforceMovementModeByStamina`); стамина-стоимости по режимам в entitystats/movement.go. Это готовые точки для terrain-капов скорости и «сниженного дрена».
- **Чего нет**: terrain creep, vines/decay, клеймы (гейт decay), потребление предметов в actiondef.

### 2.2 Действие lay_stone

- `data/actions/lay_stone.json` по образцу plow_tile: `target.kind: tile`, `approach: tile_center`, `isRepeatable: true`; cursor — отдельный «paving»-курсор или reuse dig.
- Обработчик `layStoneActionHandler` — клон plow-схемы: eligibility через `GetTileID` (предложение для нашего набора тайлов: **можно** grass, plowed, dirt, moor, heath, mountain; **нельзя** water×2, swamp×3, sand, clay; forest/thicket → отдельный reason `PLOW_FIRST` — в H&H лес мощится только через вспашку).
- **Расход 1 Stone на тайл**: в actiondef нет consume-поля. Два пути:
  - (а) handler-side: проверить и списать камень через `InventoryContainer`/`InventoryRefIndex` в `Start` (прецеденты — action_requirements.go, take_behavior.go);
  - (б) маленькое декларативное расширение actiondefs (`execution.consume: [{itemKey, qty}]`) — обслужит всю семью мощений (brick/metal/woodchips) и честнее к философии strict-loader'а «конфиг отдельно от кода».
- **Скилл-гейт**: actiondef умеет `requirements.skills`, но скилла stone_working в наборе нет; plow сделан без гейта — повторить или завести скилл (решение).
- Стамина/ticks: в H&H Lay Stone почти бесплатен по стамине — можно `stamina: 0`…50 (решение).

### 2.3 Эффекты мощения — что портить сразу, что позже

- **Сразу (F1)**: смена тайла — визуал уже работает через существующий конвейер; расход предмета.
- **Скорость (F2)**: серверный кламп режима по terrain тайла под игроком (расширение `enforceMovementModeByStamina` до «enforce by stamina + terrain»: sprint/FastRun разрешён только на fast-terrain — paving/plowed/grass…; в лесу cap Walk) + опционально пониженный стамина-дрен Run/FastRun на павинге (entitystats/movement.go).
- **Позже**: creep-blocking — хук в будущую систему terrain creep (farming-контур) с приоритетом «paved tile не зарастает»; vines/decay краями внутрь — только после клеймов (гейт «вне активного клайма»); cave-павинг без bats — после спавна фауны.

### 2.4 Фазы

- **F1 MVP**: lay_stone действие (whitelist/blacklist, consume 1 stone) + видимый тайл. Ноль нового арта, ноль новых прото-полей.
- **F2**: terrain speed caps + стамина-дрен.
- **F3** (по мере появления систем): варианты Lay Brick/Metal/Woodchips (тот же обработчик с параметром tile-ID), creep-blocking, vines decay, реверты (Stomp to Dirt / Plant Grass).

### 2.5 Открытые решения

1. Consume: расширение actiondefs (рекомендация) vs handler-side списание.
2. Скилл-гейт: без скилла (как plow) vs завести Stone Working.
3. Eligibility-маппинг на наш набор тайлов (предложение в §2.2; в H&H есть mudflats/stoneflats, у нас их нет).
4. Скоростной гейт: вводить вместе с павингом (F2) или отложить.
5. Реверты/удаление: нужен ли remove-экшен сейчас (Stomp to Dirt) или павинг пока «вечный».
6. Приоритет перекрытия разных типов мощений — релевантен только с F3.

### Тесты (по конвенциям репо)

- lay_stone: happy path (grass → stone paving, −1 stone), blacklist (water/sand/swamp/forest → fail; forest → PLOW_FIRST), отсутствие камня → fail без смены тайла, repeatable-клики по соседним тайлам.
- Персистенс: SetTile на active chunk переживает save/reload (RestoreTiles), реброадкаст viewer'ам (StreamEpoch/Seq).
- Регрессия: существующие plow-тесты и strict-loader (`loader_test.go`) не ломаются; при расширении actiondefs — тесты схемы consume.

## Источники

- [Paving — Ring of Brodgar](https://ringofbrodgar.com/wiki/Paving)
- [Terraforming — Ring of Brodgar](https://ringofbrodgar.com/wiki/Terraforming)
- [Terraforming — Fandom](https://havenandhearth.fandom.com/wiki/Terraforming)
- [Paved Ground — Fandom](https://havenandhearth.fandom.com/wiki/Paved_Ground)
- [Stone Working — Ring of Brodgar](https://ringofbrodgar.com/wiki/Stone_Working)
- [Road (disambig) / Milestone — Ring of Brodgar](https://ringofbrodgar.com/wiki/Milestone)
- [Glossary (Speed) — Fandom](https://haven-and-hearth.fandom.com) + [форум: спринт-terrain'ы](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=69983)
- [Cart — Fandom](https://havenandhearth.fandom.com/wiki/Cart) (скорость повозок по terrain)
- Исследование 2026-09-22 (память `hnh-lay-stone-research`): eligibility-тиры, vine-детали. Углублённая проходка 2026-09-28 повторно прочла Fandom Terraforming/Paved Ground (страница доступна) и добавила: shift-click-непрерывное мощение, pave-over-crops, hearth-lands/acre clay/снег в чёрном списке, береговой буфер подтверждён обеими вики.
