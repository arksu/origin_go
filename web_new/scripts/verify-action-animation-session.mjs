import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import protobuf from 'protobufjs'

// Run only against an isolated, disposable local review world (see evidence.md).
const server = new URL(process.argv[2] ?? 'http://127.0.0.1:8081')
if (!['localhost', '127.0.0.1', '[::1]'].includes(server.hostname) || server.port !== '8081') throw new Error('The review requires the isolated local server on port 8081')
const repository = new URL('../../', import.meta.url)
const fixture = JSON.parse(await readFile(new URL('tests/fixtures/action_animations/session.json', repository), 'utf8'))
const binding = JSON.parse(await readFile(new URL(fixture.binding_file, repository), 'utf8')).bindings[0]
const root = await protobuf.load(fileURLToPath(new URL('api/proto/packets.proto', repository)))
const ClientMessage = root.lookupType('proto.ClientMessage'), ServerMessage = root.lookupType('proto.ServerMessage')
const slots = root.lookupEnum('proto.EquipSlot').values
const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
const clients = []
const evidence = []
const pass = (name, details = {}) => { evidence.push({ name, ...details }); console.log('PASS', name, JSON.stringify(details)) }
async function request(path, method, body, token) {
  const response = await fetch(new URL(path, server), { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }, body: body ? JSON.stringify(body) : undefined })
  const responseText = await response.text()
  const value = responseText ? JSON.parse(responseText) : null
  if (!response.ok) throw new Error(`HTTP ${response.status} ${path}: ${JSON.stringify(value)}`)
  return value
}
async function connect(label) {
  const account = await request('/accounts/registration', 'POST', { login: `animation_${label}_${Date.now()}`, password: 'local-review-only' })
  await request('/characters', 'POST', { name: `Animation ${label}` }, account.token)
  const characters = await request('/characters', 'GET', undefined, account.token)
  const id = String(characters.list[0].id)
  const { auth_token: token } = await request(`/characters/${id}/enter`, 'POST', undefined, account.token)
  const socket = new WebSocket(new URL('/ws', server).href.replace(/^http/, 'ws'))
  socket.binaryType = 'arraybuffer'
  const client = { id, socket, packets: [], inventories: new Map(), sequence: 0, epoch: 0, lastChat: 0 }
  clients.push(client)
  socket.addEventListener('message', event => {
    const packet = ServerMessage.toObject(ServerMessage.decode(new Uint8Array(event.data)), { longs: String })
    if (packet.playerEnterWorld) client.epoch = packet.playerEnterWorld.streamEpoch
    for (const inventory of [...(packet.inventoryUpdate?.updated ?? []), ...(packet.inventoryOpResult?.updated ?? []), ...(packet.containerOpened ? [packet.containerOpened.state] : [])]) {
      client.inventories.set(inventory.ref.kind ?? 0, inventory)
    }
    client.packets.push(packet)
  })
  client.send = payload => socket.send(ClientMessage.encode(ClientMessage.fromObject({ sequence: ++client.sequence, ...payload })).finish())
  client.wait = async (predicate, since = 0, timeout = 10000) => {
    const deadline = Date.now() + timeout
    while (Date.now() < deadline) {
      const found = client.packets.slice(since).find(predicate)
      if (found) return found
      await pause(20)
    }
    const diagnostics = client.packets.slice(since).filter(p => p.error || p.chat || p.contextMenu || p.inventoryOpResult || p.miniAlert || p.playerActionProgress || p.characterActionAnimation)
    throw new Error(`${label}: timeout; recent ${JSON.stringify(diagnostics.slice(-8))}`)
  }
  client.command = async text => {
    await pause(Math.max(0, 450 - (Date.now() - client.lastChat)))
    client.lastChat = Date.now()
    const cursor = client.packets.length
    client.send({ chat: { text } })
    return cursor
  }
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, { once: true }); socket.addEventListener('error', reject, { once: true }) })
  client.send({ auth: { token } })
  await client.wait(packet => packet.objectSpawn?.entityId === id)
  return client
}
async function teleport(client, position, layer = 0) {
  const before = client.epoch
  const cursor = await client.command(`/tp ${position.x} ${position.y} ${layer}`)
  await client.wait(packet => packet.playerEnterWorld?.streamEpoch > before, cursor)
  return (await client.wait(packet => packet.objectSpawn?.entityId === client.id && packet.objectSpawn.streamEpoch === client.epoch, cursor)).objectSpawn
}
async function moveEquipment(client, itemId, source, destination, slot) {
  const opId = String(client.sequence + 1000), cursor = client.packets.length
  client.send({ inventoryOp: { op: { opId, expected: [...new Set([source, destination])].map(inventory => ({ ref: inventory.ref, expectedRevision: inventory.revision })),
    move: { src: source.ref, dst: destination.ref, itemId, ...(slot === undefined ? { dstPos: { x: 0, y: 0 } } : { dstEquipSlot: slot }) } } } })
  const result = (await client.wait(packet => packet.inventoryOpResult?.opId === opId, cursor)).inventoryOpResult
  assert.equal(result.success, true, result.message)
}
async function spawnTarget(client) {
  const cursor = await client.command(`/spawn ${fixture.target_object}`)
  await client.wait(packet => packet.chat?.text?.includes('Click on the map'), cursor)
  client.send({ playerAction: { mapClick: { ...fixture.target } } })
  const spawn = (await client.wait(packet => packet.objectSpawn?.typeId === fixture.target_type && packet.objectSpawn.position?.position?.x === fixture.target.x, cursor)).objectSpawn
  return spawn.entityId
}
async function start(client, target) {
  const cursor = client.packets.length
  client.send({ playerAction: { mapClick: { ...fixture.target, targetEntityId: target, button: 2 } } })
  await client.wait(packet => packet.contextMenu?.actions?.some(action => action.actionId === binding.source.id), cursor)
  client.send({ playerAction: { selectContextAction: { entityId: target, actionId: binding.source.id } } })
  return (await client.wait(packet => packet.characterActionAnimation?.entityId === client.id && packet.characterActionAnimation.state.animationKey === binding.key, cursor)).characterActionAnimation
}
async function cancel(client) {
  const cursor = client.packets.length
  client.send({ playerAction: { mapClick: { ...fixture.origin } } })
  return (await client.wait(packet => packet.characterActionAnimation?.entityId === client.id && !packet.characterActionAnimation.state.animationKey, cursor)).characterActionAnimation
}
try {
  const actor = await connect('performer'), observer = await connect('observer')
  const offset = (Number(actor.id) % 16) * 120
  for (const point of [fixture.origin, fixture.near, fixture.target]) point.x += offset
  await teleport(actor, fixture.origin)
  await teleport(observer, fixture.far)
  const given = await actor.command(`/give ${fixture.equipment_item}`)
  await actor.wait(packet => packet.inventoryUpdate?.updated?.some(inventory => inventory.grid?.items?.some(entry => entry.item.typeId === fixture.equipment_def_id)), given)
  const grid = actor.inventories.get(0), equipment = actor.inventories.get(2)
  const item = grid.grid.items.find(entry => entry.item.typeId === fixture.equipment_def_id).item
  await moveEquipment(actor, item.itemId, grid, equipment, slots[`EQUIP_SLOT_${binding.variants[0].equipment[0].slot.toUpperCase()}`])
  let target = await spawnTarget(actor)
  console.log('Waiting for the fixture target to mature')
  await pause(fixture.target_ready_delay_ms)
  let active = await start(actor, target)
  assert.equal(active.state.totalTicks * active.state.tickDurationMs, binding.preview.duration_ms)
  await pause(350)
  let cursor = observer.packets.length
  await teleport(observer, fixture.near)
  const late = (await observer.wait(packet => packet.objectSpawn?.entityId === actor.id && packet.objectSpawn.actionAnimation?.animationKey === binding.key, cursor)).objectSpawn.actionAnimation
  assert.ok(late.elapsedTicks > active.state.elapsedTicks)
  assert.equal(late.revision, active.state.revision)
  const sameClock = Number(late.serverTimeMs)
  const phase = state => Math.max(0, Math.min(1, ((state.elapsedTicks ?? 0) * state.tickDurationMs + sameClock - Number(state.serverTimeMs)) / (state.totalTicks * state.tickDurationMs)))
  assert.ok(Math.abs(phase(active.state) - phase(late)) <= .06)
  pass('late visibility joins the current phase', { durationMs: late.totalTicks * late.tickDurationMs, lateTicks: late.elapsedTicks, phaseDifference: Math.abs(phase(active.state) - phase(late)) })
  cursor = observer.packets.length
  const stopped = await cancel(actor)
  const observedStop = (await observer.wait(packet => packet.characterActionAnimation?.entityId === actor.id && packet.characterActionAnimation.state.revision === stopped.state.revision, cursor)).characterActionAnimation
  assert.equal(observedStop.state.animationKey ?? '', '')
  assert.equal(observedStop.streamEpoch, observer.epoch)
  pass('cancellation reaches both clients with their own stream epochs')
  cursor = observer.packets.length
  active = await start(actor, target)
  const shared = (await observer.wait(packet => packet.characterActionAnimation?.entityId === actor.id && packet.characterActionAnimation.state.revision === active.state.revision, cursor)).characterActionAnimation
  assert.deepEqual(shared.state, active.state)
  pass('simultaneous observers receive identical authoritative timing')
  cursor = observer.packets.length
  const equipped = actor.inventories.get(2)
  await moveEquipment(actor, item.itemId, equipped, equipped, slots[`EQUIP_SLOT_${binding.variants[1].equipment[0].slot.toUpperCase()}`])
  await observer.wait(packet => packet.characterVisual?.entityId === actor.id, cursor)
  pass('equipment changes reach the observer during the active cycle')
  const continuation = (await actor.wait(packet => packet.characterActionAnimation?.state?.animationKey === binding.key && BigInt(packet.characterActionAnimation.state.revision) > BigInt(active.state.revision), actor.packets.length, 3500)).characterActionAnimation
  assert.equal(continuation.state.elapsedTicks ?? 0, 0)
  pass('only a confirmed successor publishes the next cycle')
  await teleport(observer, fixture.far)
  cursor = observer.packets.length
  await pause(250)
  await teleport(observer, fixture.near)
  const reentry = (await observer.wait(packet => packet.objectSpawn?.entityId === actor.id, cursor)).objectSpawn.actionAnimation
  assert.equal(reentry.animationKey, binding.key)
  pass('visibility exit and re-entry restore the fresh cycle', { revision: reentry.revision, elapsedTicks: reentry.elapsedTicks })
  cursor = actor.packets.length
  const destroy = await observer.command('/destroy')
  await observer.wait(packet => packet.chat?.text?.includes('Click an object'), destroy)
  observer.send({ playerAction: { mapClick: { ...fixture.target, targetEntityId: target } } })
  await actor.wait(packet => packet.characterActionAnimation?.entityId === actor.id && !packet.characterActionAnimation.state.animationKey, cursor)
  pass('target destruction publishes idle to the performer')
  target = await spawnTarget(actor)
  active = await start(actor, target)
  cursor = observer.packets.length
  const transferred = await teleport(actor, fixture.origin, 1)
  assert.notEqual(transferred.actionAnimation.generation, active.state.generation)
  assert.equal(transferred.actionAnimation.animationKey ?? '', '')
  await observer.wait(packet => packet.objectDespawn?.entityId === actor.id, cursor)
  const returned = await teleport(actor, fixture.origin, 0)
  assert.equal(returned.actionAnimation.animationKey ?? '', '')
  pass('transfer resets incarnation and animation; old observer receives despawn')
  console.log(JSON.stringify({ status: 'passed', checks: evidence }, null, 2))
} finally {
  for (const client of clients) client.socket.close()
}
