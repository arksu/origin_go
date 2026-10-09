package inventory

import (
	"encoding/json"
	"errors"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

var (
	ErrSkullGrantNoSpace = errors.New("no free space for skull")
	ErrInvalidSkullGrant = errors.New("invalid prepared skull grant")
)

// PreparedSkullGrant retains placement and an owned, incrementally captured
// inventory tree. It does not allocate an item ID or mutate inventory before a
// durable claim succeeds. The caller reserves the recipient before capture.
type PreparedSkullGrant struct {
	executor    *InventoryExecutor
	world       *ecs.World
	recipientID types.EntityID
	recipient   types.Handle
	destination ecs.InventoryRefEntry
	version     uint64
	x, y        uint8
	definition  *itemdefs.ItemDef
	capture     *ObjectLootCapture
	rootIndex   int
	built       *components.InvItem
	applied     bool
}

func (e *InventoryExecutor) PrepareSkullGrant(w *ecs.World, recipientID types.EntityID, recipient types.Handle) (*PreparedSkullGrant, error) {
	if e == nil || e.service == nil || w == nil || recipientID == 0 || !w.Alive(recipient) || w.GetHandleByEntityID(recipientID) != recipient ||
		playerstate.ItemsLocked(w, recipient) || ecs.InventoryOwnerReserved(w, recipientID) || ecs.ObjectDestructionPending(w, recipient) {
		return nil, ErrInvalidSkullGrant
	}
	owner, ok := ecs.GetComponent[components.InventoryOwner](w, recipient)
	registry := itemdefs.Global()
	if !ok || registry == nil {
		return nil, ErrInvalidSkullGrant
	}
	definition, ok := registry.GetByKey("skull")
	if !ok || definition == nil || definition.Container != nil || definition.Size.W != 1 || definition.Size.H != 1 ||
		definition.Stack != nil && definition.Stack.Mode != itemdefs.StackModeNone {
		return nil, ErrInvalidSkullGrant
	}
	item := components.InvItem{TypeID: uint32(definition.DefID), Quantity: 1, W: 1, H: 1}
	links := orderedGridLinks(owner.Inventories, recipientID, defaultGivePlacementPolicy)
	links = filterPersonalInventoryLinks(w, recipientID, recipient, links)
	if hand, hasHand := playerHandLink(owner, recipientID); hasHand {
		links = append(links, hand)
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, link := range links {
		handle, indexed := refs.Lookup(link.Kind, link.OwnerID, link.Key)
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !indexed || handle != link.Handle || !w.Alive(handle) || !hasContainer || container.OwnerID != link.OwnerID ||
			container.Kind != link.Kind || container.Key != link.Key || container.Version >= uint64(math.MaxInt) {
			continue
		}
		// The recipient's root cannot have item content rules. For a personal
		// nested container resolve its parent within the owner, never scan World.
		if container.Kind == constt.InventoryGrid {
			if definition.Allowed.Grid != nil && !*definition.Allowed.Grid {
				continue
			}
			if container.OwnerID != recipientID {
				parentType := e.service.validator.findItemTypeIDInOwner(w, &owner, container.OwnerID)
				parent, found := registry.GetByID(int(parentType))
				if !found || parent.Container == nil || e.service.validator.checkContentRules(&parent.Container.Rules, definition) != nil {
					continue
				}
			}
		} else if container.Kind != constt.InventoryHand || len(container.Items) != 0 || definition.Allowed.Hand != nil && !*definition.Allowed.Hand {
			continue
		}
		x, y := uint8(0), uint8(0)
		if container.Kind == constt.InventoryGrid {
			found, px, py := e.service.placementService.FindFreeSpace(&container, item.W, item.H)
			if !found {
				continue
			}
			x, y = px, py
		}
		roots := refs.EntriesByOwnerInto(recipientID, nil)
		if len(roots) == 0 {
			return nil, ErrInvalidSkullGrant
		}
		return &PreparedSkullGrant{executor: e, world: w, recipientID: recipientID, recipient: recipient,
			destination: ecs.InventoryRefEntry{InventoryRefKey: ecs.InventoryRefKey{Kind: link.Kind, OwnerID: link.OwnerID, Key: link.Key}, Handle: handle},
			version:     container.Version, x: x, y: y, definition: definition,
			capture: newReservedInventoryCapture(recipientID, recipient, roots), rootIndex: -1,
		}, nil
	}
	return nil, ErrSkullGrantNoSpace
}

// CaptureBatch must run under the recipient shard's read lock after reservation.
// It copies at most budget item/container/verification records on each call.
func (p *PreparedSkullGrant) CaptureBatch(w *ecs.World, budget int) (bool, error) {
	if p == nil || p.world != w || p.capture == nil || p.built != nil || p.applied {
		return false, ErrInvalidSkullGrant
	}
	return p.capture.CaptureBatch(w, itemdefs.Global(), budget)
}

// Build runs on the persistence worker over owned data, with no live ECS access.
// Call it once and reuse its exact result for transaction retries.
func (p *PreparedSkullGrant) Build(item components.InvItem) ([]systems.InventorySnapshot, error) {
	if p == nil || p.capture == nil || !p.capture.done || p.capture.err != nil || p.built != nil ||
		item.ItemID == 0 || item.ItemID > math.MaxInt64 || item.TypeID != uint32(p.definition.DefID) || item.Quantity != 1 ||
		item.W != 1 || item.H != 1 || item.Skull == nil || item.Skull.HintExt() == "" {
		return nil, ErrInvalidSkullGrant
	}
	if _, duplicate := p.capture.seenItems[item.ItemID]; duplicate {
		return nil, ErrInvalidSkullGrant
	}
	type node struct {
		data  *InventoryDataV1
		owner types.EntityID
		root  int
	}
	stack := make([]node, 0, len(p.capture.rootTrees))
	for index := range p.capture.rootTrees {
		stack = append(stack, node{&p.capture.rootTrees[index], p.recipientID, index})
	}
	var destination *InventoryDataV1
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.owner == p.destination.OwnerID && current.data.Kind == uint8(p.destination.Kind) && current.data.Key == p.destination.Key {
			if destination != nil || current.data.Version != int(p.version) {
				return nil, ErrInvalidSkullGrant
			}
			destination, p.rootIndex = current.data, current.root
		}
		for index := range current.data.Items {
			child := &current.data.Items[index]
			if child.NestedInventory != nil {
				stack = append(stack, node{child.NestedInventory, types.EntityID(child.ItemID), current.root})
			}
		}
	}
	if destination == nil || destination.Version == math.MaxInt || p.capture.rootTrees[p.rootIndex].Version == math.MaxInt {
		return nil, ErrInvalidSkullGrant
	}
	item.X, item.Y, item.EquipSlot = p.x, p.y, 0
	item.Resource = p.definition.ResolveResource(false)
	item.Skull = item.Skull.Clone()
	destination.Items = append(destination.Items, InventoryItemV1{ItemID: uint64(item.ItemID), TypeID: item.TypeID,
		Quality: item.Quality, Quantity: 1, X: item.X, Y: item.Y, Skull: item.Skull.Clone()})
	destination.Version++
	if p.destination.Kind == constt.InventoryHand {
		destination.HandMouseOffsetX, destination.HandMouseOffsetY = constt.DefaultHandMouseOffset, constt.DefaultHandMouseOffset
	}
	root := &p.capture.rootTrees[p.rootIndex]
	if root != destination {
		root.Version++
	}
	p.built = &item
	snapshots := make([]systems.InventorySnapshot, 0, len(p.capture.rootTrees))
	for index := range p.capture.rootTrees {
		data := &p.capture.rootTrees[index]
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, systems.InventorySnapshot{CharacterID: int64(p.recipientID), Kind: int16(data.Kind), InventoryKey: int16(data.Key), Data: encoded, Version: data.Version})
	}
	return snapshots, nil
}

