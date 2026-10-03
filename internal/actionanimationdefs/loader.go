package actionanimationdefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:-]{0,127}$`)
var actorPattern = regexp.MustCompile(`^character/[a-zA-Z0-9_-]{1,128}$`)
var visualPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func LoadFromDirectory(directory string, logger *zap.Logger) (*Registry, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read action animation definitions %s: %w", directory, err)
	}
	var definitions []Definition
	fileCount := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		filename := filepath.Join(directory, entry.Name())
		contents, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		parsed, err := Parse(contents, filename)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, parsed...)
		fileCount++
	}
	if fileCount == 0 {
		return nil, fmt.Errorf("no action animation definition files in %s", directory)
	}
	registry, err := NewRegistry(definitions)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Info("Action animation definitions loaded", zap.Int("files", fileCount), zap.Int("bindings", len(definitions)))
	}
	return registry, nil
}

// Parse validates the whole file before exposing any binding.
func Parse(contents []byte, filename string) ([]Definition, error) {
	var file struct {
		Version  int          `json:"v"`
		Bindings []Definition `json:"bindings"`
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected trailing content", filename)
	}
	var shape any
	if err := json.Unmarshal(contents, &shape); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if err := rejectNulls(shape, "definition"); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported v %d", filename, file.Version)
	}
	if file.Bindings == nil {
		return nil, fmt.Errorf("%s: bindings must be an array", filename)
	}
	for index := range file.Bindings {
		binding := &file.Bindings[index]
		binding.SourceFile = filename
		if err := validate(binding); err != nil {
			return nil, fmt.Errorf("%s: bindings[%d]: %w", filename, index, err)
		}
	}
	return file.Bindings, nil
}

func normalizeSource(source Source) Source {
	return Source{Kind: strings.TrimSpace(source.Kind), Namespace: strings.TrimSpace(source.Namespace), ID: strings.TrimSpace(source.ID)}
}

func validate(binding *Definition) error {
	binding.Source = normalizeSource(binding.Source)
	if !namePattern.MatchString(binding.Key) {
		return fmt.Errorf("key must be a portable name (1-128 characters)")
	}
	if !actorPattern.MatchString(binding.Actor) {
		return fmt.Errorf("actor must reference character/<id>")
	}
	switch binding.Source.Kind {
	case "context", "menu", "craft", "build", "combat":
	default:
		return fmt.Errorf("source.kind is unsupported: %q", binding.Source.Kind)
	}
	if !namePattern.MatchString(binding.Source.ID) {
		return fmt.Errorf("source.id is invalid")
	}
	if binding.Source.Namespace != "" && !namePattern.MatchString(binding.Source.Namespace) {
		return fmt.Errorf("source.namespace is invalid")
	}
	if len(binding.Variants) == 0 {
		return fmt.Errorf("variants must not be empty")
	}
	for index, variant := range binding.Variants {
		if !namePattern.MatchString(variant.Clip) {
			return fmt.Errorf("variants[%d].clip is invalid", index)
		}
		if err := validateEquipment(variant.Equipment); err != nil {
			return fmt.Errorf("variants[%d].equipment: %w", index, err)
		}
	}
	seenSlots := map[string]bool{}
	for index, slot := range binding.UnbindEquipmentSlots {
		if !validEquipmentSlot(slot) || seenSlots[slot] {
			return fmt.Errorf("unbind_equipment_slots[%d]: unknown or duplicate slot %q", index, slot)
		}
		seenSlots[slot] = true
	}
	if binding.Eligibility == nil {
		return fmt.Errorf("eligibility must be an array")
	}
	seen := map[string]bool{}
	for index, predicate := range binding.Eligibility {
		switch predicate {
		case "stationary", "not_carrying", "not_knocked_out":
		default:
			return fmt.Errorf("eligibility[%d] is unsupported: %q", index, predicate)
		}
		if seen[predicate] {
			return fmt.Errorf("eligibility[%d] duplicates %q", index, predicate)
		}
		seen[predicate] = true
	}
	if binding.Facing != "preserve" && binding.Facing != "target" && binding.Facing != "direction" {
		return fmt.Errorf("facing must be preserve, target, or direction")
	}
	if binding.Facing == "direction" && binding.Source.Kind != "combat" {
		return fmt.Errorf("direction facing requires a combat source")
	}
	if binding.Source.Kind == "combat" && (binding.Facing != "direction" || seen["stationary"] || len(binding.SoundCues) > 0) {
		return fmt.Errorf("combat requires direction facing, moving eligibility, and no cycle sound cues")
	}
	seenCues := make(map[string]bool, len(binding.SoundCues))
	previousPhase := 0.0
	for index, cue := range binding.SoundCues {
		if !namePattern.MatchString(cue.ID) || seenCues[cue.ID] {
			return fmt.Errorf("sound_cues[%d].id must be a unique portable name", index)
		}
		seenCues[cue.ID] = true
		if !finite(cue.Phase) || cue.Phase <= previousPhase || cue.Phase > 1 {
			return fmt.Errorf("sound_cues[%d].phase must increase within (0,1]", index)
		}
		previousPhase = cue.Phase
		if !namePattern.MatchString(cue.SoundKey) {
			return fmt.Errorf("sound_cues[%d].sound_key is invalid", index)
		}
		if cue.Source != "actor" && cue.Source != "target" {
			return fmt.Errorf("sound_cues[%d].source must be actor or target", index)
		}
		if cue.Source == "target" && binding.Source.Kind == "craft" {
			return fmt.Errorf("sound_cues[%d]: craft execution has no guaranteed target source", index)
		}
	}
	if !finite(binding.BlendMs) || binding.BlendMs < 0 {
		return fmt.Errorf("blend_ms must be finite and nonnegative")
	}
	f := binding.Frame
	if f.Width < 1 || f.Width > 1024 || f.Height < 1 || f.Height > 1024 {
		return fmt.Errorf("frame dimensions must be integers in 1..1024")
	}
	if f.OriginX < 0 || f.OriginX > f.Width || f.OriginY < 0 || f.OriginY > f.Height {
		return fmt.Errorf("frame origin must lie inside its bounds")
	}
	if p := binding.Preview; p != nil {
		if strings.TrimSpace(p.Label) == "" || len(p.Label) > 256 {
			return fmt.Errorf("preview.label must contain 1-256 bytes")
		}
		if !finite(p.DurationMs) || p.DurationMs <= 0 {
			return fmt.Errorf("preview.duration_ms must be finite and positive")
		}
		if err := validateEquipment(p.Equipment); err != nil {
			return fmt.Errorf("preview.equipment: %w", err)
		}
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func rejectNulls(value any, field string) error {
	switch item := value.(type) {
	case nil:
		return fmt.Errorf("%s must not be null", field)
	case map[string]any:
		for key, child := range item {
			if err := rejectNulls(child, field+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range item {
			if err := rejectNulls(child, fmt.Sprintf("%s[%d]", field, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEquipment(equipment []Equipment) error {
	if equipment == nil {
		return fmt.Errorf("must be an array")
	}
	seen := map[string]bool{}
	for index, item := range equipment {
		if !validEquipmentSlot(item.Slot) {
			return fmt.Errorf("[%d].slot is unknown: %q", index, item.Slot)
		}
		if seen[item.Slot] {
			return fmt.Errorf("[%d].slot is duplicated", index)
		}
		seen[item.Slot] = true
		if !visualPattern.MatchString(item.VisualKey) {
			return fmt.Errorf("[%d].visual_key is invalid", index)
		}
	}
	return nil
}

func validEquipmentSlot(slot string) bool {
	switch slot {
	case "head", "chest", "legs", "feet", "left_hand", "right_hand", "back", "neck", "ring1", "ring2":
		return true
	default:
		return false
	}
}
