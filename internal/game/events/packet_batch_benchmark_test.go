package events

import (
	"fmt"
	"slices"
	"testing"

	_const "origin/internal/const"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"google.golang.org/protobuf/proto"
)

type movementFanoutFixture struct {
	entries        []ecs.MoveBatchEntry
	visibleIndices [][]int
}

type movementFanoutCounts struct {
	messages int
	bytes    int
	entries  int
}

var movementFanoutSink movementFanoutCounts

func newMovementFanoutFixture(entityCount, observers, visiblePerObserver int) movementFanoutFixture {
	fixture := movementFanoutFixture{entries: make([]ecs.MoveBatchEntry, entityCount), visibleIndices: make([][]int, observers)}
	for index := range fixture.entries {
		fixture.entries[index] = ecs.MoveBatchEntry{
			EntityID: types.EntityID(index + 1), X: 1000 + index*10, Y: 2000 + index*5, Heading: 0.75,
			VelocityX: 10, VelocityY: 5, MoveMode: _const.Walk, IsMoving: true,
			ServerTimeMs: 1800000000000, MoveSeq: 50,
		}
	}
	for observer := range fixture.visibleIndices {
		fixture.visibleIndices[observer] = make([]int, visiblePerObserver)
		for offset := range fixture.visibleIndices[observer] {
			fixture.visibleIndices[observer][offset] = (observer*visiblePerObserver + offset) % entityCount
		}
		slices.Sort(fixture.visibleIndices[observer])
	}
	return fixture
}

// Both paths use identical authored entries and observer subsets. This isolates
// serialization and fanout bookkeeping; it intentionally excludes socket I/O.
func serializeMovementFanout(fixture movementFanoutFixture, mode string) (movementFanoutCounts, error) {
	var counts movementFanoutCounts
	if mode == "baseline" {
		encodedEntries := make([][]byte, len(fixture.entries))
		for index := range fixture.entries {
			encoded, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMove{
				ObjectMove: buildObjectMove(&fixture.entries[index]),
			}})
			if err != nil {
				return counts, err
			}
			encodedEntries[index] = encoded
		}
		for _, indices := range fixture.visibleIndices {
			for _, index := range indices {
				counts.messages++
				counts.bytes += len(encodedEntries[index])
				counts.entries++
			}
		}
		return counts, nil
	}
	encoder := newObjectMoveEncoder(fixture.entries)
	var visibleMoves []*netproto.S2C_ObjectMove
	if mode == "per_recipient" {
		visibleMoves = make([]*netproto.S2C_ObjectMove, 0, len(encoder.moves))
	}
	for _, indices := range fixture.visibleIndices {
		var encoded []byte
		var err error
		if mode == "per_recipient" {
			visibleMoves = visibleMoves[:0]
			for _, index := range indices {
				visibleMoves = append(visibleMoves, encoder.moves[index])
			}
			encoded, err = marshalObjectMoves(visibleMoves)
		} else {
			encoded, err = encoder.marshalVisible(indices)
		}
		if err != nil {
			return counts, err
		}
		if len(encoded) > 0 {
			counts.messages++
			counts.bytes += len(encoded)
			counts.entries += len(indices)
		}
	}
	return counts, nil
}

func BenchmarkMovementBatchSerialization(b *testing.B) {
	for _, scenario := range []struct {
		name               string
		observers, visible int
	}{
		{"one_observer_100", 1, 100},
		{"sparse_10_observers_10_each", 10, 10},
		{"dense_100_observers_100_each", 100, 100},
		{"dense_100_observers_99_each", 100, 99},
	} {
		fixture := newMovementFanoutFixture(100, scenario.observers, scenario.visible)
		for _, name := range []string{"baseline", "per_recipient", "batch"} {
			b.Run(fmt.Sprintf("%s/%s", scenario.name, name), func(b *testing.B) {
				counts, err := serializeMovementFanout(fixture, name)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					movementFanoutSink, err = serializeMovementFanout(fixture, name)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(counts.messages), "messages/op")
				b.ReportMetric(float64(counts.bytes), "payload-B/op")
				b.ReportMetric(float64(counts.entries), "entries/op")
			})
		}
	}
}
