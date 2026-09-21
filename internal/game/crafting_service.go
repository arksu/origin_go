package game

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/entitystats"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	"origin/internal/game/stationreq"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const craftSyntheticActionID = "craft"

type craftRuntimeSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
	SendInventoryUpdate(entityID types.EntityID, states []*netproto.InventoryState)
	SendExpGained(entityID types.EntityID, gained *netproto.S2C_ExpGained)
	SendCraftList(entityID types.EntityID, list *netproto.S2C_CraftList)
}

type CraftingService struct {
	world               *ecs.World
	eventBus            *eventbus.EventBus
	invExec             *inventory.InventoryExecutor
	sender              craftRuntimeSender
	logger              *zap.Logger
	stationRequirements *stationreq.Evaluator
}

func NewCraftingService(
	world *ecs.World,
	eventBus *eventbus.EventBus,
	invExec *inventory.InventoryExecutor,
	sender craftRuntimeSender,
	logger *zap.Logger,
) *CraftingService {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &CraftingService{
		world:               world,
		eventBus:            eventBus,
		invExec:             invExec,
		sender:              sender,
		logger:              logger,
		stationRequirements: stationreq.NewEvaluator(),
	}
	if eventBus != nil {
		eventBus.SubscribeSync(ecs.TopicGameplayLinkCreated, eventbus.PriorityLow, s.onLinkStateChanged)
		eventBus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityLow, s.onLinkStateChanged)
		eventBus.SubscribeSync(ecs.TopicGameplayStationStateChanged, eventbus.PriorityLow, s.onStationStateChanged)
	}
	return s
}

func (s *CraftingService) IsSyntheticCraftAction(action components.ActiveCyclicAction) bool {
	return action.BehaviorKey == "" && action.ActionID == craftSyntheticActionID
}

func (s *CraftingService) HandleStartCraftOne(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, msg *netproto.C2S_StartCraftOne) {
	if msg == nil {
		return
	}
	s.startCraft(w, playerID, playerHandle, strings.TrimSpace(msg.CraftKey), 1)
}

func (s *CraftingService) HandleStartCraftMany(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, msg *netproto.C2S_StartCraftMany) {
	if msg == nil {
		return
	}
	cycles := msg.Cycles
	if cycles == 0 {
		cycles = 1
	}
	s.startCraft(w, playerID, playerHandle, strings.TrimSpace(msg.CraftKey), cycles)
}

func (s *CraftingService) startCraft(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	craftKey string,
	cycles uint32,
) {
	if s == nil || w == nil || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) || cycles == 0 {
		return
	}
	if _, has := ecs.GetComponent[components.ActiveCyclicAction](w, playerHandle); has {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "ACTION_BUSY")
		return
	}
	reg := craftdefs.Global()
	if reg == nil {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_ERROR, "CRAFT_UNAVAILABLE")
		return
	}
	craft, ok := reg.GetByKey(craftKey)
	if !ok || craft == nil {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_NOT_FOUND")
		return
	}
	if !s.isCraftVisible(w, playerHandle, craft) {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_REQUIREMENTS_NOT_MET")
		return
	}

	targetID, targetHandle, hasLinkObj := s.resolveRequiredLinkedObject(w, playerID, craft)
	if craftNeedsLinkedObject(craft) && !hasLinkObj {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_REQUIRES_LINKED_OBJECT")
		return
	}
	if !s.evaluateStationRequirements(w, playerID, targetID, craft).Passed {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_STATION_REQUIREMENTS_NOT_MET")
		return
	}
	prepared := s.prepareCraftCycle(w, playerID, playerHandle, craft)
	if prepared.failureCode != "" {
		s.sendCraftPreparationFailure(playerID, prepared)
		return
	}

	nowTick := ecs.GetResource[ecs.TimeState](w).Tick
	targetKind := components.CyclicActionTargetSelf
	if craftNeedsLinkedObject(craft) {
		targetKind = components.CyclicActionTargetObject
	}
	ecs.AddComponent(w, playerHandle, components.ActiveCraft{
		CraftKey:        craft.Key,
		RequestedCycles: cycles,
		RemainingCycles: cycles,
	})
	ecs.AddComponent(w, playerHandle, components.ActiveCyclicAction{
		ActionID:           craftSyntheticActionID,
		TargetKind:         targetKind,
		TargetID:           targetID,
		TargetHandle:       targetHandle,
		CycleDurationTicks: craft.TicksRequired,
		CycleElapsedTicks:  0,
		CycleIndex:         1,
		StartedTick:        nowTick,
	})
	ecs.MutateComponent[components.Movement](w, playerHandle, func(m *components.Movement) bool {
		m.State = constt.StateInteracting
		return true
	})
	s.SendCraftListSnapshot(w, playerID, playerHandle)
}

