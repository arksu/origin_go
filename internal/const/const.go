package _const

const (
	ChunkSize      = 128
	CoordPerTile   = 12
	ChunkWorldSize = ChunkSize * CoordPerTile
)

const (
	PlayerSpeed        = 32.0
	PlayerColliderSize = 10
	PlayerLayer        = uint64(1)
	PlayerMask         = uint64(1)
)

// keys for global_var
const (
	LAST_USED_ID = "last_used_id"
)

const StopDistance float64 = 0.2

// MoveMode represents different movement states
type MoveMode uint8

const (
	Walk MoveMode = iota
	Run
	FastRun
	Swim
)

// MoveState представляет текущее состояние движения
type MoveState uint8

const (
	StateIdle MoveState = iota
	StateMoving
	StateInteracting
	StateStunned // не может двигаться
)

type TargetType uint8

const (
	TargetNone TargetType = iota
	TargetPoint
	TargetEntity
)

type InventoryType uint8

type InventoryKind uint8

const (
	// Basic container like box, chest. with size
	InventoryGrid InventoryKind = 0
	// InventoryHand represents the inventory type associated with items held in the hand. can receive only ONE item any size
	InventoryHand        InventoryKind = 1
	InventoryEquipment   InventoryKind = 2
	InventoryDroppedItem InventoryKind = 3
)

// DefaultHandMouseOffset — дефолтный оффсет курсора при взятии предмета в руку,
// если клиент не прислал hand_pos.
const DefaultHandMouseOffset int16 = 15
