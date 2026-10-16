import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useGameStore } from '../src/stores/gameStore'
import { gameFacade } from '../src/game/GameFacade'
import { soundManager } from '../src/game/SoundManager'
import { gameConnection } from '../src/network'
import { proto } from '../src/network/proto/packets.js'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { registerMessageHandlers } from '../src/network/handlers'
import { setWorldParams } from './serverConstantsFixture'

function dispatch(packet: proto.IServerMessage) {
  messageDispatcher.dispatch(proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish()))
}

function currentWindow() { return useGameStore().customWindow }

test('custom snapshots use the active world, preserve text and order, and close locally without a command', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  const sends = t.mock.method(gameConnection, 'send', () => {})
  registerMessageHandlers()
  setWorldParams(12, 4)
  game.setConnectionState('connected')
  const window = { entityId: 200, streamEpoch: 7, title: 'Hearth Fire', fields: [{ label: 'Owner', value: '<b>Марко & Friends</b>' }] }
  dispatch({ customWindow: window })
  assert.deepEqual(currentWindow(), null, 'a snapshot before world entry cannot open a window')
  game.setPlayerEnterWorld(1, 'Visitor', 7)
  dispatch({ customWindow: window })
  assert.deepEqual(currentWindow(), { entityId: 200, title: window.title, fields: window.fields })
  for (const streamEpoch of [0, 6, 8]) {
    dispatch({ customWindow: { ...window, streamEpoch, title: 'Wrong world' } })
    dispatch({ customWindowClosed: { entityId: 200, streamEpoch } })
    assert.equal(currentWindow()?.title, 'Hearth Fire')
  }
  dispatch({ customWindowClosed: { entityId: 201, streamEpoch: 7 } })
  assert.equal(currentWindow()?.entityId, 200)
  game.closeCustomWindow()
  assert.deepEqual(currentWindow(), null)
  assert.equal(sends.mock.callCount(), 0, 'local close must preserve the server link')
  dispatch({ customWindow: window })
  assert.equal(currentWindow()?.entityId, 200, 'RMB may reopen the same linked target')
  const fields = [{ label: 'State', value: 'Idle' }, { label: 'Output', value: '2 boards\n1 branch' }]
  dispatch({ customWindow: { entityId: 201, streamEpoch: 7, title: 'Workbench', fields } })
  assert.deepEqual(currentWindow(), { entityId: 201, title: 'Workbench', fields })
  dispatch({ customWindowClosed: { entityId: 200, streamEpoch: 7 } })
  assert.equal(currentWindow()?.entityId, 201, 'old target close cannot hide the new form')
  dispatch({ customWindowClosed: { entityId: 201, streamEpoch: 7 } })
  assert.deepEqual(currentWindow(), null)
})

test('despawn, world entry/exit and connection reset clear the window and reject old snapshots', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(gameFacade, 'despawnObject', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  registerMessageHandlers()
  setWorldParams(12, 4)
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'Visitor', 7)
  const window = { entityId: 200, streamEpoch: 7, title: 'Hearth Fire' }
  dispatch({ customWindow: window })
  dispatch({ objectDespawn: { entityId: 200, streamEpoch: 6 } })
  dispatch({ objectDespawn: { entityId: 201, streamEpoch: 7 } })
  assert.equal(currentWindow()?.entityId, 200)
  dispatch({ objectDespawn: { entityId: 200, streamEpoch: 7 } })
  assert.deepEqual(currentWindow(), null)
  for (const clear of [() => game.setPlayerEnterWorld(1, 'Visitor', 8), () => game.setPlayerLeaveWorld(),
    () => game.setConnectionState('disconnected'), () => game.reset()]) {
    game.setConnectionState('connected')
    game.setPlayerEnterWorld(1, 'Visitor', 7)
    dispatch({ customWindow: window })
    assert.equal(currentWindow()?.entityId, 200)
    clear()
    assert.deepEqual(currentWindow(), null)
    dispatch({ customWindow: window })
    assert.deepEqual(currentWindow(), null)
  }
})
