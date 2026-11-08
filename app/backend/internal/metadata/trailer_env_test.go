package metadata

import (
	"strings"
	"testing"
	"time"
)

// A stand-in YouTube is http or https; plain http only on this machine.
func TestYouTubeStandInAddress(t *testing.T) {
	for _, c := range []struct {
		raw, want string
		ok        bool
	}{
		{"http://127.0.0.1:18744/youtube", "http://127.0.0.1:18744/youtube", true},
		{"http://127.0.0.1:18744/youtube/", "http://127.0.0.1:18744/youtube", true},
		{"http://localhost:18744", "http://localhost:18744", true},
		{"http://LOCALHOST:18744", "http://LOCALHOST:18744", true},
		{"http://[::1]:18744", "http://[::1]:18744", true},
		{"https://yt.example.com", "https://yt.example.com", true},
		{"https://192.0.2.10:8443/yt", "https://192.0.2.10:8443/yt", true},
		{"http://192.0.2.10:18744", "", false},
		{"http://yt.example.com", "", false},
		{"http://127.0.0.1.example.com", "", false},
		{"ftp://127.0.0.1/yt", "", false},
		{"file:///c:/yt", "", false},
		{"127.0.0.1:18744", "", false},
		{"http://user:pw@127.0.0.1:18744", "", false},
		{"http://127.0.0.1:18744/?v=1", "", false},
		{"http://127.0.0.1:18744/#x", "", false},
		{"https://", "", false},
	} {
		got, err := youtubeStandIn(c.raw)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%q: got %q, %v; want %q, ok %v", c.raw, got, err, c.want, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), EnvYouTubeURL) {
			t.Errorf("%q: the message %q does not name %s", c.raw, err, EnvYouTubeURL)
		}
	}
}

// The trailer pass durations: unset is 0 (the pass keeps its schedule);
// anything but a positive duration is refused with the variable's name.
func TestTrailerPassDurations(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"", 0, true},
		{"  ", 0, true},
		{"2s", 2 * time.Second, true},
		{" 1m30s ", 90 * time.Second, true},
		{"500ms", 500 * time.Millisecond, true},
		{"0s", 0, false},
		{"-2s", 0, false},
		{"2", 0, false},
		{"soon", 0, false},
	} {
		t.Setenv(EnvTrailerPassEvery, c.raw)
		got, err := passDuration(EnvTrailerPassEvery)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%q: got %v, %v; want %v, ok %v", c.raw, got, err, c.want, c.ok)
		}
		if err != nil && !strings.Contains(err.Error(), EnvTrailerPassEvery) {
			t.Errorf("%q: the message %q does not name %s", c.raw, err, EnvTrailerPassEvery)
		}
	}
}

// TrailerStandIns stops the start on a value that does not fit, and takes
// good ones (nothing set changes nothing).
func TestTrailerStandIns(t *testing.T) {
	t.Cleanup(func() { UseYouTube("") })
	for _, c := range []struct {
		url, first, every string
		bad               string // the variable the refusal names ("" when none)
	}{
		{"", "", "", ""},
		{"http://127.0.0.1:18744/youtube", "", "", ""},
		{"http://192.0.2.10:18744", "", "", EnvYouTubeURL},
		{"", "soon", "", EnvTrailerPassFirst},
		{"", "", "-1s", EnvTrailerPassEvery},
	} {
		t.Setenv(EnvYouTubeURL, c.url)
		t.Setenv(EnvTrailerPassFirst, c.first)
		t.Setenv(EnvTrailerPassEvery, c.every)
		err := TrailerStandIns()
		switch {
		case c.bad == "" && err != nil:
			t.Errorf("%+v: %v", c, err)
		case c.bad != "" && (err == nil || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%+v: got %v, want a refusal naming %s", c, err, c.bad)
		}
	}
}
