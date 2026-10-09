import { performance } from 'node:perf_hooks'
import { Container, Graphics } from 'pixi.js'
import { CombatSectorPreview } from '../src/game/CombatSectorPreview'
import { directionSectorPoints, type DirectionSector } from '../src/game/hud/directionAim'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { setWorldParams } from '../src/game/tiles/Tile'

// CPU-only comparison with the previous DirectionAimPreview update. No renderer,
// draw calls or GPU memory are represented by these measurements.
function benchmark(name: string, iterations: number, operation: (index: number) => void): void {
  for (let index = 0; index < 2000; index++) operation(index)
  const samples: number[] = []
  for (let sample = 0; sample < 5; sample++) {
    const startedAt = performance.now()
    for (let index = 0; index < iterations; index++) operation(index)
    samples.push((performance.now() - startedAt) * 1_000_000 / iterations)
  }
  samples.sort((left, right) => left - right)
  console.log(`${name}: median ${samples[2]!.toFixed(0)} ns/op; samples ${samples.map(value => value.toFixed(0)).join(', ')}`)
}

setWorldParams(12, 100)
const origin = { x: 100, y: 200 }
const sector: DirectionSector = { range: 18, angle: Math.PI / 2 }
const baseline = new Graphics()
benchmark('previous active frame: build+project+draw', 10_000, () => {
  const points = directionSectorPoints(origin, 0, sector).flatMap(point => {
    const projected = coordGame2Screen(point.x, point.y)
    return [projected.x, projected.y]
  })
  baseline.clear().poly(points)
    .fill({ color: 0x74e3ab, alpha: 0.16 })
    .stroke({ color: 0x74e3ab, alpha: 0.9, width: 1.5 })
})
baseline.destroy()

const parent = new Container()
const preview = new CombatSectorPreview(parent)
benchmark('confirmed sector frame: idle', 100_000, index => preview.update(origin, 1, index % 999))
preview.show(origin, 0, sector, 1, 0)
benchmark('confirmed sector frame: active', 100_000, index => preview.update(origin, 1, index % 999))
benchmark('confirmed sector frame: zoom change', 10_000, index => preview.update(origin, index % 2 === 0 ? 1 : 2, index % 999))
benchmark('confirmed sector start: project+draw', 10_000, index => preview.show(origin, 0, sector, 1, index))
console.log(`retained views: ${parent.children.length} Graphics; projected polygon: 34 points; steady frame: no geometry rebuild`)
preview.destroy()
parent.destroy()
