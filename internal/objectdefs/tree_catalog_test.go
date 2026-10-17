package objectdefs_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"origin/internal/game/behaviors"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAdditionalTreeDefinitions(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "objects"), behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)

	for _, test := range []struct {
		key, template, resource string
		treeID, stumpID         int
	}{
		{"autumn_oak", "oak", "autumn_oak", 37, 38},
		{"mulberry", "elm", "mberry", 39, 40},
	} {
		t.Run(test.key, func(t *testing.T) {
			tree, ok := objects.GetByKey("tree_" + test.key)
			require.True(t, ok)
			stump, ok := objects.GetByKey("stump_" + test.key)
			require.True(t, ok)
			template, ok := objects.GetByKey("tree_" + test.template)
			require.True(t, ok)
			templateStump, ok := objects.GetByKey("stump_" + test.template)
			require.True(t, ok)
			require.Equal(t, test.treeID, tree.DefID)
			require.Equal(t, test.stumpID, stump.DefID)
			require.Equal(t, template.HP, tree.HP)
			require.Equal(t, template.Components, tree.Components)
			require.Equal(t, templateStump.HP, stump.HP)
			require.Equal(t, templateStump.Components, stump.Components)
			require.Equal(t, "trees/"+test.resource+"/stump", stump.Resource)
			require.Equal(t, template.TreeConfig.Priority, tree.TreeConfig.Priority)
			require.Len(t, tree.TreeConfig.Stages, len(template.TreeConfig.Stages))
			for index, expected := range template.TreeConfig.Stages {
				if expected.TransformToDefKey != "" {
					expected.TransformToDefKey = stump.Key
				}
				require.Equal(t, expected, tree.TreeConfig.Stages[index], "stage %d", index+1)
				stage := index + 1
				if test.key == "mulberry" {
					stage = len(tree.TreeConfig.Stages)
				}
				require.Equal(t, fmt.Sprintf("trees/%s/%d", test.resource, stage),
					objectdefs.ResolveAppearanceResource(tree, []string{fmt.Sprintf("tree.stage%d", index+1)}))
			}
		})
	}
}
