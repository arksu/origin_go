package hearth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

// PrepareChunkActivation resolves saved ownership before any raw object becomes
// visible. The caller holds the surface shard lock and this chunk's I/O gate.
func (r *Runtime) PrepareChunkActivation(chunk *core.Chunk) error {
	if err := r.prepareChunkActivation(chunk); err != nil {
		r.logger.Fatal("hearth chunk preparation failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Runtime) prepareChunkActivation(chunk *core.Chunk) error {
	if chunk == nil || r.w == nil || r.w.Layer != SurfaceLayer || chunk.Layer != SurfaceLayer || r.Index == nil {
		return fmt.Errorf("invalid hearth activation dependencies")
	}
	_, rawObjects := chunk.GetRawActivationObjects()
	if len(rawObjects) == 0 {
		return nil
	}
	defs := objectdefs.Global()
	if defs == nil {
		return fmt.Errorf("hearth definitions are unavailable")
	}
	hearthDef, found := defs.GetByKey("hearth_fire")
	if !found || hearthDef == nil {
		return nil
	}

	params := repository.GetCharacterHearthOwnersParams{HearthTypeID: hearthDef.DefID, SurfaceLayer: SurfaceLayer}
	var candidates []*repository.Object
	var unresolved map[ObjectKey]struct{}
	for _, raw := range rawObjects {
		if raw == nil || raw.DeletedAt.Valid || raw.TypeID != hearthDef.DefID || r.w.Alive(r.w.GetHandleByEntityID(types.EntityID(raw.ID))) {
			continue
		}
		key := ObjectKey{Region: raw.Region, ID: types.EntityID(raw.ID)}
		if raw.Region != chunk.Region || !validHome(Home{Object: key, X: raw.X, Y: raw.Y, Layer: raw.Layer}) {
			return fmt.Errorf("invalid raw hearth %d in region %d", raw.ID, raw.Region)
		}
		if _, _, known := r.Index.SnapshotByObject(key); known {
			continue
		}
		if unresolved == nil {
			unresolved = make(map[ObjectKey]struct{})
		}
		if _, duplicate := unresolved[key]; duplicate {
			return fmt.Errorf("duplicate raw hearth %d in region %d", raw.ID, raw.Region)
		}
		unresolved[key] = struct{}{}
		params.ObjectRegions = append(params.ObjectRegions, raw.Region)
		params.ObjectIds = append(params.ObjectIds, raw.ID)
		candidates = append(candidates, raw)
	}
	if len(candidates) == 0 {
		return nil
	}
	if r.reader == nil {
		return fmt.Errorf("hearth reader is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	rows, err := r.reader.GetCharacterHearthOwners(ctx, params)
	cancel()
	if err != nil {
		return fmt.Errorf("read hearth owners: %w", err)
	}
	characters := make(map[types.EntityID]struct{}, len(rows))
	for _, row := range rows {
		character := types.EntityID(row.CharacterID)
		home := Home{Object: ObjectKey{Region: row.ObjectRegion, ID: types.EntityID(row.ObjectID)}, X: row.X, Y: row.Y, Layer: row.Layer}
		_, requested := unresolved[home.Object]
		_, duplicate := characters[character]
		if !requested || duplicate || !validID(character) || !validHome(home) {
			return fmt.Errorf("invalid hearth owner result for character %d", row.CharacterID)
		}
		delete(unresolved, home.Object)
		characters[character] = struct{}{}
		if err := r.Index.LoadIfUnknown(character, &home); err != nil {
			return fmt.Errorf("restore hearth owner: %w", err)
		}
	}

	var orphans []*repository.Object
	for _, raw := range candidates {
		if _, _, owned := r.Index.SnapshotByObject(ObjectKey{Region: raw.Region, ID: types.EntityID(raw.ID)}); !owned {
			orphans = append(orphans, raw)
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	campfireDef, found := defs.GetByKey("campfire")
	if !found || campfireDef == nil || campfireDef.HP <= 0 || campfireDef.BurnerConfig == nil || campfireDef.Station == nil || campfireDef.Station.InitialState != "unlit" {
		return fmt.Errorf("campfire definition is unavailable")
	}
	if r.saver == nil {
		return fmt.Errorf("hearth persistence is unavailable")
	}
	tick := int64(ecs.GetResource[ecs.TimeState](r.w).Tick)
	for i, raw := range orphans {
		orphans[i] = rawCampfire(raw, campfireDef, tick)
	}
	if err := r.saver.SaveOrphans(context.Background(), orphans); err != nil {
		return fmt.Errorf("save orphan hearth conversions: %w", err)
	}
	for _, raw := range orphans {
		chunk.UpsertRawObject(raw)
	}
	return nil
}

func rawCampfire(raw *repository.Object, def *objectdefs.ObjectDef, tick int64) *repository.Object {
	converted := *raw
	converted.TypeID = def.DefID
	converted.Hp = sql.NullFloat64{Float64: float64(def.HP), Valid: true}
	// Empty saved state restores the ordinary unlit station and burner defaults.
	converted.Data.Valid, converted.Data.RawMessage = false, nil
	converted.LastTick = tick
	return &converted
}
