package network

import (
	"testing"
	"time"

	"origin/internal/types"
)

func TestPlayerCommandInbox_Enqueue(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                10,
		MaxPacketsPerSecond:         40,
		MaxCommandsPerTickPerClient: 5,
	}
	inbox := NewPlayerCommandInbox(config)

	cmd := &PlayerCommand{
		ClientID:    1,
		CharacterID: types.EntityID(100),
		CommandID:   1,
		CommandType: 1,
		ReceivedAt:  time.Now(),
	}

	err := inbox.Enqueue(cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	received, dropped, _ := inbox.Stats()
	if received != 1 {
		t.Errorf("expected received=1, got %d", received)
	}
	if dropped != 0 {
		t.Errorf("expected dropped=0, got %d", dropped)
	}
}

func TestPlayerCommandInbox_Overflow(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                5,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 10,
	}
	inbox := NewPlayerCommandInbox(config)

	// Fill queue to capacity
	for i := 0; i < 5; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		err := inbox.Enqueue(cmd)
		if err != nil {
			t.Fatalf("expected no error on command %d, got %v", i, err)
		}
	}

	// Next command should overflow
	cmd := &PlayerCommand{
		ClientID:    1,
		CharacterID: types.EntityID(100),
		CommandID:   6,
		ReceivedAt:  time.Now(),
	}
	err := inbox.Enqueue(cmd)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
	if _, ok := err.(OverflowError); !ok {
		t.Fatalf("expected OverflowError, got %T", err)
	}

	_, dropped, _ := inbox.Stats()
	if dropped != 1 {
		t.Errorf("expected dropped=1, got %d", dropped)
	}
}

func TestPlayerCommandInbox_Drain(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 100,
	}
	inbox := NewPlayerCommandInbox(config)

	// Enqueue 3 commands
	for i := 0; i < 3; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// Drain should return all commands
	commands := inbox.Drain()
	if len(commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(commands))
	}

	// Second drain should return empty
	commands = inbox.Drain()
	if len(commands) != 0 {
		t.Fatalf("expected 0 commands after second drain, got %d", len(commands))
	}
}

func TestPlayerCommandInbox_Fairness(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 3, // Only 3 per tick per client
	}
	inbox := NewPlayerCommandInbox(config)

	// Enqueue 5 commands from client 1
	for i := 0; i < 5; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// First drain should return only 3 (fairness limit)
	commands := inbox.Drain()
	if len(commands) != 3 {
		t.Fatalf("expected 3 commands (fairness limit), got %d", len(commands))
	}

	// Second drain should return remaining 2
	commands = inbox.Drain()
	if len(commands) != 2 {
		t.Fatalf("expected 2 remaining commands, got %d", len(commands))
	}
}

func TestPlayerCommandInbox_MultiClientFairness(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 2,
	}
	inbox := NewPlayerCommandInbox(config)

	// Enqueue 3 commands from client 1
	for i := 0; i < 3; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// Enqueue 3 commands from client 2
	for i := 0; i < 3; i++ {
		cmd := &PlayerCommand{
			ClientID:    2,
			CharacterID: types.EntityID(200),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// First drain: 2 from client 1 + 2 from client 2 = 4
	commands := inbox.Drain()
	if len(commands) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(commands))
	}

	// Count by client
	client1Count := 0
	client2Count := 0
	for _, cmd := range commands {
		if cmd.ClientID == 1 {
			client1Count++
		} else if cmd.ClientID == 2 {
			client2Count++
		}
	}
	if client1Count != 2 {
		t.Errorf("expected 2 commands from client 1, got %d", client1Count)
	}
	if client2Count != 2 {
		t.Errorf("expected 2 commands from client 2, got %d", client2Count)
	}

	// Second drain: 1 from client 1 + 1 from client 2 = 2
	commands = inbox.Drain()
	if len(commands) != 2 {
		t.Fatalf("expected 2 remaining commands, got %d", len(commands))
	}
}

func TestPlayerCommandInbox_Deduplication(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 100,
	}
	inbox := NewPlayerCommandInbox(config)

	// Mark command 5 as processed
	inbox.MarkProcessed(1, 5)

	// Try to enqueue command with ID <= 5 (should be duplicate)
	cmd := &PlayerCommand{
		ClientID:    1,
		CharacterID: types.EntityID(100),
		CommandID:   5,
		ReceivedAt:  time.Now(),
	}
	err := inbox.Enqueue(cmd)
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
	if _, ok := err.(DuplicateCommandError); !ok {
		t.Fatalf("expected DuplicateCommandError, got %T", err)
	}

	// Command with ID > 5 should work
	cmd = &PlayerCommand{
		ClientID:    1,
		CharacterID: types.EntityID(100),
		CommandID:   6,
		ReceivedAt:  time.Now(),
	}
	err = inbox.Enqueue(cmd)
	if err != nil {
		t.Fatalf("expected no error for new command, got %v", err)
	}
}

