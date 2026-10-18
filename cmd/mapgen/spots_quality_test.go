package main

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateSpotsUsesEachTypesQualityRange(t *testing.T) {
	terrain := &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass}}
	for _, fixed := range []bool{false, true} {
		name := "ranges"
		if fixed {
			name = "fixed"
		}
		t.Run(name, func(t *testing.T) {
			config := spotsChanceTestConfig(1)
			for index := range config.Spots {
				config.Spots[index].PeakQualityMin = int64(20 + index*20)
				config.Spots[index].PeakQualityMax = config.Spots[index].PeakQualityMin + 1
				if fixed {
					config.Spots[index].PeakQualityMax = config.Spots[index].PeakQualityMin
				}
			}
			seen := make(map[string]map[int16]bool, spotTypeCount)
			for _, definition := range config.Spots {
				seen[definition.Type] = make(map[int16]bool)
			}
			for seed := int64(1); seed <= 64; seed++ {
				spots, _ := collectGeneratedSpots(t, terrain, config, seed)
				if len(spots) != spotTypeCount {
					t.Fatalf("expected every type, got %v", spots)
				}
				for _, spot := range spots {
					definition := config.Spots[spotTypeIndex(spot.SpotType)]
					if int64(spot.PeakQuality) < definition.PeakQualityMin || int64(spot.PeakQuality) > definition.PeakQualityMax {
						t.Fatalf("%s quality %d outside [%d,%d]", spot.SpotType, spot.PeakQuality, definition.PeakQualityMin, definition.PeakQualityMax)
					}
					seen[spot.SpotType][spot.PeakQuality] = true
				}
			}
			for _, definition := range config.Spots {
				if !seen[definition.Type][int16(definition.PeakQualityMin)] || !seen[definition.Type][int16(definition.PeakQualityMax)] {
					t.Fatalf("%s did not produce both inclusive endpoints: %v", definition.Type, seen[definition.Type])
				}
			}
		})
	}
}

func TestGenerateSpotsQualityChangesOnlySelectedType(t *testing.T) {
	terrain := spotsChanceTestTerrain(false, false)
	config := spotsChanceTestConfig(0.5)
	const seed = int64(12345)
	baseline, baselineStats := collectGeneratedSpots(t, terrain, config, seed)
	config.Spots[0].PeakQualityMin, config.Spots[0].PeakQualityMax = 211, 214
	changed, changedStats := collectGeneratedSpots(t, terrain, config, seed)
	if !reflect.DeepEqual(baselineStats, changedStats) || len(baseline) != len(changed) {
		t.Fatalf("quality changed spawn decisions: before %+v, after %+v", baselineStats, changedStats)
	}
	changedQuality := false
	for index, before := range baseline {
		after := changed[index]
		if before.SpotType == "www" {
			if after.PeakQuality < 211 || after.PeakQuality > 214 {
				t.Fatalf("WWW did not use its changed range: %+v", after)
			}
			changedQuality = changedQuality || before.PeakQuality != after.PeakQuality
			after.PeakQuality = before.PeakQuality
		}
		if before != after {
			t.Fatalf("quality changed another type or geometry: before %+v, after %+v", before, after)
		}
	}
	if !changedQuality {
		t.Fatal("fixture did not exercise a WWW quality change")
	}
	// The lookup must follow the type key, rather than the declaration's index.
	for left, right := 0, len(config.Spots)-1; left < right; left, right = left+1, right-1 {
		config.Spots[left], config.Spots[right] = config.Spots[right], config.Spots[left]
	}
	reordered, reorderedStats := collectGeneratedSpots(t, terrain, config, seed)
	if !reflect.DeepEqual(changed, reordered) || !reflect.DeepEqual(changedStats, reorderedStats) {
		t.Fatal("type declaration order changed qualities, geometry, or spawn decisions")
	}
}

func TestDecodeSpotsConfigRequiresValidPerTypeQualityRanges(t *testing.T) {
	for _, test := range []struct {
		name, field, value string
		wantTypeInError    bool
	}{
		{"missing minimum", "peak_quality_min", "", true},
		{"missing maximum", "peak_quality_max", "", true},
		{"zero minimum", "peak_quality_min", "0", true},
		{"zero maximum", "peak_quality_max", "0", true},
		{"minimum below base", "peak_quality_min", "9", true},
		{"reversed range", "peak_quality_min", "51", true},
		{"smallint overflow", "peak_quality_max", "32768", true},
		{"fractional minimum", "peak_quality_min", "20.5", false},
		{"fractional maximum", "peak_quality_max", "50.5", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := yaml.Marshal(validSpotsConfig())
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(content), "\n")
			found := false
			for index, line := range lines {
				if !strings.HasPrefix(strings.TrimSpace(line), test.field+":") {
					continue
				}
				if test.value == "" {
					lines = append(lines[:index], lines[index+1:]...)
				} else {
					indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
					lines[index] = indent + test.field + ": " + test.value
				}
				found = true
				break
			}
			if !found {
				t.Fatalf("fixture did not contain %s", test.field)
			}
			_, err = decodeSpotsConfig([]byte(strings.Join(lines, "\n")))
			if err == nil {
				t.Fatal("accepted invalid per-type quality range")
			}
			if test.wantTypeInError && !strings.Contains(err.Error(), `"www"`) {
				t.Fatalf("quality validation must identify its type: %v", err)
			}
		})
	}
}

func TestSpotsConfigPerTypeQualityAcceptsInclusiveLimits(t *testing.T) {
	config := spotsChanceTestConfig(1)
	for index := range config.Spots {
		quality := int64(math.MaxInt16)
		if index == 0 {
			quality = config.BaseQuality
		}
		config.Spots[index].PeakQualityMin, config.Spots[index].PeakQualityMax = quality, quality
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("inclusive quality limits: %v", err)
	}
	spots, _ := collectGeneratedSpots(t, &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass}}, config, 1)
	for _, spot := range spots {
		if want := int16(config.Spots[spotTypeIndex(spot.SpotType)].PeakQualityMin); spot.PeakQuality != want {
			t.Fatalf("%s quality = %d, want %d", spot.SpotType, spot.PeakQuality, want)
		}
	}
}
