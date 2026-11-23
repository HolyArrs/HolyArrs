package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A hook with every switch on, as a phone app such as Helmarr adds it,
// gets each kind of Event in the old apps' shape; a hook with imports
// alone, as Pulsarr adds it, gets none of them. A health warning goes out
// only with includeHealthWarnings on.
func TestHookEvents_AllKinds(t *testing.T) {
	h := newHookRig(t)
	l := h.library()
	rc := newReceiver(t)
	allOn := func(m map[string]any) {
		for k := range m {
			if strings.HasPrefix(k, "on") {
				m[k] = true
			}
		}
		for _, k := range []string{"onMovieAdded", "onMovieDelete", "onMovieFileDelete", "onMovieFileDeleteForUpgrade"} {
			m[k] = true // the Radarr side's own switches
		}
		m["includeHealthWarnings"] = false
	}
	h.addHook("sonarr", "Helmarr (Sonarr)", rc.srv.URL+"/sonarr", "s", allOn)
	h.addHook("radarr", "Helmarr (Radarr)", rc.srv.URL+"/radarr", "s", allOn)
	h.addHook("sonarr", "pulsarr", rc.srv.URL+"/pulsarr", "s", nil)
	tests := len(rc.got())
	s := h.sender(true)
	h.placed(placeStep{slots: []int64{l.slots["movie"]}, version: l.movieVer, path: "Lab Movie (2024)/Lab.Movie.2024.mkv", placed: true})
	var movieFile, e2Item, e3Item int64
	_ = h.st.Read().QueryRow("SELECT file_id FROM slots WHERE id = ?", l.slots["movie"]).Scan(&movieFile)
	_ = h.st.Read().QueryRow("SELECT item_id FROM slots WHERE id = ?", l.slots["e2"]).Scan(&e2Item)
	_ = h.st.Read().QueryRow("SELECT item_id FROM slots WHERE id = ?", l.slots["e3"]).Scan(&e3Item)
	e1 := l.slots["e1"]
	emit := func(e domain.Event) {
		t.Helper()
		if err := h.st.AddEvent(context.Background(), &e); err != nil {
			t.Fatal(err)
		}
		s.wait()
	}
	emit(domain.Event{Kind: "download_sent", TitleID: &l.show, SlotID: &e1, Data: map[string]any{"release": "Lab.Show.S01E01.1080p.WEB-DL.x264-GRP", "downloader": "qBittorrent"}})
	emit(domain.Event{Kind: domain.EventTitleAdded, TitleID: &l.movie})
	emit(domain.Event{Kind: domain.EventRenamed, TitleID: &l.movie, Data: map[string]any{"file_id": movieFile, "from": "Lab Movie (2024)/old.mkv", "to": "Lab Movie (2024)/Lab.Movie.2024.mkv"}})
	emit(domain.Event{Kind: domain.EventHealthBroken, Data: map[string]any{"kind": "disk", "state": "warning", "message": "low"}})
	emit(domain.Event{Kind: domain.EventHealthBroken, Data: map[string]any{"kind": "downloader", "state": "error", "message": "qBittorrent is down"}})
	emit(domain.Event{Kind: domain.EventHealthRecovered, Data: map[string]any{"kind": "downloader", "state": "ok", "was": "error"}})
	emit(domain.Event{Kind: domain.EventNeedsDecision, TitleID: &l.show, SlotID: &e1, Data: map[string]any{"reason": "the file is a sample"}})
	emit(domain.Event{Kind: "file_replaced", TitleID: &l.movie, Data: map[string]any{"old_file_id": 99, "old_release": "Lab.Movie.2024.720p"}})
	emit(domain.Event{Kind: domain.EventEpisodesRemoved, TitleID: &l.show, Data: map[string]any{"item_ids": []any{e3Item, e2Item}, "delete_files": true}})
	emit(domain.Event{Kind: domain.EventTitleRemoved, TitleID: &l.show, Data: map[string]any{"delete_files": true}})

	var got []string
	bodies := map[string]map[string]any{}
	for _, c := range rc.got()[tests:] {
		if c.uri == "/pulsarr" {
			t.Fatalf("an imports-only hook got %v", c.body)
		}
		k := c.uri + " " + fmt.Sprint(c.body["eventType"])
		got = append(got, k)
		bodies[k] = c.body
	}
	want := "/sonarr Grab,/radarr MovieAdded,/radarr Rename,/sonarr Health,/radarr Health,/sonarr HealthRestored,/radarr HealthRestored," +
		"/sonarr ManualInteractionRequired,/radarr MovieFileDelete,/sonarr EpisodeFileDelete,/sonarr SeriesDelete"
	// The two sides of a health event go out at once, in either order.
	norm := func(s []string) string {
		out := strings.Join(s, ",")
		for _, p := range [][2]string{{"/radarr Health,/sonarr Health,", "/sonarr Health,/radarr Health,"}, {"/radarr HealthRestored,/sonarr HealthRestored,", "/sonarr HealthRestored,/radarr HealthRestored,"}} {
			out = strings.Replace(out, p[0], p[1], 1)
		}
		return out
	}
	if norm(got) != want {
		t.Fatalf("events:\n got %s\nwant %s", norm(got), want)
	}

	g := bodies["/sonarr Grab"]
	rel := g["release"].(map[string]any)
	eps := g["episodes"].([]any)
	if rel["quality"] != "WEBDL-1080p" || rel["releaseGroup"] != "GRP" || rel["releaseTitle"] != "Lab.Show.S01E01.1080p.WEB-DL.x264-GRP" || g["downloadClient"] != "qBittorrent" ||
		len(eps) != 1 || eps[0].(map[string]any)["episodeNumber"].(float64) != 1 || g["series"].(map[string]any)["tmdbId"].(float64) != 100 {
		t.Fatalf("grab: %v", g)
	}
	mv := bodies["/radarr MovieAdded"]["movie"].(map[string]any)
	if mv["tmdbId"].(float64) != 693134 || mv["year"].(float64) != 2024 || !strings.HasSuffix(mv["folderPath"].(string), "/Lab Movie (2024)") {
		t.Fatalf("movie: %v", mv)
	}
	rn := bodies["/radarr Rename"]["renamedMovieFiles"].([]any)[0].(map[string]any)
	if rn["previousRelativePath"] != "old.mkv" || rn["relativePath"] != "Lab.Movie.2024.mkv" || rn["quality"] != "WEBDL-1080p" {
		t.Fatalf("rename: %v", rn)
	}
	if hb := bodies["/sonarr Health"]; hb["level"] != "error" || hb["message"] != "qBittorrent is down" || hb["type"] != "downloader" {
		t.Fatalf("health: %v", hb)
	}
	mi := bodies["/sonarr ManualInteractionRequired"]
	if msgs := fmt.Sprint(mi["downloadStatusMessages"]); !strings.Contains(msgs, "the file is a sample") || len(mi["episodes"].([]any)) != 1 {
		t.Fatalf("manual interaction: %v", mi)
	}
	if fd := bodies["/radarr MovieFileDelete"]; fd["deleteReason"] != "upgrade" || fd["movieFile"].(map[string]any)["sceneName"] != "Lab.Movie.2024.720p" {
		t.Fatalf("file delete for upgrade: %v", fd)
	}
	ed := bodies["/sonarr EpisodeFileDelete"]
	if ed["deleteReason"] != "manual" || len(ed["episodes"].([]any)) != 2 || ed["episodes"].([]any)[0].(map[string]any)["episodeNumber"].(float64) != 2 {
		t.Fatalf("episodes deleted: %v", ed)
	}
	if sd := bodies["/sonarr SeriesDelete"]; sd["deletedFiles"] != true {
		t.Fatalf("series delete: %v", sd)
	}

	// With includeHealthWarnings on, a warning goes out too.
	h.addHook("radarr", "warnings", rc.srv.URL+"/warn", "s", func(m map[string]any) { m["onHealthIssue"], m["includeHealthWarnings"] = true, true })
	tests = len(rc.got())
	emit(domain.Event{Kind: domain.EventHealthBroken, Data: map[string]any{"kind": "disk", "state": "warning", "message": "low"}})
	n := 0
	for _, c := range rc.got()[tests:] {
		if c.uri == "/warn" && c.body["level"] == "warning" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("warning sent %d times to the hook that takes warnings", n)
	}
}

