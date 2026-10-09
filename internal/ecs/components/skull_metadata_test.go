package components

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkullMetadataHintExt(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata *SkullMetadata
		want     string
	}{
		{"bound", &SkullMetadata{CharacterID: 42, Nickname: "Alice", DeathDate: "2026-01-10"}, "Alice died on January 10, 2026"},
		{"leap date", &SkullMetadata{CharacterID: 42, Nickname: "<img src=x>Алиса", DeathDate: "2024-02-29"}, "<img src=x>Алиса died on February 29, 2024"},
		{"ordinary skull", nil, ""},
		{"unbound", &SkullMetadata{Nickname: "Alice", DeathDate: "2026-01-10"}, ""},
		{"missing name", &SkullMetadata{CharacterID: 42, DeathDate: "2026-01-10"}, ""},
		{"invalid date", &SkullMetadata{CharacterID: 42, Nickname: "Alice", DeathDate: "2026-02-29"}, ""},
		{"timestamp", &SkullMetadata{CharacterID: 42, Nickname: "Alice", DeathDate: "2026-01-10T10:00:00Z"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) { require.Equal(t, test.want, test.metadata.HintExt()) })
	}
}

func TestSkullMetadataCloneOwnsSnapshot(t *testing.T) {
	var empty *SkullMetadata
	require.Nil(t, empty.Clone())
	original := &SkullMetadata{CharacterID: 42, Nickname: "Alice", DeathDate: "2026-01-10"}
	copy := original.Clone()
	require.Equal(t, original, copy)
	require.NotSame(t, original, copy)
	original.Nickname = "Changed"
	require.Equal(t, "Alice", copy.Nickname)
}
