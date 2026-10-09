package components

import (
	"time"

	"origin/internal/types"
)

// SkullMetadata is a snapshot of the deceased character associated with one
// skull. DeathDate is a calendar date in YYYY-MM-DD form, without a time zone.
// Treat instance metadata as immutable after a skull has been issued.
type SkullMetadata struct {
	CharacterID types.EntityID `json:"character_id"`
	Nickname    string         `json:"nickname"`
	DeathDate   string         `json:"death_date"`
}

// Clone creates an owned copy for persistence and worker snapshots.
func (s *SkullMetadata) Clone() *SkullMetadata {
	if s == nil {
		return nil
	}
	copy := *s
	return &copy
}

func (s *SkullMetadata) HintExt() string {
	if s == nil || s.CharacterID == 0 || s.Nickname == "" {
		return ""
	}
	date, err := time.Parse(time.DateOnly, s.DeathDate)
	if err != nil || date.Format(time.DateOnly) != s.DeathDate {
		return ""
	}
	return s.Nickname + " died on " + date.Format("January 2, 2006")
}
