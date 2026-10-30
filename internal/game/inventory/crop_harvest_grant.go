package inventory

import (
	"encoding/json"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

// PreparedCropGrant reuses the skull claim's bounded, reserved-owner capture.
// Placement is computed over owned scratch inventories on the I/O worker;
// the owning shard installs only the confirmed post-transaction containers.
type PreparedCropGrant struct {
	executor       *InventoryExecutor
	world          *ecs.World
	recipientID    types.EntityID
	recipient      types.Handle
	links          []components.InventoryLink
	capture        *ObjectLootCapture
	nodes          []*cropGrantNode
	changed        []*cropGrantNode
	discoveries    []*itemdefs.ItemDef
	granted        uint32
	hand           bool
	built, applied bool
}

type cropGrantNode struct {
	ref        ecs.InventoryRefEntry
	data       *InventoryDataV1
	parent     *cropGrantNode
	parentType *itemdefs.ItemDef
	container  components.InventoryContainer
	before     uint64
	changed    bool
}

func (e *InventoryExecutor) PrepareCropGrant(w *ecs.World, id types.EntityID, player types.Handle) (*PreparedCropGrant, error) {
	if e == nil || e.service == nil || w == nil || id == 0 || !w.Alive(player) || w.GetHandleByEntityID(id) != player ||
		playerstate.ItemsLocked(w, player) || ecs.InventoryOwnerReserved(w, id) {
		return nil, ErrInvalidCropGrant
	}
	owner, exists := ecs.GetComponent[components.InventoryOwner](w, player)
	// Inventory links are normally a handful of roots/bags. Reject malformed
	// owners before copying or traversing an unbounded admission payload.
	if !exists || len(owner.Inventories) > 64 {
		return nil, ErrInvalidCropGrant
	}
	links := orderedGridLinks(owner.Inventories, id, defaultGivePlacementPolicy)
	if hand, found := playerHandLink(owner, id); found {
		links = append(links, hand)
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	if refs.OwnerEntryCount(id) > 64 {
		return nil, ErrInvalidCropGrant
	}
	roots := refs.EntriesByOwnerInto(id, nil)
	return &PreparedCropGrant{executor: e, world: w, recipientID: id, recipient: player, links: links,
		capture: newReservedInventoryCapture(id, player, roots)}, nil
}

func (p *PreparedCropGrant) CaptureBatch(w *ecs.World, budget int) (bool, error) {
	if p == nil || p.world != w || p.built {
		return false, ErrInvalidCropGrant
	}
	return p.capture.CaptureBatch(w, itemdefs.Global(), budget)
}

// Build never reads live ECS. Inputs are individual, already allocated items;
// their quality and identity are retained unchanged in inventory or overflow.
func (p *PreparedCropGrant) Build(items []components.InvItem) ([]systems.InventorySnapshot, []components.InvItem, error) {
	if p == nil || p.built || !p.capture.done || p.capture.err != nil || len(items) == 0 || len(items) > MaxCropHarvestUnits {
		return nil, nil, ErrInvalidCropGrant
	}
	registry := itemdefs.Global()
	refs := make(map[ecs.InventoryRefKey]ecs.InventoryRefEntry, len(p.capture.refs))
	for _, ref := range p.capture.refs {
		refs[ref.InventoryRefKey] = ref
	}
	byRef := make(map[ecs.InventoryRefKey]*cropGrantNode, len(refs))
	type visit struct {
		data       *InventoryDataV1
		owner      types.EntityID
		parent     *cropGrantNode
		parentType *itemdefs.ItemDef
	}
	stack := make([]visit, 0, len(refs))
	for index := range p.capture.rootTrees {
		stack = append(stack, visit{data: &p.capture.rootTrees[index], owner: p.recipientID})
	}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		key := ecs.InventoryRefKey{OwnerID: current.owner, Kind: constt.InventoryKind(current.data.Kind), Key: current.data.Key}
		ref, found := refs[key]
		if !found || byRef[key] != nil || current.data.Version < 0 || current.data.Version >= math.MaxInt {
			return nil, nil, ErrInvalidCropGrant
		}
		n := &cropGrantNode{ref: ref, data: current.data, parent: current.parent, parentType: current.parentType, before: uint64(current.data.Version)}
		n.container = components.InventoryContainer{OwnerID: current.owner, Kind: key.Kind, Key: key.Key, Width: current.data.Width, Height: current.data.Height,
			Version: n.before, HandMouseOffsetX: current.data.HandMouseOffsetX, HandMouseOffsetY: current.data.HandMouseOffsetY}
		for index := range current.data.Items {
			item := &current.data.Items[index]
			definition, ok := registry.GetByID(int(item.TypeID))
			if !ok {
				return nil, nil, ErrInvalidCropGrant
			}
			n.container.Items = append(n.container.Items, components.InvItem{ItemID: types.EntityID(item.ItemID), TypeID: item.TypeID, Quality: item.Quality, Quantity: item.Quantity,
				X: item.X, Y: item.Y, EquipSlot: StringToEquipSlot(item.EquipSlot), W: uint8(definition.Size.W), H: uint8(definition.Size.H),
				Resource: definition.ResolveResource(item.NestedInventory != nil && len(item.NestedInventory.Items) > 0), Skull: item.Skull.Clone()})
			if item.NestedInventory != nil {
				stack = append(stack, visit{item.NestedInventory, types.EntityID(item.ItemID), n, definition})
			}
		}
		byRef[key] = n
		p.nodes = append(p.nodes, n)
	}
	overflow := make([]components.InvItem, 0, len(items))
	seen := make(map[types.EntityID]bool, len(items))
	for _, item := range items {
		definition, found := registry.GetByID(int(item.TypeID))
		_, duplicate := p.capture.seenItems[item.ItemID]
		if !found || definition.Container != nil || item.ItemID == 0 || item.ItemID > math.MaxInt64 || duplicate || seen[item.ItemID] || item.Quantity != 1 ||
			int(item.W) != definition.Size.W || int(item.H) != definition.Size.H {
			return nil, nil, ErrInvalidCropGrant
		}
		seen[item.ItemID] = true
		placed := false
		for _, link := range p.links {
			n := byRef[ecs.InventoryRefKey{OwnerID: link.OwnerID, Kind: link.Kind, Key: link.Key}]
			if n == nil || n.ref.Handle != link.Handle {
				continue
			}
			// Match GiveItem's supported personal nesting: a direct child of
			// one of this character's roots, with the parent's content rules.
			if n.parent != nil && (n.parent.parent != nil || n.parent.ref.OwnerID != p.recipientID) {
				continue
			}
			if n.container.Kind == constt.InventoryGrid {
				if definition.Allowed.Grid != nil && !*definition.Allowed.Grid {
					continue
				}
				if n.parent != nil && (n.parentType == nil || n.parentType.Container == nil || p.executor.service.validator.checkContentRules(&n.parentType.Container.Rules, definition) != nil) {
					continue
				}
				fit, x, y := p.executor.service.placementService.FindFreeSpace(&n.container, item.W, item.H)
				if !fit {
					continue
				}
				item.X, item.Y = x, y
			} else if n.container.Kind != constt.InventoryHand || len(n.container.Items) != 0 || definition.Allowed.Hand != nil && !*definition.Allowed.Hand {
				continue
			}
			item.Resource = definition.ResolveResource(false)
			n.container.Items = append(n.container.Items, item)
			n.data.Items = append(n.data.Items, InventoryItemV1{ItemID: uint64(item.ItemID), TypeID: item.TypeID, Quality: item.Quality, Quantity: 1, X: item.X, Y: item.Y})
			if n.container.Kind == constt.InventoryHand {
				n.container.HandMouseOffsetX, n.container.HandMouseOffsetY = constt.DefaultHandMouseOffset, constt.DefaultHandMouseOffset
				n.data.HandMouseOffsetX, n.data.HandMouseOffsetY = constt.DefaultHandMouseOffset, constt.DefaultHandMouseOffset
				p.hand = true
			}
			for parent := n; parent != nil; parent = parent.parent {
				parent.changed = true
			}
			p.granted++
			p.discoveries = append(p.discoveries, definition)
			placed = true
			break
		}
		if !placed {
			overflow = append(overflow, item)
		}
	}
	for _, n := range p.nodes {
		if !n.changed {
			continue
		}
		n.container.Version++
		n.data.Version++
		// A newly filled bag changes its parent's runtime visual too.
		for index := range n.container.Items {
			item := &n.container.Items[index]
			child := byRef[ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: item.ItemID, Key: 0}]
			if child != nil {
				definition, _ := registry.GetByID(int(item.TypeID))
				item.Resource = definition.ResolveResource(len(child.container.Items) > 0)
			}
		}
		p.changed = append(p.changed, n)
	}
	snapshots := make([]systems.InventorySnapshot, 0, len(p.capture.rootTrees))
	for index := range p.capture.rootTrees {
		data := &p.capture.rootTrees[index]
		root := byRef[ecs.InventoryRefKey{OwnerID: p.recipientID, Kind: constt.InventoryKind(data.Kind), Key: data.Key}]
		if !root.changed {
			continue
		}
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, nil, err
		}
		snapshots = append(snapshots, systems.InventorySnapshot{CharacterID: int64(p.recipientID), Kind: int16(data.Kind), InventoryKey: int16(data.Key), Version: data.Version, Data: encoded})
	}
	p.built = true
	return snapshots, overflow, nil
}

