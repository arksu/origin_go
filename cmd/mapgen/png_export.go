package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func ExportMapPNGs(tiles []byte, riverClass []RiverClass, widthTiles, heightTiles, chunkSize int, opts PNGOptions) error {
	if !opts.Export {
		return nil
	}
	if opts.Scale <= 0 {
		return fmt.Errorf("invalid png scale: %d", opts.Scale)
	}
	if widthTiles <= 0 || heightTiles <= 0 {
		return fmt.Errorf("invalid map size: widthTiles=%d heightTiles=%d", widthTiles, heightTiles)
	}
	if len(tiles) != widthTiles*heightTiles {
		return fmt.Errorf("tiles length mismatch: got=%d want=%d", len(tiles), widthTiles*heightTiles)
	}
	if len(riverClass) > 0 && len(riverClass) != len(tiles) {
		return fmt.Errorf("river class length mismatch: got=%d want=%d", len(riverClass), len(tiles))
	}
	if chunkSize <= 0 {
		return fmt.Errorf("invalid chunk size: %d", chunkSize)
	}
	if widthTiles%chunkSize != 0 || heightTiles%chunkSize != 0 {
		return fmt.Errorf("map size must be divisible by chunk size (width=%d height=%d chunk=%d)", widthTiles, heightTiles, chunkSize)
	}

	if err := validateOverviewImageSize(widthTiles, heightTiles, opts.Scale); err != nil {
		return err
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create png output directory: %w", err)
	}

	chunksDir := filepath.Join(opts.OutputDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		return fmt.Errorf("create chunk png directory: %w", err)
	}
	if err := cleanPNGFiles(chunksDir); err != nil {
		return fmt.Errorf("clean chunk png directory: %w", err)
	}

	chunksX := widthTiles / chunkSize
	chunksY := heightTiles / chunkSize
	for chunkY := 0; chunkY < chunksY; chunkY++ {
		for chunkX := 0; chunkX < chunksX; chunkX++ {
			if err := writeChunkPNG(chunksDir, tiles, riverClass, widthTiles, chunkSize, chunkX, chunkY, opts.Scale, opts.HighlightRivers); err != nil {
				return err
			}
		}
	}

	overviewPath := filepath.Join(opts.OutputDir, "overview.png")
	if err := writeOverviewPNG(overviewPath, tiles, riverClass, widthTiles, heightTiles, opts.Scale, opts.HighlightRivers); err != nil {
		return err
	}

	return nil
}

func writeChunkPNG(outputDir string, tiles []byte, riverClass []RiverClass, widthTiles, chunkSize, chunkX, chunkY, scale int, highlightRivers bool) error {
	chunkPxSize := chunkSize * scale
	img := image.NewRGBA(image.Rect(0, 0, chunkPxSize, chunkPxSize))

	baseTileX := chunkX * chunkSize
	baseTileY := chunkY * chunkSize
	for localY := 0; localY < chunkSize; localY++ {
		for localX := 0; localX < chunkSize; localX++ {
			globalX := baseTileX + localX
			globalY := baseTileY + localY
			idx := tileIndex(globalX, globalY, widthTiles)
			tile := tiles[idx]
			rc := riverNone
			if len(riverClass) > 0 {
				rc = riverClass[idx]
			}
			drawScaledPixel(img, localX*scale, localY*scale, scale, tileColor(tile, rc, highlightRivers))
		}
	}

	filename := fmt.Sprintf("chunk_x%04d_y%04d.png", chunkX, chunkY)
	path := filepath.Join(outputDir, filename)
	if err := writePNG(path, img); err != nil {
		return fmt.Errorf("write chunk png %q: %w", path, err)
	}
	return nil
}

func writeOverviewPNG(path string, tiles []byte, riverClass []RiverClass, widthTiles, heightTiles, scale int, highlightRivers bool) error {
	img := image.NewRGBA(image.Rect(0, 0, widthTiles*scale, heightTiles*scale))

	for y := 0; y < heightTiles; y++ {
		for x := 0; x < widthTiles; x++ {
			idx := tileIndex(x, y, widthTiles)
			tile := tiles[idx]
			rc := riverNone
			if len(riverClass) > 0 {
				rc = riverClass[idx]
			}
			drawScaledPixel(img, x*scale, y*scale, scale, tileColor(tile, rc, highlightRivers))
		}
	}

	if err := writePNG(path, img); err != nil {
		return fmt.Errorf("write overview png %q: %w", path, err)
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func drawScaledPixel(img *image.RGBA, x, y, scale int, clr color.RGBA) {
	if scale == 1 {
		img.SetRGBA(x, y, clr)
		return
	}
	for py := 0; py < scale; py++ {
		for px := 0; px < scale; px++ {
			img.SetRGBA(x+px, y+py, clr)
		}
	}
}

func cleanPNGFiles(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".png") {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func tileColor(tile byte, riverClass RiverClass, highlightRivers bool) color.RGBA {
	if highlightRivers {
		switch riverClass {
		case riverDeep:
			return color.RGBA{R: 0, G: 170, B: 255, A: 255}
		case riverShallow:
			return color.RGBA{R: 73, G: 211, B: 255, A: 255}
		}
	}

	switch tile {
	case tileWaterDeep:
		return color.RGBA{R: 14, G: 49, B: 100, A: 255}
	case tileWater:
		return color.RGBA{R: 47, G: 117, B: 182, A: 255}
	case tileSand:
		return color.RGBA{R: 217, G: 196, B: 127, A: 255}
	case tileForestPine:
		return color.RGBA{R: 43, G: 88, B: 43, A: 255}
	case tileForestLeaf:
		return color.RGBA{R: 62, G: 123, B: 60, A: 255}
	case tileGrass:
		return color.RGBA{R: 104, G: 164, B: 78, A: 255}
	case tileSwamp:
		return color.RGBA{R: 83, G: 106, B: 82, A: 255}
	case tileClay:
		return color.RGBA{R: 158, G: 124, B: 93, A: 255}
	case tileDirt:
		return color.RGBA{R: 127, G: 96, B: 66, A: 255}
	case tileStone:
		return color.RGBA{R: 132, G: 132, B: 132, A: 255}
	default:
		return color.RGBA{R: 255, G: 0, B: 255, A: 255}
	}
}
