package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/mathutil"
	"origin/internal/types"

	"go.uber.org/zap"
)

var (
	ErrSpotNotFound       = errors.New("spot not found")
	ErrSpotSaveBusy       = errors.New("spot save is busy")
	ErrSpotManagerStopped = errors.New("spot manager admission stopped")
	ErrInvalidSpotState   = errors.New("spot state must be a JSON object")
)

const spotPersistenceTimeout = 5 * time.Second

// SpotInfo is immutable geometry in world units. A tile belongs to a spot when
// its center is inside the circle, including the boundary.
type SpotInfo struct {
	ID                   int64
	Region, Layer        int
	Type                 string
	DistrictX, DistrictY int
	CenterX, CenterY     int
	Radius               int
	PeakQuality          int16
}

// SpotStateSnapshot owns its JSON bytes. State remains an opaque JSON object so
// resource-specific fill progress, recovery deadlines and unknown fields survive
// load/save without a second schema or loss of large integer identities.
type SpotStateSnapshot struct {
	State              json.RawMessage
	Revision           int64
	LastRuntimeSeconds int64
}

type managedSpot struct {
	info    SpotInfo
	state   SpotStateSnapshot
	pending *spotStateJob
	saveErr error
}

// SpotManager is owned by the surface shard. State reads, saves and Update must
// run under that shard's lock. Geometry and its chunk index never change after
// construction. Workers receive only owned snapshots, never live spot records.
type SpotManager struct {
	region             int
	minChunk, maxChunk types.ChunkCoord // Inclusive world bounds.
	byID               map[int64]*managedSpot
	byChunk            map[types.ChunkCoord][]int64
	store              SpotStore
	submit             func(PersistenceJob) bool
	logger             *zap.Logger
	pending            []*spotStateJob
	nextPending        int
	stopped            bool
	err                error
}

func NewSpotManager(ctx context.Context, cfg *config.Config, store SpotStore, submit func(PersistenceJob) bool, logger *zap.Logger) (*SpotManager, error) {
	if cfg == nil || store == nil || submit == nil || cfg.Game.Region <= 0 ||
		cfg.Game.WorldWidthChunks <= 0 || cfg.Game.WorldHeightChunks <= 0 ||
		cfg.Game.WorldMinXChunks > math.MaxInt-(cfg.Game.WorldWidthChunks-1) ||
		cfg.Game.WorldMinYChunks > math.MaxInt-(cfg.Game.WorldHeightChunks-1) {
		return nil, errors.New("invalid spot manager configuration")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	loadCtx, cancel := context.WithTimeout(ctx, spotPersistenceTimeout)
	defer cancel()
	rows, err := store.LoadSpots(loadCtx, cfg.Game.Region)
	if err != nil {
		return nil, fmt.Errorf("load spots: %w", err)
	}
	m := &SpotManager{
		region:   cfg.Game.Region,
		minChunk: types.ChunkCoord{X: cfg.Game.WorldMinXChunks, Y: cfg.Game.WorldMinYChunks},
		maxChunk: types.ChunkCoord{
			X: cfg.Game.WorldMinXChunks + cfg.Game.WorldWidthChunks - 1,
			Y: cfg.Game.WorldMinYChunks + cfg.Game.WorldHeightChunks - 1,
		},
		byID: make(map[int64]*managedSpot, len(rows)), byChunk: make(map[types.ChunkCoord][]int64),
		store: store, submit: submit, logger: logger,
		pending: make([]*spotStateJob, 0, len(rows)),
	}
	var previousID int64
	for _, row := range rows {
		// LoadSpots orders by ID; this also rejects duplicates before publication.
		if row.ID <= previousID || row.Region != m.region || row.Layer != 0 ||
			row.Radius <= 0 || row.Radius > math.MaxInt32 ||
			row.CenterX < math.MinInt32 || row.CenterX > math.MaxInt32 ||
			row.CenterY < math.MinInt32 || row.CenterY > math.MaxInt32 ||
			row.Revision <= 0 || row.LastRuntimeSeconds < 0 || !validSpotState(row.State) {
			return nil, fmt.Errorf("invalid spot record %d", row.ID)
		}
		previousID = row.ID
		spot := &managedSpot{
			info: SpotInfo{
				ID: row.ID, Region: row.Region, Layer: row.Layer, Type: row.SpotType,
				DistrictX: row.DistrictX, DistrictY: row.DistrictY,
				CenterX: row.CenterX, CenterY: row.CenterY, Radius: row.Radius, PeakQuality: row.PeakQuality,
			},
			state: SpotStateSnapshot{
				State:    append(json.RawMessage(nil), row.State...),
				Revision: row.Revision, LastRuntimeSeconds: row.LastRuntimeSeconds,
			},
		}
		m.byID[row.ID] = spot
		m.indexSpot(spot.info)
	}
	return m, nil
}

func (m *SpotManager) indexSpot(spot SpotInfo) {
	minX := max(m.minChunk.X, mathutil.FloorDiv(spot.CenterX-spot.Radius, constt.ChunkWorldSize))
	maxX := min(m.maxChunk.X, mathutil.FloorDiv(spot.CenterX+spot.Radius, constt.ChunkWorldSize))
	minY := max(m.minChunk.Y, mathutil.FloorDiv(spot.CenterY-spot.Radius, constt.ChunkWorldSize))
	maxY := min(m.maxChunk.Y, mathutil.FloorDiv(spot.CenterY+spot.Radius, constt.ChunkWorldSize))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			left, top := int64(x)*constt.ChunkWorldSize, int64(y)*constt.ChunkWorldSize
			nearestX := max(left, min(int64(spot.CenterX), left+constt.ChunkWorldSize))
			nearestY := max(top, min(int64(spot.CenterY), top+constt.ChunkWorldSize))
			if spotContainsPoint(spot, nearestX, nearestY) {
				coord := types.ChunkCoord{X: x, Y: y}
				m.byChunk[coord] = append(m.byChunk[coord], spot.ID)
			}
		}
	}
}

