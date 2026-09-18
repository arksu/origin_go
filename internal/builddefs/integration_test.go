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
