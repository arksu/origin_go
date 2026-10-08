package game

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"origin/internal/actiondefs"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const sectorContactEpsilon = 1e-9

const (
	MeleeMaxQueryCells  = 1024
	MeleeMaxQueryVisits = 4096
	MeleeMaxContacts    = 4096
)

var (
	ErrInvalidMeleeSector   = errors.New("melee sector: invalid parameters")
	ErrInvalidMeleeAttacker = errors.New("melee sector: invalid attacker identity or position")
	ErrInvalidMeleeTarget   = errors.New("melee sector: invalid target identity or bounds")
	ErrMeleeContactCapacity = errors.New("melee sector: contact capacity exceeded")
)

type SectorHit struct {
	Handle   types.Handle
	EntityID types.EntityID
	Distance float64
}

// SectorResolver returns geometry only. Damage eligibility must be checked
// before a handler selects the nearest hit or applies damage to all hits.
type SectorResolver struct {
	world             *ecs.World
	index             *core.ColliderSpatialIndex
	transforms        *ecs.ComponentStorage[components.Transform]
	colliders         *ecs.ComponentStorage[components.Collider]
	identities        *ecs.ComponentStorage[ecs.ExternalID]
	candidates        []types.Handle
	boundedCandidates [MeleeMaxQueryVisits]types.Handle
}

func NewSectorResolver(world *ecs.World) (*SectorResolver, error) {
	if world == nil {
		return nil, fmt.Errorf("sector resolver requires a world")
	}
	spatial, exists := ecs.TryGetResource[*core.WorldColliderSpatial](world)
	if !exists || *spatial == nil {
		return nil, fmt.Errorf("sector resolver requires an attached collider index")
	}
	return &SectorResolver{
		world: world, index: (*spatial).Index,
		transforms: ecs.GetOrCreateStorage[components.Transform](world),
		colliders:  ecs.GetOrCreateStorage[components.Collider](world),
		identities: ecs.GetOrCreateStorage[ecs.ExternalID](world),
	}, nil
}

func (resolver *SectorResolver) ResolveInto(attacker types.Handle, aimAngle float64, sector actiondefs.Sector, destination []SectorHit) ([]SectorHit, error) {
	destination = destination[:0]
	if !validActionAim(aimAngle) || math.IsNaN(sector.Range) || math.IsInf(sector.Range, 0) || sector.Range <= 0 ||
		math.IsNaN(sector.Angle) || math.IsInf(sector.Angle, 0) || sector.Angle <= 0 || sector.Angle > 2*math.Pi {
		return destination, fmt.Errorf("invalid sector parameters")
	}
	origin, exists := resolver.transforms.Get(attacker)
	if !exists || !resolver.world.Alive(attacker) {
		return destination, fmt.Errorf("sector attacker does not exist")
	}
	radius := sector.Range + sectorContactEpsilon
	query := core.ColliderBounds{MinX: origin.X - radius, MinY: origin.Y - radius, MaxX: origin.X + radius, MaxY: origin.Y + radius}
	var err error
	resolver.candidates, err = resolver.index.QueryInto(query, resolver.candidates[:0])
	if err != nil {
		return destination, err
	}
	geometry := makeSectorGeometry(aimAngle, sector.Angle)
	for _, handle := range resolver.candidates {
		if handle == attacker || !resolver.world.Alive(handle) {
			continue
		}
		position, hasPosition := resolver.transforms.Get(handle)
		collider, hasCollider := resolver.colliders.Get(handle)
		identity, hasIdentity := resolver.identities.Get(handle)
		if !hasPosition || !hasCollider || !hasIdentity {
			continue
		}
		distanceSquared := geometry.distanceSquared(position.X-origin.X, position.Y-origin.Y, collider.HalfWidth, collider.HalfHeight)
		if distanceSquared <= radius*radius {
			destination = append(destination, SectorHit{Handle: handle, EntityID: identity.ID, Distance: math.Sqrt(distanceSquared)})
		}
	}
	sortSectorHits(destination)
	return destination, nil
}