// Apply runs only after the transaction is confirmed, under the shard lock.
// KO/death may occur during the write; the accepted grant belongs to the same
// character identity and therefore completes into that character's corpse.
func (p *PreparedSkullGrant) Apply(w *ecs.World, item components.InvItem) (*GiveItemResult, error) {
	if p == nil || p.world != w || p.built == nil || p.applied || item.ItemID != p.built.ItemID || !w.Alive(p.recipient) ||
		w.GetHandleByEntityID(p.recipientID) != p.recipient || !ecs.InventoryOwnerReserved(w, p.recipientID) {
		return nil, ErrInvalidSkullGrant
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	destination, ok := ecs.GetComponent[components.InventoryContainer](w, p.destination.Handle)
	indexed, found := refs.Lookup(p.destination.Kind, p.destination.OwnerID, p.destination.Key)
	if !ok || !found || indexed != p.destination.Handle || !w.Alive(indexed) || destination.Version != p.version ||
		destination.OwnerID != p.destination.OwnerID || destination.Kind != p.destination.Kind || destination.Key != p.destination.Key {
		return nil, ErrInvalidSkullGrant
	}
	rootRef := p.capture.roots[p.rootIndex]
	root, ok := ecs.GetComponent[components.InventoryContainer](w, rootRef.Handle)
	indexedRoot, found := refs.Lookup(rootRef.Kind, rootRef.OwnerID, rootRef.Key)
	if !ok || !found || indexedRoot != rootRef.Handle || !w.Alive(indexedRoot) || root.OwnerID != p.recipientID ||
		root.Kind != rootRef.Kind || root.Key != rootRef.Key || root.Version+1 != uint64(p.capture.rootTrees[p.rootIndex].Version) {
		return nil, ErrInvalidSkullGrant
	}
	granted := *p.built
	granted.Skull = granted.Skull.Clone()
	ecs.MutateComponent[components.InventoryContainer](w, p.destination.Handle, func(c *components.InventoryContainer) bool {
		c.Items = append(c.Items, granted)
		c.Version++
		if c.Kind == constt.InventoryHand {
			c.HandMouseOffsetX, c.HandMouseOffsetY = constt.DefaultHandMouseOffset, constt.DefaultHandMouseOffset
		}
		return true
	})
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, p.recipient)
	if !hasOwner {
		owner.Inventories = []components.InventoryLink{{Kind: rootRef.Kind, OwnerID: rootRef.OwnerID, Key: rootRef.Key, Handle: rootRef.Handle}}
	}
	updated, _ := ecs.GetComponent[components.InventoryContainer](w, p.destination.Handle)
	containers := []*ContainerInfo{{Handle: p.destination.Handle, Container: &updated, Owner: &owner}}
	containers = p.executor.applyNestedCascade(w, p.recipientID, containers)
	if rootRef.Handle != p.destination.Handle {
		// A changed bag visual already advances its parent revision through the
		// shared cascade. Otherwise advance the embedding root exactly once so
		// its live version matches the committed JSON snapshot.
		ecs.MutateComponent[components.InventoryContainer](w, rootRef.Handle, func(c *components.InventoryContainer) bool {
			if c.Version != root.Version {
				return false
			}
			c.Version++
			return true
		})
		updatedRoot, _ := ecs.GetComponent[components.InventoryContainer](w, rootRef.Handle)
		containers = mergeUpdatedContainerInfos(containers, []*ContainerInfo{{Handle: rootRef.Handle, Container: &updatedRoot, Owner: &owner}})
	}
	p.applied = true
	ecs.MarkObjectBehaviorDirty(w, p.recipient)
	lp := p.executor.service.recordDiscoveryOnGive(w, p.recipient, p.definition)
	return &GiveItemResult{Success: true, GrantedCount: 1, PlacedInHand: p.destination.Kind == constt.InventoryHand,
		Message: "granted 1/1 items", UpdatedContainers: containers, DiscoveryLPGained: lp}, nil
}
