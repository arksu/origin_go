# Village Claim (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-27 по Ring of Brodgar wiki, официальному форуму, Fandom и Steam-обсуждениям; порт размечен по актуальному коду (отчёт explore). Смежный документ: [personal_claim.md](personal_claim.md).

## Часть 1. Механика в H&H

### 1.1 Основание деревни

- Меню **Government → Form Village** строит **Village Idol**: 200 Stone, 20 Block of Wood, 20 Board, 40 Bone Material, 10 «A Beautiful Dream!», 1 Bar of Any Metal + **30 000 LP, делящихся между 1–5 основателями** (каждый должен иметь Yeomanry; основание требует Lawspeaking).
- Размещение: на **замощённой площадке 2×3**, с буферами от чужих клаймов/баннеров/статуй; после установки — cooldown **3 игровых дня (~21–22 реальных часа)** до активации. Превью квадрата клайма видно при отдалении камеры на 50 тайлов.
- Активация — 1–5 персонажами; **первый активировавший становится Lawspeaker** (скилл Lawspeaking: 20 000 LP, пререквизиты Hearth Magic + Yeomanry).

### 1.2 Территория

- Idol клаймит **101×101 тайл**. Буфер **100 тайлов** между деревнями.
- **Village Banner** (Cloth×20, Block of Wood×5, Stone×5; только членам с правом Manage territory, на замощённой земле внутри клайма): добавляет **61×61 (квадрат радиуса 30)** вокруг баннера, в том числе **вниз в шахты** (зона строго под деревенской территорией). Следующий баннер в новой зоне — через ~24 реальных часа.
- **Statue of the Chieftain** (Bar of Any Metal×5, Stone×75) территорию **не** расширяет — только кап authority.
- Баннеры мёртвых деревень блокируют личные клаймы на своём тайле (эксклюзия 7 тайлов).
- Клайм действует и на **уровни шахт** — идол/баннер ставится на верхнем нужном уровне.

### 1.3 Authority — «HP деревни»

| Параметр | Значение |
|---|---|
| Старт | 125 000 |
| Начальный кап | 250 000 |
| Дренаж идола | 5 000 / игровой день (игровой день ≈ 7ч 18м реала; ≈200 / 20 минут) |
| Banner | кап +30 000, дренаж +250…1 500 / игровой день (страницы вики расходятся) |
| Statue of the Chieftain | кап +36 000, дренаж +350…500 / игровой день |
| Natural Wonder под территорией | −10 000 / реальный день (по странице Village Claim) |

- Пополнение — LP, зарабатываемые жителями: формула 2010 (jorb): `authority = baseLP × √(CHA×INT) / 30`; обновление 2021-04-30: `authority = LP × √(CHA×LORE) / 10` (LP — базовый, при 100% обучаемости).
- **Порог 50k**: ниже — деревенский клайм перестаёт работать: чужие взаимодействуют с объектами и крадут свободно и **без следов**, идол можно разрушать. Takeover идола: при >50k нужен скилл Trespassing + включённый criminal acts (следов не остаётся), при <50k — любой right-click.
- Ревок личного клайма Lawspeaker'ом стоит **~250 authority за тайл**.
- Выход члена деревни стоил 25 000 authority — **убрано 2025-01-31**.

### 1.4 Права и членство

- Группы = **цвета** (Village-таб окна Kin & Kith). Права per-color: взаимодействие, **Manage territory** (баннеры/статуи/charter stone), **Grant privileges** (раздавать права). **Белый = все члены без цвета**; цветовые права работают только для членов деревни.
- Права чужим (неревенцам) — только через **Field Cairn**: подзона ≤30×30 внутри деревенского клайма со своими правами per-color, где белый = все неревенцы.
- **Членство**: Lawspeaker даёт «Oath of Allegiance» (приглашение); выход добровольный; лимита членов нет.
- **Гостевые ворота** (структура с красными флагами): нечлен, прошедший через них, получает **Visitor buff** — не может совершать criminal acts внутри, даже с включённым тогглом (но лут KO-тел не блокирует). Зона буфа = клайм; сошёл с клайма — буф снят.
- Не-члены на территории: любые взаимодействия = criminal acts (тоггл + скиллы Theft/Vandalism/Trespassing), оставляют **Scent**-следы до очага; члены с правами — без следов.

### 1.5 Lawspeaker

- Практически несменяем: уходит только со смертью или добровольным ренонсом; основание второй деревни снимает роль с первой.
- Умения: **«Revoke the Privilege!»** — снести личный клайм внутри деревни (стоя на нём, платя authority), **«Oath of Allegiance»** — приглашение.
- Takeover пустой (без членов) деревни: right-click идола на расстоянии видимости.

### 1.6 Прочая деревенская инфраструктура

