package objectdefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"origin/internal/game/behaviors/contracts"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// LoadError represents an error during object definition loading.
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

// LoadFromDirectory loads all object definitions from JSONC files in the specified directory.
func LoadFromDirectory(dir string, behaviors contracts.BehaviorRegistry, logger *zap.Logger) (*Registry, error) {
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

	var allObjects []ObjectDef
	seenDefIDs := make(map[int]string)
	seenKeys := make(map[string]string)

	for _, filePath := range files {
		objects, err := loadFile(filePath, behaviors)
		if err != nil {
			return nil, err
		}

		for _, obj := range objects {
			if existingFile, exists := seenDefIDs[obj.DefID]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Message:  fmt.Sprintf("duplicate defId, already defined in %s", existingFile),
				}
			}
			seenDefIDs[obj.DefID] = filePath

			if existingFile, exists := seenKeys[obj.Key]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					Key:      obj.Key,
					Message:  fmt.Sprintf("duplicate key, already defined in %s", existingFile),
				}
			}
			seenKeys[obj.Key] = filePath

			allObjects = append(allObjects, obj)
		}

		logger.Debug("Loaded object definitions file",
			zap.String("file", filepath.Base(filePath)),
			zap.Int("count", len(objects)),
		)
	}

	logger.Info("Object definitions loaded",
		zap.Int("files", len(files)),
		zap.Int("objects", len(allObjects)),
	)

	return NewRegistry(allObjects), nil
}

func loadFile(filePath string, behaviors contracts.BehaviorRegistry) ([]ObjectDef, error) {
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

	var file ObjectsFile
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

	for i := range file.Objects {
		applyDefaults(&file.Objects[i])
		if err := validateObject(&file.Objects[i], filePath, behaviors); err != nil {
			return nil, err
		}
	}

	return file.Objects, nil
}

func applyDefaults(obj *ObjectDef) {
	if obj.Static == nil {
		obj.IsStatic = true
	} else {
		obj.IsStatic = *obj.Static
	}
	if obj.ContextMenuEvenForOneItem == nil {
		obj.ContextMenuEvenForOneItemValue = true
	} else {
		obj.ContextMenuEvenForOneItemValue = *obj.ContextMenuEvenForOneItem
	}

	if obj.Components != nil {
		if obj.Components.Collider != nil {
			if obj.Components.Collider.Layer == 0 {
				obj.Components.Collider.Layer = 1
			}
			if obj.Components.Collider.Mask == 0 {
				obj.Components.Collider.Mask = 1
			}
		}
		for i := range obj.Components.Inventory {
			if obj.Components.Inventory[i].Kind == "" {
				obj.Components.Inventory[i].Kind = "grid"
			}
		}
	}

	if len(obj.Behaviors) == 0 {
		obj.BehaviorOrder = nil
		obj.BehaviorPriorities = nil
		obj.TreeConfig = nil
		obj.TakeConfig = nil
	}
}

func validateObject(obj *ObjectDef, filePath string, behaviors contracts.BehaviorRegistry) error {
	if obj.DefID <= 0 {
		return &LoadError{
			FilePath: filePath,
			Key:      obj.Key,
			Message:  "defId must be > 0",
		}
	}

	if obj.Key == "" {
		return &LoadError{
			FilePath: filePath,
			DefID:    obj.DefID,
			Message:  "key is required",
		}
	}
	if strings.TrimSpace(obj.Name) == "" {
		return &LoadError{
			FilePath: filePath,
			DefID:    obj.DefID,
			Key:      obj.Key,
			Message:  "name is required",
		}
	}

	// Validate collider
	if obj.Components != nil && obj.Components.Collider != nil {
		c := obj.Components.Collider
		if c.W <= 0 {
			return &LoadError{
				FilePath: filePath,
				DefID:    obj.DefID,
				Key:      obj.Key,
				Message:  "components.collider.w must be > 0",
			}
		}
		if c.H <= 0 {
			return &LoadError{
				FilePath: filePath,
				DefID:    obj.DefID,
				Key:      obj.Key,
				Message:  "components.collider.h must be > 0",
			}
		}
	}

	// Validate inventory
	if obj.Components != nil {
		for idx, inv := range obj.Components.Inventory {
			if inv.W <= 0 {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("components.inventory[%d].w must be > 0", idx),
				}
			}
			if inv.H <= 0 {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("components.inventory[%d].h must be > 0", idx),
				}
			}
		}
	}

	// Validate behaviors map and resolve runtime order/config.
	if len(obj.Behaviors) > 0 {
		if behaviors == nil {
			return &LoadError{
				FilePath: filePath,
				DefID:    obj.DefID,
				Key:      obj.Key,
				Message:  "behavior registry is required when object defines behaviors",
			}
		}

		keys := make([]string, 0, len(obj.Behaviors))
		for key := range obj.Behaviors {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		if err := behaviors.ValidateBehaviorKeys(keys); err != nil {
			return &LoadError{
				FilePath: filePath,
				DefID:    obj.DefID,
				Key:      obj.Key,
				Message:  err.Error(),
			}
		}

		obj.BehaviorPriorities = make(map[string]int, len(keys))
		obj.BehaviorOrder = append(obj.BehaviorOrder[:0], keys...)

		for _, behaviorKey := range keys {
			raw := obj.Behaviors[behaviorKey]
			if len(raw) == 0 {
				raw = []byte("{}")
			}

			behavior, found := behaviors.GetBehavior(behaviorKey)
			if !found || behavior == nil {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("behavior %q not found in registry", behaviorKey),
				}
			}

			defConfigValidator, ok := behavior.(contracts.BehaviorDefConfigValidator)
			if !ok {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("behavior %q does not implement def config validator", behaviorKey),
				}
			}

			priority, err := defConfigValidator.ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
				BehaviorKey: behaviorKey,
				RawConfig:   raw,
				Def:         obj,
			})
			if err != nil {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  err.Error(),
				}
			}
			if priority <= 0 {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("behavior %q returned invalid priority %d", behaviorKey, priority),
				}
			}
			obj.BehaviorPriorities[behaviorKey] = priority
		}

		sort.SliceStable(obj.BehaviorOrder, func(i, j int) bool {
			leftKey := obj.BehaviorOrder[i]
			rightKey := obj.BehaviorOrder[j]
			leftPriority := obj.BehaviorPriorities[leftKey]
			rightPriority := obj.BehaviorPriorities[rightKey]
			if leftPriority == rightPriority {
				return leftKey < rightKey
			}
			return leftPriority < rightPriority
		})
	}

	// Validate appearance: unique IDs, no duplicates
	if len(obj.Appearance) > 0 {
		seenIDs := make(map[string]struct{}, len(obj.Appearance))
		for _, a := range obj.Appearance {
			if a.ID == "" {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  "appearance.id is required",
				}
			}
			if _, dup := seenIDs[a.ID]; dup {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("duplicate appearance.id %q", a.ID),
				}
			}
			seenIDs[a.ID] = struct{}{}
			if a.Resource == "" {
				return &LoadError{
					FilePath: filePath,
					DefID:    obj.DefID,
					Key:      obj.Key,
					Message:  fmt.Sprintf("appearance[%s].resource is required", a.ID),
				}
			}
		}
	}

	// resource is required if appearance is empty
	if len(obj.Appearance) == 0 && obj.Resource == "" {
		return &LoadError{
			FilePath: filePath,
			DefID:    obj.DefID,
			Key:      obj.Key,
			Message:  "resource is required when appearance is empty",
		}
	}

	return nil
}
