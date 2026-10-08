package ecs

import (
	"time"

	"origin/internal/types"
)

// LogoutContext is a value snapshot for read-only rules executed under the
// owning shard lock. Rules must recheck live state rather than trust deadlines.
type LogoutContext struct {
	EntityID types.EntityID
	Handle   types.Handle
	Detached DetachedEntity
	Time     TimeState
}

// LogoutDecision carries an advisory runtime duration. A blocked decision with
// no known deadline is checked again periodically, not retained indefinitely.
type LogoutDecision struct {
	Blocked    bool
	RetryAfter time.Duration
}

// LogoutPolicy checks one independent reason to retain a disconnected body.
// Check must only read prepared state and must not allocate or perform I/O.
type LogoutPolicy interface {
	Check(LogoutContext) (LogoutDecision, error)
}
