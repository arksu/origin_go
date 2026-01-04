package game

import "errors"

var (
	ErrEntitySpawnFailed = errors.New("failed to spawn entity")
	ErrEntityNotFound    = errors.New("entity not found")
	ErrChunkNotFound     = errors.New("chunk not found")
	ErrChunkNotActive    = errors.New("chunk is not active")
	ErrChunkNotLoaded    = errors.New("chunk is not loaded")
	ErrInvalidState      = errors.New("invalid chunk state")
	ErrObjectNotFound    = errors.New("object not found")

	ErrBuilderNotFound  = errors.New("object builder not found")
	ErrEntityNotInChunk = errors.New("entity not in chunk")
)
