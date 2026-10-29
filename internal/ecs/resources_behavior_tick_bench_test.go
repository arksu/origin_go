package ecs

import (
	"fmt"
	"testing"

	"origin/internal/types"
)

func BenchmarkBehaviorTickScheduleRearm(b *testing.B) {
	for _, population := range []int{1, 200, 10000} {
		b.Run(fmt.Sprintf("population_%d", population), func(b *testing.B) {
			schedule := &BehaviorTickSchedule{}
			for entityID := types.EntityID(1); entityID <= types.EntityID(population); entityID++ {
				schedule.Schedule(entityID, "npc", 0)
			}
			batch := make([]BehaviorTickKey, 0, min(population, 200))
			b.ReportAllocs()
			b.ResetTimer()
			for tick := 0; tick < b.N; tick++ {
				batch = schedule.PopDueForExecution(uint64(tick), cap(batch), batch[:0])
				for _, key := range batch {
					schedule.Schedule(key.EntityID, key.BehaviorKey, uint64(tick+1))
					schedule.CompleteTick(key)
				}
			}
		})
	}
}
