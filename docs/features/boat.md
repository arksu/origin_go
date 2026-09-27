# Boats / водный транспорт (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-27 по Ring of Brodgar wiki (Boat Building, все 6 лодок, High Seas), официальному форуму; порт размечен по отчёту explore по lift/carry и водным системам. Смежные документы: [personal_claim.md](personal_claim.md), [village_claim.md](village_claim.md).

## Часть 1. Механика в H&H

### 1.1 Скилл и типы лодок

Скилл **Boat Building** (1 500 LP; пререквизиты Carpentry + Swimming) открывает 6 плавсредств: **Coracle, Raft, Dugout, Rowboat, Snekkja, Knarr**. В открытом океане выживают только Snekkja и Knarr; все шесть ходят по мелководью; «лодки намного безопаснее плавания».

| Лодка | Материалы / постройка | Габарит | Пассажиры | Грузовые слоты | Скорость | Liftable | Вода |
|---|---|---|---|---|---|---|---|
| **Coracle** | Leather×8, Block×8, Bone Glue×2 | 4×3 инвентарь | 1 | — (это инвентарный предмет, носится в Cape-слоте) | н/д | инвентарный предмет | мелководье, болота; дроп с инвентаря на воду → board |
| **Raft** | 2 сегмента (Block×10 + Rope×2 каждый), сегменты строятся на суше, соединяются на воде | 3.7×3.7 | несколько | загрузка курсором (без числа) | crawl | «Take Apart» → 2 секции, каждая liftable | реки; может майнить с глубокой воды; HS убивает |
| **Dugout** | 1 Log + топор; **выжигание** внутренней части ~1 игровой час (~20 реальных минут) | 0.4×2 | 1 | 0 | **4.54 тайла/с** (run) | да (нельзя, пока горит) | болота/боги; берег; HS убивает почти сразу |
| **Rowboat** | 10 Blocks + 25 Boards на **суше**, затем **2.5 L Tar** (нетарьёная unusable) | 1.3×2.9 | 2 | **2 слота — пассажиры ИЛИ liftable** | **7.5 тайлов/с** (≈run) | да: ~51 STR — идти, ~200 STR — бежать; влезает в **Cart/Wagon** (предварительно опустошённые); нельзя в другую лодку | мелководье+берег; нетарьёная полупогружена; затарённая — 1 тик урона HS |
| **Snekkja** | в **воде**: Rope×8, Bar of Hard Metal, Tarsticks×10, Bone Glue×5, Board×60, Block×25 + отдельная **Snekkja Sail** | 2.2×5.8 | 4 | **16 cargo units** + инвентарь 5×5 | быстрая на побережьях; в HS сильно замедлена | нет | океан; короткие HS-переходы; замки; док |
| **Knarr** | в **воде**: Rope×65, Cloth×50, Tarsticks×50, Bar of Common Metal×20, Sunstone, Bone Glue×20, Board×200, Block×100 + **Knarr Sail** | 3.3×10.6 | **10** | **64 cargo units** + инвентарь 10×10 | base 4 (=rowboat); **самая быстрая на High Seas** | нет | HS-король; замки; док; HP 2500 |

### 1.2 Слоты для liftable-объектов (ядро запроса)

- **Слоты делятся между пассажирами и liftable-объектами** (Rowboat: 2 слота «passengers or liftable objects»). В большие корабли кладут Coffers, Crates, трупы животных («16/64 units of cargo»).
- **Погрузка сидя**: сидя в лодке берёшь liftable-объект в руки → right-click → объект убирается в слот лодки (если есть место). У Raft — режим «load cursor»: правый клик по палубе, левый клик по объектам.
- Ограничения: нельзя грузить **живых/диких животных**, **нелифтабельные транспортные средства** и **лодки в лодки**; некоторые контейнеры (**Cupboards**) — только пустыми; Rowboat в Cart — только опустошённый.
- Поднятие лодки **выселяет** находившихся внутри оффлайн-игроков (2015, анти-телепорт).
- Пока сидишь — можно рыбачить; Dugout/Coracle могут тянуть Cart по мелководью.

### 1.3 Посадка/выход и движение

- **Board**: right-click по лодке, стоящей на водном тайле. **Exit: Ctrl+left-click** в желаемом направлении. Выход в глубокую воду **блокирован** без скилла Swimming; со скиллом — можно утонуть.
- Управляет рулевой (движение водителя ведёт лодку); пассажиры сидят. Snekkja/Knarr — рулевому достаточно быть одним.
- Скорости заданы на тайл/с; с W14 (2022) **качество не влияет на скорость** (влияет на HP-пул) — все корабли одного типа равны, погони решаются скиллом.

