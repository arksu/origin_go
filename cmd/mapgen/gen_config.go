package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type worldConfig struct {
	ChunksX int   `yaml:"chunks_x"`
	ChunksY int   `yaml:"chunks_y"`
	Seed    int64 `yaml:"seed"`
	Threads int   `yaml:"threads"`
}

type mapgenConfigFile struct {
	Version int             `yaml:"version"`
	World   *worldConfig    `yaml:"world"`
	River   *RiverOptions   `yaml:"river"`
	Biomes  *BiomeOptions   `yaml:"biomes"`
	Ecology *EcologyOptions `yaml:"ecology"`
	PNG     *PNGOptions     `yaml:"png"`
}

func LoadMapgenOptionsFromYAML(path string, defaults MapgenOptions) (MapgenOptions, string, error) {
	resolvedPath, err := resolveConfigPath(path)
	if err != nil {
		return MapgenOptions{}, "", err
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return MapgenOptions{}, "", fmt.Errorf("read gen config %q: %w", resolvedPath, err)
	}
	opts, err := decodeMapgenOptions(content, resolvedPath, defaults)
	if err != nil {
		return MapgenOptions{}, "", err
	}
	return opts, resolvedPath, err
}

func decodeMapgenOptions(content []byte, path string, defaults MapgenOptions) (MapgenOptions, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	biomeDefaults := defaults.Biome
	cfg := mapgenConfigFile{Biomes: &biomeDefaults}
	if err := decoder.Decode(&cfg); err != nil {
		return MapgenOptions{}, fmt.Errorf("decode gen config %q: %w", path, err)
	}
	if err := decoder.Decode(new(yaml.Node)); err != io.EOF {
		return MapgenOptions{}, fmt.Errorf("gen config %q must contain a single YAML document", path)
	}

	if cfg.Version != 1 {
		return MapgenOptions{}, fmt.Errorf("unsupported gen config version %d in %q (expected 1)", cfg.Version, path)
	}

	opts := defaults
	if cfg.World != nil {
		opts.ChunksX = cfg.World.ChunksX
		opts.ChunksY = cfg.World.ChunksY
		opts.Seed = cfg.World.Seed
		opts.Threads = cfg.World.Threads
	}
	if cfg.River != nil {
		opts.River = *cfg.River
	}
	if cfg.Biomes != nil {
		opts.Biome = *cfg.Biomes
	}
	if cfg.Ecology != nil {
		opts.Ecology = *cfg.Ecology
	}
	if cfg.PNG != nil {
		opts.PNG = *cfg.PNG
	}

	return opts, nil
}

func resolveConfigPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("gen config path must not be empty")
	}

	var candidates []string
	if filepath.IsAbs(trimmed) {
		candidates = []string{trimmed}
	} else {
		candidates = []string{
			trimmed,
			filepath.Join("..", trimmed),
			filepath.Join("..", "..", trimmed),
		}
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	return "", fmt.Errorf("gen config file not found: %q", path)
}
