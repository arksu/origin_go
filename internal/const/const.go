package _const

import "time"

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
	LAST_USED_ID                 = "last_used_id"
	SERVER_TICK_TOTAL            = "server_tick_total"
	SERVER_RUNTIME_SECONDS_TOTAL = "server_runtime_seconds_total"
)

const (
	PlayerVisionRadius     = 600
	PlayerVisionPower      = 600
	VisionUpdateInterval   = 3 * time.Second
	VisionUpdateJitter     = 50 * time.Millisecond
	VisionPosEpsilon       = 0.01
	DefaultMaxHearDistance = 200.0
)

const StopDistance float64 = 0.2

// MoveMode represents different movement states
type MoveMode uint8

const (
	Crawl MoveMode = iota
	Walk
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
	InventoryBuild       InventoryKind = 4
)

// DefaultHandMouseOffset — дефолтный оффсет курсора при взятии предмета в руку,
// если клиент не прислал hand_pos.
const DefaultHandMouseOffset int16 = 15

// Dropped item constants
const (
	DroppedItemTypeID     = 1000
	BuildObjectTypeID     = 1001
	DroppedDespawnSeconds = 10
	DroppedPickupRadius   = 5.0
	DroppedPickupRadiusSq = DroppedPickupRadius * DroppedPickupRadius
)

// Entity stats (stamina/energy) constants.
const (
	DefaultEnergy                    = 1000.0
	EnergyMax                        = 1100.0
	StaminaScalePerCon               = 1000.0
	RegenEnergySpendPerTick          = 1.0
	StaminaPerEnergyUnit             = 5.0
	DefaultStaminaRegenIntervalTicks = 50

	MovementStaminaCostStayPerTick    = 0.0
	MovementStaminaCostCrawlPerTick   = 0.0
	MovementStaminaCostWalkPerTick    = 0.002
	MovementStaminaCostRunPerTick     = 0.02
	MovementStaminaCostFastRunPerTick = 0.5

	StaminaNoMoveThresholdPercent    = 0.05
	StaminaCrawlOnlyThresholdPercent = 0.10
	StaminaNoRunThresholdPercent     = 0.25
	StaminaNoFastRunThresholdPercent = 0.50
	LongActionStaminaFloorPercent    = 0.10
)
