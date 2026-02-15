# PRD: Player Stamina System

## 1. Problem Statement

Нужно внедрить в игру механику выносливости персонажа (`stamina`) как ограничитель движения и длительных действий.

Требования продукта:
1. stamina расходуется при движении и длительных действиях.
2. stamina восстанавливается из `energy`.
3. max stamina определяется атрибутами персонажа.
4. При усталости меняются доступные режимы движения.
5. Для длительных действий действует защита от полного истощения (порог 10%).
6. Состояние stamina хранится в runtime как `float` и персистится в `character.stamina`.
7. Клиент получает `S2C_PlayerStats` при изменении значений.

## 2. Goals

1. Ввести runtime-компонент статов игрока: `EntityStats`.
2. Реализовать формулу максимума stamina от `CON`:
   - `maxStamina = sqrt(CON) * 1000`.
3. Добавить механизм регенерации stamina через трату energy:
   - реген не крутится каждый тик;
   - обновление по `nextRegenTick`.
4. Добавить сетевой пакет `S2C_PlayerStats`:
   - `uint32 stamina`
   - `uint32 energy`
   - отправка округленных значений (`math.Round`).
5. Реализовать ограничения на режимы движения по проценту stamina.
6. Реализовать правило `10% floor` только для длительных действий.
7. Перенести персист stamina с `TODO=100` на реальное runtime значение.

## 3. Non-Goals

1. Персист `energy` в БД (runtime-only на текущей итерации).
2. Локализация текстов алертов.
3. Балансировка финальных коэффициентов расхода/регена (на первом этапе допускаются заглушки формул).
4. Переработка UI beyond обработка нового пакета stats.

## 4. Product Rules

### 4.1 Initial values

1. Стартовый `Energy = 1000`.
2. `Stamina` при создании персонажа:
   - `round(sqrt(CON)*1000)`;
   - для дефолтного `CON=1` это `1000`.
3. При login/reattach:
   - stamina берется из БД;
   - применяется жесткий clamp в диапазон `[0, maxStamina]`.

### 4.2 Movement limitations by stamina

| Stamina % | Ограничение |
|---|---|
| `< 50%` | Нельзя `FastRun` (4th speed) |
| `< 25%` | Нельзя `Run` (3rd speed) |
| `< 10%` | Разрешен только `Crawl` |
| `< 5%` | Движение запрещено |

Дополнительно:
1. При достижении порога, где текущий режим становится недопустим, режим автоматически понижается.
2. При `<5%` активное движение останавливается принудительно в тот же тик.
3. При входящих попытках движения на `<5%` сервер молча отклоняет команду (без алерта).

### 4.3 Long actions floor

1. Порог `10%` применяется только к длительным действиям (рубка, крафт и т.д.).
2. Если очередное списание stamina опустит значение ниже `10% maxStamina`:
   - действие отменяется до списания;
   - отправляется `S2C_MiniAlert` с `reason_code=LOW_STAMINA`, `ttl_ms=2000`.

### 4.4 Regeneration

1. stamina регенерирует только если:
   - `Energy > 0`
   - `Stamina < MaxStamina`
2. Реген выполняется по расписанию (`nextRegenTick`), а не в каждом тике.
3. Интервал регенерации выносится в конфиг (`stamina_regen_interval_ticks`), стартовое значение: `10` тиков.
4. Регенерация тратит energy и увеличивает stamina по формуле (на первом этапе разрешена заглушка).
5. Базовое правило продукта: stamina naturally regenerates `10%` per `10% energy`.

### 4.5 Attribute influence

1. Скорость потери stamina в целом не модифицируется атрибутами.
2. Единственное исключение:
   - `CON` уменьшает rate loss только при `Swim`.

### 4.6 Per-action stamina drain

1. Для длительных действий нужен per-action коэффициент расхода.
2. Для `tree` behavior поле обязательное: `behaviors.tree.chopStaminaCost`.
3. Если для `tree` behavior поле отсутствует — загрузка def завершается fail-fast ошибкой.

## 5. Technical Scope

1. ECS component: `EntityStats` (минимум `Stamina`, `Energy`).
2. ECS/system logic:
   - расход от движения;
   - расход от длительных действий;
   - regen scheduler (`nextRegenTick`);
   - auto-downgrade movement mode;
   - forced stop `<5%`.
3. Persistence:
   - `character.stamina` сохраняется как `round(runtimeStamina)`.
4. Network:
   - `S2C_PlayerStats` + отправка on-change rounded значений.
   - initial packet сразу после `S2C_PlayerEnterWorld`.
5. Def contract:
   - новое поле коэффициента расхода stamina для длительных действий.
   - v1 обязательный контракт для tree: `behaviors.tree.chopStaminaCost`.

## 6. Acceptance Criteria

1. Создание персонажа пишет stamina как `round(sqrt(CON)*1000)`.
2. При login/reattach stamina корректно clamp-ится до `maxStamina`.
3. При stamina `<50/<25/<10/<5` применяются соответствующие ограничения режимов движения.
4. При `<5%` активное движение останавливается в тот же тик.
5. Для длительных действий работает `10%` floor и отправка `LOW_STAMINA`.
6. Пер-action коэффициент расхода берется из def-поля.
   - для tree используется `behaviors.tree.chopStaminaCost` (обязательное поле).
7. Реген stamina не дергается каждый тик; работает через `nextRegenTick` и только при `Energy > 0 && Stamina < MaxStamina`.
   - интервал берется из конфига, default `10` тиков.
8. `S2C_PlayerStats` отправляется:
   - initial после `S2C_PlayerEnterWorld`;
   - затем при изменении rounded stamina/energy.
9. `character.stamina` в save-path больше не хардкодится в `100`.
10. После relogin `energy` снова `1000` (осознанно для снижения scope v1).

## 7. Rollout & Observability

1. Логирование отмен длительных действий по `LOW_STAMINA`.
2. Метрики (минимум):
   - count forced-stop `<5%`
   - count auto-downgrade mode
   - count long-action cancel by stamina floor
   - count stats packets sent.

## 8. Open Questions

На текущую итерацию открытых вопросов нет.
