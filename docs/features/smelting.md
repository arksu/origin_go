# Smelting (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 по Ring of Brodgar wiki (Metal Working, Ore, Ore Smelter, Stack Furnace, Crucible, Bar of Bronze, Finery Forge, Bloom, Bar of Wrought Iron, Steelmaking, Bar of Cast Iron — формулы цитируются дословно). Смежные документы: [mining.md](mining.md) (добыча руды из узлов), [build_objects.md](build_objects.md) (двухстадийная loftar-формула качества), [fishing.md](fishing.md) ( общий паттерн «узел → Q → софткапы»).

## Часть 1. Механика в H&H

### 1.1 Скиллы и структура цепи

| Скилл | LP | Пререквизиты | Открывает |
|---|---|---|---|
| **Metal Working** | 1000 | Stone Working, Firecrafts | Basic Mechanics, Forging, Jewelry; постройку Ore Smelter/Crucible/Anvil; «*Yields more metal when smelting Ore*» |
| **Forging** | 2000 | Metal Working | Steelmaking; финери-форж, «pound Cast Iron Bloom into Wrought Iron» |
| **Steelmaking** | 5000 | Forging | Metallurgy; steel crucible |
| **Metallurgy / Deep Artifice / Tunneling** | — | — | ↑ шансы плавки (~10% суммарно) и ↑ шанс положительного крита ковки |

Цепочка: **руда → плавка (Stack Furnace / Ore Smelter) → слитки → сплав (Crucible) → рафинирование (Finery Forge → Bloom → Anvil) → сталь (Steel Crucible)**.

### 1.2 Руда — 18 видов

Руда — обычный локализованный ресурс-узел (см. [mining.md](mining.md), [spot.md](spot.md)): Q узла, софткап качеством инструмента, **хардкап Masonry**; с 2023-06-22 качество равномерно по тайлу («all ore from any single mine tile will be of uniform quality»). Руда считается камнем (павинг, стокпайлы).

| Руда | Уровень пещеры | Плавится в | Базовый шанс |
|---|---|---|---|
| Cassiterite | 1 | Bar of Tin | 30% |
| Lead Glance | 1 | Bar of Lead | 25% |
| Wine Glance | 2 | Bar of Copper | 10% |
| Chalcopyrite | 3 | Bar of Copper 8% **или** Bar of Cast Iron 4% | 12% |
| Cinnabar | 3 | Quicksilver | 12% |
| Heavy Earth | 3 | Bar of Cast Iron | 6% |
| Galena | 3 | Silver Nugget или Bar of Lead | 5–10% |
| Malachite | 4 | Bar of Copper | 20% |
| Iron Ochre | 4 | Bar of Cast Iron | 12% |
| Silvershine / Horn Silver | 4 | Silver Nugget | 20% / 30% |
| Bloodstone | 5 | Bar of Cast Iron | 20% |
| Direvein | 5 | Gold Nugget или Gold Pebbles | 10% |
| Black Ore | 6 | Bar of Cast Iron | 30% |
| Schrifterz | 6 | Gold Nugget / Pebbles 50/50 | 35% |
| Leaf Ore | 6 | Gold Nugget / Pebbles | 25% |
| Peacock Ore | 7 | Bar of Copper | ~30% |
| Meteorite | поверхность | Bar of Metiron | ?? |

Ролл «металл или шлак» происходит **при плавке**; неудача → **Slag** (Q шлака = Q руды). Металлы: Lead, Tin, Copper, Cast Iron, Silver, Gold, Quicksilver, Metiron + кованое железо и сталь из чугуна; бронза — сплав.

### 1.3 Плавильни — два объекта

**Stack Furnace** (W13, 2021-04-02) — примитивная, только «мягкие» металлы: copper, tin, lead, gold, silver, quicksilver. **Не плавит железо** (Black Ore/Bloodstone/Heavy Earth/Iron Ochre/Meteorite → нельзя; Chalcopyrite отдаёт только медь — железо в ней теряется со шлаком). Build: Metal Working, **Clay×15 + Stone×10 + Board×2 + Block×4 + Leather×2**, 1.1×1.8 тайла, склад 3×6 = 18 руд. Топливо: 15–30 веток или 3–6 блоков дерева. **Bellows**: раздувание (~27 мин, +51% процесса) сильно ускоряет плавку — качать непрерывно не нужно (loftar: буст самоподдерживающийся).