func (p *PreparedCropGrant) Apply(w *ecs.World) (*GiveItemResult, error) {
	if p == nil || p.world != w || !p.built || p.applied || !w.Alive(p.recipient) || w.GetHandleByEntityID(p.recipientID) != p.recipient || !ecs.InventoryOwnerReserved(w, p.recipientID) {
		return nil, ErrInvalidCropGrant
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, n := range p.changed {
		h, found := refs.Lookup(n.ref.Kind, n.ref.OwnerID, n.ref.Key)
		live, exists := ecs.GetComponent[components.InventoryContainer](w, n.ref.Handle)
		if !found || h != n.ref.Handle || !w.Alive(h) || !exists || live.OwnerID != n.ref.OwnerID || live.Kind != n.ref.Kind || live.Key != n.ref.Key || live.Version != n.before {
			return nil, ErrInvalidCropGrant
		}
	}
	owner, _ := ecs.GetComponent[components.InventoryOwner](w, p.recipient)
	result := &GiveItemResult{Success: true, GrantedCount: p.granted, PlacedInHand: p.hand}
	for _, n := range p.changed {
		ecs.MutateComponent[components.InventoryContainer](w, n.ref.Handle, func(c *components.InventoryContainer) bool { *c = n.container; return true })
		container := n.container
		result.UpdatedContainers = append(result.UpdatedContainers, &ContainerInfo{Handle: n.ref.Handle, Container: &container, Owner: &owner})
	}
	for _, definition := range p.discoveries {
		result.DiscoveryLPGained += p.executor.service.recordDiscoveryOnGive(w, p.recipient, definition)
	}
	p.applied = true
	ecs.MarkObjectBehaviorDirty(w, p.recipient)
	return result, nil
}
