package game

import (
	"context"
	"testing"

	"origin/internal/builddefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type constructionTestBehavior struct {
	complete func(*contracts.ConstructionContext, func() bool) contracts.ConstructionResult
}

func (*constructionTestBehavior) Key() string { return "construction_test" }
func (*constructionTestBehavior) ValidateAndApplyDefConfig(*contracts.BehaviorDefConfigContext) (int, error) {
	return 0, nil
}
func (b *constructionTestBehavior) CompleteConstruction(ctx *contracts.ConstructionContext, complete func() bool) contracts.ConstructionResult {
	return b.complete(ctx, complete)
}

type constructionTestSender struct {
	lockedBuildSender
	closed []types.EntityID
}

func (s *constructionTestSender) SendBuildStateClosed(playerID types.EntityID, _ *netproto.S2C_BuildStateClosed) {
	s.closed = append(s.closed, playerID)
}

type constructionTestFixture struct {
	w       *ecs.World
	player  types.Handle
	site    types.Handle
	service *BuildService
	sender  *constructionTestSender
	bus     *eventbus.EventBus
	stamina float64
}

func newConstructionTestFixture(t *testing.T, behavior *constructionTestBehavior) *constructionTestFixture {
	t.Helper()
	previousBuilds, previousObjects := builddefs.Global(), objectdefs.Global()
	t.Cleanup(func() {
		builddefs.SetGlobalForTesting(previousBuilds)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	build := builddefs.BuildDef{DefID: 900, Key: "construction_test", ObjectKey: "constructed_test", TicksRequired: 1, StaminaCost: 8}
	builddefs.SetGlobalForTesting(builddefs.NewRegistry([]builddefs.BuildDef{build}))
	definition := objectdefs.ObjectDef{DefID: 901, Key: build.ObjectKey, HP: 100}
	var registry contracts.BehaviorRegistry
	if behavior != nil {
		definition.BehaviorOrder = []string{behavior.Key()}
		var err error
		registry, err = behaviors.NewRegistry(behavior)
		require.NoError(t, err)
	}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{definition}))
	bus := newGameTestEventBus(t)
	t.Cleanup(func() { shutdownGameTestEventBus(t, bus) })
	w := ecs.NewWorld(bus, 0)
	stamina := entitystats.MaxStaminaFromCon(characterattrs.DefaultValue)
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: stamina})
	})
	site := w.Spawn(200, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: constt.BuildObjectTypeID})
		state := components.ObjectInternalState{}
		components.SetBehaviorState(&state, buildBehaviorStateKey, &components.BuildBehaviorState{
			BuildDefID: build.DefID, ObjectTypeID: uint32(definition.DefID),
			Items: []components.BuildRequiredItemState{{ItemKey: "branch", RequiredCount: 2,
				PutItems: []components.BuildPutItemState{{ItemKey: "branch", Count: 2, Quality: 10}}}},
		})
		ecs.AddComponent(w, h, state)
	})
	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 200, TargetHandle: site})
	sender := &constructionTestSender{}
	service := NewBuildService(w, bus, nil, nil, nil, nil, registry, nil, sender, nil, nil, nil)
	return &constructionTestFixture{w: w, player: player, site: site, service: service, sender: sender, bus: bus, stamina: stamina}
}

func (f *constructionTestFixture) cycle() contracts.BehaviorCycleDecision {
	return f.service.HandleBuildCycleComplete(f.w, 1, f.player, components.ActiveCyclicAction{
		ActionID: buildSyntheticActionID, TargetID: 200, TargetHandle: f.site,
	})
}

func (f *constructionTestFixture) progress(t *testing.T) *components.BuildBehaviorState {
	t.Helper()
	state, ok := ecs.GetComponent[components.ObjectInternalState](f.w, f.site)
	require.True(t, ok)
	build, ok := components.GetBehaviorState[components.BuildBehaviorState](state, buildBehaviorStateKey)
	require.True(t, ok)
	return build
}

func TestConstructionCompletionWrapsOnlyFinalMaterialAndPreservesLinkClosure(t *testing.T) {
	behavior := &constructionTestBehavior{}
	f := newConstructionTestFixture(t, behavior)
	called := 0
	behavior.complete = func(ctx *contracts.ConstructionContext, complete func() bool) contracts.ConstructionResult {
		called++
		require.Equal(t, &contracts.ConstructionContext{World: f.w, PlayerID: 1, PlayerHandle: f.player, TargetID: 200, TargetHandle: f.site, ResultType: 901}, ctx)
		require.Equal(t, uint32(1), f.progress(t).Items[0].BuildCount)
		stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
		require.Equal(t, f.stamina-8, stats.Stamina, "preflight precedes the final cost")
		require.True(t, complete())
		info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.site)
		require.Equal(t, uint32(901), info.TypeID)
		return contracts.ConstructionResult{BehaviorResult: contracts.BehaviorResult{OK: true}}
	}
	helper := f.w.Spawn(2, nil)
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 2, PlayerHandle: helper, TargetID: 200, TargetHandle: f.site})
	broken := 0
	f.bus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		broken++
		require.Equal(t, types.EntityID(2), event.(*ecs.LinkBrokenEvent).PlayerID, "the finishing actor still unlinks silently")
		info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.site)
		require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID, "link hooks still see the build site")
		return nil
	})
	require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.cycle())
	require.Zero(t, called)
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, f.cycle())
	require.Equal(t, 1, called)
	require.Equal(t, 1, broken)
	require.Equal(t, []types.EntityID{2, 1}, f.sender.closed)
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, f.stamina-16, stats.Stamina)
	require.Equal(t, contracts.BehaviorCycleDecisionCanceled, f.cycle())
	require.Equal(t, 1, called, "a transformed site cannot run completion again")
}

