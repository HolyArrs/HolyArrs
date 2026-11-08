package metadata

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// Stand-ins for the trailer reader, for the browser tests. Unset, nothing
// changes: the reader asks YouTube, and the trailer pass keeps its own
// schedule.
const (
	// EnvYouTubeURL points the trailer reader at a stand-in YouTube: https
	// anywhere, plain http only on this machine.
	EnvYouTubeURL = "HOLYARRS_YOUTUBE_URL"
	// EnvTrailerPassFirst and EnvTrailerPassEvery are durations (such as 2s):
	// when the trailer pass first runs after start, and how often after that.
	EnvTrailerPassFirst = "HOLYARRS_TRAILER_PASS_FIRST"
	EnvTrailerPassEvery = "HOLYARRS_TRAILER_PASS_EVERY"
)

// TrailerStandIns applies the stand-ins named in the environment; a value
// that does not fit stops the start with a message.
func TrailerStandIns() error {
	if raw := strings.TrimSpace(os.Getenv(EnvYouTubeURL)); raw != "" {
		base, err := youtubeStandIn(raw)
		if err != nil {
			return err
		}
		UseYouTube(base)
	}
	first, err := passDuration(EnvTrailerPassFirst)
	if err != nil {
		return err
	}
	every, err := passDuration(EnvTrailerPassEvery)
	if err != nil {
		return err
	}
	TrailerPassTiming(first, every)
	return nil
}

// youtubeStandIn checks a stand-in YouTube address: http or https with a
// host and nothing after the path, and plain http only for this machine.
func youtubeStandIn(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%s must look like https://host or http://127.0.0.1:port", EnvYouTubeURL)
	}
	if u.Scheme == "http" && !loopbackName(strings.ToLower(u.Hostname())) {
		return "", fmt.Errorf("%s may use plain http only for this machine (127.0.0.1, ::1 or localhost)", EnvYouTubeURL)
	}
	return strings.TrimRight(raw, "/"), nil
}

// passDuration reads one trailer pass duration (0 when unset).
func passDuration(name string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, errors.New(name + " must be a duration such as 2s or 5m")
	}
	return d, nil
}

// loopbackName says whether a host is this machine.
func loopbackName(h string) bool {
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return h == "localhost"
}
