package main

import (
	"math"
	_const "origin/internal/const"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func spotChanceTestValue(value float64) *float64 { return &value }

func spotsChanceTestConfig(chance float64) SpotsConfig {
	config := spotsGeneratorTestConfig()
	for index := range config.Spots {
		config.Spots[index].CenterTiles = []int{int(tileGrass)}
		config.Spots[index].SpawnChance = spotChanceTestValue(chance)
	}
	return config
}

func spotsChanceTestYAML(t *testing.T, chance string) string {
	t.Helper()
	config := validSpotsConfig()
	for index := range config.Spots {
		config.Spots[index].SpawnChance = nil
	}
	content, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	input := string(content)
	if chance != "" {
		input = strings.Replace(input, "type: www", "type: www\n      spawn_chance: "+chance, 1)
	}
	return input
}

func TestLoadSpotsConfigSpawnChanceDefaultsAndExplicitValues(t *testing.T) {
	for _, test := range []struct {
		name, input string
		want        float64
	}{
		{name: "omitted", want: 1},
		{name: "zero", input: "0", want: 0},
		{name: "one", input: "1", want: 1},
		{name: "fractional", input: "0.25", want: 0.25},
		{name: "scientific", input: "2.5e-1", want: 0.25},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "spots.yaml")
			if err := os.WriteFile(path, []byte(spotsChanceTestYAML(t, test.input)), 0o644); err != nil {
				t.Fatal(err)
			}
			config, _, err := LoadSpotsConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, definition := range config.Spots {
				want := float64(1)
				if definition.Type == "www" {
					want = test.want
				}
				if got := definition.spawnChance(); got != want {
					t.Fatalf("%s spawn chance = %v, want %v", definition.Type, got, want)
				}
			}
			terrain := &TerrainPrecompute{WidthTiles: 1, HeightTiles: 1, Tiles: []byte{tileGrass}}
			_, stats := collectGeneratedSpots(t, terrain, config, 12345)
			if test.input == "" && stats.Generated["www"] != 1 {
				t.Fatalf("omitted chance did not preserve placement: %+v", stats)
			}
			if test.input == "0" && (stats.Generated["www"] != 0 || stats.SkippedByChance["www"] != 1) {
				t.Fatalf("explicit zero did not disable placement: %+v", stats)
			}
		})
	}
}

func TestSpotsConfigSpawnChanceRejectsInvalidProbabilities(t *testing.T) {
	for _, input := range []string{"-0.01", "1.01", ".nan", ".inf", "-.inf", "null", "true"} {
		t.Run("YAML_"+input, func(t *testing.T) {
			if _, err := decodeSpotsConfig([]byte(spotsChanceTestYAML(t, input))); err == nil {
				t.Fatalf("accepted spawn_chance: %s", input)
			}
		})
	}
	for _, test := range []struct {
		name  string
		value float64
	}{
		{name: "negative", value: -0.01},
		{name: "above one", value: 1.01},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := spotsChanceTestConfig(test.value)
			if err := config.Validate(); err == nil {
				t.Fatalf("accepted spawn chance %v", test.value)
			}
		})
	}
}

func TestSpotsConfigSpawnChanceAllowsFloatsOnlyForChance(t *testing.T) {
	valid := spotsChanceTestYAML(t, "0.25")
	if _, err := decodeSpotsConfig([]byte(valid)); err != nil {
		t.Fatalf("fractional spawn chance rejected: %v", err)
	}
	for _, test := range []struct{ name, before, after string }{
		{"version", "version: 1", "version: 1.0"},
		{"district", "district_size_tiles: 512", "district_size_tiles: 512.0"},
		{"radius", "radius_tiles: 64", "radius_tiles: 64.0"},
		{"base quality", "base_quality: 10", "base_quality: 10.0"},
		{"minimum quality", "peak_quality_min: 20", "peak_quality_min: 20.0"},
		{"maximum quality", "peak_quality_max: 50", "peak_quality_max: 50.0"},
		{"center tile", "- 35", "- 35.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Replace(valid, test.before, test.after, 1)
			if input == valid {
				t.Fatalf("fixture did not contain %q", test.before)
			}
			if _, err := decodeSpotsConfig([]byte(input)); err == nil {
				t.Fatalf("floating-point %s accepted alongside fractional chance", test.name)
			}
		})
	}
}

