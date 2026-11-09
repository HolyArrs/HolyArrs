package subtitles

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// JSON goldens for the records the subtitle routes return. Set
// PIN_UPDATE_GOLDEN=1 to rewrite one on purpose.

func subPinGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE_GOLDEN") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Fatalf("%s changed\n got: %s\nwant: %s", name, got, want)
	}
}

func TestSubPinRecordGoldens(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	meta := domain.Meta{ID: 5, CreatedAt: at, CreatedBy: domain.Actor{Kind: "user", ID: 2}, UpdatedAt: at, UpdatedBy: domain.System}
	idx, site, score := 2, int64(3), 88
	codec, rel, name, key := "subrip", "a.en.srt", "Lab", "k1"
	subPinGolden(t, "pin_subtitle.golden.json", Subtitle{Meta: meta, FileID: 9, Language: "en", Forced: true, HearingImpaired: true, Place: "beside", TrackIndex: &idx,
		Codec: &codec, RelativePath: &rel, SiteID: &site, SiteName: &name, SiteKey: &key, Score: &score,
		ScoreParts: []Part{{Name: "title", Points: 40, Matched: true}}, Origin: "fetched", State: "present", SavedAt: &at, Tries: 2,
		FirstTriedAt: &at, LastTriedAt: &at, NextTryAt: &at, Sync: map[string]any{"offset_ms": 250}})
	subPinGolden(t, "pin_subtitle_wish_row.golden.json", Subtitle{Meta: meta, FileID: 9, Language: "fr", Place: "wish", ScoreParts: []Part{}, Origin: "found", State: "wanted", Sync: map[string]any{}})
	reason := "paused"
	subPinGolden(t, "pin_site_row.golden.json", SiteRow{Meta: meta, Name: "Lab", Kind: "fake", URL: "http://x", State: "paused", PauseReason: &reason, PausedUntil: &at, Failures: 1,
		Limits: SiteLimits{GapMS: 1, PerWindow: 2, WindowSeconds: 3, DownloadsToDay: 4}, DownloadsToday: 5, DownloadsResetAt: &at, AllowLocalNetwork: true, Priority: 6, HasAccount: true})
	it, ver := int64(4), int64(5)
	subPinGolden(t, "pin_wish.golden.json", Wish{Meta: meta, TitleID: 3, VersionID: &ver, ItemID: &it, Languages: []Want{{Language: "en", HearingImpaired: HINormal, Forced: false}}})
	subPinGolden(t, "pin_settings_default.golden.json", DefaultSettings())
	season, ep := 1, 2
	subPinGolden(t, "pin_missing_row.golden.json", MissingRow{FileID: 9, ItemID: 4, TitleID: 3, Title: "Show", Kind: "episode", Season: &season, Episode: &ep, Language: "en", Forced: true,
		HI: true, State: "wanted", Tries: 2, NextTryAt: &at})
	subPinGolden(t, "pin_missing_row_movie.golden.json", MissingRow{FileID: 9, ItemID: 4, TitleID: 3, Title: "Film", Kind: "movie", Language: "en", State: "none_good_enough"})
	subPinGolden(t, "pin_missing_title.golden.json", MissingTitle{ID: 3, Name: "Show", Kind: "show", Count: 7})
	subPinGolden(t, "pin_status.golden.json", Status{FileID: 9, Wish: Resolved{Wants: []Want{{Language: "en", HearingImpaired: HINormal}}, From: FromDefault},
		Haves: []Have{{Language: "en", Forced: false, HearingImpaired: true}}, Missing: []Want{{Language: "fr", HearingImpaired: HIEither, Forced: true}}})
}
