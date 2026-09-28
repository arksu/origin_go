# Скот: корова, бык, телёнок (Haven & Hearth) — механика и план порта в origin_go

Референс собран 2026-09-28 по Ring of Brodgar wiki (Cow, Aurochs, Milk, Cheese, Cheesemaking) + уже собранные общие исследования: приручение/кормушки/персистенс — [sheep.md](sheep.md) §1.2–1.3, §1.7 (идентичны для всего скота), скилл Animal Husbandry — [sheep.md](sheep.md) §1.8, спавн/качество — [spot.md](spot.md), боевой ИИ — [combat_system.md](combat_system.md). Порт размечен по актуальному коду.

## Часть 1. Механика в H&H

### 1.1 Скот и тур — статы

| Параметр | Cow / Bull (домашние) | Aurochs (дикий предок) |
|---|---|---|
| Размер (obj) | 0.6 × 1.7 тайла | 0.8 × 2.2 тайла |
| Base-Quality | не опубликован | **30** |
| Hitpoints | не опубликован | **350** |
| Fleeing-Hitpoints | — | **250** (дерётся до ~71%) |
| Armor | — | **15** (с w14; «виртуально обязывает к оружию») |
| Агрессия | нет | пассивен до атаки; **стадо агрится всей массой** (w14) |

- **Среда тура**: Grass, Heath, Moor, Ox Pasture, Pine Barren (+ редко Deep Tangle, Timber Land, Wald), круглый год. Боевой набор: Low Horn Swipe, Mule Kick, Bucking, Thunder Over, Sudden Charge, Roar of the Wild.
- **Особенность стада**: атака одного тура агрит всё стадо; исключение — агр от «дёрганья за волос» (Pull Hair) стадо не трогает.
- **Качество трупа** капится Survival и оружием (общее правило [animals.md](animals.md) §1.3).

### 1.2 Приручение

Полная процедура — [sheep.md](sheep.md) §1.2 (Clover → Rope → Hitching Post → прогресс 15→100 по +30, окно боевой стойки 12–46 ч, «воспитание» = 70% урона + Peace, ~3–4 дня, **загон не нужен**). Отличия для тура:

- Результат — **Cow или Bull, пол случайный** (как у овцы).
- Тур агрессивнее и бронирован — фаза «воспитания» требует оружия (кулаками H&H-вики уже не обещает), но убийство = провал.
- «Дикое доение» (аналог стрижки муфлона): скормить **Clover** → доить **Bucket'ом один раз — 2L молока**; Q клевера влияет на Q молока. Второй клевер тур не примет (как муфлон).

### 1.3 Доение

- **Действие**: ПКМ **Bucket / Barrel / Cistern** по доильному животному. Скилл Animal Husbandry.
- **Скорость**: **Milk Quantity × 0.01 л / 10 мин реального времени** — 0.1 л/10 мин при MQ 10 (≈ 14.4 л/сутки), 0.4 л при MQ 40, 2.0 л при MQ 200.
- **Кэп накопления**: **корова 25L** (овца 25L, коза 15–25L, тур 2L разово).
- **Качество**: Q молока = Q коровы, модифицированная статом **Milk Quality**.
- **Смешивание** разных молок даёт нейтральное «Milk».
- **Питание**: Q10 молоко — 10% выносливости / −20% энергии за глоток 0.05L; выше Q — меньше слив энергии (Energy Drain = (1 − Q/10 + 1) × 10); молоко даёт энергию при Q > 20.

### 1.4 Разведение и наследование

- **Bull** осеменяет корову (анимация спаривания). **Беременность 4.5 дня → телёнок**, **редкие двойни**; после родов корова начинает давать молоко. **Телёнок растёт 10 дней** (пол виден в info-окне).
- **Формула наследования** (RoB Cow, точнее форумной общей): стат потомства = **стат родителя + ролл от −5 до +20**, затем **софткап качеством самца (breeding quality) и качеством съеденного Swill** (корма из кормушки).
- **Скрытые статы продуктов**: Meat Quantity (выход мяса), Meat Quality, Milk Quantity/Quality, Hide Quality — растут кормлением и наследуются.
- **Кастрация**: низкокачественного быка можно кастрировать — прекращает разведение, даёт **Bollock** (у овец Bollocks выходит при butcher барана — Fine Butchery).
- Практика: держать быков отдельно от стада (выбор производителя).

