# Fishing (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 по Ring of Brodgar wiki (Fishing, Primitive Casting-Rod, Bushcraft Fishingpole, Fishing Net, Earthworm, Seasons, Quality) и ченджлогам 2018. Смежные документы: [spot.md](spot.md) (рыба — локализованный quality-узел, тот же jorb-кейс «limited and localized resource»), [light.md](light.md) (день из `SERVER_TICK_TOTAL` — носитель лунного цикла), [eat_fep.md](eat_fep.md) (FEP-таблицы рыб станут едой).

## Часть 1. Механика в H&H

### 1.1 Инструменты и снасти

Скилл **Fishing** (200 LP, пререквизит Foraging с 2018-01-31) открывает весь набор; он же открывает Swimming и харт-магию **Fisher's Request for a Catch** (бафф шанса клёва ~30% → ~80%).

Два семейства ловли (цитата вики): *«There are two types of fishing: Rod and pole. Pole fishing uses various forms of live or fleshy bait -- such as earthworms, entrails, or ants -- whereas Rod fishing uses lures.»*

| | **Pole** (Bushcraft Fishingpole) | **Rod** (Primitive Casting-Rod) |
|---|---|---|
| Что надевается | Fishline + Hook + **Bait** | Fishline + Hook + **Lure** |
| Процесс | полуавтомат: рыба ловится сама на поклёвке | ручная мини-игра «riddles of the sport» |
| Расход | наживка теряется на каждой рыбе, авто-заменяется из инвентаря | приманка не тратится, но может потеряться |
| Выбор рыбы | случайный от узла; связка bait/line/hook слабо влияет | список целевых рыб с % поклёвки; lure/line/hook влияют на вид |

- **Крючки**: Bone / Chitin / Gold / Metal; теряются иногда, выше Q — реже. **Лески** (с 2021-04-25, отдельный предмет вместо String): Bushcraft, Farmer's, Fine, Macabre, Shepherd's, Shoreline, Tanner's, Woodsman's — иногда рвутся, выше Q — реже.
- **Наживки (35 видов)**: Earthworm, Entrails, ants (Queen/Soldiers/Larvae/Pupae/Empress), Leech (только «while bloated»), Tick (тоже bloated), Grasshopper, Grub, Ladybug, Silkmoth/Silkworm, Raw Crab, Raw Lobster, Chum Bait и пр. **Earthworm**: из копки земли («Digging for Soil will sometimes yield earthworms»), во время дождя на земле, из Compost Bin.
- **Приманки (10)**: Copper Comet, Feather Fly, Gold Spoon-Lure, Pinecone Plug, Poppy Wobbler, Rock Lobster, Steelbrush Plunger, Tin Fly, Woodfish, Copperbrush Snapper.
- Pole занимает руку (слот 5L/5R), 1×2; сборка: Tree Bough + Block of Wood + String, затем click line/hook/bait → на удочку.

### 1.2 Где и как ловят

- Ловят **там, где «прыгает рыба»** — видимые споты на воде: *«used by right-clicking an area where fish are jumping»*. Спот = проявленный локальный рыбный узел (см. 1.3).
- Места: lake shallows/depths, river shallows/depths, shallow ocean, ocean depths, cave shallows/depths. Каждый вид привязан к своему типу воды.
- **Lure-ловля**: после заброса клиент показывает список рыб с двумя процентами — *«The first percentage is the chance that the fish will bite and the second percentage is the chance to land the fish if it bites»*. Размер списка: **`1+floor(Will*Survival/20)`**, максимум 10. Если не выбрать — авто-выбор самой вероятной.
- **Внутри узла у каждого вида своя оптимальная точка**: ходишь по узлу, пока % поклёвки не вырастет (птот же «поиск максимума», что в травах).
- Что определяет улов (jorb): *«Time of day, location, lure, hook, pole and line type as well as the quality level of each piece of fishing equipment»* — т.е. время суток + локация + вся снасть целиком.
- С 2024: parts снасти можно менять на лету; Tick/Leech работают только на сытого персонажа.

### 1.3 Качество

