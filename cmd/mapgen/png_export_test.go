package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestExportMapPNGsWritesChunksAndOverview(t *testing.T) {
	tempDir := t.TempDir()

	widthTiles := 4
	heightTiles := 4
	chunkSize := 2
	tiles := []byte{
		tileWaterDeep, tileWater, tileSand, tileGrass,
		tileWater, tileForestPine, tileForestLeaf, tileGrass,
		tileSand, tileGrass, tileWaterDeep, tileWater,
		tileGrass, tileGrass, tileWater, tileWaterDeep,
	}

	chunksDir := filepath.Join(tempDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatalf("mkdir chunks dir: %v", err)
	}
	stale := filepath.Join(chunksDir, "stale.png")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	opts := PNGOptions{
		Export:          true,
		OutputDir:       tempDir,
		Scale:           1,
		HighlightRivers: true,
	}
	riverClass := make([]RiverClass, len(tiles))
	riverClass[0] = riverDeep
	if err := ExportMapPNGs(tiles, riverClass, widthTiles, heightTiles, chunkSize, opts); err != nil {
		t.Fatalf("ExportMapPNGs error: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("expected stale png to be removed, err=%v", err)
	}

	expectedChunks := []string{
		"chunk_x0000_y0000.png",
		"chunk_x0001_y0000.png",
		"chunk_x0000_y0001.png",
		"chunk_x0001_y0001.png",
	}
	for _, file := range expectedChunks {
		path := filepath.Join(chunksDir, file)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected chunk png %q: %v", path, err)
		}
	}

	overview := filepath.Join(tempDir, "overview.png")
	if _, err := os.Stat(overview); err != nil {
		t.Fatalf("expected overview png %q: %v", overview, err)
	}

	chunkPath := filepath.Join(chunksDir, "chunk_x0001_y0001.png")
	chunkFile, err := os.Open(chunkPath)
	if err != nil {
		t.Fatalf("open chunk png: %v", err)
	}
	defer chunkFile.Close()
	chunkConfig, err := png.DecodeConfig(chunkFile)
	if err != nil {
		t.Fatalf("decode chunk config: %v", err)
	}
	if chunkConfig.Width != 2 || chunkConfig.Height != 2 {
		t.Fatalf("unexpected chunk png dimensions: %dx%d", chunkConfig.Width, chunkConfig.Height)
	}

	overviewFile, err := os.Open(overview)
	if err != nil {
		t.Fatalf("open overview png: %v", err)
	}
	defer overviewFile.Close()
	overviewConfig, err := png.DecodeConfig(overviewFile)
	if err != nil {
		t.Fatalf("decode overview config: %v", err)
	}
	if overviewConfig.Width != widthTiles || overviewConfig.Height != heightTiles {
		t.Fatalf("unexpected overview dimensions: %dx%d", overviewConfig.Width, overviewConfig.Height)
	}
}
