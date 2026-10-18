package world

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/world/wildweed"
	"origin/internal/mathutil"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

const (
	wildWeedCheckSeconds    int64 = 15 * 60
	wildWeedTrialChance           = 1.0 / 120
	wildWeedSpotBudget            = 32
	wildWeedCandidateBudget       = 64
	wildWeedObjectBudget          = 4
)

type wildWeedIDAllocator interface {
	ReserveIDs(uint64) (types.EntityID, types.EntityID, error)
}

type wildWeedRefillRuntime struct {
	chunks  *ChunkManager
	ids     wildWeedIDAllocator
	random  *rand.Rand
	handles []types.Handle
}

type wildWeedManifest struct {
	count     int
	lastCheck *int64
	valid     bool
}

// Only membership counts and the check cursor are cached, keyed by the committed
// revision. Opaque JSON (including exact integer identities) stays authoritative.
func (s *managedSpot) wildWeedManifest() wildWeedManifest {
	if s.wwwRevision != s.state.Revision {
		s.wwwRevision = s.state.Revision
		s.wwwState = wildWeedManifest{}
		var envelope struct {
			WWW *struct {
				Objects   map[string]json.RawMessage `json:"objects"`
				LastCheck *int64                     `json:"last_check_runtime_seconds"`
			} `json:"www"`
		}
		if json.Unmarshal(s.state.State, &envelope) == nil && envelope.WWW != nil && envelope.WWW.Objects != nil &&
			(envelope.WWW.LastCheck == nil || *envelope.WWW.LastCheck >= 0) {
			s.wwwState = wildWeedManifest{len(envelope.WWW.Objects), envelope.WWW.LastCheck, true}
		}
	}
	return s.wwwState
}

// EnableWildWeedRefill connects existing shard-owned services once at startup.
// No chunk lifecycle hooks or additional workers are installed.
func (m *SpotManager) EnableWildWeedRefill(chunks *ChunkManager, ids wildWeedIDAllocator) error {
	if chunks == nil || ids == nil || chunks.region != m.region || chunks.layer != 0 {
		return errors.New("invalid WWW refill services")
	}
	if _, ok := m.store.(WildWeedRefillStore); !ok {
		return errors.New("spot store does not support WWW refill")
	}
	if objectdefs.Global() == nil {
		return errors.New("WWW definition registry is missing")
	}
	def, ok := objectdefs.Global().GetByKey(wildweed.ObjectKey)
	if !ok || def.HP <= 0 {
		return errors.New("WWW definition is missing or has invalid HP")
	}
	m.www = &wildWeedRefillRuntime{chunks: chunks, ids: ids, random: rand.New(rand.NewSource(time.Now().UnixNano()))}
	return nil
}

// Geometric waits skip failed trials. Work is proportional to births, capped by
// the deficit, rather than to the number of elapsed quarter-hours.
func wildWeedSuccesses(random *rand.Rand, periods int64, deficit int) int {
	count := 0
	for periods > 0 && count < deficit {
		failures := math.Floor(math.Log1p(-random.Float64()) / math.Log1p(-wildWeedTrialChance))
		if failures >= float64(periods) {
			break
		}
		periods -= int64(failures) + 1
		count++
	}
	return count
}

type wildWeedTileArea struct{ minX, minY, width, height int }

type wildWeedRefillOperation struct {
	write     WildWeedRefillWrite
	remaining int
	tries     int
	random    *rand.Rand
	areas     []wildWeedTileArea
	tiles     int
	pins      []types.ChunkCoord
	prepared  bool
	commit    SpotStateCommit // Written by worker, read only after Complete's acknowledgement.
	committed bool
	published int
}

type wildWeedPrepareBudget struct{ candidates, objects, publications int }

func newWildWeedPrepareBudget() wildWeedPrepareBudget {
	return wildWeedPrepareBudget{wildWeedCandidateBudget, wildWeedObjectBudget, wildWeedObjectBudget}
}

