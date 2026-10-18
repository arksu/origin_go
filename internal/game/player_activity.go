package game

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"time"

	"origin/internal/ecs"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	playerActivityBatchSize     = 512
	playerActivityFlushInterval = time.Second
	playerActivityWriteTimeout  = 5 * time.Second
)

type characterActivity struct {
	sessionID     uint64
	previousPing  time.Time
	onlineSeconds int64
	remainder     time.Duration
	lastPingMs    int64
	revision      uint64
}

type activityCheckpoint struct {
	id                        types.EntityID
	onlineSeconds, lastPingMs int64
	revision                  uint64
}

// Player activity is connection metadata, independent of ECS attachment and
// transfers. Retain each character's fractional second until process restart.
type playerActivity struct {
	mu         sync.Mutex
	characters map[types.EntityID]*characterActivity
	dirty      map[types.EntityID]*characterActivity
	stopped    bool
	writeMu    sync.Mutex
	persist    func(context.Context, repository.UpdateCharacterActivityParams) error
	onPing     func(types.EntityID, int64)
	logger     *zap.Logger
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func newPlayerActivity(persist func(context.Context, repository.UpdateCharacterActivityParams) error, onPing func(types.EntityID, int64), logger *zap.Logger) *playerActivity {
	return &playerActivity{
		characters: make(map[types.EntityID]*characterActivity),
		dirty:      make(map[types.EntityID]*characterActivity),
		persist:    persist, onPing: onPing, logger: logger, stopCh: make(chan struct{}),
	}
}

func (a *playerActivity) beginSession(character types.EntityID, session uint64, savedSeconds int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped || character == 0 || session == 0 {
		return
	}
	state := a.characters[character]
	if state == nil {
		state = &characterActivity{}
		a.characters[character] = state
	}
	state.onlineSeconds = max(state.onlineSeconds, savedSeconds)
	state.sessionID, state.previousPing = session, time.Time{}
}

func (a *playerActivity) recordPing(character types.EntityID, session uint64, at time.Time) bool {
	a.mu.Lock()
	state := a.characters[character]
	if a.stopped || session == 0 || state == nil || state.sessionID != session {
		a.mu.Unlock()
		return false
	}
	if !state.previousPing.IsZero() {
		elapsed := at.Sub(state.previousPing)
		if elapsed > 0 {
			seconds := int64(elapsed / time.Second)
			state.remainder += elapsed % time.Second
			seconds += int64(state.remainder / time.Second)
			state.remainder %= time.Second
			state.onlineSeconds += min(seconds, math.MaxInt64-state.onlineSeconds)
		}
	}
	// Preserve time.Time's monotonic part for elapsed intervals. A backwards wall
	// clock sample without a monotonic part cannot move the accounting anchor back.
	if state.previousPing.IsZero() || at.Sub(state.previousPing) > 0 {
		state.previousPing = at
	}
	state.lastPingMs = max(state.lastPingMs, at.UnixMilli())
	state.revision++
	a.dirty[character] = state
	lastPingMs := state.lastPingMs
	a.mu.Unlock()
	if a.onPing != nil {
		a.onPing(character, lastPingMs)
	}
	return true
}

func (a *playerActivity) endSession(character types.EntityID, session uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.characters[character]
	if state == nil || session == 0 || state.sessionID != session {
		return false
	}
	state.sessionID, state.previousPing = 0, time.Time{}
	return true
}

// Serialize periodic, disconnect and shutdown writes. Acknowledging a snapshot
// never clears a newer ping; absolute SQL values make retries idempotent.
func (a *playerActivity) flush(ctx context.Context, only types.EntityID) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	capacity := playerActivityBatchSize
	if only != 0 {
		capacity = 1
	}
	batch := make([]activityCheckpoint, 0, capacity)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch = batch[:0]
		a.mu.Lock()
		if only != 0 {
			if state := a.dirty[only]; state != nil {
				batch = append(batch, activityCheckpoint{only, state.onlineSeconds, state.lastPingMs, state.revision})
			}
		} else {
			for id, state := range a.dirty {
				batch = append(batch, activityCheckpoint{id, state.onlineSeconds, state.lastPingMs, state.revision})
				if len(batch) == playerActivityBatchSize {
					break
				}
			}
		}
		a.mu.Unlock()
		if len(batch) == 0 {
			return nil
		}
		params := repository.UpdateCharacterActivityParams{
			Ids: make([]int64, len(batch)), OnlineTimes: make([]int64, len(batch)), LastPingMs: make([]int64, len(batch)),
		}
		for i, snapshot := range batch {
			params.Ids[i], params.OnlineTimes[i], params.LastPingMs[i] = int64(snapshot.id), snapshot.onlineSeconds, snapshot.lastPingMs
		}
		if err := a.persist(ctx, params); err != nil {
			return err
		}
		a.mu.Lock()
		for _, snapshot := range batch {
			if state := a.dirty[snapshot.id]; state != nil && state.revision == snapshot.revision {
				delete(a.dirty, snapshot.id)
			}
		}
		a.mu.Unlock()
		if only != 0 {
			return nil
		}
	}
}

func (a *playerActivity) flushWithTimeout(only types.EntityID) {
	ctx, cancel := context.WithTimeout(context.Background(), playerActivityWriteTimeout)
	defer cancel()
	if err := a.flush(ctx, only); err != nil {
		a.logger.Error("Failed to persist character activity; checkpoints retained", zap.Error(err))
	}
}

func (a *playerActivity) start() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(playerActivityFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.flushWithTimeout(0)
			}
		}
	}()
}

func (a *playerActivity) stop() {
	a.stopOnce.Do(func() {
		a.mu.Lock()
		a.stopped = true
		a.mu.Unlock()
		close(a.stopCh)
		a.wg.Wait()
		a.flushWithTimeout(0)
	})
}

// Login hydrates offline last-seen metadata before entry. Live ping callbacks use
// the stable index captured at initialization, without touching ECS from I/O.
func (g *Game) rememberCharacterPing(character types.EntityID, at sql.NullTime) {
	if !at.Valid || g.shardManager == nil {
		return
	}
	surface := g.shardManager.GetShard(hearth.SurfaceLayer)
	if surface == nil {
		return
	}
	surface.mu.Lock()
	defer surface.mu.Unlock()
	if runtime, ok := ecs.TryGetResource[hearth.Runtime](surface.world); ok && runtime.Index != nil {
		runtime.Index.RecordPing(character, at.Time.UnixMilli())
	}
}
