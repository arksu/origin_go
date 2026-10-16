import assert from 'node:assert/strict'
import { after, beforeEach, test } from 'node:test'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import { compileScript, parse } from '@vue/compiler-sfc'
import { createRenderer, markRaw, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

const clientRoot = fileURLToPath(new URL('..', import.meta.url))
const temporaryRoot = join(clientRoot, 'node_modules', '.tmp')
await mkdir(temporaryRoot, { recursive: true })
const directory = await mkdtemp(join(temporaryRoot, 'render-init-ui-'))
after(() => rm(directory, { recursive: true, force: true }))
const outfile = join(directory, 'views.mjs')

await build({
  stdin: {
    contents: `
      export { default as CharactersView } from '@/views/CharactersView.vue'
      export { default as GameView } from '@/views/GameView.vue'
      export { useGameStore } from '@/stores/gameStore'
      export { controls } from 'test:controls'
    `,
    resolveDir: clientRoot,
    loader: 'ts',
  },
  outfile, bundle: true, platform: 'node', format: 'esm', external: ['vue', 'pinia', 'axios'],
  alias: { '@': join(clientRoot, 'src') },
  define: { 'import.meta.env': '{}', '__APP_VERSION__': '"test"', '__BUILD_TIME__': '"test"', '__COMMIT_HASH__': '"test"' },
  plugins: [{
    name: 'view-boundaries',
    setup(builder) {
      const modules = {
        'test:controls': `
          export const controls = { calls: [], route: { query: {} }, init: async () => {}, list: async () => [], enter: async () => ({ auth_token: 'token' }) }
        `,
        'vue-router': `
          import { controls } from 'test:controls'
          const router = Object.fromEntries(['push', 'replace'].map(name => [name, target => {
            controls.calls.push({ name, target })
            return Promise.resolve()
          }]))
          export const useRouter = () => router
          export const useRoute = () => controls.route
        `,
        '@/api/characters': `
          import { controls } from 'test:controls'
          export const listCharacters = () => controls.list()
          export const enterCharacter = id => controls.enter(id)
          export const createCharacter = async () => {}
          export const deleteCharacter = async () => {}
        `,
        '@/game': `
          import { controls } from 'test:controls'
          let ready = false
          export const gameFacade = {
            async init() {
              controls.calls.push({ name: 'init' })
              await controls.init()
              ready = true
              controls.calls.push({ name: 'ready' })
            },
            destroy() { ready = false; controls.calls.push({ name: 'destroy' }) },
            isInitialized: () => ready,
            setActorRenderSettings() {}, setRenderDebugEnabled() {}, setEconomyRendering() {},
            setActionCursor() {}, setKeyboardMovementEnabled() {}, onPlayerClick() {},
            cancelBuildGhost() {}, cancelLiftGhost() {},
          }
        `,
        '@/network': `
          import { controls } from 'test:controls'
          export const connectToGame = token => controls.calls.push({ name: 'connect', token })
          export const disconnectFromGame = () => controls.calls.push({ name: 'disconnect' })
          export const sendActivateAction = actionId => controls.calls.push({ name: 'activateAction', actionId })
          export const sendCancelAction = () => controls.calls.push({ name: 'cancelAction' })
          ${['sendChatMessage', 'sendOpenWindow', 'sendCloseWindow', 'sendStartBuild', 'sendBuildProgress',
            'sendBuildTakeBack', 'sendInventoryOp']
            .map(name => `export const ${name} = () => {}`).join('\n')}
        `,
      }
      builder.onResolve({ filter: /^(test:controls|vue-router|@\/api\/characters|@\/game|@\/network)$/ }, args => ({
        path: args.path, namespace: 'view-test',
      }))
      builder.onLoad({ filter: /.*/, namespace: 'view-test' }, args => ({ contents: modules[args.path] }))
      builder.onLoad({ filter: /\.vue$/ }, async args => {
        if (!/\/(views\/(GameView|CharactersView)|components\/ui\/App(Alert|Button|Input|Spinner))\.vue$/.test(args.path)) {
          return { contents: 'export default { render: () => null }' }
        }
        const { descriptor } = parse(await readFile(args.path, 'utf8'), { filename: args.path })
        const script = compileScript(descriptor, { id: 'render-init-ui-test', inlineTemplate: true })
        return { contents: script.content, loader: 'ts', resolveDir: dirname(args.path) }
      })
    },
  }],
})
const { CharactersView, GameView, useGameStore, controls } = await import(pathToFileURL(outfile).href)

function node(type, text = '') {
  return markRaw({ type, text, props: {}, style: {}, children: [], parent: null, focus() {} })
}
const renderer = createRenderer({
  createElement: node,
  createText: text => node('#text', text),
  createComment: text => node('#comment', text),
  setText: (target, text) => { target.text = text },
  setElementText: (target, text) => { target.text = text; target.children = [] },
  patchProp: (target, key, previous, value) => { target.props[key] = value },
  parentNode: target => target.parent,
  nextSibling: target => target.parent?.children[target.parent.children.indexOf(target) + 1] ?? null,
  insert(target, parent, anchor = null) {
    if (target.parent) target.parent.children.splice(target.parent.children.indexOf(target), 1)
    target.parent = parent
    const index = anchor ? parent.children.indexOf(anchor) : -1
    parent.children.splice(index < 0 ? parent.children.length : index, 0, target)
  },
  remove(target) {
    if (target.parent) target.parent.children.splice(target.parent.children.indexOf(target), 1)
    target.parent = null
  },
})
const descendants = target => [target, ...target.children.flatMap(descendants)]
const textContent = target => [target.text, ...target.children.map(textContent)].join(' ')
const alerts = root => descendants(root).filter(target => String(target.props.class).includes('app-alert--error'))
const button = (root, label) => descendants(root).find(target => target.type === 'button' && textContent(target).trim() === label)

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
async function flush() {
  await new Promise(resolve => setImmediate(resolve))
  await nextTick()
}
function mount(component, context) {
  const root = node('root')
  const app = renderer.createApp(component)
  const errors = []
  app.config.errorHandler = error => errors.push(error)
  app.mount(root)
  context.after(() => { app.unmount(); assert.deepEqual(errors, []) })
  return { root, app }
}

const previousGlobals = ['window', 'document', 'navigator', 'localStorage', 'sessionStorage'].map(
  name => [name, Object.getOwnPropertyDescriptor(globalThis, name)],
)
after(() => {
  for (const [name, previous] of previousGlobals) {
    if (previous) Object.defineProperty(globalThis, name, previous)
    else Reflect.deleteProperty(globalThis, name)
  }
})

beforeEach(() => {
  const storage = new Map()
  const browserStorage = {
    getItem: key => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: key => storage.delete(key),
  }
  const values = {
    window: Object.assign(new EventTarget(), {
      matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
    }),
    document: new EventTarget(),
    navigator: { maxTouchPoints: 0, userAgent: 'test' },
    localStorage: browserStorage,
    sessionStorage: browserStorage,
  }
  for (const [name, value] of Object.entries(values)) {
    Object.defineProperty(globalThis, name, { configurable: true, value })
  }
  setActivePinia(createPinia())
  controls.calls.length = 0
  controls.route.query = {}
  controls.init = async () => {}
  controls.list = async () => [{ id: 1, name: 'Survivor' }]
  controls.enter = async () => ({ auth_token: 'token' })
})

const message = 'Unable to initialize WebGL 2. Enable hardware acceleration or try another browser.'

test('renderer failure returns to characters with its message and never opens the connection', async context => {
  const store = useGameStore()
  store.setGameSession('token', 1)
  const original = new Error(message, { cause: new Error('No available renderer') })
  controls.init = async () => { throw original }
  const diagnostic = context.mock.method(console, 'error', () => {})
  const { app } = mount(GameView, context)
  await flush()
  assert.equal(store.connectionError.code, 'RENDER_INIT_FAILED')
  assert.equal(store.connectionError.message, message)
  assert.ok(diagnostic.mock.calls.some(call => call.arguments[1] === original))
  assert.deepEqual(controls.calls.find(call => call.name === 'replace').target, {
    path: '/characters', query: { disconnectReason: message },
  })
  assert.equal(controls.calls.some(call => call.name === 'connect'), false)
  app.unmount()
  assert.deepEqual(controls.calls.slice(-2).map(call => call.name), ['disconnect', 'destroy'])
})

test('successful rendering finishes before opening the connection', async context => {
  const store = useGameStore()
  store.setGameSession('token', 1)
  const pending = deferred()
  controls.init = () => pending.promise
  mount(GameView, context)
  await flush()
  assert.deepEqual(controls.calls.map(call => call.name), ['init'])
  pending.resolve()
  await flush()
  assert.deepEqual(controls.calls.map(call => call.name), ['init', 'ready', 'connect'])
  assert.equal(controls.calls.at(-1).token, 'token')
})

test('leaving during renderer initialization prevents a late connection and destroys the renderer', async context => {
  useGameStore().setGameSession('token', 1)
  const pending = deferred()
  controls.init = () => pending.promise
  const { app } = mount(GameView, context)
  await flush()
  app.unmount()
  pending.resolve()
  await flush()
  assert.equal(controls.calls.some(call => call.name === 'connect'), false)
  assert.equal(controls.calls.at(-1).name, 'destroy')
})

test('number hotkeys cancel Hearth return for another cooling-down action without activating it', async context => {
  const previous = ['HTMLInputElement', 'HTMLTextAreaElement', 'requestAnimationFrame', 'cancelAnimationFrame'].map(
    name => [name, Object.getOwnPropertyDescriptor(globalThis, name)],
  )
  for (const [name, value] of Object.entries({
    HTMLInputElement: class {}, HTMLTextAreaElement: class {},
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
  })) Object.defineProperty(globalThis, name, { configurable: true, value })
  context.after(() => {
    useGameStore().reset()
    for (const [name, descriptor] of previous) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else Reflect.deleteProperty(globalThis, name)
    }
  })
  const hearthId = 'travel_to_hearth_fire'
  const store = useGameStore()
  store.setGameSession('token', 1)
  localStorage.setItem('hotbar_assignments_v1:anonymous', JSON.stringify([
    'game:lift', `game:${hearthId}`, 'game:retired', 'inventory', null, null, null, null, null, null,
  ]))
  mount(GameView, context)
  await flush()
  store.setGameActionList([{ id: 'lift' }, { id: hearthId }])
  const cooldowns = [
    { actionId: 'lift', startedAtMs: 10000, expiresAtMs: 20000 },
    { actionId: hearthId, startedAtMs: 10000, expiresAtMs: 20000 },
  ]
  store.setGameActionState({ actionId: hearthId, phase: 'executing', serverTimeMs: 10000, cooldowns })
  const state = store.gameActionState
  const press = key => document.dispatchEvent(Object.assign(new Event('keydown', { cancelable: true }), { key }))
  controls.calls.length = 0
  press('1')
  assert.deepEqual(controls.calls, [{ name: 'cancelAction' }])
  assert.equal(store.gameActionState, state)
  controls.calls.length = 0
  for (const key of ['2', '3', '4', '0']) press(key)
  assert.equal(store.playerInventoryVisible, true)
  assert.deepEqual(controls.calls, [], 'same action, unknown and empty slots, and personal windows preserve return')
  store.setGameActionState({ actionId: 'lift_down', phase: 'executing', serverTimeMs: 10000, cooldowns })
  press('1')
  assert.deepEqual(controls.calls, [], 'cooldown rejection preserves unrelated active actions')
})

