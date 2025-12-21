package capacity

import "testing"

func TestParseQuantileLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		// p-notation
		{"p50", 0.50, false},
		{"p75", 0.75, false},
		{"p90", 0.90, false},
		{"p95", 0.95, false},
		{"p99", 0.99, false},
		{"P90", 0.90, false}, // case insensitive

		// decimal notation
		{"0.50", 0.50, false},
		{"0.75", 0.75, false},
		{"0.90", 0.90, false},
		{"0.95", 0.95, false},
		{"0.999", 0.999, false},

		// disabled
		{"0", 0, false},
		{"", 0, false},

		// errors
		{"p101", 0, true},   // percentile > 100
		{"p-5", 0, true},    // negative
		{"1.5", 0, true},    // > 1
		{"-0.5", 0, true},   // negative
		{"pabc", 0, true},   // invalid
		{"invalid", 0, true},// invalid
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseQuantileLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQuantileLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseQuantileLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatQuantileLevel(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "disabled"},
		{0.50, "p50"},
		{0.75, "p75"},
		{0.90, "p90"},
		{0.95, "p95"},
		{0.99, "p99"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatQuantileLevel(tt.input)
			if got != tt.want {
				t.Errorf("FormatQuantileLevel(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
