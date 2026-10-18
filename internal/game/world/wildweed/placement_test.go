package wildweed

import (
	"math"
	"testing"
)

func TestTargetKeepsCurrentMapgenDensity(t *testing.T) {
	for _, test := range []struct{ radius, tileSize, want int }{
		{64 * 12, 12, 5},
		{128 * 12, 12, 20},
		{0, 12, 0},
		{768, 0, 0},
	} {
		if got := Target(test.radius, test.tileSize); got != test.want {
			t.Errorf("Target(%d, %d) = %d, want %d", test.radius, test.tileSize, got, test.want)
		}
	}
}

func TestQualityAndOverlapOwnership(t *testing.T) {
	spot := Spot{ID: 1, CenterX: 1000, CenterY: 1000, Radius: 768, PeakQuality: 50, DistrictX: 1}
	for _, test := range []struct {
		point  Position
		want   int16
		inside bool
	}{
		{Position{1000, 1000}, 50, true},
		{Position{1384, 1000}, 30, true},
		{Position{1768, 1000}, 10, true},
		{Position{1769, 1000}, 0, false},
		{Position{1000, 1769}, 0, false},
	} {
		quality, inside := Quality(spot, test.point)
		if quality != test.want || inside != test.inside {
			t.Errorf("Quality(%+v) = %d, %t; want %d, %t", test.point, quality, inside, test.want, test.inside)
		}
	}
	point := Position{1000, 1000}
	other := spot
	other.ID, other.DistrictX = 9007199254740993, 0
	if !Prefer(other, spot, point) || Prefer(spot, other, point) {
		t.Fatal("equal quality must prefer earlier district X, independently of ID")
	}
	other.DistrictX, other.DistrictY = 9, -1
	if !Prefer(other, spot, point) {
		t.Fatal("district Y must precede district X")
	}
	other.PeakQuality = 49
	if !Prefer(spot, other, point) {
		t.Fatal("quality must precede district order")
	}
	other.CenterX += 2000
	if Prefer(other, spot, point) || !Prefer(spot, other, point) {
		t.Fatal("a circle that does not contain the point cannot own it")
	}
}

func TestAllowedTileWhitelist(t *testing.T) {
	for tile := 0; tile <= math.MaxUint8; tile++ {
		want := tile == 20 || tile == 25 || tile == 35 || tile == 50
		if got := AllowedTile(byte(tile)); got != want {
			t.Errorf("AllowedTile(%d) = %t, want %t", tile, got, want)
		}
	}
}

func TestSeparatedUsesCoordinateDistance(t *testing.T) {
	point := Position{20, 20}
	for _, test := range []struct {
		other Position
		want  bool
	}{
		{Position{39, 20}, false},
		{Position{40, 20}, true},
		{Position{20, 0}, true},
		{Position{35, 35}, true},
		{Position{34, 34}, false},
		{Position{-1, -1}, true},
		{Position{math.MinInt32, math.MinInt32}, true},
		{Position{math.MaxInt32, math.MaxInt32}, true},
	} {
		if got := Separated(point, test.other); got != test.want {
			t.Errorf("Separated(%+v, %+v) = %t, want %t", point, test.other, got, test.want)
		}
	}
}
