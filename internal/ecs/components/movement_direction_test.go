package components

import (
	"testing"
	"time"

	constt "origin/internal/const"
)

func TestDirectionOwnershipTransitions(t *testing.T) {
	for _, supersede := range []struct {
		name   string
		apply  func(*Movement)
		target constt.TargetType
	}{
		{"clear", func(m *Movement) { m.ClearTarget() }, constt.TargetNone},
		{"point", func(m *Movement) { m.SetTargetPoint(10, 20) }, constt.TargetPoint},
		{"entity", func(m *Movement) { m.SetTargetHandle(123, 10, 20) }, constt.TargetEntity},
	} {
		t.Run(supersede.name, func(t *testing.T) {
			m := Movement{}
			m.ResetDirectionSession(7, 8)
			m.SetDirection(1, 0, 9, time.Unix(100, 0))
			if m.HasReachedTarget(0, 0) {
				t.Fatal("direction has a point destination")
			}
			supersede.apply(&m)
			if m.TargetType != supersede.target || m.Direction.Revision != 9 || m.Direction.ClientID != 7 || m.Direction.StreamEpoch != 8 || !m.Direction.ExpiresAt.IsZero() || !m.Direction.UpdatePending {
				t.Fatalf("invalid supersession: %+v", m)
			}
			before := m
			m.ReleaseDirection()
			if m != before {
				t.Fatal("release modified another owner")
			}
			m.SetDirection(0, 1, 10, time.Unix(101, 0))
			if m.TargetType != constt.TargetDirection || m.TargetHandle != 0 || m.TargetX != 0 || m.TargetY != 0 || m.PointStopPending {
				t.Fatalf("stale destination: %+v", m)
			}
			m.ResetDirectionSession(11, 12)
			if m.TargetType != constt.TargetNone || m.Direction.Revision != 0 || m.Direction.ClientID != 11 || m.Direction.StreamEpoch != 12 {
				t.Fatalf("invalid reset: %+v", m)
			}
		})
	}
}

func TestDirectionReleasePreservesStun(t *testing.T) {
	m := Movement{}
	m.SetDirection(1, 0, 1, time.Unix(100, 0))
	m.State = constt.StateStunned
	m.ReleaseDirection()
	if m.State != constt.StateStunned || m.TargetType != constt.TargetNone || m.Direction.Revision != 1 {
		t.Fatalf("stun or watermark lost: %+v", m)
	}
}
