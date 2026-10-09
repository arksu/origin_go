package inventory

import (
	"errors"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"
)

var ErrInvalidObjectLootCapture = errors.New("invalid object loot capture")

// ObjectLootItem owns the item state needed to create a ground drop. Quantity is
// retained here; splitting stacks and reserving new IDs happen outside ECS reads.
type ObjectLootItem struct {
	ItemID          types.EntityID
	TypeID          uint32
	Resource        string
	Quality         uint32
	Quantity        uint32
	W, H            uint8
	NestedInventory *InventoryDataV1
}

type objectLootFrame struct {
	ref         ecs.InventoryRefEntry
	data        *InventoryDataV1
	initialized bool
	version     uint64
	length      int
	position    int
}

type objectLootContainerStamp struct {
	version uint64
	length  int
	width   uint8
	height  uint8
}

// ObjectLootCapture incrementally reads a quarantined object's inventory tree.
// Destruction requires zero HP; lifecycle transformation retains valid HP. The
// caller holds the owning shard's read lock for each CaptureBatch and prevents
// inventory mutations until completion. No mutable ECS data is retained.
type ObjectLootCapture struct {
	targetID       types.EntityID
	target         types.Handle
	world          *ecs.World
	registry       *itemdefs.Registry
	roots          []ecs.InventoryRefEntry
	nextRoot       int
	frames         []objectLootFrame
	items          []ObjectLootItem
	maxItemID      types.EntityID
	refs           []ecs.InventoryRefEntry
	stamps         []objectLootContainerStamp
	seenItems      map[types.EntityID]struct{}
	seenRefs       map[types.Handle]struct{}
	validateNext   int
	err            error
	done           bool
	transformation bool
}

// NewObjectLootCapture takes its own copy of the owner's ordered root refs.
// Ordinary destruction still requires the target's HP to be zero.
func NewObjectLootCapture(targetID types.EntityID, roots []ecs.InventoryRefEntry) *ObjectLootCapture {
	return newObjectLootCapture(targetID, roots, false)
}

// NewObjectLootCaptureForTransformation captures a quarantined lifecycle
// transition without reducing HP. Health must remain finite and nonnegative.
func NewObjectLootCaptureForTransformation(targetID types.EntityID, roots []ecs.InventoryRefEntry) *ObjectLootCapture {
	return newObjectLootCapture(targetID, roots, true)
}

func newObjectLootCapture(targetID types.EntityID, roots []ecs.InventoryRefEntry, transformation bool) *ObjectLootCapture {
	return &ObjectLootCapture{
		targetID:       targetID,
		transformation: transformation,
		roots:          append([]ecs.InventoryRefEntry(nil), roots...),
		seenItems:      make(map[types.EntityID]struct{}),
		seenRefs:       make(map[types.Handle]struct{}),
	}
}

// CaptureBatch copies at most maxRecords item records. Container visits and final
// identity checks share the same budget, bounding empty/deep trees as well.
// An error exposes no partial result and leaves ECS unchanged.
func (c *ObjectLootCapture) CaptureBatch(w *ecs.World, registry *itemdefs.Registry, maxRecords int) (bool, error) {
	if c == nil {
		return false, ErrInvalidObjectLootCapture
	}
	if c.err != nil {
		return false, c.err
	}
	if maxRecords <= 0 || !c.validTarget(w, registry) {
		return c.fail()
	}
	if c.done {
		return true, nil
	}
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	for work := 0; work < maxRecords; work++ {
		if len(c.frames) == 0 {
			if c.nextRoot < len(c.roots) {
				ref := c.roots[c.nextRoot]
				if ref.OwnerID != c.targetID {
					return c.fail()
				}
				c.nextRoot++
				c.frames = append(c.frames, objectLootFrame{ref: ref})
			} else {
				if c.validateNext < len(c.refs) {
					ref, stamp := c.refs[c.validateNext], c.stamps[c.validateNext]
					container, ok := c.readContainer(w, refs, ref)
					if !ok || container.Version != stamp.version || len(container.Items) != stamp.length ||
						container.Width != stamp.width || container.Height != stamp.height {
						return c.fail()
					}
					c.validateNext++
					continue
				}
				c.done = true
				return true, nil
			}
		}
		frame := &c.frames[len(c.frames)-1]
		container, ok := c.readContainer(w, refs, frame.ref)
		if !ok {
			return c.fail()
		}
		if !frame.initialized {
			if _, duplicate := c.seenRefs[frame.ref.Handle]; duplicate || container.Version > uint64(math.MaxInt) {
				return c.fail()
			}
			c.seenRefs[frame.ref.Handle] = struct{}{}
			c.refs = append(c.refs, frame.ref)
			c.stamps = append(c.stamps, objectLootContainerStamp{
				version: container.Version, length: len(container.Items), width: container.Width, height: container.Height,
			})
			frame.initialized, frame.version, frame.length = true, container.Version, len(container.Items)
			if frame.data != nil {
				*frame.data = InventoryDataV1{
					Kind: uint8(container.Kind), Key: container.Key, Width: container.Width, Height: container.Height,
					Version: int(container.Version),
				}
			}
			continue
		}
		if container.Version != frame.version || len(container.Items) != frame.length {
			return c.fail()
		}
		if frame.position == frame.length {
			c.frames = c.frames[:len(c.frames)-1]
			continue
		}
		item := container.Items[frame.position]
		definition, ok := registry.GetByID(int(item.TypeID))
		if !ok || !validObjectLootItem(item, definition) || item.ItemID == c.targetID ||
			container.Kind == constt.InventoryGrid && (int(item.X)+int(item.W) > int(container.Width) || int(item.Y)+int(item.H) > int(container.Height)) {
			return c.fail()
		}
		if _, duplicate := c.seenItems[item.ItemID]; duplicate {
			return c.fail()
		}
		c.seenItems[item.ItemID] = struct{}{}
		if item.ItemID > c.maxItemID {
			c.maxItemID = item.ItemID
		}
		var nested *InventoryDataV1
		var childRef ecs.InventoryRefEntry
		hasContents := false
		childCount := refs.OwnerEntryCount(item.ItemID)
		if definition.Container != nil {
			handle, found := refs.Lookup(constt.InventoryGrid, item.ItemID, 0)
			if !found || childCount != 1 || item.Quantity != 1 {
				return c.fail()
			}
			childRef = ecs.InventoryRefEntry{
				InventoryRefKey: ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: item.ItemID, Key: 0}, Handle: handle,
			}
			child, valid := c.readContainer(w, refs, childRef)
			if !valid || int(child.Width) != definition.Container.Size.W || int(child.Height) != definition.Container.Size.H {
				return c.fail()
			}
			hasContents = len(child.Items) != 0
			nested = &InventoryDataV1{}
		} else if childCount != 0 {
			return c.fail()
		}
		if frame.data == nil {
			c.items = append(c.items, ObjectLootItem{
				ItemID: item.ItemID, TypeID: item.TypeID, Resource: definition.ResolveResource(hasContents),
				Quality: item.Quality, Quantity: item.Quantity, W: item.W, H: item.H, NestedInventory: nested,
			})
		} else {
			frame.data.Items = append(frame.data.Items, InventoryItemV1{
				ItemID: uint64(item.ItemID), TypeID: item.TypeID, Quality: item.Quality, Quantity: item.Quantity,
				X: item.X, Y: item.Y, EquipSlot: EquipSlotToString(item.EquipSlot), NestedInventory: nested,
			})
		}
		frame.position++
		if nested != nil {
			c.frames = append(c.frames, objectLootFrame{ref: childRef, data: nested})
		}
	}
	return false, nil
}

