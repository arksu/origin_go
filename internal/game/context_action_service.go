package game

import (
	"context"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	contextActionOpen = "open"
)

var contextActionDuplicateTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "context_action_duplicate_total",
		Help: "Duplicate context actions produced by different behaviors",
	},
	[]string{"entity_def", "action_id", "winner_behavior", "loser_behavior"},
)

type miniAlertSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
}

type cyclicActionFinishSender interface {
	SendCyclicActionFinished(entityID types.EntityID, finished *netproto.S2C_CyclicActionFinished)
}

type visionUpdateForcer interface {
	ForceUpdateForObserver(w *ecs.World, observerHandle types.Handle)
}

type ContextActionService struct {
	world       *ecs.World
	logger      *zap.Logger
	openSvc     systems.OpenContainerCoordinator
	alerts      miniAlertSender
	cyclicOut   cyclicActionFinishSender
	vision      visionUpdateForcer
	soundEvents *SoundEventService
	eventBus    *eventbus.EventBus
	chunks      chunkProvider
	idAlloc     entityIDAllocator
	behaviors   map[string]contextActionBehavior
}

type contextActionBehavior interface {
	Actions(w *ecs.World, targetID types.EntityID, targetHandle types.Handle) []systems.ContextAction
	Execute(
		w *ecs.World,
		playerID types.EntityID,
		playerHandle types.Handle,
		targetID types.EntityID,
		targetHandle types.Handle,
		actionID string,
		openSvc systems.OpenContainerCoordinator,
	) contextActionExecuteResult
}

type contextActionExecuteResult struct {
	ok          bool
	userVisible bool
	reasonCode  string
	severity    netproto.AlertSeverity
}

func NewContextActionService(
	world *ecs.World,
	eventBus *eventbus.EventBus,
	openSvc systems.OpenContainerCoordinator,
	alerts miniAlertSender,
	cyclicOut cyclicActionFinishSender,
	vision visionUpdateForcer,
	chunks chunkProvider,
	idAlloc entityIDAllocator,
	logger *zap.Logger,
) *ContextActionService {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &ContextActionService{
		world:       world,
		logger:      logger,
		openSvc:     openSvc,
		alerts:      alerts,
		cyclicOut:   cyclicOut,
		vision:      vision,
		soundEvents: NewSoundEventService(nil, logger),
		eventBus:    eventBus,
		chunks:      chunks,
		idAlloc:     idAlloc,
		behaviors: map[string]contextActionBehavior{
			"container": containerContextActionBehavior{},
			"tree": treeContextActionBehavior{
				eventBus:     eventBus,
				chunks:       chunks,
				idAllocator:  idAlloc,
				visionForcer: vision,
				logger:       logger,
			},
		},
	}

	if eventBus != nil {
		eventBus.SubscribeSync(ecs.TopicGameplayLinkCreated, eventbus.PriorityHigh, s.onLinkCreated)
		eventBus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityHigh, s.onLinkBroken)
	}

	return s
}

func (s *ContextActionService) SetSoundEventSender(sender soundEventSender) {
	if s == nil || s.soundEvents == nil {
		return
	}
	s.soundEvents.SetSender(sender)
}

var _ systems.ContextActionResolver = (*ContextActionService)(nil)

func (s *ContextActionService) ComputeActions(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) []systems.ContextAction {
	_ = playerID
	_ = playerHandle

	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo || len(entityInfo.Behaviors) == 0 {
		return nil
	}

	actions := make([]systems.ContextAction, 0, 4)
	seen := make(map[string]string, 4) // actionID -> behavior key
	defName := s.entityDefName(entityInfo.TypeID)

	// Deterministic merge in def order.
	// Duplicate action IDs are treated as content mistakes:
	// first behavior wins + WARN + metric.
	for _, behaviorKey := range entityInfo.Behaviors {
		behavior, ok := s.behaviors[behaviorKey]
		if !ok {
			continue
		}

		behaviorActions := behavior.Actions(w, targetID, targetHandle)
		for _, action := range behaviorActions {
			if action.ActionID == "" {
				continue
			}
			if winnerBehavior, exists := seen[action.ActionID]; exists {
				s.logger.Warn("duplicate context action detected",
					zap.Uint32("entity_type_id", entityInfo.TypeID),
					zap.String("entity_def", defName),
					zap.String("action_id", action.ActionID),
					zap.String("winner_behavior", winnerBehavior),
					zap.String("loser_behavior", behaviorKey),
				)
				contextActionDuplicateTotal.WithLabelValues(defName, action.ActionID, winnerBehavior, behaviorKey).Inc()
				continue
			}

			seen[action.ActionID] = behaviorKey
			actions = append(actions, action)
		}
	}

	return actions
}

