import { createApp, defineComponent, h, ref } from 'vue'
import { createPinia } from 'pinia'
import '../src/assets/main.scss'
import CustomWindow from '../src/components/ui/CustomWindow.vue'
import { useGameStore } from '../src/stores/gameStore'
import { gameConnection } from '../src/network/GameConnection'
import { setWorldParams } from './serverConstantsFixture'

// Keep the acceptance fixture independent of the user's saved window positions.
Object.defineProperty(window, 'localStorage', { value: { getItem: () => '8', setItem: () => {} } })
const commands = ref(0)
gameConnection.send = () => { commands.value++ }

createApp(defineComponent({ setup() {
  const game = useGameStore()
  game.setConnectionState('connected')
  setWorldParams(12, 4)
  game.setPlayerEnterWorld(1, 'Visitor', 7)
  const hearth = () => game.setCustomWindow({ entityId: 200, streamEpoch: 7, title: 'Hearth Fire', fields: [{ label: 'Owner', value: '<b>Марко & Friends</b>' }] })
  const workbench = () => game.setCustomWindow({ entityId: 201, streamEpoch: 7, title: 'Workbench', fields: [
    { label: 'State', value: 'Idle' },
    { label: 'Recipe', value: 'A very long recipe name with a LongUnbrokenTextThatMustWrapWithinTheWindow' },
    { label: 'Output', value: '2 boards\n1 branch' },
  ] })
  hearth()
  return () => h('main', [
    h('h1', 'Custom windows acceptance'),
    h('nav', [h('button', { onClick: hearth }, 'Open Hearth'), h('button', { onClick: workbench }, 'Open Workbench')]),
    h('output', `Gameplay commands: ${commands.value}`),
    h('div', { class: 'stage' }, game.customWindow
      ? h(CustomWindow, { title: game.customWindow.title, fields: game.customWindow.fields, onClose: game.closeCustomWindow })
      : h('p', 'Window closed')),
  ])
} })).use(createPinia()).mount('#app')

const style = document.createElement('style')
style.textContent = `
body { background: #172414; color: #eee7d1; }
main { padding: 16px; }
h1 { font-size: 20px; }
nav { display: flex; gap: 8px; margin: 12px 0; }
button { background: #3a4930; color: #eee7d1; padding: 6px; }
.stage { position: relative; height: 320px; margin-top: 12px; }
`
document.head.appendChild(style)