test('return message survives loading, API failure and retry, while the API error is separate', async context => {
  controls.route.query = { disconnectReason: message, preserved: 'value' }
  let pending = deferred()
  controls.list = () => pending.promise
  const { root } = mount(CharactersView, context)
  await flush()
  assert.ok(alerts(root).some(target => textContent(target).includes(message)))
  assert.deepEqual(controls.calls.find(call => call.name === 'replace').target, { query: { preserved: 'value' } })

  pending.reject(new Error('API unavailable'))
  await flush()
  assert.equal(alerts(root).length, 2)
  assert.ok(alerts(root).some(target => textContent(target).includes(message)))
  assert.ok(alerts(root).some(target => textContent(target).includes('Failed to load characters')))

  pending = deferred()
  button(root, 'Retry').props.onClick()
  await flush()
  assert.equal(alerts(root).length, 1)
  assert.ok(textContent(alerts(root)[0]).includes(message))
  pending.resolve([{ id: 1, name: 'Survivor' }])
  await flush()
  assert.ok(textContent(alerts(root)[0]).includes(message))

  const entry = descendants(root).find(target => target.props.onClick && textContent(target).trim() === 'Survivor')
  entry.props.onClick()
  await flush()
  assert.equal(alerts(root).length, 0)
  assert.ok(controls.calls.some(call => call.name === 'push' && call.target === '/game'))
})

test('starting character creation clears the old graphics error', async context => {
  controls.route.query = { disconnectReason: message }
  const { root } = mount(CharactersView, context)
  await flush()
  const create = descendants(root).find(target => target.props.onClick && textContent(target).trim() === 'Create New')
  create.props.onClick()
  await flush()
  const input = descendants(root).find(target => target.type === 'input')
  input.props.onInput({ target: { value: 'New survivor' } })
  await nextTick()
  const form = descendants(root).find(target => target.type === 'form')
  form.props.onSubmit({ preventDefault() {} })
  await flush()
  assert.equal(alerts(root).length, 0)
})