- **Charter Stone** (Rock Crystal, 5× «A Beautiful Dream!», 100 Stone, 10 Bar of metal; Stone Working): телепорт-назначение из Thingwall по секретному имени (гейт Will × расстояние), точка спавна новичков (с visitor debuff), одна на деревню.
- Realms-слой (Lawspeaking открывает): Border Cairn, Coronation Stone, Fealty Stone, Grotesque Idol, Menhir, War Flag, Bonfire.
- В клиенте деревенский клайм рисуется зелёной границей (в мире/миникарте), деревня имеет свой чат-канал.

### 1.7 Защита

- Блок взаимодействий нечленам (легальный обход — criminal acts, см. personal_claim.md), подавление decay, иммунитет палисад к ручному разрушению, осада через Battering Ram ×Power Level, W15: осадка только в провинциях со сломанным Thing Peace.

## Часть 2. Порт в origin_go

### 2.1 Готовые субстраты (что уже есть в коде)

| Механика H&H | Готовый код origin_go |
|---|---|
| LP-экономика (30k на основание, пополнение authority) | `character.exp` JSONB `{lp,nature,industry,combat}`; `DiscoveryLP` в `internal/game/inventory/give_item.go:211`; teach-экшен |
| Yeomanry/Lawspeaking гейты | `CharacterProfile.Skills` (string set) + проверка в `internal/game/action_requirements.go`; strict actiondefs |
| CHA/INT формула | `internal/characterattrs/attributes.go` — атрибуты есть (без инкрементов — не блокирует формулу) |
| Идол/баннер как строения с поведением | `data/objects/*.jsonc`, `ObjectDef.Behaviors`, behavior registry (`internal/game/behaviors/registry.go`), валидация footprint в `build_service.go`/`build_behavior.go` |
| Daily drain + catch-up | `ecs.ScheduleBehaviorTick` + `ScheduledTickBehavior` + persisted next-tick (образцы: `tree_behavior.go:963`, `burner_behavior.go:311`, `burner_exhaustion.go:166`) |
| Чек прав в интеракциях | `ContextActionService.ComputeActions/ValidateAction/ExecuteAction`; `ActionService` (targeted); `build_service.go` — размещение |
| Деревня/членство в БД | sqlc, `migrations/schema.sql` (append + `sqlc generate`); `object.owner_id` зарезервирован и пустует |
| Окно деревни (снапшот) | паттерн `S2C_CraftList`/`S2C_BuildState` full-snapshot; свободные теги: **S2C 49, C2S 28** |
| Границы клайма в клиенте | оверлеи-прецеденты `BuildGhostController/View`, `MoveMarkerManager` (в `objectsContainer`); tile=12 units |
| Village chat | `ChatChannel` enum в proto + `findChatRecipients` (сейчас LOCAL радиусный) |
| Идентичность персонажа | character DB id == EntityID — членство просто по character_id |

### 2.2 Модель данных (черновик)

```sql
CREATE TABLE IF NOT EXISTS village (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  idol_object_id BIGINT NOT NULL,
  founder_id BIGINT NOT NULL,
  authority DOUBLE PRECISION NOT NULL DEFAULT 125000,
  authority_cap DOUBLE PRECISION NOT NULL DEFAULT 250000,
  next_drain_tick BIGINT NOT NULL DEFAULT 0,
  permissions JSONB NOT NULL DEFAULT '{}' -- {color_group: {interact,manage,grant}}
);
CREATE TABLE IF NOT EXISTS village_member (
  village_id BIGINT NOT NULL REFERENCES village(id),
  character_id BIGINT NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',      -- 'lawspeaker' | 'member'
  color_group TEXT NOT NULL DEFAULT 'white',
  joined_tick BIGINT NOT NULL,
  PRIMARY KEY (village_id, character_id)
);
CREATE TABLE IF NOT EXISTS village_area (   -- idol 101x101 + banner 61x61
  village_id BIGINT NOT NULL REFERENCES village(id),
  x0 INT, y0 INT, x1 INT, y1 INT,           -- tile coords, inclusive
  source TEXT NOT NULL,                     -- 'idol' | 'banner' | 'cairn'
  created_tick BIGINT NOT NULL,
  PRIMARY KEY (village_id, x0, y0)
);
```

Идол — обычный объект с behavior `village_idol` (village_id в `ObjectStateEnvelope.behaviors`), `owner_id` = основатель. In-memory: `ClaimIndex` per shard — прямоугольники в срезе, точечный запрос линейным сканом (деревень мало; кэш не нужен, KISS).

### 2.3 Ядро — чек прав

`internal/game/claimcheck.go` (или `internal/village`): `VerdictAt(characterID, tileX, tileY) (verdict, *Village)` со статусами `none | own_village | foreign_protected | foreign_unprotected` (последнее — authority < 50k). Точки встраивания:

