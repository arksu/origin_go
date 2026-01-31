# Отладка движения (Movement Debug)

## Включение логирования

Для включения детального логирования движения установите в `src/config/index.ts`:

```typescript
DEBUG_MOVEMENT: true, // Enable detailed movement logging
```

## Что логируется

### 1. PlayerCommandController
- **Когда**: Отправка команд движения
- **Что**: `MoveTo`, `MoveToEntity`, `Interact`
- **Данные**: цель, модификаторы, timestamp

```
[PlayerCommandController] Sending MoveTo: {
  target: "(100, 200)",
  modifiers: 0,
  timestamp: 1706723456789
}
```

### 2. MoveController
- **Когда**: Получение ключевых кадров от сервера
- **Что**: Инициализация сущности, телепорты, out-of-order пакеты, интерполяция
- **Данные**: позиции, velocity, временные дельты, буфер

```
[MoveController] Keyframe added for entity 123: {
  seq: 45,
  pos: "(100.50, 200.75)",
  velocity: "(5.20, 3.10)",
  isMoving: true,
  timeDelta: "50ms",
  posDelta: "2.83",
  bufferSize: 3
}
```

```
[MoveController] Entity 123: {
  prevPos: "(98.20, 198.45)",
  newPos: "(100.50, 200.75)",
  distance: "3.25",
  isMoving: true,
  isExtrapolating: false,
  keyframes: 3,
  renderTimeMs: 1706723456789,
  lastKeyframe: 1706723456739,
  moveSeq: 45
}
```

### 3. CameraController
- **Когда**: Движение камеры следования за игроком
- **Что**: Позиция сущности, цель камеры, текущая позиция, дельта
- **Данные**: все позиции, расстояние, pan offset

```
[CameraController] Following entity 123: {
  entityPos: "(100.50, 200.75)",
  targetPos: "(100.50, 200.75)",
  currentPos: "(99.80, 200.10)",
  delta: "(0.70, 0.65)",
  distance: "0.95",
  panOffset: "(0.00, 0.00)",
  isMoving: true
}
```

### 4. Render
- **Когда**: Необычное время кадра
- **Что**: Frame time и FPS
- **Данные**: время в мс, FPS

```
[Render] Frame time: 25.34ms (39.5 FPS)
```

## Поиск причин рывков

### 1. Пропуски кадров
Ищите сообщения от Render с высоким frame time (>20ms):
```
[Render] Frame time: 35.20ms (28.4 FPS)
```

### 2. Проблемы с буфером движения
Проверяйте `bufferSize` и `isExtrapolating`:
```
[MoveController] Entity 123: {
  bufferSize: 0,        // Пустой буфер - extrapolation
  isExtrapolating: true, // Экстраполяция из-за пропуска пакетов
}
```

### 3. Out-of-order пакеты
Ищите предупреждения:
```
[MoveController] Out-of-order packet for entity 123: seq 47 <= last 48
```

### 4. Большие скачки позиции
Проверяйте `distance` в логах MoveController:
```
[MoveController] Entity 123: {
  distance: "15.75",    // Большой скачок за один кадр
}
```

### 5. Проблемы с камерой
Сравнивайте `entityPos` и `currentPos`:
```
[CameraController] Following entity 123: {
  entityPos: "(100.50, 200.75)",
  currentPos: "(85.20, 190.30)", // Камера отстает
  delta: "(15.30, 10.45)",       // Большая дельта
  distance: "18.42",
}
```

## Типичные сценарии проблем

### Сценарий 1: Рывок при начале движения
```
[PlayerCommandController] Sending MoveTo: { target: "(100, 200)" }
// ...пауза...
[MoveController] Keyframe added: { seq: 1, pos: "(100, 200)" }
[MoveController] Entity 123: { distance: "25.50" } // Большой скачок
```

**Причина**: Задержка между командой и первым ключевым кадром.

### Сценарий 2: Дрожание при движении
```
[MoveController] Entity 123: { distance: "2.10", isExtrapolating: true }
[MoveController] Entity 123: { distance: "1.85", isExtrapolating: false }
[MoveController] Entity 123: { distance: "2.30", isExtrapolating: true }
```

**Причина**: Нестабильная доставка пакетов, постоянная смена extrapolation.

### Сценарий 3: Камера "прыгает"
```
[CameraController] Following entity 123: { distance: "12.75" }
[CameraController] Following entity 123: { distance: "0.15" }
[CameraController] Following entity 123: { distance: "11.80" }
```

**Причина**: Проблемы с интерполяцией позиции сущности.

## Рекомендации по анализу

1. **Включите логирование** только для отладки
2. **Соберите логи** для конкретного сценария проблемы
3. **Ищите аномалии**: большие дельты, пустые буферы, extrapolation
4. **Сравнивайте тайминги** между командами и ответами сервера
5. **Проверяйте последовательность** move_seq

## Отключение логирования

```typescript
DEBUG_MOVEMENT: false, // Disable detailed movement logging
```
