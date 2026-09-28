# Animals: лиса и кролик (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 по Ring of Brodgar wiki (Rabbit Buck, Fox, Hunting) и перекрёстно с ранее собранными исследованиями: спавн/качество животных — [spot.md](spot.md) и память `hnh-spot-resources-research`, боевой ИИ животных — [combat_system.md](combat_system.md), disengage/execute — [knocked_dead.md](knocked_dead.md). Порт размечен по актуальному коду.

## Часть 1. Механика в H&H

### 1.1 Общая модель диких животных

- **Спавн**: фиксированные точки спавна, привязанные к quality-нодам; **качество животного фиксировано точкой** (буст ноды «до +500%»); спавн происходит на unload/reload тайла (~15-минутный цикл проверки) **плотными стадами, которые потом разбредаются**; часть спавнов зависит от дня/ночи. W16: noise-поля животных **дрейфуют каждое новолуние** (быстрее трав). Заядлый анти-паттерн H&H: кемпинг точки с логином каждые 15 минут.
- **Клеймы**: на клеймленной земле wildlife **не спавнится** (травы — спавнятся).
- **Боевой ИИ** (Legacy): у зверей скриптовые боевые сценарии — накопитель атаки, «поддерживающие» движения, удары при заполненной шкале (лиса — укус/когти при полной атаке); per-hit disengage-роллы; хищники (волк/кабан/медведь/рысь…) могут добивать KO — лиса и кролик в списке исполнителей не состоят.

### 1.2 Кролик (Rabbit Buck / Rabbit Doe)

| Параметр | Значение |
|---|---|
| Размер | 0.2×0.2 тайла |
| Base-Quality | 10 |
| Hitpoints | 30 |
| Fleeing-Hitpoints | 30 (убегает сразу) |

- **Среда**: 31 тип terrain — леса (бук/дуб/сосна…), Grass, Heath, Moor, Flower Meadow, Dry Flat.
- **Поведение**: попытка интеракта на расстоянии — **убегает на running speed** («сначала подойди вплотную»).
- **Мелкая дичь — без боя**: кролика можно **поднять в инвентарь** (слот 2×2), на минимапе тоже; в инвентаре убивается действием **Wring Neck** → Dead Rabbit. Живой кролик **вне клейма деградирует и умирает без еды**, внутри клейма — впадает в спячку (анти-гриф складирования).
- **Дроп из Dead Rabbit**: Fresh Rabbit Fur, Entrails, Tiny Brain, **Raw Rabbit ×2**, Bone Material, **Lucky Rabbit's Foot (очень редко)**.
- Разведение (вне первого порта): Rabbit Hutch + Doe + Buck, скилл Animal Husbandry, «плодятся быстро».

### 1.3 Лиса (Fox)

| Параметр | Значение |
|---|---|
| Размер | 0.4×0.6 тайла |
| Base-Quality | 25 |
| Hitpoints | 110 |
| Fleeing-Hitpoints | 75 (дерётся до 32% HP) |

- **Среда**: поодиночке во всех лесных и травяных terrain круглый год; **Fox Holes** (апдейт «Foxhole Foxtrot», 2023-05-11) — мини-подземелья с большим числом лис.
- **Поведение**: **нейтральна, пока не атакуешь** — тогда вступает в бой и преследует. **Быстрее бегущего игрока** — догнать можно только спринтом, верхом или на технике. **Охотится на мелкую дичь** (белки, кролики) и **атакует укравших её добычу**. Сами лисы — добыча барсуков и росомах (убивают и оставляют труп, который свободно можно забрать).
- **Бой**: слаба, не использует Oppressive Openings — «хороший первый противник» (~15 в боевых атрибутах). Атаки: Fell Scratch (Striking/Backhanded, Off Balance), Chomp (Striking/Sweeping, Dizzy); реставрации Bristle/Tail Spin.
- **Разделка требует скилл Hunting**: Skin → Fresh Fox Hide (+редкая Fresh Sly Ear of the Fox), Clean → Intestines/Entrails, Butcher → **Raw Fox ×3** (+Small Brain с Fine Butchery), Collect Bones → **Bone Material ×2**. **Порядок важен: skin до butcher** — иначе шкура портится.
- **Качество**: Q трупа считается в момент смерти и **жёстко капится и Survival охотника, и качеством оружия/инструмента**; лисы, **убитые другими зверями, обходят оба капа** — способ для новичка добыть высококачественную лису. При разделке — двойной softcap: сначала инструмент ((Q+ToolQ)/2), потом Survival ((Q+Survival)/2).
- **Применение**: Fox Hat (Tanning), Fox Hide Patch, еда с INT/AGI FEP (Roast Fox, Fox Fuet STR+3/AGI+3, Fox Wurst INT+3/PER+2), Bone Ash.

