package main

import (
	"image"
	"reflect"
	"testing"
)

func TestLakeIslandOutlineVariety(test *testing.T) {
	opts := DefaultMapgenOptions().River
	opts.LakeIslandRadiusMin, opts.LakeIslandRadiusMax = 12, 12
	shape := &lakeShape{Lake: drawLake{ID: 7}, Bounds: image.Rect(0, 0, 256, 256), Water: make([]bool, 256*256)}
	accepted, asymmetric, indented := 0, 0, 0
	for seed := int64(1); seed <= 48; seed++ {
		island, cut := shape.island(seed, 0, 256, opts)
		if len(island.Tiles) == 0 {
			continue
		}
		accepted++
		repeated, repeatedCut := shape.island(seed, 0, 256, opts)
		if !reflect.DeepEqual(island, repeated) || !reflect.DeepEqual(cut, repeatedCut) {
			test.Fatalf("seed %d: island outline is not deterministic", seed)
		}
		if !lakeWaterConnected(cut, 256) {
			test.Fatalf("seed %d: fragmented island", seed)
		}
		filled := append([]bool(nil), cut...)
		fillLakeHoles(filled, 256, 256)
		if !reflect.DeepEqual(filled, cut) {
			test.Fatalf("seed %d: enclosed water hole in island", seed)
		}
		mismatched := 0
		for _, index := range island.Tiles {
			point := image.Pt(index%256, index/256)
			reflected := island.Center.Add(island.Center.Sub(point))
			if !reflected.In(shape.Bounds) || !cut[shape.index(reflected.X, reflected.Y)] {
				mismatched++
			}
		}
		if mismatched*5 > len(island.Tiles) {
			asymmetric++
		}
		if lakeIslandHasBay(cut, 256) {
			indented++
		}
	}
	if accepted < 32 || asymmetric*4 < accepted*3 || indented*2 < accepted {
		test.Fatalf("insufficient outline variety: accepted=%d asymmetric=%d indented=%d", accepted, asymmetric, indented)
	}
}

func lakeIslandHasBay(cut []bool, width int) bool {
	for _, transpose := range []bool{false, true} {
		for row := 0; row < width; row++ {
			seenLand, seenGap := false, false
			for column := 0; column < width; column++ {
				index := row*width + column
				if transpose {
					index = column*width + row
				}
				if cut[index] {
					if seenGap {
						return true
					}
					seenLand = true
				} else if seenLand {
					seenGap = true
				}
			}
		}
	}
	return false
}

func TestLakeIslandSmallOutlinesStayConnected(test *testing.T) {
	opts := DefaultMapgenOptions().River
	shape := &lakeShape{Lake: drawLake{ID: 3}, Bounds: image.Rect(0, 0, 128, 128), Water: make([]bool, 128*128)}
	for _, radius := range []int{2, 4, 8, 12, 32, 64} {
		opts.LakeIslandRadiusMin, opts.LakeIslandRadiusMax = radius, radius
		accepted := 0
		for seed := int64(1); seed <= 48; seed++ {
			island, cut := shape.island(seed, 0, 128, opts)
			if len(island.Tiles) == 0 {
				continue
			}
			accepted++
			if island.Radius != radius || !cut[shape.index(island.Center.X, island.Center.Y)] || !lakeWaterConnected(cut, 128) {
				test.Fatalf("radius %d seed %d: invalid island footprint", radius, seed)
			}
			filled := append([]bool(nil), cut...)
			fillLakeHoles(filled, 128, 128)
			if !reflect.DeepEqual(filled, cut) {
				test.Fatalf("radius %d seed %d: enclosed water hole", radius, seed)
			}
		}
		if accepted == 0 {
			test.Fatalf("radius %d: all candidates rejected", radius)
		}
	}
}
