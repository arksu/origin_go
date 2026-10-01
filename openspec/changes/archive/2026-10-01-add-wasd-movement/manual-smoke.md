# WASD: протокол ручной проверки

Статус: **PASS по подтверждению пользователя от 2026-10-01**. После получения этого протокола пользователь сообщил: «я провел ручные тесты. все ок». На этом основании пункты 8.1–8.4 закрыты. Автоматические результаты — в `validation.md`.

## Результат ручной приёмки

| Пункт | Результат | Источник |
|---|---|---|
| 8.1 — направления, FPS, zoom и раскладка | PASS | Подтверждение пользователя |
| 8.2 — UI/handoff и lifecycle | PASS | Подтверждение пользователя |
| 8.3 — коллизии, carry, observer и renderer | PASS | Подтверждение пользователя |
| 8.4 — задержки, обрыв доставки и suspension | PASS | Подтверждение пользователя |

Подтверждение предоставлено общим итогом. Численные замеры задержки, параметры среды и вывод browser harness в переписку не переданы; они здесь не воспроизводятся. Агент не выполнял эти browser-проверки самостоятельно. Ниже сохранён протокол для повторных прогонов.

## Подготовка

1. Запустить сервер из текущего working tree и клиент с обновлённым protobuf. Уже работающий старый сервер нужно обновить самостоятельно; агент его не перезапускал.
2. Войти двумя разными персонажами в двух браузерных сессиях рядом друг с другом. Первый — управляемый игрок, второй — наблюдатель.
3. В enter-world первого клиента проверить `directionalMovementSupported=true`, ненулевой `streamEpoch`, готовность мира. В Console не должно быть новых ошибок.
4. Подготовить свободную площадку, стену, угол, воду, границу чанков, переносимый объект и повторяемое действие. Для отмены действия на последнем тике удобен короткий цикл с известной стоимостью stamina и результатом.
5. Записать ревизию/dirty state, версию браузера и ОС, tick rate, режим рендера персонажей, coordPerTile, zoom, измеренный FPS и RTT. Ограничение CPU само по себе не доказывает заданный FPS; записывать фактический FPS. Недоступные 144 FPS помечать `NOT RUN`.

## 8.1. Направления и частота кадров

Повторить при фактических 30/60/144 FPS, zoom 0.5/1/2 и русской раскладке:

| Ввод | Ожидаемый результат |
|---|---|
| W / D / S / A | Вверх / вправо / вниз / влево относительно экрана |
| W+D / D+S / S+A / A+W | Соответствующая экранная диагональ; та же мировая скорость, что у одиночной клавиши |
| W+S, A+D, все четыре | Нулевой итоговый вектор; остановка |
| W+S+D, затем отпустить S | Вправо, затем вверх-вправо |
| Держать 3 с, быстро нажать/отпустить, развернуть | Изменение команды сразу; после отпускания нет возобновления |
| Удержание с Shift | Направление и выбранный movement mode не меняются из-за Shift |
| Pan средней кнопкой, zoom, hover | Продолжается исходное направление; камера не меняет мировую скорость |

Скорость сравнивать по серверным позициям за одинаковые интервалы при одинаковом movement mode, без столкновений, carry и изменения stamina. Экранная длина шага в изометрии может отличаться между направлениями. Проверить нормализованный wire-вектор W+D: примерно `(-0.31622777, -0.9486833)`.

## 8.2. Переключение управления и lifecycle

Для каждого handoff: удерживать W → выполнить действие → оставить W зажатой ещё секунду → отпустить W → нажать D заново.

| Сценарий | Ожидаемый результат |
|---|---|
| Primary по земле/объекту, dropped-item pickup | Один zero release перед обычной командой; маршрут/действие продолжается после старого keyup |
| Secondary по земле/объекту, touch long-press | Тот же порядок; long-press не создаёт дополнительный primary на отпускании |
| Предмет в руке: drop; build/lift ghost: подтверждение | Release перед drop/placement; hand/carry не меняются от самого WASD |
| Hotbar/action activation, context selection, craft one/many, build execution | Release перед запросом; отказ сервера не возобновляет старое удержание |
| Action в фазе selecting → новое W | Движение разрешено, armed action/cursor сохранён |
| W → Enter/chat → keyup в input | Остановка при переходе фокуса; ввод текста не двигает персонажа |
| textarea, select, contenteditable, IME, Ctrl/Alt/Meta | Нет движения из текстового ввода/shortcut; активное удержание прекращается |
| Открыть обычные inventory/character окна | Окно само по себе не останавливает WASD; фокус в его текстовом поле останавливает |
| Blocking modal, Alt-Tab, скрыть вкладку | Однократная остановка и очистка refresh; возврат требует нового нажатия |
| Disconnect/reconnect, leave/enter, teleport, смена layer, rollback неудачного transfer | Старое удержание не переносится; новый ввод работает только после bootstrap и с новой эпохой |
| Потерять keyup вне окна, вернуться и снова нажать ту же клавишу | Свежее неповторное нажатие снова работает |
| Mouse-only secondary по земле | Нет лишнего directional release; прежнее поведение мыши сохранено |

