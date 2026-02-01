# План разработки игрового клиента web_new

## Обзор проекта

Новый игровой клиент объединяет:
- **Изометрическую графику** из `web_old` (PixiJS v8, тайлы, объекты, Spine-анимации)
- **Сетевой протокол** из `web` (Protobuf, WebSocket)
- **Современный стек**: Vue 3, TypeScript, Vite, Pinia

---

## ✅ Этап 1: Инициализация проекта

### Цель
Создать базовую структуру проекта с настроенными зависимостями.

### Задачи
1. Инициализировать Vite проект с Vue 3 + TypeScript
2. Установить зависимости:
   - `vue@3`, `vue-router@4`, `pinia`
   - `pixi.js@8`
   - `protobufjs`, `protobufjs-cli`
   - `axios`
   - `sass`
3. Настроить TypeScript (`tsconfig.json`)
4. Настроить Vite (`vite.config.ts`)
5. Создать базовую структуру директорий:
   ```
   src/
   ├── api/           # HTTP API для авторизации
   ├── assets/        # Статические ресурсы
   ├── components/    # Vue компоненты
   ├── config/        # Конфигурация приложения
   ├── game/          # Игровая логика (PixiJS) — ФАСАД для рендера
   ├── network/       # WebSocket и Protobuf
   ├── router/        # Vue Router
   ├── stores/        # Pinia stores
   ├── types/         # TypeScript типы
   ├── utils/         # Утилиты
   └── views/         # Страницы
   ```
6. Создать `src/config/index.ts` — единый модуль конфигурации:
   - `API_BASE_URL` — базовый URL для HTTP API
   - `WS_URL` — URL для WebSocket соединения
   - `DEBUG` — флаг режима отладки
   - `BUILD_INFO` — версия сборки, дата, commit hash
7. Добавить npm скрипт для генерации protobuf типов

### Архитектурные правила

> **ВАЖНО**: Код PixiJS **НЕ импортируется** в Vue-компоненты напрямую.
> Вся работа с рендером — только через фасад `game/`.
> Это предотвращает неуправляемую связь UI ↔ Render.

```
✅ Vue component → game/GameFacade.ts → game/Render.ts → PIXI
❌ Vue component → import * as PIXI from 'pixi.js'
```

### Критерии завершения
- [x] Проект запускается командой `npm run dev`
- [x] Пустая страница открывается в браузере без ошибок
- [x] TypeScript компилируется без ошибок
- [x] Конфигурация читается из `config/index.ts`
- [x] ESLint правило запрещает импорт `pixi.js` вне `game/`

---

## ✅ Этап 2: Авторизация и навигация

### Цель
Реализовать страницы авторизации и выбора персонажа с HTTP API.

### Задачи
1. Создать Pinia store для авторизации (`authStore.ts`):
   - Хранение токена в localStorage
   - Проверка валидности токена (exp claim если JWT)
   - Методы: `login()`, `logout()`, `isAuthenticated`, `isTokenExpired`
2. Реализовать API клиент (`api/client.ts`, `api/auth.ts`, `api/characters.ts`):
   - Axios instance с interceptors
   - Классификация ошибок: `ValidationError`, `AuthError`, `NetworkError`
   - **НЕ делать автоматический logout** при ошибках валидации или сети
3. Создать Vue компоненты:
   - `LoginView.vue` — страница входа
   - `RegisterView.vue` — страница регистрации  
   - `CharactersView.vue` — список персонажей
4. Настроить Vue Router с guards:
   - `/login`, `/register` — публичные
   - `/characters`, `/game` — требуют авторизации
   - **После обновления страницы** → редирект на `/characters` (если токен валиден)
   - Guard корректно обрабатывает:
     - Токена нет → `/login`
     - Токен есть, но протух → `/login` + очистка токена
     - Токен валиден → пропустить
5. Добавить базовые UI компоненты (формы, кнопки, спиннер)

### Обработка ошибок API

| Тип ошибки | HTTP код | Действие |
|------------|----------|----------|
| Валидация | 400, 422 | Показать ошибку в форме, **НЕ logout** |
| Auth | 401 | Logout + редирект на `/login` |
| Forbidden | 403 | Показать сообщение |
| Сеть | 0, timeout | Показать "Нет соединения", **НЕ logout** |
| Сервер | 500+ | Показать "Ошибка сервера" |

### Критерии завершения
- [x] Пользователь может войти/зарегистрироваться
- [x] Отображается список персонажей
- [x] Можно создать нового персонажа
- [x] Можно выбрать персонажа для входа в игру
- [x] F5 на любой странице → корректный редирект
- [x] Ошибка сети не вызывает ложный logout

---

