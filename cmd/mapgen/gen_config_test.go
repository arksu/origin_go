package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMapgenOptionsFromYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mapgen.yaml")
	content := `
version: 1
world:
  chunks_x: 9
  chunks_y: 8
  seed: 1234
  threads: 3
river:
  enabled: true
  layout_draw: true
  major_count: 20
  lake_count: 120
  lake_size_small_min: 8
  lake_size_small_max: 24
  lake_size_medium_min: 30
  lake_size_medium_max: 70
  lake_size_large_min: 80
  lake_size_large_max: 180
  lake_size_medium_chance: 0.25
  lake_size_large_chance: 0.05
  lake_border_mix: 0.2
  max_lake_degree: 2
  shape_long_meander_scale: 0.7
  shape_short_meander_scale: 1.2
  shape_short_meander_bias: 0.002
  shape_amplitude_scale: 1
  shape_frequency_scale: 1
  shape_noise_scale: 0.3
  shape_along_scale: 0.2
  shape_distance_cap: 0.4
  shape_segment_length: 60
  source_elevation_min: 0.6
  source_chance: 0.0002
  meander_strength: 0.003
  voronoi_cell_size: 64
  voronoi_edge_threshold: 0.12
  voronoi_source_boost: 0.02
  voronoi_bias: 0.02
  sink_lake_chance: 0.03
  lake_min_size: 30
  lake_connect_chance: 0.8
  lake_connection_limit: 100
  lake_link_min_distance: 100
  lake_link_max_distance: 1800
  river_width_min: 5
  river_width_max: 14
  grid_enabled: true
  grid_spacing: 700
  grid_jitter: 80
  trunk_count: 8
  trunk_source_elevation_min: 0.62
  trunk_min_length: 180
  coast_sample_chance: 0.01
  flow_shallow_threshold: 6
  flow_deep_threshold: 20
  bank_radius: 1
  lake_flow_threshold: 28
biomes:
  enabled: true
  hnh_enabled: true
  hnh_variant_density: 0.55
  hnh_region_count: 30
  hnh_region_jitter: 140
  hnh_blend_width: 70
  hnh_smoothing_passes: 2
  hnh_min_patch_tiles: 20
  hnh_swamp_clump_scale: 1.0
  hnh_mountain_rugged_threshold: 0.66
  hnh_forest_share: 0.30
  hnh_grassland_share: 0.30
  hnh_wetland_share: 0.14
  hnh_heath_moor_share: 0.14
  hnh_mountain_share: 0.12
  temperature_scale: 1.1
  moisture_scale: 0.9
  continentalness_scale: 1.0
  erosion_scale: 1.0
  weirdness_scale: 1.0
  domain_warp_strength: 48
ecology:
  enabled: true
  tick_interval: 300
  cells_per_tick: 5000
  chunk_budget: 12
  spread_scale: 1.0
png:
  export: false
  overview_only: false
  output_dir: map_png
  scale: 1
  highlight_rivers: true
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	opts, resolvedPath, err := LoadMapgenOptionsFromYAML(path, DefaultMapgenOptions())
	if err != nil {
		t.Fatalf("LoadMapgenOptionsFromYAML error: %v", err)
	}
	if opts.ChunksX != 9 || opts.ChunksY != 8 {
		t.Fatalf("world section not applied: got chunks=(%d,%d)", opts.ChunksX, opts.ChunksY)
	}
	if opts.Biome.TemperatureScale != 1.1 {
		t.Fatalf("biome temperature scale not applied")
	}
	if opts.River.MajorRiverCount != 20 {
		t.Fatalf("river major count not applied")
	}
	if resolvedPath == "" {
		t.Fatalf("resolved path must be set")
	}
}

func TestLoadMapgenOptionsFromYAMLRejectsUnknownField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	content := `
version: 1
world:
  chunks_x: 10
  chunks_y: 10
  seed: 1
  threads: 2
unexpected: true
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, _, err := LoadMapgenOptionsFromYAML(path, DefaultMapgenOptions())
	if err == nil {
		t.Fatalf("expected unknown field error")
	}
}
