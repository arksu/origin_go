# ADR 0004: Player Stamina Runtime Model, Regen Scheduler и Stats Replication

- Status: Proposed
- Date: 2026-02-16
- Owners: Game Server

## 1. Context

Нужна stamina-механика с требованиями:
1. Runtime storage в float.
2. Max stamina от attributes (`CON`).
3. Реген через расход `energy`.
4. Нельзя вызывать regen каждый тик (нагрузка).
5. Ограничения movement mode по stamina thresholds.
6. Правило 10% floor для long actions.
7. Персист stamina в `character.stamina`.
8. Отдельная доставка stats клиенту при изменении.

В текущем состоянии:
1. stamina уже есть в DDL/queries.
2. Runtime-компонента stats нет.
3. Save-path записывает `Stamina=100` (TODO), фактически теряя реальное состояние.

## 2. Decision

### 2.1 Runtime entity model

1. Добавляем ECS-компонент `EntityStats` (component ID `27`).
2. Поля v1:
   - `Stamina float64`
   - `Energy float64`
   - `NextRegenTick uint64` (scheduler cursor)
3. Max stamina не хранится в компоненте как source-of-truth, а вычисляется функцией:
   - `maxStamina = sqrt(CON) * 1000`.

### 2.2 Initialization policy

1. `CreateCharacter`:
   - записывает `stamina = round(sqrt(CON)*1000)`.
2. Login/Reattach:
   - `Energy = 1000` (runtime-only)
   - `Stamina` загружается из DB и clamp-ится в `[0, maxStamina]`.
3. Если DB stamina выше max stamina после изменения CON:
   - применяется жесткий clamp вниз без компенсации.

### 2.3 Regeneration scheduling

1. Regen не запускается каждый тик.
2. Regen обрабатывается только когда `nowTick >= NextRegenTick`.
3. После обработки regen система пересчитывает и ставит следующий `NextRegenTick`.
   - интервал берется из конфига, default `10` тиков.
4. Regen вообще не вызывается, если:
   - `Energy <= 0` или
   - `Stamina >= MaxStamina`.
5. Формула regen на первой итерации может быть заглушкой, но API фиксируется:
   - тратит energy,
   - увеличивает stamina,
   - clamp значений.

### 2.4 Movement constraints and fallback

1. Threshold rules:
   - `<50%` запрет `FastRun`
   - `<25%` запрет `Run`
   - `<10%` только `Crawl`
   - `<5%` движение запрещено
2. Если активный режим недопустим по текущему stamina:
   - режим автоматически понижается до ближайшего допустимого.
3. На `<5%`:
   - активное движение останавливается в тот же тик (forced stop).
4. На входящие команды движения при `<5%`:
   - команда отклоняется молча (без алерта).

### 2.5 Stamina drain semantics

1. Movement drain:
   - формула v1 допускает заглушку, но учитывает mode.
2. Long action drain:
   - применяется per-action коэффициент из def-поля.
3. Attribute exception:
   - `CON` влияет на loss rate только в `Swim`.

### 2.6 Long actions safety floor

1. Для long actions действует floor: `10% maxStamina`.
2. Если следующий расход уводит ниже floor:
   - действие отменяется до списания;
   - отправляется `S2C_MiniAlert` с `reason_code=LOW_STAMINA`, `ttl_ms=2000`.

### 2.7 Network replication

1. Добавляется `S2C_PlayerStats` в protobuf:
   - `uint32 stamina`
   - `uint32 energy`
2. На отправке используется `math.Round`.
3. Доставка:
   - один initial packet сразу после `S2C_PlayerEnterWorld`;
   - последующие пакеты только при изменении rounded stamina или rounded energy.

### 2.8 Persistence

1. В `CharacterSaveSystem` stamina берется из `EntityStats` и сохраняется в `character.stamina`.
2. Хардкод `Stamina=100` удаляется.
3. `energy` не персистится на этой итерации.
4. После relogin `energy` снова инициализируется в `1000` (осознанное упрощение scope v1).

### 2.9 Def contract change

1. В schema defs добавляется новое поле per-action stamina drain coefficient.
2. Для tree behavior обязательный ключ: `behaviors.tree.chopStaminaCost`.
3. Отсутствие `behaviors.tree.chopStaminaCost` в tree-def приводит к fail-fast ошибке загрузки defs.
4. Коэффициент читается в runtime для соответствующего long action.

## 3. Rationale

1. Отдельный `EntityStats` сохраняет SRP: attrs/progression отдельно от runtime resources.
2. Regen scheduler через `NextRegenTick` убирает лишнюю тик-нагрузку.
3. Diff-send rounded stats минимизирует сетевой шум.
4. Авто-понижение режима делает поведение детерминированным и UX-предсказуемым.
5. Жесткий clamp при login защищает инварианты после изменения attributes.

## 4. Consequences

### Positive

1. Корректный жизненный цикл stamina: load -> runtime mutate -> persist.
2. Нет постоянного per-tick regen для каждого игрока.
3. Строгие правила движения и long-actions унифицированы сервером.
4. Клиент получает стабильный snapshot/delta stats-канал.

### Trade-offs

1. Добавляется новая системная логика (regen scheduler + movement guard + long-action guard).
2. Появляется зависимость runtime от корректности нового def-поля.
3. Требуется синхронная генерация protobuf для Go и web клиента.

## 5. Rejected Alternatives

1. Regen every tick for all players
   - Отклонено: избыточная нагрузка.
2. Хранить max stamina в БД
   - Отклонено: derived value от attributes, риск рассинхрона.
3. Отправлять stats каждую итерацию тика
   - Отклонено: сетевой спам, не нужен при unchanged rounded values.
4. Применять 10% floor к movement
   - Отклонено: продуктовое правило ограничивает floor только long actions.

## 6. Implementation Notes

1. Добавить `EntityStatsComponentID = 27` и обновить component registry doc.
2. Инициализировать stats в spawn/reattach paths.
3. Ввести helper-функции:
   - `ComputeMaxStamina(attributes)`
   - `RegenerateStamina(...)`
   - `ResolveAllowedMoveModeByStamina(...)`
4. Добавить/обновить систему, которая:
   - применяет regen по `NextRegenTick`,
   - применяет movement drains/guards,
   - инициирует stats replication при rounded-change.
5. В long-action completion path проверить floor до списания и отправлять mini alert.
6. Обновить `character_save` snapshot на runtime stamina.
7. Протянуть protobuf изменения в Go (`protoc`) и web (`pbjs/pbts`).

## 7. Testing Requirements

1. Unit: max stamina formula.
2. Unit: login clamp behavior.
3. Unit: mode restrictions thresholds + auto-downgrade.
4. Unit: forced stop on `<5%`.
5. Unit: long-action floor cancel + `LOW_STAMINA` alert.
6. Unit: regen scheduler (`NextRegenTick`) и no-op когда `Energy<=0`/`Stamina>=Max`.
7. Unit: stats packet send only on rounded diff.
8. Integration: save/load stamina without regression.
