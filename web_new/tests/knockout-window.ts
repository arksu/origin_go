import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { createPinia } from 'pinia'
import KnockoutWindow from '../src/components/ui/KnockoutWindow.vue'
import GameWindow from '../src/components/ui/GameWindow.vue'
import { useGameStore } from '../src/stores/gameStore'
import { useActionCooldownStore } from '../src/stores/actionCooldownStore'
import { gameConnection } from '../src/network/GameConnection'
import { timeSync } from '../src/network/TimeSync'

// A disposable storage adapter reports window reads/writes without accessing
// browser profile data. Old KO coordinates deliberately disagree with the center.
const reads = ref(0), writes = ref(0), commands = ref(0), escapes = ref(0), clicks = ref(0), keys = ref(0)
const lastMessage = ref('none')
const stored = new Map([['wnd_-60_left', '1'], ['wnd_-60_top', '2']])
Object.defineProperty(window, 'localStorage', { value: {
  getItem(key: string) { if (key.startsWith('wnd_-60')) reads.value++; return stored.get(key) ?? null },
  setItem(key: string, value: string) { if (key.startsWith('wnd_-60')) writes.value++; stored.set(key, value) },
} })
gameConnection.send = packet => { lastMessage.value = JSON.stringify(packet); if (packet.playerAction?.standUp?.streamEpoch === 7) commands.value++ }
const pinia = createPinia()
createApp(defineComponent({ setup() {
  const game = useGameStore()
  const clock = useActionCooldownStore()
  const width = ref(900), height = ref(600), ordinary = ref(true)
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(17, 'KO fixture', 12, 4, 7, true)
  function stats(knockedOut: boolean, lying: boolean, canStand: boolean, deadline = clock.serverNow() + 60000) {
    game.setPlayerStats({ isKnockedOut: knockedOut, isLying: lying, canStandUp: canStand, koUntilMs: knockedOut ? deadline : 0 })
  }
  timeSync.reset(); clock.setSnapshot({ serverTimeMs: Date.now() })
  window.addEventListener('keydown', event => {
    if (event.key === 'Escape') { ordinary.value = false; escapes.value++ }
    if (event.code === 'KeyW' && !(event.target instanceof HTMLInputElement)) keys.value++
  })
  nextTick(() => { document.querySelector<HTMLInputElement>('#behind-input')?.focus(); stats(true, true, false) })
  const control = (label: string, action: () => void) => h('button', { onClick: action }, label)
  return () => h('main', [
    h('nav', [
      control('Local timer expired', () => stats(true, true, false, clock.serverNow() - 1)),
      control('Server completed KO', () => stats(false, true, true)),
      control('Stun', () => stats(false, true, false)),
      control('Confirm standing', () => stats(false, false, false)),
      control('New KO', () => { game.setDeathDialog(null); stats(true, true, false) }),
      control('Resize', () => { width.value = 650; height.value = 450 }),
      control('Touch drag', () => {
        const header = document.querySelector<HTMLElement>('#ko-layer .header')
        if (!header) return
        const position = header.getBoundingClientRect()
        const touch = (x: number, y: number) => new Touch({ identifier: 17, target: header, clientX: x, clientY: y })
        header.dispatchEvent(new TouchEvent('touchstart', { touches: [touch(position.x + 40, position.y + 10)], bubbles: true, cancelable: true }))
        document.dispatchEvent(new TouchEvent('touchmove', { touches: [touch(position.x + 100, position.y + 30)], bubbles: true, cancelable: true }))
        document.dispatchEvent(new TouchEvent('touchend', { touches: [], bubbles: true }))
      }),
      control('Death', () => game.setDeathDialog({ title: 'Death', message: 'You have died' })),
    ]),
    h('output', `commands=${commands.value}; storage reads=${reads.value}; writes=${writes.value}; escapes=${escapes.value}; clicks=${clicks.value}; keys=${keys.value}; epoch=${game.worldParams?.streamEpoch}; inGame=${game.isInGame}; last=${lastMessage.value}`),
    h('section', { id: 'game', style: { position: 'relative', width: `${width.value}px`, height: `${height.value}px`, background: '#23372b', color: '#eee' } }, [
      h('input', { id: 'behind-input', 'aria-label': 'Chat behind the window', style: { position: 'absolute', left: '12px', bottom: '12px' } }),
      h('button', { style: { position: 'absolute', right: '12px', bottom: '12px' }, onClick: () => clicks.value++ }, 'Click behind the window'),
      ordinary.value ? h('div', { style: { position: 'absolute', inset: '0', zIndex: 300, pointerEvents: 'none' } }, [h(GameWindow, { id: 12, title: 'Inventory', innerWidth: 180, innerHeight: 70, onClose: () => { ordinary.value = false } }, () => 'Viewing is allowed')]) : null,
      !game.deathDialog && (game.playerStats.isKnockedOut || game.playerStats.isLying) ? h('div', { id: 'ko-layer', style: { position: 'absolute', inset: '0', zIndex: 900, pointerEvents: 'none' } }, [h(KnockoutWindow)]) : null,
      game.deathDialog ? h('div', { style: { position: 'absolute', inset: '0', zIndex: 1000, background: '#171717' } }, game.deathDialog.message) : null,
    ]),
  ])
} })).use(pinia).mount('#app')
