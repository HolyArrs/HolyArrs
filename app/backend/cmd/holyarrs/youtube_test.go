package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	ytlib "github.com/HolyArrs/holyarrs/app/backend/internal/youtube"
	"github.com/HolyArrs/holyarrs/app/backend/internal/youtube/youtubetest"
)

// ytJoin signs a new person in through an invite made with invite (the
// invite's JSON body).
func ytJoin(t *testing.T, r *askRig, name, invite string) *signedIn {
	t.Helper()
	var inv struct {
		Path string `json:"path"`
	}
	decodeJSON(t, r.owner.call("POST", "/api/v1/users/invites", invite, 201), &inv)
	jar, _ := cookiejar.New(nil)
	b := &signedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jar}}
	var x struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(inv.Path, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201), &x)
	b.csrf = x.CSRF
	var me struct {
		ID int64 `json:"id"`
	}
	decodeJSON(t, b.call("GET", "/api/v1/auth/me", "", 200), &me)
	b.id = me.ID
	return b
}

type ytStatus struct {
	Missing    []string `json:"missing"`
	FolderSet  bool     `json:"folder_set"`
	FloorBytes int64    `json:"floor_bytes"`
}

// route: GET /api/v1/youtube/status
// route: GET /api/v1/settings/youtube
// route: PUT /api/v1/settings/youtube
func TestYouTubeSettingsAndStatusRoutes(t *testing.T) {
	fake := youtubetest.NewFake(t, "ok")
	t.Setenv("HOLYARRS_YTDLP", fake.Bin)
	t.Setenv("HOLYARRS_FFMPEG", fake.Bin)
	r := newAskRig(t)
	folder := t.TempDir()

	r.owner.call("PUT", "/api/v1/settings/youtube", `{"folder":`+jsonString(folder+"/")+`,"min_free_gb":5}`, 200)
	var got struct {
		Folder    string `json:"folder"`
		MinFreeGB int    `json:"min_free_gb"`
		Totals    struct {
			Videos int `json:"videos"`
		} `json:"totals"`
		Version    string `json:"version"`
		YTDLPPath  string `json:"ytdlp_path"`
		FFmpegPath string `json:"ffmpeg_path"`
	}
	decodeJSON(t, r.owner.call("GET", "/api/v1/settings/youtube", "", 200), &got)
	if got.Folder != filepath.Clean(folder) || got.MinFreeGB != 5 || got.Totals.Videos != 0 || got.Version != "2026.10.26" || got.YTDLPPath != fake.Bin || got.FFmpegPath != fake.Bin {
		t.Fatalf("settings %+v", got)
	}
	// A field left out keeps its value; a bad folder or floor is refused.
	r.owner.call("PUT", "/api/v1/settings/youtube", `{"min_free_gb":7}`, 200)
	decodeJSON(t, r.owner.call("GET", "/api/v1/settings/youtube", "", 200), &got)
	if got.Folder != filepath.Clean(folder) || got.MinFreeGB != 7 {
		t.Fatalf("partial save %+v", got)
	}
	r.owner.call("PUT", "/api/v1/settings/youtube", `{"folder":"relative/path"}`, 400)
	r.owner.call("PUT", "/api/v1/settings/youtube", `{"folder":`+jsonString(filepath.Join(folder, "missing"))+`}`, 400)
	r.owner.call("PUT", "/api/v1/settings/youtube", `{"min_free_gb":0}`, 400)

	// Admins see the card but may not change it.
	r.owner.call("PUT", "/api/v1/users/"+itoa(r.bo.id), `{"role":"admin"}`, 200)
	r.bo.call("GET", "/api/v1/settings/youtube", "", 200)
	r.bo.call("PUT", "/api/v1/settings/youtube", `{"min_free_gb":9}`, 403)
	r.bo.call("GET", "/api/v1/youtube/status", "", 403) // not ticked

	// A member needs the youtube permission.
	r.ana.call("GET", "/api/v1/youtube/status", "", 403)
	r.ana.call("GET", "/api/v1/settings/youtube", "", 403)
	r.owner.call("PUT", "/api/v1/users/"+itoa(r.ana.id), `{"perms":["ask","youtube"]}`, 200)
	var st ytStatus
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/status", "", 200), &st)
	if st.Missing == nil || len(st.Missing) != 0 || !st.FolderSet || st.FloorBytes != 7<<30 {
		t.Fatalf("status %+v", st)
	}

	// A kid gets nothing, ticked or not. The users screen refuses the tick
	// for a kid, so it goes straight into the row.
	kai := ytJoin(t, r, "kai", `{"max_rating":"PG"}`)
	kai.call("GET", "/api/v1/youtube/status", "", 403)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","youtube","save_files"]' WHERE id = ?`, kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kai.call("GET", "/api/v1/youtube/status", "", 403)
	var me struct {
		Perms []string `json:"perms"`
	}
	decodeJSON(t, kai.call("GET", "/api/v1/auth/me", "", 200), &me)
	if !slices.Contains(me.Perms, "ask") || slices.Contains(me.Perms, "youtube") || slices.Contains(me.Perms, "save_files") {
		t.Fatalf("kid perms %v", me.Perms)
	}

	// A program path pointing nowhere counts as missing.
	t.Setenv("HOLYARRS_YTDLP", filepath.Join(t.TempDir(), "nowhere", "yt-dlp"))
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/status", "", 200), &st)
	if !slices.Equal(st.Missing, []string{"yt-dlp"}) {
		t.Fatalf("missing %v", st.Missing)
	}
	decodeJSON(t, r.owner.call("GET", "/api/v1/settings/youtube", "", 200), &got)
	if got.Version != "" || got.YTDLPPath != "" || got.FFmpegPath != fake.Bin {
		t.Fatalf("programs without yt-dlp: %+v", got)
	}
}

