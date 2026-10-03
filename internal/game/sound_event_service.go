package game

import (
	"fmt"
	"math"
	"slices"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/sounddefs"
	"origin/internal/types"

	"google.golang.org/protobuf/proto"
)

type soundBatchDelivery struct {
	result     network.AudioSendResult
	bytes      int
	encodeTime time.Duration
}

type soundEventSender interface {
	SendSoundBatch(types.EntityID, uint64, *netproto.S2C_SoundBatch) soundBatchDelivery
}

type soundPoint struct{ x, y float64 }

type worldSoundEvent struct {
	point    soundPoint
	profile  *sounddefs.Profile
	sequence uint64
	layer    int
	unixMs   int64
}

type soundEntry struct {
	key    string
	point  soundPoint
	radius float64
	gain   float32
}

type SoundTickStats struct {
	Events, EventDrops, InvalidEvents, StaleEvents                      uint64
	Cells, Candidates, Recipients, Entries                              uint64
	Messages, Bytes                                                     uint64
	EntryDrops, ByteDrops, WorkDrops                                    uint64
	TruncatedQueries, Disconnected, TransportDrops                      uint64
	CellBudgetCutoffs, CandidateBudgetCutoffs, GlobalEntryBudgetCutoffs uint64
	PropagationTime, EncodeTime                                         time.Duration
}

type SoundEventService struct {
	config        config.AudioConfig
	profiles      *sounddefs.Registry
	sender        soundEventSender
	listeners     soundListenerIndex
	events        []worldSoundEvent
	sequence      uint64
	recipients    []*soundListener
	sounds        []netproto.S2C_Sound
	gains         []float32
	soundPointers []*netproto.S2C_Sound
	stats         SoundTickStats
	LastTick      SoundTickStats
}

func NewSoundEventService(profiles *sounddefs.Registry, audio config.AudioConfig, sender soundEventSender) (*SoundEventService, error) {
	if profiles == nil {
		return nil, fmt.Errorf("sound service requires sound definitions")
	}
	if err := audio.Validate(); err != nil {
		return nil, err
	}
	if err := profiles.ValidateHearing(audio.BaseHearing, audio.MaxEffectiveRadius); err != nil {
		return nil, err
	}
	service := &SoundEventService{
		config: audio, profiles: profiles, sender: sender,
		listeners:     newSoundListenerIndex(audio.ListenerCellSize),
		events:        make([]worldSoundEvent, 0, min(audio.MaxEventsPerTick, 1024)),
		recipients:    make([]*soundListener, 0, min(audio.MaxEntriesPerTick, 128)),
		sounds:        make([]netproto.S2C_Sound, audio.MaxEntriesPerBatch),
		gains:         make([]float32, audio.MaxEntriesPerBatch),
		soundPointers: make([]*netproto.S2C_Sound, audio.MaxEntriesPerBatch),
	}
	for index := range service.sounds {
		service.sounds[index].DistanceGain = &service.gains[index]
		service.soundPointers[index] = &service.sounds[index]
	}
	return service, nil
}

// EffectiveHearing is the single player-hearing policy. No per-player storage
// or per-event population scan is needed while the shared base is universal.
func (service *SoundEventService) EffectiveHearing(_ types.Handle) float64 {
	return service.config.BaseHearing
}

func (service *SoundEventService) maximumListenerHearing() float64 {
	// A future individual modifier must update this bound as membership changes,
	// rather than scanning listeners when each sound is emitted.
	return service.config.BaseHearing
}

func (service *SoundEventService) Attach(w *ecs.World, handle types.Handle, clientID uint64, epoch uint32) bool {
	if service == nil || w == nil || !w.Alive(handle) || clientID == 0 || epoch == 0 {
		return false
	}
	entityID, exists := w.GetExternalID(handle)
	position, hasPosition := ecs.GetComponent[components.Transform](w, handle)
	if !exists || !hasPosition || !finiteSoundPoint(position.X, position.Y) {
		return false
	}
	service.listeners.add(handle, entityID, clientID, epoch, position.X, position.Y)
	return true
}

func (service *SoundEventService) Detach(handle types.Handle, clientID uint64) {
	if service == nil {
		return
	}
	if listener := service.listeners.members[handle]; listener != nil && (clientID == 0 || listener.clientID == clientID) {
		service.listeners.remove(handle)
	}
}