func TestPlayerCommandInbox_RemoveClient(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 100,
	}
	inbox := NewPlayerCommandInbox(config)

	// Mark some commands as processed
	inbox.MarkProcessed(1, 10)

	// Remove client
	inbox.RemoveClient(1)

	// Now command 5 should not be duplicate (client state cleared)
	cmd := &PlayerCommand{
		ClientID:    1,
		CharacterID: types.EntityID(100),
		CommandID:   5,
		ReceivedAt:  time.Now(),
	}
	err := inbox.Enqueue(cmd)
	if err != nil {
		t.Fatalf("expected no error after client removal, got %v", err)
	}
}

func TestServerJobInbox_Basic(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                10,
		MaxPacketsPerSecond:         40,
		MaxCommandsPerTickPerClient: 5,
	}
	inbox := NewServerJobInbox(config)

	job := &ServerJob{
		JobType:   1,
		TargetID:  types.EntityID(100),
		CreatedAt: time.Now(),
	}

	err := inbox.Enqueue(job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	jobs := inbox.Drain()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	received, dropped, processed := inbox.Stats()
	if received != 1 {
		t.Errorf("expected received=1, got %d", received)
	}
	if dropped != 0 {
		t.Errorf("expected dropped=0, got %d", dropped)
	}
	if processed != 1 {
		t.Errorf("expected processed=1, got %d", processed)
	}
}

func TestServerJobInbox_Overflow(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                3,
		MaxPacketsPerSecond:         40,
		MaxCommandsPerTickPerClient: 5,
	}
	inbox := NewServerJobInbox(config)

	// Fill to capacity
	for i := 0; i < 3; i++ {
		job := &ServerJob{
			JobType:   1,
			TargetID:  types.EntityID(uint64(i)),
			CreatedAt: time.Now(),
		}
		err := inbox.Enqueue(job)
		if err != nil {
			t.Fatalf("expected no error on job %d, got %v", i, err)
		}
	}

	// Next should overflow
	job := &ServerJob{
		JobType:   1,
		TargetID:  types.EntityID(100),
		CreatedAt: time.Now(),
	}
	err := inbox.Enqueue(job)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
	if _, ok := err.(OverflowError); !ok {
		t.Fatalf("expected OverflowError, got %T", err)
	}
}

func TestPlayerCommandInbox_DoubleBuffer(t *testing.T) {
	config := CommandQueueConfig{
		MaxQueueSize:                100,
		MaxPacketsPerSecond:         100,
		MaxCommandsPerTickPerClient: 100,
	}
	inbox := NewPlayerCommandInbox(config)

	// Enqueue during "tick 1"
	for i := 0; i < 3; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 1),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// Drain for tick 1
	tick1Commands := inbox.Drain()
	if len(tick1Commands) != 3 {
		t.Fatalf("expected 3 commands in tick 1, got %d", len(tick1Commands))
	}

	// Enqueue during "tick 2" (while tick 1 commands are being processed)
	for i := 0; i < 2; i++ {
		cmd := &PlayerCommand{
			ClientID:    1,
			CharacterID: types.EntityID(100),
			CommandID:   uint64(i + 4),
			ReceivedAt:  time.Now(),
		}
		inbox.Enqueue(cmd)
	}

	// Drain for tick 2
	tick2Commands := inbox.Drain()
	if len(tick2Commands) != 2 {
		t.Fatalf("expected 2 commands in tick 2, got %d", len(tick2Commands))
	}

	// Verify command IDs are correct
	for i, cmd := range tick1Commands {
		expected := uint64(i + 1)
		if cmd.CommandID != expected {
			t.Errorf("tick1 command %d: expected ID %d, got %d", i, expected, cmd.CommandID)
		}
	}
	for i, cmd := range tick2Commands {
		expected := uint64(i + 4)
		if cmd.CommandID != expected {
			t.Errorf("tick2 command %d: expected ID %d, got %d", i, expected, cmd.CommandID)
		}
	}
}
