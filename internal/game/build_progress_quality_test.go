package game

import (
	"testing"

	"origin/internal/builddefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestComputeCompletedCampfireQualityUsesAllBranchSlots(t *testing.T) {
	service := &BuildService{}
	quality, ok := service.computeCompletedBuildObjectQuality(
		&builddefs.BuildDef{ObjectKey: "campfire"},
		&components.BuildBehaviorState{Items: []components.BuildRequiredItemState{
			{ItemKey: "branch", BuildCount: 1, BuildQualityTotal: 3},
			{ItemKey: "branch", BuildCount: 1, BuildQualityTotal: 8},
			{ItemKey: "stone", BuildCount: 4, BuildQualityTotal: 400},
		}},
	)
	require.True(t, ok)
	require.Equal(t, uint32(5), quality)
}

func TestComputeCompletedCampfireQualityWeightsBranchCount(t *testing.T) {
	service := &BuildService{}
	quality, ok := service.computeCompletedBuildObjectQuality(
		&builddefs.BuildDef{ObjectKey: "campfire"},
		&components.BuildBehaviorState{Items: []components.BuildRequiredItemState{
			{ItemKey: "branch", BuildCount: 2, BuildQualityTotal: 8},
			{ItemKey: "branch", BuildCount: 1, BuildQualityTotal: 7},
		}},
	)
	require.True(t, ok)
	require.Equal(t, uint32(5), quality)
}

func TestComputeCompletedCampfireQualityLeavesMissingProvenanceUnchanged(t *testing.T) {
	quality, ok := (&BuildService{}).computeCompletedBuildObjectQuality(
		&builddefs.BuildDef{ObjectKey: "campfire"},
		&components.BuildBehaviorState{Items: []components.BuildRequiredItemState{{ItemKey: "stone", BuildCount: 1, BuildQualityTotal: 9}}},
	)
	require.False(t, ok)
	require.Zero(t, quality)
}

func TestCompletedCampfireQualityPersistsAndLegacyQualityRemainsZero(t *testing.T) {
	previousObjects := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects) })
	campfireDef := objectdefs.ObjectDef{DefID: 16, Key: "campfire"}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{campfireDef}))

	tests := []struct {
		name        string
		buildState  *components.BuildBehaviorState
		wantQuality uint32
	}{
		{
			name: "completed campfire",
			buildState: &components.BuildBehaviorState{Items: []components.BuildRequiredItemState{
				{ItemKey: "branch", BuildCount: 2, BuildQualityTotal: 8},
				{ItemKey: "branch", BuildCount: 1, BuildQualityTotal: 7},
			}},
			wantQuality: 5,
		},
		{
			name:        "legacy campfire",
			buildState:  &components.BuildBehaviorState{},
			wantQuality: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			targetID := types.EntityID(200)
			h := w.Spawn(targetID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: constt.BuildObjectTypeID, Quality: 0, Region: 1})
				ecs.AddComponent(w, h, components.Transform{})
				state := components.ObjectInternalState{}
				components.SetBehaviorState(&state, buildBehaviorStateKey, test.buildState)
				ecs.AddComponent(w, h, state)
			})

			(&BuildService{}).transformCompletedBuildTarget(w, targetID, h, &builddefs.BuildDef{ObjectKey: "campfire"}, test.buildState, &campfireDef)
			info, ok := ecs.GetComponent[components.EntityInfo](w, h)
			require.True(t, ok)
			require.Equal(t, test.wantQuality, info.Quality)
			ecs.AddComponent(w, h, components.ChunkRef{})

			raw, err := (&gameworld.ObjectFactory{}).Serialize(w, h)
			require.NoError(t, err)
			restoredWorld := ecs.NewWorldForTesting()
			restored, err := (&gameworld.ObjectFactory{}).Build(restoredWorld, raw, nil)
			require.NoError(t, err)
			restoredInfo, ok := ecs.GetComponent[components.EntityInfo](restoredWorld, restored)
			require.True(t, ok)
			require.Equal(t, test.wantQuality, restoredInfo.Quality)
		})
	}
}
