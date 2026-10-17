package behaviors

import "testing"

func TestFormatLastDisconnect(t *testing.T) {
	const at int64 = 1_000_000
	for _, test := range []struct {
		name    string
		at, now int64
		want    string
	}{
		{"unknown", 0, at, "Unknown"},
		{"future", at, at - 1, "just now"},
		{"same", at, at, "just now"},
		{"seconds", at, at + 59_999, "just now"},
		{"minute", at, at + 60_000, "1 minute ago"},
		{"minutes", at, at + 119_999, "1 minute ago"},
		{"two minutes", at, at + 120_000, "2 minutes ago"},
		{"before hour", at, at + 3_599_999, "59 minutes ago"},
		{"hour", at, at + 3_600_000, "1 hour ago"},
		{"hours", at, at + 7_200_000, "2 hours ago"},
		{"before day", at, at + 86_399_999, "23 hours ago"},
		{"day", at, at + 86_400_000, "1 day ago"},
		{"days", at, at + 3*86_400_000, "3 days ago"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := formatLastDisconnect(test.at, test.now); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
