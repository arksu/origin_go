package game

import (
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type soundEventSender interface {
	SendSound(entityID types.EntityID, sound *netproto.S2C_Sound)
}

type SoundEventService struct {
	logger *zap.Logger
	sender soundEventSender
}

func NewSoundEventService(sender soundEventSender, logger *zap.Logger) *SoundEventService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SoundEventService{
		logger: logger,
		sender: sender,
	}
}

func (s *SoundEventService) SetSender(sender soundEventSender) {
	if s == nil {
		return
	}
	s.sender = sender
}

func (s *SoundEventService) EmitForVisibleTarget(
	w *ecs.World,
	targetHandle types.Handle,
	soundKey string,
) {
	if s == nil || s.sender == nil || w == nil {
		return
	}

	normalizedSoundKey := strings.TrimSpace(soundKey)
	if normalizedSoundKey == "" {
		return
	}
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return
	}

	transform, hasTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	if !hasTransform {
		return
	}

	visibilityState := ecs.GetResource[ecs.VisibilityState](w)
	if visibilityState == nil {
		return
	}

	visibilityState.Mu.RLock()
	observers, hasObservers := visibilityState.ObserversByVisibleTarget[targetHandle]
	if !hasObservers || len(observers) == 0 {
		visibilityState.Mu.RUnlock()
		return
	}
	observerHandles := make([]types.Handle, 0, len(observers))
	for observerHandle := range observers {
		observerHandles = append(observerHandles, observerHandle)
	}
	visibilityState.Mu.RUnlock()

	payload := &netproto.S2C_Sound{
		SoundKey:        normalizedSoundKey,
		X:               transform.X,
		Y:               transform.Y,
		MaxHearDistance: constt.DefaultMaxHearDistance,
	}

	for _, observerHandle := range observerHandles {
		if observerHandle == types.InvalidHandle || !w.Alive(observerHandle) {
			continue
		}
		observerEntityID, ok := w.GetExternalID(observerHandle)
		if !ok || observerEntityID == 0 {
			continue
		}
		s.sender.SendSound(observerEntityID, payload)
	}
}