## ✅ Этап 3: Сетевой слой (Protobuf + WebSocket)

### Цель
Реализовать сетевое взаимодействие с игровым сервером.

### Задачи
1. Сгенерировать TypeScript типы из `packets.proto`
2. Создать `network/GameConnection.ts`:
   - Подключение по WebSocket (binaryType: 'arraybuffer')
   - Отправка/приём бинарных сообщений (Protobuf)
   - Ping/Pong для поддержания соединения
   - Версионирование протокола: отправлять `client_version` в `C2S_Auth`
3. **Handshake авторизации** (строгая последовательность):
   ```
   1. HTTP POST /auth/login → получить wsToken
   2. new WebSocket(WS_URL)
   3. ws.onopen → отправить C2S_Auth { token, client_version }
   4. Ждать S2C_AuthResult
   5. Если success=false → disconnect, показать ошибку
   6. Если success=true → состояние 'connected', запустить ping
   ```
   > ⚠️ **Частая ошибка**: считать соединение успешным сразу после `ws.onopen`
4. Создать `network/MessageDispatcher.ts`:
   - Таблица `messageType → handler`
   - Строгая обработка неизвестных типов: `console.warn` + метрика
   - Кольцевой буфер последних N сообщений (для дебага десинхронов)
5. Создать Pinia store для игрового состояния (`gameStore.ts`):
   - Состояние подключения: `disconnected | connecting | authenticating | connected | error`
   - Позиция игрока (POJO: `{ x, y, heading }`)
   - Чанки карты (Map<string, ChunkData>)
   - Игровые объекты (Map<entityId, GameObjectData>)
   - Параметры мира: `coordPerTile`, `chunkSize` (из `S2C_PlayerEnterWorld`)

   > ⚠️ **ВАЖНО**: В store хранить только **чистые данные (POJO)**, не Pixi-объекты!
   > Pixi-объекты создаются отдельным слоем `game/` на основе данных из store.
   > Иначе: утечки памяти, сложности с пересозданием сцены.

6. Реализовать обработчики серверных сообщений:
   - `S2C_AuthResult` → установить состояние connected/error
   - `S2C_PlayerEnterWorld` → сохранить entityId, coordPerTile, chunkSize
   - `S2C_ChunkLoad` / `S2C_ChunkUnload` → обновить Map чанков
   - `S2C_ObjectSpawn` / `S2C_ObjectDespawn` / `S2C_ObjectMove` → обновить Map объектов
   - `S2C_Error` / `S2C_Warning` → показать пользователю

### Структура данных в gameStore

```typescript
interface GameObjectData {
  entityId: number
  objectType: number
  position: { x: number, y: number }
  movement?: {
    targetX: number
    targetY: number
    velocity: { x: number, y: number }
    isMoving: boolean
  }
}

interface ChunkData {
  coord: { x: number, y: number }
  tiles: Uint8Array
  version: number
}
```

### Debug: кольцевой лог сообщений

```typescript
class MessageLog {
  private buffer: Array<{ time: number, type: string, data: any }>
  private maxSize = 100
  
  add(type: string, data: any) { ... }
  getLast(n: number): Array<...> { ... }
  dump(): void { console.table(this.buffer) }
}
```

### Критерии завершения
- [x] Клиент подключается к серверу
- [x] Handshake выполняется корректно (не считаем connected до S2C_AuthResult)
- [x] Получение и хранение чанков карты (как POJO)
- [x] Получение и хранение игровых объектов (как POJO)
- [x] Неизвестные сообщения логируются, не крашат клиент
- [x] `messageDispatcher.getDebugBuffer()` возвращает последние 100 сообщений

---

## ✅ Этап 4: Базовый рендер (PixiJS)

### Цель
Инициализировать PixiJS и отобразить пустую сцену с debug overlay.

### Задачи

#### 4.1 Жизненный цикл Pixi

1. Создать `game/GameFacade.ts` — единственная точка входа для Vue:
   ```typescript
   class GameFacade {
     private render: Render | null = null
     
     async init(canvas: HTMLCanvasElement): Promise<void>
     destroy(): void
     isInitialized(): boolean
     
     // Методы для Vue
     onPlayerClick(screenX: number, screenY: number): void
     setZoom(zoom: number): void
     getDebugInfo(): DebugInfo
   }
   ```

2. Создать `game/Render.ts` — основной класс рендера:
   - **Владелец**: `Render` владеет `PIXI.Application`
   - Инициализация через `app.init()` (async в PixiJS v8)
   - Контейнеры: `mapContainer`, `objectsContainer`
   - Игровой цикл (ticker)

