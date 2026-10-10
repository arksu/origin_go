package game

import (
	"errors"
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type adminTimeExecutorFunc func(int64) error

func (f adminTimeExecutorFunc) RequestAdminAddTime(seconds int64) error { return f(seconds) }

type addTimeChatMessage struct {
	playerID types.EntityID
	channel  netproto.ChatChannel
	senderID types.EntityID
	name     string
	text     string
}

type addTimeChatRecorder struct {
	messages   []addTimeChatMessage
	broadcasts int
}

func (r *addTimeChatRecorder) SendChatMessage(playerID types.EntityID, channel netproto.ChatChannel, senderID types.EntityID, name, text string) {
	r.messages = append(r.messages, addTimeChatMessage{playerID, channel, senderID, name, text})
}

func (r *addTimeChatRecorder) BroadcastChatMessage([]types.EntityID, netproto.ChatChannel, types.EntityID, string, string) {
	r.broadcasts++
}

type addTimeChatSelector struct{ calls int }

func (s *addTimeChatSelector) AppendLocalChatRecipients(_ *ecs.World, _, _, _, _ float64, dst []types.EntityID) []types.EntityID {
	s.calls++
	return append(dst, 99)
}

func TestAddTimeCommandInterceptedBeforeLocalChat(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		want        string
		seconds     int64
		missing     bool
		executorErr error
	}{
		{name: "valid", text: "/addtime 60", seconds: 60, want: "Queued +60 runtime seconds; applies next tick."},
		{name: "whitespace", text: "/addtime\t 60  ", seconds: 60, want: "Queued +60 runtime seconds; applies next tick."},
		{name: "maximum signed integer", text: "/addtime 9223372036854775807", seconds: math.MaxInt64, want: "Queued +9223372036854775807 runtime seconds; applies next tick."},
		{name: "missing argument", text: "/addtime", want: "usage: /addtime <seconds>"},
		{name: "extra argument", text: "/addtime 60 1", want: "usage: /addtime <seconds>"},
		{name: "fractional", text: "/addtime 0.5", want: "invalid seconds: 0.5; expected a positive integer"},
		{name: "negative", text: "/addtime -60", want: "invalid seconds: -60; expected a positive integer"},
		{name: "zero", text: "/addtime 0", want: "invalid seconds: 0; expected a positive integer"},
		{name: "integer overflow", text: "/addtime 9223372036854775808", want: "invalid seconds: 9223372036854775808; expected a positive integer"},
		{name: "very large integer", text: "/addtime 999999999999999999999999999999", want: "invalid seconds: 999999999999999999999999999999; expected a positive integer"},
		{name: "invalid", text: "/addtime tomorrow", want: "invalid seconds: tomorrow; expected a positive integer"},
		{name: "missing executor", text: "/addtime 60", missing: true, want: "runtime service unavailable"},
		{name: "executor error", text: "/addtime 60", seconds: 60, executorErr: errors.New("runtime limit exceeded"), want: "addtime failed: runtime limit exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const playerID types.EntityID = 42
			world := ecs.NewWorldWithCapacity(100, nil, 0)
			name := "Player"
			world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Appearance{Name: &name})
			})
			chat := &addTimeChatRecorder{}
			selector := &addTimeChatSelector{}
			core, logs := observer.New(zap.InfoLevel)
			logger := zap.New(core)
			handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, logger)
			var calls []int64
			if !test.missing {
				handler.SetTimeExecutor(adminTimeExecutorFunc(func(seconds int64) error {
					calls = append(calls, seconds)
					return test.executorErr
				}))
			}
			inbox := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
			jobs := network.NewServerJobInbox(network.DefaultCommandQueueConfig())
			system := systems.NewNetworkCommandSystem(inbox, jobs, chat, selector, nil, nil, nil, 1000, logger)
			system.SetAdminHandler(handler)
			require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
				ClientID: 1, CharacterID: playerID, CommandID: 1, CommandType: network.CmdChat,
				Payload: &network.ChatCommandPayload{Channel: netproto.ChatChannel_CHAT_CHANNEL_LOCAL, Text: test.text},
			}))
			system.Update(world, 0)

			if test.seconds > 0 {
				require.Equal(t, []int64{test.seconds}, calls)
			} else {
				require.Empty(t, calls)
			}
			require.Equal(t, []addTimeChatMessage{{playerID, netproto.ChatChannel_CHAT_CHANNEL_LOCAL, 0, "[Server]", test.want}}, chat.messages)
			require.Zero(t, chat.broadcasts, "recognized commands must never reach local chat")
			require.Zero(t, selector.calls, "recognized commands must bypass recipient selection")
			if test.seconds > 0 && test.executorErr == nil {
				require.Equal(t, 1, logs.Len())
				require.Equal(t, "Admin /addtime queued", logs.All()[0].Message)
				require.Equal(t, uint64(playerID), logs.All()[0].ContextMap()["player_id"])
				require.Equal(t, test.seconds, logs.All()[0].ContextMap()["seconds"])
			} else {
				require.Zero(t, logs.Len(), "rejected commands must not be logged as accepted")
			}
		})
	}
}

func TestAddTimeCommandKeepsCurrentBatchTimeSnapshot(t *testing.T) {
	const playerID types.EntityID = 42
	world := ecs.NewWorldWithCapacity(100, nil, 0)
	world.Spawn(playerID, nil)
	ecs.SetResource(world, ecs.TimeState{RuntimeSecondsTotal: 28_799})
	chat := &addTimeChatRecorder{}
	handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zap.NewNop())
	queued := int64(0)
	handler.SetTimeExecutor(adminTimeExecutorFunc(func(seconds int64) error {
		queued += seconds
		return nil
	}))
	inbox := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
	jobs := network.NewServerJobInbox(network.DefaultCommandQueueConfig())
	system := systems.NewNetworkCommandSystem(inbox, jobs, chat, &addTimeChatSelector{}, nil, nil, nil, 1000, zap.NewNop())
	system.SetAdminHandler(handler)
	for i, text := range []string{"/addtime 60", "/time"} {
		require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
			ClientID: 1, CharacterID: playerID, CommandID: uint64(i + 1), CommandType: network.CmdChat,
			Payload: &network.ChatCommandPayload{Text: text},
		}))
	}
	system.Update(world, 0)
	require.Equal(t, int64(60), queued)
	require.Equal(t, int64(28_799), ecs.GetResource[ecs.TimeState](world).RuntimeSecondsTotal)
	require.Len(t, chat.messages, 2)
	require.Equal(t, "Queued +60 runtime seconds; applies next tick.", chat.messages[0].text)
	require.Equal(t, "Year 1, Month 1, Day 1, 23:59:57; runtime_seconds: 28799", chat.messages[1].text)
	require.Zero(t, chat.broadcasts)
}
