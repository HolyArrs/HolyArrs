package migrate

import (
	"encoding/json"
	"testing"
)

func TestQualityNameFromAPIAndBackup(t *testing.T) {
	var api map[string]any
	_ = json.Unmarshal([]byte(`{"quality":{"id":3,"name":"WEBDL-1080p","source":"web","resolution":1080},"revision":{"version":1}}`), &api)
	if got := qualityName(api, nil); got != "WEBDL-1080p" {
		t.Fatalf("API shape: %q", got)
	}
	defs := map[int64]string{7: "Bluray-1080p"}
	if got := qualityName(`{"quality": 7, "revision": {"version": 1}}`, defs); got != "Bluray-1080p" {
		t.Fatalf("backup shape: %q", got)
	}
	if got := qualityName("not json", defs); got != "" {
		t.Fatalf("junk: %q", got)
	}
}

func TestFilesByTitleKeysOnBaseName(t *testing.T) {
	got := filesByTitle([]map[string]any{
		{"titleid": int64(4), "relativepath": `Season 01\The Rookie - S01E01 - Pilot.mkv`, "quality": "WEBDL-1080p"},
		{"titleid": int64(4), "relativepath": "Season 01/The Rookie - S01E02 - Crash Course.mkv", "quality": "HDTV-720p"},
		{"titleid": int64(4), "relativepath": "Season 01/no quality.mkv", "quality": ""},
	})
	files := got[4]
	if len(files) != 2 || files[0].Name != "The Rookie - S01E01 - Pilot.mkv" || files[1].Quality != "HDTV-720p" {
		t.Fatalf("got %+v", files)
	}
}
