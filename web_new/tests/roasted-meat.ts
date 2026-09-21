import { createApp, h, nextTick } from 'vue'
import { createPinia } from 'pinia'
import CraftWindow from '../src/components/ui/CraftWindow.vue'
import { useGameStore } from '../src/stores/gameStore'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'

const result = document.querySelector<HTMLPreElement>('#result')!
const checks: string[] = []
function check(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message)
  checks.push(`PASS: ${message}`)
}

async function run() {
  const pinia = createPinia()
  const app = createApp({ render: () => h(CraftWindow) })
  app.use(pinia)
  const store = useGameStore(pinia)
  registerMessageHandlers()
  const recipe = {
    craftKey: 'roasted_meat', name: 'Roasted meat', staminaCost: 10, ticksRequired: 10,
    inputs: [{ itemTag: 'raw_meat', count: 1, qualityWeight: 1 }],
    outputs: [{ itemKey: 'roasted_meat', count: 1 }],
    stationRequirements: [{ capability: 'cooking', state: 'burning' }],
    flags: { hasInputs: true, hasOutputSpace: true, hasStamina: true, hasRequiredLinkedObject: true, hasStationRequirements: true, stationRequirementsMet: true, canStartNow: true },
  }
  store.setCraftListSnapshot({ recipes: [recipe] })
  app.mount('#app')
  for (const source of ['beef', 'raw_pork']) {
    // The server resolves the species; the recipe payload remains generic for both inventories.
    messageDispatcher.dispatch(proto.ServerMessage.create({ craftList: { recipes: [recipe] } }))
    await nextTick()
    const inputIcon = document.querySelector<HTMLElement>('.craft-window__chip:not(.craft-window__chip--output) .craft-window__icon')
    const outputIcon = document.querySelector<HTMLElement>('.craft-window__chip--output .craft-window__icon')
    check(inputIcon?.style.backgroundImage.includes('/assets/game/items/raw_meat.png'), `${source}: raw meat consume icon`)
    check(outputIcon?.style.backgroundImage.includes('/assets/game/items/roasted_meat.png'), `${source}: generic roasted meat preview icon`)
    check(document.querySelector('#app')?.textContent?.includes('Any Raw Meat x1'), `${source}: one tagged input displayed`)
    check(document.querySelector('#app')?.textContent?.includes('Roasted Meat x1'), `${source}: one preview output displayed`)
  }
  for (const icon of ['raw_meat', 'roasted_meat']) {
    const image = new Image()
    image.src = `/assets/game/items/${icon}.png`
    await image.decode()
    check(image.naturalWidth > 0, `${icon}: icon asset decodes`)
  }
  const exactMessage = "Roast can't be processed: no info rabbit_meat in roast map"
  const encoded = proto.ServerMessage.encode(proto.ServerMessage.create({ miniAlert: {
    reasonCode: 'CRAFT_ROAST_MAP_ENTRY_MISSING', message: exactMessage,
    severity: proto.AlertSeverity.ALERT_SEVERITY_ERROR, ttlMs: 60000,
  } })).finish()
  messageDispatcher.dispatch(proto.ServerMessage.decode(encoded))
  check(store.miniAlerts.at(-1)?.message === exactMessage, 'network alert preserves exact text and underscores')
  messageDispatcher.dispatch(proto.ServerMessage.create({ miniAlert: {
    reasonCode: 'CRAFT_NO_SPACE', severity: proto.AlertSeverity.ALERT_SEVERITY_WARNING, ttlMs: 60000,
  } }))
  check(store.miniAlerts.at(-1)?.message === 'Craft No Space', 'legacy alert retains reason-code fallback')
  store.setCraftListSnapshot({ recipes: [{ ...recipe, flags: { ...recipe.flags, stationRequirementsMet: false, canStartNow: false } }] })
  await nextTick()
  const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('#app button'))
  const craftButtons = buttons.filter(button => /craft (one|all)/i.test(button.textContent || ''))
  check(craftButtons.length === 2 && craftButtons.every(button => button.disabled), 'unlit station disables both craft buttons')
  result.textContent = checks.join('\n')
  document.title = 'PASS: Roasted meat craft smoke test'
}

run().catch(error => {
  result.textContent = `${checks.join('\n')}\nFAIL: ${String(error)}`
  document.title = 'FAIL: Roasted meat craft smoke test'
  console.error(error)
})
