package itemdefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"go.uber.org/zap"
)

// LoadError represents an error during item definition loading.
type LoadError struct {
	FilePath string
	DefID    int
	Key      string
	Message  string
}

func (e *LoadError) Error() string {
	if e.DefID != 0 {
		return fmt.Sprintf("%s: defId=%d: %s", e.FilePath, e.DefID, e.Message)
	}
	if e.Key != "" {
		return fmt.Sprintf("%s: key=%s: %s", e.FilePath, e.Key, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
}

// stripJSONCComments removes single-line (//) and multi-line (/* */) comments
// from JSONC input so it can be parsed by standard JSON decoder.
var reLineComment = regexp.MustCompile(`(?m)//.*$`)
var reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func stripJSONCComments(data []byte) []byte {
	data = reBlockComment.ReplaceAll(data, nil)
	data = reLineComment.ReplaceAll(data, nil)
	return data
}

// LoadFromDirectory loads all item definitions from JSONC files in the specified directory.
func LoadFromDirectory(dir string, logger *zap.Logger) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var files []string
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
		logger.Warn("No JSON/JSONC files found in data directory", zap.String("dir", dir))
		return NewRegistry(nil), nil
	}

	var allItems []ItemDef
	seenDefIDs := make(map[int]string)
	seenKeys := make(map[string]string)

	for _, filePath := range files {
		items, err := loadFile(filePath)
		if err != nil {
			return nil, err
		}

		for _, item := range items {
			if existingFile, exists := seenDefIDs[item.DefID]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    item.DefID,
					Message:  fmt.Sprintf("duplicate defId, already defined in %s", existingFile),
				}
			}
			seenDefIDs[item.DefID] = filePath

			if existingFile, exists := seenKeys[item.Key]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					Key:      item.Key,
					Message:  fmt.Sprintf("duplicate key, already defined in %s", existingFile),
				}
			}
			seenKeys[item.Key] = filePath

			allItems = append(allItems, item)
		}

		logger.Debug("Loaded item definitions file",
			zap.String("file", filepath.Base(filePath)),
			zap.Int("count", len(items)),
		)
	}

	logger.Info("Item definitions loaded",
		zap.Int("files", len(files)),
		zap.Int("items", len(allItems)),
	)

	return NewRegistry(allItems), nil
}

func loadFile(filePath string) ([]ItemDef, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &LoadError{
			FilePath: filePath,
			Message:  fmt.Sprintf("failed to read file: %v", err),
		}
	}

	data = stripJSONCComments(data)

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var file ItemsFile
	if err := decoder.Decode(&file); err != nil {
		return nil, &LoadError{
			FilePath: filePath,
			Message:  fmt.Sprintf("failed to parse JSON: %v", err),
		}
	}

	if file.Version != 1 {
		return nil, &LoadError{
			FilePath: filePath,
			Message:  fmt.Sprintf("unsupported version %d, expected 1", file.Version),
		}
	}

	for i := range file.Items {
		applyDefaults(&file.Items[i])
		if err := validateItem(&file.Items[i], filePath); err != nil {
			return nil, err
		}
	}

	return file.Items, nil
}

func validateItem(item *ItemDef, filePath string) error {
	if item.DefID <= 0 {
		return &LoadError{
			FilePath: filePath,
			Key:      item.Key,
			Message:  "defId must be > 0",
		}
	}

	if item.Key == "" {
		return &LoadError{
			FilePath: filePath,
			DefID:    item.DefID,
			Message:  "key is required",
		}
	}

	if item.Size.W < 1 {
		return &LoadError{
			FilePath: filePath,
			DefID:    item.DefID,
			Key:      item.Key,
			Message:  "size.w must be >= 1",
		}
	}

	if item.Size.H < 1 {
		return &LoadError{
			FilePath: filePath,
			DefID:    item.DefID,
			Key:      item.Key,
			Message:  "size.h must be >= 1",
		}
	}

	if item.Stack != nil {
		switch item.Stack.Mode {
		case StackModeNone:
			// ok
		case StackModeStack:
			if item.Stack.Max < 2 {
				return &LoadError{
					FilePath: filePath,
					DefID:    item.DefID,
					Key:      item.Key,
					Message:  "stack.max must be >= 2 when stack.mode is 'stack'",
				}
			}
		default:
			return &LoadError{
				FilePath: filePath,
				DefID:    item.DefID,
				Key:      item.Key,
				Message:  fmt.Sprintf("invalid stack.mode '%s', expected 'none' or 'stack'", item.Stack.Mode),
			}
		}
	}

	if item.Container != nil {
		if item.Container.Size.W < 1 {
			return &LoadError{
				FilePath: filePath,
				DefID:    item.DefID,
				Key:      item.Key,
				Message:  "container.size.w must be >= 1",
			}
		}
		if item.Container.Size.H < 1 {
			return &LoadError{
				FilePath: filePath,
				DefID:    item.DefID,
				Key:      item.Key,
				Message:  "container.size.h must be >= 1",
			}
		}
	}

	return nil
}

func applyDefaults(item *ItemDef) {
	trueVal := true
	emptySlice := []string{}

	if item.Allowed.Hand == nil {
		item.Allowed.Hand = &trueVal
	}
	if item.Allowed.Grid == nil {
		item.Allowed.Grid = &trueVal
	}
	if item.Allowed.EquipmentSlots == nil {
		item.Allowed.EquipmentSlots = emptySlice
	}
	if item.Resource == "" {
		item.Resource = item.Key
	}
}
