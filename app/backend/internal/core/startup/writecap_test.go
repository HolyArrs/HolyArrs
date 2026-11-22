package startup

import (
	"testing"
	"time"
)

func TestDecideWriteCap(t *testing.T) {
	for env, want := range map[string]struct {
		limit time.Duration
		odd   bool
	}{
		"":       {0, false},
		"90s":    {90 * time.Second, false},
		" 2m ":   {2 * time.Minute, false},
		"90":     {90 * time.Second, false},
		"1h":     {time.Hour, false},
		"5s":     {5 * time.Second, false},
		"1s":     {0, true}, // too short: ordinary writes would fail
		"2h":     {0, true}, // too long: one stuck write holds up the rest
		"0":      {0, true}, // never means no cap
		"-30s":   {0, true},
		"lots":   {0, true},
		"60 sec": {0, true},
		// Bare seconds that would wrap past the int64 nanoseconds of a
		// duration into the accepted range.
		"36028797018963973": {0, true},
	} {
		limit, odd := DecideWriteCap(env)
		if limit != want.limit || odd != want.odd {
			t.Errorf("DecideWriteCap(%q) = %v, %v; want %v, %v", env, limit, odd, want.limit, want.odd)
		}
	}
}
