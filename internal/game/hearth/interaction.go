package hearth

import (
	"slices"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// Owner reads the active hearth's copied metadata, including offline owners.
// The caller holds the owning surface shard lock; this never performs I/O.
func (r *Runtime) Owner(target types.Handle) (types.EntityID, string, int64, bool) {
	if r == nil || r.w == nil || r.Index == nil || r.w.Layer != SurfaceLayer || !r.w.Alive(target) {
		return 0, "", 0, false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](r.w, target)
	if !ok || info.Layer != SurfaceLayer || !slices.Contains(info.Behaviors, "hearth") {
		return 0, "", 0, false
	}
	id, ok := r.w.GetExternalID(target)
	if !ok {
		return 0, "", 0, false
	}
	owner, snapshot, found := r.Index.SnapshotByObject(ObjectKey{Region: info.Region, ID: id})
	if !found || !snapshot.CanReturn() {
		return 0, "", 0, false
	}
	return owner, snapshot.Home.OwnerName, snapshot.LastPingUnixMs, true
}

// Break while the old behavior is still present so its normal LinkBroken
// handler closes the window before an in-place conversion preserves the ID.
func (r *Runtime) breakLinksBeforeConversion(targetID types.EntityID) {
	links := ecs.GetResource[ecs.LinkState](r.w)
	for playerID := range links.PlayersByTarget[targetID] {
		if _, _, err := ecs.BreakLinkForPlayer(r.w, playerID, ecs.LinkBreakClosed); err != nil {
			r.logger.Warn("failed to break link before hearth conversion",
				zap.Uint64("player_id", uint64(playerID)), zap.Uint64("target_id", uint64(targetID)), zap.Error(err))
		}
	}
}