func (s *CraftingService) HandleCraftCycleComplete(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) contracts.BehaviorCycleDecision {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	activeCraft, hasActiveCraft := ecs.GetComponent[components.ActiveCraft](w, playerHandle)
	if !hasActiveCraft || activeCraft.CraftKey == "" {
		return contracts.BehaviorCycleDecisionCanceled
	}
	craft, ok := craftdefs.Global().GetByKey(activeCraft.CraftKey)
	if !ok || craft == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if activeCraft.RemainingCycles == 0 {
		ecs.RemoveComponent[components.ActiveCraft](w, playerHandle)
		s.SendCraftListSnapshot(w, playerID, playerHandle)
		return contracts.BehaviorCycleDecisionComplete
	}
	if !s.isCraftActionLinkValid(w, playerID, action, craft) {
		return contracts.BehaviorCycleDecisionCanceled
	}

	stationRequirements := s.evaluateStationRequirements(w, playerID, action.TargetID, craft)
	if !stationRequirements.Passed {
		s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_STATION_REQUIREMENTS_NOT_MET")
		s.SendCraftListSnapshot(w, playerID, playerHandle)
		return contracts.BehaviorCycleDecisionCanceled
	}
	prepared := s.prepareCraftCycle(w, playerID, playerHandle, craft)
	if prepared.failureCode != "" {
		s.sendCraftPreparationFailure(playerID, prepared)
		s.SendCraftListSnapshot(w, playerID, playerHandle)
		return contracts.BehaviorCycleDecisionCanceled
	}

	stationHandle := action.TargetHandle
	if stationHandle == types.InvalidHandle || !w.Alive(stationHandle) {
		stationHandle = w.GetHandleByEntityID(action.TargetID)
	}
	snapshot := captureCraftCycleSnapshot(w, playerHandle, stationHandle)
	rollback := func(severity netproto.AlertSeverity, reasonCode string) contracts.BehaviorCycleDecision {
		snapshot.restore(w, playerHandle, stationHandle)
		s.sendMiniAlert(playerID, severity, reasonCode)
		s.SendCraftListSnapshot(w, playerID, playerHandle)
		return contracts.BehaviorCycleDecisionCanceled
	}

	consume := s.invExec.CommitPreparedCraftInputs(w, playerID, playerHandle, &prepared.inputs)
	if consume.Overflow {
		return rollback(netproto.AlertSeverity_ALERT_SEVERITY_ERROR, "CRAFT_QUALITY_OVERFLOW")
	}
	if !consume.Success {
		return rollback(netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_MISSING_INPUTS")
	}
	if !behaviors.ConsumePlayerLongActionStamina(w, playerHandle, craft.StaminaCost) {
		return rollback(netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "LOW_STAMINA")
	}
	if !consumeStationResources(w, stationHandle, stationRequirements.Consumptions) {
		return rollback(netproto.AlertSeverity_ALERT_SEVERITY_WARNING, "CRAFT_STATION_REQUIREMENTS_NOT_MET")
	}

	updated := consume.UpdatedContainers
	var discoveryLP int64
	stopAfterCycle := false
	for _, out := range prepared.outputs {
		give := s.invExec.GiveItem(w, playerID, playerHandle, out.ItemKey, out.Count, prepared.quality)
		if give == nil || !give.Success || give.GrantedCount != out.Count {
			return rollback(netproto.AlertSeverity_ALERT_SEVERITY_ERROR, "CRAFT_OUTPUT_CREATION_FAILED")
		}
		updated = mergeCraftUpdatedContainers(updated, give.UpdatedContainers)
		discoveryLP += give.DiscoveryLPGained
	}

	if len(updated) > 0 {
		states := s.invExec.ConvertContainersToStates(w, updated)
		protoStates := make([]*netproto.InventoryState, 0, len(states))
		for _, st := range states {
			protoStates = append(protoStates, systems.BuildInventoryStateProto(st))
		}
		if len(protoStates) > 0 && s.sender != nil {
			s.sender.SendInventoryUpdate(playerID, protoStates)
		}
	}
	if discoveryLP > 0 && s.sender != nil {
		lp := discoveryLP
		s.sender.SendExpGained(playerID, &netproto.S2C_ExpGained{
			EntityId: uint64(playerID),
			Lp:       &lp,
		})
	}

	nextRemaining := activeCraft.RemainingCycles - 1
	shouldStop := stopAfterCycle || activeCraft.StopAfterCurrentCycle || nextRemaining == 0
	if shouldStop {
		ecs.RemoveComponent[components.ActiveCraft](w, playerHandle)
		s.refreshCraftSnapshotsAfterCompletion(w, playerID, playerHandle, action.TargetID)
		return contracts.BehaviorCycleDecisionComplete
	}

	ecs.MutateComponent[components.ActiveCraft](w, playerHandle, func(ac *components.ActiveCraft) bool {
		ac.RemainingCycles = nextRemaining
		ac.StopAfterCurrentCycle = stopAfterCycle
		return true
	})
	if !s.isCraftActionLinkValid(w, playerID, action, craft) || !s.evaluateStationRequirements(w, playerID, action.TargetID, craft).Passed {
		ecs.RemoveComponent[components.ActiveCraft](w, playerHandle)
		s.refreshCraftSnapshotsAfterCompletion(w, playerID, playerHandle, action.TargetID)
		return contracts.BehaviorCycleDecisionComplete
	}
	next := s.prepareCraftCycle(w, playerID, playerHandle, craft)
	if next.failureCode != "" {
		s.sendCraftPreparationFailure(playerID, next)
		ecs.RemoveComponent[components.ActiveCraft](w, playerHandle)
		s.refreshCraftSnapshotsAfterCompletion(w, playerID, playerHandle, action.TargetID)
		return contracts.BehaviorCycleDecisionComplete
	}
	s.refreshCraftSnapshotsAfterCompletion(w, playerID, playerHandle, action.TargetID)
	return contracts.BehaviorCycleDecisionContinue
}

func (s *CraftingService) IsActiveCraftStillValid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) bool {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return false
	}
	activeCraft, hasActiveCraft := ecs.GetComponent[components.ActiveCraft](w, playerHandle)
	if !hasActiveCraft || activeCraft.CraftKey == "" {
		return false
	}
	craft, ok := craftdefs.Global().GetByKey(activeCraft.CraftKey)
	if !ok || craft == nil {
		return false
	}
	if !s.isCraftVisible(w, playerHandle, craft) {
		return false
	}
	return s.isCraftActionLinkValid(w, playerID, action, craft)
}