### 1.4 Hunting — скилл охотника

- **60 LP**, пререквизит Foraging; **необходим для pickup/attack/kill/skin/butcher** существ; открывает Animal Husbandry, Archery, Tanning.
- Лук: вертикальный aim-метр — полный бар = точность/урон, мгновенный выстрел почти гарантированный промах.
- Мелкая дичь (курицы, кролики, белки, ежи, кроты, гадюки) — без боя; крупная — бой («лодочный метод» против почти всей крупной дичи).

## Часть 2. Порт в origin_go

### 2.1 Субстраты (что уже есть в коде)

- **Клиентская часть животных готова**: web_new/src/game/objects/animals.json — рендер-дефы **fox, deer, bear, aurochs, mufflon, sheep** с 8 направлениями (арт animals/fox/0..7.png из H&H-ассетов). Серверной части нет (data/objects: containers/objects/trees — животных нет; behaviors: build/burner/container/player/lift/player_death/take/tree).
- **ECS-движок**: Transform с Direction (8 направлений — уже для клиентских дефов), Movement + коллайдеры + spatial dynamic (прецедент — игроки), behaviors registry с contracts (ProvideActions/Validate/Execute).
- **HP-пулы и смерть**: entityhealth (SHP/HHP), player_death_system — прецедент превращения сущности в статичный труп-объект (`player_death` → по аналогии `dead_rabbit`/`dead_fox`); take_behavior — прецедент забора лута.
- **Спавн**: world/def_spawner.go `SpawnEntityFromDef` — спавн по ObjectDef с качеством/направлением — база для спавн-точек.
- **Качество**: Quality у объектов/стаков есть; quality-ноды ([spot.md](spot.md)) — research-стадия.
- **Чего нет — критично**: **боевой системы** (grep Attack по internal/game — пусто; data/actions — только lift/plow; есть только HP-пулы/KO), ИИ-систем, спавн-точек, скилл-гейта Hunting как проверки (скиллы есть в модели персонажа).

### 2.2 Дизайн порта (лиса и кролик — первая пара)

Осознанный выбор пары: **кролик не требует боя** (pickup → Wring Neck → разделка), **лиса требует** — она же станет первым бенчмарком будущей боевой системы.

- **F1 «Живот-сущность + ИИ»**:
  - objectdef `rabbit`/`fox` (не-static, крошечный коллайдер 0.2×0.2 / 0.4×0.6, DisplayState по 8 направлениям из animals.json — Direction уже транслируется клиентом);
  - **AnimalAISystem** (per-tick, аналог movement-системы, а не low-freq behavior ticks): state machine `Idle → Wander → Flee` (+ позже `Chase`). Кролик: flee при приближении игрока на N тайлов (в H&H — интеракт издалека), скорость ≈ бег игрока. Лиса: neutral; wander; chase/fight атакующего до Fleeing-HP (75/110), затем flee со спринт-скоростью (> run игрока);
  - despawn при unload чанка + respawn-цикл спавн-точек (~15 мин, [[spot.md]]).
- **F2 «Убийство и лут»** — детальный план пайплайна разделки вынесен в **Часть 3**:
  - кролик: действие pickup → live-предмет в инвентарь (2×2 — проверить, поддерживает ли наш инвентарь размеры; если нет — упростить) → Wring Neck → разделка (см. Часть 3, §3.5);
  - лиса: нужен **минимальный attack-экшен** (простой melee-удар с кулдауном поверх entityhealth) — либо отложить лису до боевой системы;
  - труп-объект `dead_fox` с действиями **Skin → Clean → Butcher → Collect Bones** (behavior на трупе, гейт Hunting) — полная спецификация в Части 3.
- **F3 «Качество и спавн-точки»**: точки по quality-нодам (spot.md), фиксированный Q точки, стада кроликов (spawn группой + scatter), day/night фаза, капсы: Q трупа ← min(Survival, weapon) в момент смерти; softcaps инструмента и Survival при разделке. Правило «в клеймах wildlife не спавнится» — после клеймов.
- **F4 (далеко)**: разведение (Rabbit Hutch, Animal Husbandry), симуляция хищничества (лиса охотится на кроликов), Fox Holes как мини-подземелья (после шахт/подземелий), остальные 4 животного из animals.json (deer/sheep — база для Animal Husbandry).

### 2.3 Открытые решения

