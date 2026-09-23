package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/network"
)

func TestEnterWorldQueuesActionSnapshot(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, nil)
	shard := &Shard{serverInbox: network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 16})}
	(&Game{}).enqueuePlayerBootstrapSnapshots(shard, 1, player)
	jobs := shard.ServerInbox().Drain()
	if len(jobs) == 0 {
		t.Fatal("enter world queued no snapshots")
	}
	last := jobs[len(jobs)-1]
	payload, ok := last.Payload.(*network.ActionSnapshotJobPayload)
	if last.JobType != network.JobSendActionSnapshot || last.TargetID != 1 || !ok || payload.Handle != player {
		t.Fatalf("action snapshot missing from enter-world bootstrap: %#v", last)
	}
}