func (s *CraftingService) SendCraftListSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	if s == nil || s.sender == nil || w == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return
	}
	if !ecs.GetResource[ecs.OpenedWindowsState](w).IsOpen(entityID, "craft") {
		return
	}
	list := &netproto.S2C_CraftList{Recipes: s.buildCraftList(w, entityID, handle)}
	s.sender.SendCraftList(entityID, list)
}

func (s *CraftingService) buildCraftList(w *ecs.World, playerID types.EntityID, playerHandle types.Handle) []*netproto.CraftRecipeEntry {
	reg := craftdefs.Global()
	if reg == nil {
		return nil
	}
	all := reg.All()
	if len(all) == 0 {
		return nil
	}
	hasInvExec := s.invExec != nil
	out := make([]*netproto.CraftRecipeEntry, 0, len(all))
	for _, craft := range all {
		if craft == nil || !s.isCraftVisible(w, playerHandle, craft) {
			continue
		}
		flags := &netproto.CraftRequirementFlags{}
		stationID, _, hasLink := s.resolveRequiredLinkedObject(w, playerID, craft)
		flags.HasRequiredLinkedObject = hasLink
		stationEvaluation := s.evaluateStationRequirements(w, playerID, stationID, craft)
		flags.HasStationRequirements = len(craft.StationRequirements) > 0
		flags.StationRequirementsMet = !flags.HasStationRequirements || stationEvaluation.Passed
		if flags.HasStationRequirements && !stationEvaluation.Passed {
			failureCode := stationEvaluation.FailureCode
			flags.StationFailureCode = &failureCode
		}
		var inputs inventory.CraftConsumeInputsResult
		if hasInvExec {
			inputs = s.invExec.PreviewCraftInputs(w, playerID, playerHandle, craft)
		}
		flags.HasInputs = inputs.Success && !inputs.Overflow
		flags.HasStamina = s.hasCraftStamina(w, playerHandle, craft.StaminaCost)
		outputs, outputError := inventory.ResolveCraftOutputs(craft, inputs)
		quality := s.computeCraftQuality(craft, inputs.QualityWeighted, inputs.QualityWeightSum)
		flags.HasOutputSpace = hasInvExec && outputError == nil && quality != nil && s.invExec.CanFitResolvedCraftOutputs(w, playerID, playerHandle, outputs, *quality)
		flags.CanStartNow = flags.HasRequiredLinkedObject && flags.StationRequirementsMet && flags.HasInputs && flags.HasStamina && flags.HasOutputSpace

		entry := &netproto.CraftRecipeEntry{
			CraftKey:            craft.Key,
			Name:                craft.Name,
			StaminaCost:         craft.StaminaCost,
			TicksRequired:       craft.TicksRequired,
			RequiredSkills:      append([]string(nil), craft.RequiredSkills...),
			RequiredDiscovery:   append([]string(nil), craft.RequiredDiscovery...),
			QualityFormula:      craft.QualityFormula,
			Flags:               flags,
			Inputs:              make([]*netproto.CraftInputDef, 0, len(craft.Inputs)),
			Outputs:             make([]*netproto.CraftOutputDef, 0, len(craft.Outputs)),
			StationRequirements: buildCraftStationRequirements(craft.StationRequirements),
		}
		if craft.RequiredLinkedObject != "" {
			key := craft.RequiredLinkedObject
			entry.RequiredLinkedObjectKey = &key
		}
		for _, in := range craft.Inputs {
			inputDef := &netproto.CraftInputDef{
				Count:         in.Count,
				QualityWeight: in.QualityWeight,
			}
			if in.ItemKey != "" {
				key := in.ItemKey
				inputDef.ItemKey = &key
			}
			if in.ItemTag != "" {
				tag := in.ItemTag
				inputDef.ItemTag = &tag
			}
			entry.Inputs = append(entry.Inputs, inputDef)
		}
		for _, o := range craft.Outputs {
			entry.Outputs = append(entry.Outputs, &netproto.CraftOutputDef{
				ItemKey: o.ItemKey,
				Count:   o.Count,
			})
		}
		out = append(out, entry)
	}
	return out
}