После handoff проверить, что свежий D не добавляет всё ещё подавленный W. Проверить KO/death: движение прекращается, heartbeat не снимает stun и не возобновляет ходьбу после восстановления. Для диагностики таймера проверить отсутствие исходящих `moveDirection` после остановки в течение 2 с.

## 8.3. Коллизии, carry и observer

В обеих сессиях наблюдать один и тот же управляемый объект:

1. Упереться в стену и угол. Первый resolved stop виден обоим клиентам; после визуального settling нет ходьбы на месте, движения сквозь стену или потока одинаковых stop updates.
2. Продолжая удерживать клавишу, удалить препятствие. Движение возобновляется. Повторить с уже истёкшим удержанием: без нового нажатия движения нет.
3. Идти под углом к стене. Скольжение сохраняется, расстояние за тик не превосходит разрешённой скорости. Повернуть от препятствия: персонаж отходит.
4. Проверить запрещённую воду, границу мира и phantom при lift placement: прохода сквозь ограничения нет.
5. Пересечь границу чанка в обе стороны, затем с carry. Игрок и объект остаются согласованными у наблюдателя; carry ограничивает режим как при кликах.
6. Проверить stamina downgrade и exhaustion. Полный упор не тратит movement stamina. После ограничения, завершившего ввод, нужен новый keydown.
7. Во время approach и на последнем тике repeating action нажать WASD, в том числе в сторону стены. Незавершённый цикл отменяется без результата и action stamina charge. Уже завершённый предыдущий цикл не откатывается. Поздний callback не меняет новое действие.
8. Повторить для lift pickup/put-down и tile approach. При отмене put-down carry остаётся валидным; старый placement не срабатывает позже.
9. В отдельной вкладке dev-сервера открыть `/tests/hybrid-integration.html` (обычно `http://localhost:5173/tests/hybrid-integration.html`). Дождаться **ALL CHECKS PASSED**, сохранить весь вывод. Harness включает `tests/movement-stop.ts`; это отдельная проверка настоящего renderer.

## 8.4. Задержки и обрыв доставки

Профили: обычная сеть; измеренный RTT около 100 мс; около 250 мс; RTT с jitter; прекращение доставки без корректного release. Настройка throttling должна реально воздействовать на WebSocket. Подтверждать её Ping/Pong/TimeSync; название профиля DevTools не является измерением.

На каждый профиль сделать минимум 10 отдельных стартов из полностью неподвижного состояния, отпусканий и разворотов. Записать median/p95/max. После каждого опыта дождаться окончания settling. Не смешивать старт, разворот и release в одну выборку.

| Метрика | Как считать |
|---|---|
| Input → send | Разность `performance.now()` при физическом событии и вызове отправки нового revision; не должна ждать 200-мс refresh |
| Input → server update | `serverTimeMs` первого относящегося к вводу update минус оценка server time в момент ввода; это оценка с погрешностью синхронизации часов |
| Input → receive | Локальное время приёма этого update минус локальное время ввода; включает обратную сетевую задержку |
| Input → visual response | Первый изменённый render position; подтвердить Performance recording/filmstrip, поскольку вычисление позиции предшествует paint |
| Stop settling | От начала применения stationary stop в renderer до точного совпадения с конечной позицией и idle; текущая длительность кривой — 300 мс, отдельно от сети/interpolation delay |
| Authoritative TTL | В серверной трассе: последний принятый fresh receipt → первый тик release; 800 мс с округлением вверх до тика, при работающем tick loop |

Для точного TTL нужна серверная трасса receipt (`game.go`, `handlePlayerAction`), drain (`directional_movement.go`) и runtime `TimeState.Now` на release (`movement.go`). Не вычитать напрямую клиентский `performance.now()` из серверного timestamp. Без серверной трассы записать TTL как `NOT MEASURED`; fake-clock тесты проверяют его отдельно, но это не ручной сетевой замер. При остановке доставки S2C наблюдатель с работающим соединением нужен для проверки серверного stop.

