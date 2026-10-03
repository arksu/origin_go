package combat

import (
	"fmt"
	"math"
)

// GeometryTolerance is a linear contact tolerance in world units, never a sort tolerance.
const GeometryTolerance = 1e-7

type Point struct{ X, Y float64 }
type AABB struct{ Min, Max Point }
type Sector struct {
	Origin       Point
	Direction    Point
	Range        float64
	AngleDegrees float64
}

func (point Point) valid() bool                { return finite(point.X) && finite(point.Y) }
func (point Point) dot(other Point) float64    { return point.X*other.X + point.Y*other.Y }
func (point Point) sub(other Point) Point      { return Point{point.X - other.X, point.Y - other.Y} }
func (point Point) add(other Point) Point      { return Point{point.X + other.X, point.Y + other.Y} }
func (point Point) scale(amount float64) Point { return Point{point.X * amount, point.Y * amount} }
func (point Point) length() float64            { return math.Hypot(point.X, point.Y) }

func Direction(from, to Point) (Point, error) {
	if !from.valid() || !to.valid() {
		return Point{}, fmt.Errorf("direction endpoints must be finite")
	}
	delta := to.sub(from)
	length := delta.length()
	if !finite(length) || length == 0 {
		return Point{}, fmt.Errorf("attack direction must be finite and nonzero")
	}
	return delta.scale(1 / length), nil
}

func (sector Sector) Validate() error {
	if !sector.Origin.valid() || !sector.Direction.valid() || !finite(sector.Range) || sector.Range <= 0 || !finite(sector.AngleDegrees) || sector.AngleDegrees <= 0 || sector.AngleDegrees > 180 {
		return fmt.Errorf("invalid finite convex sector")
	}
	if math.Abs(sector.Direction.length()-1) > 1e-12 {
		return fmt.Errorf("sector direction must be normalized")
	}
	return nil
}

func (bounds AABB) Validate() error {
	if !bounds.Min.valid() || !bounds.Max.valid() || bounds.Min.X > bounds.Max.X || bounds.Min.Y > bounds.Max.Y {
		return fmt.Errorf("invalid collider bounds")
	}
	return nil
}

// ContactDistance clips the rectangle by the cone before testing the circular range.
// This gives the nearest intersecting point, even when the closest rectangle point
// is outside the cone or the rectangle center is outside the finite sector.
func ContactDistance(sector Sector, bounds AABB) (float64, bool, error) {
	if err := sector.Validate(); err != nil {
		return 0, false, err
	}
	if err := bounds.Validate(); err != nil {
		return 0, false, err
	}
	min, max := bounds.Min.sub(sector.Origin), bounds.Max.sub(sector.Origin)
	if !min.valid() || !max.valid() {
		return 0, false, fmt.Errorf("collider relative position overflow")
	}
	polygon := []Point{min, {max.X, min.Y}, max, {min.X, max.Y}}
	halfAngle := sector.AngleDegrees * math.Pi / 360
	sin, cos := math.Sincos(halfAngle)
	direction := sector.Direction
	normals := [2]Point{
		{direction.X*sin - direction.Y*cos, direction.Y*sin + direction.X*cos},
		{direction.X*sin + direction.Y*cos, direction.Y*sin - direction.X*cos},
	}
	for _, normal := range normals {
		polygon = clipPolygon(polygon, normal)
	}
	if len(polygon) == 0 {
		return 0, false, nil
	}
	distance := math.Inf(1)
	if min.X <= 0 && max.X >= 0 && min.Y <= 0 && max.Y >= 0 {
		distance = 0
	} else {
		for index, point := range polygon {
			next := polygon[(index+1)%len(polygon)]
			edge := next.sub(point)
			lengthSquared := edge.dot(edge)
			fraction := 0.0
			if lengthSquared > 0 {
				fraction = math.Max(0, math.Min(1, -point.dot(edge)/lengthSquared))
			}
			distance = math.Min(distance, point.add(edge.scale(fraction)).length())
		}
	}
	if !finite(distance) {
		return 0, false, fmt.Errorf("contact distance overflow")
	}
	return distance, distance <= sector.Range+GeometryTolerance, nil
}

func clipPolygon(polygon []Point, normal Point) []Point {
	if len(polygon) == 0 {
		return polygon
	}
	result := make([]Point, 0, len(polygon)+1)
	previous := polygon[len(polygon)-1]
	previousDistance := previous.dot(normal)
	for _, current := range polygon {
		currentDistance := current.dot(normal)
		previousInside, currentInside := previousDistance >= -GeometryTolerance, currentDistance >= -GeometryTolerance
		if previousInside != currentInside {
			// Intersect the tolerance boundary only when it separates the endpoints.
			boundary := 0.0
			if previousDistance < 0 && currentDistance < 0 {
				boundary = -GeometryTolerance
			}
			fraction := (previousDistance - boundary) / (previousDistance - currentDistance)
			result = append(result, previous.add(current.sub(previous).scale(fraction)))
		}
		if currentInside {
			result = append(result, current)
		}
		previous, previousDistance = current, currentDistance
	}
	return result
}

// Bounds is conservative; exact contact remains the authority.
func (sector Sector) Bounds() AABB {
	return AABB{Point{sector.Origin.X - sector.Range, sector.Origin.Y - sector.Range}, Point{sector.Origin.X + sector.Range, sector.Origin.Y + sector.Range}}
}
