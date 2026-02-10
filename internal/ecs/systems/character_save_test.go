package systems

import (
	"math"
	"testing"
)

func TestNormalizeCharacterHeading(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction float64
		want      int16
	}{
		{name: "zero", direction: 0, want: 0},
		{name: "pi over two", direction: math.Pi / 2, want: 90},
		{name: "negative pi over two", direction: -math.Pi / 2, want: 270},
		{name: "wrap greater than two pi", direction: 2*math.Pi + math.Pi/4, want: 44},
		{name: "nan", direction: math.NaN(), want: 0},
		{name: "positive inf", direction: math.Inf(1), want: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeCharacterHeading(tt.direction)
			if got != tt.want {
				t.Fatalf("normalizeCharacterHeading(%v) = %d, want %d", tt.direction, got, tt.want)
			}
		})
	}
}