func (s *ContextActionService) ExecuteAction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
	actionID string,
) bool {
	behavior, found := s.resolveBehaviorForAction(w, targetID, targetHandle, actionID)
	if !found {
		// State changed or action no longer provided by behaviors.
		// By product rule this is a silent ignore.
		return false
	}

	result := behavior.Execute(w, playerID, playerHandle, targetID, targetHandle, actionID, s.openSvc)
	if result.ok || !result.userVisible {
		return true
	}

	s.sendMiniAlert(playerID, result.severity, result.reasonCode)
	return true
}

func (s *ContextActionService) behaviorOrder(targetHandle types.Handle, w *ecs.World) []string {
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return nil
	}
	return entityInfo.Behaviors
}

func (s *ContextActionService) resolveBehaviorForAction(
	w *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	actionID string,
) (contextActionBehavior, bool) {
	if actionID == "" {
		return nil, false
	}

	for _, behaviorKey := range s.behaviorOrder(targetHandle, w) {
		behavior, ok := s.behaviors[behaviorKey]
		if !ok {
			continue
		}
		for _, action := range behavior.Actions(w, targetID, targetHandle) {
			if action.ActionID == actionID {
				return behavior, true
			}
		}
	}
	return nil, false
}

func (s *ContextActionService) onLinkCreated(_ context.Context, event eventbus.Event) error {
	linkEvent, ok := event.(*ecs.LinkCreatedEvent)
	if !ok || linkEvent.Layer != s.world.Layer {
		return nil
	}

	playerHandle := s.world.GetHandleByEntityID(linkEvent.PlayerID)
	if playerHandle == types.InvalidHandle || !s.world.Alive(playerHandle) {
		return nil
	}

	pending, hasPending := ecs.GetComponent[components.PendingContextAction](s.world, playerHandle)
	if !hasPending || pending.TargetEntityID != linkEvent.TargetID {
		return nil
	}

	ecs.RemoveComponent[components.PendingContextAction](s.world, playerHandle)
	if pending.ActionID == "" {
		return nil
	}

	targetHandle := s.world.GetHandleByEntityID(pending.TargetEntityID)
	if targetHandle == types.InvalidHandle || !s.world.Alive(targetHandle) {
		return nil
	}

	// LinkCreated is the only execution trigger for pending context actions.
	// This guarantees movement->link->validate->execute order.
	s.ExecuteAction(
		s.world,
		linkEvent.PlayerID,
		playerHandle,
		pending.TargetEntityID,
		targetHandle,
		pending.ActionID,
	)

	return nil
}

func (s *ContextActionService) onLinkBroken(_ context.Context, event eventbus.Event) error {
	linkEvent, ok := event.(*ecs.LinkBrokenEvent)
	if !ok || linkEvent.Layer != s.world.Layer {
		return nil
	}

	playerHandle := s.world.GetHandleByEntityID(linkEvent.PlayerID)
	if playerHandle == types.InvalidHandle || !s.world.Alive(playerHandle) {
		return nil
	}
	ecs.RemoveComponent[components.PendingContextAction](s.world, playerHandle)
	s.cancelActiveCyclicAction(linkEvent.PlayerID, playerHandle, "link_broken")
	return nil
}

func (s *ContextActionService) handleCyclicCycleComplete(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) cyclicActionDecision {
	if action.BehaviorKey == "" {
		return cyclicActionDecisionCanceled
	}

	rawBehavior, found := s.behaviors[action.BehaviorKey]
	if !found {
		return cyclicActionDecisionCanceled
	}

	cyclicBehavior, ok := rawBehavior.(cyclicActionBehavior)
	if !ok {
		return cyclicActionDecisionCanceled
	}

	targetHandle := action.TargetHandle
	if targetHandle == types.InvalidHandle {
		targetHandle = w.GetHandleByEntityID(action.TargetID)
	}

	return cyclicBehavior.OnCycleComplete(cyclicActionCycleContext{
		World:        w,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     action.TargetID,
		TargetHandle: targetHandle,
		Action:       action,
	})
}

func (s *ContextActionService) completeActiveCyclicAction(playerID types.EntityID, playerHandle types.Handle) {
	s.finishActiveCyclicAction(
		playerID,
		playerHandle,
		netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED,
		"",
	)
}

func (s *ContextActionService) cancelActiveCyclicAction(playerID types.EntityID, playerHandle types.Handle, reasonCode string) {
	s.finishActiveCyclicAction(
		playerID,
		playerHandle,
		netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED,
		reasonCode,
	)
}

