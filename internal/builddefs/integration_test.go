package builddefs_test

import (
	"path/filepath"
	"testing"

	"origin/internal/builddefs"
	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadAllBuilds_RegistersCampfireForOneBranch(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	previousObjects := objectdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})

	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)

	builds, err := builddefs.LoadFromDirectory(filepath.Join("..", "..", "data", "builds"), zap.NewNop())
	require.NoError(t, err)

	campfire, ok := objects.GetByKey("campfire")
	require.True(t, ok)
	require.Equal(t, 16, campfire.DefID)

	build, ok := builds.GetByKey("campfire")
	require.True(t, ok)
	require.Equal(t, "campfire", build.ObjectKey)
	require.Equal(t, []builddefs.BuildInput{{ItemKey: "branch", Count: 1, QualityWeight: 1}}, build.Inputs)
}

func TestLoadAllBuilds_RegistersHearthFireWithConfiguredBranches(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})

	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	builds, err := builddefs.LoadFromDirectory(filepath.Join("..", "..", "data", "builds"), zap.NewNop())
	require.NoError(t, err)

	hearth, found := objects.GetByKey("hearth_fire")
	require.True(t, found)
	require.Equal(t, 36, hearth.DefID)
	build, found := builds.GetByKey("hearth_fire")
	require.True(t, found)
	require.Equal(t, 6, build.DefID)
	require.NotEqual(t, hearth.DefID, build.DefID)
	require.Equal(t, "Hearth Fire", build.Name)
	require.Equal(t, "hearth_fire", build.ObjectKey)
	require.Len(t, build.Inputs, 1)
	require.Equal(t, "branch", build.Inputs[0].ItemKey)
	require.Positive(t, build.Inputs[0].Count)
	require.Equal(t, uint32(1), build.Inputs[0].QualityWeight)
	require.Equal(t, uint32(20), build.TicksRequired)
	require.Equal(t, 8.0, build.StaminaCost)
	require.Empty(t, build.RequiredSkills)
	require.Empty(t, build.RequiredDiscovery)
	byID, found := builds.GetByID(build.DefID)
	require.True(t, found)
	require.Same(t, build, byID)
}