3. **Destroy при уходе со страницы**:
   ```typescript
   // В GameView.vue
   onUnmounted(() => {
     gameFacade.destroy()
   })
   
   // В Render.ts
   destroy() {
     this.app.ticker.stop()
     this.app.destroy({ removeView: true }, { children: true, texture: true })
     // Очистить все ссылки
   }
   ```

4. **Защита от повторной инициализации**:
   - Флаг `isInitialized` в GameFacade
   - Проверка перед `init()`: если уже инициализирован — сначала `destroy()`

#### 4.2 Resize и разрешение

1. Обработка resize:
   - `resizeTo: window` или ручной resize
   - Учёт `window.devicePixelRatio` для чёткости на Retina
   - Настройка `resolution` в PIXI.Application
   
   ```typescript
   const resolution = Math.min(window.devicePixelRatio, 2) // Ограничить для производительности
   await app.init({
     resolution,
     autoDensity: true,
     resizeTo: window,
   })
   ```

#### 4.3 Минимальный pointer mapping

Для возможности тестировать этап 5 (конвертация координат):

1. Слушать `pointerdown` на canvas
2. Сохранять последние экранные координаты клика
3. Эмитить событие для debug overlay

```typescript
canvas.addEventListener('pointerdown', (e) => {
  this.lastClickScreen = { x: e.clientX, y: e.clientY }
  this.emit('debug:click', this.lastClickScreen)
})
```

#### 4.4 Debug Overlay

Создать `game/DebugOverlay.ts` (PIXI.Text):

| Метрика | Источник |
|---------|----------|
| FPS | `app.ticker.FPS` |
| Camera X, Y | `camera.position` |
| Zoom | `camera.scale` |
| Viewport | `app.screen.width x height` |
| Last click (screen) | из pointer mapping |
| Last click (world) | после этапа 5 |
| Objects count | из gameStore |
| Chunks loaded | из gameStore |

```typescript
class DebugOverlay {
  private text: PIXI.Text
  
  update(info: DebugInfo) {
    this.text.text = `
      FPS: ${info.fps.toFixed(0)}
      Camera: ${info.cameraX}, ${info.cameraY}
      Zoom: ${info.zoom.toFixed(2)}
      Viewport: ${info.viewportWidth}x${info.viewportHeight}
      Click: ${info.lastClickX}, ${info.lastClickY}
    `
  }
}
```

