package craftdefs

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

	"go.uber.org/zap"
)

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
			logger.Warn("Craft definitions directory not found, using empty registry", zap.String("dir", dir))
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
		logger.Info("No craft definitions found", zap.String("dir", dir))
		return NewRegistry(nil), nil
	}

	all := make([]CraftDef, 0, 64)
	seenIDs := make(map[int]string)
	seenKeys := make(map[string]string)
	for _, filePath := range files {
		crafts, err := loadFile(filePath)
		if err != nil {
			return nil, err
		}
		for _, craft := range crafts {
			if prev, exists := seenIDs[craft.DefID]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    craft.DefID,
					Key:      craft.Key,
					Message:  fmt.Sprintf("duplicate defId, already defined in %s", prev),
				}
			}
			if prev, exists := seenKeys[craft.Key]; exists {
				return nil, &LoadError{
					FilePath: filePath,
					DefID:    craft.DefID,
					Key:      craft.Key,
					Message:  fmt.Sprintf("duplicate key, already defined in %s", prev),
				}
			}
			seenIDs[craft.DefID] = filePath
			seenKeys[craft.Key] = filePath
			all = append(all, craft)
		}
		logger.Debug("Loaded craft definitions file", zap.String("file", filepath.Base(filePath)), zap.Int("count", len(crafts)))
	}

	logger.Info("Craft definitions loaded", zap.Int("files", len(files)), zap.Int("crafts", len(all)))
	return NewRegistry(all), nil
}

func loadFile(filePath string) ([]CraftDef, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("failed to read file: %v", err)}
	}

	data = stripJSONCComments(data)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var file CraftsFile
	if err := dec.Decode(&file); err != nil {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("failed to parse JSON: %v", err)}
	}
	if file.Version != 1 {
		return nil, &LoadError{FilePath: filePath, Message: fmt.Sprintf("unsupported version %d, expected 1", file.Version)}
	}

	for i := range file.Crafts {
		applyDefaults(&file.Crafts[i])
		if err := validateCraft(&file.Crafts[i], filePath); err != nil {
			return nil, err
		}
	}

	return file.Crafts, nil
}

func applyDefaults(c *CraftDef) {
	c.RequiredSkills = normalizeStringSet(c.RequiredSkills)
	c.RequiredDiscovery = normalizeStringSet(c.RequiredDiscovery)
	c.RequiredLinkedObject = strings.TrimSpace(c.RequiredLinkedObject)
	if c.QualityFormula == "" {
		c.QualityFormula = QualityFormulaWeightedAverageFloor
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = c.Key
	}
}

func validateCraft(c *CraftDef, filePath string) error {
	if c.DefID <= 0 {
		return &LoadError{FilePath: filePath, Key: c.Key, Message: "defId must be > 0"}
	}
	if strings.TrimSpace(c.Key) == "" {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Message: "key is required"}
	}
	if len(c.Inputs) == 0 {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: "inputs must not be empty"}
	}
	if len(c.Outputs) == 0 {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: "outputs must not be empty"}
	}
	if c.TicksRequired == 0 {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: "ticksRequired must be > 0"}
	}
	if c.StaminaCost < 0 {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: "staminaCost must be >= 0"}
	}

	totalQualityWeight := uint64(0)
	for i, in := range c.Inputs {
		if strings.TrimSpace(in.ItemKey) == "" {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("inputs[%d].itemKey is required", i)}
		}
		if in.Count == 0 {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("inputs[%d].count must be > 0", i)}
		}
		if _, ok := itemdefs.Global().GetByKey(in.ItemKey); !ok {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("inputs[%d].itemKey unknown: %s", i, in.ItemKey)}
		}
		totalQualityWeight += uint64(in.QualityWeight)
	}
	if totalQualityWeight == 0 {
		return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: "at least one input qualityWeight must be > 0"}
	}
	for i, out := range c.Outputs {
		if strings.TrimSpace(out.ItemKey) == "" {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("outputs[%d].itemKey is required", i)}
		}
		if out.Count == 0 {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("outputs[%d].count must be > 0", i)}
		}
		if _, ok := itemdefs.Global().GetByKey(out.ItemKey); !ok {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("outputs[%d].itemKey unknown: %s", i, out.ItemKey)}
		}
	}
	if c.RequiredLinkedObject != "" {
		if _, ok := objectdefs.Global().GetByKey(c.RequiredLinkedObject); !ok {
			return &LoadError{FilePath: filePath, DefID: c.DefID, Key: c.Key, Message: fmt.Sprintf("requiredLinkedObjectKey unknown: %s", c.RequiredLinkedObject)}
		}
	}
	switch c.QualityFormula {
	case QualityFormulaWeightedAverageFloor:
		// ok (default)
	default:
		// Allow future custom formula ids now; runtime may reject unsupported formulas.
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
