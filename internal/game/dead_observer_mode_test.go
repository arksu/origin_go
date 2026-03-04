package game

import (
	"testing"

	netproto "origin/internal/network/proto"
)

func TestIsPayloadAllowedWhileDeadObserver(t *testing.T) {
	if !isPayloadAllowedWhileDeadObserver(&netproto.ClientMessage_Ping{}) {
		t.Fatalf("expected ping payload to be allowed in dead observer mode")
	}
	if isPayloadAllowedWhileDeadObserver(&netproto.ClientMessage_Chat{}) {
		t.Fatalf("expected non-ping payload to be blocked in dead observer mode")
	}
}

func TestIsDeadObserverDeadlineExpired(t *testing.T) {
	if isDeadObserverDeadlineExpired(0, 100) {
		t.Fatalf("deadline=0 should not expire")
	}
	if isDeadObserverDeadlineExpired(101, 100) {
		t.Fatalf("future deadline should not expire")
	}
	if !isDeadObserverDeadlineExpired(100, 100) {
		t.Fatalf("deadline should expire at exact tick")
	}
	if !isDeadObserverDeadlineExpired(100, 101) {
		t.Fatalf("past deadline should expire")
	}
}
