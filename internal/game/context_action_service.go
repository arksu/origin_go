package game

import (
	"context"
	"strconv"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var contextActionDuplicateTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "context_action_duplicate_total",
		Help: "Duplicate context actions produced by different behaviors",
	},
	[]string{"entity_def", "action_id", "winner_behavior", "loser_behavior"},
)

const (
	teachContextActionID                 = "teach"
	teachSuccessTeacherReasonCode        = "TEACH_SUCCESS"
	teachSuccessLearnerReasonCode        = "TAUGHT_BY_PLAYER"
	teachCycleDurationTicks       uint32 = 10
	teachCycleStaminaCost                = 100.0
)

type miniAlertSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
}

type cyclicActionFinishSender interface {
	SendCyclicActionFinished(entityID types.EntityID, finished *netproto.S2C_CyclicActionFinished)
}

type ContextActionService struct {
	world            *ecs.World
	logger           *zap.Logger
	alerts           miniAlertSender
	cyclicOut        cyclicActionFinishSender
	soundEvents      *SoundEventService
	behaviorRegistry contracts.BehaviorRegistry
	actionDeps       contracts.ExecutionDeps
	crafting         *CraftingService
}

func NewContextActionService(
	world *ecs.World,
	eventBus *eventbus.EventBus,
	openSvc systems.OpenContainerCoordinator,
	giveItem contracts.GiveItemFn,
	alerts miniAlertSender,
	cyclicOut cyclicActionFinishSender,
	vision contracts.VisionUpdateForcer,
	chunks contracts.TreeChunkProvider,
	idAlloc contracts.EntityIDAllocator,
	behaviorRegistry contracts.BehaviorRegistry,
	logger *zap.Logger,
) *ContextActionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	if behaviorRegistry == nil {
		panic("context action service requires non-nil behavior registry")
	}

	s := &ContextActionService{
		world:            world,
		logger:           logger,
		alerts:           alerts,
		cyclicOut:        cyclicOut,
		soundEvents:      NewSoundEventService(nil, logger),
		behaviorRegistry: behaviorRegistry,
		actionDeps: contracts.ExecutionDeps{
			OpenContainer: func(
				w *ecs.World,
				playerID types.EntityID,
				playerHandle types.Handle,
				ref *netproto.InventoryRef,
			) *contracts.OpenContainerError {
				if openSvc == nil {
					return nil
				}
				openErr := openSvc.HandleOpenRequest(w, playerID, playerHandle, ref)
				if openErr == nil {
					return nil
				}
				return &contracts.OpenContainerError{
					Code:    openErr.Code,
					Message: openErr.Message,
				}
			},
			GiveItem:         giveItem,
			EventBus:         eventBus,
			Chunks:           chunks,
			IDAllocator:      idAlloc,
			VisionForcer:     vision,
			Alerts:           alerts,
			BehaviorRegistry: behaviorRegistry,
			Logger:           logger,
		},
	}
	if buildStateSender, ok := any(alerts).(contracts.BuildStateSender); ok {
		s.actionDeps.BuildState = buildStateSender
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

func (s *ContextActionService) SetCraftingService(crafting *CraftingService) {
	if s == nil {
		return
	}
	s.crafting = crafting
}

var _ systems.ContextActionResolver = (*ContextActionService)(nil)

func (s *ContextActionService) ComputeActions(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) []systems.ContextAction {
	entityInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)

	actions := make([]systems.ContextAction, 0, 4)
	seen := make(map[string]string, 4) // actionID -> behavior key
	defName := s.entityDefName(0)
	if hasInfo {
		defName = s.entityDefName(entityInfo.TypeID)
	}

	// Deterministic merge in definition order.
	// Duplicate action IDs are content mistakes:
	// first behavior wins + WARN + metric.
	if hasInfo && len(entityInfo.Behaviors) > 0 && s.behaviorRegistry != nil {
		for _, behaviorKey := range entityInfo.Behaviors {
			behavior, found := s.behaviorRegistry.GetBehavior(behaviorKey)
			if !found {
				continue
			}
			provider, ok := behavior.(contracts.ContextActionProvider)
			if !ok {
				continue
			}

			behaviorActions := provider.ProvideActions(&contracts.BehaviorActionListContext{
				World:        w,
				PlayerID:     playerID,
				PlayerHandle: playerHandle,
				TargetID:     targetID,
				TargetHandle: targetHandle,
				Deps:         &s.actionDeps,
			})
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
	}

	if _, exists := seen[teachContextActionID]; !exists &&
		s.isTeachTargetPlayer(w, playerID, targetID, targetHandle) {
		if _, ok := s.resolvePlayerHandItemDef(w, playerID, playerHandle); ok {
			actions = append(actions, systems.ContextAction{
				ActionID: teachContextActionID,
				Title:    "Teach",
			})
		}
	}

	if len(actions) == 0 {
		return nil
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
	if actionID == teachContextActionID {
		return s.executeTeachAction(w, playerID, playerHandle, targetID, targetHandle)
	}

	behavior, found := s.resolveBehaviorForAction(w, playerID, playerHandle, targetID, targetHandle, actionID)
	if !found {
		// State changed or action no longer provided by behaviors.
		// By product rule this is a silent ignore.
		return false
	}

	if validator, hasValidator := behavior.(contracts.ContextActionValidator); hasValidator {
		validation := validator.ValidateAction(&contracts.BehaviorActionValidateContext{
			World:        w,
			PlayerID:     playerID,
			PlayerHandle: playerHandle,
			TargetID:     targetID,
			TargetHandle: targetHandle,
			ActionID:     actionID,
			Phase:        contracts.BehaviorValidationPhaseExecute,
			Deps:         &s.actionDeps,
		})
		if !validation.OK {
			if validation.UserVisible {
				s.sendMiniAlert(playerID, mapBehaviorSeverity(validation.Severity), validation.ReasonCode)
			}
			return true
		}
	}

	executor, ok := behavior.(contracts.ContextActionExecutor)
	if !ok {
		return false
	}

	result := executor.ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World:        w,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     actionID,
		Deps:         &s.actionDeps,
	})
	if result.OK || !result.UserVisible {
		return true
	}

	s.sendMiniAlert(playerID, mapBehaviorSeverity(result.Severity), result.ReasonCode)
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
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
	actionID string,
) (contracts.Behavior, bool) {
	if actionID == "" || s.behaviorRegistry == nil {
		return nil, false
	}

	for _, behaviorKey := range s.behaviorOrder(targetHandle, w) {
		behavior, found := s.behaviorRegistry.GetBehavior(behaviorKey)
		if !found {
			continue
		}
		provider, ok := behavior.(contracts.ContextActionProvider)
		if !ok {
			continue
		}
		for _, action := range provider.ProvideActions(&contracts.BehaviorActionListContext{
			World:        w,
			PlayerID:     playerID,
			PlayerHandle: playerHandle,
			TargetID:     targetID,
			TargetHandle: targetHandle,
			Deps:         &s.actionDeps,
		}) {
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
	// This guarantees movement -> link -> validate -> execute order.
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
	// Preserve pending action when player is retargeting to a different object.
	if pending, hasPending := ecs.GetComponent[components.PendingContextAction](s.world, playerHandle); hasPending &&
		pending.TargetEntityID == linkEvent.TargetID {
		ecs.RemoveComponent[components.PendingContextAction](s.world, playerHandle)
	}
	s.cancelActiveCyclicAction(linkEvent.PlayerID, playerHandle, "link_broken")
	return nil
}

func (s *ContextActionService) handleCyclicCycleComplete(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) contracts.BehaviorCycleDecision {
	if s.isSyntheticTeachCyclicAction(action) {
		return s.handleSyntheticTeachCycleComplete(w, playerID, playerHandle, action)
	}
	if s.crafting != nil && s.crafting.IsSyntheticCraftAction(action) {
		return s.crafting.HandleCraftCycleComplete(w, playerID, playerHandle, action)
	}
	if action.BehaviorKey == "" || s.behaviorRegistry == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}

	behavior, found := s.behaviorRegistry.GetBehavior(action.BehaviorKey)
	if !found {
		return contracts.BehaviorCycleDecisionCanceled
	}

	cyclicHandler, ok := behavior.(contracts.CyclicActionHandler)
	if !ok {
		return contracts.BehaviorCycleDecisionCanceled
	}

	targetHandle := action.TargetHandle
	if targetHandle == types.InvalidHandle {
		targetHandle = w.GetHandleByEntityID(action.TargetID)
	}

	return cyclicHandler.OnCycleComplete(&contracts.BehaviorCycleContext{
		World:        w,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     action.TargetID,
		TargetHandle: targetHandle,
		ActionID:     action.ActionID,
		Action:       action,
		Deps:         &s.actionDeps,
	})
}

func (s *ContextActionService) isActiveCyclicActionStillValid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) bool {
	if w == nil {
		return false
	}
	if s.isSyntheticTeachCyclicAction(action) {
		targetID := action.TargetID
		targetHandle := action.TargetHandle
		if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
			targetHandle = w.GetHandleByEntityID(targetID)
		}
		if !s.isTeachTargetPlayer(w, playerID, targetID, targetHandle) {
			return false
		}
		_, ok := s.resolvePlayerHandItemDef(w, playerID, playerHandle)
		return ok
	}
	if s.crafting != nil && s.crafting.IsSyntheticCraftAction(action) {
		return s.crafting.IsActiveCraftStillValid(w, playerID, playerHandle, action)
	}
	if w == nil || s.behaviorRegistry == nil || action.BehaviorKey == "" || action.ActionID == "" {
		return false
	}

	behavior, found := s.behaviorRegistry.GetBehavior(action.BehaviorKey)
	if !found || behavior == nil {
		return true
	}

	validator, hasValidator := behavior.(contracts.ContextActionValidator)
	if !hasValidator {
		return true
	}

	targetID := action.TargetID
	targetHandle := action.TargetHandle
	if action.TargetKind == components.CyclicActionTargetSelf {
		targetID = playerID
		targetHandle = playerHandle
	} else if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		targetHandle = w.GetHandleByEntityID(targetID)
	}

	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return false
	}

	result := validator.ValidateAction(&contracts.BehaviorActionValidateContext{
		World:        w,
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		ActionID:     action.ActionID,
		Phase:        contracts.BehaviorValidationPhasePreview,
		Deps:         &s.actionDeps,
	})
	return result.OK
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
	ecs.RemoveComponent[components.ActiveCraft](s.world, playerHandle)
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
	s.emitTargetSound(action.CycleSoundKey, action.TargetHandle, action.TargetID)
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