- Рыба — **ограниченный локализованный ресурс-узел** (jorb, W13-эпоха): *«Fish are now a limited and localized resource, working in the same way as any other localized resource»* — тот же механизм, что травы/животные (см. spot.md): пиковая клетка, спад качества по кругу, истощение от сбора.
- **Вид рыбы случаен от узла**, внутри узла — своя оптимум-точка на вид.
- **Q улова хардкапится Survival** (bait-fishing); в lure-fishing участвуют качество каждой части снасти.
- Общая шкала: Q от 1 до ∞, дефолт Q10 = 1×; **QM = Q/10**; FEP еды умножаются на QM (энергия — нет); дробное — вниз.
- Бонусы поверх: реалм-бафф **Marriage of the Sea +15%** на рыбу (клеймо «Backwater» ошибочно упоминалось — фикс 2018-02-22).
- W16-дрейф «каждую новую луну» официально заявлен только для трав и животных, но по данным Sevenless (см. §1.7) **узлы рыбы тоже переезжают ежемесячно** — не только качество, но и место лова. В W16.1 обсуждали убрать Q у сырья вообще: «Raw resources should have no Quality until processed… fish».

### 1.4 Лунный цикл и фазы

Со страницы Seasons (всё дословно):

- *«In addition Haven has 8 moon phases.»* — New Moon, Waxing Crescent, First Quarter, Waxing Gibbous, Full Moon, Waning Gibbous, Last Quarter, Waning Crescent.
- *«With one moon cycle taking 30 in-game days, which is also a Haven calendar month (DD-MM-YY) of which there are 6 in a year.»*
- *«And each separate moon phase takes 3.75 in-game days.»*
- *«Each month starts halfway in the New Moon phase.»* (месяц начинается в середине фазы New Moon — фазовый сдвиг +1.875 дня).

Масштабы: 1 игровой день 24ч ≈ **7ч18м** реала (день/ночь ≈ 3.29:1 к реалу), месяц = 9д 2ч51м реала, год 180 игровых дней. На странице Seasons к отдельным фазам геймплей-эффекты не привязаны; единственный подтверждённый эффект на рыбалку — цитата с Primitive Casting-Rod:

> *«The type of fish that appear and the chance to catch them are determined by your fishing gear, the moon phase, and the local fish node, along with a chance to catch that particular fish.»*

Т.е. **фаза луны = третий множитель таблицы улова** (вместе со снастями и узлом). Формулы влияния не опубликованы; «время суток» (jorb, §1.2) — отдельный четвёртый фактор. Лунный цикл также визуален ночью (см. light.md: амплитуда затемнения от фазы).

### 1.5 Таблица рыб (FEP при Q10, World 16)

**Пресноводная (24)** — озеро/река, отмели/глубины:

| Рыба | FEP | Рыба | FEP |
|---|---|---|---|
| Asp | 5 DEX | Perch | 4 INT |
| Brill | 2 STR, 2 DEX | Pike | 2 STR, 2 INT, 1 CON |
| Bream | 2 INT, 3 CHA | Plaice | 1 INT, 3 PER |
| Burbot | 3 CON, 1.5 CHA | Roach | 2 INT, 2 DEX |
| Carp | 2.5 CHA, 2.5 CON, 1 PER | Ruffe | 3 CHA |
| Catfish | 3 порции; 1.25 AGI, 2.5 INT, 2.5 PSY | Salmon | 2 INT, 3 CHA |
| Chub | 1 CHA, 1 INT | Silver Bream | 2 INT |
| Grayling | 2.5 PER, 1.25 AGI | Smelt | 1 CHA, 1 DEX |
| Ide | 2 DEX, 0.5 PSY | Sturgeon | 3 порции; 3 STR, 1 PSY |
| Lavaret | 4.5 CON, 1.5 AGI | Tench | 2 INT, 1 CON, 1 PER |
| Zander | 3 INT | Trout | 2 STR, 1.5 PER, 1.5 DEX |
| Zope | 2 PER, 1 INT | «A Talking Whale» | 6 порций (пасхалка) |