// ResolveBoundedInto performs the combat query without growing scratch or contact
// buffers. Limits apply before deduplication and geometry; overflow fails the
// entire query rather than returning a truncated sweep or a misleading nearest.
func (resolver *SectorResolver) ResolveBoundedInto(attacker types.Handle, aimAngle float64, sector actiondefs.Sector, destination []SectorHit) ([]SectorHit, error) {
	destination = destination[:0]
	if resolver == nil || resolver.world == nil || resolver.index == nil || resolver.transforms == nil || resolver.colliders == nil || resolver.identities == nil ||
		!validActionAim(aimAngle) || math.IsNaN(sector.Range) || math.IsInf(sector.Range, 0) || sector.Range <= 0 ||
		math.IsNaN(sector.Angle) || math.IsInf(sector.Angle, 0) || sector.Angle <= 0 || sector.Angle > 2*math.Pi {
		return destination, ErrInvalidMeleeSector
	}
	if !resolver.world.Alive(attacker) {
		return destination, ErrInvalidMeleeAttacker
	}
	origin, hasOrigin := resolver.transforms.Get(attacker)
	identity, hasIdentity := resolver.identities.Get(attacker)
	if !hasOrigin || !hasIdentity || identity.ID == 0 || resolver.world.GetHandleByEntityID(identity.ID) != attacker ||
		math.IsNaN(origin.X) || math.IsInf(origin.X, 0) || math.IsNaN(origin.Y) || math.IsInf(origin.Y, 0) {
		return destination, ErrInvalidMeleeAttacker
	}
	radius := sector.Range + sectorContactEpsilon
	query := core.ColliderBounds{MinX: origin.X - radius, MinY: origin.Y - radius, MaxX: origin.X + radius, MaxY: origin.Y + radius}
	candidates, err := resolver.index.QueryBoundedInto(query, resolver.boundedCandidates[:0], core.ColliderQueryBudget{
		MaxCells: MeleeMaxQueryCells, MaxVisits: MeleeMaxQueryVisits,
	})
	if err != nil {
		return destination, err
	}
	geometry := makeSectorGeometry(aimAngle, sector.Angle)
	for _, handle := range candidates {
		if handle == attacker || !resolver.world.Alive(handle) {
			continue
		}
		targetID, hasID := resolver.identities.Get(handle)
		// The handle and persistence identity both exclude the attacker, before
		// geometry or target selection. Malformed duplicate identities cannot hit it.
		if hasID && targetID.ID == identity.ID {
			continue
		}
		position, hasPosition := resolver.transforms.Get(handle)
		collider, hasCollider := resolver.colliders.Get(handle)
		if !hasPosition || !hasCollider || math.IsNaN(position.X) || math.IsInf(position.X, 0) || math.IsNaN(position.Y) || math.IsInf(position.Y, 0) ||
			math.IsNaN(collider.HalfWidth) || math.IsInf(collider.HalfWidth, 0) || collider.HalfWidth <= 0 ||
			math.IsNaN(collider.HalfHeight) || math.IsInf(collider.HalfHeight, 0) || collider.HalfHeight <= 0 {
			return destination[:0], ErrInvalidMeleeTarget
		}
		distanceSquared := geometry.distanceSquared(position.X-origin.X, position.Y-origin.Y, collider.HalfWidth, collider.HalfHeight)
		if distanceSquared <= radius*radius {
			if !hasID || targetID.ID == 0 || resolver.world.GetHandleByEntityID(targetID.ID) != handle {
				return destination[:0], ErrInvalidMeleeTarget
			}
			if len(destination) == cap(destination) || len(destination) == MeleeMaxContacts {
				return destination[:0], ErrMeleeContactCapacity
			}
			destination = append(destination, SectorHit{Handle: handle, EntityID: targetID.ID, Distance: math.Sqrt(distanceSquared)})
		}
	}
	sortSectorHits(destination)
	return destination, nil
}