func mapBehaviorSeverity(severity contracts.BehaviorAlertSeverity) netproto.AlertSeverity {
	switch severity {
	case contracts.BehaviorAlertSeverityError:
		return netproto.AlertSeverity_ALERT_SEVERITY_ERROR
	case contracts.BehaviorAlertSeverityWarning:
		return netproto.AlertSeverity_ALERT_SEVERITY_WARNING
	default:
		return netproto.AlertSeverity_ALERT_SEVERITY_INFO
	}
}

func (s *ContextActionService) entityDefName(typeID uint32) string {
	return "type_" + strconv.Itoa(int(typeID))
}

func (s *ContextActionService) isTeachTargetPlayer(
	w *ecs.World,
	playerID types.EntityID,
	targetID types.EntityID,
	targetHandle types.Handle,
) bool {
	if w == nil || playerID == 0 || targetID == 0 || playerID == targetID {
		return false
	}
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return false
	}
	_, hasProfile := ecs.GetComponent[components.CharacterProfile](w, targetHandle)
	return hasProfile
}

func (s *ContextActionService) resolvePlayerHandItemDef(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
) (*itemdefs.ItemDef, bool) {
	if w == nil || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return nil, false
	}

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return nil, false
	}

	var handHandle types.Handle
	for _, link := range owner.Inventories {
		if link.Kind != constt.InventoryHand || link.OwnerID != playerID || link.Key != 0 {
			continue
		}
		handHandle = link.Handle
		break
	}
	if handHandle == types.InvalidHandle || !w.Alive(handHandle) {
		return nil, false
	}

	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, handHandle)
	if !hasContainer || container.Kind != constt.InventoryHand || len(container.Items) != 1 {
		return nil, false
	}

	registry := itemdefs.Global()
	if registry == nil {
		return nil, false
	}
	itemDef, found := registry.GetByID(int(container.Items[0].TypeID))
	if !found || itemDef == nil || strings.TrimSpace(itemDef.Key) == "" {
		return nil, false
	}
	return itemDef, true
}