### 1.5 Разделка

| Действие | Продукт | Кол-во | Примечание |
|---|---|---|---|
| Skin | Fresh Cow Hide | 1 | Q модифицируется статом Hide Quality |
| Butcher | **Raw Beef** | **10** при Meat Quantity 10 | Q = Q коровы × Meat Quality |
| Butcher | Animal Fat | 1 | |
| Butcher | Intestines | 2 | |
| Butcher | Entrails | 3 | |
| Butcher | Tiny Brain | 1 | |
| Collect Bones (скелет) | Bone Material | 6 | |
| Clean (только телёнок) | Suckling's Maw | 1 | Animal Husbandry |

- **Тур**: Fresh Aurochs Hide ×2, Intestines ×3, Entrails ×3, Small Brain (Fine Butchery), Animal Fat ×1, **Raw Wild Beef ×14**, Bone Material ×6. Отдельное действие **Pull Hair** на живом туре даёт **Aurochs Hair** (любопытство для изучения) или провоцирует атаку (стадо при этом не агрится).
- Нюанс вики: у коровы Intestines/Entrails отнесены к butcher, у лисы/кролика — к Clean; в нашей схеме [animals.md](animals.md) Часть 3 (Skin → Clean → Butcher → Bones) расхождение некритично.

### 1.6 Молочная переработка

- **Butter** — крафт из молока (Churn).
- **Cheesemaking** (скилл, W14, 2022-08-12): **10000 LP**, требует Animal Husbandry + Farming; открывает **Cheese Rack, Curding Tub, Rennet**.
- **Творог**: Curding Tub — **1.0L молока + 0.02L Rennet → 1 Cheese Curd каждые 36 мин** (не смешивать молока!). Творог: 1 STR FEP или закладка в **Cheese Tray** (4 творога).
- **Созревание**: Tray на **Cheese Rack**; сорт определяется **локацией стойла** — Cabin / Cellar / Outside / Mine («считается только самое внутреннее»); непоказанные последовательности → **Generic Gouda (9 дней = 65 ч 39 м)**. Молоко дикого тура даёт только Gouda.
- **Качество**: на каждой стадии софткапится **Cheese Tray**; качество стойла не важно (jorb, Raw Trucker update). Полная карта коровьих сортов — картинкой на вики (текстом не извлекается).
- Редкость: The Perfect Hole при нарезке.

### 1.7 Использование в хозяйстве

- **Молочная ферма** — главный доход: 0.1L/10 мин при MQ10 (полный кэп за ~42 ч), сыры как долгие timed-процессы с локационной специализацией (прецедент [herbalist_table.md](herbalist_table.md)).
- **Bull тянет Wagon** (медленнее лошади); **Packrack** на взрослом скоте = переносное хранилище.
- Шляпы на скоте (2022-11-04), Herder credo-квесты — косметика/флейвор.

## Часть 2. Порт в origin_go

### 2.1 Субстраты (что уже есть в коде)

- **Клиент: спрайт тура готов** (animals.json `aurochs`, 8 направлений + web_new/public/assets/game/animals/aurochs/) — **домашней коровы/быка в дефах НЕТ** (всего 6 животных). Варианты: достать cow/bull арт из тех же H&H-ассетов, что и остальные 6 (прецедент — animals.md §2.1), или временно переиспользовать aurochs с масштабом. Открытое решение §2.6.1.
- **Мясная ветка готова**: `beef`/`roasted_beef` (data/items/food.jsonc:32, 265). **Молока, ведра, творога/сыра нет** — новые предметы.
- Общая база скота — [sheep.md](sheep.md) §2.1: behaviors registry, GiveItem(quality), cyclic build-паттерн, HP-пулы, труп-прецедент player_death, бинарные скиллы; **боя нет**, контейнеры есть (container behavior), объёмов нет.

