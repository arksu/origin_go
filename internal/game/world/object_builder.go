package world

import (
	"errors"
)

var (
	ErrEntitySpawnFailed = errors.New("entity spawn failed")
	ErrEntityNotFound    = errors.New("entity not found")
	ErrDefNotFound       = errors.New("object definition not found")
)