func (s *ContextActionService) learnerHasDiscoveryKey(
	w *ecs.World,
	targetHandle types.Handle,
	itemKey string,
) bool {
	if w == nil || targetHandle == types.InvalidHandle || itemKey == "" {
		return false
	}
	profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, targetHandle)
	if !hasProfile {
		return false
	}
	for _, existingKey := range profile.Discovery {
		if existingKey == itemKey {
			return true
		}
	}
	return false
}

func (s *ContextActionService) executeTeachAction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) bool {
	if !s.isTeachTargetPlayer(w, playerID, targetID, targetHandle) {
		return true
	}
	itemDef, ok := s.resolvePlayerHandItemDef(w, playerID, playerHandle)
	if !ok {
		return true
	}
	if s.learnerHasDiscoveryKey(w, targetHandle, itemDef.Key) {
		return true
	}
	s.startTeachCyclicAction(w, playerID, playerHandle, targetID, targetHandle)
	return true
}

func (s *ContextActionService) startTeachCyclicAction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) {
	if w == nil || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	nowTick := ecs.GetResource[ecs.TimeState](w).Tick
	ecs.AddComponent(w, playerHandle, components.ActiveCyclicAction{
		ActionID:           teachContextActionID,
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           targetID,
		TargetHandle:       targetHandle,
		CycleDurationTicks: teachCycleDurationTicks,
		CycleElapsedTicks:  0,
		CycleIndex:         1,
		StartedTick:        nowTick,
	})
	ecs.MutateComponent[components.Movement](w, playerHandle, func(m *components.Movement) bool {
		m.State = constt.StateInteracting
		return true
	})
}

