package components

import (
	"encoding/json"
	"sort"
	"strings"
)

type characterExperiencePayload struct {
	LP       int64 `json:"lp"`
	Nature   int64 `json:"nature"`
	Industry int64 `json:"industry"`
	Combat   int64 `json:"combat"`
}

func MarshalCharacterExperience(exp CharacterExperience) ([]byte, error) {
	return json.Marshal(characterExperiencePayload{
		LP:       exp.LP,
		Nature:   exp.Nature,
		Industry: exp.Industry,
		Combat:   exp.Combat,
	})
}

func UnmarshalCharacterExperience(raw []byte) (CharacterExperience, error) {
	if len(raw) == 0 {
		return CharacterExperience{}, nil
	}

	var payload characterExperiencePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CharacterExperience{}, err
	}

	return CharacterExperience{
		LP:       payload.LP,
		Nature:   payload.Nature,
		Industry: payload.Industry,
		Combat:   payload.Combat,
	}, nil
}

func MarshalStringSet(values []string) ([]byte, error) {
	return json.Marshal(NormalizeStringSet(values))
}

func UnmarshalStringSet(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return NormalizeStringSet(values), nil
}

func NormalizeStringSet(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		unique[item] = struct{}{}
	}

	if len(unique) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(unique))
	for item := range unique {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