func TestYouTubeRoutesAbsentWhenFlagOff(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	addr, _, _, stop := runApp(t, config{ConfigDir: t.TempDir()})
	defer func() { _ = stop() }()
	jar, _ := cookiejar.New(nil)
	own := &signedIn{t: t, base: "http://" + addr, c: &http.Client{Jar: jar}}
	var x struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, own.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201), &x)
	own.csrf = x.CSRF
	own.call("GET", "/api/v1/youtube/status", "", 404)
	own.call("GET", "/api/v1/settings/youtube", "", 404)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type ytVideo struct {
	ID       int64  `json:"id"`
	VideoID  string `json:"video_id"`
	Title    string `json:"title"`
	State     string  `json:"state"`
	FollowIDs []int64 `json:"follow_ids"`
	Manual    bool    `json:"manual"`
}

type ytFollow struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	YTID      string `json:"yt_id"`
	Title     string `json:"title"`
	Backfill  *int   `json:"backfill"`
	KeepLast  *int   `json:"keep_last"`
	MaxHeight int    `json:"quality_max_height"`
	Enabled   bool   `json:"enabled"`
	Mirror    bool   `json:"mirror"`
}

// ytRig is an askRig with the fake yt-dlp, a YouTube folder and ana ticked
// for youtube.
func ytRig(t *testing.T) *askRig {
	t.Helper()
	fake := youtubetest.NewFake(t, "ok")
	t.Setenv("HOLYARRS_YTDLP", fake.Bin)
	t.Setenv("HOLYARRS_FFMPEG", fake.Bin)
	r := newAskRig(t)
	r.owner.call("PUT", "/api/v1/settings/youtube", `{"folder":`+jsonString(t.TempDir())+`,"min_free_gb":1}`, 200)
	r.owner.call("PUT", "/api/v1/users/"+itoa(r.ana.id), `{"perms":["ask","youtube"]}`, 200)
	return r
}