func (m *SpotManager) admitWildWeedChecks() {
	if m.stopped || m.www == nil || len(m.wwwSpots) == 0 {
		return
	}
	ts := ecs.GetResource[ecs.TimeState](m.www.chunks.world)
	if ts == nil || ts.RuntimeSecondsTotal < 0 {
		return
	}
	runtime := ts.RuntimeSecondsTotal
	for range min(len(m.wwwSpots), wildWeedSpotBudget) {
		spot := m.wwwSpots[m.nextWWW]
		m.nextWWW = (m.nextWWW + 1) % len(m.wwwSpots)
		if spot.pending != nil || spot.saveErr != nil {
			continue
		}
		manifest := spot.wildWeedManifest()
		if !manifest.valid {
			continue
		} // Legacy worlds without mapgen manifests stay untouched.
		target := wildweed.Target(spot.info.Radius, constt.CoordPerTile)
		deficit := max(0, target-manifest.count)
		if deficit == 0 && manifest.lastCheck == nil {
			continue
		}
		check := runtime
		var periods int64
		if manifest.lastCheck != nil && deficit > 0 {
			periods = max(int64(0), runtime-*manifest.lastCheck) / wildWeedCheckSeconds
			if periods == 0 {
				continue
			}
			check = *manifest.lastCheck + periods*wildWeedCheckSeconds
		}
		areas, tiles := m.activeWildWeedAreas(spot.info)
		if tiles == 0 {
			continue
		}
		op := &wildWeedRefillOperation{
			write: WildWeedRefillWrite{SpotID: spot.info.ID, Region: m.region, RuntimeSeconds: runtime, CheckRuntimeSeconds: check, Target: target},
			areas: areas, tiles: tiles,
		}
		// Retain ready source chunks before the draw. Pins prevent a reload;
		// subsequent raw appends are committed publications, not unfinished
		// restoration. I/O backpressure must neither discard nor reroll births.
		if periods > 0 && deficit > 0 && !m.pinWildWeedSourceAreas(op) {
			continue
		}
		op.random = rand.New(rand.NewSource(m.www.random.Int63()))
		op.remaining = wildWeedSuccesses(op.random, periods, deficit)
		if op.remaining == 0 {
			for _, pin := range op.pins {
				m.www.chunks.UnpinPersistence(pin)
			}
			op.pins = nil
		}
		job := &spotStateJob{store: m.store, spot: spot, refill: op, chunks: m.www.chunks, write: SpotStateWrite{ID: spot.info.ID}}
		spot.pending = job
		m.pending = append(m.pending, job)
	}
}

func (m *SpotManager) wildWeedChunkActive(coord types.ChunkCoord) bool {
	cm := m.www.chunks
	chunk := cm.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return false
	}
	cm.interestMu.RLock()
	interest := cm.chunkInterests[coord]
	active := interest != nil && interest.hasActive()
	cm.interestMu.RUnlock()
	return active
}

func (m *SpotManager) wildWeedChunkReady(coord types.ChunkCoord) bool {
	return m.wildWeedChunkActive(coord) && len(m.www.chunks.GetChunkFast(coord).GetRawObjects()) == 0
}

func (m *SpotManager) pinWildWeedSourceAreas(op *wildWeedRefillOperation) bool {
	retained := op.areas[:0]
	tiles := 0
	for _, area := range op.areas {
		coord := types.ChunkCoord{X: mathutil.FloorDiv(area.minX, constt.ChunkSize), Y: mathutil.FloorDiv(area.minY, constt.ChunkSize)}
		if !m.wildWeedChunkReady(coord) || m.www.chunks.PinPersistence(coord) != nil {
			continue
		}
		retained = append(retained, area)
		tiles += area.width * area.height
		op.pins = append(op.pins, coord)
	}
	op.areas, op.tiles = retained, tiles
	return tiles > 0
}

func (m *SpotManager) activeWildWeedAreas(spot SpotInfo) ([]wildWeedTileArea, int) {
	minX := max(m.minChunk.X*constt.ChunkSize, mathutil.FloorDiv(spot.CenterX-spot.Radius, constt.CoordPerTile))
	maxX := min((m.maxChunk.X+1)*constt.ChunkSize-1, mathutil.FloorDiv(spot.CenterX+spot.Radius, constt.CoordPerTile))
	minY := max(m.minChunk.Y*constt.ChunkSize, mathutil.FloorDiv(spot.CenterY-spot.Radius, constt.CoordPerTile))
	maxY := min((m.maxChunk.Y+1)*constt.ChunkSize-1, mathutil.FloorDiv(spot.CenterY+spot.Radius, constt.CoordPerTile))
	var areas []wildWeedTileArea
	total := 0
	for y := mathutil.FloorDiv(minY, constt.ChunkSize); y <= mathutil.FloorDiv(maxY, constt.ChunkSize); y++ {
		for x := mathutil.FloorDiv(minX, constt.ChunkSize); x <= mathutil.FloorDiv(maxX, constt.ChunkSize); x++ {
			coord := types.ChunkCoord{X: x, Y: y}
			if !slices.Contains(m.byChunk[coord], spot.ID) || !m.wildWeedChunkReady(coord) {
				continue
			}
			left, top := max(minX, x*constt.ChunkSize), max(minY, y*constt.ChunkSize)
			width, height := min(maxX+1, (x+1)*constt.ChunkSize)-left, min(maxY+1, (y+1)*constt.ChunkSize)-top
			areas = append(areas, wildWeedTileArea{left, top, width, height})
			total += width * height
		}
	}
	return areas, total
}