func TestConstructionCompletionRejectionPreservesFinalCosts(t *testing.T) {
	behavior := &constructionTestBehavior{complete: func(*contracts.ConstructionContext, func() bool) contracts.ConstructionResult {
		return contracts.ConstructionResult{BehaviorResult: contracts.BehaviorResult{UserVisible: true, ReasonCode: "CONSTRUCTION_NOT_READY", Severity: contracts.BehaviorAlertSeverityWarning}}
	}}
	f := newConstructionTestFixture(t, behavior)
	require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.cycle())
	require.Equal(t, contracts.BehaviorCycleDecisionCanceled, f.cycle())
	state := f.progress(t)
	require.Equal(t, uint32(1), state.Items[0].BuildCount)
	require.Equal(t, uint32(1), state.Items[0].PutCount())
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, f.stamina-8, stats.Stamina)
	require.Empty(t, f.sender.closed)
	require.Len(t, f.sender.alerts, 1)
	require.Equal(t, "CONSTRUCTION_NOT_READY", f.sender.alerts[0].ReasonCode)
}

func TestConstructionCompletionSeesFailedDefaultCompletion(t *testing.T) {
	for _, failure := range []string{"stamina", "transform"} {
		t.Run(failure, func(t *testing.T) {
			behavior := &constructionTestBehavior{}
			f := newConstructionTestFixture(t, behavior)
			require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.cycle())
			if failure == "stamina" {
				ecs.WithComponent(f.w, f.player, func(stats *components.EntityStats) { stats.Stamina = 0 })
			} else {
				ecs.SetResource(f.w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{f.site: true}})
			}
			called := false
			behavior.complete = func(_ *contracts.ConstructionContext, complete func() bool) contracts.ConstructionResult {
				called = true
				require.False(t, complete())
				return contracts.ConstructionResult{}
			}
			require.Equal(t, contracts.BehaviorCycleDecisionCanceled, f.cycle())
			require.True(t, called)
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.site)
			require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
			if failure == "stamina" {
				require.Equal(t, uint32(1), f.progress(t).Items[0].BuildCount)
				require.Equal(t, uint32(1), f.progress(t).Items[0].PutCount())
			}
		})
	}
}

func TestConstructionCompletionDoesNotInventFinisherOnCompleteSite(t *testing.T) {
	for _, entry := range []string{"cycle", "start"} {
		for _, capable := range []bool{false, true} {
			t.Run(entry+map[bool]string{false: "/ordinary", true: "/capable"}[capable], func(t *testing.T) {
				var behavior *constructionTestBehavior
				if capable {
					behavior = &constructionTestBehavior{complete: func(*contracts.ConstructionContext, func() bool) contracts.ConstructionResult {
						t.Fatal("a completed site no longer identifies the last material's worker")
						return contracts.ConstructionResult{}
					}}
				}
				f := newConstructionTestFixture(t, behavior)
				state := f.progress(t)
				state.Items[0].BuildCount, state.Items[0].PutItems = 2, nil
				if entry == "cycle" {
					decision := f.cycle()
					if capable {
						require.Equal(t, contracts.BehaviorCycleDecisionCanceled, decision)
					} else {
						require.Equal(t, contracts.BehaviorCycleDecisionComplete, decision)
					}
				} else {
					f.service.startBuildCyclicAction(f.w, 1, f.player, 200, f.site)
				}
				info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.site)
				if capable {
					require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
					require.Empty(t, f.sender.closed)
				} else {
					require.Equal(t, uint32(901), info.TypeID)
				}
				stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
				require.Equal(t, f.stamina, stats.Stamina)
			})
		}
	}
}

func TestConstructionCompletionRejectsReusedTargetHandle(t *testing.T) {
	behavior := &constructionTestBehavior{complete: func(*contracts.ConstructionContext, func() bool) contracts.ConstructionResult {
		t.Fatal("a stale action must not finish a replacement entity")
		return contracts.ConstructionResult{}
	}}
	f := newConstructionTestFixture(t, behavior)
	require.Equal(t, contracts.BehaviorCycleDecisionContinue, f.cycle())
	oldHandle := f.site
	info, _ := ecs.GetComponent[components.EntityInfo](f.w, oldHandle)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, oldHandle)
	f.w.Despawn(oldHandle)
	f.site = f.w.Spawn(200, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, info)
		ecs.AddComponent(w, h, state)
	})
	require.NotEqual(t, oldHandle, f.site)
	decision := f.service.HandleBuildCycleComplete(f.w, 1, f.player, components.ActiveCyclicAction{
		ActionID: buildSyntheticActionID, TargetID: 200, TargetHandle: oldHandle,
	})
	require.Equal(t, contracts.BehaviorCycleDecisionCanceled, decision)
	require.Equal(t, uint32(1), f.progress(t).Items[0].BuildCount)
	require.Equal(t, uint32(1), f.progress(t).Items[0].PutCount())
	stats, _ := ecs.GetComponent[components.EntityStats](f.w, f.player)
	require.Equal(t, f.stamina-8, stats.Stamina)
}