// route: POST /api/v1/youtube/probe
// route: POST /api/v1/youtube/videos
// route: GET /api/v1/youtube/videos
// route: GET /api/v1/youtube/videos/{id}
// route: DELETE /api/v1/youtube/videos/{id}
// route: GET /api/v1/youtube/follows
// route: POST /api/v1/youtube/follows
// route: PUT /api/v1/youtube/follows/{id}
// route: DELETE /api/v1/youtube/follows/{id}
func TestYouTubeAddRoutes(t *testing.T) {
	r := ytRig(t)
	var p struct {
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Count   int    `json:"count"`
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/probe", `{"url":"https://www.youtube.com/@veritasium"}`, 200), &p)
	if p.Kind != "channel" || p.Title != "Veritasium" || p.Count != 0 || len(p.Entries) != 2 {
		t.Fatalf("probe %+v", p)
	}
	body := r.ana.call("POST", "/api/v1/youtube/probe", `{"url":"https://evil.example/watch?v=dQw4w9WgXcQ"}`, 400)
	if !strings.Contains(string(body), "paste a link to a YouTube video, playlist or channel") {
		t.Fatalf("bad link: %s", body)
	}

	var v ytVideo
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/videos", `{"url":"https://youtu.be/dQw4w9WgXcQ"}`, 201), &v)
	if v.VideoID != "dQw4w9WgXcQ" || v.State != "queued" || v.Title != "Never Gonna Give You Up" || !v.Manual || v.FollowIDs == nil || len(v.FollowIDs) != 0 {
		t.Fatalf("video %+v", v)
	}
	r.ana.call("POST", "/api/v1/youtube/videos", `{"url":"https://www.youtube.com/@veritasium"}`, 400)

	var f ytFollow
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/@veritasium","keep_last":5}`, 201), &f)
	if f.Kind != "channel" || f.YTID != "UCHnyfMqiRRG1u-2MsSQLbXA" || f.Backfill == nil || *f.Backfill != 5 || f.MaxHeight != 1080 || !f.Enabled {
		t.Fatalf("follow %+v", f)
	}
	var dup struct {
		Error      string `json:"error"`
		ExistingID int64  `json:"existing_id"`
	}
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/channel/UCHnyfMqiRRG1u-2MsSQLbXA"}`, 409), &dup)
	if dup.Error != "you already follow this" || dup.ExistingID != f.ID {
		t.Fatalf("duplicate %+v", dup)
	}

	var follows []ytFollow
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/follows", "", 200), &follows)
	if len(follows) != 1 || follows[0].ID != f.ID {
		t.Fatalf("follows %+v", follows)
	}
	decodeJSON(t, r.ana.call("PUT", "/api/v1/youtube/follows/"+itoa(f.ID), `{"enabled":false,"keep_last":null,"quality_max_height":720}`, 200), &f)
	if f.Enabled || f.KeepLast != nil || f.MaxHeight != 720 {
		t.Fatalf("updated %+v", f)
	}
	r.ana.call("PUT", "/api/v1/youtube/follows/"+itoa(f.ID), `{"keep_last":0}`, 400)
	// Only a playlist can be mirrored, and a mirror keeps every video.
	body = r.ana.call("PUT", "/api/v1/youtube/follows/"+itoa(f.ID), `{"mirror":true}`, 400)
	if !strings.Contains(string(body), "only a playlist can be mirrored") {
		t.Fatalf("mirrored channel: %s", body)
	}
	r.ana.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/playlist?list=PLtestlist01","mirror":true,"keep_last":3}`, 400)
	var pl ytFollow
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/playlist?list=PLtestlist01","mirror":true}`, 201), &pl)
	if pl.Kind != "playlist" || !pl.Mirror || pl.Backfill != nil {
		t.Fatalf("mirrored playlist %+v", pl)
	}
	decodeJSON(t, r.ana.call("PUT", "/api/v1/youtube/follows/"+itoa(pl.ID), `{"mirror":false}`, 200), &pl)
	if pl.Mirror {
		t.Fatalf("mirror off %+v", pl)
	}
	r.ana.call("DELETE", "/api/v1/youtube/follows/"+itoa(pl.ID)+"?files=delete", "", 204)

	var list []ytVideo
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/videos?unwatched=1&limit=10&offset=0", "", 200), &list)
	if len(list) != 1 || list[0].ID != v.ID {
		t.Fatalf("list %+v", list)
	}
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/videos?follow="+itoa(f.ID), "", 200), &list)
	if len(list) != 0 {
		t.Fatalf("follow filter %+v", list)
	}
	r.ana.call("GET", "/api/v1/youtube/videos?limit=x", "", 400)
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/videos/"+itoa(v.ID), "", 200), &v)
	if !v.Manual || v.FollowIDs == nil {
		t.Fatalf("one video %+v", v)
	}

	r.ana.call("DELETE", "/api/v1/youtube/follows/"+itoa(f.ID), "", 400)
	r.ana.call("DELETE", "/api/v1/youtube/follows/"+itoa(f.ID)+"?files=keep", "", 204)
	r.ana.call("DELETE", "/api/v1/youtube/follows/"+itoa(f.ID)+"?files=keep", "", 404)
	r.ana.call("DELETE", "/api/v1/youtube/videos/"+itoa(v.ID), "", 204)
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/videos", "", 200), &list)
	if len(list) != 0 {
		t.Fatalf("removed video still listed %+v", list)
	}

	// Without the programs nothing can be looked up.
	t.Setenv("HOLYARRS_FFMPEG", filepath.Join(t.TempDir(), "nowhere"))
	body = r.ana.call("POST", "/api/v1/youtube/probe", `{"url":"https://youtu.be/dQw4w9WgXcQ"}`, 503)
	if !strings.Contains(string(body), "needs yt-dlp and ffmpeg") {
		t.Fatalf("no tools: %s", body)
	}
	// A member without the permission gets nothing.
	r.bo.call("GET", "/api/v1/youtube/videos", "", 403)
}

func TestOtherPersonsRowsAre404(t *testing.T) {
	r := ytRig(t)
	var own, mine ytVideo
	decodeJSON(t, r.owner.call("POST", "/api/v1/youtube/videos", `{"url":"https://youtu.be/dQw4w9WgXcQ"}`, 201), &own)
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/videos", `{"url":"https://www.youtube.com/watch?v=xxxxxxxxxx1"}`, 201), &mine)
	var follow ytFollow
	decodeJSON(t, r.owner.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/@veritasium"}`, 201), &follow)

	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/youtube/videos/" + itoa(own.ID)},
		{"DELETE", "/api/v1/youtube/videos/" + itoa(own.ID)},
		{"PUT", "/api/v1/youtube/follows/" + itoa(follow.ID)},
		{"DELETE", "/api/v1/youtube/follows/" + itoa(follow.ID) + "?files=delete"},
		{"DELETE", "/api/v1/youtube/follows/" + itoa(follow.ID) + "?files=keep"},
	} {
		body := r.ana.call(c.method, c.path, `{"enabled":false}`, 404)
		for _, secret := range []string{"Never Gonna", "Rick Astley", "dQw4w9WgXcQ", "Veritasium", "UCHnyfMqiRRG1u"} {
			if strings.Contains(string(body), secret) {
				t.Fatalf("%s %s leaks %q: %s", c.method, c.path, secret, body)
			}
		}
	}
	r.owner.call("GET", "/api/v1/youtube/videos/"+itoa(mine.ID), "", 404)
	r.owner.call("DELETE", "/api/v1/youtube/videos/"+itoa(mine.ID), "", 404)

	// Nothing changed for the owner.
	var v ytVideo
	decodeJSON(t, r.owner.call("GET", "/api/v1/youtube/videos/"+itoa(own.ID), "", 200), &v)
	var follows []ytFollow
	decodeJSON(t, r.owner.call("GET", "/api/v1/youtube/follows", "", 200), &follows)
	if v.State != "queued" || len(follows) != 1 || !follows[0].Enabled {
		t.Fatalf("owner rows changed: %+v %+v", v, follows)
	}

	var list []ytVideo
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/videos", "", 200), &list)
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("ana sees %+v", list)
	}
	decodeJSON(t, r.ana.call("GET", "/api/v1/youtube/follows", "", 200), &follows)
	if len(follows) != 0 {
		t.Fatalf("ana sees follows %+v", follows)
	}
	// Following the same channel is her own follow, not a duplicate.
	r.ana.call("POST", "/api/v1/youtube/follows", `{"url":"https://www.youtube.com/@veritasium"}`, 201)
}

