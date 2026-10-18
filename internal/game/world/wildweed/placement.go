package wildweed

import "math"

const (
	ObjectKey = "wild_windsown_weed"
	MinGap    = 20
	density   = 0.0004
)

type Spot struct {
	ID                       int64
	DistrictX, DistrictY     int
	CenterX, CenterY, Radius int
	PeakQuality              int16
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func Target(radius, coordPerTile int) int {
	if radius <= 0 || coordPerTile <= 0 {
		return 0
	}
	radiusTiles := float64(radius) / float64(coordPerTile)
	return int(math.Floor(density * math.Pi * radiusTiles * radiusTiles))
}

func Quality(spot Spot, point Position) (int16, bool) {
	if spot.Radius <= 0 {
		return 0, false
	}
	dx, dy := int64(point.X)-int64(spot.CenterX), int64(point.Y)-int64(spot.CenterY)
	radius := int64(spot.Radius)
	if dx < -radius || dx > radius || dy < -radius || dy > radius || dx*dx+dy*dy > radius*radius {
		return 0, false
	}
	distance := math.Hypot(float64(dx), float64(dy))
	return int16(10 + math.Floor(float64(spot.PeakQuality-10)*(1-distance/float64(spot.Radius)))), true
}

// Prefer resolves overlapping WWW circles by quality, then district Y and X.
// A spot that does not contain the point cannot own it.
func Prefer(candidate, other Spot, point Position) bool {
	quality, inside := Quality(candidate, point)
	if !inside {
		return false
	}
	otherQuality, otherInside := Quality(other, point)
	return !otherInside || quality > otherQuality || quality == otherQuality &&
		(candidate.DistrictY < other.DistrictY || candidate.DistrictY == other.DistrictY && candidate.DistrictX < other.DistrictX)
}

func AllowedTile(tile byte) bool {
	switch tile {
	case 20, 25, 35, 50:
		return true
	default:
		return false
	}
}

func Separated(point, other Position) bool {
	dx, dy := int64(point.X)-int64(other.X), int64(point.Y)-int64(other.Y)
	if dx <= -MinGap || dx >= MinGap || dy <= -MinGap || dy >= MinGap {
		return true
	}
	return dx*dx+dy*dy >= MinGap*MinGap
}