// With forceSave, as Helmarr sends it, a hook is saved without the test
// call, so a push server that refuses test events does not block it; a
// change with forceSave is saved the same way.
func TestHooks_ForceSaveSkipsTest(t *testing.T) {
	h := newHookRig(t)
	rc := newReceiver(t)
	rc.answer(401)
	st, body := h.do("radarr", "POST", "notification?forceSave=true", pulsarrHook(0, "Helmarr (Radarr)", rc.srv.URL+"/push", ""))
	if st != 201 || len(rc.got()) != 0 {
		t.Fatalf("forced add: %d %s, %d calls", st, body, len(rc.got()))
	}
	var added map[string]any
	_ = json.Unmarshal([]byte(body), &added)
	id := fmt.Sprint(int64(added["id"].(float64)))
	added["onGrab"] = true
	b, _ := json.Marshal(added)
	if st, body := h.do("radarr", "PUT", "notification/"+id+"?forceSave=true", string(b)); st != 202 || len(rc.got()) != 0 {
		t.Fatalf("forced change: %d %s, %d calls", st, body, len(rc.got()))
	}
	// Without forceSave the test still runs and a refusal still stops it.
	st, body = h.do("radarr", "POST", "notification", pulsarrHook(0, "other", rc.srv.URL+"/push", ""))
	failedOn(t, st, body, "Url")
}
