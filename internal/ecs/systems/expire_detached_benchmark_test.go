package systems

import (
	"fmt"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/types"

	"go.uber.org/zap"
)

// The same idle workload measures the original map scan and the due scheduler.
func BenchmarkDetachedExpiryIdle(b *testing.B) {
	for _, count := range []int{1_000, 30_000} {
		b.Run(fmt.Sprintf("players_%d", count), func(b *testing.B) {
			world := ecs.NewWorld(nil, 0)
			clock := ecs.GetResource[ecs.TimeState](world)
			clock.Now = time.Unix(100, 0)
			detached := ecs.GetResource[ecs.DetachedEntities](world)
			for i := 1; i <= count; i++ {
				id := types.EntityID(i)
				handle := world.Spawn(id, nil)
				detached.AddDetachedEntity(id, handle, clock.Now.Add(time.Hour), clock.Now)
			}
			system := NewExpireDetachedSystem(zap.NewNop(), nil, nil, nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				system.Update(world, 0.1)
			}
		})
	}
}
