package actiondefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/network/proto"

	"go.uber.org/zap"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type definitionFile struct {
	Version int          `json:"v"`
	Actions []Definition `json:"actions"`
}

func LoadFromDirectory(directory string, logger *zap.Logger) (*Registry, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read action definitions directory %s: %w", directory, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no action definitions in %s", directory)
	}

	definitions := make([]Definition, 0, len(files))
	seen := make(map[string]string, len(files))
	for _, filename := range files {
		fileDefinitions, err := loadFile(filename)
		if err != nil {
			return nil, err
		}
		for _, definition := range fileDefinitions {
			if previous, duplicate := seen[definition.ID]; duplicate {
				return nil, fmt.Errorf("%s: duplicate action %q (already in %s)", filename, definition.ID, previous)
			}
			seen[definition.ID] = filename
			definitions = append(definitions, definition)
		}
	}
	if logger != nil {
		logger.Info("Action definitions loaded", zap.Int("files", len(files)), zap.Int("actions", len(definitions)))
	}
	return NewRegistry(definitions), nil
}

func loadFile(filename string) ([]Definition, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%s: read action definition: %w", filename, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var file definitionFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: parse action definition: %w", filename, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected content after action definition", filename)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported action definition version %d", filename, file.Version)
	}
	var rawFile struct {
		Actions []map[string]json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(contents, &rawFile); err != nil {
		return nil, fmt.Errorf("%s: parse action sections: %w", filename, err)
	}
	if len(file.Actions) == 0 {
		return nil, fmt.Errorf("%s: actions must not be empty", filename)
	}
	for index := range file.Actions {
		definition := &file.Actions[index]
		definition.SourceFile = filename
		for _, section := range []string{"presentation", "target", "requirements", "execution"} {
			value, exists := rawFile.Actions[index][section]
			if !exists || bytes.Equal(value, []byte("null")) {
				return nil, fmt.Errorf("%s: action %q: %s section is required", filename, definition.ID, section)
			}
		}
		if err := validateDefinition(definition); err != nil {
			return nil, fmt.Errorf("%s: action %q: %w", filename, definition.ID, err)
		}
	}
	return file.Actions, nil
}

func validateDefinition(definition *Definition) error {
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Presentation.Label = strings.TrimSpace(definition.Presentation.Label)
	definition.Presentation.MenuIcon = strings.TrimSpace(definition.Presentation.MenuIcon)
	definition.Target.Cursor = strings.TrimSpace(definition.Target.Cursor)
	if !identifierPattern.MatchString(definition.ID) {
		return fmt.Errorf("id must be a lowercase identifier")
	}
	if definition.Presentation.Label == "" {
		return fmt.Errorf("presentation.label is required")
	}
	if !validIconPath(definition.Presentation.MenuIcon) {
		return fmt.Errorf("presentation.menuIcon must be a local asset under /assets/")
	}
	switch definition.Target.Kind {
	case TargetNone:
		if definition.Target.Cursor != "" || definition.IsRepeatable != nil {
			return fmt.Errorf("none target cannot declare cursor or isRepeatable")
		}
	case TargetObject, TargetTile:
	default:
		return fmt.Errorf("unknown target kind %q", definition.Target.Kind)
	}
	if definition.Target.Cursor != "" && !identifierPattern.MatchString(definition.Target.Cursor) {
		return fmt.Errorf("target.cursor must be a lowercase identifier")
	}
	if definition.Target.Approach != "" && (definition.Target.Approach != ApproachTileCenter || definition.Target.Kind != TargetTile) {
		return fmt.Errorf("target.approach must be tile_center on a tile target")
	}
	if definition.Execution.Ticks < 0 || definition.Execution.Stamina < 0 || math.IsNaN(definition.Execution.Stamina) || math.IsInf(definition.Execution.Stamina, 0) {
		return fmt.Errorf("execution ticks and stamina must be finite and non-negative")
	}
	for index, skill := range definition.Requirements.Skills {
		skill = strings.TrimSpace(skill)
		if !identifierPattern.MatchString(skill) {
			return fmt.Errorf("requirements.skills[%d] is invalid", index)
		}
		definition.Requirements.Skills[index] = skill
	}
	for index := range definition.Requirements.Equipment {
		requirement := &definition.Requirements.Equipment[index]
		requirement.ItemKey = strings.TrimSpace(requirement.ItemKey)
		requirement.ItemTag = strings.TrimSpace(requirement.ItemTag)
		if (requirement.ItemKey == "") == (requirement.ItemTag == "") {
			return fmt.Errorf("requirements.equipment[%d] needs exactly one itemKey or itemTag", index)
		}
		if len(requirement.Slots) == 0 {
			return fmt.Errorf("requirements.equipment[%d].slots must not be empty", index)
		}
		for slotIndex, slot := range requirement.Slots {
			slot = strings.TrimSpace(slot)
			if proto.EquipSlot_EQUIP_SLOT_NONE == inventory.StringToEquipSlot(slot) {
				return fmt.Errorf("requirements.equipment[%d].slots[%d] is unknown", index, slotIndex)
			}
			requirement.Slots[slotIndex] = slot
		}
		if requirement.ItemKey != "" {
			if !identifierPattern.MatchString(requirement.ItemKey) {
				return fmt.Errorf("requirements.equipment[%d].itemKey is invalid", index)
			}
			items := itemdefs.Global()
			if items == nil {
				return fmt.Errorf("item definitions must load before action definitions")
			}
			if _, exists := items.GetByKey(requirement.ItemKey); !exists {
				return fmt.Errorf("requirements.equipment[%d].itemKey %q is unknown", index, requirement.ItemKey)
			}
		} else if !identifierPattern.MatchString(requirement.ItemTag) {
			return fmt.Errorf("requirements.equipment[%d].itemTag is invalid", index)
		}
	}
	return nil
}

func validIconPath(icon string) bool {
	if !strings.HasPrefix(icon, "/assets/") || strings.ContainsAny(icon, `\?#%`) {
		return false
	}
	return path.Clean(icon) == icon && icon != "/assets/"
}