### 1.4 Урон, ремонт, морские зоны

- **High Seas** («очень глубокий океан», темнейший бирюзовый тайл) наносит **периодический урон корпусу** всем лодкам; ремонт **только на мелководье** (Tarsticks восстанавливают 40% HP-пула). Затарённый Rowboat выдерживает 1 тик; Dugout/Raft гибнут почти сразу; Snekkja — короткие переходы; Knarr — длинные.
- **Seaworthiness meter** (2021) показывает текущий урон корпуса.
- Snekkja HP рушится руками/инструментами (осадка не нужна), но **у дока на клайме руками ломать нельзя** (2020); Catapult по кораблям (2025/2026 правки).
- Ремонт мелких: Rowboat — Boards, Dugout — Block of Wood, Coracle — Prepared Animal Hide.

### 1.5 Доступ, замки, доки

- Лодка **«помнит» последнего пользователя** — доступ независимо от permissions.
- Snekkja/Knarr: **замки** (Wooden/Metal/Steel; невскрываемые; без master key контроль теряется мгновенно; Slave Keys для команды). **Груз НЕ защищён** — вне клайма лутается всеми.
- **Ship Dock** (2020): доканка; «Travel to Home Dock» — телепорт корабля+рулевого за тяжёлый **Travel Weariness**; KO-рулевой может «Travel to Hearth Fire»; рулевой может «Maroon» KO-пассажира на мелководье (2022).

### 1.6 Клиент

- Лодки — обычные gob-объекты с 8 фасингами; пассажиры отображаются сидящими внутри; анимация боббинг-в-воде и водный след (2022); immersion/ватерлиния.

## Часть 2. Порт в origin_go

### 2.1 Готовые субстраты (отчёт explore)

| Механика | Готовый код |
|---|---|
| Объект-лодка со спрайтом | `web_new/src/game/objects/vehicles.json` **уже содержит boat-спрайт с 8 фасингами**; ObjectDef + behaviors registry |
| Follow-механика за носителем | `SyncLiftCarryFollow` (`internal/game/lift_service.go:305`) — per-tick `RelocateWorldObjectImmediate` + `S2C_ObjectMove{carried_by_entity_id}`; точный образец для «лодка следует за рулевым» |
| Лифтинг малых лодок | behavior `"lift"` + `data/actions/lift.json`; STR-гейта нет (карьера капает режим Walk — `applyCarryMoveCap`) |
| Слоты/хранилище состояния | `ObjectStateEnvelope{v, behaviors:{...}}` в `object.data` — туда пишем `boat:{slots:[...]}` |
| Вода | `TileDeepWater=1/ShallowWater=3`, `IsTileSwimmable` (`internal/core/chunk.go:430`), режим Swim (`collision.go:179`), стоимость `SwimStaminaCostPerTick`; ResolveTileStaminaModifier — заготовка под модификаторы |
| Обнуление коллизий (посадка) | `Collider.Layer=0, Mask=0` (так lift нейтрализует носимый объект) + phantom-коллайдеры |
| Релокация объектов | `RelocateWorldObjectImmediate` (`world/object_relocate.go`) — SpatialHash+ChunkRef+AOI; требует **активный чанк** (рядом с игроками — истина) |
| Клиентская привязка | `ObjectManager.setCarryVisualRelation`/`syncActiveCarryVisuals` (`ObjectManager.ts:148,211`): position override, z-index, interaction suppression — образец для «пассажир сидит в лодке»; ватерлиния — `ShallowWaterVisual`/`shallowWaterConfig.ts` |
| Выход по направлению | MapClick при aboard → unboard в соседний тайл (роутер `handlePrimaryMapClick`) |

### 2.2 Модель

**Лодка** — объект с behavior `"boat"`; состояние в envelope:

```
boat: {
  slots: [ {kind:"passenger", entity_id} | {kind:"object", entity_id} | null ] // Rowboat: 2
  driver_entity_id, last_user_id, tarred: bool (Ф3)
}
```

**Пассажир** — компонент `BoatPassengerState{BoatEntityID, SlotIndex}` на игроке; **рулевой** = пассажир в слоте 0 с правом управления (Rowboat: оба слота равнозначны, рулевой — кто вошёл).