func buildCraftStationRequirements(requirements []craftdefs.StationRequirement) []*netproto.CraftStationRequirementDef {
	if len(requirements) == 0 {
		return nil
	}

	out := make([]*netproto.CraftStationRequirementDef, 0, len(requirements))
	for _, requirement := range requirements {
		entry := &netproto.CraftStationRequirementDef{
			Capability: requirement.Capability,
			Conditions: make([]*netproto.CraftStationConditionDef, 0, len(requirement.Conditions)),
			Consume:    make([]*netproto.CraftStationResourceConsumptionDef, 0, len(requirement.Consume)),
		}
		if requirement.State != "" {
			state := requirement.State
			entry.State = &state
		}
		for _, condition := range requirement.Conditions {
			entry.Conditions = append(entry.Conditions, &netproto.CraftStationConditionDef{
				Source:   condition.Source,
				Kind:     condition.Kind,
				Key:      condition.Key,
				Operator: condition.Operator,
				Value:    condition.Value,
			})
		}
		for _, consumption := range requirement.Consume {
			entry.Consume = append(entry.Consume, &netproto.CraftStationResourceConsumptionDef{
				ResourceKey: consumption.ResourceKey,
				Amount:      consumption.Amount,
			})
		}
		out = append(out, entry)
	}
	return out
}