// The check, download and adopt jobs have handlers only with the youtube
// feature switch on, and all belong to the YouTube part.
func TestYouTubeJobKindsOnlyWithFlag(t *testing.T) {
	for _, flags := range []string{"", "youtube"} {
		t.Setenv("HOLYARRS_FLAGS", flags)
		var kinds []string
		startApp(t, config{}, runOpts{kinds: func(k []string) { kinds = k }})
		check, download, adopt := slices.Contains(kinds, "youtube_check"), slices.Contains(kinds, "youtube_download"), slices.Contains(kinds, "youtube_adopt")
		if on := flags == "youtube"; check != on || download != on || adopt != on {
			t.Fatalf("flags %q: kinds %v", flags, kinds)
		}
		for _, k := range kinds {
			if strings.HasPrefix(k, "youtube_") && parts.JobKinds()[k] != parts.Of(parts.YouTube) {
				t.Errorf("%s belongs to %v", k, parts.JobKinds()[k])
			}
		}
	}
}

// ytDone adds a downloaded video of userID with its file and thumbnail in
// the YouTube folder, as a finished download leaves it.
func ytDone(t *testing.T, r *askRig, userID int64, vid, name, content string) int64 {
	t.Helper()
	ctx := context.Background()
	rel := itoa(userID) + "-p/Chan/" + name
	file := filepath.Join(ytlib.LoadSettings(ctx, r.st.Read()).Folder, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	for f, b := range map[string]string{file: content, strings.TrimSuffix(file, ".mp4") + ".jpg": "\xff\xd8\xff"} {
		if err := os.WriteFile(f, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var id int64
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		if id, _, err = ytlib.InsertVideo(tx, ytlib.Video{UserID: userID, VideoID: vid, State: ytlib.StateQueued, AddedAt: time.Now()}); err != nil {
			return err
		}
		return ytlib.FinishVideo(tx, id, rel, int64(len(content)), ytlib.Meta{Title: "Zażółć"})
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// raw sends a request as s with extra headers and returns the answer and its
// body, without the gzip the Go client adds or undoes on its own.
func (s *signedIn) raw(method, p string, hdr map[string]string) (*http.Response, []byte) {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+p, nil)
	req.Header.Set("Accept-Encoding", "identity")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// route: GET /api/v1/youtube/videos/{id}/file
// route: GET /api/v1/youtube/videos/{id}/thumb
// route: PUT /api/v1/youtube/videos/{id}/position
// route: POST /api/v1/youtube/videos/{id}/retry
func TestYouTubePlayerRoutes(t *testing.T) {
	r := ytRig(t)
	content := strings.Repeat("mp4 bytes ", 300)
	id := ytDone(t, r, r.ana.id, "abcdefghijk", "2024-01-01 Zażółć [abcdefghijk].mp4", content)
	file := "/api/v1/youtube/videos/" + itoa(id) + "/file"

	resp, b := r.ana.raw("GET", file, map[string]string{"Range": "bytes=0-1"})
	if resp.StatusCode != 206 || string(b) != "mp" || resp.Header.Get("Content-Range") != "bytes 0-1/3000" {
		t.Fatalf("range %d %q %v", resp.StatusCode, b, resp.Header)
	}
	resp, b = r.ana.raw("HEAD", file, nil)
	if resp.StatusCode != 200 || len(b) != 0 || resp.Header.Get("Content-Length") != "3000" {
		t.Fatalf("head %d %v", resp.StatusCode, resp.Header)
	}
	resp, b = r.ana.raw("GET", file, map[string]string{"Accept-Encoding": "gzip"})
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Content-Type") != "video/mp4" || string(b) != content {
		t.Fatalf("gzip %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = r.ana.raw("GET", file+"?download=1", nil)
	if cd := resp.Header.Get("Content-Disposition"); resp.StatusCode != 200 || !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "filename*=utf-8''2024-01-01%20Za%C5%BC") {
		t.Fatalf("download %d %q", resp.StatusCode, cd)
	}
	resp, _ = r.ana.raw("GET", "/api/v1/youtube/videos/"+itoa(id)+"/thumb", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb %d %v", resp.StatusCode, resp.Header)
	}

	// Not hers: the owner gets 404 with nothing of the video.
	for _, p := range []string{file, file + "?download=1", "/api/v1/youtube/videos/" + itoa(id) + "/thumb"} {
		resp, b = r.owner.raw("GET", p, nil)
		if resp.StatusCode != 404 || strings.Contains(string(b), "Zażółć") || resp.Header.Get("Content-Disposition") != "" {
			t.Fatalf("owner %s: %d %s", p, resp.StatusCode, b)
		}
	}
	r.owner.call("PUT", "/api/v1/youtube/videos/"+itoa(id)+"/position", `{"seconds":1,"duration":2}`, 404)
	r.owner.call("POST", "/api/v1/youtube/videos/"+itoa(id)+"/retry", "", 404)
	// Without the permission nothing is served.
	if resp, _ = r.bo.raw("GET", file, nil); resp.StatusCode != 403 {
		t.Fatalf("bo %d", resp.StatusCode)
	}
	// A kid is denied, even ticked for youtube and with a video of her own.
	kai := ytJoin(t, r, "kai", `{"max_rating":"PG"}`)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","youtube","save_files"]' WHERE id = ?`, kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kid := ytDone(t, r, kai.id, "kidkidkidki", "2024-01-01 K [kidkidkidki].mp4", "x")
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/youtube/videos/" + itoa(kid) + "/file"},
		{"GET", "/api/v1/youtube/videos/" + itoa(kid) + "/file?download=1"},
		{"GET", "/api/v1/youtube/videos/" + itoa(kid) + "/thumb"},
		{"PUT", "/api/v1/youtube/videos/" + itoa(kid) + "/position"},
		{"POST", "/api/v1/youtube/videos/" + itoa(kid) + "/retry"},
	} {
		kai.call(c.method, c.path, `{"seconds":1,"duration":2}`, 403)
	}

	var pos struct {
		Watched bool `json:"watched"`
	}
	decodeJSON(t, r.ana.call("PUT", "/api/v1/youtube/videos/"+itoa(id)+"/position", `{"seconds":95,"duration":100}`, 200), &pos)
	if !pos.Watched {
		t.Fatal("95/100 not watched")
	}
	r.ana.call("POST", "/api/v1/youtube/videos/"+itoa(id)+"/retry", "", 409)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error { return ytlib.SetVideoState(tx, id, ytlib.StateFailed, "x") }); err != nil {
		t.Fatal(err)
	}
	var v ytVideo
	decodeJSON(t, r.ana.call("POST", "/api/v1/youtube/videos/"+itoa(id)+"/retry", "", 200), &v)
	if v.State != "queued" {
		t.Fatalf("retried %+v", v)
	}
	// Queued again, the file is no longer served.
	if resp, _ = r.ana.raw("GET", file, nil); resp.StatusCode != 404 {
		t.Fatalf("queued file %d", resp.StatusCode)
	}
}

// Deleting a person takes their YouTube rows and folder with them, and
// leaves everyone else's alone.
func TestDeletingAPersonForgetsTheirYouTube(t *testing.T) {
	r := ytRig(t)
	youtubeForgets(r.api.s.Auth, r.st, r.api.s.Queue, r.live)
	ana := ytDone(t, r, r.ana.id, "aaaaaaaaaaa", "a.mp4", "anna")
	bo := ytDone(t, r, r.bo.id, "bbbbbbbbbbb", "b.mp4", "bo")
	r.owner.call("DELETE", "/api/v1/users/"+itoa(r.ana.id), "", 204)
	ctx := context.Background()
	if _, err := ytlib.VideoByID(ctx, r.st.Read(), ana); err == nil {
		t.Fatal("the deleted person's video row is still there")
	}
	if _, err := ytlib.VideoByID(ctx, r.st.Read(), bo); err != nil {
		t.Fatalf("another person's video went: %v", err)
	}
	root := ytlib.LoadSettings(ctx, r.st.Read()).Folder
	if _, err := os.Stat(filepath.Join(root, itoa(r.ana.id)+"-p")); !os.IsNotExist(err) {
		t.Fatalf("the deleted person's folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, itoa(r.bo.id)+"-p", "Chan", "b.mp4")); err != nil {
		t.Fatalf("another person's file: %v", err)
	}
}

// route: GET /api/v1/files/{id}/download
// Saving a movie file needs save_files; a kid is denied even ticked, and
// with the youtube feature switch off the route is not there.
func TestFileDownloadRoute(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	var add struct {
		Title struct {
			Slots []struct {
				ID        int64 `json:"id"`
				VersionID int64 `json:"version_id"`
			} `json:"slots"`
		} `json:"title"`
	}
	decodeJSON(t, r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:900002"}`, 201), &add)
	slot := add.Title.Slots[0]
	root, err := mediafiles.LibraryPathOfVersion(ctx, r.st.Read(), slot.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	rel := "Paper Harbour (2023)/Paper Harbour.mkv"
	if err := os.MkdirAll(filepath.Join(root, "Paper Harbour (2023)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("movie bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &mediafiles.File{SlotIDs: []int64{slot.ID}, VersionID: slot.VersionID, RelativePath: rel, Size: 11, AddedAt: time.Now().UTC(),
		Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p := "/api/v1/files/" + itoa(f.ID) + "/download"

	resp, b := r.owner.raw("GET", p, nil)
	if resp.StatusCode != 200 || string(b) != "movie bytes" || resp.Header.Get("Content-Disposition") != `attachment; filename="Paper Harbour.mkv"` {
		t.Fatalf("owner %d %v", resp.StatusCode, resp.Header)
	}
	if resp, _ = r.ana.raw("GET", p, nil); resp.StatusCode != 403 {
		t.Fatalf("member without save_files %d", resp.StatusCode)
	}
	r.owner.call("PUT", "/api/v1/users/"+itoa(r.ana.id), `{"perms":["ask","save_files"]}`, 200)
	if resp, b = r.ana.raw("GET", p, map[string]string{"Range": "bytes=0-4"}); resp.StatusCode != 206 || string(b) != "movie" {
		t.Fatalf("member with save_files %d %q", resp.StatusCode, b)
	}

	kai := ytJoin(t, r, "kai", `{"max_rating":"PG"}`)
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","save_files"]' WHERE id = ?`, kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if resp, b = kai.raw("GET", p, nil); resp.StatusCode == 200 || strings.Contains(string(b), "movie bytes") {
		t.Fatalf("kid %d", resp.StatusCode)
	}
}

func TestFileDownloadAbsentWhenFlagOff(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	addr, _, _, stop := runApp(t, config{ConfigDir: t.TempDir()})
	defer func() { _ = stop() }()
	jar, _ := cookiejar.New(nil)
	own := &signedIn{t: t, base: "http://" + addr, c: &http.Client{Jar: jar}}
	var x struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, own.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201), &x)
	own.csrf = x.CSRF
	own.call("GET", "/api/v1/files/1/download", "", 404)
}

// route: POST /api/v1/youtube/adopt
func TestYouTubeAdoptRoute(t *testing.T) {
	r := ytRig(t)
	var set struct {
		Folder string `json:"folder"`
	}
	decodeJSON(t, r.owner.call("GET", "/api/v1/settings/youtube", "", 200), &set)
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Course"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"01 - What？ ｜ Part： One.mp4": "video",
		"01 - What？ ｜ Part： One.nfo": `<episodedetails><title>What? | Part: One</title><showtitle>Someone</showtitle>` +
			`<uniqueid type="youtube" default="true">abcdefghijk</uniqueid><aired>2016-08-01 00:00:00Z</aired></episodedetails>`,
	} {
		if err := os.WriteFile(filepath.Join(src, "Course", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	req := func(uid int64, p string, dry bool) string {
		b, _ := json.Marshal(map[string]any{"user_id": uid, "path": p, "dry_run": dry})
		return string(b)
	}

	var plan struct {
		Items []struct {
			From    string `json:"from"`
			To      string `json:"to"`
			VideoID string `json:"video_id"`
			Folder  string `json:"folder"`
			Reason  string `json:"reason"`
		} `json:"items"`
	}
	decodeJSON(t, r.owner.call("POST", "/api/v1/youtube/adopt", req(r.ana.id, src, true), 200), &plan)
	if len(plan.Items) != 1 || plan.Items[0].VideoID != "abcdefghijk" || plan.Items[0].Folder != "Course" || plan.Items[0].Reason != "" ||
		plan.Items[0].To != itoa(r.ana.id)+"-ana/Someone/2016-08-01 What_ _ Part_ One [abcdefghijk].mp4" {
		t.Fatalf("plan %+v", plan)
	}

	// Refused: not the owner, a person without youtube, a relative path,
	// the YouTube folder itself, a link that is not a playlist.
	r.ana.call("POST", "/api/v1/youtube/adopt", req(r.ana.id, src, true), 403)
	r.owner.call("POST", "/api/v1/youtube/adopt", req(r.bo.id, src, true), 400)
	r.owner.call("POST", "/api/v1/youtube/adopt", req(r.ana.id, "Course", true), 400)
	r.owner.call("POST", "/api/v1/youtube/adopt", req(r.ana.id, set.Folder, false), 400)
	r.owner.call("POST", "/api/v1/youtube/adopt", `{"user_id":`+itoa(r.ana.id)+`,"path":`+jsonString(src)+
		`,"playlists":{"Course":"https://youtu.be/dQw4w9WgXcQ"}}`, 400)

	var started struct {
		JobID int64 `json:"job_id"`
	}
	decodeJSON(t, r.owner.call("POST", "/api/v1/youtube/adopt", req(r.ana.id, src, false), 202), &started)
	var kind, params string
	if err := r.st.Read().QueryRow("SELECT kind, params FROM jobs WHERE id = ?", started.JobID).Scan(&kind, &params); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		t.Fatal(err)
	}
	if kind != "youtube_adopt" || len(p) != 2 || p["path"] != src || p["user_id"] != float64(r.ana.id) {
		t.Fatalf("job %s %s", kind, params)
	}
}
