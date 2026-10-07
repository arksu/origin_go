package ecs

import (
	"slices"
	"testing"

	"origin/internal/types"
)

func TestCharacterVisualDirtyQueuePreparationAndFIFO(t *testing.T) {
	var queue CharacterVisualDirtyQueue
	one, two, three := types.MakeHandle(1, 1), types.MakeHandle(2, 1), types.MakeHandle(3, 1)
	if queue.Prepare(types.InvalidHandle) || queue.IsPrepared(types.InvalidHandle) || queue.Mark(types.InvalidHandle) {
		t.Fatal("invalid handle must not be registered or enqueued")
	}
	if !queue.Prepare(one) || !queue.Prepare(one) || !queue.IsPrepared(one) {
		t.Fatal("preparation must be idempotent")
	}
	if queue.PendingCount() != 0 || len(queue.records) != 1 {
		t.Fatal("preparation must not mark a handle dirty")
	}
	if !queue.Mark(one) || queue.Mark(one) || !queue.Mark(two) || !queue.Mark(three) {
		t.Fatal("mark must lazily register and deduplicate handles")
	}
	if !queue.IsPrepared(two) || queue.PendingCount() != 3 {
		t.Fatal("incorrect lazy registration or pending count")
	}
	buffer := make([]types.Handle, 1, 4)
	buffer[0] = types.MakeHandle(9, 1)
	buffer = queue.Drain(2, buffer)
	if !slices.Equal(buffer, []types.Handle{types.MakeHandle(9, 1), one, two}) || queue.PendingCount() != 1 {
		t.Fatalf("partial drain must append in FIFO order: %v", buffer)
	}
	if !queue.IsPrepared(one) || !queue.IsPrepared(two) {
		t.Fatal("drain must retain registrations")
	}
	if !queue.Mark(one) {
		t.Fatal("drained handle must be enqueued again")
	}
	buffer = queue.Drain(-1, buffer[:0])
	if !slices.Equal(buffer, []types.Handle{three, one}) || queue.PendingCount() != 0 {
		t.Fatalf("re-mark must append to the tail: %v", buffer)
	}
	if queue.head != nil || queue.tail != nil {
		t.Fatal("fully drained queue must not retain pending links")
	}
	for _, record := range queue.records {
		if record.queued || record.prev != nil || record.next != nil {
			t.Fatal("drained records must be unlinked")
		}
	}
}

func TestCharacterVisualDirtyQueueForgetUnlinksAnyPosition(t *testing.T) {
	for _, forgotten := range []int{0, 1, 2} {
		t.Run([]string{"head", "middle", "tail"}[forgotten], func(t *testing.T) {
			var queue CharacterVisualDirtyQueue
			handles := []types.Handle{types.MakeHandle(1, 1), types.MakeHandle(2, 1), types.MakeHandle(3, 1)}
			for _, handle := range handles {
				queue.Mark(handle)
			}
			if !queue.Forget(handles[forgotten]) || queue.Forget(handles[forgotten]) || queue.IsPrepared(handles[forgotten]) {
				t.Fatal("forget must delete a registration exactly once")
			}
			if queue.PendingCount() != 2 || len(queue.records) != 2 {
				t.Fatal("forget must remove the actual pending record")
			}
			expected := append(slices.Clone(handles[:forgotten]), handles[forgotten+1:]...)
			if got := queue.Drain(0, nil); !slices.Equal(got, expected) {
				t.Fatalf("forget changed surviving FIFO order: %v", got)
			}
			queue.Mark(handles[forgotten])
			if got := queue.Drain(0, nil); !slices.Equal(got, []types.Handle{handles[forgotten]}) {
				t.Fatalf("forgotten handle must support a new registration: %v", got)
			}
			if !queue.Forget(handles[forgotten]) || queue.head != nil || queue.tail != nil {
				t.Fatal("forget of a drained record must preserve an empty queue")
			}
		})
	}
}

func TestCharacterVisualDirtyQueueForgetsOnDespawn(t *testing.T) {
	world := NewWorldWithCapacity(1, nil, 0)
	queue := GetResource[CharacterVisualDirtyQueue](world)
	for i := 0; i < 100; i++ {
		id := types.EntityID(i + 1)
		handle := world.Spawn(id, nil)
		if handle == types.InvalidHandle {
			t.Fatal("could not spawn handle for generation reuse test")
		}
		MarkCharacterVisualDirty(world, id)
		if !queue.IsPrepared(handle) || queue.PendingCount() != 1 {
			t.Fatal("visual mark did not register a live entity")
		}
		if !world.Despawn(handle) {
			t.Fatal("despawn failed")
		}
		if queue.IsPrepared(handle) || queue.PendingCount() != 0 || len(queue.records) != 0 {
			t.Fatal("despawn must remove pending and persistent records")
		}
		MarkCharacterVisualDirty(world, id)
		if queue.PendingCount() != 0 {
			t.Fatal("stale identity must not register a dirty handle")
		}
	}
}

func TestCharacterVisualDirtyQueuePreparedOperationsDoNotAllocate(t *testing.T) {
	var queue CharacterVisualDirtyQueue
	var handles [64]types.Handle
	buffer := make([]types.Handle, 0, len(handles))
	for i := range handles {
		handles[i] = types.MakeHandle(uint32(i+1), 1)
		queue.Prepare(handles[i])
	}
	if got := testing.AllocsPerRun(1000, func() {
		for _, handle := range handles {
			queue.Mark(handle)
			queue.Mark(handle)
		}
		buffer = queue.Drain(32, buffer[:0])
		buffer = queue.Drain(0, buffer[:0])
	}); got != 0 {
		t.Fatalf("prepared mark/drain cycles allocated: %g", got)
	}
	if queue.PendingCount() != 0 || len(queue.records) != len(handles) {
		t.Fatal("repeated cycles must retain only prepared registrations")
	}
	if got := testing.AllocsPerRun(1000, func() {
		queue.Mark(types.InvalidHandle)
		queue.Forget(types.InvalidHandle)
		queue.IsPrepared(types.InvalidHandle)
	}); got != 0 {
		t.Fatalf("invalid handle operations allocated: %g", got)
	}
}