func (s *CraftingService) isCraftVisible(w *ecs.World, playerHandle types.Handle, craft *craftdefs.CraftDef) bool {
	if w == nil || playerHandle == types.InvalidHandle || craft == nil {
		return false
	}
	profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, playerHandle)
	if !hasProfile {
		return false
	}
	if !containsAllStrings(profile.Skills, craft.RequiredSkills) {
		return false
	}
	if !containsAllStrings(profile.Discovery, craft.RequiredDiscovery) {
		return false
	}
	return true
}

func (s *CraftingService) evaluateStationRequirements(
	w *ecs.World,
	playerID types.EntityID,
	stationID types.EntityID,
	craft *craftdefs.CraftDef,
) stationreq.Evaluation {
	if craft == nil || len(craft.StationRequirements) == 0 {
		return stationreq.Evaluation{Passed: true}
	}
	if s == nil || s.stationRequirements == nil {
		return stationreq.Evaluation{}
	}
	return s.stationRequirements.Evaluate(stationreq.Context{
		World:     w,
		ActorID:   playerID,
		StationID: stationID,
	}, craft.StationRequirements)
}

type craftCyclePreparation struct {
	inputs      inventory.CraftConsumeInputsResult
	outputs     []craftdefs.CraftOutput
	quality     uint32
	failureCode string
	message     string
}

func (s *CraftingService) prepareCraftCycle(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, craft *craftdefs.CraftDef) craftCyclePreparation {
	result := craftCyclePreparation{}
	if s.invExec == nil {
		result.failureCode = "CRAFT_UNAVAILABLE"
		return result
	}
	result.inputs = s.invExec.PreviewCraftInputs(w, playerID, playerHandle, craft)
	if result.inputs.Overflow {
		result.failureCode = "CRAFT_QUALITY_OVERFLOW"
		return result
	}
	if !result.inputs.Success {
		result.failureCode = "CRAFT_MISSING_INPUTS"
		return result
	}
	var err error
	result.outputs, err = inventory.ResolveCraftOutputs(craft, result.inputs)
	if err != nil {
		var missing *inventory.CraftMapEntryMissingError
		if errors.As(err, &missing) {
			result.failureCode = "CRAFT_ROAST_MAP_ENTRY_MISSING"
			// Roast-specific player text pinned by the roasted-meat spec; the mapped-output mechanism itself stays recipe-agnostic.
			result.message = fmt.Sprintf("Roast can't be processed: no info %s in roast map", missing.SourceItemKey)
		} else {
			result.failureCode = "CRAFT_OUTPUT_INVALID"
		}
		return result
	}
	quality := s.computeCraftQuality(craft, result.inputs.QualityWeighted, result.inputs.QualityWeightSum)
	if quality == nil {
		result.failureCode = "CRAFT_QUALITY_FORMULA_UNSUPPORTED"
		return result
	}
	result.quality = *quality
	if !s.hasCraftStamina(w, playerHandle, craft.StaminaCost) {
		result.failureCode = "LOW_STAMINA"
		return result
	}
	if !s.invExec.CanFitResolvedCraftOutputs(w, playerID, playerHandle, result.outputs, result.quality) {
		result.failureCode = "CRAFT_NO_SPACE"
	}
	return result
}

