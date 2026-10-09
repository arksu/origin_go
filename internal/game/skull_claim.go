package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sqlc-dev/pqtype"
)

type skullClaimPersister interface {
	LoadDeadCharacter(context.Context, types.EntityID) (inventory.DeadCharacterInfo, error)
	PersistSkullClaim(context.Context, inventory.SkullClaimPersistenceRecord) error
}

// One slot in the existing bounded destruction queue owns the reservation and
// immutable transaction payload through retries, including uncertain commits.
type skullClaimOperation struct {
	recipientID types.EntityID
	recipient   types.Handle
	grant       *inventory.PreparedSkullGrant
	source      *repository.Object
	item        components.InvItem
	record      *inventory.SkullClaimPersistenceRecord
	applied     bool
}

func (s *Shard) takeSkull(w *ecs.World, playerID types.EntityID, player types.Handle, targetID types.EntityID, target types.Handle) contracts.BehaviorResult {
	failure := func(reason string) contracts.BehaviorResult {
		return contracts.BehaviorResult{UserVisible: true, ReasonCode: reason, Severity: contracts.BehaviorAlertSeverityWarning}
	}
	if s == nil || w != s.world || s.objectDestruction == nil || s.inventoryExecutor == nil ||
		!w.Alive(player) || !w.Alive(target) || w.GetHandleByEntityID(playerID) != player || w.GetHandleByEntityID(targetID) != target ||
		playerstate.ItemsLocked(w, player) || ecs.ObjectDestructionPending(w, target) {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	links, hasLinks := ecs.TryGetResource[ecs.LinkState](w)
	if !hasLinks {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	link, linked := links.GetLink(playerID)
	if !linked || link.PlayerHandle != player || link.TargetID != targetID || link.TargetHandle != target {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, target)
	if !ok {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	definition, exists := objectdefs.Global().GetByID(int(info.TypeID))
	if !exists || definition.Key != "player_skeleton" {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	grant, err := s.inventoryExecutor.PrepareSkullGrant(w, playerID, player)
	if errors.Is(err, inventory.ErrSkullGrantNoSpace) {
		return failure("TAKE_SKULL_NO_SPACE")
	}
	if err != nil {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	destination, exists := objectdefs.Global().GetByKey("player_skeleton_without_skull")
	if !exists || s.objectDestruction.takeSkull(target, playerID, player, grant, destination) != nil {
		return failure("TAKE_SKULL_UNAVAILABLE")
	}
	return contracts.BehaviorResult{OK: true}
}

func (s *ObjectDestructionService) takeSkull(target types.Handle, recipientID types.EntityID, recipient types.Handle, grant *inventory.PreparedSkullGrant, destination *objectdefs.ObjectDef) error {
	if grant == nil || destination == nil || destination.Key != "player_skeleton_without_skull" || s.deps.TransformCommitted == nil {
		return ErrInvalidObjectDestructionService
	}
	if _, ok := s.deps.Persister.(skullClaimPersister); !ok {
		return ErrInvalidObjectDestructionService
	}
	if _, ok := s.deps.Chunks.(objectLootTransformationChunks); !ok {
		return ErrInvalidObjectDestructionService
	}
	w := s.world
	info, ok := ecs.GetComponent[components.EntityInfo](w, target)
	if !ok || !w.Alive(target) || !w.Alive(recipient) || w.GetHandleByEntityID(recipientID) != recipient || playerstate.ItemsLocked(w, recipient) {
		return ErrObjectDestructionCapture
	}
	sourceDef, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || sourceDef.Key != "player_skeleton" || ecs.ObjectDestructionPending(w, target) {
		return ErrObjectDestructionPending
	}
	itemDef, ok := s.deps.Items.GetByKey("skull")
	if !ok || itemDef.DefID != 3014 {
		return ErrInvalidObjectDestructionService
	}
	source, err := gameworld.NewObjectFactory(nil).Serialize(w, target)
	if err != nil {
		return err
	}
	if source == nil {
		return ErrObjectDestructionCapture
	}
	if err := s.PrepareTarget(target); err != nil {
		return err
	}
	reservation, err := s.reserve(target)
	if err != nil {
		return err
	}
	if !ecs.ReserveInventoryOwner(w, recipientID, recipient) {
		s.cancelReservation(reservation)
		return ErrObjectDestructionPending
	}
	op := s.reservationOperation(reservation, destructionReserved)
	replacement := *source
	replacement.TypeID = destination.DefID
	replacement.Hp = sql.NullFloat64{Float64: float64(destination.HP), Valid: true}
	op.replacement, op.replacementDef = &replacement, destination
	op.skullClaim = &skullClaimOperation{
		recipientID: recipientID, recipient: recipient, grant: grant, source: source,
		item: components.InvItem{TypeID: uint32(itemDef.DefID), Resource: itemDef.ResolveResource(false), Quality: info.Quality, Quantity: 1, W: 1, H: 1},
	}
	s.commitReservation(reservation)
	s.finalizeReservation(reservation)
	return nil
}

func (op *objectDestructionOperation) runSkullClaim() error {
	if op.committed {
		return nil
	}
	s, claim := op.service, op.skullClaim
	persister := s.deps.Persister.(skullClaimPersister)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if claim.record == nil {
		if claim.item.Skull == nil {
			dead, err := persister.LoadDeadCharacter(ctx, op.id)
			if err != nil {
				return err
			}
			metadata := &components.SkullMetadata{CharacterID: op.id, Nickname: dead.Nickname, DeathDate: dead.DeathDate}
			if metadata.HintExt() == "" {
				return inventory.ErrSkullClaimCharacterMissing
			}
			claim.item.Skull = metadata
		}
		if claim.item.ItemID == 0 {
			first, last, err := s.deps.IDs.ReserveIDs(1)
			if err != nil {
				return err
			}
			if first == 0 || first != last || first > math.MaxInt64 {
				return ErrObjectDestructionOverflow
			}
			claim.item.ItemID = first
		}
		for {
			var done bool
			var err error
			s.deps.WithWorldRead(func(w *ecs.World) {
				done, err = claim.grant.CaptureBatch(w, ObjectDestructionCaptureBudget)
			})
			if err != nil {
				return err
			}
			if done {
				break
			}
		}
		snapshots, err := claim.grant.Build(claim.item)
		if err != nil {
			return err
		}
		receipt := inventory.SkullClaimReceipt{SkullItemID: claim.item.ItemID, RecipientID: claim.recipientID}
		var envelope components.ObjectStateEnvelope
		if claim.source.Data.Valid && len(claim.source.Data.RawMessage) > 0 {
			if err := json.Unmarshal(claim.source.Data.RawMessage, &envelope); err != nil {
				return inventory.ErrInvalidSkullGrant
			}
		}
		envelope.Version = 1
		if envelope.Behaviors == nil {
			envelope.Behaviors = make(map[string]json.RawMessage, 1)
		}
		encodedReceipt, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		envelope.Behaviors["player_skeleton"] = encodedReceipt
		data, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		op.replacement.Data = pqtype.NullRawMessage{RawMessage: data, Valid: true}
		claim.record = &inventory.SkullClaimPersistenceRecord{Source: claim.source, Replacement: op.replacement, RecipientID: claim.recipientID, SkullItemID: claim.item.ItemID, PlayerInventories: snapshots}
	}
	if err := s.deps.Chunks.WithPersistence(op.pinned, func() error { return persister.PersistSkullClaim(ctx, *claim.record) }); err != nil {
		return err
	}
	op.committed = true
	return nil
}

func definiteSkullClaimFailure(err error) bool {
	// Data and integrity errors have an explicit PostgreSQL rejection. The
	// immutable request cannot succeed on retry. Transport/commit uncertainty
	// and retryable transaction failures retain their reservation and payload.
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && len(databaseError.Code) >= 2 && (databaseError.Code[:2] == "22" || databaseError.Code[:2] == "23") {
		return true
	}
	return errors.Is(err, inventory.ErrSkullClaimCharacterMissing) || errors.Is(err, inventory.ErrSkullClaimSourceConflict) ||
		errors.Is(err, inventory.ErrSkullClaimInventoryConflict) || errors.Is(err, inventory.ErrInvalidSkullGrant) ||
		errors.Is(err, inventory.ErrInvalidObjectLootCapture) || errors.Is(err, inventory.ErrSkullGrantNoSpace) || errors.Is(err, ErrObjectDestructionOverflow)
}

func (s *ObjectDestructionService) rejectSkullClaim(op *objectDestructionOperation) bool {
	state := ecs.GetResource[ecs.ObjectDestructionState](s.world)
	state.Pending[op.target] = false
	// The reserved skeleton was never hidden or changed before commit.
	// Rejection only releases its interaction lock; no visual refresh is needed.
	if s.deps.SkullRejected != nil {
		s.deps.SkullRejected(op.skullClaim.recipientID)
	}
	return true
}

func (s *ObjectDestructionService) finalizeSkullClaim(op *objectDestructionOperation, budget *int) bool {
	if *budget == 0 {
		return false
	}
	*budget--
	claim := op.skullClaim
	if err := s.deps.Chunks.(objectLootTransformationChunks).ReplaceCommittedSource(op.replacement); err != nil {
		return false
	}
	if !claim.applied {
		result, err := claim.grant.Apply(s.world, claim.item)
		if err != nil {
			return false
		}
		claim.applied = true
		if s.deps.SkullGranted != nil {
			s.deps.SkullGranted(claim.recipientID, claim.recipient, result)
		}
	}
	ecs.WithComponent(s.world, op.target, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "player_skeleton", &inventory.SkullClaimReceipt{SkullItemID: claim.item.ItemID, RecipientID: claim.recipientID})
	})
	state := ecs.GetResource[ecs.ObjectDestructionState](s.world)
	state.Pending[op.target] = false
	if !s.deps.TransformCommitted(op.target, op.replacementDef) {
		state.Pending[op.target] = true
		return false
	}
	return true
}

func (s *Shard) skullClaimRejected(recipient types.EntityID) {
	s.SendMiniAlert(recipient, &proto.S2C_MiniAlert{ReasonCode: "TAKE_SKULL_FAILED", Severity: proto.AlertSeverity_ALERT_SEVERITY_WARNING})
}
