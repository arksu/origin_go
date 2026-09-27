# Light Source / день-ночь / луна (Haven & Hearth) — механика и план порта в origin_go

Референс: 2026-09-27 (день/ночь, источники света) + 2026-09-28 (календарь, лунный цикл, клиентский протокол) — Ring of Brodgar wiki (Glossary, Seasons, Category:Light Sources, Torch, Lantern, Candelabrum, Wolf, Primitive Casting-Rod), форум havenandhearth.com (посты jorb/loftar), код dolda2000/hafen-client (Glob.java, Cal.java, Astronomy.java, Light.java, Lighting.java) и nephtyws/amber-client. Порт размечен по коду (grep-обследование рендера/тайминга/станций). Смежные: [boat.md](boat.md), [spot.md](spot.md) (quality-ноды → дрейф новолуния), [animals.md](animals.md) (волки), [hearth_fire.md](hearth_fire.md).

## Часть 1. Механика в H&H

### 1.1 Время и календарь

- Игровые сутки 24ч = **7ч 17м 41с** реального времени (loftar, t=74504; вики округляет до «7ч18м»/«8ч»); масштаб **×3.29** (loftar, t=61963). День = ночь = 12 игровых часов ≈ 3.6 реальных часа («Day is four real-life hours, while night is the same» — Glossary).
- Календарь: **год = 180 игровых дней = 6 месяцев**; **месяц = один лунный цикл = 30 дней** (формат DD-MM-YY); сезоны: весна 30 / лето 105 / осень 30 / зима 15 дней; год начинается с первого дня весны, **новые миры стартуют в начале лета**; система сезонов введена обновлением «Season's Greetings» 2019-08-21. Реальная длительность: месяц ≈ 9д 2ч 51м, год ≈ 54д 17ч 4м.
- Часы UI — виджет-«календарь» сверху экрана (Cal.java, панель gui-um): фон день/ночь, солнце и луна вращаются по орбите-кругу, луна тонируется серверным цветом; числовых часов нет — при наведении тултип «N-й день N-го месяца N-го года».

### 1.2 Лунный цикл

- **8 фаз** × 3.75 игровых дня = 30-дневный цикл = календарный месяц: New Moon → Waxing Crescent → First Quarter → Waxing Gibbous → Full Moon → Waning Gibbous → Last Quarter → Waning Crescent.
- **«Each month starts halfway in the New Moon phase»** (Seasons wiki): mp=0 — центр новолуния = 1-й день месяца; полнолуние ≈ середина месяца (день ~15, mp≈0.5).
- Сервер шлёт фазу дробью **mp∈[0..1)** в glob-сообщении «astro»; кадр луны = `round(mp * frames) % frames` (Cal.java).

### 1.3 Эффекты луны и времени суток