func (s *CraftingService) sendCraftPreparationFailure(playerID types.EntityID, prepared craftCyclePreparation) {
	severity := netproto.AlertSeverity_ALERT_SEVERITY_ERROR
	switch prepared.failureCode {
	case "CRAFT_MISSING_INPUTS", "LOW_STAMINA", "CRAFT_NO_SPACE":
		severity = netproto.AlertSeverity_ALERT_SEVERITY_WARNING
	}
	s.sendMiniAlert(playerID, severity, prepared.failureCode, prepared.message)
}

type craftCycleSnapshot struct {
	inventoryOwner    components.InventoryOwner
	hasInventoryOwner bool
	inventories       map[types.Handle]components.InventoryContainer

	profile     components.CharacterProfile
	hasProfile  bool
	stats       components.EntityStats
	hasStats    bool
	movement    components.Movement
	hasMovement bool

	station         components.StationState
	hasStation      bool
	stationInternal components.ObjectInternalState
	hasInternal     bool
}

func captureCraftCycleSnapshot(w *ecs.World, playerHandle, stationHandle types.Handle) craftCycleSnapshot {
	snapshot := craftCycleSnapshot{inventories: make(map[types.Handle]components.InventoryContainer)}
	if w == nil {
		return snapshot
	}
	if owner, ok := ecs.GetComponent[components.InventoryOwner](w, playerHandle); ok {
		snapshot.hasInventoryOwner = true
		snapshot.inventoryOwner = owner
		snapshot.inventoryOwner.Inventories = append([]components.InventoryLink(nil), owner.Inventories...)
		for _, link := range owner.Inventories {
			container, exists := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
			if !exists {
				continue
			}
			container.Items = append([]components.InvItem(nil), container.Items...)
			snapshot.inventories[link.Handle] = container
		}
	}
	if profile, ok := ecs.GetComponent[components.CharacterProfile](w, playerHandle); ok {
		snapshot.hasProfile = true
		snapshot.profile = profile
		snapshot.profile.Skills = append([]string(nil), profile.Skills...)
		snapshot.profile.Discovery = append([]string(nil), profile.Discovery...)
	}
	if stats, ok := ecs.GetComponent[components.EntityStats](w, playerHandle); ok {
		snapshot.hasStats = true
		snapshot.stats = stats
	}
	if movement, ok := ecs.GetComponent[components.Movement](w, playerHandle); ok {
		snapshot.hasMovement = true
		snapshot.movement = movement
	}
	if station, ok := ecs.GetComponent[components.StationState](w, stationHandle); ok {
		snapshot.hasStation = true
		snapshot.station = station.Snapshot()
	}
	if internal, ok := ecs.GetComponent[components.ObjectInternalState](w, stationHandle); ok {
		snapshot.hasInternal = true
		snapshot.stationInternal = internal
	}
	return snapshot
}

func (s craftCycleSnapshot) restore(w *ecs.World, playerHandle, stationHandle types.Handle) {
	if w == nil {
		return
	}
	if s.hasInventoryOwner {
		if currentOwner, ok := ecs.GetComponent[components.InventoryOwner](w, playerHandle); ok {
			for _, link := range currentOwner.Inventories {
				if _, existed := s.inventories[link.Handle]; existed {
					continue
				}
				ecs.GetResource[ecs.InventoryRefIndex](w).Remove(link.Kind, link.OwnerID, link.Key)
				if w.Alive(link.Handle) {
					w.Despawn(link.Handle)
				}
			}
		}
		ecs.AddComponent(w, playerHandle, s.inventoryOwner)
		for handle, container := range s.inventories {
			if w.Alive(handle) {
				ecs.AddComponent(w, handle, container)
			}
		}
	}
	if s.hasProfile {
		ecs.AddComponent(w, playerHandle, s.profile)
	}
	if s.hasStats {
		ecs.AddComponent(w, playerHandle, s.stats)
	}
	if s.hasMovement {
		ecs.AddComponent(w, playerHandle, s.movement)
	}
	if s.hasStation && w.Alive(stationHandle) {
		ecs.AddComponent(w, stationHandle, s.station)
	}
	if s.hasInternal && w.Alive(stationHandle) {
		ecs.AddComponent(w, stationHandle, s.stationInternal)
	}
}

