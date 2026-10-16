package game

import (
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

type PlayerTransferCause uint8

const (
	PlayerTransferCauseAdminTeleport PlayerTransferCause = iota + 1
	PlayerTransferCauseStairs
	PlayerTransferCauseHearthReturn
)

type PlayerTransferRequest struct {
	PlayerID types.EntityID

	SourceLayer int
	TargetLayer int
	TargetX     int
	TargetY     int

	IgnoreObjectCollision bool
	Cause                 PlayerTransferCause

	// Optional feature policies run at existing transfer stages. SourcePolicy
	// replaces the default incapacity check and runs before saving/capture, then
	// again after SaveSync. OnResult runs once under the result shard lock; success
	// means an authoritative target body exists, before attachment/bootstrap.
	// SpawnTarget forwards its finalizer to the successful locked spawn, so
	// participant restoration, completion and attachment precede the next update.
	SourcePolicy func(*Shard, types.Handle, *network.Client) error
	SpawnTarget  func(*Shard, repository.Character, playerRuntimeState, playerSpawnFinalizer) (types.Handle, repository.Character, error)
	OnResult     func(*Shard, types.Handle, error)
}

type PlayerTransferSnapshot struct {
	Client       *network.Client
	SourceLayer  int
	SourceX      int
	SourceY      int
	Character    repository.Character
	RuntimeState playerRuntimeState

	ParticipantStates map[string]any
}

type PlayerTransferParticipant interface {
	Key() string
	CaptureSource(g *Game, sourceShard *Shard, req PlayerTransferRequest, playerHandle types.Handle) (any, error)
	RestoreTarget(g *Game, targetShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any) error
	RestoreSourceRollback(g *Game, sourceShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any) error
	OnTargetRestoreFailure(g *Game, targetShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any, restoreErr error)
}