- **Темнота**: ночью сервер шлёт тёмные ambient/diffuse цвета освещения; лунный цикл модулирует ночную освещённость — «On a new moon there will be barely any light» (Glossary), полнолуние — сумеречно. Без источников света ночью не видно → свет жизненно необходим.
- **Дрейф quality-нод (W16)** — jorb, «Prelude: World 16» (2024-10-28): *«The quality noise fields of foragables and animals drift every new moon, with animal quality drifting faster than herb quality.»* Замеры первого дрейфа (t=76677): растения ≈ 1.5 тайла на запад и 1 на север; направление/дистанция, вероятно, случайны и независимы по типам нод (bug-ноды дрейфуют иначе).
- **Волки**: «found in mountains or occasionally in forests if the moon is full» (Wolf wiki). Антволки — «once under a blue moon...» (jorb, 2018-12-14) — «blue moon»-механика намеренно не раскрыта.
- **Рыбалка**: Primitive Casting-Rod перечисляет «...the moon phase...» среди факторов улова; современная страница Fishing (цитата jorb) луну не упоминает — конфликт источников.
- **Время суток**: рыбы/форажи по времени («Certain items spawn at certain times» — Dewy Lady's Mantle: сбор на рассвете 4:45–7:15 игровых часов), experience-события привязаны ко времени. **Не** зависит от луны: farming, WWW, кража/stealth.

### 1.4 Источники света (категория Light Sources, 29 объектов)

**Статические (горят/светят на базе):** Fire/Campfire, Fireplace, **Hearth Fire**, Kiln, Oven, Ore Smelter, Finery Forge, Stack Furnace, Tar Kiln, Smoke Shed, Brazier, Torchpost, Lantern Hanger/Post/Stand (инфраструктура подвеса), Pumpkin Lantern, Snow Lantern, Candelabrum, Village Claim (идол светится при основании).

**Переносные:**

| Предмет | Топливо/заправка | Время горения | Где носится | Особенности |
|---|---|---|---|---|
| **Torch** | встроенное 100% (+доливка Tar 1L) | ~14–15 реальных часов; зажигание −1% | рука 6L/6R | крафт: Tree Bough + 6 String + 1L Tar; гаснет в инвентаре; зажигает печи/kiln/смelter |
| **Lantern** | свеча (Tallow/Wax Candle) целиком | **~100 реальных часов** | рука 9L/9R или **Pouch-слот** (2024) | Bar of Any Metal + 2 Glass Panes; гаснет в инвентаре; светит и поджигает; с 2022 — «2D overlay light effect» |
| **Candelabrum** | свеча (замена = новая свеча) | ~48 реальных часов | 0.2×0.2 объект, **liftable**, можно носить как «тяжёлый факел» | радиус света **×2 от torchpost** при той же интенсивности; не уничтожается выгоранием |
| **Candle Crown / Pumpkin Lantern / Miner's Helm** | — | — | голова | полезны в шахтах, где «Ctrl-трюк» факела не работает |

- **Зажигание**: firebrand из костра / спички / огниво; значения поджига: candelabrum +6, pumpkin lantern +10, torch +20.
- Носимый источник в руке можно нести при ходьбе (Ctrl+клик перемещает предмет на курсоре); в инвентаре факел/фонарь **гаснет**.
- Лампы на Lantern Post/Hanger/Stand — стационарный свет во дворах/на дорогах.
- Абсолютных радиусов вики не даёт: затухание живёт в серверных resource-слоях «light» (квадратичные константы ac/al/aq); задокументированы только относительные (candelabrum = 2× torchpost).

### 1.5 Клиент H&H (hafen-client)

- Протокол glob-«astro» (Glob.java): `dt` — доля суток [0..1) (позиции солнца/луны), `mp` — фаза луны [0..1), `yt` — не используется, `night` — bool, `mc` — цвет луны (тинт), `is` — сезон 0..3 (фон календаря), `sp/sd/years/ym/md` — добавлены позже (amber-client их ещё не читает). Игровое время — отдельное «tm»; освещение — «light»: `lightamb/lightdif/lightspc` + `lightang/lightelev` (направление солнца/луны), клиент интерполирует ~2 c.
- Темнота — **не оверлей, а сценическое освещение**: DirLight (amb/dif/spc + направление) в Phong-шейдер (MapView.java). Ночью сервер просто шлёт тёмные цвета. Источники света объектов — resource-слой «light»: цвета + квадратичное затухание ac/al/aq, конус/направление (Light.java, PosLight/SpotLight).
- **Zoned lighting** (jorb, «Lights in the Dark», 2022-10-30): сетка зон, до 16 источников на зону (слайдер до 32) против «3 источника + солнце/луна» старого глобального режима. Луна в 3D-мире **не рисуется** — только в календаре: «moon» встречается ровно в одном файле (Cal.java).
- «Light overlays» (свечение/искры; рождественский патч 2019, расширен 2020-11-16 «Twinkle-Twinkle») — декоративные оверлеи, отдельно от освещения.
- Night Vision — только кастомные клиенты (Amber: «Daylight mode», Ctrl+N); в ванильном клиенте отсутствует.

## Часть 2. Порт в origin_go

### 2.1 Что уже есть (обследование кода)

| Нужно | Готовый код |
|---|---|
| Ход времени | `TimeState{Tick, TickRate=10, RuntimeSecondsTotal, UnixMs}` (`resources_time_movement.go`), SERVER_TICK_TOTAL персистится в `global_var` — фаза суток/луны считается детерминированно, без дрейфа |
| Синхронизация времени клиенту | `S2C_PlayerEnterWorld{tick_rate, stream_epoch,...}` — добавляем поля времени (аддитивно); троттл-пуш образец — `PlayerStatsPushSystem` (prio 490) |
| Клиентская оценка времени | `web_new/src/network/TimeSync.ts` — EWMA-оценка `estimateServerNowMs()`: интерполяция фаз суток/луны между серверными ресинками |
| «Горит/не горит» уже доезжает до клиента | станция: `campfire` (defId 16) `station{states:[unlit,burning]}` + burner behavior; клиент получает `resource_path="campfire/<state>"` в `S2C_ObjectSpawn`, при смене сервер пересылает спавн — клиент переключает спрайт **и уже запускает fx-пресет дыма** (`structures.json: campfire.burning.fx`) |
| Метаданные отображения | клиентский `objects/*.json` (merge в `index.ts`): слои, offset, shadow, z, fx — сюда ляжет `light:{...}` per-state |
| Рендер-слои | `Render.ts`: `mapContainer` / `objectsContainer` (z-sort, TERRAIN_BASE_Z_INDEX=100) / `uiContainer`; cullingController; ObjectManager перечисляет видимые объекты — источник позиций эмиттеров |
| Стек | **pixi.js 8.21.0**, pixi-filters не подключён, RenderTexture/BlendMode сегодня не используются — оверлей пишем с нуля (лёгкий, но новый код) |
| Топливо/выгорание | burner behavior (`fuelAbilities:["fuel"]`, ticksPerFuel) — свеча/фонарь портируются как burner-предметы |
| Арт | sky/celestial-кадров в атласе **нет** (tiles.json, 1899 кадров — ноль moon/sun/star/sky); есть только `items/torch.png` (ItemDef тоже нет) — арт луны/солнца/неба новая задача |

### 2.2 Дизайн (Ф1 MVP — визуальный свет)

1. **Сервер**: `config game.day_length_seconds` (H&H-верно ≈ 26 280 с; для тестов — меньше), фаза суток = f(SERVER_TICK_TOTAL) — детерминированно на всех шардах. В `S2C_PlayerEnterWorld` добавить `day_length_ticks` + `day_start_tick` (или отдельный `S2C_WorldTime` тег 49 с периодическим ресинком по образцу stats-push; тег 49 также метит [village_claim.md](village_claim.md) — согласовать порядок).
2. **Клиент — оверлей**: новый слой поверх `objectsContainer` (под UI): полноэкранный тёмный спрайт, альфа = кривая суток (`darknessCurve`: день 0 → сумерки → ночь max, с плавным dusk/dawn); «дыры света» — pre-rendered радиальные градиент-текстуры с `blendMode 'erase'`/RenderTexture-композит (техника фиксируется при имплементации, Pixi v8 API).
3. **Эмиттеры**: `light:{radius_px, color, intensity, flicker}` в клиентском display-json **per-state** (`campfire.burning.light`) — источник света активен ровно тогда, когда активен соответствующий display-state (тот же сигнал, что ведёт спрайт/дым). Позиции/радиусы собираются из ObjectManager'а каждый кадр (кэшировать список видимых эмиттеров).
4. **Мерцание** огня — лёгкий шум амплитуды радиуса/альфы (как H&H-мерцание), фонари — без.

### 2.3 Фазы

1. **Ф1**: время суток (сервер+синк) + ночной оверлей + свет от горящих станций (campfire первец) + кривая/цвет в конфиге. Чисто визуально.
2. **Ф2**: переносные источники — факел/фонарь как предметы с burner-топливом, экипировка в руку/pouch, **свет, привязанный к игроку**; малый личный «ambient glow» ночью (см. решения); Lantern Post/Hanger.
3. **Ф3**: лунный цикл + календарный виджет (см. 2.4), затем геймплейные сцепки (дрейф нод, волки).

### 2.4 Лунный цикл и календарь (детали Ф3)

Один счётчик SERVER_TICK_TOTAL крутит всё: сутки, луну, дату, сезон.

- Математика (чистая функция тика — детерминизм на всех шардах и после рестарта без ресинка):
  - `game_day = floor(tick / day_length_ticks)`; `dt = frac(game_day)` — доля суток;
  - `mp = frac(game_day / 30)` — фаза луны (месяц = 30 дней = лунный цикл; mp=0 = центр новолуния = 1-й день месяца, полнолуние на 15-й день);
  - кадр луны = `round(mp*8) % 8` (формула Cal.java);
  - дата: `md = game_day % 30`, `ym = floor(game_day/30) % 6`, `year = floor(game_day/180)`; сезон по дню года: <30 весна, <135 лето, <165 осень, иначе зима.
- Протокол: аддитивно в `S2C_PlayerEnterWorld` — `day_length_ticks`, `lunar_cycle_days` (30); далее клиент считает сам. Плюс лёгкий периодический ресинк `S2C_WorldTime` (тег 49, свободен) — текущий тик + mp, раз в игровые минуты (не каждый тик), защита от дрейфа после лагов/сна клиента.
- Ночная кривая × луна: `darknessCurve` даёт базу, луна модулирует ночную ветвь — таблица 8 фаз → (амплитуда ночного затемнения, цвет тинта `mc`) в конфиге: новолуние почти чёрная ночь, полнолуние — светлая серо-синяя.
- Календарный виджет (по образцу Cal.java): контейнер в UI-слое; фон день/ночь × сезон; орбита — солнце и луна напротив друг друга по `dt`; луна — 8 кадров + тинт mc; тултип «N-й день N-го месяца N-го года». Требует арт (луна ×8, солнце, фоны 2×4); до появления арта — цифровой HUD/тултип.
- Геймплейные сцепки — отдельные изменения после визуала: дрейф quality-нод на новолуние → система нод ([spot.md](spot.md), ещё не портирована; направление/дистанция дрейфа случайны и независимы по типам ресурсов, животные дрейфуют быстрее трав); волки в лесу на полнолуние → [animals.md](animals.md); рыба по фазе/времени суток → когда появится рыбалка.

### 2.5 Открытые решения

1. Длина суток: H&H-верно 7ч18м vs короче для играбельности (конфиг, но дефолт?).
2. Личный ambient-glow игрока ночью: H&H ≈ 0 (почти слеп), для origin_go скорее нужен малый радиус 1–2 тайла — иначе неиграбельно без фонарей.
3. Лунный цикл в Ф1 или Ф3 (новолуние = почти чёрная ночь — сурово); дефолтная длина цикла: H&H-верно 30 игровых дней (≈9д 3ч реального времени) vs короче.
4. Техника оверлея: RenderTexture+erase vs фильтр vs затёмнение вершинных цветов тайл-меша.
5. Влияет ли темнота на геймплей (дистанция интеракций/спавны) — в H&H в основном визуал.
6. Дрейф нод: Ф3 = только визуал+календарь, или сразу геймплейная часть (зависит от порта системы нод, spot.md).

### Тесты

- Go: детерминизм фазы суток (границы dusk/dawn, рестарт без дрейфа по SERVER_TICK_TOTAL), поля enter-world; луна: mp на границах фаз (0, 1/8, 0.5…), дата/сезон из game_day (месяц=30, год=180, сезоны 30/105/30/15), ресинк S2C_WorldTime.
- Web: unit — кривая darkness (таблица время→альфа) × луна (mp→амплитуда/тинт), кадр луны из mp (round-формула), сборка списка эмиттеров из стора; harness-страница (образец `tests/hybrid-integration.ts`) — оверлей+свет+календарь скриншотом.

## Источники

- Ring of Brodgar wiki: Glossary, Seasons, Category:Light Sources, Torch, Lantern, Candelabrum, Wolf, Primitive Casting-Rod — ringofbrodgar.com/wiki/…
- jorb, «Prelude: World 16» (2024-10-28) — forum t=76519 (дрейф quality-нод на новолуние); замеры первого дрейфа — t=76677
- jorb, «Lights in the Dark» (2022-10-30) — t=73871 (zoned lighting, parchment lanterns, lantern post); «Christmas Antwolf» (2018-12-14) — t=62617 (волки, blue moon)
- loftar: t=74504 (сутки = 7ч 17м 41с), t=61963 (масштаб ×3.29); «Twinkle-Twinkle» (2020-11-16) — p=868558 (light overlays)
- dolda2000/hafen-client: src/haven/{Glob,Astronomy,Cal,Light,PosLight,MapView,render/Lighting}.java; nephtyws/amber-client (старый «astro» — 5 полей)