func (service *SoundEventService) OnPositionCommitted(handle types.Handle, positionX, positionY float64) {
	if service != nil && finiteSoundPoint(positionX, positionY) {
		service.listeners.move(handle, positionX, positionY)
	}
}

func finiteSoundPoint(positionX, positionY float64) bool {
	return !math.IsNaN(positionX) && !math.IsNaN(positionY) && !math.IsInf(positionX, 0) && !math.IsInf(positionY, 0)
}

func (service *SoundEventService) EmitPoint(w *ecs.World, point soundPoint, key string) bool {
	if service == nil || w == nil || key == "" {
		return false
	}
	profile, exists := service.profiles.Get(key)
	if !exists || !finiteSoundPoint(point.x, point.y) {
		service.stats.InvalidEvents++
		return false
	}
	if profile.Mode != "world" {
		return false
	}
	service.stats.Events++
	service.sequence++
	event := worldSoundEvent{point: point, profile: profile, sequence: service.sequence, layer: w.Layer, unixMs: ecs.GetResource[ecs.TimeState](w).UnixMs}
	return service.offerEvent(event)
}

// A weakest-first heap keeps overload selection bounded without rescanning the
// full event buffer for every offered sound. Equal priority keeps earlier events.
func weakerSound(first, second worldSoundEvent) bool {
	if first.profile.Priority != second.profile.Priority {
		return first.profile.Priority < second.profile.Priority
	}
	return first.sequence > second.sequence
}

func (service *SoundEventService) offerEvent(event worldSoundEvent) bool {
	if len(service.events) == service.config.MaxEventsPerTick {
		service.stats.EventDrops++
		if !weakerSound(service.events[0], event) {
			return false
		}
		service.events[0] = event
		for parent := 0; ; {
			child := parent*2 + 1
			if child >= len(service.events) {
				break
			}
			if child+1 < len(service.events) && weakerSound(service.events[child+1], service.events[child]) {
				child++
			}
			if !weakerSound(service.events[child], service.events[parent]) {
				break
			}
			service.events[parent], service.events[child] = service.events[child], service.events[parent]
			parent = child
		}
		return true
	}
	service.events = append(service.events, event)
	for child := len(service.events) - 1; child > 0; {
		parent := (child - 1) / 2
		if !weakerSound(service.events[child], service.events[parent]) {
			break
		}
		service.events[parent], service.events[child] = service.events[child], service.events[parent]
		child = parent
	}
	return true
}

func (service *SoundEventService) Flush(w *ecs.World) {
	if service == nil {
		return
	}
	if len(service.events) == 0 {
		service.LastTick = service.stats
		observeSoundTick(w.Layer, service.stats)
		service.stats = SoundTickStats{}
		return
	}
	started := time.Now()
	nowMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	slices.SortFunc(service.events, func(first, second worldSoundEvent) int {
		if weakerSound(first, second) {
			return 1
		}
		if weakerSound(second, first) {
			return -1
		}
		return 0
	})
	positions := ecs.GetOrCreateStorage[components.Transform](w)
	for eventIndex, event := range service.events {
		if event.layer != w.Layer || nowMs-event.unixMs > int64(service.config.FreshnessMs) {
			service.stats.StaleEvents++
			continue
		}
		if limit := service.propagate(w, positions, event, nowMs); limit != "" {
			switch limit {
			case "cells":
				service.stats.CellBudgetCutoffs++
			case "candidates":
				service.stats.CandidateBudgetCutoffs++
			case "entries":
				service.stats.GlobalEntryBudgetCutoffs++
			}
			service.stats.WorkDrops += uint64(len(service.events) - eventIndex)
			service.stats.TruncatedQueries++
			break
		}
	}
	service.stats.PropagationTime = time.Since(started)
	for _, listener := range service.recipients {
		for index, entry := range listener.entries {
			service.setProtoSound(index, entry)
		}
		batch := &netproto.S2C_SoundBatch{Sounds: service.soundPointers[:len(listener.entries)], StreamEpoch: listener.streamEpoch, ServerTimeMs: nowMs}
		if service.sender != nil && w.Alive(listener.handle) {
			result := service.sender.SendSoundBatch(listener.entityID, listener.clientID, batch)
			service.stats.EncodeTime += result.encodeTime
			switch result.result {
			case network.AudioSendAccepted:
				service.stats.Messages++
				service.stats.Bytes += uint64(result.bytes)
			case network.AudioSendClosed:
				service.stats.Disconnected++
			default:
				service.stats.TransportDrops++
			}
		} else {
			service.stats.Disconnected++
		}
		listener.entries = listener.entries[:0]
		listener.encodedBytes = 0
	}
	service.LastTick = service.stats
	observeSoundTick(w.Layer, service.stats)
	service.stats = SoundTickStats{}
	clear(service.recipients)
	service.recipients = service.recipients[:0]
	clear(service.events)
	service.events = service.events[:0]
}

