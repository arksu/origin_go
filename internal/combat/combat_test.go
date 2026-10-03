package combat

import (
	"math"
	"math/rand"
	"origin/internal/types"
	"slices"
	"testing"
)

func TestSectorContactAndDistance(t *testing.T) {
	sector := Sector{Direction: Point{1, 0}, Range: 18, AngleDegrees: 90}
	cases := []struct {
		name     string
		bounds   AABB
		hit      bool
		distance float64
	}{
		{"range tangent", AABB{Point{18, -1}, Point{20, 1}}, true, 18},
		{"range outside", AABB{Point{18 + 2e-7, -1}, Point{20, 1}}, false, 18 + 2e-7},
		{"range tolerance", AABB{Point{18 + 5e-8, 0}, Point{18 + 5e-8, 0}}, true, 18 + 5e-8},
		{"upper angular tangent", AABB{Point{5, 6}, Point{6, 7}}, true, math.Sqrt(72)},
		{"lower angular tangent", AABB{Point{5, -7}, Point{6, -6}}, true, math.Sqrt(72)},
		{"angular outside", AABB{Point{5, 6 + 1e-6}, Point{6, 7}}, false, 0},
		{"contains origin", AABB{Point{-30, -30}, Point{30, 30}}, true, 0},
		{"center outside range", AABB{Point{17, -2}, Point{50, 2}}, true, 17},
		{"center outside angle", AABB{Point{5, 7}, Point{9, 30}}, true, math.Sqrt(98)},
		{"nearest point outside cone", AABB{Point{5, 6}, Point{8, 10}}, true, math.Sqrt(72)},
		{"degenerate point", AABB{Point{3, 0}, Point{3, 0}}, true, 3},
		{"degenerate line", AABB{Point{3, -1}, Point{3, 1}}, true, 3},
		{"behind", AABB{Point{-10, -1}, Point{-5, 1}}, false, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			distance, hit, err := ContactDistance(sector, test.bounds)
			if err != nil || hit != test.hit {
				t.Fatalf("contact=%v distance=%v err=%v", hit, distance, err)
			}
			if hit && math.Abs(distance-test.distance) > 1e-6 {
				t.Fatalf("distance %v, want %v", distance, test.distance)
			}
		})
	}
	rotated := Sector{Origin: Point{100, 200}, Direction: Point{0, 1}, Range: 18, AngleDegrees: 90}
	distance, hit, err := ContactDistance(rotated, AABB{Point{99, 218}, Point{101, 220}})
	if err != nil || !hit || distance != 18 {
		t.Fatalf("translated rotation: %v %v %v", distance, hit, err)
	}
}

func TestInvalidGeometry(t *testing.T) {
	for _, sector := range []Sector{{Direction: Point{}, Range: 18, AngleDegrees: 90}, {Direction: Point{1, 0}, Range: math.NaN(), AngleDegrees: 90}, {Direction: Point{1, 0}, Range: 18, AngleDegrees: 181}} {
		if _, _, err := ContactDistance(sector, AABB{}); err == nil {
			t.Fatal("invalid sector accepted")
		}
	}
	if _, _, err := ContactDistance(Sector{Direction: Point{1, 0}, Range: 18, AngleDegrees: 90}, AABB{Point{1, 0}, Point{0, 1}}); err == nil {
		t.Fatal("invalid rectangle accepted")
	}
	if _, err := Direction(Point{}, Point{}); err == nil {
		t.Fatal("zero direction accepted")
	}
	if _, err := Direction(Point{}, Point{math.NaN(), 0}); err == nil {
		t.Fatal("invalid direction accepted")
	}
}

