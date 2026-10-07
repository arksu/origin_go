package ecs

import (
	"testing"

	"origin/internal/types"
)

// These benchmarks use the same Mark/Drain operations before and after queue
// preparation was introduced. Warm-up remains outside the measured hit path.
func BenchmarkCharacterVisualDirtyQueue(b *testing.B) {
	b.Run("FreshMarkDrain", func(b *testing.B) {
		world := NewWorldWithCapacity(1, nil, 0)
		queue := GetResource[CharacterVisualDirtyQueue](world)
		handle := types.MakeHandle(1, 1)
		buffer := make([]types.Handle, 0, 1)
		queue.Mark(handle)
		buffer = queue.Drain(1, buffer[:0])
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			queue.Mark(handle)
			buffer = queue.Drain(1, buffer[:0])
		}
	})
	b.Run("PendingMark", func(b *testing.B) {
		world := NewWorldWithCapacity(1, nil, 0)
		queue := GetResource[CharacterVisualDirtyQueue](world)
		handle := types.MakeHandle(1, 1)
		queue.Mark(handle)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			queue.Mark(handle)
		}
	})
	b.Run("FreshMarkDrain64", func(b *testing.B) {
		world := NewWorldWithCapacity(64, nil, 0)
		queue := GetResource[CharacterVisualDirtyQueue](world)
		var handles [64]types.Handle
		buffer := make([]types.Handle, 0, len(handles))
		for i := range handles {
			handles[i] = types.MakeHandle(uint32(i+1), 1)
			queue.Mark(handles[i])
		}
		buffer = queue.Drain(0, buffer[:0])
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, handle := range handles {
				queue.Mark(handle)
			}
			buffer = queue.Drain(0, buffer[:0])
		}
	})
}
