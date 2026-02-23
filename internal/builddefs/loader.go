package builddefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

type LoadError struct {
	FilePath string
	DefID    int
	Key      string
	Message  string
}

func (e *LoadError) Error() string {
	if e.DefID != 0 && e.Key != "" {
		return fmt.Sprintf("%s: defId=%d key=%s: %s", e.FilePath, e.DefID, e.Key, e.Message)
	}
	if e.DefID != 0 {
		return fmt.Sprintf("%s: defId=%d: %s", e.FilePath, e.DefID, e.Message)
	}
	if e.Key != "" {
		return fmt.Sprintf("%s: key=%s: %s", e.FilePath, e.Key, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
}

var reLineComment = regexp.MustCompile(`(?m)//.*$`)
var reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func stripJSONCComments(data []byte) []byte {
	data = reBlockComment.ReplaceAll(data, nil)
	data = reLineComment.ReplaceAll(data, nil)
	return data
}

func LoadFromDirectory(dir string, logger *zap.Logger) (*Registry, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("Build definitions directory not found, using empty registry", zap.String("dir", dir))
			return NewRegistry(nil), nil
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".json" || ext == ".jsonc" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		logger.Info("No build definitions found", zap.String("dir", dir))
		return NewRegistry(nil), nil
	}

	all := make([]BuildDef, 0, 64)
	seenIDs := make(map[int]string)
	seenKeys := make(map[string]string)
	for _, filePath := range files {
		builds, err := loadFile(filePath)
		if err != nil {
			return nil, err
		}
		for _, build := range builds {
			if prev, exists := seenIDs[build.DefID]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    build.DefID,
					Key:      build.Key,
					Message:  fmt.Sprintf("duplicate defId, already defined in %s", prev),
				}
			}
			if prev, exists := seenKeys[build.Key]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    build.DefID,
					Key:      build.Key,
					Message:  fmt.Sprintf("duplicate key, already defined in %s", prev),
				}
			}
			seenIDs[build.DefID] = filePath
			seenKeys[build.Key] = filePath
			all = append(all, build)
		}
		logger.Debug("Loaded build definitions file", zap.String("file", filepath.Base(filePath)), zap.Int("count", len(builds)))
	}

	logger.Info("Build definitions loaded", zap.Int("files", len(files)), zap.Int("builds", len(all)))
	return NewRegistry(all), nil
}

func loadFile(filePath string) ([]BuildDef, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("failed to read file: %v", err)}
	}

	data = stripJSONCComments(data)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var file BuildsFile
	if err := dec.Decode(&file); err != nil {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("failed to parse JSON: %v", err)}
	}
	if file.Version != 1 {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("unsupported version %d, expected 1", file.Version)}
	}

	for i := range file.Builds {
		applyDefaults(&file.Builds[i])
		if err := validateBuild(&file.Builds[i], filePath); err != nil {
			return nil, err
		}
	}

	return file.Builds, nil
}

func applyDefaults(b *BuildDef) {
	b.Key = strings.TrimSpace(b.Key)
	b.Name = strings.TrimSpace(b.Name)
	b.ObjectKey = strings.TrimSpace(b.ObjectKey)
	b.RequiredSkills = normalizeStringSet(b.RequiredSkills)
	b.RequiredDiscovery = normalizeStringSet(b.RequiredDiscovery)

	for i := range b.Inputs {
		b.Inputs[i].ItemKey = strings.TrimSpace(b.Inputs[i].ItemKey)
		b.Inputs[i].ItemTag = strings.TrimSpace(b.Inputs[i].ItemTag)
	}

	b.AllowedTiles = normalizeIntSet(b.AllowedTiles)
	b.DisallowedTiles = normalizeIntSet(b.DisallowedTiles)

	if b.Name == "" {
		b.Name = b.Key
	}
}

func validateBuild(b *BuildDef, filePath string) error {
	if b.DefID <= 0 {
		return &LoadError{FilePath: filePath, Key: b.Key, Message: "defId must be > 0"}
	}
	if b.Key == "" {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Message: "key is required"}
	}
	if len(b.Inputs) == 0 {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "inputs must not be empty"}
	}
	if b.TicksRequired == 0 {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "ticksRequired must be > 0"}
	}
	if b.StaminaCost < 0 {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "staminaCost must be >= 0"}
	}
	if b.ObjectKey == "" {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "objectKey is required"}
	}

	objectRegistry := objectdefs.Global()
	if objectRegistry == nil {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "object defs registry not loaded"}
	}
	if _, ok := objectRegistry.GetByKey(b.ObjectKey); !ok {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: fmt.Sprintf("objectKey unknown: %s", b.ObjectKey)}
	}

	totalQualityWeight := uint64(0)
	itemRegistry := itemdefs.Global()
	for i, in := range b.Inputs {
		hasItemKey := in.ItemKey != ""
		hasItemTag := in.ItemTag != ""
		if hasItemKey == hasItemTag {
			return &LoadError{
				FilePath: filePath,
				DefID:    b.DefID,
				Key:      b.Key,
				Message:  fmt.Sprintf("inputs[%d] must define exactly one of itemKey or itemTag", i),
			}
		}
		if in.Count == 0 {
			return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: fmt.Sprintf("inputs[%d].count must be > 0", i)}
		}
		if hasItemKey {
			if itemRegistry == nil {
				return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "item defs registry not loaded"}
			}
			if _, ok := itemRegistry.GetByKey(in.ItemKey); !ok {
				return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: fmt.Sprintf("inputs[%d].itemKey unknown: %s", i, in.ItemKey)}
			}
		}
		totalQualityWeight += uint64(in.QualityWeight)
	}
	if totalQualityWeight == 0 {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "at least one input qualityWeight must be > 0"}
	}

	if len(b.AllowedTiles) > 0 && len(b.DisallowedTiles) > 0 {
		return &LoadError{FilePath: filePath, DefID: b.DefID, Key: b.Key, Message: "only one of allowedTiles or disallowedTiles may be set"}
	}
	if err := validateTilesKnown(b.AllowedTiles, "allowedTiles", b, filePath); err != nil {
		return err
	}
	if err := validateTilesKnown(b.DisallowedTiles, "disallowedTiles", b, filePath); err != nil {
		return err
	}

	return nil
}

func validateTilesKnown(tileIDs []int, fieldName string, b *BuildDef, filePath string) error {
	for i, tileID := range tileIDs {
		if !types.IsKnownTileID(tileID) {
			return &LoadError{
				FilePath: filePath,
				DefID:    b.DefID,
				Key:      b.Key,
				Message:  fmt.Sprintf("%s[%d] unknown tile id: %d", fieldName, i, tileID),
			}
		}
	}
	return nil
}

func normalizeStringSet(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func normalizeIntSet(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
