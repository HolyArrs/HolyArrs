package library

import (
	"testing"
	"time"
)

func d(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestGate(t *testing.T) {
	cases := []struct {
		name                      string
		gate                      string
		delay                     int
		cinema, digital, physical string
		driver, opens             string
	}{
		{"no dates", GateReleased, 0, "", "", "", "none", ""},
		{"no dates, cinemas", GateInCinemas, 0, "", "", "", "none", ""},
		{"no dates, announced", GateAnnounced, 5, "", "", "", "announced", ""},
		{"cinema only, released", GateReleased, 0, "2026-03-01", "", "", "cinema_plus_90", "2026-05-30"},
		{"cinema only, cinemas", GateInCinemas, 0, "2026-03-01", "", "", "cinema", "2026-03-01"},
		{"cinemas without cinema date", GateInCinemas, 0, "", "2026-04-16", "", "digital", "2026-04-16"},
		{"digital before physical", GateReleased, 0, "2026-03-01", "2026-04-16", "2026-05-14", "digital", "2026-04-16"},
		{"physical before digital", GateReleased, 0, "2026-03-01", "2026-06-01", "2026-05-14", "physical", "2026-05-14"},
		{"same day picks digital", GateReleased, 0, "", "2026-05-14", "2026-05-14", "digital", "2026-05-14"},
		{"physical only", GateReleased, 0, "", "", "2026-05-14", "physical", "2026-05-14"},
		{"delay days", GateReleased, 7, "2026-03-01", "2026-04-16", "", "digital", "2026-04-23"},
		{"delay on cinema", GateInCinemas, 3, "2026-03-01", "", "", "cinema", "2026-03-04"},
		{"delay on cinema plus 90", GateReleased, 10, "2026-03-01", "", "", "cinema_plus_90", "2026-06-09"},
		{"announced ignores dates and delay", GateAnnounced, 30, "2030-01-01", "", "", "announced", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			drv, at := Gate(c.gate, c.delay, Dates{Cinema: d(c.cinema), Digital: d(c.digital), Physical: d(c.physical)})
			got := ""
			if at != nil {
				got = at.Format("2006-01-02")
			}
			if drv != c.driver || got != c.opens {
				t.Fatalf("got %s %q, want %s %q", drv, got, c.driver, c.opens)
			}
		})
	}
}

func TestGateOpen(t *testing.T) {
	now := *d("2026-04-16")
	if !GateOpen("announced", nil, now) || GateOpen("none", nil, now) {
		t.Fatal("announced is always open, none never")
	}
	if !GateOpen("digital", d("2026-04-16"), now) || GateOpen("digital", d("2026-04-17"), now) {
		t.Fatal("opens on the day, not before")
	}
}

func TestStage(t *testing.T) {
	ds := Dates{Cinema: d("2026-03-01"), Digital: d("2026-04-16")}
	for now, want := range map[string]string{"2026-02-01": StageAnnounced, "2026-03-01": StageInCinemas, "2026-04-15": StageInCinemas, "2026-04-16": StageReleased} {
		if got := Stage(ds, *d(now)); got != want {
			t.Errorf("%s: %s, want %s", now, got, want)
		}
	}
	if Stage(Dates{}, *d("2026-01-01")) != StageAnnounced {
		t.Error("no dates is announced")
	}
}
