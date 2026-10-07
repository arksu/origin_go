package ecs

import (
	"fmt"
	"testing"

	"origin/internal/types"
)

var playerStatsBenchmarkDue []types.EntityID

func BenchmarkPlayerStatsDirty(b *testing.B) {
	for _, population := range []int{1, 128} {
		b.Run(fmt.Sprintf("players_%d", population), func(b *testing.B) {
			newState := func() *EntityStatsUpdateState {
				state := GetResource[EntityStatsUpdateState](NewWorldForTesting())
				// The same benchmark runs against the original scheduler, which
				// has no explicit preparation, and the prepared scheduler.
				preparer, prepared := any(state).(interface {
					PreparePlayer(types.EntityID, types.Handle) bool
				})
				for index := 1; index <= population; index++ {
					id := types.EntityID(index)
					if prepared && !preparer.PreparePlayer(id, types.MakeHandle(uint32(index), 1)) {
						b.Fatal("failed to prepare player")
					}
					if index > 1 {
						state.MarkPlayerDirty(id, 10_000+int64(index), 0)
					}
				}
				return state
			}
			b.Run("fresh_mark_drain", func(b *testing.B) {
				state := newState()
				due := make([]types.EntityID, 0, 1)
				state.MarkPlayerDirty(1, 1000, 0)
				due = state.PopDuePlayerStatsPush(1000, due[:0])
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					state.MarkPlayerDirty(1, 1000, 0)
					due = state.PopDuePlayerStatsPush(1000, due[:0])
				}
				playerStatsBenchmarkDue = due
			})
			b.Run("already_pending", func(b *testing.B) {
				state := newState()
				state.MarkPlayerDirty(1, 1000, 0)
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					state.MarkPlayerDirty(1, 1000, 0)
				}
			})
		})
	}
}
