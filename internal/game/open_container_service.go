package game

import (
	"context"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type containerEventSender interface {
	SendContainerOpened(entityID types.EntityID, state *netproto.InventoryState)
	SendContainerClosed(entityID types.EntityID, ref *netproto.InventoryRef)
	SendInventoryUpdate(entityID types.EntityID, states []*netproto.InventoryState)
}

// OpenContainerService is a non-ECS service that listens link sync events
// and manages object container open/close lifecycle in OpenContainerState.
type OpenContainerService struct {
	world    *ecs.World
	eventBus *eventbus.EventBus
	sender   containerEventSender
	logger   *zap.Logger
}

func NewOpenContainerService(
	world *ecs.World,
	eventBus *eventbus.EventBus,
	sender containerEventSender,
	logger *zap.Logger,
) *OpenContainerService {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &OpenContainerService{
		world:    world,
		eventBus: eventBus,
		sender:   sender,
		logger:   logger,
	}

	if eventBus != nil {
		eventBus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityHigh, s.onLinkBroken)
	}
	return s
}

var _ systems.OpenContainerCoordinator = (*OpenContainerService)(nil)

func (s *OpenContainerService) HandleOpenRequest(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	ref *netproto.InventoryRef,
) *systems.OpenContainerError {
	if w != s.world {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message: "invalid world context",
		}
	}
	if ref == nil {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message: "container ref is nil",
		}
	}

	ownerID := types.EntityID(ref.OwnerId)
	kind := constt.InventoryKind(ref.Kind)
	key := ref.InventoryKey

	openState := ecs.GetResource[ecs.OpenContainerState](w)

	// Nested under currently opened world-object root must be treated as
	// world-object open flow (tracked in OpenContainerState), even if a stale
	// player-owned link still exists for this nested owner_id.
	if kind == constt.InventoryGrid && key == 0 {
		if rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID); hasRoot &&
			ownerID != playerID &&
			s.nestedBelongsToRoot(w, rootOwnerID, ownerID) {
			return s.openAnyRefForPlayer(w, playerID, kind, ownerID, key, true)
		}
	}

	// Keep legacy behavior for any player-owned inventory refs (root + nested item containers).
	if s.isPlayerOwnedRef(w, playerHandle, kind, ownerID, key) {
		return s.openAnyRefForPlayer(w, playerID, kind, ownerID, key, false)
	}
	// Fallback for personal nested refs when InventoryOwner link is not yet synchronized.
	if kind == constt.InventoryGrid && key == 0 && s.isNestedContainerOwnedByPlayer(w, playerHandle, ownerID) {
		return s.openAnyRefForPlayer(w, playerID, kind, ownerID, key, false)
	}

	// Root world object open requires active link to that object.
	if kind == constt.InventoryGrid && key == 0 && s.isContainerObjectOwner(w, ownerID) {
		linkState := ecs.GetResource[ecs.LinkState](w)
		link, hasLink := linkState.GetLink(playerID)
		if !hasLink || link.TargetID != ownerID {
			return &systems.OpenContainerError{
				Code:    netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
				Message: "active link with object is required",
			}
		}
		return s.openRootForPlayer(w, playerID, ownerID)
	}

	// Nested open: only inside currently opened root world object.
	rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID)
	if !hasRoot {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			Message: "root container is not opened",
		}
	}
	if kind != constt.InventoryGrid || key != 0 {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			Message: "only grid nested container can be opened",
		}
	}
	if !s.nestedBelongsToRoot(w, rootOwnerID, ownerID) {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			Message: "nested container is not accessible",
		}
	}

	return s.openAnyRefForPlayer(w, playerID, kind, ownerID, key, true)
}

func (s *OpenContainerService) isPlayerOwnedRef(
	w *ecs.World,
	playerHandle types.Handle,
	kind constt.InventoryKind,
	ownerID types.EntityID,
	key uint32,
) bool {
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return false
	}
	for _, link := range owner.Inventories {
		if link.Kind != kind || link.OwnerID != ownerID || link.Key != key {
			continue
		}
		return w.Alive(link.Handle)
	}
	return false
}