В dev-клиенте можно установить временный сборщик через Console. Он не меняет файлы и не подменяет обработчики протокола; перед повторной установкой восстанавливает предыдущие методы. Делать отдельные короткие прогоны, затем выгружать `wasdProbe.rows`.

```js
window.wasdProbe?.stop()
const { gameConnection } = await import('/src/network/GameConnection.ts')
const { moveController } = await import('/src/game/MoveController.ts')
const { timeSync } = await import('/src/network/TimeSync.ts')
const { useGameStore } = await import('/src/stores/gameStore.ts')
const store = useGameStore()
const rows = []
const record = (kind, fields) => rows.push({ kind, at: performance.now(),
  serverEstimate: timeSync.estimateServerNowMs(Date.now()), ...fields })
const key = event => {
  if (['KeyW', 'KeyA', 'KeyS', 'KeyD'].includes(event.code) && !event.repeat)
    record(event.type, { code: event.code })
}
window.addEventListener('keydown', key, true)
window.addEventListener('keyup', key, true)
const originalSend = gameConnection.send
const originalMove = moveController.onObjectMove
const originalUpdate = moveController.update
gameConnection.send = function (packet) {
  const direction = packet.playerAction?.moveDirection
  if (direction) record('send', { ...direction })
  return originalSend.call(this, packet)
}
moveController.onObjectMove = function (...args) {
  if (args[0] === store.playerEntityId) record('receive', {
    serverTimeMs: args[1], moveSeq: args[2], x: args[4], y: args[5],
    vx: args[6], vy: args[7], isMoving: args[8],
  })
  return originalMove.apply(this, args)
}
let previous
moveController.update = function () {
  const positions = originalUpdate.call(this)
  const position = positions.get(store.playerEntityId)
  if (position && (!previous || position.x !== previous.x || position.y !== previous.y ||
      position.isMoving !== previous.isMoving)) record('visual', { ...position })
  previous = position && { ...position }
  return positions
}
window.wasdProbe = { rows, network: () => timeSync.getDebugMetrics(), stop() {
  window.removeEventListener('keydown', key, true)
  window.removeEventListener('keyup', key, true)
  gameConnection.send = originalSend
  moveController.onObjectMove = originalMove
  moveController.update = originalUpdate
} }
```

Сборщик фиксирует вызов send, а не подтверждение доставки. В `ObjectMove` нет echo input revision: сопоставлять только изолированные опыты, исключая уже летящие обновления предыдущего движения. RTT/jitter/interpolation delay смотреть через `wasdProbe.network()`. По завершении выполнить `wasdProbe.stop()` и сохранить `JSON.stringify(wasdProbe.rows)`.

Проверка client timer suspension: на отдельном тестовом клиенте запланировать блокировку main thread, затем сфокусировать игру и удерживать W:

```js
setTimeout(() => {
  const end = performance.now() + 1200
  while (performance.now() < end) { /* намеренная пауза тестового клиента */ }
}, 3000)
```

После паузы ожидается release без серии догоняющих refresh, старое удержание не возобновляется. На сервере истекает lease. Вернуть управление свежим keydown. При blackhole TCP учитывать уже буферизованные сообщения; фиксировать фактический receipt, а не момент изменения сетевого профиля.

## Форма результата

```text
Дата / git revision + dirty state:
Браузер / ОС / renderer mode / tick rate:
Фактические FPS / zoom / coordPerTile:
8.1: PASS | FAIL | NOT RUN — сценарий, наблюдение, запись
8.2: PASS | FAIL | NOT RUN — сценарий, наблюдение, запись
8.3: PASS | FAIL | NOT RUN — player/observer, harness output

8.4 — отдельная строка для каждого профиля и start/turn/release:
RTT median / jitter / interpolation delay / samples:
Input→send median/p95/max:
Input→server update estimate median/p95/max + погрешность:
Input→receive median/p95/max:
Input→visual median/p95/max:
Stop settling / authoritative TTL / timer suspension:
Оценка отзывчивости и выявленные дефекты:
```

Неприемлемая отзывчивость или непроверенный обязательный сценарий остаются открытым результатом. Smoothing/TimeSync/locomotion constants в рамках этой проверки не менять. Отметки в tasks.md обновляются по результатам, а не по факту передачи протокола.
