package systems

import (
	"math"
	"strconv"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type chatSelectorFunc func(*ecs.World, float64, float64, float64, float64, []types.EntityID) []types.EntityID

func (f chatSelectorFunc) AppendLocalChatRecipients(w *ecs.World, x, y, radius, radiusSq float64, dst []types.EntityID) []types.EntityID {
	return f(w, x, y, radius, radiusSq, dst)
}

type capturedChat struct {
	ids      []types.EntityID
	channel  netproto.ChatChannel
	senderID types.EntityID
	name     string
	text     string
}

type chatDeliveryRecorder struct{ messages []capturedChat }

func (*chatDeliveryRecorder) SendChatMessage(types.EntityID, netproto.ChatChannel, types.EntityID, string, string) {
	panic("local chat must use one broadcast")
}

func (r *chatDeliveryRecorder) BroadcastChatMessage(ids []types.EntityID, channel netproto.ChatChannel, senderID types.EntityID, name, text string) {
	// Production delivery borrows ids only for this call. Retained test evidence
	// must own a copy so later buffer reuse cannot rewrite earlier messages.
	r.messages = append(r.messages, capturedChat{append([]types.EntityID(nil), ids...), channel, senderID, name, text})
}

func spawnChatCommandSender(w *ecs.World, id types.EntityID) types.Handle {
	name := "Sender"
	return w.Spawn(id, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Transform{X: -12.5, Y: 256})
		ecs.AddComponent(w, handle, components.Appearance{Name: &name})
	})
}

func TestNetworkCommandChatReusesAndResetsRecipientBuffer(t *testing.T) {
	w := ecs.NewWorldWithCapacity(128, nil, 0)
	const senderID types.EntityID = 50
	spawnChatCommandSender(w, senderID)
	inbox := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
	jobs := network.NewServerJobInbox(network.DefaultCommandQueueConfig())
	delivery := &chatDeliveryRecorder{}
	calls := 0
	var backing *types.EntityID
	var grownCap int
	selector := chatSelectorFunc(func(gotWorld *ecs.World, x, y, radius, radiusSq float64, dst []types.EntityID) []types.EntityID {
		require.Same(t, w, gotWorld)
		require.Equal(t, -12.5, x)
		require.Equal(t, 256.0, y)
		require.Equal(t, 1000.0, radius)
		require.Equal(t, 1000000.0, radiusSq)
		require.Empty(t, dst)
		calls++
		switch calls {
		case 1:
			require.Equal(t, 32, cap(dst))
			for id := types.EntityID(1); id <= 80; id++ {
				dst = append(dst, id)
			}
			backing, grownCap = &dst[0], cap(dst)
			return dst
		case 2, 3:
			require.Equal(t, grownCap, cap(dst))
			require.Same(t, backing, &dst[:cap(dst)][0])
			if calls == 2 {
				return append(dst, senderID)
			}
			return dst
		default:
			t.Fatal("unexpected selector invocation")
			return dst
		}
	})
	system := NewNetworkCommandSystem(inbox, jobs, delivery, selector, nil, nil, nil, 1000, zap.NewNop())
	for sequence, text := range []string{"first", "second", "empty"} {
		require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
			ClientID: 1, CharacterID: senderID, CommandID: uint64(sequence + 1), CommandType: network.CmdChat,
			Payload: &network.ChatCommandPayload{Channel: netproto.ChatChannel_CHAT_CHANNEL_LOCAL, Text: text},
		}))
		system.Update(w, 0)
	}
	require.Equal(t, 3, calls)
	require.Len(t, delivery.messages, 2)
	require.Len(t, delivery.messages[0].ids, 80)
	require.Equal(t, []types.EntityID{senderID}, delivery.messages[1].ids)
	for i, message := range delivery.messages {
		require.Equal(t, senderID, message.senderID)
		require.Equal(t, "Sender", message.name)
		require.Equal(t, netproto.ChatChannel_CHAT_CHANNEL_LOCAL, message.channel)
		require.Equal(t, []string{"first", "second"}[i], message.text)
	}
	require.Empty(t, system.chatRecipients)
	require.Equal(t, grownCap, cap(system.chatRecipients))
}

