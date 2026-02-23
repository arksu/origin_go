package game

import (
	"origin/internal/builddefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

func (s *Shard) sendBuildListSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	if s == nil || w == nil || entityID == 0 || handle == types.InvalidHandle || !w.Alive(handle) {
		return
	}
	list := &netproto.S2C_BuildList{
		Builds: s.buildVisibleBuildList(w, handle),
	}
	s.SendBuildList(entityID, list)
}

func (s *Shard) buildVisibleBuildList(w *ecs.World, playerHandle types.Handle) []*netproto.BuildRecipeEntry {
	reg := builddefs.Global()
	if reg == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return nil
	}

	all := reg.All()
	if len(all) == 0 {
		return nil
	}

	out := make([]*netproto.BuildRecipeEntry, 0, len(all))
	for _, build := range all {
		if !isBuildVisibleForPlayer(w, playerHandle, build) {
			continue
		}

		inputs := make([]*netproto.BuildInputDef, 0, len(build.Inputs))
		for _, in := range build.Inputs {
			entry := &netproto.BuildInputDef{
				Count:         in.Count,
				QualityWeight: in.QualityWeight,
			}
			if in.ItemKey != "" {
				entry.ItemKey = &in.ItemKey
			}
			if in.ItemTag != "" {
				entry.ItemTag = &in.ItemTag
			}
			inputs = append(inputs, entry)
		}

		allowedTiles := make([]uint32, 0, len(build.AllowedTiles))
		for _, tileID := range build.AllowedTiles {
			allowedTiles = append(allowedTiles, uint32(tileID))
		}
		disallowedTiles := make([]uint32, 0, len(build.DisallowedTiles))
		for _, tileID := range build.DisallowedTiles {
			disallowedTiles = append(disallowedTiles, uint32(tileID))
		}

		out = append(out, &netproto.BuildRecipeEntry{
			BuildKey:           build.Key,
			Name:               build.Name,
			Inputs:             inputs,
			StaminaCost:        build.StaminaCost,
			TicksRequired:      build.TicksRequired,
			RequiredSkills:     append([]string(nil), build.RequiredSkills...),
			RequiredDiscovery:  append([]string(nil), build.RequiredDiscovery...),
			AllowedTiles:       allowedTiles,
			DisallowedTiles:    disallowedTiles,
			ObjectKey:          build.ObjectKey,
			ObjectResourcePath: resolveBuildObjectResourcePath(build.ObjectKey),
		})
	}
	return out
}

func resolveBuildObjectResourcePath(objectKey string) string {
	key := objectKey
	if key == "" {
		return ""
	}
	reg := objectdefs.Global()
	if reg == nil {
		return ""
	}
	def, ok := reg.GetByKey(key)
	if !ok || def == nil {
		return ""
	}
	return def.Resource
}

func isBuildVisibleForPlayer(w *ecs.World, playerHandle types.Handle, build *builddefs.BuildDef) bool {
	if w == nil || playerHandle == types.InvalidHandle || build == nil {
		return false
	}
	profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, playerHandle)
	if !hasProfile {
		return false
	}
	if !containsAllStrings(profile.Skills, build.RequiredSkills) {
		return false
	}
	if !containsAllStrings(profile.Discovery, build.RequiredDiscovery) {
		return false
	}
	return true
}
