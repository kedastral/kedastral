package durationx

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"1w2d3h", (7*24 + 2*24 + 3) * time.Hour},
		{"168h", 168 * time.Hour},
		{"30m", 30 * time.Minute},
		{"90s", 90 * time.Second},
		{"1.5d", 36 * time.Hour},
		{"12h7d", (168 + 12) * time.Hour},
		{"2d30m", 48*time.Hour + 30*time.Minute},
		{"-2d", -48 * time.Hour},
		{"0", 0},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParse_Errors(t *testing.T) {
	for _, in := range []string{"", "abc", "7x", "d", "w"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", in)
		}
	}
}

func TestParse_MatchesStdlib(t *testing.T) {
	// For inputs without d/w, Parse must equal time.ParseDuration exactly.
	for _, in := range []string{"30m", "1h30m", "500ms", "2h45m30s", "-15m"} {
		want, err := time.ParseDuration(in)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}
