package characterattrs

import (
	"encoding/json"
)

type Name string

const (
	INT Name = "INT"
	STR Name = "STR"
	PER Name = "PER"
	PSY Name = "PSY"
	AGI Name = "AGI"
	CON Name = "CON"
	CHA Name = "CHA"
	DEX Name = "DEX"
	WIL Name = "WIL"
)

const DefaultValue = 1

var requiredNames = [...]Name{
	INT,
	STR,
	PER,
	PSY,
	AGI,
	CON,
	CHA,
	DEX,
	WIL,
}

type Values map[Name]int

func RequiredNames() []Name {
	names := make([]Name, len(requiredNames))
	copy(names, requiredNames[:])
	return names
}

func Default() Values {
	values := make(Values, len(requiredNames))
	for _, name := range requiredNames {
		values[name] = DefaultValue
	}
	return values
}

func Clone(values Values) Values {
	cloned := make(Values, len(values))
	for name, value := range values {
		cloned[name] = value
	}
	return cloned
}

func Get(values Values, name Name) int {
	value, ok := values[name]
	if !ok || value < DefaultValue {
		return DefaultValue
	}
	return value
}

func Normalize(values Values) Values {
	normalized := Default()
	for _, name := range requiredNames {
		value, ok := values[name]
		if !ok || value < DefaultValue {
			continue
		}
		normalized[name] = value
	}
	return normalized
}

func FromRaw(raw json.RawMessage) (Values, bool) {
	if len(raw) == 0 {
		return Default(), true
	}

	var rawValues map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawValues); err != nil {
		return Default(), true
	}

	values := make(Values, len(requiredNames))
	changed := len(rawValues) != len(requiredNames)

	for _, name := range requiredNames {
		rawValue, ok := rawValues[string(name)]
		if !ok {
			values[name] = DefaultValue
			changed = true
			continue
		}

		var value int
		if err := json.Unmarshal(rawValue, &value); err != nil || value < DefaultValue {
			values[name] = DefaultValue
			changed = true
			continue
		}

		values[name] = value
	}

	return values, changed
}

func Marshal(values Values) (json.RawMessage, error) {
	normalized := Normalize(values)
	rawMap := make(map[string]int, len(requiredNames))
	for _, name := range requiredNames {
		rawMap[string(name)] = normalized[name]
	}

	data, err := json.Marshal(rawMap)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(data), nil
}
