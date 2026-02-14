package behaviors

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const defaultBehaviorPriority = 100

type priorityOnlyDefConfig struct {
	Priority int `json:"priority,omitempty"`
}

func decodeStrictJSON(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing JSON data")
	}
	return nil
}

func parsePriorityOnlyConfig(raw []byte, behaviorKey string) (int, error) {
	var cfg priorityOnlyDefConfig
	if err := decodeStrictJSON(raw, &cfg); err != nil {
		return 0, fmt.Errorf("invalid %s config: %w", behaviorKey, err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	return cfg.Priority, nil
}