func (service *SoundEventService) propagate(w *ecs.World, positions *ecs.ComponentStorage[components.Transform], event worldSoundEvent, nowMs int64) string {
	radius := event.profile.Loudness * service.maximumListenerHearing()
	minimum := service.listeners.cellAt(event.point.x-radius, event.point.y-radius)
	maximum := service.listeners.cellAt(event.point.x+radius, event.point.y+radius)
	for cellX := minimum.x; cellX <= maximum.x; cellX++ {
		for cellY := minimum.y; cellY <= maximum.y; cellY++ {
			if service.stats.Cells >= uint64(service.config.MaxCellsPerTick) {
				return "cells"
			}
			if service.stats.Candidates >= uint64(service.config.MaxCandidatesPerTick) {
				return "candidates"
			}
			if service.stats.Entries >= uint64(service.config.MaxEntriesPerTick) {
				return "entries"
			}
			service.stats.Cells++
			for _, handle := range service.listeners.cells[soundCell{cellX, cellY}] {
				if service.stats.Candidates >= uint64(service.config.MaxCandidatesPerTick) {
					return "candidates"
				}
				if service.stats.Entries >= uint64(service.config.MaxEntriesPerTick) {
					return "entries"
				}
				service.stats.Candidates++
				listener := service.listeners.members[handle]
				if listener == nil || !w.Alive(handle) {
					continue
				}
				position, exists := positions.Get(handle)
				if !exists || !finiteSoundPoint(position.X, position.Y) {
					continue
				}
				listenerRadius := event.profile.Loudness * service.EffectiveHearing(handle)
				distanceX, distanceY := position.X-event.point.x, position.Y-event.point.y
				distanceSquared := distanceX*distanceX + distanceY*distanceY
				if distanceSquared >= listenerRadius*listenerRadius {
					continue
				}
				if len(listener.entries) == service.config.MaxEntriesPerBatch {
					service.stats.EntryDrops++
					continue
				}
				fraction := math.Sqrt(distanceSquared) / listenerRadius
				remaining := 1 - fraction
				entry := soundEntry{key: event.profile.Key, point: event.point, radius: listenerRadius, gain: float32(remaining * remaining * (1 + 2*fraction))}
				service.setProtoSound(0, entry)
				entrySize := proto.Size(&service.sounds[0])
				// Include the entry, recipient/time envelope and outer message framing.
				encodedEntries := listener.encodedBytes + 1 + varintBytes(uint64(entrySize)) + entrySize
				batchSize := encodedEntries + 1 + varintBytes(uint64(listener.streamEpoch)) + 1 + varintBytes(uint64(nowMs))
				packetSize := 2 + varintBytes(uint64(batchSize)) + batchSize
				if packetSize > service.config.MaxBatchBytes {
					service.stats.ByteDrops++
					continue
				}
				if len(listener.entries) == 0 {
					service.recipients = append(service.recipients, listener)
					service.stats.Recipients++
				}
				listener.entries = append(listener.entries, entry)
				listener.encodedBytes = encodedEntries
				service.stats.Entries++
			}
		}
	}
	return ""
}

func (service *SoundEventService) setProtoSound(index int, entry soundEntry) {
	payload := &service.sounds[index]
	payload.SoundKey, payload.X, payload.Y, payload.MaxHearDistance = entry.key, entry.point.x, entry.point.y, entry.radius
	service.gains[index] = entry.gain
}

func varintBytes(value uint64) int {
	count := 1
	for value >= 128 {
		value >>= 7
		count++
	}
	return count
}