func consumeStationResources(w *ecs.World, stationHandle types.Handle, consumptions map[string]uint32) bool {
	if len(consumptions) == 0 {
		return true
	}
	if w == nil || stationHandle == types.InvalidHandle || !w.Alive(stationHandle) {
		return false
	}
	station, exists := ecs.GetComponent[components.StationState](w, stationHandle)
	if !exists {
		return false
	}
	for resourceKey, amount := range consumptions {
		if amount == 0 || station.Resources[resourceKey] < amount {
			return false
		}
	}
	ecs.MutateComponent[components.StationState](w, stationHandle, func(state *components.StationState) bool {
		for resourceKey, amount := range consumptions {
			state.Resources[resourceKey] -= amount
		}
		return true
	})
	ecs.MutateComponent[components.ObjectInternalState](w, stationHandle, func(state *components.ObjectInternalState) bool {
		state.IsDirty = true
		return true
	})
	return true
}

func containsAllStrings(have []string, need []string) bool {
	if len(need) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, v := range have {
		set[v] = struct{}{}
	}
	for _, v := range need {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}

func (s *CraftingService) resolveRequiredLinkedObject(
	w *ecs.World,
	playerID types.EntityID,
	craft *craftdefs.CraftDef,
) (types.EntityID, types.Handle, bool) {
	if !craftNeedsLinkedObject(craft) {
		return 0, types.InvalidHandle, true
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	link, hasLink := linkState.GetLink(playerID)
	if !hasLink {
		return 0, types.InvalidHandle, false
	}
	targetHandle := link.TargetHandle
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		targetHandle = w.GetHandleByEntityID(link.TargetID)
	}
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return 0, types.InvalidHandle, false
	}
	if craft.RequiredLinkedObject != "" && !s.isHandleObjectKey(w, targetHandle, craft.RequiredLinkedObject) {
		return 0, types.InvalidHandle, false
	}
	return link.TargetID, targetHandle, true
}

func craftNeedsLinkedObject(craft *craftdefs.CraftDef) bool {
	return craft != nil && (craft.RequiredLinkedObject != "" || len(craft.StationRequirements) > 0)
}

func (s *CraftingService) isCraftActionLinkValid(w *ecs.World, playerID types.EntityID, action components.ActiveCyclicAction, craft *craftdefs.CraftDef) bool {
	if !craftNeedsLinkedObject(craft) {
		return true
	}
	targetID, _, valid := s.resolveRequiredLinkedObject(w, playerID, craft)
	return valid && targetID == action.TargetID && action.TargetKind == components.CyclicActionTargetObject
}

