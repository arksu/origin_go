import { performance } from 'node:perf_hooks'
import { Container } from 'pixi.js'
import { DamageNumberManager } from '../src/game/DamageNumberManager'
import { CAPACITY, DamageNumbersPresentation, type DamageNumberHit } from '../src/game/hud/damageNumbers'

// CPU-only benchmark: it deliberately does not install an atlas or create a
// renderer. Browser rendering, glyph layout and GPU memory are measured in the
// visual fixture rather than presented as Node measurements.
function benchmark(name: string, iterations: number, operation: (index: number) => void): void {
  for (let index = 0; index < Math.min(iterations, 2000); index++) operation(index)
  const samples: number[] = []
  for (let sample = 0; sample < 5; sample++) {
    const startedAt = performance.now()
    for (let index = 0; index < iterations; index++) operation(index)
    samples.push((performance.now() - startedAt) * 1_000_000 / iterations)
  }
  samples.sort((a, b) => a - b)
  console.log(`${name}: median ${samples[2]!.toFixed(0)} ns/op; samples ${samples.map(sample => sample.toFixed(0)).join(', ')}`)
}

for (const count of [0, 1, 10, CAPACITY]) {
  const presentation = new DamageNumbersPresentation()
  for (let id = 1; id <= count; id++) presentation.emit(id, 3.6, id, id, 0)
  benchmark(`presentation.update/${count}`, 10_000, index => presentation.update((index % 500) + 1, 1.5))

  const parent = new Container()
  const manager = new DamageNumberManager(parent)
  const positions = new Map<number, { getContainer(): Container }>()
  const bounds = { top: -30 }
  const hits: DamageNumberHit[] = []
  for (let id = 1; id <= count; id++) {
    const container = { x: id, y: id, getLocalBounds: () => bounds } as unknown as Container
    positions.set(id, { getContainer: () => container })
    hits.push({ targetId: String(id), damage: 3.6 })
  }
  const objects = { getObject: (id: number) => positions.get(id) }
  manager.show(hits, objects, 0, 1.5)
  benchmark(`manager.update/${count}`, 10_000, index => manager.update((index % 500) + 1, 1.5))
  if (count === CAPACITY) {
    benchmark('manager.clear+show/512', 100, () => {
      manager.clear()
      manager.show(hits, objects, 0, 1.5)
    })
  }
  console.log(`pool/${count}: ${manager.presentation.entries.length} fixed records, ${manager.getContainer().children.length} reusable views`)
  manager.destroy()
  parent.destroy()
}

benchmark('presentation.construct', 100, () => { new DamageNumbersPresentation() })
benchmark('manager.construct+destroy', 50, () => {
  const parent = new Container()
  const manager = new DamageNumberManager(parent)
  manager.destroy()
  parent.destroy()
})

function retainedMemory<T>(name: string, create: () => T, cleanup: (value: T) => void): void {
  const gc = (globalThis as typeof globalThis & { gc?: () => void }).gc
  if (!gc) throw new Error('Retained-memory measurements require --expose-gc')
  gc()
  const before = process.memoryUsage().heapUsed
  const instances = Array.from({ length: 20 }, create)
  gc()
  console.log(`${name}: ${(Math.max(0, process.memoryUsage().heapUsed - before) / instances.length / 1024).toFixed(1)} KiB retained JS heap per instance; atlas/GPU excluded`)
  for (const value of instances) cleanup(value)
}

retainedMemory('presentation.memory', () => new DamageNumbersPresentation(), value => value.clear())
retainedMemory('manager.memory', () => new DamageNumberManager(new Container()), value => {
  const parent = value.getContainer().parent!
  value.destroy()
  parent.destroy()
})
