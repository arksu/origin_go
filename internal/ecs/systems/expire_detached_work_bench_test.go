package systems

import (
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/types"

	"go.uber.org/zap"
)

// Character capture is measured independently by BenchmarkCharacterSaveCapture;
// this isolates queue checks, lifecycle hooks and despawn from serialization.
func BenchmarkDetachedExpiryCleanup(b *testing.B) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = time.Unix(100, 0)
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	cleaned := 0
	system := NewExpireDetachedSystem(zap.NewNop(), nil,
		func(types.EntityID, types.Handle) { cleaned++ }, nil)
	b.ReportAllocs()
	b.ReportMetric(256, "players/op")
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		for id := types.EntityID(1); id <= 256; id++ {
			handle := world.Spawn(id, nil)
			detached.AddDetachedEntity(id, handle, clock.Now, clock.Now)
		}
		b.StartTimer()
		system.Update(world, .1)
	}
	if cleaned != b.N*256 {
		b.Fatalf("cleaned %d players, expected %d", cleaned, b.N*256)
	}
}
