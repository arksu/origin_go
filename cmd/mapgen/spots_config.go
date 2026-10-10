package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	_const "origin/internal/const"
	"origin/internal/types"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	defaultSpotsConfigPath = "etc/mapgen/spots.yaml"
	spotTypeCount          = 5
)

var spotTypes = [spotTypeCount]string{"www", "clay", "soil", "sand", "water"}

// SpotsConfig describes offline placement in tile units. Persisted geometry is
// converted to world units using the shared coordinate constants.
type SpotsConfig struct {
	Version           int              `yaml:"version"`
	DistrictSizeTiles int64            `yaml:"district_size_tiles"`
	RadiusTiles       int64            `yaml:"radius_tiles"`
	BaseQuality       int64            `yaml:"base_quality"`
	PeakQualityMin    int64            `yaml:"peak_quality_min"`
	PeakQualityMax    int64            `yaml:"peak_quality_max"`
	Spots             []SpotTypeConfig `yaml:"spots"`
}

type SpotTypeConfig struct {
	Type        string   `yaml:"type"`
	CenterTiles []int    `yaml:"center_tiles"`
	SpawnChance *float64 `yaml:"spawn_chance,omitempty"`
}

func (definition SpotTypeConfig) spawnChance() float64 {
	if definition.SpawnChance == nil {
		return 1
	}
	return *definition.SpawnChance
}

func LoadSpotsConfig(path string) (SpotsConfig, string, error) {
	resolvedPath, err := resolveConfigPath(path)
	if err != nil {
		return SpotsConfig{}, "", fmt.Errorf("resolve spots config: %w", err)
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return SpotsConfig{}, "", fmt.Errorf("read spots config %q: %w", resolvedPath, err)
	}
	config, err := decodeSpotsConfig(content)
	if err != nil {
		return SpotsConfig{}, "", fmt.Errorf("spots config %q: %w", resolvedPath, err)
	}
	return config, resolvedPath, nil
}

func decodeSpotsConfig(content []byte) (SpotsConfig, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var config SpotsConfig
	if err := decoder.Decode(&config); err != nil {
		return SpotsConfig{}, fmt.Errorf("decode YAML: %w", err)
	}
	if err := decoder.Decode(new(yaml.Node)); err != io.EOF {
		return SpotsConfig{}, fmt.Errorf("must contain a single YAML document")
	}
	// yaml.v3 can truncate floating-point scalars while decoding into integers.
	// Only spawn_chance allows fractional values; integer fields stay strict.
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return SpotsConfig{}, fmt.Errorf("decode YAML: %w", err)
	}
	if err := validateSpotIntegerInput(&document); err != nil {
		return SpotsConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return SpotsConfig{}, err
	}
	return config, nil
}

func validateSpotIntegerInput(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return validateSpotIntegerInput(node.Alias)
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "spawn_chance" {
				for value.Kind == yaml.AliasNode {
					value = value.Alias
				}
				if value.Kind != yaml.ScalarNode || value.Tag != "!!int" && value.Tag != "!!float" {
					return fmt.Errorf("spawn_chance must be a number (line %d)", value.Line)
				}
				continue
			}
			if err := validateSpotIntegerInput(value); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!float" {
		return fmt.Errorf("numeric settings must be integers (line %d)", node.Line)
	}
	for _, child := range node.Content {
		if err := validateSpotIntegerInput(child); err != nil {
			return err
		}
	}
	return nil
}