func wildWeedGeometry(spot SpotInfo) wildweed.Spot {
	return wildweed.Spot{ID: spot.ID, DistrictX: spot.DistrictX, DistrictY: spot.DistrictY, CenterX: spot.CenterX, CenterY: spot.CenterY, Radius: spot.Radius, PeakQuality: spot.PeakQuality}
}

func (op *wildWeedRefillOperation) candidate() wildweed.Position {
	index := op.random.Intn(op.tiles)
	for _, area := range op.areas {
		if count := area.width * area.height; index >= count {
			index -= count
			continue
		}
		return wildweed.Position{X: (area.minX+index%area.width)*constt.CoordPerTile + constt.CoordPerTile/2, Y: (area.minY+index/area.width)*constt.CoordPerTile + constt.CoordPerTile/2}
	}
	panic("invalid WWW tile area")
}

func (m *SpotManager) prepareWildWeeds(job *spotStateJob, budget *wildWeedPrepareBudget) error {
	op := job.refill
	for op.remaining > 0 && budget.candidates > 0 && budget.objects > 0 {
		point := op.candidate()
		budget.candidates--
		op.tries++
		quality, valid := wildweed.Quality(wildWeedGeometry(job.spot.info), point)
		coord := types.ChunkCoord{X: mathutil.FloorDiv(point.X, constt.ChunkWorldSize), Y: mathutil.FloorDiv(point.Y, constt.ChunkWorldSize)}
		if valid && m.wildWeedChunkActive(coord) {
			chunk := m.www.chunks.GetChunkFast(coord)
			tile, found := chunk.TileID(mathutil.FloorDiv(point.X, constt.CoordPerTile)-coord.X*constt.ChunkSize, mathutil.FloorDiv(point.Y, constt.CoordPerTile)-coord.Y*constt.ChunkSize, constt.ChunkSize)
			valid = found && wildweed.AllowedTile(tile) && m.ownsWildWeedPoint(job.spot.info, coord, point)
		} else {
			valid = false
		}
		var pins []types.ChunkCoord
		if valid {
			pins, valid = m.pinWildWeedNeighborhood(op, point)
		}
		if valid {
			valid = m.wildWeedPositionFree(point)
		}
		if valid {
			if err := m.prepareWildWeedObject(op, point, quality); err != nil {
				for _, pin := range pins {
					m.www.chunks.UnpinPersistence(pin)
				}
				return err
			}
			op.pins = append(op.pins, pins...)
			budget.objects--
			op.remaining--
			op.tries = 0
		} else {
			for _, pin := range pins {
				m.www.chunks.UnpinPersistence(pin)
			}
			if op.tries == wildWeedCandidateBudget {
				op.remaining--
				op.tries = 0
			}
		}
	}
	op.prepared = op.remaining == 0
	return nil
}

func (m *SpotManager) ownsWildWeedPoint(spot SpotInfo, coord types.ChunkCoord, point wildweed.Position) bool {
	for _, id := range m.byChunk[coord] {
		other := m.byID[id].info
		if id != spot.ID && other.Type == "www" && wildweed.Prefer(wildWeedGeometry(other), wildWeedGeometry(spot), point) {
			return false
		}
	}
	return true
}

func (m *SpotManager) pinWildWeedNeighborhood(op *wildWeedRefillOperation, point wildweed.Position) ([]types.ChunkCoord, bool) {
	var acquired []types.ChunkCoord
	for y := mathutil.FloorDiv(point.Y-wildweed.MinGap, constt.ChunkWorldSize); y <= mathutil.FloorDiv(point.Y+wildweed.MinGap, constt.ChunkWorldSize); y++ {
		for x := mathutil.FloorDiv(point.X-wildweed.MinGap, constt.ChunkWorldSize); x <= mathutil.FloorDiv(point.X+wildweed.MinGap, constt.ChunkWorldSize); x++ {
			coord := types.ChunkCoord{X: x, Y: y}
			if !m.www.chunks.IsWithinWorldBounds(coord) {
				continue
			}
			chunk := m.www.chunks.GetChunkFast(coord)
			if chunk == nil || chunk.GetState() == types.ChunkStateUnloaded || chunk.GetState() == types.ChunkStateLoading {
				return acquired, false
			}
			retained := false
			for _, pin := range op.pins {
				if pin == coord {
					retained = true
					break
				}
			}
			if retained {
				continue
			}
			if m.www.chunks.PinPersistence(coord) != nil {
				return acquired, false
			}
			acquired = append(acquired, coord)
		}
	}
	return acquired, true
}

