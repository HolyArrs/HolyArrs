package main

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// TestMain keeps the update check off the real network: the release feed
// points at a loopback address the public-only policy refuses before dialing,
// and the artist picture source is off. Held-back features are tested
// switched on, the music cover preload runs only where a test asks, and
// YouTube is never asked (fakeReader stands in). A full run fails when a
// registered route was never served to a test request.
func TestMain(m *testing.M) {
	_ = os.Setenv("HOLYARRS_RELEASE_FEED", "http://127.0.0.1:1/no-network-in-tests")
	_ = os.Setenv("HOLYARRS_DEEZER_URL", "off") // no artist pictures from the real Deezer
	startup.RegisterHeldFeatures()
	parts.Register()
	registerHooks()
	_ = os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	wrapMux = webserver.RecordRouteHits
	music.SetPreloadOn(false)
	metadata.UseReader(&youtube.Reader{Base: "http://127.0.0.1:1"})
	os.Exit(webserver.CheckRouteHits(m.Run(), "."))
}
