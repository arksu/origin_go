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

const (
	InventoryCharacter InventoryType = iota
	InventoryHand      InventoryType = iota
	InventoryContainer
)