func (m *SpotManager) wildWeedPositionFree(point wildweed.Position) bool {
	cm := m.www.chunks
	for y := mathutil.FloorDiv(point.Y-wildweed.MinGap, constt.ChunkWorldSize); y <= mathutil.FloorDiv(point.Y+wildweed.MinGap, constt.ChunkWorldSize); y++ {
		for x := mathutil.FloorDiv(point.X-wildweed.MinGap, constt.ChunkWorldSize); x <= mathutil.FloorDiv(point.X+wildweed.MinGap, constt.ChunkWorldSize); x++ {
			coord := types.ChunkCoord{X: x, Y: y}
			if !cm.IsWithinWorldBounds(coord) {
				continue
			}
			chunk := cm.GetChunkFast(coord)
			m.www.handles = m.www.handles[:0]
			chunk.Spatial().QueryRadius(float64(point.X), float64(point.Y), wildweed.MinGap, &m.www.handles)
			for _, h := range m.www.handles {
				if !cm.world.Alive(h) {
					continue
				}
				transform, ok := ecs.GetComponent[components.Transform](cm.world, h)
				if ok {
					dx, dy := transform.X-float64(point.X), transform.Y-float64(point.Y)
					if dx*dx+dy*dy < wildweed.MinGap*wildweed.MinGap {
						return false
					}
				}
			}
			for _, raw := range chunk.GetRawObjects() {
				if raw != nil && !raw.DeletedAt.Valid && !wildweed.Separated(point, wildweed.Position{X: raw.X, Y: raw.Y}) {
					return false
				}
			}
			// The existing intersection index also locates reservations held by
			// neighboring spots, including prepared records not yet published.
			for _, id := range m.byChunk[coord] {
				job := m.byID[id].pending
				if job == nil || job.refill == nil {
					continue
				}
				for _, raw := range job.refill.write.Objects {
					if !wildweed.Separated(point, wildweed.Position{X: raw.X, Y: raw.Y}) {
						return false
					}
				}
			}
		}
	}
	return true
}

func (m *SpotManager) prepareWildWeedObject(op *wildWeedRefillOperation, point wildweed.Position, quality int16) error {
	ts := ecs.GetResource[ecs.TimeState](m.www.chunks.world)
	if ts == nil || ts.RuntimeSecondsTotal < 0 || ts.RuntimeSecondsTotal > math.MaxInt64-contracts.WildWeedGrowthSeconds {
		return fmt.Errorf("invalid WWW preparation runtime")
	}
	first, last, err := m.www.ids.ReserveIDs(1)
	if err != nil {
		return err
	}
	state := components.WildWeedBehaviorState{SpotID: op.write.SpotID, Stage: 0, NextGrowthRuntimeSeconds: ts.RuntimeSecondsTotal + contracts.WildWeedGrowthSeconds}
	data, err := json.Marshal(struct {
		Version   int                                         `json:"v"`
		Behaviors map[string]components.WildWeedBehaviorState `json:"behaviors"`
	}{1, map[string]components.WildWeedBehaviorState{"wild_weed": state}})
	if err != nil {
		return err
	}
	def, _ := objectdefs.Global().GetByKey(wildweed.ObjectKey)
	op.write.Objects = append(op.write.Objects, repository.Object{
		ID: int64(first), TypeID: def.DefID, Region: m.region, Layer: 0, X: point.X, Y: point.Y,
		ChunkX: mathutil.FloorDiv(point.X, constt.ChunkWorldSize), ChunkY: mathutil.FloorDiv(point.Y, constt.ChunkWorldSize),
		Quality: quality, Hp: sql.NullFloat64{Float64: float64(def.HP), Valid: true}, Data: pqtype.NullRawMessage{RawMessage: data, Valid: true}, CreateTick: int64(ts.Tick), LastTick: int64(ts.Tick),
	})
	op.write.MaxAllocatedID = max(op.write.MaxAllocatedID, last)
	return nil
}

func (m *SpotManager) publishWildWeeds(job *spotStateJob, budget *wildWeedPrepareBudget) (bool, error) {
	op := job.refill
	if err := m.ApplyCommittedState(op.commit); err != nil {
		return false, err
	}
	for op.published < len(op.write.Objects) && budget.publications > 0 {
		if err := m.www.chunks.ReplaceCommittedSource(&op.write.Objects[op.published]); err != nil {
			return false, err
		}
		op.published++
		budget.publications--
	}
	if op.published != len(op.write.Objects) {
		return false, nil
	}
	for _, coord := range op.pins {
		m.www.chunks.UnpinPersistence(coord)
	}
	return true, nil
}
