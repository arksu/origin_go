package contracts

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// InventoryItemProgressContext is a read-only view under the owning shard lock.
// Providers must neither mutate state nor retain component-backed references.
type InventoryItemProgressContext struct {
	World      *ecs.World
	Handle     types.Handle
	EntityID   types.EntityID
	EntityType uint32
	Container  components.InventoryContainer
	Item       components.InvItem
}

// InventoryItemProgressProvider projects one process without creating or
// reconciling it. A non-nil result must be owned by the outgoing snapshot.
type InventoryItemProgressProvider interface {
	InventoryItemProgress(*InventoryItemProgressContext) *netproto.ItemProgress
}