Включать/выключать по клавише \` (backtick) или из config.DEBUG.

#### 4.5 Vue компонент

1. Создать `GameView.vue`:
   - `<canvas ref="gameCanvas">` для PixiJS
   - `onMounted` → `gameFacade.init(canvas)`
   - `onUnmounted` → `gameFacade.destroy()`
   - Не импортировать PIXI напрямую!

### Критерии завершения
- [x] Canvas отображается на странице игры
- [x] PixiJS инициализируется без ошибок
- [x] Уход со страницы `/game` → корректный destroy (без утечек)
- [x] Повторный вход на `/game` → reinit без ошибок
- [x] Resize работает, картинка не мыльная на Retina
- [x] Debug overlay показывает FPS, viewport, click coords
- [x] Клик по canvas регистрируется (в debug overlay)

---

## ✅ Этап 5: Изометрическая проекция и константы

### Цель
Портировать систему координат и констант из `web_old`.

### Задачи
1. Создать `game/Tile.ts` с константами:
   - `COORD_PER_TILE` =  (игровые единицы на тайл), брать из пакета S2C_PlayerEnterWorld coord_per_tile
   - `TEXTURE_WIDTH` = 64, `TEXTURE_HEIGHT` = 32
   - `CHUNK_SIZE` = (тайлов в чанке), брать из пакета S2C_PlayerEnterWorld chunk_size
   - `FULL_CHUNK_SIZE` = пересчитать из CHUNK_SIZE * COORD_PER_TILE
2. Создать `utils/Point.ts` — класс для работы с координатами
3. Создать `utils/Coord.ts` — тип для координат
4. Реализовать функции конвертации:
   - `coordGame2Screen(x, y)` — игровые → экранные
   - `coordScreen2Game(screenX, screenY)` — экранные → игровые
   - **Учитывать текущую позицию и масштаб камеры** (`camera.x`, `camera.y`, `zoom`) при преобразовании координат, чтобы перемещение/масштабирование камеры корректно отражалось в результатах конвертации. 

### Критерии завершения
- [x] Функции конвертации работают корректно (coordGame2Screen, coordScreen2Game, с камерой)
- [x] Клик по canvas возвращает правильные игровые координаты

---

## ✅ Этап 6: Рендер тайлов (Chunk)

### Цель
Отобразить тайловую карту с изометрической проекцией.

### Задачи
1. Загрузить атласы тайлов (`base.json`, `tiles.json`)
2. Создать `game/TileSet.ts`:
   - Загрузка конфигураций тайлов (ground, borders, corners)
   - Случайный выбор текстуры по координатам
+3. Создать `game/Chunk.ts`:
   - **Запекание (baking) геометрии тайлов чанка в VBO** при получении данных о чанке.
     Это позволяет один раз сформировать Vertex Buffer Object и переиспользовать его
     до тех пор, пока чанк не будет выгружен, минимизируя динамические загрузки в GPU.
     Дробим чанк на subchunks (divide_factor = 4), значи дробим чанк на области 4x4 и создаем для каждой из них отдельный  VBO
   - Построение меша для чанка (используя заранее запечённые VBO)
   - WebGL‑шейдеры для оптимизации отрисовки
   - Автоматические переходы между тайлами (borders/corners)4. Интегрировать с `gameStore`:
   - При получении `S2C_ChunkLoad` создавать Chunk
   - При получении `S2C_ChunkUnload` скрывать Chunk
5. Добавить кэширование и удаление старых гридов

### Критерии завершения
- [x] Тайлы отображаются в изометрической проекции
- [x] Переходы между типами тайлов плавные (borders/corners)
- [x] Карта прокручивается при движении игрока (через setCamera)
- [x] Производительность: VBO-рендеринг через Mesh
- [x] При загрузке чанка создаётся 16 subchunks (DIVIDER=4), каждый со своим VBO

---

## ✅ Этап 7: Рендер объектов

### Цель
Отобразить игровые объекты (персонажи, ресурсы, строения).

### Задачи
1. Создать `game/GameObject.ts` — интерфейс игрового объекта
2. Создать `game/ObjectView.ts`:
   - Загрузка спрайтов по `resource` полю
   - Поддержка многослойных объектов (shadows, layers)
   - Интерактивность (клик, hover)
3. Загрузить конфигурацию объектов (`objects.json`)
4. Реализовать Z-сортировку объектов по Y-координате
5. Обработка сообщений:
   - `S2C_ObjectSpawn` → создать ObjectView
   - `S2C_ObjectDespawn` → удалить ObjectView
6. читать object_type из S2C_ObjectSpawn
7. временно. Рендер всех объектов с типом (object_type) 1 в виде крестика синим цветом, с типом=6 красным цветом


### Критерии завершения
- [x] Объекты отображаются на карте
- [x] Объекты правильно сортируются по глубине
- [x] Клик по объекту регистрируется

---

## ✅ Этап 8: Движение объектов (production-grade)

### Цель
Реализовать **плавное и стабильное** перемещение объектов на клиенте в 2D/изометрическом мире на основе серверных обновлений, с устойчивостью к **jitter / out-of-order / packet loss**, без микротелепортов при малых рассинхронизациях. Скорости невысокие (до ~0.5 тайла/с).

---

### Основные принципы (обязательно)

1. **Сервер авторитетен**: клиент не “угадывает” коллизии и не исправляет сервер; клиент только сглаживает отображение.
2. **Не “lerp к последнему пакету”**, а:
    - буфер серверных состояний (keyframes) на сущность
    - интерполяция по времени на “задержанном” renderTime
3. Используем метаданные протокола:
    - `stream_epoch` — защита от гонок при enter/leave/teleport
    - `move_seq` — монотонный порядок обновлений per entity
    - `server_time_ms` — привязка к времени сервера
    - `is_teleport` — принудительный snap/reset

---

### Задачи

#### 8.1 TimeSync (оценка серверного времени на клиенте)
1. Создать модуль `network/TimeSync.ts` (или `game/TimeSync.ts`), который:
    - по `Ping/Pong` оценивает `serverTimeOffsetMs` (server_now ≈ client_now + offset)
    - оценивает `rttMs` и `jitterMs`
    - сглаживает offset (EWMA/median последних N измерений), чтобы offset не прыгал

2. Ввести целевую задержку интерполяции:
    - `interpolationDelayMs` = base (например 120ms, вывести в константу) + запас от jitter
    - ограничить диапазон: например `[80ms..250ms]`, вывести в константу

> Это необходимо, чтобы интерполяция была устойчивой при неравномерной доставке пакетов.

---

#### 8.2 Буфер движения (keyframes) на сущность
1. Создать `game/MoveController.ts`, который ведёт runtime-состояние per entity, отдельно от Pinia store.

2. На каждую сущность хранить кольцевой буфер кадров движения:
    - `{ tServerMs, x, y, vx, vy, isMoving, moveMode, heading, moveSeq }`
    - максимум N кадров (например 32, вывести в константу)

3. Правила приёмки `S2C_ObjectMove`:
    - если `stream_epoch` не совпадает с текущим — **игнор**
    - если `move_seq <= lastMoveSeq` (с учётом wrap, если нужно) — **игнор**
    - если `is_teleport=true` — **сбросить буфер**, установить позицию снапом, начать буфер заново
    - добавить кадр в буфер, поддерживая сортировку по `tServerMs` (на практике достаточно игнора out-of-order по seq)

---

#### 8.3 Интерполяция по времени (главный режим)
1. В тикере рендера вычислять:
    - `serverNowMs = TimeSync.estimateServerNowMs(clientNowMs)`
    - `renderTimeMs = serverNowMs - interpolationDelayMs`

2. Если в буфере есть два кадра `A` и `B`, такие что:
    - `A.tServerMs <= renderTimeMs <= B.tServerMs`
      то вычислять:
    - `alpha = (renderTimeMs - A.t) / (B.t - A.t)`
    - `pos = lerp(A.pos, B.pos, alpha)`
    - `heading`/direction брать из сервера

3. Обновлять визуальную позицию объекта через `MoveController` (а не через store).

---

#### 8.4 Ограниченная экстраполяция (на короткое окно)
Если `renderTimeMs` вышло за последний кадр в буфере (нет `B`):
1. Разрешить экстраполяцию по velocity на короткое время:
    - `maxExtrapolationMs` например 150–200ms, вывести в константу
2. Если окно превышено:
    - мягко затухать скорость до нуля

> Это защищает от кратких провалов доставки без резких остановок/рывков.

---

#### 8.5 Коррекция рассинхронизации (error correction) и пороги snap
Добавить правила, предотвращающие микротелепорты и “плавание”:
1. Ввести `snapDistance` (в игровых координатах или в долях тайла), например:
    - snap если ошибка > 0.75 тайла (вывести в константу)
2. Для малых ошибок:
    - применять не постоянный “lerp к цели”, а сглаживание ошибки с ограничением скорости коррекции (smooth-damp / critically damped)
3. `is_teleport=true` всегда приводит к snap/reset.

> Важно: при больших ошибках нельзя “красиво” проезжать сквозь препятствия — нужен snap.

---

#### 8.6 Остановка и направление (heading)
1. Остановка:
    - если сервер прислал `is_moving=false` и/или `velocity=0`, клиент должен прийти в стоп **без инерционного “проскальзывания”**
    - допускается только микросглаживание до финальной позиции

2. Heading:
    - использовать `position.heading` с сервера

---

#### 8.7 Интеграция с рендером и store
1. В `gameStore` хранить POJO (серверные данные), но:
    - не обновлять store на каждом тике движения
    - store обновляется только по входящим `S2C_ObjectMove` (как source of truth)

2. `MoveController`:
    - принимает входящие движения
    - на каждом кадре выдаёт `renderPosition` для `ObjectView`

3. `ObjectView`:
    - читает сглаженную позицию и применяет её к PIXI-объекту
    - анимации (walk/idle) будут в следующем этапе (Spine), но флаги `isMoving`/direction уже должны быть готовы

---

#### 8.8 Debug: визуализация и метрики (обязательно для production)
В debug mode добавить:
1. Рисование:
    - текущей позиции (visual)
    - интерполированной целевой позиции (target)
    - линии ошибки (visual -> target)
    - последних N keyframes (точки/полилиния)

2. Текстовые метрики:
    - `rttMs`, `jitterMs`, `timeOffsetMs`
    - `interpolationDelayMs`
    - `bufferSize` per entity, “buffer underrun” (вышли в extrapolation)
    - `lastMoveSeq`, количество игнорированных out-of-order пакетов
    - количество snap/teleport событий

---

### Критерии завершения
- [x] Объекты перемещаются плавно при обычной сети (без рывков)
- [x] При jitter (±50–100ms) нет дрожания и микротелепортов: интерполяция стабильна
- [x] Out-of-order пакеты не “откатывают” объект назад (работает `move_seq`)
- [x] При `stream_epoch` mismatch пакеты игнорируются (нет гонок после enter/teleport)
- [x] При `is_teleport=true` происходит корректный snap/reset буфера
- [x] При кратком отсутствии апдейтов движение продолжается предсказуемо (bounded extrapolation) и не “улетает”
- [x] При остановке объект приходит к стопу без проскальзывания и без дрожания
- [x] Debug overlay показывает метрики (RTT/jitter/offset, buffer health, snaps) и визуализацию траектории
---

## ✅ Этап 9: Управление игроком

### Цель
Реализовать управление персонажем игрока (клик-to-move, клики по объектам, камера: follow/pan/zoom) без спама команд и без конфликтов с будущим UI (инвентарь/контекстное меню).

---

### Принципы (важно)
1. **Сервер авторитетен**: клиент отправляет намерения (MoveTo / MoveToEntity), а отображение движения уже сглаживает `MoveController` (этап 8).
2. **Разделение ответственности**:
    - Input (мышь/клавиатура) → нормализованные события
    - CameraController → follow/pan/zoom
    - PlayerCommandController → формирование и отправка сетевых команд
3. **Не отправлять команды при drag/pan** (клик и перетаскивание должны различаться threshold’ом).
4. **Follow камеры** должен использовать **интерполированную (visual/render) позицию** игрока, а не “сырую” из store, чтобы не было jitter.
5. **Модификаторы** (Shift/Ctrl/Alt) отправляются как bitflags в `C2S_PlayerAction.modifiers` (семантика — на стороне сервера).

---

### Задачи

#### 9.1 Клик по карте → MoveTo
1. На `pointerup` (если это клик, а не drag):
    - взять screen coords
    - конвертировать в world/game coords (через существующую конвертацию, учитывая камеру/zoom)
    - отправить `C2S_PlayerAction { move_to: { x, y }, modifiers }`

> Примечание: формат координат должен соответствовать серверному `Position.x/y` (игровые единицы), без дополнительных догадок.

---

#### 9.2 Клик по объекту → MoveToEntity
1. При клике по интерактивному объекту (hit-test):
    - отправить `C2S_PlayerAction { move_to_entity: { entity_id, auto_interact }, modifiers }`
2. Приоритет:
    - если под курсором объект → `MoveToEntity`
    - иначе → `MoveTo`

---

#### 9.3 Модификаторы (Shift/Ctrl/Alt)
1. На каждый отправляемый `C2S_PlayerAction` добавлять `modifiers`:
    - SHIFT = 1
    - CTRL = 2
    - ALT = 4
2. Если окно потеряло фокус (`blur`/`visibilitychange`) — сбросить состояние модификаторов/зажатых кнопок (антизалипание).

---

#### 9.4 Камера следует за игроком (follow)
1. Камера центрируется на игроке:
    - target позиции берутся из визуальной позиции игрока (runtime/MoveController), а не из store.
2. Плавность:
    - камера двигается с демпфированием (например, lerp по dt или critically-damped) вместо мгновенного снапа.
    - также камера может двигаться с жесткой привязкой к позиции игрока с отступом, выбора поведения камеры вывести в константу (feature flag)
3. При ручном перемещении камеры (pan) follow:
    - запоминаем отступ камеры от позиции игрока

---

#### 9.5 Перетаскивание карты (средняя кнопка мыши)
1. При `pointerdown` middle mouse:
    - начать режим pan, включить `setPointerCapture`
2. При `pointermove`:
    - смещать камеру на delta движения в screen space с учетом zoom.
3. При `pointerup`:
    - завершить pan, release capture.

---

#### 9.6 Масштабирование колёсиком мыши
1. На `wheel` над canvas:
    - `preventDefault()`
    - изменить zoom в пределах `[ZOOM_MIN..ZOOM_MAX]`
2. Zoom должен быть **стабильным и ожидаемым**:
    - рекомендуется zoom-to-cursor (точка под курсором сохраняет world position).

---

### Константы (вынести)
- `CLICK_DRAG_THRESHOLD_PX` (например 8)
- `ZOOM_MIN`, `ZOOM_MAX`
- `ZOOM_SPEED`
- `CAMERA_FOLLOW_LERP` или параметры демпфирования
- `CAMERA_PAN_SPEED`

---

### Критерии завершения
- [x] Клик по карте отправляет `MoveTo` с корректными координатами и modifiers.
- [x] Клик по объекту отправляет `MoveToEntity` (объект имеет приоритет над землёй).
- [x] Drag middle mouse перемещает камеру и **не** отправляет команду движения.
- [x] Камера следует за игроком плавно (без jitter, источник — визуальная позиция).
- [x] Масштабирование колесом работает, не скроллит страницу, zoom ограничен min/max.
- [x] После `blur`/`visibilitychange` нет “залипших” состояний (drag/modifiers).
---

## Этап 10: Spine анимации

### Цель
Добавить поддержку Spine анимаций для персонажей.

### Задачи
1. Установить `@esotericsoftware/spine-pixi-v8`
2. Расширить `ObjectView.ts`:
   - Загрузка Spine skeleton
   - Управление анимациями (idle, walk)
   - Смена направления (8 направлений)
3. Интеграция с `MoveController`:
   - При движении включать walk анимацию
   - При остановке включать idle анимацию
   - Выбор анимации по направлению

### Критерии завершения
- [ ] Персонажи со Spine отображаются
- [ ] Анимация меняется при движении/остановке
- [ ] Направление анимации соответствует движению

---

## Этап 11: Terrain объекты (sprites сейчас, VBO позже)

### Цель
Добавить **детерминированную** генерацию декоративных terrain-объектов (деревья, кусты, камни) поверх тайлов.
В текущей версии — **обычными спрайтами** с корректным **z-order относительно игрока**.
Архитектурно заложить возможность будущего **baking terrain в VBO Mesh** (все текстуры в одном атласе).

### Принципы (обязательно)
1. **Terrain = client-only декор**, НЕ хранится в Pinia store и НЕ приходит с сервера.
2. **Один атлас** для всех terrain-текстур → ожидаем хорошее batching, но z-order остаётся важнее.
3. **Корректный z-order с игроком**:
    - terrain и игровые объекты должны сортироваться **в одном sortable-контейнере** (единая сортировка по `y`).
4. **Детерминированность**:
    - генерация (вариант/слои) зависит от `(tileX, tileY, layerIndex, seed)` — при rebuild раскладка не меняется.

---

### Задачи

#### 11.1 Модели и загрузка конфигов
1. Добавить `game/terrain/types.ts`:
    - `TerrainConfig`, `TerrainVariant`, `TerrainLayer`
2. Добавить `game/terrain/TerrainGenerator.ts`:
    - `generate(tileX, tileY, anchorScreenX, anchorScreenY): PIXI.Sprite[] | undefined`
    - детерминированный выбор варианта (chance) и слоёв (p), опциональные `z`-offsets per layer
3. Добавить `game/terrain/TerrainRegistry.ts`:
    - `tileType -> TerrainGenerator` (например: лес/пустошь)
4. Подключить загрузку конфигов `wald.json`, `heath.json` через `ResourceLoader` (или аналог), без динамических `fetch` в рендер-цикле.

#### 11.2 Интеграция в построение чанка (без запекания)
1. При сборке subchunk (в этапе 6) после ground/borders/corners:
    - terrain генерировать **только если** на тайле **не** были добавлены borders/corners (аналог `wasCorners`).
2. Terrain-спрайты добавлять **не в mapContainer**, а в общий слой объектов:
    - `objectsContainer` (или `objectManager.getContainer()`), чтобы игрок и terrain сортировались вместе.

> Примечание: при текущей архитектуре `objectsContainer` сортируется отдельно (и mapContainer тоже). Поэтому terrain, который должен перекрывать игрока, обязан жить в том же контейнере, что и игрок.

#### 11.3 Z-order (единая формула)
1. Ввести единое правило `zIndex` для всех drawable-объектов (игроки/объекты/terrain):
    - базово: `zIndex = sprite.y + localZOffset`
2. Для многослойного terrain:
    - `localZOffset` берётся из слоя конфига (`layer.z`, если есть), иначе 0.
3. Обеспечить сортировку:
    - `objectsContainer.sortableChildren = true`
    - сортировку выполнять **после** обновления позиций (terrain статичен, игроки двигаются).

#### 11.4 Cleanup / жизненный цикл
1. Terrain принадлежит чанку/subchunk’у по данным, но живёт в objectsContainer:
    - при `unloadChunk` необходимо удалить все terrain-спрайты, принадлежащие этому чанку/subchunk’у.
2. Ввести структуру учёта:
    - `chunkKey -> Sprite[]` или `subChunkKey -> Sprite[]` (чтобы быстро чистить).
3. При rebuild чанка:
    - сначала удалить старые terrain спрайты для чанка
    - затем сгенерировать заново (результат детерминирован — визуально не меняется).

---

### 11.5 Архитектурная закладка под будущее VBO-baking (не реализуем сейчас)
Добавить абстракцию рендера terrain:

- `ITerrainRenderer`
    - `addTile(tileX, tileY, anchorScreenX, anchorScreenY, tileType, context)`
    - `finalize()` / `destroy()`

Две реализации (в будущем):
1. `TerrainSpriteRenderer` (делаем в этом этапе)
    - создаёт `Sprite` и добавляет в `objectsContainer`
2. `TerrainMeshRenderer` (будущий этап оптимизации)
    - собирает terrain-quad’ы в `VertexBuffer` и строит `Mesh` по subchunk’ам
    - использует один атлас (один sampler), как у земли

Критично: формат входных данных должен позволять обоим рендерам работать одинаково.
Поэтому `TerrainGenerator` должен уметь выдавать не только `Sprite`, но и “render commands”:
- `TerrainDrawCmd = { textureFrameId, x, y, w, h, zOffset }`
  а `TerrainSpriteRenderer` уже превращает cmds в Sprite, `TerrainMeshRenderer` — в VBO.

---

### Критерии завершения
- [ ] Terrain рендерится обычными спрайтами, корректно перекрывается с игроком (единая сортировка в objectsContainer).
- [ ] Детерминированность: rebuild/unload+load не меняет раскладку terrain.
- [ ] Все terrain текстуры берутся из одного атласа, без лишних draw calls из-за разных texture sources.
- [ ] Архитектура допускает замену рендера terrain на Mesh/VBO без изменения логики генерации (через `TerrainDrawCmd` + `ITerrainRenderer`).

---

## Этап 12: Инвентарь (UI)

### Цель
Реализовать базовый UI инвентаря.

### Задачи
1. Создать `stores/inventoryStore.ts`
2. Обработка сообщений:
   - `S2C_InventoryUpdate`
   - `S2C_ContainerOpened` / `S2C_ContainerClosed`
3. Vue компоненты:
   - `Inventory.vue` — сетка инвентаря
   - `ItemSlot.vue` — ячейка
   - `Item.vue` — предмет
   - `Hand.vue` — предмет в руке (курсор)
4. Drag & Drop предметов
5. Отправка `C2S_InventoryOp` при перемещении

### Критерии завершения
- [ ] Инвентарь открывается по Tab
- [ ] Предметы отображаются в ячейках
- [ ] Можно перетаскивать предметы

---

## Этап 13: Контекстное меню

### Цель
Реализовать контекстное меню для объектов.

### Задачи
1. Создать `ContextMenu.vue`
2. При ПКМ по объекту показывать меню
3. Отправка соответствующих action на сервер

### Критерии завершения
- [ ] ПКМ по объекту показывает меню
- [ ] Выбор действия отправляет команду на сервер

---

## Этап 14: Чат

### Цель
Реализовать игровой чат.

### Задачи
1. Создать `Chat.vue` компонент
2. Обработка серверных сообщений чата
3. Отправка сообщений на сервер
4. Горячая клавиша Enter для фокуса

### Критерии завершения
- [ ] Сообщения отображаются в чате
- [ ] Можно отправлять сообщения
- [ ] Enter фокусирует поле ввода

---

## Этап 15: Статус игрока (UI)

### Цель
Отобразить статистику и состояние игрока.

### Задачи
1. Создать `PlayerStats.vue`
2. Отображение:
   - Здоровье
   - Выносливость
   - Голод
3. Создать `DayTime.vue` — время суток

### Критерии завершения
- [ ] Полоски состояния отображаются
- [ ] Значения обновляются при получении данных

---

## Этап 16: Оптимизация и polish

### Цель
Оптимизировать производительность и улучшить UX.

### Задачи
1. Профилирование и оптимизация:
   - Culling невидимых объектов
   - Batching спрайтов
   - Object pooling
2. Обработка ошибок соединения
3. Reconnect логика
4. Loading states
5. Кастомные курсоры
6. Звуки (опционально)

### Критерии завершения
- [ ] 60 FPS на среднем оборудовании
- [ ] Корректная обработка отключений
- [ ] Отсутствие memory leaks

---

## Зависимости между этапами

```
1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9
                         ↓
                        10 → 11
                         