func (s *OpenContainerService) isNestedContainerOwnedByPlayer(
	w *ecs.World,
	playerHandle types.Handle,
	nestedOwnerID types.EntityID,
) bool {
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return false
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, link := range owner.Inventories {
		// Skip the nested container itself; we need parent containers that hold the item.
		if link.Kind == constt.InventoryGrid && link.Key == 0 && link.OwnerID == nestedOwnerID {
			continue
		}
		if !w.Alive(link.Handle) {
			continue
		}

		container, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !ok {
			continue
		}

		for _, item := range container.Items {
			if item.ItemID != nestedOwnerID {
				continue
			}
			nestedHandle, found := refIndex.Lookup(constt.InventoryGrid, nestedOwnerID, 0)
			return found && w.Alive(nestedHandle)
		}
	}

	return false
}

func (s *OpenContainerService) HandleCloseRequest(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	ref *netproto.InventoryRef,
) *systems.OpenContainerError {
	if w != s.world {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message: "invalid world context",
		}
	}
	if ref == nil {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message: "container ref is nil",
		}
	}

	key := ecs.InventoryRefKey{
		Kind:    constt.InventoryKind(ref.Kind),
		OwnerID: types.EntityID(ref.OwnerId),
		Key:     ref.InventoryKey,
	}

	openState := ecs.GetResource[ecs.OpenContainerState](w)
	rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID)
	isRootClose := hasRoot &&
		key.Kind == constt.InventoryGrid &&
		key.Key == 0 &&
		key.OwnerID == rootOwnerID
	if !isRootClose && key.Kind == constt.InventoryGrid && key.Key == 0 {
		if link, hasLink := ecs.GetResource[ecs.LinkState](w).GetLink(playerID); hasLink && link.TargetID == key.OwnerID {
			isRootClose = true
		}
	}

	if isRootClose {
		s.breakLinkForRootClose(w, playerID)
		return nil
	}

	if openState.CloseRef(playerID, key) {
		s.sender.SendContainerClosed(playerID, inventoryKeyToProto(key))
		s.markRootObjectBehaviorDirtyForRef(w, key)
	}
	return nil
}

func (s *OpenContainerService) BroadcastInventoryUpdates(
	w *ecs.World,
	actorID types.EntityID,
	updated []*netproto.InventoryState,
) {
	if w != s.world || len(updated) == 0 || s.sender == nil {
		return
	}

	openState := ecs.GetResource[ecs.OpenContainerState](w)
	perPlayer := make(map[types.EntityID]map[ecs.InventoryRefKey]*netproto.InventoryState, 8)

	for _, state := range updated {
		if state == nil || state.Ref == nil {
			continue
		}
		key := ecs.InventoryRefKey{
			Kind:    constt.InventoryKind(state.Ref.Kind),
			OwnerID: types.EntityID(state.Ref.OwnerId),
			Key:     state.Ref.InventoryKey,
		}
		players := openState.PlayersOpenedRef(key)
		if len(players) == 0 {
			continue
		}
		for playerID := range players {
			if playerID == actorID {
				continue
			}
			refs, ok := perPlayer[playerID]
			if !ok {
				refs = make(map[ecs.InventoryRefKey]*netproto.InventoryState, 4)
				perPlayer[playerID] = refs
			}
			refs[key] = state
		}
	}

	for playerID, refs := range perPlayer {
		states := make([]*netproto.InventoryState, 0, len(refs))
		for _, st := range refs {
			states = append(states, st)
		}
		if len(states) > 0 {
			s.sender.SendInventoryUpdate(playerID, states)
		}
	}
}

func (s *OpenContainerService) CloseRefsForOpenedPlayers(
	w *ecs.World,
	refs []*netproto.InventoryRef,
) {
	if w != s.world || len(refs) == 0 || s.sender == nil {
		return
	}

	openState := ecs.GetResource[ecs.OpenContainerState](w)

	for _, ref := range refs {
		if ref == nil {
			continue
		}

		key := ecs.InventoryRefKey{
			Kind:    constt.InventoryKind(ref.Kind),
			OwnerID: types.EntityID(ref.OwnerId),
			Key:     ref.InventoryKey,
		}

		players := openState.PlayersOpenedRef(key)
		if len(players) == 0 {
			continue
		}

		// Snapshot IDs first because CloseRef mutates reverse indexes.
		playerIDs := make([]types.EntityID, 0, len(players))
		for playerID := range players {
			playerIDs = append(playerIDs, playerID)
		}

		for _, playerID := range playerIDs {
			if openState.CloseRef(playerID, key) {
				s.sender.SendContainerClosed(playerID, inventoryKeyToProto(key))
				s.markRootObjectBehaviorDirtyForRef(w, key)
			}
		}
	}
}