func TestCandidateSelectionStableAndShared(t *testing.T) {
	sector := Sector{Direction: Point{1, 0}, Range: 18, AngleDegrees: 90}
	candidates := []Candidate{
		{ID: 1, Incarnation: 1, Bounds: AABB{}},
		{ID: 2, Incarnation: 20, Bounds: AABB{Point{4, -2}, Point{6, -1}}},
		{ID: 3, Incarnation: 30, Bounds: AABB{Point{4, 1}, Point{6, 2}}},
		{ID: 2, Incarnation: 19, Bounds: AABB{}},
		{ID: 4, Incarnation: 40, Bounds: AABB{Point{1, -1}, Point{2, 1}}},
		{ID: 5, Incarnation: 50, Bounds: AABB{Point{8, -1}, Point{9, 1}}},
	}
	candidates = append(candidates, candidates[1])
	eligible := func(candidate Candidate) bool {
		return candidate.Incarnation == types.Handle(candidate.ID*10) && candidate.ID != 4
	}
	random := rand.New(rand.NewSource(42))
	for count := 0; count < 30; count++ {
		random.Shuffle(len(candidates), func(left, right int) { candidates[left], candidates[right] = candidates[right], candidates[left] })
		all, err := Select(1, sector, candidates, false, eligible)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]types.EntityID, len(all))
		for index, contact := range all {
			ids[index] = contact.ID
		}
		if !slices.Equal(ids, []types.EntityID{2, 3, 5}) {
			t.Fatalf("AoE eligibility/dedup changed: %v", ids)
		}
		nearest, err := Select(1, sector, candidates, true, eligible)
		if err != nil || len(nearest) != 1 || nearest[0].ID != 2 {
			t.Fatalf("tie changed: %+v %v", nearest, err)
		}
	}
	onlySelf, err := Select(1, sector, []Candidate{{ID: 1}}, true, func(Candidate) bool { t.Fatal("self reached eligibility"); return true })
	if err != nil || len(onlySelf) != 0 {
		t.Fatal("self was selected")
	}
}

func TestNearestIntersectionBeatsCenterAndOutsideConePoint(t *testing.T) {
	sector := Sector{Direction: Point{1, 0}, Range: 18, AngleDegrees: 90}
	candidates := []Candidate{
		{ID: 1, Bounds: AABB{Point{5, 6}, Point{8, 10}}},
		{ID: 2, Bounds: AABB{Point{8, -1}, Point{40, 1}}},
	}
	contacts, err := Select(99, sector, candidates, true, func(Candidate) bool { return true })
	if err != nil || len(contacts) != 1 || contacts[0].ID != 2 {
		t.Fatalf("nearest must use clipped geometry: %+v %v", contacts, err)
	}
}

func TestDamageFormula(t *testing.T) {
	for _, test := range []struct{ strength, quality, multiplier, armor, want float64 }{
		{1, 10, 1, 0, 6}, {1, 10, 1.5, 0, 9}, {16, 10, 1, 0, 12}, {1, 160, 1, 0, 12},
		{1, 10, 1, 4, 3.6}, {1, 10, 1.5, 4, 81.0 / 13}, {1, 10, 0, 0, 0},
		{1, 10, 0.01, 12, 0.06 * 0.06 / 12.06},
	} {
		raw, err := RawDamage(Weapon{BaseDamage: 6, Range: 18, Quality: test.quality}, test.strength, test.multiplier)
		if err != nil {
			t.Fatal(err)
		}
		damage, err := ArmorDamage(raw, test.armor)
		if err != nil || math.Abs(damage-test.want) > 1e-12 {
			t.Fatalf("damage %v != %v: %v", damage, test.want, err)
		}
	}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := ArmorDamage(invalid, 0); err == nil {
			t.Fatal("invalid raw accepted")
		}
		if _, err := ArmorDamage(1, invalid); err == nil {
			t.Fatal("invalid armor accepted")
		}
		if _, err := RawDamage(Weapon{BaseDamage: 6, Range: 18, Quality: 10}, invalid, 1); err == nil {
			t.Fatal("invalid strength accepted")
		}
	}
	damage, err := ArmorDamage(math.MaxFloat64, math.MaxFloat64)
	if err != nil || damage != math.MaxFloat64/2 {
		t.Fatal("avoidable intermediate overflow")
	}
}
