package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Series is a time-ordered metric series: Times[i] corresponds to Values[i].
type Series struct {
	Times  []time.Time
	Values []float64
}

// Len returns the number of points in the series.
func (s Series) Len() int { return len(s.Values) }

// LoadCSV reads a two-column CSV of (timestamp, value). The timestamp may be RFC3339
// or a Unix timestamp in seconds. A header row is detected and skipped automatically
// (when the second column of the first row is not numeric). Rows are assumed to be in
// ascending time order at a fixed step.
func LoadCSV(r io.Reader) (Series, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return Series{}, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return Series{}, fmt.Errorf("csv is empty")
	}

	start := 0
	if len(records[0]) >= 2 {
		if _, err := strconv.ParseFloat(records[0][1], 64); err != nil {
			start = 1 // first row is a header
		}
	}

	series := Series{}
	for i := start; i < len(records); i++ {
		record := records[i]
		if len(record) < 2 {
			return Series{}, fmt.Errorf("row %d: expected at least 2 columns, got %d", i+1, len(record))
		}

		ts, err := parseTime(record[0])
		if err != nil {
			return Series{}, fmt.Errorf("row %d: %w", i+1, err)
		}

		value, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return Series{}, fmt.Errorf("row %d: invalid value %q: %w", i+1, record[1], err)
		}

		series.Times = append(series.Times, ts)
		series.Values = append(series.Values, value)
	}

	if series.Len() == 0 {
		return Series{}, fmt.Errorf("csv contained no data rows")
	}

	return series, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q (want RFC3339 or Unix seconds)", s)
}