1. **Бой для лисы**: минимальный attack-экшен сейчас (упрощённый, без deck-системы) или отложить лису до порта боёвки [combat2.md]. Кролик в обоих случаях доступен.
2. **Live pickup кролика**: нужен инвентарный предмет 2×2 и деградация вне клейма — либо упрощение (мгновенный corpse).
3. **ИИ**: per-tick AnimalAISystem (рекомендация — движение животных должно быть плавным и видимым) vs behavior ticks.
4. **Персистенс**: животные не персистятся (respawn-цикл как в H&H) — состояние спавн-точек персистится, сами звери нет.
5. **Числа**: брать H&H (HP 30/110, Q10/Q25, fleeing-пороги) или масштабировать под наш баланс.
6. Скилл Hunting: заводить скилл с гейтом на pickup/attack/skin или отложить гейты.

### Тесты (по конвенциям репо)

- ИИ: переходы Idle/Wander/Flee по дистанции/урону, facing по направлению движения, despawn/respawn цикла точки.
- Разделка: гейт Hunting, порядок skin→butcher (butcher первым портит шкуру), дроп-таблица и редкие шансы, капсы качества (weapon/Survival/tool).
- Регрессия: существующие тесты behaviors/entityhealth зелёные; новые objectdefs проходят валидацию загрузчиков.

## Часть 3. Детальный план: пайплайн разделки (Skin → Clean → Butcher → Collect Bones)

### 3.1 Правила H&H — что документировано, а что вывод

- **Документировано (RoB Hunting)**: «You must skin before butchering — butchering first ruins the hide»; «можно отойти в середине butcher и вернуться (к шкуре или к оставшемуся мясу)». Т.е. butcher — **прогрессивное** действие (мясо снимается по кускам), а нарушение порядка **не блокирует** butcher — он **уничтожает шкуру** (Skin исчезает).
- **Clean (потрошение)** и **Collect Bones** — отдельные действия над трупом; их точный порядок вики не фиксирует (каноническая последовательность Skin → Clean → Butcher → Bones — игровой обычай).
- **Dead Rabbit — исключение из пайплайна**: это **инвентарный предмет** 1×2 (результат Wring Neck над живым кроликом), разделывается из инвентаря без стадий: сразу Raw Rabbit ×2 + Bone Material + Fresh Rabbit Fur, «стартуют с quality 10», шанс Raw Lucky Rabbit's Foot.
- **Качество**: база — Q трупа; при разделке два softcap'а подряд: сначала инструмент — если ToolQ < Q, то Q ← (Q+ToolQ)/2, затем Survival — если Survival < Q, то Q ← (Q+Survival)/2. На момент смерти действует жёсткий кап Q трупа от оружия (после появления оружия).
- **Fine Butchery** — отдельный скилл для Small Brain (мозги есть не у всех и не у всех выходов).

### 3.2 Модель данных (наш порт)

Труп лисы — **мировой объект** `dead_fox` (прецедент: `player_death`-труп) с behavior `butcherable` и персистентным состоянием по образцу `BuildBehaviorState`:

```go
components.CorpseButcheryState{
    CorpseQuality    uint32 // Q животного (спавн-точка); дефолты F2: fox 25, rabbit 10
    Skinned          bool
    Cleaned          bool
    ButcheryStarted  bool   // первое completed-цикл butcher
    MeatLeft         uint8  // raw fox ×3
    BonesLeft        uint8  // bone material ×2
    SkinningDone     bool   // редкий дроп разыгран при Skin
}
```

Кролик — упрощённая ветка без трупа-объекта (§3.5).

### 3.3 Действия и гейты

Behavior `butcherable` динамически предоставляет контекст-действия (паттерн burner/builde) с преусловиями:

| Действие | Доступно когда | Результат | Циклы |
|---|---|---|---|
| `skin` | !Skinned && !ButcheryStarted | Fresh Fox Hide (+ шанс Sly Ear) | 1 цикл |
| `clean` | !Cleaned | Intestines + Entrails | 1 цикл |
| `butcher` | MeatLeft > 0 | 1 × Raw Fox за цикл | MeatLeft циклов |
| `collect_bones` | BonesLeft > 0 | 1 × Bone Material за цикл | BonesLeft циклов |

- **Гейт Hunting** — бинарная проверка `profile.Skills` (`action_requirements.go:44` — скиллы у нас списки-имён, без уровней): все 4 действия скрыты/отказаны без скилла.
- Правило порядка: **Skin исчезает из меню после первого завершённого цикла butcher** (эквивалент H&H-штрафа «испортил шкуру», но без фрустрации потери). Альтернатива — оставить H&H-семантику (Skin доступен, но даётNothing) — решение в §3.7.
- Каждый шаг — **cyclic action** по образцу build (`startBuildCyclicAction`/`HandleBuildCycleComplete`): один завершённый цикл = один продукт через `contracts.GiveItemFn` (player_give_item.go → `InventoryOperationService.GiveItem(world, playerID, handle, itemKey, count, quality)` — **принимает quality**), отмена/отход в любой момент — состояние трупа сохраняется (MeatLeft и т.д.), «достроить» может любой игрок с Hunting.
- Тухлость/ despawn трупа: таймер (в H&H трупы decay) — открытый вопрос §3.7.