### 2.2 Зависимости

1. **sheep.md F1–F2** (AnimalAISystem + таминг-конвейер) — корова садится на тот же конвейер без изменений процедуры.
2. **Минимальный combat** — тур бронирован и контратакует; стада агрятся группой (опция F1).
3. **Молоко требует объёмы** — главная коровья специфика порта (§2.3).

### 2.3 Модель данных

`LivestockState` из [sheep.md](sheep.md) §2.3 переиспользуется + коровьи статы продуктов. Обобщение: скрытые статы — **пары (Quantity, Quality) на продукт**:

```go
// общее для скота (sheep.md §2.3: Sex/Tamed/TamingProgress/TetheredTo/
// LeashedBy/Satiation/FoodEatenQ/Wool*/Milk*/Pregnant/IsLamb/...)
ProductStats struct {  // вместо отдельных полей
    Quantity uint8  // выход (Meat 10, Milk 5 дефолты)
    Quality  uint8
}
Meat, Milk, Hide ProductStats
BreedingQuality uint8 // самец; софткап потомства
Castrated bool
MilkStoredL uint8 // литры: 1 предмет = 1L (§2.6.2)
```

### 2.4 Фазы порта

- **F0 «Предметы»**: `milk` (1 шт = 1L, Q), `bucket` (контейнер-предмет), `rennet`, `cheese_curd`, `animal_fat`, `butter`, `fresh_cow_hide`, `intestines`, `entrails`, `bone_material` (общий с animals.md §3.5), `aurochs_hair`. Крафты: молоко→butter (F5), жарка beef уже есть.
- **F1 «Тур-сущность»** (F1 animals.md): objectdef `aurochs` (0.8×2.2, HP350, Q30, Armor 15 — как свойство цели для будущего combat); ИИ Idle/Wander + neutral-chase; **стадный аггро-шеринг** — опция (спавн группой по [spot.md](spot.md) уже планируется); Pull Hair — флейвор-экшен без аггро (решение §2.6.4).
- **F2 «Таминг»** — без изменений к sheep.md F2 (общий конвейер, случайный пол Cow/Bull). «Дикое доение» тура: клевер + bucket → 2L разово (симметрия со стрижкой муфлона).
- **F3 «Доение»**: behavior-тик копит MilkStoredL (MilkQuantity × 0.01 л / 10 мин, кэп 25 → 25 предметов-литров, catch-up офлайна); экшен **milk** с ведром в руке (или ведро→объект); Q молока = софткап (Q коровы, Milk Quality stat — при скилл-левелах).
- **F4 «Разведение»** — sheep.md F4 + коровьи числа (4.5 дн, двойни — редкий ролл, телёнок 10 дн) + **кастрация** (экшен на быке: Castrated=true, дроп Bollock, гейт Animal Husbandry) + формула наследования §1.4 (родитель + [−5..+20], софткап самцом и кормом) — **эта формула общая для скота, обновляет и овец**.
- **F5 «Молочная переработка»** (после крафт-стадий): butter; творог (Curding Tub: 1 молоко + rennet → curd / 36 мин — pattern timed-процессов как herbalist table); Cheese Tray (4 curd) → Cheese Rack со сменой сорта по **типу тайла/помещения** (наш аналог локаций H&H: под открытым небом / в доме / в подвале / в шахте) → Generic Gouda как дефолт. Полная карта сортов — после арта/баланса.
- **Wagon/Packrack** — вне скоупа до транспорта (см. [boat.md](boat.md) как прецедент ride-механик).

### 2.5 Качество (сводка порта)

```
Молоко:  q := CowQ (модификатор Milk Quality — при скилл-левелах); дикий тур: вмешать Q клевера
Мясо:    Q трупа капится min(Survival, weapon); выход = Meat Quantity (10 → Raw Beef ×10)
Шкура:   Q ← модификатор Hide Quality (при скилл-левелах)
Потомство: stat := parent + roll(−5..+20), софткап (stat + maleBreedingQ)/2 и (stat + FoodQ)/2
Сыр:     софткап качеством Tray на каждой стадии (качество стойла не важно)
```

