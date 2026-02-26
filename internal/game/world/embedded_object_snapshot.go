package world

import "encoding/json"

type EmbeddedInventorySnapshotV1 struct {
	Kind         int16           `json:"kind"`
	InventoryKey int16           `json:"inventory_key"`
	Version      int             `json:"version"`
	Data         json.RawMessage `json:"data"`
}

// EmbeddedObjectSnapshotV1 is a portable object+inventories snapshot used for transfer
// and future object-in-object storage (e.g. boat cargo).
type EmbeddedObjectSnapshotV1 struct {
	Version int `json:"version"`

	EntityID uint64 `json:"entity_id"`
	TypeID   int    `json:"type_id"`
	Region   int    `json:"region"`
	Layer    int    `json:"layer"`
	Quality  int16  `json:"quality"`

	Heading *int16          `json:"heading,omitempty"`
	ObjectData json.RawMessage `json:"object_data,omitempty"`

	RootInventories []EmbeddedInventorySnapshotV1 `json:"root_inventories,omitempty"`
}