func (s *CraftingService) isHandleObjectKey(w *ecs.World, handle types.Handle, requiredObjectKey string) bool {
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasInfo {
		return false
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def == nil {
		return false
	}
	return def.Key == requiredObjectKey
}

func (s *CraftingService) hasCraftStamina(w *ecs.World, playerHandle types.Handle, cost float64) bool {
	if cost <= 0 {
		return true
	}
	stats, hasStats := ecs.GetComponent[components.EntityStats](w, playerHandle)
	if !hasStats {
		return true
	}
	con := characterattrs.DefaultValue
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, playerHandle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	maxStamina := entitystats.MaxStaminaFromCon(con)
	currentStamina := entitystats.ClampStamina(stats.Stamina, maxStamina)
	return entitystats.CanConsumeLongActionStamina(currentStamina, maxStamina, cost)
}

func (s *CraftingService) computeCraftQuality(craft *craftdefs.CraftDef, weighted, weightSum uint64) *uint32 {
	if craft == nil {
		return nil
	}
	if weightSum == 0 {
		q := uint32(0)
		return &q
	}
	switch craft.QualityFormula {
	case "", craftdefs.QualityFormulaWeightedAverageFloor:
		value := weighted / weightSum
		if value > math.MaxUint32 {
			max := uint32(math.MaxUint32)
			return &max
		}
		q := uint32(value)
		return &q
	default:
		return nil
	}
}

func (s *CraftingService) sendMiniAlert(entityID types.EntityID, severity netproto.AlertSeverity, reasonCode string, message ...string) {
	if s == nil || s.sender == nil || reasonCode == "" {
		return
	}
	alert := &netproto.S2C_MiniAlert{Severity: severity, ReasonCode: reasonCode, TtlMs: ttlBySeverity(severity)}
	if len(message) > 0 && message[0] != "" {
		alert.Message = &message[0]
	}
	s.sender.SendMiniAlert(entityID, alert)
}

func (s *CraftingService) onLinkStateChanged(_ context.Context, event eventbus.Event) error {
	switch ev := event.(type) {
	case *ecs.LinkCreatedEvent:
		if ev.Layer != s.world.Layer {
			return nil
		}
		handle := s.world.GetHandleByEntityID(ev.PlayerID)
		if handle != types.InvalidHandle {
			s.SendCraftListSnapshot(s.world, ev.PlayerID, handle)
		}
	case *ecs.LinkBrokenEvent:
		if ev.Layer != s.world.Layer {
			return nil
		}
		handle := s.world.GetHandleByEntityID(ev.PlayerID)
		if handle != types.InvalidHandle {
			s.SendCraftListSnapshot(s.world, ev.PlayerID, handle)
		}
	}
	return nil
}

func (s *CraftingService) onStationStateChanged(_ context.Context, event eventbus.Event) error {
	ev, ok := event.(*ecs.StationStateChangedEvent)
	if !ok || s == nil || s.world == nil || ev.Layer != s.world.Layer {
		return nil
	}
	s.sendCraftListSnapshotsToLinkedPlayers(s.world, ev.StationID, 0)
	return nil
}

func (s *CraftingService) refreshCraftSnapshotsAfterCompletion(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	stationID types.EntityID,
) {
	s.SendCraftListSnapshot(w, playerID, playerHandle)
	s.sendCraftListSnapshotsToLinkedPlayers(w, stationID, playerID)
}

func (s *CraftingService) sendCraftListSnapshotsToLinkedPlayers(w *ecs.World, stationID, skipPlayerID types.EntityID) {
	if s == nil || w == nil || w != s.world || stationID == 0 {
		return
	}
	players := ecs.GetResource[ecs.LinkState](w).PlayersByTarget[stationID]
	if len(players) == 0 {
		return
	}
	playerIDs := make([]types.EntityID, 0, len(players))
	for playerID := range players {
		if playerID != skipPlayerID {
			playerIDs = append(playerIDs, playerID)
		}
	}
	slices.Sort(playerIDs)
	for _, playerID := range playerIDs {
		handle := w.GetHandleByEntityID(playerID)
		if handle != types.InvalidHandle {
			s.SendCraftListSnapshot(w, playerID, handle)
		}
	}
}

func mergeCraftUpdatedContainers(
	existing []*inventory.ContainerInfo,
	updated []*inventory.ContainerInfo,
) []*inventory.ContainerInfo {
	if len(updated) == 0 {
		return existing
	}
	indexByHandle := make(map[types.Handle]int, len(existing)+len(updated))
	for i, info := range existing {
		if info != nil {
			indexByHandle[info.Handle] = i
		}
	}
	for _, info := range updated {
		if info == nil {
			continue
		}
		if idx, ok := indexByHandle[info.Handle]; ok {
			existing[idx] = info
			continue
		}
		indexByHandle[info.Handle] = len(existing)
		existing = append(existing, info)
	}
	return existing
}
