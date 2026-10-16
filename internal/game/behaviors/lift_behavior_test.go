package behaviors

import (
	"path/filepath"
	"testing"

	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLiftBehaviorConfig(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		priority int
		size     *objectdefs.LiftPlacementSize
	}{
		{name: "existing empty config", raw: `{}`, priority: 100},
		{name: "priority", raw: `{"priority":20}`, priority: 20},
		{name: "nonpositive priority default", raw: `{"priority":-1}`, priority: 100},
		{name: "placement size", raw: `{"placementSize":{"w":24,"h":6}}`, priority: 100, size: &objectdefs.LiftPlacementSize{W: 24, H: 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := &objectdefs.ObjectDef{}
			priority, err := (liftBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
				Def: def, RawConfig: []byte(tc.raw),
			})
			require.NoError(t, err)
			require.Equal(t, tc.priority, priority)
			require.Equal(t, &objectdefs.LiftBehaviorConfig{Priority: tc.priority, PlacementSize: tc.size}, def.LiftConfig)
			require.Nil(t, def.Components, "placement size must not add a collider")
		})
	}
}

func TestLiftBehaviorConfigRejectsInvalidPlacementSize(t *testing.T) {
	for _, raw := range []string{
		`{"placementSize":{}}`,
		`{"placementSize":{"w":24}}`,
		`{"placementSize":{"h":6}}`,
		`{"placementSize":{"w":0,"h":6}}`,
		`{"placementSize":{"w":24,"h":-6}}`,
		`{"placementSize":{"w":1e309,"h":6}}`,
		`{"placementSize":{"w":24,"h":1e309}}`,
		`{"placementSize":{"w":NaN,"h":6}}`,
		`{"placementSize":{"w":24,"h":6,"depth":1}}`,
		`{"placementSize":{"w":24,"h":6},"unknown":true}`,
	} {
		t.Run(raw, func(t *testing.T) {
			def := &objectdefs.ObjectDef{}
			_, err := (liftBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
				Def: def, RawConfig: []byte(raw),
			})
			require.Error(t, err)
			require.Nil(t, def.LiftConfig)
		})
	}
}

func TestLiftLogPlacementSizeCatalog(t *testing.T) {
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	previousItems := itemdefs.Global()
	itemdefs.SetGlobalForTesting(items)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "..", "data", "objects"), MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	for key, size := range map[string]objectdefs.LiftPlacementSize{
		"log_x": {W: 24, H: 6},
		"log_y": {W: 6, H: 24},
	} {
		def, exists := objects.GetByKey(key)
		require.True(t, exists)
		require.NotNil(t, def.LiftConfig)
		require.Equal(t, &size, def.LiftConfig.PlacementSize, key)
		require.True(t, def.Components == nil || def.Components.Collider == nil, "%s must remain passable", key)
	}
}
