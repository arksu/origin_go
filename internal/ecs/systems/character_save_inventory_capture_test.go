package systems

import (
	"context"
	"errors"
	"testing"

	"origin/internal/ecs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type strictCharacterInventoryCapture struct {
	err         error
	legacyCalls int
	strictCalls int
}

func (s *strictCharacterInventoryCapture) SerializeInventories(interface{}, types.EntityID, types.Handle) []InventorySnapshot {
	s.legacyCalls++
	return nil
}

func (s *strictCharacterInventoryCapture) SerializeInventoriesStrict(*ecs.World, types.EntityID, types.Handle) ([]InventorySnapshot, error) {
	s.strictCalls++
	return nil, s.err
}

func TestCharacterSaveRejectsInventoryCaptureWithoutReplacingValidPending(t *testing.T) {
	for _, mode := range []string{"save", "detached", "sync"} {
		t.Run(mode, func(t *testing.T) {
			world, player := newCharacterSaveTestPlayer()
			capture := &strictCharacterInventoryCapture{err: errors.New("broken inventory tree")}
			persisted := 0
			saver := newCharacterSaver(0, capture, zap.NewNop(), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
				persisted++
				return nil
			})
			require.True(t, saver.enqueueSnapshot(characterSaveTestSnapshot(10, 1)))
			previous := characterSavePending(t, saver, 10)
			var err error
			switch mode {
			case "sync":
				err = saver.SaveSync(world, 10, player)
			case "detached":
				err = saver.SaveDetached(world, 10, player)
			default:
				err = saver.Save(world, 10, player)
			}
			require.ErrorIs(t, err, capture.err)
			require.Equal(t, 1, capture.strictCalls)
			require.Zero(t, capture.legacyCalls)
			require.Zero(t, persisted)
			require.Same(t, previous, characterSavePending(t, saver, 10))
			require.True(t, world.Alive(player))
			require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
			require.Equal(t, 1, persisted)
			capture.err = nil
			require.NoError(t, saver.Save(world, 10, player))
			require.NotNil(t, characterSavePending(t, saver, 10), "repaired state can be captured normally")
		})
	}
}