3 → 12 → 13
3 → 14
3 → 15
```

- Этапы 1-9 — последовательные, формируют ядро
- Этапы 10-11 — расширение рендера объектов
- Этапы 12-15 — UI, можно делать параллельно после этапа 3
- Этап 16 — финальный

---

## Ресурсы из существующих проектов

### Из `web_old` (адаптировать):
- `game/Render.ts` — основа рендера
- `game/Chunk.ts` — рендер тайлов
- `game/Tile.ts` — константы и TileSet
- `game/ObjectView.ts` — отображение объектов
- `game/MoveController.ts` — интерполяция движения
- `util/Point.ts`, `util/Coord.ts` — утилиты координат
- `util/VertexBuffer.ts` — буфер вершин для меша
- `util/random.ts` — детерминированный random
- `assets/` — графические ресурсы

### Из `web` (использовать как есть или адаптировать):
- `network/GameConnection.js` → `.ts` — WebSocket логика
- `proto/packets.js` → regenerate с типами
- `stores/game.js` → `.ts` — game store
- `stores/auth.js` → `.ts` — auth store
- `api/` — HTTP API

### Из `api/proto/packets.proto`:
- Генерация TypeScript типов

---

## Технический стек

| Категория | Технология | Версия |
|-----------|-----------|--------|
| Framework | Vue | 3.x |
| Language | TypeScript | 5.x |
| Build | Vite | 6.x |
| State | Pinia | 2.x |
| Rendering | PixiJS | 8.x |
| Animation | Spine | 4.2.x |
| Protocol | Protobuf | 7.x |
| Styles | SCSS | - |
