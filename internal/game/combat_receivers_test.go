package game

import (
	"math"
	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
	"testing"
)

type combatTestChunks map[types.ChunkCoord]*core.Chunk

func (chunks combatTestChunks) GetChunk(coord types.ChunkCoord) *core.Chunk { return chunks[coord] }

type testCombatReceiver struct {
	hp, armor float64
	hits      int
}

func (receiver *testCombatReceiver) Eligible() bool          { return receiver.hp > 0 }
func (receiver *testCombatReceiver) Armor() (float64, error) { return receiver.armor, nil }
func (receiver *testCombatReceiver) ApplyHit(hit CombatHit) error {
	receiver.hp = math.Max(0, receiver.hp-hit.Damage)
	receiver.hits++
	return nil
}

func TestCombatBroadPhaseAndDispatch(t *testing.T) {
	world := ecs.NewWorldForTesting()
	chunks := combatTestChunks{}
	for _, coord := range []types.ChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}} {
		chunks[coord] = core.NewChunk(coord, 0, 0, constt.ChunkSize)
	}
	receivers := NewCombatReceivers(world, chunks)
	origin := combat.Point{X: constt.ChunkWorldSize - 25, Y: 50}
	targetX := origin.X + 50
	target := world.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: targetX, Y: origin.Y})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 40, HalfHeight: 2})
	})
	receiver := &testCombatReceiver{hp: 1}
	if err := receivers.Register(target, receiver); err != nil {
		t.Fatal(err)
	}
	for _, chunk := range chunks {
		chunk.Spatial().AddStatic(target, int(targetX), int(origin.Y))
	}
	contacts, err := receivers.Contacts(1, combat.Sector{Origin: origin, Direction: combat.Point{X: 1}, Range: 18, AngleDegrees: 90}, false)
	if err != nil || len(contacts) != 1 || contacts[0].ID != 2 || contacts[0].Distance != 10 {
		t.Fatalf("large cross-chunk collider missed/doubled: %+v %v", contacts, err)
	}
	hit := CombatHit{EventSequence: 1, ExecutionID: 1, AttackerID: 1, AttackerIncarnation: 1, TargetID: 2, TargetIncarnation: target, RawDamage: 0.4}
	if _, applied, err := receivers.Apply(hit); err != nil || !applied {
		t.Fatalf("apply: %v %v", applied, err)
	}
	if receiver.hp != 0.6 || receiver.hits != 1 {
		t.Fatalf("fractional HP lost: %+v", receiver)
	}
	if _, applied, err := receivers.Apply(hit); err != nil || applied {
		t.Fatal("duplicate event applied")
	}
	hit.EventSequence = 2
	hit.RawDamage = math.NaN()
	if _, _, err := receivers.Apply(hit); err == nil || receiver.hits != 1 {
		t.Fatal("invalid hit changed receiver")
	}
	hit.RawDamage = 1
	world.Despawn(target)
	replacement := world.Spawn(2, nil)
	if replacement == target {
		t.Fatal("expected new incarnation")
	}
	if _, applied, err := receivers.Apply(hit); err != nil || applied || receiver.hits != 1 {
		t.Fatal("old incarnation hit applied")
	}
}