**Ore Smelter** — полная плавильня. Build: Metal Working, **Brick×35 + Stone×10 + Bar of Hard Metal×3** (бутстрап: медь+олово → бронза в Crucible → бронза = Hard Metal), 2.9×2.1 тайла, склад 5×5. Загрузка **до 25 руды**, топливо **минимум 12 charcoal/black coal** (Mining-кредо «Well mined» — 9, −25%, счёт на кусок руды), поджиг факелом. **Одна загрузка = 55 минут.** Незавершённая плавка сбрасывает руду в 0% (2023-03-07 чинили топливо: теперь предмет «track the average fuel quality it has seen while burning, and uses that upon completion»). Можно докинуть руду+уголь до конца — не перезапускать.

### 1.4 Формулы качества (дословно с RoB, «Verified W16»)

| Продукт | Формула |
|---|---|
| **Слиток (Ore Smelter)** | `q = (qOre×2 + qSmelter + qAvgFuel) / 4` |
| Печь/килн/форж (общее семейство) | `q = (2·qInput + qObject + qFuel) / 4` |
| **Bar of Bronze** (Crucible: 2 Copper + 1 Tin → 3 Bronze) | `q = (AvgCopperQ + AvgTinQ) / 2`, софткап Strength × Smithing |
| Finery Forge (объект) | `qForge = (qBrick + qCastIron + qMetal) / 3` |
| **Bloom** | Q = Q чугуна, софткап `(qForge + qCoal) / 2` |
| **Bar of Wrought Iron** (Anvil + Smithy's Hammer) | `q = (Bloom×9 + Hammer×4 + Anvil×3) / 16` **±20%**: крит 10% +20%, 40% −20%, 50% ровно; Metallurgy/Steelmaking/Deep Artifice ↑ положительные криты |
| **Bar of Steel** (Steel Crucible) | Q = Q кованого железа, софткап `(qCrucible + qCoal + qAvgFuel) / 3` |

Важно: **Q Crucible не влияет на продукты**; 2023-измeнение — топливо в горящих объектах усредняется за всё горение и применяется в конце.

### 1.5 Рафинирование железа и сталь

- **Finery Forge** (Forging; Brick×30 + Bar of Cast Iron×2 + Bronze/Iron/Steel×3; с W16 нужен discovery Iron Ochre): до **9 чугунных слитков + 2 charcoal**, ~9 минут → на каждый слиток **50% Bloom / 50% Dross**. Bloom остывает в Dross за 1–2 часа; Dross переплавляется обратно (шанс снова стать чугуном). Одноходовая вероятность: 50% дросс / 33.3% чугун / 16.7% ковка.
- **Anvil + Smithy's Hammer**: bloom → **50% Bar of Wrought Iron / 50% обратно чугун** (сейчас «now always 50%, unaffected by skills»).
- **Steel Crucible** (Steelmaking): до **2 слитков кованого железа + равное количество coal/black coal**, горение **56 реальных часов** непрерывно (потух/уголь вынут → сброс); выход 1:1. Топливо на цикл: 42 угля, или 84 ветки, или 16.8 блоков.

### 1.6 Металлы, категории, топливо

- Категории слитков: **Bar of Any Metal ⊃ Common Metal (Copper, Tin) ⊃ Hard Metal (Bronze, Cast Iron, Wrought Iron, Steel)** — рецепты ссылаются на категорию.
- Топливо: **Charcoal** (обжиг дерева в Kiln, формула семейства (2·qWood+qKiln+qFuel)/4) и **Black Coal** (добывается в глубоких пещерах).
- Бронза дешевле и быстрее железа, но слабее («Weapons/armor from bronze aren't as strong as Wrought Iron or Steel»); узкое место бутстрапа — медь и олово не лежат в одной жиле, часто нужна торговля.

## Часть 2. Порт в origin_go

### 2.1 Готовые субстраты (что уже есть в коде)

- **Руды заведены**: `data/items/ores.jsonc` — `ore_tin`/`ore_copper`/`ore_iron` (tag `ore`; упрощение против 18 видов H&H). Спрайт `ore_iron.png` есть.
- **Весь арт слитков уже в атласе**: `web_new/public/assets/game/items/bar_{copper,tin,bronze,castiron,wroughtiron,steel,iron,gold,silver,hardmetal,metal,toolmetal}.png` + `coal.png` — ноль нового арта для F1–F2 предметов.
- **Арт структур**: `obj/anvil`, `obj/oven`, `obj/kiln` (+3 состояния огня `kiln_.png/kiln_2_/kiln_3_`) — визуальная база для печей; спрайтов smelter/crucible в атласе нет.
- **Станционный конвейер готов целиком**: `station {capabilities, states, values, resources}` + `behaviors.burner` (fuelAbilities/fuelCapacity/ticksPerFuel/onExhausted) — прецедент campfire (data/objects/objects.jsonc:62).
- **Крафты умеют станцию**: `requiredLinkedObjectKey` + `stationRequirements {capability, state, conditions (value gte), consume}` (internal/craftdefs/types.go:20, прецеденты basic.jsonc:71, cooking.jsonc:35; evaluator internal/game/stationreq/evaluator.go — capability-missing фейл + списание ресурсов станции).
- **QualityFormula расширяемо**: `craft.QualityFormula` с единственным значением `weighted_average_floor` (internal/game/crafting_service.go:752); `qualityWeight` на входах уже работает.
- **`requiredDiscovery`** в крафтах — готовый гейт «открытия металла» вместо скилла Metal Working.
- **Kiln-объект уже заведён** (data/objects/objects.jsonc:48, behaviors пустые) — готовая точка первой плавильни.
- **object_quality.go** в behaviors — качество объектов уже моделируется.

### 2.2 Гэпы

1. **Таймед-процесс плавки**: загрузка 25 руды на 55 минут с состояниями — инстант-крафт не покрывает; прецеденты: burner-тик, per-slot таймеры herbalist table (память `hnh-herbalist-table-research`).
2. **Формулы качества с объектом и топливом**: `(2qOre+qSmelter+qFuel)/4` требует Q станции и Q топлива в craftdef-формуле — новый kind QualityFormula; бронза — двухстадийная (type-avg → avg) из build_objects.md.
3. **Вероятностные выходы**: bar vs slag, bloom/dross 50/50, криты ±20% — в outputs нет шансов.
4. **Цепь железа упрощена**: сейчас ore_iron подразумевает bar_iron напрямую; в H&H — cast iron → bloom → wrought iron.
5. **Скиллов нет** — гейты через requiredDiscovery; софткапы по скиллам (Smithing, Masonry) некуда применить.
6. **Топливо**: charcoal/black coal предметов нет (арт coal.png уже извлечён), kiln не реализован.

### 2.3 Фазы

- **F1 MVP**: `bar_copper`/`bar_tin`/`bar_iron` из руд через **kiln** (station capability `smelting`, state `burning`, burner на ветках/дровах) — крафты в data/crafts с `stationRequirements`, качество = существующий weighted average по руде. Ноль нового арта, ноль новых прото-полей.
- **F2**: **Ore Smelter** как объект-процесс: загрузка руды (инвентарь объекта), топливо coal/charcoal (charcoal из kiln-обжига веток — burner-тик), тик плавки по ScheduleBehaviorTick с catch-up (образец burner/herbalist); новый kind `QualityFormula` `(2qOre+qSmelter+qFuel)/4`; вероятностный выход bar/slag (slag — предмет с Q руды); шансы по видам руд из §1.2.
- **F3**: бронза (2:1 в отдельном крафте, двухстадийная формула), cast iron → bloom (Finery) → wrought iron (anvil-объект уже в атласе!) с критами ±20%, сталь как длинный таймед-процесс (56 игро-часов эквивалент), металл-категории через tags (`metal_hard`, `metal_common`) для рецептов.

### 2.4 Открытые решения

1. Модель плавки: инстант-крафт на станции (F1) vs загрузка-процесс с таймером (F2) — рекомендация staged: F1 закрывает прогрессию, F2 даёт H&H-ощущение.
2. Состав руд: оставить 3 прямых (ore→metal) vs ввести H&H-виды с базовыми шансами (Cassiterite 30% и т.д.) — рекомендация: не расширять до появления узлов руды в mapgen (mining.md).
3. Формула качества: расширение QualityFormula (рекомендация, философия «конфиг отдельно от кода») vs handler-side.
4. Учитывать ли Q топлива в F1 (константа 10 vs реальный Q).
5. Slag как предмет (мусор с Q, можно переплавить) vs просто потеря руды.
6. Категории металлов через tags — сразу заложить в items при F1.

### Тесты (по конвенциям репо)

- F1: happy path (ore_copper + горящий kiln → bar_copper Q=weighted), станция не горит → fail `station_capability_missing`/state, без `requiredLinkedObjectKey` — инвентарный крафт отклонён, расход топлива burner-ом не съедает крафт.
- F2: загрузка 25 руды → тик-процесс до завершения (в т.ч. catch-up после рестарта сервера — persист состояния объекта), шансы bar/slag (фиксированный seed), формула качества (2·qOre+qSmelter+qFuel)/4 на эталонных числах, сброс при погасании (опция).
- Регрессия: strict-loader'ы craftdefs/objectdefs, существующие stationreq/burner тесты, computeCraftQuality не меняется для старых формул.

## Источники

- [Metal Working — Ring of Brodgar](https://ringofbrodgar.com/wiki/Metal_Working) (скилл 1000 LP, «Yields more metal when smelting Ore», открытие Ore Smelter/Crucible/Anvil)
- [Ore — Ring of Brodgar](https://ringofbrodgar.com/wiki/Ore) (18 руд, уровни пещер, base chances, Slag, Q: узел→софткап инструмента→хардкап Masonry, per-tile uniform 2023-06-22)
- [Ore Smelter — Ring of Brodgar](https://ringofbrodgar.com/wiki/Ore_Smelter) (build Brick×35+Stone×10+Hard Metal×3, 25 руды, 12 угля, 55 мин, формула `(qOre*2)+qSmelter+qAvgFuel4`, «Well mined» −25% топлива, fuel-Q tracking 2023)
- [Stack Furnace — Ring of Brodgar](https://ringofbrodgar.com/wiki/Stack_Furnace) (примитивная плавильня W13, мягкие металлы, bellows +51%, Chalcopyrite-ловушка)
- [Bar of Bronze — Ring of Brodgar](https://ringofbrodgar.com/wiki/Bar_of_Bronze) (2Cu+1Sn→3 Bronze в Crucible, `q=(AvgCopperQ+AvgTinQ)/2`, софткап STR×Smithing, категории Hard/Common/Any)
- [Crucible — Ring of Brodgar](https://ringofbrodgar.com/wiki/Crucible) (Brick×15+Clay×5, нужна энергия угля, **Q Crucible не влияет на продукты**)
- [Finery Forge — Ring of Brodgar](https://ringofbrodgar.com/wiki/Finery_Forge) (9 чугуна + 2 charcoal ≈ 9 мин, qForge=(qBrick+qCast+qMetal)3, Bloom софткап (qForge+qCoal)2)
- [Bloom — Ring of Brodgar](https://ringofbrodgar.com/wiki/Bloom) (50/50 Bloom/Dross, остывание 1–2 ч, Q bloom = Q чугуна при условиях)
- [Bar of Wrought Iron — Ring of Brodgar](https://ringofbrodgar.com/wiki/Bar_of_Wrought_Iron) (anvil+hammer 50%, `q=(Bloom*9+Hammer*4+Anvil*3/16)±20%`, криты 10/40/50, спираль металла)
- [Steelmaking — Ring of Brodgar](https://ringofbrodgar.com/wiki/Steelmaking) (5000 LP, 2 wrought+равный уголь, 56 ч горения, `qCrucible+qCoal+qAvgFuel3`, топливо-математика Apocalypse Please)
- [Bar of Cast Iron — Ring of Brodgar](https://ringofbrodgar.com/wiki/Bar_of_Cast_Iron) (источники руды, категории Any/Common/Hard)
- [Quality — Ring of Brodgar](https://ringofbrodgar.com/wiki/Quality) (общесемейная формула печей (2qi+qo+qf)/4, QM=Q/10)
- Исследование 2026-09-28 (память `hnh-smelting-research`)
