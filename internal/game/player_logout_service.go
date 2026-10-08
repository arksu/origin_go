package game

import (
	"errors"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/types"
)

var (
	ErrInvalidPlayerLogoutService = errors.New("player logout: invalid dependencies or policies")
	ErrInvalidPlayerLogoutTarget  = errors.New("player logout: stale, unprepared or changed detached target")
	ErrInvalidPlayerLogoutHealth  = errors.New("player logout: invalid health or knockout deadline")
)

const logoutPolicyRetry = time.Second

// PlayerLogoutService is a World-bound, read-only policy evaluator. Capture and
// final cleanup remain in the detached expiry lifecycle. Policies are copied at
// construction and may not mutate ECS or perform I/O during Check.
type PlayerLogoutService struct {
	world      *ecs.World
	detached   *ecs.DetachedEntities
	identities *ecs.ComponentStorage[ecs.ExternalID]
	time       *ecs.TimeState
	policies   []ecs.LogoutPolicy
}

func NewPlayerLogoutService(world *ecs.World, policies ...ecs.LogoutPolicy) (*PlayerLogoutService, error) {
	if world == nil || len(policies) == 0 {
		return nil, ErrInvalidPlayerLogoutService
	}
	detached, hasDetached := ecs.TryGetResource[ecs.DetachedEntities](world)
	clock, hasTime := ecs.TryGetResource[ecs.TimeState](world)
	if !hasDetached || !hasTime {
		return nil, ErrInvalidPlayerLogoutService
	}
	for _, policy := range policies {
		if policy == nil {
			return nil, ErrInvalidPlayerLogoutService
		}
	}
	service := &PlayerLogoutService{
		world: world, detached: detached, time: clock,
		identities: ecs.GetOrCreateStorage[ecs.ExternalID](world),
		policies:   append([]ecs.LogoutPolicy(nil), policies...),
	}
	world.AddDespawnObserver(func(handle types.Handle) { detached.Release(handle) })
	// Removing EntityHealth also retires a living body's preparation when the
	// same handle is converted to a corpse rather than despawned.
	health := ecs.GetOrCreateStorage[components.EntityHealth](world)
	world.AddComponentObserver(components.EntityHealthComponentID, func(handle types.Handle) {
		if !health.Has(handle) {
			detached.Release(handle)
		}
	})
	return service, nil
}

// PreparePlayer reserves the persistent scheduler record before publication.
// Reattachment reuses the exact handle and never resets detached deadlines.
func (s *PlayerLogoutService) PreparePlayer(handle types.Handle) error {
	if s == nil || !s.world.Alive(handle) {
		return ErrInvalidPlayerLogoutTarget
	}
	identity, exists := s.identities.Get(handle)
	if !exists || identity.ID == 0 || s.world.GetHandleByEntityID(identity.ID) != handle {
		return ErrInvalidPlayerLogoutTarget
	}
	return s.detached.PreparePlayer(identity.ID, handle)
}

// Check always inspects current identity and detached state. RetryAfter is a
// scheduling hint, never an authoritative permission to remove a body later.
func (s *PlayerLogoutService) Check(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
	if s == nil || !s.world.Alive(context.Handle) {
		return ecs.LogoutDecision{Blocked: true, RetryAfter: logoutPolicyRetry}, ErrInvalidPlayerLogoutTarget
	}
	identity, exists := s.identities.Get(context.Handle)
	detached, isDetached := s.detached.GetDetachedEntity(context.EntityID)
	if !exists || identity.ID != context.EntityID || context.EntityID == 0 ||
		s.world.GetHandleByEntityID(context.EntityID) != context.Handle ||
		!s.detached.IsPrepared(context.EntityID, context.Handle) || !isDetached ||
		detached != context.Detached || detached.Handle != context.Handle {
		return ecs.LogoutDecision{Blocked: true, RetryAfter: logoutPolicyRetry}, ErrInvalidPlayerLogoutTarget
	}
	decision := ecs.LogoutDecision{}
	if context.Time.Now.Before(detached.SaveRetryAt) {
		decision = ecs.LogoutDecision{Blocked: true, RetryAfter: detached.SaveRetryAt.Sub(context.Time.Now)}
	}
	for _, policy := range s.policies {
		current, err := policy.Check(context)
		if err != nil {
			return ecs.LogoutDecision{Blocked: true, RetryAfter: logoutPolicyRetry}, err
		}
		if !current.Blocked {
			continue
		}
		if current.RetryAfter <= 0 {
			current.RetryAfter = logoutPolicyRetry
		}
		if !decision.Blocked || current.RetryAfter < decision.RetryAfter {
			decision.RetryAfter = current.RetryAfter
		}
		decision.Blocked = true
	}
	return decision, nil
}

func (s *PlayerLogoutService) RequestRecheck(handle types.Handle) {
	if s != nil {
		s.detached.RequestRecheck(handle, s.time.Now)
	}
}

type disconnectDelayLogoutPolicy struct{}

func NewDisconnectDelayLogoutPolicy() ecs.LogoutPolicy { return disconnectDelayLogoutPolicy{} }

func (disconnectDelayLogoutPolicy) Check(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
	if context.Time.Now.Before(context.Detached.ExpirationTime) {
		return ecs.LogoutDecision{Blocked: true, RetryAfter: context.Detached.ExpirationTime.Sub(context.Time.Now)}, nil
	}
	return ecs.LogoutDecision{}, nil
}

type combatLogoutPolicy struct {
	world    *ecs.World
	health   *ecs.ComponentStorage[components.EntityHealth]
	activity *ecs.CombatActivityState
}

func NewCombatLogoutPolicy(world *ecs.World) (ecs.LogoutPolicy, error) {
	if world == nil {
		return nil, ErrInvalidPlayerLogoutService
	}
	activity, exists := ecs.TryGetResource[ecs.CombatActivityState](world)
	if !exists {
		return nil, ErrInvalidPlayerLogoutService
	}
	return combatLogoutPolicy{world: world, health: ecs.GetOrCreateStorage[components.EntityHealth](world), activity: activity}, nil
}

func (p combatLogoutPolicy) Check(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
	if !p.world.Alive(context.Handle) || !p.activity.IsPrepared(context.Handle, context.EntityID) {
		return ecs.LogoutDecision{}, ErrInvalidPlayerLogoutTarget
	}
	health, exists := p.health.Get(context.Handle)
	if !exists || entityhealth.ValidatePools(health.SHP, health.HHP) != nil || health.SHP > health.HHP || health.KOUntilUnixMs < 0 {
		return ecs.LogoutDecision{}, ErrInvalidPlayerLogoutHealth
	}
	state, _ := p.activity.Capture(context.Handle)
	if err := ecs.ValidateCombatState(state); err != nil {
		return ecs.LogoutDecision{}, err
	}
	if context.Time.UnixMs < 0 {
		return ecs.LogoutDecision{}, ecs.ErrInvalidCombatActivity
	}
	if (state.HasEvent && context.Time.UnixMs < state.LastCombatEventAtUnixMs+ecs.CombatLogoutHoldMs) ||
		(health.KOUntilUnixMs != 0 && context.Time.UnixMs <= health.KOUntilUnixMs) {
		// Wall-based eligibility is always re-evaluated. Capping the hint before
		// duration conversion avoids overflow and bounds wall-clock jump lag.
		return ecs.LogoutDecision{Blocked: true, RetryAfter: logoutPolicyRetry}, nil
	}
	return ecs.LogoutDecision{}, nil
}
