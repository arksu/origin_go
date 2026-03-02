package main

import (
	"math/rand"
	"testing"
)

func TestMapgenOptionsValidateRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*MapgenOptions)
	}{
		{
			name: "chunks x must be positive",
			mut: func(opts *MapgenOptions) {
				opts.ChunksX = 0
			},
		},
		{
			name: "threads must be positive",
			mut: func(opts *MapgenOptions) {
				opts.Threads = 0
			},
		},
		{
			name: "png scale must be positive",
			mut: func(opts *MapgenOptions) {
				opts.PNG.Scale = 0
			},
		},
		{
			name: "river source chance range",
			mut: func(opts *MapgenOptions) {
				opts.River.SourceChance = 1.1
			},
		},
		{
			name: "river major count must be positive",
			mut: func(opts *MapgenOptions) {
				opts.River.MajorRiverCount = 0
			},
		},
		{
			name: "river lake count must be > 1",
			mut: func(opts *MapgenOptions) {
				opts.River.LakeCount = 1
			},
		},
		{
			name: "river lake border mix range",
			mut: func(opts *MapgenOptions) {
				opts.River.LakeBorderMix = -0.1
			},
		},
		{
			name: "river max lake degree must be positive",
			mut: func(opts *MapgenOptions) {
				opts.River.MaxLakeDegree = 0
			},
		},
		{
			name: "voronoi cell size must be > 1",
			mut: func(opts *MapgenOptions) {
				opts.River.VoronoiCellSize = 1
			},
		},
		{
			name: "voronoi source boost range",
			mut: func(opts *MapgenOptions) {
				opts.River.VoronoiSourceBoost = -0.1
			},
		},
		{
			name: "sink lake chance range",
			mut: func(opts *MapgenOptions) {
				opts.River.SinkLakeChance = 1.1
			},
		},
		{
			name: "lake link distance ordering",
			mut: func(opts *MapgenOptions) {
				opts.River.LakeLinkMinDistance = 100
				opts.River.LakeLinkMaxDistance = 100
			},
		},
		{
			name: "river width ordering",
			mut: func(opts *MapgenOptions) {
				opts.River.RiverWidthMin = 10
				opts.River.RiverWidthMax = 5
			},
		},
		{
			name: "grid jitter must be less than spacing half",
			mut: func(opts *MapgenOptions) {
				opts.River.GridSpacing = 100
				opts.River.GridJitter = 50
			},
		},
		{
			name: "trunk count must be >= 0",
			mut: func(opts *MapgenOptions) {
				opts.River.TrunkRiverCount = -1
			},
		},
		{
			name: "coast sample chance must be in range",
			mut: func(opts *MapgenOptions) {
				opts.River.CoastSampleChance = 0
			},
		},
		{
			name: "river deep threshold ordering",
			mut: func(opts *MapgenOptions) {
				opts.River.FlowShallowThreshold = 10
				opts.River.FlowDeepThreshold = 10
			},
		},
		{
			name: "river lake threshold ordering",
			mut: func(opts *MapgenOptions) {
				opts.River.FlowShallowThreshold = 20
				opts.River.LakeFlowThreshold = 19
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultMapgenOptions()
			tc.mut(&opts)
			if err := opts.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestMapgenOptionsValidateOverviewLimit(t *testing.T) {
	opts := DefaultMapgenOptions()
	opts.PNG.Export = true
	opts.ChunksX = 200
	opts.ChunksY = 200
	opts.PNG.Scale = 8

	if err := opts.Validate(); err == nil {
		t.Fatalf("expected overview size validation error")
	}
}

func TestDeterministicChunkSeedStableAcrossOrder(t *testing.T) {
	seed := int64(123456)
	coordsOrderA := [][2]int{{0, 0}, {1, 0}, {0, 1}, {5, 7}, {7, 5}}
	coordsOrderB := [][2]int{{7, 5}, {0, 1}, {5, 7}, {1, 0}, {0, 0}}

	valuesA := make(map[[2]int]int64, len(coordsOrderA))
	for _, coord := range coordsOrderA {
		rng := rand.New(rand.NewSource(deterministicChunkSeed(seed, coord[0], coord[1])))
		valuesA[coord] = rng.Int63()
	}

	for _, coord := range coordsOrderB {
		rng := rand.New(rand.NewSource(deterministicChunkSeed(seed, coord[0], coord[1])))
		got := rng.Int63()
		if got != valuesA[coord] {
			t.Fatalf("chunk (%d,%d) mismatch across order: got=%d want=%d", coord[0], coord[1], got, valuesA[coord])
		}
	}
}