func (s *ContextActionService) isSyntheticTeachCyclicAction(action components.ActiveCyclicAction) bool {
	return action.BehaviorKey == "" && action.ActionID == teachContextActionID
}

func (s *ContextActionService) handleSyntheticTeachCycleComplete(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) contracts.BehaviorCycleDecision {
	if w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}

	targetHandle := action.TargetHandle
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		targetHandle = w.GetHandleByEntityID(action.TargetID)
	}
	if !s.isTeachTargetPlayer(w, playerID, action.TargetID, targetHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}

	itemDef, ok := s.resolvePlayerHandItemDef(w, playerID, playerHandle)
	if !ok {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if s.learnerHasDiscoveryKey(w, targetHandle, itemDef.Key) {
		return contracts.BehaviorCycleDecisionComplete
	}

	if !behaviors.ConsumePlayerLongActionStamina(w, playerHandle, teachCycleStaminaCost) {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}

	ecs.MutateComponent[components.CharacterProfile](w, targetHandle, func(profile *components.CharacterProfile) bool {
		profile.Discovery = components.NormalizeStringSet(append(profile.Discovery, itemDef.Key))
		return true
	})

	s.sendTeachMiniAlerts(playerID, action.TargetID)
	return contracts.BehaviorCycleDecisionComplete
}

func (s *ContextActionService) sendTeachMiniAlerts(teacherID, learnerID types.EntityID) {
	s.sendMiniAlert(teacherID, netproto.AlertSeverity_ALERT_SEVERITY_INFO, teachSuccessTeacherReasonCode)
	s.sendMiniAlert(learnerID, netproto.AlertSeverity_ALERT_SEVERITY_INFO, teachSuccessLearnerReasonCode)
}
