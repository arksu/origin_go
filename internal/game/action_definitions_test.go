package game

import (
	"go.uber.org/zap"
	"origin/internal/actiondefs"
	"origin/internal/itemdefs"
	"path/filepath"
	"testing"
)

func loadProductionActionsForTest(t *testing.T) *actiondefs.Registry {
	t.Helper()
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "items"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	itemdefs.SetGlobalForTesting(items)
	actions, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return actions
}