func (s *OpenContainerService) onLinkBroken(_ context.Context, event eventbus.Event) error {
	linkEvent, ok := event.(*ecs.LinkBrokenEvent)
	if !ok || linkEvent.Layer != s.world.Layer {
		return nil
	}

	openState := ecs.GetResource[ecs.OpenContainerState](s.world)
	rootOwnerID, hasRoot := openState.GetOpenedRoot(linkEvent.PlayerID)
	closed := openState.CloseAllForPlayer(linkEvent.PlayerID)
	for _, key := range closed {
		s.sender.SendContainerClosed(linkEvent.PlayerID, inventoryKeyToProto(key))
	}
	if hasRoot {
		s.markRootObjectBehaviorDirty(s.world, rootOwnerID)
	}
	return nil
}

func (s *OpenContainerService) breakLinkForRootClose(w *ecs.World, playerID types.EntityID) {
	linkState := ecs.GetResource[ecs.LinkState](w)
	linkState.ClearIntent(playerID)

	link, removed := linkState.RemoveLink(playerID)
	openState := ecs.GetResource[ecs.OpenContainerState](w)
	rootOwnerID, hasRoot := openState.GetOpenedRoot(playerID)
	closed := openState.CloseAllForPlayer(playerID)
	for _, key := range closed {
		s.sender.SendContainerClosed(playerID, inventoryKeyToProto(key))
	}
	if hasRoot {
		s.markRootObjectBehaviorDirty(w, rootOwnerID)
	}

	if removed && s.eventBus != nil {
		if err := s.eventBus.PublishSync(ecs.NewLinkBrokenEvent(w.Layer, playerID, link.TargetID, ecs.LinkBreakClosed)); err != nil {
			s.logger.Warn("failed to publish LinkBroken(closed)",
				zap.Error(err),
				zap.Uint64("player_id", uint64(playerID)),
			)
		}
	}
}

func (s *OpenContainerService) openRootForPlayer(
	w *ecs.World,
	playerID types.EntityID,
	rootOwnerID types.EntityID,
) *systems.OpenContainerError {
	if !s.isContainerObjectOwner(w, rootOwnerID) {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			Message: "target is not a container object",
		}
	}

	openState := ecs.GetResource[ecs.OpenContainerState](w)
	if currentRoot, ok := openState.GetOpenedRoot(playerID); ok && currentRoot != rootOwnerID {
		closed := openState.CloseAllForPlayer(playerID)
		for _, key := range closed {
			s.sender.SendContainerClosed(playerID, inventoryKeyToProto(key))
		}
		s.markRootObjectBehaviorDirty(w, currentRoot)
	}

	if err := s.openAnyRefForPlayer(w, playerID, constt.InventoryGrid, rootOwnerID, 0, true); err != nil {
		return err
	}

	openState.SetRootOpened(playerID, rootOwnerID)
	s.markRootObjectBehaviorDirty(w, rootOwnerID)
	return nil
}

func (s *OpenContainerService) openAnyRefForPlayer(
	w *ecs.World,
	playerID types.EntityID,
	kind constt.InventoryKind,
	ownerID types.EntityID,
	key uint32,
	trackInOpenState bool,
) *systems.OpenContainerError {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	containerHandle, found := refIndex.Lookup(kind, ownerID, key)
	if !found || !w.Alive(containerHandle) {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message: "container not found",
		}
	}

	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if !hasContainer {
		return &systems.OpenContainerError{
			Code:    netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message: "container not found",
		}
	}

	if trackInOpenState {
		ecs.GetResource[ecs.OpenContainerState](w).OpenRef(playerID, ecs.InventoryRefKey{
			Kind:    kind,
			OwnerID: ownerID,
			Key:     key,
		})
	}

	if s.sender != nil {
		s.sender.SendContainerOpened(playerID, buildInventoryStateFromContainer(w, container))
	}
	return nil
}

func (s *OpenContainerService) isContainerObjectOwner(w *ecs.World, ownerID types.EntityID) bool {
	targetHandle := w.GetHandleByEntityID(ownerID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return false
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return false
	}
	hasContainerBehavior := false
	for _, behavior := range info.Behaviors {
		if behavior == "container" {
			hasContainerBehavior = true
			break
		}
	}
	if !hasContainerBehavior {
		return false
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, ownerID, 0)
	return found && w.Alive(rootHandle)
}