1. `ContextActionService.ComputeActions` — не выдавать контекст-экшены по объектам на чужой защищённой территории;
2. `ValidateAction`/`ExecuteAction` — защита от гонки (территория изменилась после выдачи меню);
3. `ActionService` targeted-экшены (прецедент гейта: `plow_tile_action.go`);
4. `build_service.go` — запрет стройки на чужой территории + валидация буферов (100 тайлов между деревнями);
5. v2 (опционально): вход на территорию (trespassing) — потребует тоггл criminal acts, в v1 вход свободен.

v1-правило KISS: member → всё; не-член → всё запрещено на защищённой территории (без криминального пути — он отдельная фича).

### 2.4 Authority upkeep

- Behavior tick на идоле: `next_drain_tick` персистится в envelope; игровой день = фикс (H&H: 8 реальных часов; при tickRate 10 → 288 000 тиков; конфиг). Догонка арифметикой при рестарте — паттерн burner catch-up (лимит `game.behavior_tick_catchup_limit_ticks`).
- Drain: 5000/день + баннеры/статуи; при authority < 5000·N — структуры «погасают» (теряют эффект, разрушаемы), при < 50k — клайм unprotected.
- Пополнение: хук на выдачу LP (`give_item.go` DiscoveryLP сейчас; позже — study/eat-FEP система) → событие в village service → `+LP × √(CHA×INT)/30`.
- Баннер: добавляет rect 61×61 (с футпринтом на замощённой земле, буфер 24 ч между баннерами), кап +30k, drain; статуя: кап +36k, drain.

### 2.5 Прото и клиент

- `S2C_VillageState` (снапшот: name, members{char_id,role,color}, authority/cap/drain, rects) по `C2S_OpenWindow('village')` — копия паттерна CraftList;
- `S2C_ClaimsInAOI` (прямоугольники с типами) — пуш при входе зоны/изменении через visibility dispatcher (аналог chunk stream);
- клиент: `VillageWindow.vue` + слайс в `gameStore.ts`; `VillageBordersOverlay` — зелёные прямоугольники по образцу BuildGhostView; точка клика уже шлётся как есть.

### 2.6 Фазы

1. **Ф1 MVP**: таблицы + village service + чек прав (member/non-member) + idol (крафт/build с LP-гейтом 30 000, 1–5 основателей, Lawspeaker = первый) + территория 101×101 + буфер 100 + оверлей + окно деревни + invite/leave.
2. **Ф2**: Authority (drain/refill/порог 50k/«погасание»), Banner + Statue (территория/кап), village chat channel.
3. **Ф3** (после личных клаймов): Field Cairn, гостевые ворота + Visitor buff, Revoke the Privilege, criminal acts/scents, осада/Power Level, Charter Stone.

### 2.7 Открытые решения

1. Вход на территорию: v1 свободный vs H&H-блокировка (нужен тоггл).
2. Числа H&H (101×101, буфер 100, 30k LP, дренаж) — брать как есть или scale-down под меньший онлайн.
3. Формула пополнения: `/30` (2010, подтверждена) vs `/10` (2021) — масштаб.
4. Пермадет: удалять/оставлять членство при постоянной смерти персонажа.
5. Криминальный путь (Theft/Vandalism/следы): отдельно после личных клаймов.

### Тесты (по конвенциям репо)

- Go in-package: village service (основание/членство/права), claimcheck (границы rect, unprotected-порог), upkeep catch-up (образец burner-тестов), build placement reject на чужой территории.
- Web: packet-shaping тест (S2C_VillageState → store), harness-страница для оверлея по образцу `tests/hybrid-integration.ts`.

## Источники

- [Village Claim — Ring of Brodgar](https://ringofbrodgar.com/wiki/Village_Claim), [Authority](https://ringofbrodgar.com/wiki/Authority), [Village Banner](https://ringofbrodgar.com/wiki/Village_Banner), [Statue of the Chieftain](https://ringofbrodgar.com/wiki/Statue_of_the_Chieftain), [Field Cairn](https://ringofbrodgar.com/wiki/Field_Cairn), [Charter Stone](https://ringofbrodgar.com/wiki/Charter_Stone), [Lawspeaking](https://ringofbrodgar.com/wiki/Lawspeaking), [Claim](https://ringofbrodgar.com/wiki/Claim), [Criminal Acts](https://ringofbrodgar.com/wiki/Criminal_Acts)
- Форум: [Village claim in mines](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=80936), [Prelude: World 16.2](https://www.havenandhearth.com/forum/viewtopic.php?t=80930), [Shared Personal Claims?](https://www.havenandhearth.com/forum/viewtopic.php?f=42&t=46886)
- [Village Claim — Fandom](https://havenandhearth.fandom.com/wiki/Village_Claim), [Steam-обсуждения 2024–2025](https://steamcommunity.com/app/3051280/discussions/0/4638240774905925047)