**Движение (KISS, рекомендовано)**: игрок-рулевой остаётся обычным мовером в режиме Swim-аналога («boat mode»: ходит только по swimmable-тайлам, скорость = конфиг лодки, 7.5 тайл/с ≈ run), лодка **следует** за ним per-tick (образец carry-follow), пассажиры ко-двигаются аналогично. Альтернатива (boat-as-mover: Movement на объекте + перер роутинг ввода) — отклонена как избыточная для Ф1.

**Посадка**: контекст-экшен `board` на behavior (ContextActionProvider) при лодке на водном тайле; выход — MapClick по направлению → unboard в соседний тайл (глубокая вода — только если режим Swim разрешён).

**Слоты**: взял в руки liftable → right-click по лодке → `stow` (объект уходит с карты в слот, деспавн + запись entity_id); выгрузка — контекст-экшен на лодке → спавн объекта на соседнем тайле. Ограничения Ф1: только behavior `"lift"`, запрет boat-in-boat, «пустые контейнеры» — конфиг.

**Персистентность**: слоты/рулевой в `object.data` (envelope), позиция — обычный object row (chunk_x/y пересчитываются при сохранении чанка); лодка движется только с рулевым ⇒ чанки активны (AOI). Оффлайн-владелец в лодке при рестарте: пассажиры-игроки восстанавливаются как BoatPassengerState (или выгружаются на ближайший тайл — решить).

### 2.3 Прото и клиент

- Переиспользовать `S2C_ObjectMove.carried_by_entity_id` для co-move (семантика «attached to») или добавить `boat_entity_id` — решение при имплементации.
- Клиент: `setScreenPositionOverride`-привязка лодки к рулевому (и пассажиров к лодке), сидячая поза (аналог `setCarrying`), ватерлиния через ShallowWaterVisual; interaction suppression на лодке-объекте при aboard.

### 2.4 Фазы

1. **Ф1 MVP**: rowboat (постройка на суше, без тара), board/unboard (1–2 чел.), водное движение с фиксированной скоростью, **2 общих слота пассажир/liftable**, stow/unstow, персист, клиентская привязка + ватерлиния.
2. **Ф2**: STR-гейт переноса лодки (скорость Walk/Run по STR), HP+ремонт, «помнит последнего пользователя», dugout/coracle (одноместные дешёвые), restrictions-полировка.
3. **Ф3**: Tar (2.5L → usable), High Seas как зона карты (урон корпусу по тику, seaworthiness, ремонт только на мелководье), Snekkja/Knarr (многопассажирные, cargo units, замки), Ship Dock.

### 2.5 Открытые решения

1. Рулевой-модель: игрок-мовер + лодка-фолловер (рекомендовано) vs лодка-мовер.
2. Пассажиры видимы сидящими (как в H&H) vs скрыты.
3. `carried_by_entity_id` реюз vs новое поле.
4. Выход в глубокую воду: блок (H&H) vs разрешён при Swim-режиме.
5. Пассажиры-игроки при рестарте сервера: восстанавливать в лодке или выгружать.
6. Ограничения слотов (животные, не-лодки) — актуальны после появления животных/кортов.

### Тесты (по конвенциям репо)

- Go: boat behavior state (slots stow/unstow/переполнение), board/unboard routing, водный режим движения (запрет суша), co-move + chunk migration, рестарт с пассажирами.
- Web: packet-shaping (board → позиционные апдейты), harness-страница привязки лодки (образец `tests/hybrid-integration.ts`).

## Источники

- [Boat Building — Ring of Brodgar](https://ringofbrodgar.com/wiki/Boat_Building), [Rowboat](https://ringofbrodgar.com/wiki/Rowboat), [Coracle](https://ringofbrodgar.com/wiki/Coracle), [Dugout](https://ringofbrodgar.com/wiki/Dugout), [Raft](https://ringofbrodgar.com/wiki/Raft), [Snekkja](https://ringofbrodgar.com/wiki/Snekkja), [Knarr](https://ringofbrodgar.com/wiki/Knarr), [High Seas](https://ringofbrodgar.com/wiki/High_Seas)
- Форум: [How do i Boat?](https://www.havenandhearth.com/forum/), [Locks for rowboats](https://www.havenandhearth.com/forum/), [Boat damage](https://www.havenandhearth.com/forum/)
- [Raft — Fandom](https://havenandhearth.fandom.com/wiki/Raft), [Boat Building — Fandom](https://havenandhearth.fandom.com/wiki/Boat_Building)