**Океаническая (13)** — shallow ocean / ocean depths (Eel также river depths): Bass (1 PER, 2 WIL), Cod (2 INT, 3 PER), Eel (1×2; 4 AGI), Haddock (3.5 PER, 2 AGI), Herring (1 INT, 1 DEX), Mackerel (3 INT), Mullet (3 STR, 2 PER), Pomfret (2 INT, 1 WIL), Rose Fish (2 WIL, 2 CHA), Saithe (2 INT, 2 AGI), Whiting (1 CHA, 2 WIL), Seahorse (curiosity).

**Пещерная (4, W10-данные)** — cave shallows/depths: Abyss Gazer, Cavelacanth, Cave Sculpin, Pale Ghostfish (добавлялись по одному в 2018: Wish upon a Star, Searchin' Angler, Petrified Reindeer).

**Мусор**: An Old Boot, Petrified Seashell (ловятся удочкой); выброшенное в воду попадает в **глобальный пул**, рыбачится где угодно (можно бросить пергамент — message in a bottle).

### 1.6 Пассивная рыбалка — Fishing Net (2018-04-11)

4×4 предмет (Rope×6 + Block of Wood×4), ставится **только в глубокой воде** («right click it to place it exactly in deeper water», фикс 2018-06-25: нельзя в мелкой). Дословно jorb:

> *«The net catches fish over time, but also loses efficiency in doing so over time. Fish caught have a chance to disappear per unit of time, so there's a judgment call to be made on when to recover the nets, as leaving them for too long will cause them to empty.»*

Без наживки, без чисел в ченджлоге; снимать можно только с Fishing-скиллом (2019-12-11). Смежное: **Filets рыб сушатся в Drying Frame** (2018-11-24, FEP растут и концентрируются); **Roe/Caviar** — случайный дроп при разделке рыбы, у Sturgeon отдельный тип «Caviar» (2021-01-31); **Creel** (рыбацкая корзина) держит приманки/крючки/устриц/крабов/лобстеров (2021-02-21); дальность заброса конечна («Fixed a bug by which it was possible to fish at infinite distances», 2021-05-23); лески выделены в отдельные предметы патчем «Fishing for Finery» (2021-04-25, 8 видов).

### 1.7 Наблюдения сообщества — как на самом деле работает клёв

Формул jorb не публиковал, но сообщество нащупало модель. Главный источник — **Sevenless, «A brief exposé on casting rod fishing»** (2019, ~270k просмотров, актуализирован в «W16 Casting Rod Cheatsheet» 2023 на 1200 точках замеров) + дата-дамп ItsFunToLose (2022) + легаси-таблица RoB.

**Модель клёва (современная система, W12–W16):** в UI две цифры —
- **левая (bite)** = соответствие снасти `(lure+hook+line)` **+ плотность узла**;
- **правая (land)** = только снасть; `<100%` ускоряет потерю приманок.

**Плотность узла** зависит от: физической позиции (узел), **времени суток** и **фазы луны** (jorb-факторы из §1.2 подтверждены практикой). Дословно Sevenless: луна и время суток влияют на *node size, peak density* и слегка *сдвигают узел* («shift around slightly»); рыба под плохой комбинацией луна/время получает «incredibly tiny nodes/bad density» — узлы редкой рыбы бывают **~10 тайлов шириной**. Типы рыб от луны не меняются («moon affects density rather than the types of fish obtainable, I believe so») — луна = модификатор доступности, не жёсткий гейт. W16-ревизия: *«As of W16, fish nodes seem to move every full moon along with herb and animal qualities. Not just the quality, but where you can actually catch a fish»* — ежемесячный дрейф позиции узлов (jorb анонсировал тот же дрейф «каждую новую луну» для трав/животных; расхождение full/new moon — вероятно, «месячный дрейф» в терминах игроков).

**Структура узла:** узлы «roughly circular» внутри зон прыгающей рыбы; процент растёт к центру — пример из треда: **Pike 86% в одной точке узла и 11% в другой**; центр ищут «wandering around looking for the number on the left to go up». Редкая рыба часто сидит за пределами показанного списка (список = топ по `Will×Surv`, взвешенный ещё и фазой/временем).

**Снасть:** предпочтения рыбы статичны в пределах мира («a 100% combo stays 100%»); тип крючка создаёт новые комбинации («using a crab hook can completly change the result»); таблицы лучших комбинаций собраны в Google Sheets cheatsheet. Среднее Q снасти (rod/line/hook/lure) = **софткап качества пойманной рыбы** (тест с Q7.7 удочкой); Fisher credo даёт шаг «increased chance of catching rare fish» — без него редкая рыба (Sturgeon) почти невидима. Реальные уловы: ~30% базово, 70–90% на хорошем узле со снастью; даже с Q60–80 теряется ~5 приманок и 2–3 лески/крючка на 20 рыб.

**Прочие факторы:** сезон не влияет (*«Time of day perhaps, moon phase perhaps, but not season»* — дата-дамп); угорь и осётр «strictly tied to moon phases and/or time of day» (наблюдение ещё с W7, живо и сейчас: «lunar-migration of eels», рабочий сетап на угря — «bloated leeches on a waxing gibbous»); bait-fishing сильно хуже casting rod для таргет-лова (2 часа нулевого улова в тесте) и годится для AFK-масс-производства.

**Легаси-система (до реворка 2018) — таблица окон поклёвки** ([Legacy:Fishing](https://ringofbrodgar.com/wiki/Legacy:Fishing)): окно улова = **(фаза луны × время суток × локация × наживка/приманка)**; в шпаргалке сообщества все 8 фаз с точностью до 20 минут («Perch 100% — Lobster Lure — 3:00–3:30 — Waxing Crescent — Lake Shallows», «Perch 100% — Tin Fly — 16–17 — New Moon — Lake Depths»…). Это готовая данные-таблица, если захотим портить «жёсткую» версию фаз. Мелочи легаси: клёв до 2 минут ожидания, стамины не тратит, ловить не ближе 1 тайла от берега, Survival капил и снасть, и улов.

## Часть 2. Порт в origin_go

### 2.1 Готовые субстраты (что уже есть в коде)

- **Курсор уже нарисован**: `web_new/public/assets/cursor/fish.png` — действие получит родной курсор без нового арта.
- **actiondef умеет требовать снаряжение**: `Requirements.Equipment {Slots, ItemKey, ItemTag}` (internal/actiondefs/types.go:35) уже сериализуется в прото (`ActionEquipmentRequirement`, internal/game/action_service.go:156) — удочка в руке гейтится декларативно.
- **Прецедент тайлового действия**: `data/actions/plow_tile.json` + `plowTileActionHandler` (реестр internal/game/shard.go:184); таргет `kind: tile`, `approach: tile_center`, `execution {ticks, stamina}`; выдача предмета с качеством — `giveItem(..., quality)` в `dig_tile_action.go` (константа `digItemQuality = 10`). У предметов уже есть `Quality uint32` (internal/ecs/components/inventory.go:60).
- **Вода есть в контракте тайлов**: `TileDeepWater=1`, `TileShallowWater=3`, `IsTileSwimmable` (internal/types/tile.go) — естественное разделение «пресноводный/морской» наборов рыбы.
- **Детерминированное время**: `TimeState{Tick, TickRate=10}` + персист `SERVER_TICK_TOTAL` (internal/timeutil/server_time.go, internal/const/const.go:21) — лунный цикл считается чистой функцией тика, без дрейфа; light.md уже планирует `day_length_seconds` в конфиг и дневную фазу в `S2C_PlayerEnterWorld` — лунная фаза едет тем же пакетом.
- **Пассивная ловушка ложится на behaviors**: `ScheduleBehaviorTick` (internal/ecs/resources_behavior_tick.go:204) + прецеденты `burner`/`tree`; объекты описываются в `data/objects/objects.jsonc` (components: collider/inventory, behaviors, station).
- **Анимационная привязка**: `data/action_animations/*.json` — BehaviorKey+ActionID → клип (как в change sync-character-action-animations).

### 2.2 Чего нет (гэпы)

1. **Ни одной рыбы/снасти в items** — нужны defId'ы: `fishing_pole` (equippable), `earthworm` (+ пара наживок), raw-рыбы первого набора.
2. **consume-поля в actiondef нет** (та же дыра, что в lay_stone §2.2): наживка тратится за рыбу.
3. **Рыбных узлов/спотов нет** — зависят от будущей системы quality-узлов (spot.md): узел = локальный максимум шумового поля качества; пока узлов нет — рыба «однородная».
4. **Лунного цикла нет** — легко выводится из тика, но нужен конфиг длины суток (light.md F1) и поле фазы в S2C.
5. **Скиллов нет** — хардкап Survival нечего капить; requirements.skills есть, но система скиллов не реализована.
6. FEP у еды отсутствует (eat_fep.md ждёт) — до тех пор рыба это просто предмет с качеством.

### 2.3 Действие fish

- `data/actions/fish.json`: `target.kind: tile`, `approach: tile_center`, `cursor: fish`, `requirements.equipment: [{itemKey: fishing_pole}]`, `execution {ticks, stamina}`, **`isRepeatable: true`** (рыбалка циклична).
- Обработчик `fishActionHandler` — клон plow-схемы: eligibility = тайл `TileShallowWater|TileDeepWater`; на завершении — ролл вида рыбы (взвешенный по типу воды: shallow → пресноводный набор, deep → морской), `giveItem(raw_fish, 1, Q)`, списать 1 наживку из инвентаря.
- Вид/качество — точки расширения: до узлов Q константой 10 (как dig); после — Q узла, кап по «Survival»-аналогу.

### 2.4 Лунный цикл (F2, вместе с light.md)

- Конфиг: `game.day_length_seconds` (H&H-верно ≈ 26 280 с) и `game.moon_cycle_days = 30`; фаза = `floor(((dayIndex + 1.875) mod 30) / 3.75)` — 8 фаз, месяц стартует с половины New Moon (§1.4).
- Сервер шлёт фазу в S2C (тем же пакетом, что дневную фазу light.md); клиент рисует луну + использует в рыболовном UI (если появится список рыб).
- Геймплейно фаза в H&H — **модификатор плотности узла, а не гейт вида** (§1.7): у нас это множитель веса/шанса вида `base × phaseFactor(species, phase) × timeFactor(hour)` с таблицей в конфиге; плюс ежемесячный дрейф позиции рыбных узлов вместе с quality-нодами spot.md. Альтернатива — легаси-версия с жёсткими окнами (фаза × час × наживка, готовая таблица в §1.7) — проще данными, но архаичнее.

### 2.5 Fishing Net (F3)

Объект `fishing_net` в data/objects: placement только на deep water (eligibility как у fish-действия), behavior-тик по ScheduleBehaviorTick: накапливает улов с затуханием скорости, рыба с шансом исчезает — контроль раз в N минут; снятие предметом из инвентаря объекта.

### 2.6 Фазы

- **F1 MVP**: предметы (pole, earthworm, 6–10 raw fish) + действие fish (shallow/deep наборы, Q10, расход наживки) + курсор fish.png. Ноль новых прото-полей, ноль нового арта.
- **F2**: рыбные узлы (после spot-системы: Q от узла, «оптимальная точка» на вид), лунный цикл (S2C-фаза + веса по фазе), время суток как модификатор.
- **F3**: Fishing Net с behavior-тиком, полный видовой состав, Drying Frame (сушка филе — timed-процесс по образцу herbalist table), Fisher's Request-подобный бафф.

### 2.7 Открытые решения

1. Расход наживки: декларативное `execution.consume` в actiondefs (рекомендация, закрывает и lay_stone) vs handler-side списание.
2. Споты: рыба с любого водного тайла (F1) vs только видимые «прыгающие» споты (F2, нужны узлы).
3. Состав первого набора рыб: сколько видов и какой маппинг shallow/deep (у нас нет рек/озёр как отдельных типов — living-water в процессе).
4. Лунный цикл: вводить вместе с днём из light.md (F2) или отложить до F3.
5. FEP рыб: ждать eat_fep.md или закладывать данные сразу (FEP при Q10 уже собраны в §1.5).

### Тесты (по конвенциям репо)

- fish: happy path (shallow → пресноводная рыба, −1 наживка, Q10), deep → морской набор, без удочки в руке → fail по equipment, без наживки → fail, не-водный тайл → fail, repeatable-клики.
- Лунная фаза: чистая функция тика — таблица ожидаемых фаз по день-индексам (сдвиг 1.875 дня, границы 3.75), персист через рестарт (SERVER_TICK_TOTAL).
- Fishing Net (F3): тик копит улов с затуханием, рыба исчезает по шансу, снятие даёт содержимое; placement только deep water.
- Регрессия: strict-loader (`loader_test.go`), plow-тесты, поведение при consume-расширении схемы.

## Источники

- [Fishing — Ring of Brodgar](https://ringofbrodgar.com/wiki/Fishing) (два типа ловли, список рыб с FEP, наживки/приманки, кап Survival, jorb-цитата о факторах)
- [Primitive Casting-Rod — Ring of Brodgar](https://ringofbrodgar.com/wiki/Primitive_Casting-Rod) (формула списка `1+floor(Will*Surv/20)`, цитата про фазу луны)
- [Bushcraft Fishingpole — Ring of Brodgar](https://ringofbrodgar.com/wiki/Bushcraft_Fishingpole) (сборка снасти, «right-clicking an area where fish are jumping», авто-замена наживки)
- [Seasons — Ring of Brodgar](https://ringofbrodgar.com/wiki/Seasons) (8 фаз, цикл 30 дней, фаза 3.75 дня, старт месяца в половине New Moon, длины сезонов/года в реале)
- [Fishing Net — Ring of Brodgar](https://ringofbrodgar.com/wiki/Fishing_Net) + [Game Updates/Changes/2018](https://ringofbrodgar.com/wiki/Game_Updates/Changes/2018) (Wood Grease 2018-04-11: цитата jorb про сеть; Navy Seals 2018-01-31: Fishing←Foraging; Wish upon a Star 2018-08-20: пещерные рыбы; Fishy Owl 2018-11-24: сушка филе)
- [Earthworm — Ring of Brodgar](https://ringofbrodgar.com/wiki/Earthworm) (добыча наживки)
- [Quality — Ring of Brodgar](https://ringofbrodgar.com/wiki/Quality) (QM=Q/10, FEP×QM, дефолт Q10)
- [spot.md](spot.md) (рыба как quality-узел: jorb «limited and localized resource», W16-дрейф полей — травы/животные)
- [A brief exposé on casting rod fishing — форум, Sevenless](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=65074) (модель узла/клёва: bite = снасть+плотность, луна/время суток → размер и плотность узла, софткап Q снастью)
- [W16 Casting Rod Cheatsheet — форум, Sevenless](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=74903) (1200 точек замеров, дрейф узлов «every full moon», Fisher credo и редкая рыба)
- [Fishing and Autism; Data dump — форум, ItsFunToLose](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=73993) (замеры комбинаций по узлу, «not season», слабость bait-fishing)
- [fishing chances — форум](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=65017) (Pike 86%↔11% внутри узла, left% = присутствие в узле)
- [about fishing (World 7) — форум](https://www.havenandhearth.com/forum/viewtopic.php?f=2&t=31486) (eel/sturgeon «strictly tied to moon phases and/or time of day», bloated leeches on a waxing gibbous)
- [Legacy:Fishing — Ring of Brodgar](https://ringofbrodgar.com/wiki/Legacy:Fishing) (легаси-таблица окон (фаза × час × локация × наживка) по всем 8 фазам)
- [Moon cycle and fishing — форум, 2011](https://www.havenandhearth.com/forum/viewtopic.php?f=2&t=18645) (ранние наблюдения фаз)
- Исследование 2026-09-28 (память `hnh-fishing-research`)
