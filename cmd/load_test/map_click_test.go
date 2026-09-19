package main

import (
	"google.golang.org/protobuf/proto"
	netproto "origin/internal/network/proto"
	"testing"
)

func TestVirtualClientUsesMapClick(t *testing.T) {
	client := NewVirtualClient(nil, nil, nil, nil, nil)
	action := client.moveMsg.GetPlayerAction()
	click := action.GetMapClick()
	if click == nil {
		t.Fatal("load client has no MapClick action")
	}
	click.X, click.Y = 42, -99
	encoded, err := proto.Marshal(&client.moveMsg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded netproto.ClientMessage
	if err := proto.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.GetPlayerAction().GetMapClick()
	if got == nil || got.X != 42 || got.Y != -99 || got.TargetEntityId != 0 {
		t.Fatalf("wrong map click: %v", got)
	}
}