### 2.6 Открытые решения

1. **Спрайт коровы/быка**: добыть H&H-арт cow (одним конвейером с остальными животными) vs reuse aurochs. Рекомендация: добыть арт — овечья пара mufflon/sheep уже задала прецедент различия дикий/домашний.
2. **Молоко**: 1 предмет = 1L (рекомендация — числа H&H ложатся 1:1: кэп 25 предметов, творог = 1 предмет) vs литровая модель контейнеров.
3. **Двойни**: редкий ролл (аутентично) vs всегда один телёнок (KISS).
4. **Pull Hair / Aurochs Hair**: флейвор-экшен без риска (рекомендация для F1) vs отложить до combat (аггро-исключение стада).
5. **Кастрация**: включать в F4 (аутентичный контроль поголовья) vs отрезать как экзотику.
6. **Стадный аггро тура**: шеринг аггро по стаду (требует combat + список стада) vs индивидуальная агрессия.
7. **Масштаб таймеров**: RL-числа H&H (4.5/10 дн, 36 мин творог, 9 дн Gouda) vs наши игровые сутки ×3.29 (см. [light.md](light.md)) — единое решение со всеми животными фичами.
8. **Cheese Rack локации**: тайл-типы (mine/подвал/дом/улица) — нужны интерьеры/подвалы; F5 после строительства.

### Тесты (по конвенциям репо)

- Доение: скорость от MilkQuantity, кэп 25, catch-up офлайна, доение только коровы (и тур после клевера, разово 2L), требуется ведро, смешивание молок → нейтральное.
- Разведение: 4.5 дн беременность, двойни (seeded ролл), рост телёнка, пол при рождении, наследование §2.5, кастрация блокирует осеменение и даёт Bollock.
- Тампинг: общий конвейер sheep.md — пол Cow/Bull случайно.
- Сыр: 1 молоко + rennet → curd за 36 мин; tray 4 curd; смена сорта по локации; Gouda как дефолт; софткап Tray.
- Регрессия: behaviors/entityhealth зелёные, новые objectdefs/items проходят валидацию загрузчиков.

## Источники

- [Cow — Ring of Brodgar](https://ringofbrodgar.com/wiki/Cow) (статы, доение, 4.5 дн/двойни/10 дн, формула наследования +20/−5, кастрация, дроп-таблица, Packrack/Wagon)
- [Aurochs — Ring of Brodgar](https://ringofbrodgar.com/wiki/Aurochs) (Q30/HP350/Armor15, стада w14, дроп ×14 Wild Beef, Pull Hair, дикое доение 2L)
- [Milk — Ring of Brodgar](https://ringofbrodgar.com/wiki/Milk) (Milk Quantity × 0.01/10 мин, кэпы 25L, ведро/бочка/цистерна, смешивание, FEP питья)
- [Cheese — Ring of Brodgar](https://ringofbrodgar.com/wiki/Cheese) (Curding Tub 1.0L+0.02L→curd/36 мин, Tray 4, Rack-локации, Gouda 9 дн, софткап Tray)
- [Cheesemaking — Ring of Brodgar](https://ringofbrodgar.com/wiki/Cheesemaking) (10000 LP, AH+Farming, W14)
- [Taming](https://ringofbrodgar.com/wiki/Taming), [Animal Husbandry](https://ringofbrodgar.com/wiki/Animal_Husbandry), [Food Trough](https://ringofbrodgar.com/wiki/Food_Trough), [Hitching Post](https://ringofbrodgar.com/wiki/Hitching_Post) — общие для скота, см. [sheep.md](sheep.md)
- [sheep.md](sheep.md) — общий конвейер скота (таминг/кормушка/персистенс/LivestockState); [animals.md](animals.md) — ИИ/разделка; [spot.md](spot.md) — спавн; [herbalist_table.md](herbalist_table.md) — прецедент timed-процессов; [boat.md](boat.md) — прецедент ride/тяги
- Код: web_new/src/game/objects/animals.json (`aurochs`), web_new/public/assets/game/animals/aurochs/, data/items/food.jsonc:32,265 (beef)
