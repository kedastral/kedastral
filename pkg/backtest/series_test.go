package backtest

import (
	"strings"
	"testing"
)

func TestLoadCSV_HeaderAndRFC3339(t *testing.T) {
	input := "timestamp,value\n2026-01-01T00:00:00Z,100\n2026-01-01T00:01:00Z,110.5\n"
	series, err := LoadCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadCSV() error = %v", err)
	}
	if series.Len() != 2 {
		t.Fatalf("Len = %d, want 2", series.Len())
	}
	if series.Values[1] != 110.5 {
		t.Errorf("Values[1] = %v, want 110.5", series.Values[1])
	}
	if series.Times[0].Hour() != 0 || series.Times[1].Minute() != 1 {
		t.Errorf("timestamps parsed incorrectly: %v", series.Times)
	}
}

func TestLoadCSV_UnixNoHeader(t *testing.T) {
	input := "1735689600,5\n1735689660,7\n"
	series, err := LoadCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadCSV() error = %v", err)
	}
	if series.Len() != 2 {
		t.Fatalf("Len = %d, want 2", series.Len())
	}
	if series.Values[0] != 5 || series.Values[1] != 7 {
		t.Errorf("values = %v, want [5 7]", series.Values)
	}
}

func TestLoadCSV_Errors(t *testing.T) {
	if _, err := LoadCSV(strings.NewReader("")); err == nil {
		t.Error("expected error for empty input")
	}
	if _, err := LoadCSV(strings.NewReader("2026-01-01T00:00:00Z,notanumber\n")); err == nil {
		t.Error("expected error for non-numeric value")
	}
	if _, err := LoadCSV(strings.NewReader("not-a-time,5\n")); err == nil {
		t.Error("expected error for invalid timestamp")
	}
}