func (c *ObjectLootCapture) validTarget(w *ecs.World, registry *itemdefs.Registry) bool {
	if w == nil || registry == nil || c.targetID == 0 || c.targetID > math.MaxInt64 || c.world != nil && (c.world != w || c.registry != registry) {
		return false
	}
	handle := w.GetHandleByEntityID(c.targetID)
	if handle == types.InvalidHandle || !w.Alive(handle) || c.world != nil && c.target != handle {
		return false
	}
	id, ok := ecs.GetComponent[ecs.ExternalID](w, handle)
	if !ok || id.ID != c.targetID {
		return false
	}
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !ok || !state.HasHP {
		return false
	}
	if c.transformation {
		if !ecs.ObjectDestructionPending(w, handle) || state.HP < 0 || math.IsNaN(state.HP) || math.IsInf(state.HP, 0) {
			return false
		}
	} else if state.HP != 0 {
		return false
	}
	if ecs.GetResource[ecs.InventoryRefIndex](w).OwnerEntryCount(c.targetID) != len(c.roots) {
		return false
	}
	c.world, c.target, c.registry = w, handle, registry
	return true
}

func (c *ObjectLootCapture) readContainer(w *ecs.World, refs *ecs.InventoryRefIndex, ref ecs.InventoryRefEntry) (components.InventoryContainer, bool) {
	indexed, found := refs.Lookup(ref.Kind, ref.OwnerID, ref.Key)
	if !found || indexed != ref.Handle || !w.Alive(ref.Handle) {
		return components.InventoryContainer{}, false
	}
	container, ok := ecs.GetComponent[components.InventoryContainer](w, ref.Handle)
	if !ok || container.OwnerID != ref.OwnerID || container.Kind != ref.Kind || container.Key != ref.Key {
		return components.InventoryContainer{}, false
	}
	if container.Kind > constt.InventoryBuild ||
		container.Kind == constt.InventoryGrid && (container.Width == 0 || container.Height == 0 || len(container.Items) > int(container.Width)*int(container.Height)) ||
		(container.Kind == constt.InventoryHand || container.Kind == constt.InventoryDroppedItem) && len(container.Items) > 1 {
		return components.InventoryContainer{}, false
	}
	return container, true
}

func validObjectLootItem(item components.InvItem, definition *itemdefs.ItemDef) bool {
	if item.ItemID == 0 || item.ItemID > math.MaxInt64 || item.TypeID == 0 || item.Quality == 0 || item.Quantity == 0 || item.W == 0 || item.H == 0 ||
		int(item.W) != definition.Size.W || int(item.H) != definition.Size.H {
		return false
	}
	if definition.Container != nil {
		return item.Quantity == 1
	}
	if definition.Stack == nil || definition.Stack.Mode == itemdefs.StackModeNone {
		return item.Quantity == 1
	}
	return definition.Stack.Mode == itemdefs.StackModeStack && definition.Stack.Max >= 2 && uint64(item.Quantity) <= uint64(definition.Stack.Max)
}

func (c *ObjectLootCapture) fail() (bool, error) {
	c.err = ErrInvalidObjectLootCapture
	return false, c.err
}

// Items returns the owned root items only after complete successful capture.
func (c *ObjectLootCapture) Items() []ObjectLootItem {
	if c == nil || !c.done || c.err != nil {
		return nil
	}
	return c.items
}

// ContainerRefs returns every exact root/descendant ref for post-commit cleanup.
func (c *ObjectLootCapture) ContainerRefs() []ecs.InventoryRefEntry {
	if c == nil || !c.done || c.err != nil {
		return nil
	}
	return c.refs
}

// MaxItemID includes descendants retained inside bags, for durable ID reservation.
func (c *ObjectLootCapture) MaxItemID() types.EntityID {
	if c == nil || !c.done || c.err != nil {
		return 0
	}
	return c.maxItemID
}