func (s *ContextActionService) finishActiveCyclicAction(
	playerID types.EntityID,
	playerHandle types.Handle,
	result netproto.CyclicActionFinishResult,
	reasonCode string,
) {
	if playerHandle == types.InvalidHandle || !s.world.Alive(playerHandle) {
		return
	}
	activeAction, has := ecs.GetComponent[components.ActiveCyclicAction](s.world, playerHandle)
	if !has {
		return
	}
	if result == netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED {
		s.emitTargetSound(activeAction.CompleteSoundKey, activeAction.TargetHandle, activeAction.TargetID)
	}
	s.sendCyclicActionFinished(playerID, activeAction, result, reasonCode)
	ecs.RemoveComponent[components.ActiveCyclicAction](s.world, playerHandle)
	ecs.MutateComponent[components.Movement](s.world, playerHandle, func(m *components.Movement) bool {
		if m.State == constt.StateInteracting {
			m.State = constt.StateIdle
		}
		return true
	})
}

func (s *ContextActionService) sendCyclicActionFinished(
	playerID types.EntityID,
	action components.ActiveCyclicAction,
	result netproto.CyclicActionFinishResult,
	reasonCode string,
) {
	if s.cyclicOut == nil || playerID == 0 {
		return
	}

	finished := &netproto.S2C_CyclicActionFinished{
		ActionId:       action.ActionID,
		TargetEntityId: uint64(action.TargetID),
		CycleIndex:     action.CycleIndex,
		Result:         result,
	}
	if result == netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED && reasonCode != "" {
		finished.ReasonCode = &reasonCode
	}
	s.cyclicOut.SendCyclicActionFinished(playerID, finished)
}

func (s *ContextActionService) emitCycleSound(action components.ActiveCyclicAction) {
	if action.TargetKind != components.CyclicActionTargetObject {
		return
	}
	s.emitTargetSound(action.FinishSoundKey, action.TargetHandle, action.TargetID)
}

func (s *ContextActionService) emitTargetSound(
	soundKey string,
	targetHandle types.Handle,
	targetID types.EntityID,
) {
	if s == nil || s.world == nil || s.soundEvents == nil {
		return
	}

	resolvedTargetHandle := targetHandle
	if resolvedTargetHandle == types.InvalidHandle || !s.world.Alive(resolvedTargetHandle) {
		resolvedTargetHandle = s.world.GetHandleByEntityID(targetID)
	}
	if resolvedTargetHandle == types.InvalidHandle || !s.world.Alive(resolvedTargetHandle) {
		return
	}

	s.soundEvents.EmitForVisibleTarget(s.world, resolvedTargetHandle, soundKey)
}

func (s *ContextActionService) sendMiniAlert(playerID types.EntityID, severity netproto.AlertSeverity, reasonCode string) {
	if s.alerts == nil || reasonCode == "" {
		return
	}

	alert := &netproto.S2C_MiniAlert{
		Severity:   severity,
		ReasonCode: reasonCode,
		TtlMs:      ttlBySeverity(severity),
	}
	s.alerts.SendMiniAlert(playerID, alert)
}

func ttlBySeverity(severity netproto.AlertSeverity) uint32 {
	switch severity {
	case netproto.AlertSeverity_ALERT_SEVERITY_ERROR:
		return 2500
	case netproto.AlertSeverity_ALERT_SEVERITY_WARNING:
		return 2000
	default:
		return 1500
	}
}

func reasonFromErrorCode(code netproto.ErrorCode) string {
	name := strings.TrimPrefix(code.String(), "ERROR_CODE_")
	if name == "" {
		return "internal_error"
	}
	return strings.ToLower(name)
}

func (s *ContextActionService) entityDefName(typeID uint32) string {
	return "type_" + strconv.Itoa(int(typeID))
}

type containerContextActionBehavior struct{}

func (containerContextActionBehavior) Actions(
	w *ecs.World,
	targetID types.EntityID,
	_ types.Handle,
) []systems.ContextAction {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, targetID, 0)
	if !found || !w.Alive(rootHandle) {
		return nil
	}

	return []systems.ContextAction{
		{
			ActionID: contextActionOpen,
			Title:    "Open",
		},
	}
}

func (containerContextActionBehavior) Execute(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	_ types.Handle,
	actionID string,
	openSvc systems.OpenContainerCoordinator,
) contextActionExecuteResult {
	if actionID != contextActionOpen || openSvc == nil {
		return contextActionExecuteResult{ok: false}
	}

	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId:      uint64(targetID),
		InventoryKey: 0,
	}
	if openErr := openSvc.HandleOpenRequest(w, playerID, playerHandle, ref); openErr != nil {
		return contextActionExecuteResult{
			ok:          false,
			userVisible: true,
			reasonCode:  reasonFromErrorCode(openErr.Code),
			severity:    netproto.AlertSeverity_ALERT_SEVERITY_ERROR,
		}
	}

	return contextActionExecuteResult{ok: true}
}
