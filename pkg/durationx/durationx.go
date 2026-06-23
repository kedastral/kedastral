// Package durationx parses Go-style durations extended with day ("d") and week ("w")
// units, which time.ParseDuration does not support.
//
// It is a strict superset of time.ParseDuration: any value the standard library
// accepts is parsed identically, while "d" and "w" components are also allowed and may
// be combined with standard units (e.g. "1w2d3h", "168h", "7d", "90m").
//
// It also provides flag helpers (Var, Duration) that drop in for flag.DurationVar and
// flag.Duration so command-line flags accept the extended syntax.
package durationx

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	day  = 24 * time.Hour
	week = 7 * day
)

var dayWeekRe = regexp.MustCompile(`(\d+(?:\.\d+)?)([wd])`)

// Parse parses a duration string, supporting "d" (days) and "w" (weeks) in addition to
// the units understood by time.ParseDuration. An optional leading sign is honored.
func Parse(s string) (time.Duration, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty duration")
	}

	negative := false
	switch trimmed[0] {
	case '-':
		negative = true
		trimmed = trimmed[1:]
	case '+':
		trimmed = trimmed[1:]
	}

	var extra time.Duration
	var parseErr error
	remainder := dayWeekRe.ReplaceAllStringFunc(trimmed, func(token string) string {
		match := dayWeekRe.FindStringSubmatch(token)
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			parseErr = err
			return ""
		}
		switch match[2] {
		case "w":
			extra += time.Duration(value * float64(week))
		case "d":
			extra += time.Duration(value * float64(day))
		}
		return ""
	})
	if parseErr != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, parseErr)
	}

	if remainder != "" {
		base, err := time.ParseDuration(remainder)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		extra += base
	}

	if negative {
		extra = -extra
	}
	return extra, nil
}

// durationValue adapts a *time.Duration to flag.Value using Parse.
type durationValue struct {
	target *time.Duration
}

func (v durationValue) String() string {
	if v.target == nil {
		return "0s"
	}
	return v.target.String()
}

func (v durationValue) Set(s string) error {
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*v.target = parsed
	return nil
}

// withPlaceholder ensures the flag's help shows a "duration" placeholder. The flag
// package only derives a placeholder from a backquoted word in the usage string for
// custom flag.Value types, so we inject one (which also advertises the d/w syntax)
// unless the caller already provided their own.
func withPlaceholder(usage string) string {
	if strings.ContainsRune(usage, '`') {
		return usage
	}
	return usage + " (`duration`, e.g. 30m, 6h, 7d)"
}

// Var defines a duration flag with the extended d/w syntax, mirroring flag.DurationVar.
func Var(target *time.Duration, name string, value time.Duration, usage string) {
	*target = value
	flag.Var(durationValue{target}, name, withPlaceholder(usage))
}

// Duration defines a duration flag with the extended d/w syntax and returns a pointer,
// mirroring flag.Duration.
func Duration(name string, value time.Duration, usage string) *time.Duration {
	target := new(time.Duration)
	Var(target, name, value, usage)
	return target
}