### 3.4 Качество продуктов

```
q := CorpseQuality
если в руке инструмент (например, stone_axe) и его Q < q:  q = (q + ToolQ) / 2
если есть уровень Survival и он < q:                        q = (q + Survival) / 2
```

- Первый шаг можно реализовать сразу (инструменты и Q есть). Второй **отложен**: скиллы у нас бинарные, уровней Survival нет — включить при появлении скилл-левелов (или заменить прокси-атрибутом — решение).
- Weapon-hardcap в момент смерти — после появления боевой системы (в F2 для лисы от минимального attack-экшена — кап по Q инструмента удара).
- Редкие дропы (Lucky Rabbit's Foot, Sly Ear) — ролл при соответствующем действии, Q = CorpseQuality.

### 3.5 Кролик: упрощённая ветка

- `wring_neck` над живым кроликом (в инвентаре) — **сразу выдаёт продукты** (Raw Rabbit ×2, Bone Material, Fresh Rabbit Fur, шанс Foot), минуя промежуточный предмет `dead_rabbit` — итог H&H тот же, минус лишняя сущность. Промежуточный предмет 1×2 — опция, если появится use-item-действие.
- Требует Hunting (в H&H разделка кролика тоже под скиллом).
- Все нужные **продуктовые items уже существуют**: `rabbit_meat`, `fox_meat` (+ `roasted_*` варианты для будущей готовки); добавить придётся только: `bone_material`, `fresh_rabbit_fur`, `fresh_fox_hide`, `entrails`, `intestines`, `small_brain`, `lucky_rabbits_foot`, `sly_ear`.

### 3.6 Клиент

- Контекст-меню трупа — существующий конвейер ContextActionService (действия приходят из behavior ProvideActions, доступность пересчитывается по состоянию трупа).
- Прогресс-бары шагов — уже работающие анимации cyclic-действий (cursor для разделки — из H&H-атласа курсоров есть harvest/atk).
- Состояние трупа («освежёвана», «осталось мяса: 2») — в S2C-снапшоте объекта по образцу BuildState (можно минималистично: actions availability + client-side отображение по состоянию).

### 3.7 Открытые решения

1. **Штраф за порядок**: Skin исчезает после первого butcher (рекомендация: чётко, без потери ресурсов) vs H&H-семантика «шкура испорчена» (Skin доступен и даётNothing).
2. **Wring Neck**: сразу продукты (рекомендация) vs промежуточный предмет dead_rabbit 1×2.
3. **Despawn трупа**: таймер (сколько?), превращение в skeleton/bone pile или бесконечный.
4. **Survival-softcap**: ждать скилл-левелы vs прокси.
5. **Fine Butchery / Small Brain**: включать ли второй скилл в F2 или отложить предмет.
6. **Мультиплеер**: достраивать труп может любой игрок с Hunting (как стройка) или только убийца.

### 3.8 Тесты

- Преconditions: skin недоступен после ButcheryStarted; butcher недоступен при MeatLeft=0; все действия скрыты без Hunting.
- Циклы: N продуктов ровно по MeatLeft/BonesLeft; отмена середины сохраняет состояние; второй игрок продолжает.
- Качество: pipeline §3.4 (с инструментом / без), rare-роллы (seeded).
- Регрессия: существующие behaviors/entityhealth зелёные; загрузчик objectdefs принимает новые дефы.

## Источники

- [Rabbit Buck — Ring of Brodgar](https://ringofbrodgar.com/wiki/Rabbit_Buck) (+ disambig [Rabbit](https://ringofbrodgar.com/wiki/Rabbit) → Buck/Doe)
- [Fox — Ring of Brodgar](https://ringofbrodgar.com/wiki/Fox)
- [Hunting — Ring of Brodgar](https://ringofbrodgar.com/wiki/Hunting) (порядок «skin before butchering», softcaps, прогрессивность)
- [Dead Rabbit — Ring of Brodgar](https://ringofbrodgar.com/wiki/Dead_Rabbit) (инвентарная разделка кролика, «стартуют с quality 10»)
- [spot.md](spot.md) / память `hnh-spot-resources-research` — спавн-точки, Q точек, стада, дрейф W16
- [combat_system.md](combat_system.md) — боевой ИИ животных Legacy; [knocked_dead.md](knocked_dead.md) — disengage/execute
- Код: web_new/src/game/objects/animals.json, internal/game/world/def_spawner.go, internal/entityhealth/, behaviors registry, internal/inventory/give_item.go (GiveItem с quality), player_give_item.go (contracts.GiveItemFn), cyclicaction/build-паттерн циклов, action_requirements.go:44 (бинарные скиллы)
