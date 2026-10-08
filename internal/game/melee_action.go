package game

import (
	"errors"
	"math"

	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const MeleeMaxHits = 512

var (
	ErrInvalidMeleeExecution = errors.New("melee: invalid execution dependencies or actor")
	ErrMeleeExecutionBusy    = errors.New("melee: execution is already prepared or hit budget exceeded")
)

// preparedActionCompletion keeps failure-prone planning ahead of the action's
// cost commit. Implementations must consume their preparation in the same
// shard-locked call; CommitPrepared cannot perform fallible admission. A failed
// PrepareCompletion must release only its own preparation before returning.
type preparedActionCompletion interface {
	PrepareCompletion(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) string
	CommitPrepared()
	AbortPrepared()
	AfterCompletion()
}

type attackResultSender interface {
	SendAttackResult(types.Handle, types.EntityID, uint64, []netproto.AttackHit)
}

type meleeDamageCommit struct {
	creature bool
	soft     creatureDamageCommit
	object   objectDamageCommit
}

// MeleeExecutionService owns fixed scratch buffers for one World. All methods
// run under its shard lock. No prepared plan survives the completion call.
type MeleeExecutionService struct {
	world        *ecs.World
	equipment    *CombatEquipmentResolver
	sectors      *SectorResolver
	creatures    *CreatureDamageService
	objects      *ObjectDamageService
	events       *AttackEventSequence
	sender       attackResultSender
	profiles     *ecs.ComponentStorage[components.CharacterProfile]
	stats        *ecs.ComponentStorage[components.EntityStats]
	identities   *ecs.ComponentStorage[ecs.ExternalID]
	health       *ecs.ComponentStorage[components.EntityHealth]
	objectHealth *ecs.ComponentStorage[components.ObjectInternalState]
	dropped      *ecs.ComponentStorage[components.DroppedItem]
	contacts     []SectorHit
	commits      [MeleeMaxHits]meleeDamageCommit
	hits         [MeleeMaxHits]netproto.AttackHit
	count        int
	active       bool
	actor        types.Handle
	actorID      types.EntityID
	eventID      uint64
}

func NewMeleeExecutionService(w *ecs.World, equipment *CombatEquipmentResolver, sectors *SectorResolver,
	creatures *CreatureDamageService, objects *ObjectDamageService, events *AttackEventSequence, sender attackResultSender,
) (*MeleeExecutionService, error) {
	if w == nil || equipment == nil || equipment.world != w || sectors == nil || sectors.world != w ||
		creatures == nil || creatures.world != w || objects == nil || objects.world != w || events == nil {
		return nil, ErrInvalidMeleeExecution
	}
	return &MeleeExecutionService{world: w, equipment: equipment, sectors: sectors, creatures: creatures,
		objects: objects, events: events, sender: sender,
		profiles:     ecs.GetOrCreateStorage[components.CharacterProfile](w),
		stats:        ecs.GetOrCreateStorage[components.EntityStats](w),
		identities:   ecs.GetOrCreateStorage[ecs.ExternalID](w),
		health:       ecs.GetOrCreateStorage[components.EntityHealth](w),
		objectHealth: ecs.GetOrCreateStorage[components.ObjectInternalState](w),
		dropped:      ecs.GetOrCreateStorage[components.DroppedItem](w),
		contacts:     make([]SectorHit, 0, MeleeMaxContacts)}, nil
}

func (s *MeleeExecutionService) strength(actor types.Handle) (float64, error) {
	profile, exists := s.profiles.Get(actor)
	if !exists || !s.world.Alive(actor) {
		return 0, ErrInvalidMeleeExecution
	}
	return float64(characterattrs.Get(profile.Attributes, characterattrs.STR)), nil
}

// eligible filters health state before nearest selection. A malformed or
// unprepared damageable body is an error rather than silent immunity.
func (s *MeleeExecutionService) eligible(hit SectorHit) (bool, bool, error) {
	if !s.world.Alive(hit.Handle) {
		return false, false, nil
	}
	identity, ok := s.identities.Get(hit.Handle)
	if !ok || identity.ID != hit.EntityID || identity.ID == 0 || s.world.GetHandleByEntityID(identity.ID) != hit.Handle {
		return false, false, ErrInvalidMeleeExecution
	}
	if hit.EntityID == s.actorID || hit.Handle == s.actor {
		return false, false, nil
	}
	if s.health.Has(hit.Handle) {
		id, _, err := s.creatures.readTarget(hit.Handle)
		if err == ErrCreatureTargetDead {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		if s.creatures.targets[hit.Handle] != id || !s.creatures.stats.IsPlayerPrepared(id, hit.Handle) || !s.creatures.visual.IsPrepared(hit.Handle) {
			return false, false, ErrCreatureTargetUnprepared
		}
		return true, true, nil
	}
	if s.dropped.Has(hit.Handle) {
		return false, false, nil
	}
	state, hasState := s.objectHealth.Get(hit.Handle)
	if !hasState || !state.HasHP {
		return false, false, nil
	}
	_, err := s.objects.readTarget(hit.Handle)
	if err == ErrObjectDamageTargetDead {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !s.objects.state.Prepared[hit.Handle] {
		return false, false, ErrObjectDamageTargetUnprepared
	}
	return false, true, nil
}

func (s *MeleeExecutionService) prepare(actor types.Handle, actorID types.EntityID, definition *actiondefs.Definition, action *PreparedMeleeAction, aim float64) error {
	if s.active {
		return ErrMeleeExecutionBusy
	}
	s.active, s.actor, s.actorID, s.count = true, actor, actorID, 0
	identity, hasID := s.identities.Get(actor)
	if !s.world.Alive(actor) || !hasID || identity.ID != actorID || s.world.GetHandleByEntityID(actorID) != actor || directionActionUnavailable(s.world, actor) {
		return ErrInvalidMeleeExecution
	}
	if _, _, err := s.creatures.readTarget(actor); err != nil {
		return err
	}
	stats, exists := s.stats.Get(actor)
	if !exists || math.IsNaN(stats.Stamina) || math.IsInf(stats.Stamina, 0) || stats.Stamina < 0 || math.IsNaN(stats.Energy) || math.IsInf(stats.Energy, 0) {
		return ErrInvalidMeleeExecution
	}
	strength, err := s.strength(actor)
	if err != nil {
		return err
	}
	weapon, err := s.equipment.ResolveMeleeWeapon(actor, action, strength)
	if err != nil {
		return err
	}
	s.contacts, err = s.sectors.ResolveBoundedInto(actor, aim, *definition.Sector, s.contacts[:0])
	if err != nil {
		return err
	}
	for _, contact := range s.contacts {
		creature, eligible, err := s.eligible(contact)
		if err != nil {
			return err
		}
		if !eligible {
			continue
		}
		if definition.Combat.HitMode == actiondefs.HitNearest && s.count != 0 {
			continue
		}
		if s.count == MeleeMaxHits {
			return ErrMeleeExecutionBusy
		}
		var commit meleeDamageCommit
		commit.creature = creature
		if creature {
			commit.soft, err = s.creatures.prepareDamage(contact.Handle, weapon.RawDamage)
		} else {
			commit.object, err = s.objects.prepareDamage(contact.Handle, weapon.RawDamage)
		}
		if err != nil {
			return err
		}
		s.commits[s.count] = commit
		damage := commit.object.result.Damage
		if creature {
			damage = commit.soft.result.Damage
		}
		s.hits[s.count] = netproto.AttackHit{TargetId: uint64(contact.EntityID), Damage: damage}
		s.count++
	}
	// Every calculation succeeds before any destruction slot is reserved.
	for i := 0; i < s.count; i++ {
		if !s.commits[i].creature {
			if err := s.objects.reserveDamage(&s.commits[i].object); err != nil {
				return err
			}
		}
	}
	s.eventID, err = s.events.Next()
	return err
}

func (s *MeleeExecutionService) abort() {
	for i := 0; i < s.count; i++ {
		if !s.commits[i].creature {
			s.objects.abortDamage(s.commits[i].object)
		}
	}
	s.reset()
}

func (s *MeleeExecutionService) commit() {
	for i := 0; i < s.count; i++ {
		if s.commits[i].creature {
			s.creatures.commitDamage(s.commits[i].soft)
		} else {
			s.objects.commitDamage(s.commits[i].object)
		}
	}
}

func (s *MeleeExecutionService) after() {
	for i := 0; i < s.count; i++ {
		if !s.commits[i].creature {
			s.objects.finalizeDamage(s.commits[i].object)
		}
	}
	if s.sender != nil {
		s.sender.SendAttackResult(s.actor, s.actorID, s.eventID, s.hits[:s.count])
	}
	s.reset()
}

func (s *MeleeExecutionService) reset() {
	clear(s.commits[:s.count])
	clear(s.hits[:s.count])
	s.contacts = s.contacts[:0]
	s.active, s.count, s.actor, s.actorID, s.eventID = false, 0, types.InvalidHandle, 0, 0
}

// MeleeActionHandler contains only definition parameters. Weapon capabilities,
// selectors and damage multipliers are compiled outside action execution.
type MeleeActionHandler struct {
	execution    *MeleeExecutionService
	definition   *actiondefs.Definition
	prepared     *PreparedMeleeAction
	preparedHere bool
}

func NewMeleeActionHandler(execution *MeleeExecutionService, definition *actiondefs.Definition, prepared *PreparedMeleeAction) (*MeleeActionHandler, error) {
	if execution == nil || definition == nil || definition.Sector == nil || definition.Combat == nil || prepared == nil || prepared.catalog != execution.equipment.catalog ||
		(definition.Combat.HitMode != actiondefs.HitAll && definition.Combat.HitMode != actiondefs.HitNearest) {
		return nil, ErrInvalidMeleeExecution
	}
	return &MeleeActionHandler{execution: execution, definition: definition, prepared: prepared}, nil
}

func (h *MeleeActionHandler) UnavailableReason(w *ecs.World, _ types.EntityID, actor types.Handle) string {
	if w != h.execution.world {
		return "ACTION_UNAVAILABLE"
	}
	strength, err := h.execution.strength(actor)
	if err != nil {
		return "ACTION_UNAVAILABLE"
	}
	_, err = h.execution.equipment.ResolveMeleeWeapon(actor, h.prepared, strength)
	return meleeFailureReason(err)
}

func (h *MeleeActionHandler) ValidateTarget(_ *ecs.World, _ types.EntityID, _ types.Handle, target ActionTarget) string {
	if !validActionAim(target.AimAngle) {
		return "ACTION_INVALID_TARGET"
	}
	return ""
}

func (*MeleeActionHandler) Start(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) ActionResult {
	return ActionResult{Outcome: ActionFailed, Reason: "ACTION_FAILED"} // Only the atomic completion capability may execute.
}

func (*MeleeActionHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}

func (h *MeleeActionHandler) PrepareCompletion(w *ecs.World, id types.EntityID, actor types.Handle, target ActionTarget, _ uint64) string {
	if w != h.execution.world || h.execution.active {
		return "ACTION_COMBAT_BUSY"
	}
	h.preparedHere = true
	err := h.execution.prepare(actor, id, h.definition, h.prepared, target.AimAngle)
	if err != nil {
		h.AbortPrepared()
	}
	return meleeFailureReason(err)
}

func (h *MeleeActionHandler) CommitPrepared() { h.execution.commit() }
func (h *MeleeActionHandler) AbortPrepared() {
	if h.preparedHere {
		h.execution.abort()
		h.preparedHere = false
	}
}
func (h *MeleeActionHandler) AfterCompletion() { h.execution.after(); h.preparedHere = false }

func meleeFailureReason(err error) string {
	if err == nil {
		return ""
	}
	if err == ErrMeleeWeaponUnavailable {
		return "ACTION_REQUIRES_EQUIPMENT"
	}
	return "ACTION_COMBAT_BUSY"
}
