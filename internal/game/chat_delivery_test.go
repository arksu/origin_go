package game

import (
	"net"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// A Pong queued after chat proves both delivery and non-delivery without waiting
// for a timeout on recipients that should not receive the message.
func drainChatMessages(tb testing.TB, client *network.Client, conn net.Conn) []*netproto.S2C_ChatMessage {
	tb.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(tb, err)
	require.True(tb, client.SendCritical(barrier))
	require.NoError(tb, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var messages []*netproto.S2C_ChatMessage
	for {
		payload, op, err := wsutil.ReadServerData(conn)
		require.NoError(tb, err)
		require.Equal(tb, ws.OpBinary, op)
		message := &netproto.ServerMessage{}
		require.NoError(tb, proto.Unmarshal(payload, message))
		if message.GetPong() != nil {
			return messages
		}
		require.NotNil(tb, message.GetChat())
		messages = append(messages, message.GetChat())
	}
}

func TestLocalChatSpatialRecipientsDeliverPacketsAndReuseBuffer(t *testing.T) {
	f := newAttackResultFixture(t, 8)
	w := f.shard.world
	var err error
	f.shard.soundEvents, err = NewSoundEventService(soundTestProfiles(t), config.DefaultAudioConfig(), f.shard)
	require.NoError(t, err)

	const senderID types.EntityID = 9007199254740993
	const senderName = "Local speaker"
	recipients := []struct {
		id     types.EntityID
		x, y   float64
		epoch  uint32
		handle types.Handle
		client *network.Client
		conn   net.Conn
	}{
		{id: senderID, epoch: 17},
		{id: 2, x: 600, y: 800, epoch: 19}, // Exactly on the inclusive radius.
		{id: 3, x: -300, epoch: 21},
		{id: 4, x: 1000.01, epoch: 23},
	}
	for i := range recipients {
		recipient := &recipients[i]
		recipient.client, recipient.conn = f.connect(t, recipient.id, recipient.epoch)
		f.shard.mu.Lock()
		recipient.handle = w.Spawn(recipient.id, func(w *ecs.World, handle types.Handle) {
			ecs.AddComponent(w, handle, components.Transform{X: recipient.x, Y: recipient.y})
			ecs.AddComponent(w, handle, components.Appearance{Name: proto.String(senderName)})
		})
		ecs.GetResource[ecs.CharacterEntities](w).Map[recipient.id] = ecs.CharacterEntity{Handle: recipient.handle}
		attached := f.shard.soundEvents.Attach(w, recipient.handle, recipient.client.ID, recipient.epoch)
		f.shard.mu.Unlock()
		require.True(t, attached)
	}
	require.Empty(t, ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget,
		"local chat delivery must not require visual visibility")

	queueConfig := network.CommandQueueConfig{MaxQueueSize: 8, MaxPacketsPerSecond: 8, MaxCommandsPerTickPerClient: 8}
	inbox := network.NewPlayerCommandInbox(queueConfig)
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(queueConfig),
		f.shard, f.shard, nil, nil, nil, 1000, f.shard.logger)
	send := func(commandID uint64, text string) {
		t.Helper()
		require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
			ClientID: recipients[0].client.ID, CharacterID: senderID, CommandID: commandID,
			CommandType: network.CmdChat, Layer: f.shard.layer,
			Payload: &network.ChatCommandPayload{Channel: netproto.ChatChannel_CHAT_CHANNEL_LOCAL, Text: text},
		}))
		f.shard.mu.Lock()
		commands.Update(w, 0)
		f.shard.mu.Unlock()
	}
	check := func(text string, expected ...types.EntityID) {
		t.Helper()
		wanted := make(map[types.EntityID]bool, len(expected))
		for _, id := range expected {
			wanted[id] = true
		}
		for _, recipient := range recipients {
			messages := drainChatMessages(t, recipient.client, recipient.conn)
			if !wanted[recipient.id] {
				require.Empty(t, messages, "out-of-radius recipient %d received chat", recipient.id)
				continue
			}
			require.Len(t, messages, 1, "recipient %d must receive exactly one message, including self echo", recipient.id)
			message := messages[0]
			require.Equal(t, netproto.ChatChannel_CHAT_CHANNEL_LOCAL, message.Channel)
			require.Equal(t, uint64(senderID), message.FromEntityId)
			require.Equal(t, senderName, message.FromName)
			require.Equal(t, text, message.Text)
			require.Nil(t, message.ToEntityId)
		}
	}

	send(1, "At the first position")
	check("At the first position", senderID, 2, 3)

	// Reuse the same command system after the sender crosses spatial cells. The
	// second selection is shorter and must drop both old neighbours while adding
	// the formerly out-of-radius client, without retaining old recipient IDs.
	f.shard.mu.Lock()
	ecs.WithComponent(w, recipients[0].handle, func(position *components.Transform) { position.X = 2000 })
	f.shard.soundEvents.OnPositionCommitted(recipients[0].handle, 2000, 0)
	f.shard.mu.Unlock()
	send(2, "At the second position")
	check("At the second position", senderID, 4)
}
