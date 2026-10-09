package systems

import (
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExpireDetachedRetriesReservedInventoryOwner(t *testing.T) {
	w := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.Now = time.Unix(100, 0)
	clock.TickPeriod = 100 * time.Millisecond
	owner := w.Spawn(10, nil)
	detached := ecs.GetResource[ecs.DetachedEntities](w)
	detached.AddDetachedEntity(10, owner, clock.Now, clock.Now)
	require.True(t, ecs.ReserveInventoryOwner(w, 10, owner))
	cleanups := 0
	system := NewExpireDetachedSystem(zap.NewNop(), nil, func(types.EntityID, types.Handle) { cleanups++ }, nil)
	system.Update(w, 0)
	require.True(t, w.Alive(owner))
	require.Zero(t, cleanups)
	require.Equal(t, 1, detached.PendingCheckCount())
	require.True(t, ecs.ReleaseInventoryOwner(w, 10, owner))
	clock.Now = clock.Now.Add(time.Second - time.Nanosecond)
	system.Update(w, 0)
	require.True(t, w.Alive(owner))
	clock.Now = clock.Now.Add(time.Nanosecond)
	system.Update(w, 0)
	require.False(t, w.Alive(owner))
	require.Equal(t, 1, cleanups)
}