func TestNetworkCommandChatRadiusConversionDoesNotOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, configured := range []int{0, -1000, 1000, maxInt, -maxInt - 1} {
		t.Run(strconv.Itoa(configured), func(t *testing.T) {
			w := ecs.NewWorldWithCapacity(128, nil, 0)
			handle := spawnChatCommandSender(w, 1)
			called := false
			selector := chatSelectorFunc(func(_ *ecs.World, _, _, radius, square float64, dst []types.EntityID) []types.EntityID {
				called = true
				expected := math.Abs(float64(configured))
				require.Equal(t, expected, radius)
				require.Equal(t, expected*expected, square)
				require.False(t, math.IsInf(square, 0))
				return append(dst, 1)
			})
			system := NewNetworkCommandSystem(nil, nil, &chatDeliveryRecorder{}, selector, nil, nil, nil, configured, zap.NewNop())
			system.handleChat(w, handle, &network.PlayerCommand{CharacterID: 1,
				Payload: &network.ChatCommandPayload{Text: "hello"}})
			require.True(t, called)
		})
	}
}

func TestNetworkCommandChatMissingDependenciesDoesNotScanCharacters(t *testing.T) {
	for _, missing := range []string{"selector", "delivery"} {
		t.Run(missing, func(t *testing.T) {
			w := ecs.NewWorldWithCapacity(128, nil, 0)
			handle := spawnChatCommandSender(w, 1)
			ecs.GetResource[ecs.CharacterEntities](w).Map[1] = ecs.CharacterEntity{Handle: handle}
			core, logs := observer.New(zap.ErrorLevel)
			var selector ChatRecipientSelector = chatSelectorFunc(func(*ecs.World, float64, float64, float64, float64, []types.EntityID) []types.EntityID {
				t.Fatal("selector must not run with incomplete routing dependencies")
				return nil
			})
			recorder := &chatDeliveryRecorder{}
			var delivery ChatDeliveryService = recorder
			if missing == "selector" {
				selector = nil
			} else {
				delivery = nil
			}
			system := NewNetworkCommandSystem(nil, nil, delivery, selector, nil, nil, nil, 1000, zap.New(core))
			system.handleChat(w, handle, &network.PlayerCommand{CharacterID: 1,
				Payload: &network.ChatCommandPayload{Text: "hello"}})
			require.Empty(t, recorder.messages)
			require.Equal(t, 1, logs.Len())
		})
	}
}

type handledChatAdmin struct {
	AdminCommandHandler
	calls int
}

func (a *handledChatAdmin) HandleCommand(*ecs.World, types.EntityID, types.Handle, string) bool {
	a.calls++
	return true
}

func TestNetworkCommandChatAdminInterceptionPrecedesRecipientSelection(t *testing.T) {
	w := ecs.NewWorldWithCapacity(128, nil, 0)
	handle := spawnChatCommandSender(w, 1)
	delivery := &chatDeliveryRecorder{}
	selector := chatSelectorFunc(func(*ecs.World, float64, float64, float64, float64, []types.EntityID) []types.EntityID {
		t.Fatal("handled admin command must not select chat recipients")
		return nil
	})
	system := NewNetworkCommandSystem(nil, nil, delivery, selector, nil, nil, nil, 1000, zap.NewNop())
	admin := &handledChatAdmin{}
	system.SetAdminHandler(admin)
	system.handleChat(w, handle, &network.PlayerCommand{CharacterID: 1,
		Payload: &network.ChatCommandPayload{Text: "/handled"}})
	require.Equal(t, 1, admin.calls)
	require.Empty(t, delivery.messages)
}