func TestSpotsConfigSpawnChanceYAMLAliasesPreserveIntegerValidation(t *testing.T) {
	// A chance anchor must not make the same floating scalar acceptable in an
	// integer field, even when yaml.v3 can coerce it to a valid integer value.
	valid := spotsChanceTestYAML(t, "&chance 1.0")
	if _, err := decodeSpotsConfig([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	input := strings.Replace(valid, "version: 1", "version: *chance", 1)
	// YAML aliases can refer only to an earlier anchor. Move version below spots.
	input = strings.Replace(input, "version: *chance\n", "", 1) + "version: *chance\n"
	if _, err := decodeSpotsConfig([]byte(input)); err == nil {
		t.Fatal("floating chance alias was accepted as an integer version")
	}
}

func TestGenerateSpotsSpawnChanceZeroAndOne(t *testing.T) {
	terrain := &TerrainPrecompute{WidthTiles: 2, HeightTiles: 1, Tiles: []byte{tileGrass, tileGrass}}
	for _, chance := range []float64{0, 1} {
		config := spotsChanceTestConfig(chance)
		spots, stats := collectGeneratedSpots(t, terrain, config, 12345)
		if len(spots) != int(chance)*spotTypeCount || stats.Districts != 1 {
			t.Fatalf("chance %v: wrong output %v %+v", chance, spots, stats)
		}
		for _, spotType := range spotTypes {
			if stats.Generated[spotType] != uint64(chance) || stats.SkippedByChance[spotType] != 1-uint64(chance) || stats.Skipped[spotType] != 0 {
				t.Fatalf("chance %v: wrong counts for %s: %+v", chance, spotType, stats)
			}
		}
	}
	defaultConfig := spotsChanceTestConfig(1)
	for index := range defaultConfig.Spots {
		defaultConfig.Spots[index].SpawnChance = nil
	}
	defaultSpots, defaultStats := collectGeneratedSpots(t, terrain, defaultConfig, 12345)
	oneSpots, oneStats := collectGeneratedSpots(t, terrain, spotsChanceTestConfig(1), 12345)
	if !reflect.DeepEqual(defaultSpots, oneSpots) || !reflect.DeepEqual(defaultStats, oneStats) {
		t.Fatal("omitted and explicit chance 1 produced different spots")
	}
	// Terrain eligibility retains its own skip reason at both chance boundaries.
	terrain.Tiles = []byte{tileWaterDeep, tileWater}
	for _, chance := range []float64{0, 1} {
		spots, stats := collectGeneratedSpots(t, terrain, spotsChanceTestConfig(chance), 12345)
		if len(spots) != 0 {
			t.Fatalf("chance %v emitted ineligible spots: %v", chance, spots)
		}
		for _, spotType := range spotTypes {
			if stats.Skipped[spotType] != 1 || stats.SkippedByChance[spotType] != 0 {
				t.Fatalf("chance %v: wrong no-eligible skip reason: %+v", chance, stats)
			}
		}
	}
}

type spotChanceTestKey struct {
	typeName string
	x, y     int
}

func spotsChanceTestByKey(spots []GeneratedSpot) map[spotChanceTestKey]GeneratedSpot {
	result := make(map[spotChanceTestKey]GeneratedSpot, len(spots))
	for _, spot := range spots {
		result[spotChanceTestKey{spot.SpotType, spot.DistrictX, spot.DistrictY}] = spot
	}
	return result
}

func spotsChanceTestPresence(spots []GeneratedSpot) map[spotChanceTestKey]bool {
	result := make(map[spotChanceTestKey]bool, len(spots))
	for key := range spotsChanceTestByKey(spots) {
		result[key] = true
	}
	return result
}

func spotsChanceTestTerrain(sparse, lastTile bool) *TerrainPrecompute {
	width, height := _const.ChunkSize*8+1, _const.ChunkSize*2+1
	terrain := &TerrainPrecompute{WidthTiles: width, HeightTiles: height, Tiles: make([]byte, width*height)}
	for index := range terrain.Tiles {
		terrain.Tiles[index] = tileGrass
		if sparse {
			terrain.Tiles[index] = tileWaterDeep
		}
	}
	if sparse {
		for startY := 0; startY < height; startY += _const.ChunkSize {
			for startX := 0; startX < width; startX += _const.ChunkSize {
				x, y := startX, startY
				if lastTile {
					x = minInt(startX+_const.ChunkSize, width) - 1
					y = minInt(startY+_const.ChunkSize, height) - 1
				}
				terrain.Tiles[y*width+x] = tileGrass
			}
		}
	}
	return terrain
}

func TestGenerateSpotsSpawnChanceIndependentOfEligibleTileCountAndPosition(t *testing.T) {
	config := spotsChanceTestConfig(0.5)
	const seed = int64(12345)
	first, firstStats := collectGeneratedSpots(t, spotsChanceTestTerrain(true, false), config, seed)
	for _, terrain := range []*TerrainPrecompute{spotsChanceTestTerrain(true, true), spotsChanceTestTerrain(false, false)} {
		spots, stats := collectGeneratedSpots(t, terrain, config, seed)
		if !reflect.DeepEqual(spotsChanceTestPresence(first), spotsChanceTestPresence(spots)) || !reflect.DeepEqual(firstStats, stats) {
			t.Fatal("chance retried per tile or depended on eligible tile coordinates/density")
		}
	}
	for _, spotType := range spotTypes {
		if firstStats.Generated[spotType] == 0 || firstStats.Generated[spotType] == firstStats.Districts {
			t.Fatalf("chance fixture must exercise accepted and rejected %s districts: %+v", spotType, firstStats)
		}
		if firstStats.Generated[spotType]+firstStats.SkippedByChance[spotType] != firstStats.Districts || firstStats.Skipped[spotType] != 0 {
			t.Fatalf("chance outcomes did not account for each eligible district: %+v", firstStats)
		}
	}
	presence := spotsChanceTestPresence(first)
	var changedAlongX, changedAlongY bool
	for _, spotType := range spotTypes {
		for y := range 3 {
			for x := range 9 {
				current := presence[spotChanceTestKey{spotType, x, y}]
				changedAlongX = changedAlongX || current != presence[spotChanceTestKey{spotType, 0, y}]
				changedAlongY = changedAlongY || current != presence[spotChanceTestKey{spotType, x, 0}]
			}
		}
	}
	if !changedAlongX || !changedAlongY {
		t.Fatal("chance decisions did not vary with both district coordinates")
	}
}

func TestGenerateSpotsSpawnChanceDeterministicAndPreservesRetainedSpots(t *testing.T) {
	terrain := spotsChanceTestTerrain(false, false)
	config := spotsChanceTestConfig(1)
	const seed = int64(12345)
	all, _ := collectGeneratedSpots(t, terrain, config, seed)
	baseline := spotsChanceTestByKey(all)
	for index := range config.Spots {
		config.Spots[index].SpawnChance = spotChanceTestValue(float64(index+1) / float64(spotTypeCount+1))
	}
	first, firstStats := collectGeneratedSpots(t, terrain, config, seed)
	for _, spot := range first {
		if want := baseline[spotChanceTestKey{spot.SpotType, spot.DistrictX, spot.DistrictY}]; spot != want {
			t.Fatalf("chance changed retained center, radius, or quality: got %+v, want %+v", spot, want)
		}
	}
	reordered := config
	reordered.Spots = append([]SpotTypeConfig(nil), config.Spots...)
	for left, right := 0, len(reordered.Spots)-1; left < right; left, right = left+1, right-1 {
		reordered.Spots[left], reordered.Spots[right] = reordered.Spots[right], reordered.Spots[left]
	}
	second, secondStats := collectGeneratedSpots(t, terrain, reordered, seed)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstStats, secondStats) {
		t.Fatal("type declaration order changed chance outcomes or output order")
	}
	differentSeed, _ := collectGeneratedSpots(t, terrain, config, seed+1)
	if reflect.DeepEqual(spotsChanceTestPresence(first), spotsChanceTestPresence(differentSeed)) {
		t.Fatal("seed did not affect chance outcomes")
	}
}

func TestGenerateSpotsSpawnChanceIndependentOfOtherTypes(t *testing.T) {
	terrain := spotsChanceTestTerrain(true, false)
	config := spotsChanceTestConfig(0.5)
	const seed = int64(12345)
	all, _ := collectGeneratedSpots(t, terrain, config, seed)
	baseline := spotsChanceTestByKey(all)
	for _, test := range []struct {
		name          string
		removeTerrain bool
	}{
		{name: "other types disabled"},
		{name: "other types ineligible", removeTerrain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := spotsChanceTestConfig(0.5)
			for index := range 2 {
				if test.removeTerrain {
					changed.Spots[index].CenterTiles = []int{int(tileClay)}
				} else {
					changed.Spots[index].SpawnChance = spotChanceTestValue(0)
				}
			}
			spots, _ := collectGeneratedSpots(t, terrain, changed, seed)
			want := make(map[spotChanceTestKey]GeneratedSpot)
			for key, spot := range baseline {
				if key.typeName != "www" && key.typeName != "clay" {
					want[key] = spot
				}
			}
			if got := spotsChanceTestByKey(spots); !reflect.DeepEqual(got, want) {
				t.Fatalf("other types changed remaining chance outcomes: got %v, want %v", got, want)
			}
		})
	}
	// Equal chance and eligibility must still have independent type decisions.
	www, clay := make(map[[2]int]bool), make(map[[2]int]bool)
	for _, spot := range all {
		if spot.SpotType == "www" {
			www[[2]int{spot.DistrictX, spot.DistrictY}] = true
		}
		if spot.SpotType == "clay" {
			clay[[2]int{spot.DistrictX, spot.DistrictY}] = true
		}
	}
	if reflect.DeepEqual(www, clay) {
		t.Fatal("chance decisions did not vary by spot type")
	}
}