func spotContainsPoint(spot SpotInfo, x, y int64) bool {
	dx, dy, radius := x-int64(spot.CenterX), y-int64(spot.CenterY), int64(spot.Radius)
	// SQL INTEGER geometry bounds each squared term and their sum within int64.
	if dx < -radius || dx > radius || dy < -radius || dy > radius {
		return false
	}
	return dx*dx+dy*dy <= radius*radius
}

func (m *SpotManager) Get(id int64) (SpotInfo, bool) {
	spot, ok := m.byID[id]
	if !ok {
		return SpotInfo{}, false
	}
	return spot.info, true
}

// FindAtTile returns all matching IDs in ascending order, reusing dst. An empty
// type matches every resource. Only chunk candidates are visited; there is no
// SQL, terrain lookup, full-catalog traversal or result truncation.
func (m *SpotManager) FindAtTile(layer, tileX, tileY int, spotType string, dst []int64) []int64 {
	dst = dst[:0]
	if layer != 0 {
		return dst
	}
	coord := types.ChunkCoord{X: mathutil.FloorDiv(tileX, constt.ChunkSize), Y: mathutil.FloorDiv(tileY, constt.ChunkSize)}
	ids := m.byChunk[coord]
	if len(ids) == 0 {
		return dst
	}
	// Indexed candidates come from SQL INTEGER circles, so tile centers for
	// their chunks fit int64 even when the caller accepts arbitrary int input.
	x, y := int64(tileX)*constt.CoordPerTile+constt.CoordPerTile/2, int64(tileY)*constt.CoordPerTile+constt.CoordPerTile/2
	for _, id := range ids {
		spot := m.byID[id].info
		if (spotType == "" || spot.Type == spotType) && spotContainsPoint(spot, x, y) {
			dst = append(dst, id)
		}
	}
	return dst
}

// IntersectingChunk returns broad-phase candidates, not tile membership. The
// caller owns the result; changing it cannot change the manager's index.
func (m *SpotManager) IntersectingChunk(layer int, coord types.ChunkCoord, dst []int64) []int64 {
	dst = dst[:0]
	if layer == 0 {
		dst = append(dst, m.byChunk[coord]...)
	}
	return dst
}

func (m *SpotManager) GetState(id int64) (SpotStateSnapshot, bool) {
	spot, ok := m.byID[id]
	if !ok {
		return SpotStateSnapshot{}, false
	}
	snapshot := spot.state
	snapshot.State = append(json.RawMessage(nil), snapshot.State...)
	return snapshot, true
}

// SaveState accepts one standalone state write. Nil means queued, not durable;
// GetState changes only after Update applies the worker's acknowledgement.
// Writes involving world objects must instead join that object's transaction.
func (m *SpotManager) SaveState(id, expectedRevision int64, state json.RawMessage, runtimeSeconds int64) error {
	if m.stopped {
		return ErrSpotManagerStopped
	}
	spot, ok := m.byID[id]
	if !ok {
		return ErrSpotNotFound
	}
	if spot.saveErr != nil {
		return spot.saveErr
	}
	if spot.pending != nil {
		return ErrSpotSaveBusy
	}
	if spot.state.Revision != expectedRevision {
		return ErrSpotRevisionConflict
	}
	if !validSpotState(state) {
		return ErrInvalidSpotState
	}
	if runtimeSeconds < 0 || expectedRevision == math.MaxInt64 {
		return errors.New("invalid spot write runtime or exhausted revision")
	}
	job := &spotStateJob{store: m.store, spot: spot, running: true, write: SpotStateWrite{
		ID: id, Region: m.region, ExpectedRevision: expectedRevision,
		State: append(json.RawMessage(nil), state...), RuntimeSeconds: runtimeSeconds,
	}}
	if !m.submit(job) {
		return ErrSpotSaveBusy
	}
	spot.pending = job
	m.pending = append(m.pending, job)
	return nil
}

func (m *SpotManager) StopAdmission()    { m.stopped = true }
func (m *SpotManager) PendingCount() int { return len(m.pending) }

// Err reports a terminal persistence conflict. Its snapshot is retained, and
// only that spot rejects further changes; other spots may continue saving.
func (m *SpotManager) Err() error { return m.err }
