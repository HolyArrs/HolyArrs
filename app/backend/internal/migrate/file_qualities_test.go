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

// The old app's episodes, from the API or a backup's Episodes table, go
// with the file that holds them; one with no file is left out.
func TestOldEpisodesByFile(t *testing.T) {
	var api []any
	_ = json.Unmarshal([]byte(`[{"seasonNumber":1,"episodeNumber":1,"episodeFileId":9,"airDate":"2022-09-20","title":"In the Beginnings"},
		{"seasonNumber":1,"episodeNumber":2,"episodeFileId":0,"airDate":"2022-09-21","title":"Faith/Off"}]`), &api)
	got := oldEpisodesByFile(api)
	if len(got) != 1 || len(got[9]) != 1 || got[9][0] != (OldEpisode{Season: 1, Number: 1, AirDate: "2022-09-20", Title: "In the Beginnings"}) {
		t.Fatalf("api %+v", got)
	}
	backup := oldEpisodesByFile([]any{map[string]any{"seasonnumber": int64(1), "episodenumber": int64(3), "episodefileid": int64(4), "airdate": "2022-09-22T00:00:00Z", "title": "Renaissance Woman"}})
	if e := backup[4]; len(e) != 1 || e[0].AirDate != "2022-09-22" || e[0].Number != 3 {
		t.Fatalf("backup %+v", backup)
	}
	files := filesByTitle([]map[string]any{{"titleid": int64(2), "relativepath": "a.mkv", "quality": "", "episodes": got[9]}})
	if len(files[2]) != 1 || len(files[2][0].Episodes) != 1 {
		t.Fatalf("files %+v", files)
	}
}

func TestFileFactsSkipsRepeatedBaseNames(t *testing.T) {
	a := []OldEpisode{{Season: 1, Number: 1, AirDate: "2022-09-27", Title: "Gods"}}
	b := []OldEpisode{{Season: 2, Number: 1, AirDate: "2023-01-01", Title: "Other"}}
	_, eps := fileFacts([]TitleFile{{Name: "Episode 01.mkv", Episodes: a}, {Name: "Episode 01.mkv", Episodes: b}, {Name: "x.mkv", Episodes: a}})
	if _, ok := eps["Episode 01.mkv"]; ok || len(eps["x.mkv"]) != 1 {
		t.Fatalf("eps = %v", eps)
	}
}
