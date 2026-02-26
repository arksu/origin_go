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
)

type PlayerTransferRequest struct {
	PlayerID types.EntityID

	SourceLayer int
	TargetLayer int
	TargetX     int
	TargetY     int

	IgnoreObjectCollision bool
	Cause                 PlayerTransferCause
}

type PlayerTransferSnapshot struct {
	Client      *network.Client
	SourceLayer int
	SourceX     int
	SourceY     int
	Character   repository.Character

	ParticipantStates map[string]any
}

type PlayerTransferParticipant interface {
	Key() string
	CaptureSource(g *Game, sourceShard *Shard, req PlayerTransferRequest, playerHandle types.Handle) (any, error)
	RestoreTarget(g *Game, targetShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any) error
	RestoreSourceRollback(g *Game, sourceShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any) error
	OnTargetRestoreFailure(g *Game, targetShard *Shard, req PlayerTransferRequest, playerHandle types.Handle, state any, restoreErr error)
}