func (s *OpenContainerService) nestedBelongsToRoot(w *ecs.World, rootOwnerID, nestedOwnerID types.EntityID) bool {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, rootOwnerID, 0)
	if !found || !w.Alive(rootHandle) {
		return false
	}

	rootContainer, ok := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if !ok {
		return false
	}

	for _, item := range rootContainer.Items {
		if item.ItemID != nestedOwnerID {
			continue
		}
		nestedHandle, nestedFound := refIndex.Lookup(constt.InventoryGrid, nestedOwnerID, 0)
		return nestedFound && w.Alive(nestedHandle)
	}
	return false
}

func (s *OpenContainerService) markRootObjectBehaviorDirtyForRef(w *ecs.World, key ecs.InventoryRefKey) {
	if key.Kind != constt.InventoryGrid || key.Key != 0 {
		return
	}
	s.markRootObjectBehaviorDirty(w, key.OwnerID)
}

func (s *OpenContainerService) markRootObjectBehaviorDirty(w *ecs.World, rootOwnerID types.EntityID) {
	if rootOwnerID == 0 {
		return
	}

	handle := w.GetHandleByEntityID(rootOwnerID)
	if handle == types.InvalidHandle || !w.Alive(handle) {
		return
	}
	if _, hasObjectState := ecs.GetComponent[components.ObjectInternalState](w, handle); !hasObjectState {
		return
	}
	ecs.MarkObjectBehaviorDirty(w, handle)
}

func inventoryKeyToProto(key ecs.InventoryRefKey) *netproto.InventoryRef {
	return &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(key.Kind),
		OwnerId:      uint64(key.OwnerID),
		InventoryKey: key.Key,
	}
}

func buildInventoryStateFromContainer(w *ecs.World, container components.InventoryContainer) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	switch container.Kind {
	case constt.InventoryGrid:
		invState.Title = inventory.MustResolveGridInventoryTitle(w, container.OwnerID)
		gridItems := make([]*netproto.GridItem, 0, len(container.Items))
		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		for _, item := range container.Items {
			itemProto := &netproto.ItemInstance{
				ItemId:   uint64(item.ItemID),
				TypeId:   item.TypeID,
				Resource: item.Resource,
				Quality:  item.Quality,
				Quantity: item.Quantity,
				W:        uint32(item.W),
				H:        uint32(item.H),
			}
			if _, hasNested := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); hasNested {
				itemProto.NestedRef = &netproto.InventoryRef{
					Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
					OwnerId:      uint64(item.ItemID),
					InventoryKey: 0,
				}
			}
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: itemProto,
			})
		}
		invState.State = &netproto.InventoryState_Grid{
			Grid: &netproto.InventoryGridState{
				Width:  uint32(container.Width),
				Height: uint32(container.Height),
				Items:  gridItems,
			},
		}
	case constt.InventoryHand:
		handState := &netproto.InventoryHandState{}
		if len(container.Items) > 0 {
			item := container.Items[0]
			handState.Item = &netproto.ItemInstance{
				ItemId:   uint64(item.ItemID),
				TypeId:   item.TypeID,
				Resource: item.Resource,
				Quality:  item.Quality,
				Quantity: item.Quantity,
				W:        uint32(item.W),
				H:        uint32(item.H),
			}
			handState.HandPos = &netproto.HandPos{
				MouseOffsetX: int32(container.HandMouseOffsetX),
				MouseOffsetY: int32(container.HandMouseOffsetY),
			}
		}
		invState.State = &netproto.InventoryState_Hand{Hand: handState}
	case constt.InventoryEquipment:
		items := make([]*netproto.EquipmentItem, 0, len(container.Items))
		for _, item := range container.Items {
			items = append(items, &netproto.EquipmentItem{
				Slot: item.EquipSlot,
				Item: &netproto.ItemInstance{
					ItemId:   uint64(item.ItemID),
					TypeId:   item.TypeID,
					Resource: item.Resource,
					Quality:  item.Quality,
					Quantity: item.Quantity,
					W:        uint32(item.W),
					H:        uint32(item.H),
				},
			})
		}
		invState.State = &netproto.InventoryState_Equipment{
			Equipment: &netproto.InventoryEquipmentState{Items: items},
		}
	}

	return invState
}
