import { Container, Graphics, Text } from 'pixi.js'
import { useGameStore } from '@/stores/gameStore'
import { formatCombatValue } from '@/types/combat'
import { coordGame2Screen } from './utils/coordConvert'
import { cameraController } from './CameraController'
import type { ObjectManager } from './ObjectManager'

export class CombatOverlay {
  private readonly root = new Container()
  private readonly shapes = new Graphics()
  private readonly labels = new Map<string, Text>()
  constructor(parent: Container) {
    this.root.eventMode = 'none'
    this.root.zIndex = 1900000000
    this.root.addChild(this.shapes)
    parent.addChild(this.root)
  }
  update(objects: ObjectManager, pointer: { x: number; y: number } | null, nowMs: number): void {
    const store = useGameStore(), used = new Set<string>()
    this.shapes.clear()
    const zoom = cameraController.getState().zoom
    for (const [id, entity] of store.combat.entities) {
      const numericID = Number(id)
      // Rendering uses the existing world-object registry; never round an ID.
      if (!Number.isSafeInteger(numericID) || String(numericID) !== id) continue
      const object = objects.getObject(numericID)
      if (!object) continue
      const origin = object.getPosition(), state = entity.execution
      let label = ''
      if (state && state.phase !== 'idle') {
        this.sector(origin, state.direction, state.range, state.angle, state.phase === 'windup' ? 0xf6bc4f : 0x62b9ee, zoom)
        const deadline = state.phase === 'windup' ? state.strikeAtMs : state.recoveryEndMs
        label = state.phase === 'windup' ? 'Windup' : 'Recovery'
        label += ' ' + (Math.max(0, deadline - nowMs) / 1000).toFixed(1) + 's'
      }
      if (entity.target) {
        label = (entity.target.depleted ? 'Depleted · ' : '') + formatCombatValue(entity.target.hp) + ' / ' + formatCombatValue(entity.target.maxHp)
        const bounds = store.entities.get(numericID)?.size
        if (bounds) {
          const points = [[-1,-1],[1,-1],[1,1],[-1,1]].flatMap(([x,y]) => {
            const point = coordGame2Screen(origin.x + x! * bounds.x / 2, origin.y + y! * bounds.y / 2)
            return [point.x, point.y]
          })
          this.shapes.poly(points).stroke({ color: entity.target.depleted ? 0x777777 : 0xec6666, width: 1 / zoom, alpha: .75 })
        }
      }
      if (entity.feedback && nowMs - entity.feedback.timeMs < 1200) label += (label ? '\n' : '') + entity.feedback.text
      if (label) {
        used.add(id)
        let text = this.labels.get(id)
        if (!text) {
          text = new Text({ text: '', style: { fontFamily: 'sans-serif', fontSize: 12, fill: 0xffffff, stroke: { color: 0x151b22, width: 3 }, align: 'center' } })
          text.anchor.set(.5, 1); this.labels.set(id, text); this.root.addChild(text)
        }
        text.text = label
        const position = coordGame2Screen(origin.x, origin.y)
        text.position.set(position.x, position.y - 22 / zoom); text.scale.set(1 / zoom)
      }
    }
    for (const [id, label] of this.labels) if (!used.has(id)) { label.destroy(); this.labels.delete(id) }
    const selected = store.gameActionState
    if (selected.phase === 'selecting' && pointer && store.worldParams?.combatSupported && store.playerEntityId != null) {
      const action = store.gameActions.find(action => action.id === selected.actionId)
      const origin = objects.getObject(store.playerEntityId)?.getPosition()
      if (origin && action?.combat && selected.combatRange) {
        const dx = pointer.x - origin.x, dy = pointer.y - origin.y, length = Math.hypot(dx, dy)
        if (length > 0) this.sector(origin, { x: dx / length, y: dy / length }, selected.combatRange, action.combat.sectorAngleDegrees ?? 90, 0x74e3ab, zoom)
      }
    }
  }
  private sector(origin: { x: number; y: number }, direction: { x: number; y: number }, range: number, angle: number, color: number, zoom: number): void {
    const center = coordGame2Screen(origin.x, origin.y), points = [center.x, center.y]
    const start = Math.atan2(direction.y, direction.x) - angle * Math.PI / 360
    for (let index = 0; index <= 32; index++) {
      const radians = start + angle * Math.PI / 180 * index / 32
      const point = coordGame2Screen(origin.x + Math.cos(radians) * range, origin.y + Math.sin(radians) * range)
      points.push(point.x, point.y)
    }
    this.shapes.poly(points).fill({ color, alpha: .16 }).stroke({ color, width: 1.5 / zoom, alpha: .9 })
  }
  clear(): void { this.shapes.clear(); for (const label of this.labels.values()) label.destroy(); this.labels.clear() }
  destroy(): void { this.clear(); this.root.destroy({ children: true }) }
}
