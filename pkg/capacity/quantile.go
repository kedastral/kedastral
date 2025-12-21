package capacity

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseQuantileLevel parses a quantile level from either p-notation (p90, p95)
// or decimal notation (0.90, 0.95).
//
// Examples:
//   - "p50" → 0.50
//   - "p90" → 0.90
//   - "p95" → 0.95
//   - "p99" → 0.99
//   - "0.90" → 0.90
//   - "0" → 0 (disabled)
//
// Returns error if the format is invalid or value is out of range [0, 1].
func ParseQuantileLevel(s string) (float64, error) {
	s = strings.TrimSpace(s)

	if s == "" || s == "0" {
		return 0, nil
	}

	if strings.HasPrefix(strings.ToLower(s), "p") {
		percentileStr := s[1:]
		percentile, err := strconv.ParseFloat(percentileStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid p-notation %q: %w", s, err)
		}
		if percentile < 0 || percentile > 100 {
			return 0, fmt.Errorf("percentile %v out of range [0, 100]", percentile)
		}
		return percentile / 100.0, nil
	}

	quantile, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid quantile %q: %w", s, err)
	}
	if quantile < 0 || quantile > 1 {
		return 0, fmt.Errorf("quantile %v out of range [0, 1]", quantile)
	}
	return quantile, nil
}

// FormatQuantileLevel formats a quantile level as p-notation for display.
//
// Examples:
//   - 0.50 → "p50"
//   - 0.90 → "p90"
//   - 0.95 → "p95"
//   - 0 → "disabled"
func FormatQuantileLevel(q float64) string {
	if q == 0 {
		return "disabled"
	}
	percentile := q * 100
	if percentile == float64(int(percentile)) {
		return fmt.Sprintf("p%d", int(percentile))
	}
	return fmt.Sprintf("p%.1f", percentile)
}
