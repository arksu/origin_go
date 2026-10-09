package ecs

import "origin/internal/types"

// InventoryReservations protects each recipient's complete inventory tree while
// an asynchronous durable grant is unresolved. It belongs to the owning shard;
// retaining the exact body handle permits KO and same-handle corpse conversion.
type InventoryReservations struct {
	Owners            map[types.EntityID]types.Handle
	cleanupRegistered bool
}

func EnsureInventoryReservations(w *World) *InventoryReservations {
	if w == nil {
		return nil
	}
	reservations, found := TryGetResource[InventoryReservations](w)
	if !found {
		reservations = InitResource(w, InventoryReservations{})
	}
	if !reservations.cleanupRegistered {
		w.AddDespawnObserver(func(handle types.Handle) {
			if id, exists := w.GetExternalID(handle); exists {
				reservations.Release(id, handle)
			}
		})
		reservations.cleanupRegistered = true
	}
	return reservations
}

func (r *InventoryReservations) Reserve(w *World, owner types.EntityID, handle types.Handle) bool {
	if r == nil || w == nil || owner == 0 || handle == types.InvalidHandle || !w.Alive(handle) || w.GetHandleByEntityID(owner) != handle {
		return false
	}
	if id, found := w.GetExternalID(handle); !found || id != owner {
		return false
	}
	if current, found := r.Owners[owner]; found && w.Alive(current) {
		return false
	}
	if r.Owners == nil {
		r.Owners = make(map[types.EntityID]types.Handle)
	}
	r.Owners[owner] = handle
	return true
}

func (r *InventoryReservations) Release(owner types.EntityID, handle types.Handle) bool {
	if r == nil || handle == types.InvalidHandle || r.Owners[owner] != handle {
		return false
	}
	delete(r.Owners, owner)
	return true
}

func ReserveInventoryOwner(w *World, owner types.EntityID, handle types.Handle) bool {
	return EnsureInventoryReservations(w).Reserve(w, owner, handle)
}

func ReleaseInventoryOwner(w *World, owner types.EntityID, handle types.Handle) bool {
	if w == nil {
		return false
	}
	reservations, found := TryGetResource[InventoryReservations](w)
	return found && reservations.Release(owner, handle)
}

func InventoryOwnerReserved(w *World, owner types.EntityID) bool {
	if w == nil || owner == 0 {
		return false
	}
	reservations, found := TryGetResource[InventoryReservations](w)
	if !found {
		return false
	}
	handle, reserved := reservations.Owners[owner]
	if !reserved || !w.Alive(handle) || w.GetHandleByEntityID(owner) != handle {
		return false
	}
	id, found := w.GetExternalID(handle)
	return found && id == owner
}

func InventoryHandleReserved(w *World, handle types.Handle) bool {
	if w == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}
	owner, found := w.GetExternalID(handle)
	return found && w.GetHandleByEntityID(owner) == handle && InventoryOwnerReserved(w, owner)
}