func sortSectorHits(hits []SectorHit) {
	slices.SortFunc(hits, func(first, second SectorHit) int {
		if first.Distance < second.Distance {
			return -1
		}
		if first.Distance > second.Distance {
			return 1
		}
		if first.EntityID < second.EntityID {
			return -1
		}
		if first.EntityID > second.EntityID {
			return 1
		}
		return 0
	})
}

type sectorPoint struct{ X, Y float64 }
type sectorWedge struct{ lower, upper sectorPoint }
type sectorGeometry struct {
	wedges [2]sectorWedge
	count  int
}

func makeSectorGeometry(aim, angle float64) sectorGeometry {
	if angle == 2*math.Pi {
		return sectorGeometry{}
	}
	if angle <= math.Pi {
		return sectorGeometry{wedges: [2]sectorWedge{makeSectorWedge(aim, angle)}, count: 1}
	}
	return sectorGeometry{wedges: [2]sectorWedge{
		makeSectorWedge(aim-angle/4, angle/2), makeSectorWedge(aim+angle/4, angle/2),
	}, count: 2}
}

func makeSectorWedge(aim, angle float64) sectorWedge {
	lowerSin, lowerCos := math.Sincos(aim - angle/2)
	upperSin, upperCos := math.Sincos(aim + angle/2)
	return sectorWedge{lower: sectorPoint{-lowerSin, lowerCos}, upper: sectorPoint{upperSin, -upperCos}}
}

func (geometry sectorGeometry) distanceSquared(centerX, centerY, halfWidth, halfHeight float64) float64 {
	minimumX, maximumX := centerX-halfWidth, centerX+halfWidth
	minimumY, maximumY := centerY-halfHeight, centerY+halfHeight
	if minimumX <= 0 && maximumX >= 0 && minimumY <= 0 && maximumY >= 0 {
		return 0
	}
	if geometry.count == 0 {
		nearestX := math.Max(minimumX, math.Min(0, maximumX))
		nearestY := math.Max(minimumY, math.Min(0, maximumY))
		return nearestX*nearestX + nearestY*nearestY
	}
	minimumDistance := math.Inf(1)
	for _, wedge := range geometry.wedges[:geometry.count] {
		polygon := [8]sectorPoint{{minimumX, minimumY}, {maximumX, minimumY}, {maximumX, maximumY}, {minimumX, maximumY}}
		var clipped [8]sectorPoint
		count := clipSectorPolygon(&polygon, 4, wedge.lower, &clipped)
		count = clipSectorPolygon(&clipped, count, wedge.upper, &polygon)
		for point := 0; point < count; point++ {
			minimumDistance = math.Min(minimumDistance, segmentDistanceSquared(polygon[point], polygon[(point+1)%count]))
		}
	}
	return minimumDistance
}

func clipSectorPolygon(source *[8]sectorPoint, count int, normal sectorPoint, destination *[8]sectorPoint) int {
	if count == 0 {
		return 0
	}
	result := 0
	previous := source[count-1]
	previousDistance := previous.X*normal.X + previous.Y*normal.Y + sectorContactEpsilon
	for _, current := range source[:count] {
		currentDistance := current.X*normal.X + current.Y*normal.Y + sectorContactEpsilon
		if (previousDistance >= 0) != (currentDistance >= 0) {
			fraction := previousDistance / (previousDistance - currentDistance)
			destination[result] = sectorPoint{previous.X + fraction*(current.X-previous.X), previous.Y + fraction*(current.Y-previous.Y)}
			result++
		}
		if currentDistance >= 0 {
			destination[result] = current
			result++
		}
		previous, previousDistance = current, currentDistance
	}
	return result
}

func segmentDistanceSquared(start, end sectorPoint) float64 {
	deltaX, deltaY := end.X-start.X, end.Y-start.Y
	lengthSquared := deltaX*deltaX + deltaY*deltaY
	fraction := 0.0
	if lengthSquared > 0 {
		fraction = math.Max(0, math.Min(1, -(start.X*deltaX+start.Y*deltaY)/lengthSquared))
	}
	nearestX, nearestY := start.X+fraction*deltaX, start.Y+fraction*deltaY
	return nearestX*nearestX + nearestY*nearestY
}
