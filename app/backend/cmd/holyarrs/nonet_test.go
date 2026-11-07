package main

import (
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/feature"
	"os"
	"testing"
)

// TestMain keeps the update check off the real network: the release feed
// points at a loopback address the public-only policy refuses before dialing,
// and the artist picture source is off.
func TestMain(m *testing.M) {
	_ = os.Setenv("HOLYARRS_RELEASE_FEED", "http://127.0.0.1:1/no-network-in-tests")
	_ = os.Setenv("HOLYARRS_DEEZER_URL", "off") // no artist pictures from the real Deezer
	// Held-back features are tested switched on (internal/feature).
	_ = os.Setenv("HOLYARRS_FLAGS", strings.Join(feature.Held, ","))
	os.Exit(m.Run())
}