func (config SpotsConfig) Validate() error {
	if config.Version != 1 {
		return fmt.Errorf("unsupported version %d (expected 1)", config.Version)
	}
	if config.DistrictSizeTiles <= 0 || config.DistrictSizeTiles%int64(_const.ChunkSize) != 0 {
		return fmt.Errorf("district_size_tiles must be positive and a multiple of ChunkSize (%d)", _const.ChunkSize)
	}
	if config.RadiusTiles <= 0 || config.RadiusTiles > config.DistrictSizeTiles {
		return fmt.Errorf("radius_tiles must be positive and no greater than district_size_tiles")
	}
	if config.BaseQuality < 10 || config.BaseQuality > config.PeakQualityMin ||
		config.PeakQualityMin > config.PeakQualityMax || config.PeakQualityMax > math.MaxInt16 {
		return fmt.Errorf("quality must satisfy 10 <= base_quality <= peak_quality_min <= peak_quality_max <= %d", math.MaxInt16)
	}
	if len(config.Spots) != spotTypeCount {
		return fmt.Errorf("spots must declare each of www, clay, soil, sand, water exactly once")
	}
	var seen [spotTypeCount]bool
	for _, definition := range config.Spots {
		index := spotTypeIndex(definition.Type)
		if index < 0 {
			return fmt.Errorf("unknown spot type %q", definition.Type)
		}
		if seen[index] {
			return fmt.Errorf("duplicate spot type %q", definition.Type)
		}
		seen[index] = true
		chance := definition.spawnChance()
		if math.IsNaN(chance) || math.IsInf(chance, 0) || chance < 0 || chance > 1 {
			return fmt.Errorf("spot %q spawn_chance must be finite and between 0 and 1", definition.Type)
		}
		if len(definition.CenterTiles) == 0 {
			return fmt.Errorf("spot %q center_tiles must not be empty", definition.Type)
		}
		var seenTiles [256]bool
		for _, tile := range definition.CenterTiles {
			if !types.IsKnownTileID(tile) {
				return fmt.Errorf("spot %q has unknown center tile %d", definition.Type, tile)
			}
			if tile == types.TileDeepWater || tile == types.TileShallowWater {
				return fmt.Errorf("spot %q cannot have water center tile %d", definition.Type, tile)
			}
			if seenTiles[tile] {
				return fmt.Errorf("spot %q has duplicate center tile %d", definition.Type, tile)
			}
			seenTiles[tile] = true
		}
	}
	if _, err := config.DistrictWorldSize(); err != nil {
		return fmt.Errorf("district_size_tiles: %w", err)
	}
	if _, err := config.RadiusWorld(); err != nil {
		return fmt.Errorf("radius_tiles: %w", err)
	}
	return nil
}

// ValidateForWorld is run before destructive map reset. Mapgen currently starts
// at tile (0,0); validating the last center covers all generated coordinates.
func (config SpotsConfig) ValidateForWorld(widthTiles, heightTiles int) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if widthTiles <= 0 || heightTiles <= 0 {
		return fmt.Errorf("spot world dimensions must be positive")
	}
	if _, err := spotTileCenter(int64(widthTiles) - 1); err != nil {
		return fmt.Errorf("spot world width: %w", err)
	}
	if _, err := spotTileCenter(int64(heightTiles) - 1); err != nil {
		return fmt.Errorf("spot world height: %w", err)
	}
	return nil
}

func (config SpotsConfig) RadiusWorld() (int, error) {
	return spotTileLengthWorld(config.RadiusTiles)
}

func (config SpotsConfig) DistrictWorldSize() (int, error) {
	return spotTileLengthWorld(config.DistrictSizeTiles)
}

func spotTypeIndex(spotType string) int {
	for index, candidate := range spotTypes {
		if candidate == spotType {
			return index
		}
	}
	return -1
}

func spotTileLengthWorld(tiles int64) (int, error) {
	if tiles <= 0 {
		return 0, fmt.Errorf("tile length must be positive")
	}
	world, err := spotCheckedScale(tiles)
	if err != nil {
		return 0, err
	}
	return spotPostgresCoordinate(world)
}

func spotTileCenter(tile int64) (int, error) {
	start, err := spotCheckedScale(tile)
	if err != nil {
		return 0, err
	}
	halfTile := int64(_const.CoordPerTile) / 2
	if start > math.MaxInt64-halfTile {
		return 0, fmt.Errorf("tile center overflows int64")
	}
	return spotPostgresCoordinate(start + halfTile)
}

func spotCheckedScale(tile int64) (int64, error) {
	scale := int64(_const.CoordPerTile)
	if scale <= 0 {
		return 0, fmt.Errorf("CoordPerTile must be positive")
	}
	if tile > math.MaxInt64/scale || tile < math.MinInt64/scale {
		return 0, fmt.Errorf("tile-to-world conversion overflows int64")
	}
	return tile * scale, nil
}

func spotPostgresCoordinate(world int64) (int, error) {
	if world < math.MinInt32 || world > math.MaxInt32 {
		return 0, fmt.Errorf("world value %d is outside PostgreSQL INTEGER range", world)
	}
	return int(world), nil
}

// Floor division preserves the containing tile for negative world coordinates.
func spotWorldToTile(world int64) int64 {
	scale := int64(_const.CoordPerTile)
	tile := world / scale
	if world < 0 && world%scale != 0 {
		tile--
	}
	return tile
}
