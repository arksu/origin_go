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

## Этап 8: Движение объектов (production-grade)

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
- [ ] Объекты перемещаются плавно при обычной сети (без рывков)
- [ ] При jitter (±50–100ms) нет дрожания и микротелепортов: интерполяция стабильна
- [ ] Out-of-order пакеты не “откатывают” объект назад (работает `move_seq`)
- [ ] При `stream_epoch` mismatch пакеты игнорируются (нет гонок после enter/teleport)
- [ ] При `is_teleport=true` происходит корректный snap/reset буфера
- [ ] При кратком отсутствии апдейтов движение продолжается предсказуемо (bounded extrapolation) и не “улетает”
- [ ] При остановке объект приходит к стопу без проскальзывания и без дрожания
- [ ] Debug overlay показывает метрики (RTT/jitter/offset, buffer health, snaps) и визуализацию траектории
---

## Этап 9: Управление игроком

### Цель
Реализовать управление персонажем игрока.

### Задачи
1. Обработка кликов по карте:
   - Конвертация экранных координат в игровые
   - Отправка `C2S_PlayerAction` (MoveTo)
2. Обработка кликов по объектам:
   - Отправка `C2S_PlayerAction` (MoveToEntity, Interact)
3. Обработка модификаторов (Shift, Ctrl, Alt)
4. Камера следует за игроком:
   - Центрирование на позиции игрока
   - Плавное перемещение камеры
5. Перетаскивание карты (средняя кнопка мыши)
6. Масштабирование колёсиком мыши
7. Непрерывное движение при зажатой ЛКМ

### Критерии завершения
- [ ] Клик по карте отправляет команду движения
- [ ] Персонаж движется к указанной точке
- [ ] Камера следует за персонажем
- [ ] Масштабирование работает

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

## Этап 11: Terrain объекты

### Цель
Добавить генерацию terrain объектов (деревья, кусты, камни).

### Задачи
1. Портировать `TerrainObjects` из `web_old`
2. Загрузить конфигурации terrain (`wald.json`, `heath.json`)
3. При создании Chunk генерировать terrain sprites
4. Правильная Z-сортировка terrain с объектами

### Критерии завершения
- [ ] На тайлах леса появляются деревья
- [ ] На тайлах пустоши появляется растительность
- [ ] Terrain правильно сортируется с объектами

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
