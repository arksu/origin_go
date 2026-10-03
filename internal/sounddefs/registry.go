package sounddefs

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
	"sync"

	"go.uber.org/zap"
)

const (
	ModeWorld = "world"
	ModeLocal = "local"
)

type LocalAttenuation struct {
	NearGain     float64 `json:"near_gain"`
	FarGain      float64 `json:"far_gain"`
	Shape        float64 `json:"shape"`
	NearDistance float64 `json:"near_distance,omitempty"`
}

type Profile struct {
	Key                string            `json:"key"`
	Mode               string            `json:"mode"`
	Loudness           float64           `json:"loudness"`
	Volume             float64           `json:"volume"`
	Files              []string          `json:"files"`
	Priority           int               `json:"priority"`
	MaxVoices          int               `json:"max_voices"`
	MaxVoicesPerSource int               `json:"max_voices_per_source"`
	LocalAttenuation   *LocalAttenuation `json:"local_attenuation,omitempty"`
	FeedbackTrigger    string            `json:"feedback_trigger,omitempty"`
	SourceFile         string            `json:"-"`
}

type Registry struct {
	byKey map[string]*Profile
	all   []*Profile
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var audioPathPattern = regexp.MustCompile(`^sound/[a-zA-Z0-9_/-]+\.(mp3|wav|ogg)$`)
var globalRegistry *Registry
var registryOnce sync.Once

func SetGlobal(registry *Registry)           { registryOnce.Do(func() { globalRegistry = registry }) }
func Global() *Registry                      { return globalRegistry }
func SetGlobalForTesting(registry *Registry) { globalRegistry = registry }

func NewRegistry(profiles []Profile) (*Registry, error) {
	registry := &Registry{byKey: make(map[string]*Profile, len(profiles))}
	triggers := make(map[string]string)
	for _, profile := range profiles {
		if err := validate(profile); err != nil {
			return nil, fmt.Errorf("%s: sound %s: %w", profile.SourceFile, profile.Key, err)
		}
		if _, exists := registry.byKey[profile.Key]; exists {
			return nil, fmt.Errorf("%s: duplicate sound key %q", profile.SourceFile, profile.Key)
		}
		if profile.FeedbackTrigger != "" {
			if previous, exists := triggers[profile.FeedbackTrigger]; exists {
				return nil, fmt.Errorf("%s: feedback trigger %q is already used by %s", profile.SourceFile, profile.FeedbackTrigger, previous)
			}
			triggers[profile.FeedbackTrigger] = profile.Key
		}
		profile.Files = append([]string(nil), profile.Files...)
		registry.byKey[profile.Key] = &profile
		registry.all = append(registry.all, &profile)
	}
	return registry, nil
}

func (registry *Registry) Get(key string) (*Profile, bool) {
	if registry == nil {
		return nil, false
	}
	profile, exists := registry.byKey[key]
	return profile, exists
}

func (registry *Registry) All() []*Profile {
	if registry == nil {
		return nil
	}
	return append([]*Profile(nil), registry.all...)
}

func EffectiveRadius(loudness, hearing, maximum float64) (float64, error) {
	if !finitePositive(loudness) || !finitePositive(hearing) || !finitePositive(maximum) || maximum > math.Sqrt(math.MaxFloat64) {
		return 0, fmt.Errorf("loudness, hearing and maximum radius must be finite positive values safe to square")
	}
	radius := loudness * hearing
	if !finitePositive(radius) || radius > maximum {
		return 0, fmt.Errorf("effective radius %g exceeds configured maximum %g", radius, maximum)
	}
	return radius, nil
}

func (registry *Registry) ValidateHearing(hearing, maximum float64) error {
	if !finitePositive(hearing) {
		return fmt.Errorf("hearing must be finite and positive")
	}
	for _, profile := range registry.all {
		radius, err := EffectiveRadius(profile.Loudness, hearing, maximum)
		if err != nil {
			return fmt.Errorf("%s: sound %s: %w", profile.SourceFile, profile.Key, err)
		}
		if profile.Mode == ModeLocal && radius <= profile.LocalAttenuation.NearDistance {
			return fmt.Errorf("%s: sound %s: effective radius %g must exceed local_attenuation.near_distance %g", profile.SourceFile, profile.Key, radius, profile.LocalAttenuation.NearDistance)
		}
	}
	return nil
}

func validate(profile Profile) error {
	if !namePattern.MatchString(profile.Key) {
		return fmt.Errorf("key must be a portable name")
	}
	if profile.Mode != ModeWorld && profile.Mode != ModeLocal {
		return fmt.Errorf("unsupported mode %q", profile.Mode)
	}
	if !finitePositive(profile.Loudness) {
		return fmt.Errorf("loudness must be finite and positive")
	}
	if !finiteGain(profile.Volume) {
		return fmt.Errorf("volume must be finite and in [0,1]")
	}
	if len(profile.Files) == 0 {
		return fmt.Errorf("files must not be empty")
	}
	seenFiles := make(map[string]bool)
	for _, filename := range profile.Files {
		if !audioPathPattern.MatchString(filename) || strings.Contains(filename, "//") || seenFiles[filename] {
			return fmt.Errorf("invalid or duplicate audio path %q", filename)
		}
		seenFiles[filename] = true
	}
	if profile.Priority < 0 || profile.Priority > 255 {
		return fmt.Errorf("priority must be in 0..255")
	}
	if profile.MaxVoices < 1 || profile.MaxVoices > 128 || profile.MaxVoicesPerSource < 1 || profile.MaxVoicesPerSource > profile.MaxVoices {
		return fmt.Errorf("voice budgets must satisfy 1 <= max_voices_per_source <= max_voices <= 128")
	}
	if profile.Mode == ModeLocal {
		curve := profile.LocalAttenuation
		if curve == nil || !finiteGain(curve.NearGain) || !finiteGain(curve.FarGain) || curve.FarGain > curve.NearGain || !finitePositive(curve.Shape) {
			return fmt.Errorf("local_attenuation requires 0 <= far_gain <= near_gain <= 1 and finite shape > 0")
		}
		if math.IsNaN(curve.NearDistance) || math.IsInf(curve.NearDistance, 0) || curve.NearDistance < 0 || curve.NearDistance >= profile.Loudness {
			return fmt.Errorf("local_attenuation.near_distance must be finite and satisfy 0 <= near_distance < loudness")
		}
	} else if profile.LocalAttenuation != nil || profile.FeedbackTrigger != "" {
		return fmt.Errorf("world sound cannot declare local attenuation or feedback trigger")
	}
	if profile.FeedbackTrigger != "" && !namePattern.MatchString(profile.FeedbackTrigger) {
		return fmt.Errorf("invalid feedback_trigger")
	}
	return nil
}

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
func finiteGain(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func Parse(contents []byte, filename string) ([]Profile, error) {
	var file struct {
		Version int       `json:"v"`
		Sounds  []Profile `json:"sounds"`
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected trailing content", filename)
	}
	var shape map[string]any
	if err := json.Unmarshal(contents, &shape); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if err := rejectNulls(shape, "definition"); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if file.Version != 1 || file.Sounds == nil {
		return nil, fmt.Errorf("%s: expected v=1 and sounds array", filename)
	}
	for index, raw := range shape["sounds"].([]any) {
		profile := raw.(map[string]any)
		for _, required := range []string{"key", "mode", "loudness", "volume", "files", "priority", "max_voices", "max_voices_per_source"} {
			if _, present := profile[required]; !present {
				return nil, fmt.Errorf("%s: sounds[%d].%s is required", filename, index, required)
			}
		}
		if curve, present := profile["local_attenuation"]; present {
			for _, required := range []string{"near_gain", "far_gain", "shape"} {
				if _, present := curve.(map[string]any)[required]; !present {
					return nil, fmt.Errorf("%s: sounds[%d].local_attenuation.%s is required", filename, index, required)
				}
			}
		}
		file.Sounds[index].SourceFile = filename
	}
	if _, err := NewRegistry(file.Sounds); err != nil {
		return nil, err
	}
	return file.Sounds, nil
}

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

func LoadFromDirectory(directory string, logger *zap.Logger) (*Registry, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read sound definitions %s: %w", directory, err)
	}
	var profiles []Profile
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
		profiles = append(profiles, parsed...)
		fileCount++
	}
	if fileCount == 0 {
		return nil, fmt.Errorf("no sound definition files in %s", directory)
	}
	registry, err := NewRegistry(profiles)
	if err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Info("Sound definitions loaded", zap.Int("files", fileCount), zap.Int("sounds", len(profiles)))
	}
	return registry, nil
}
