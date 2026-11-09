package main

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A member watching their own ask does not see the download's save folder
// on the host; the owner does.
func TestDownloadRowPathHiddenFromMember(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, `UPDATE downloads SET save_path = '/data/downloads/secret-layout'`)
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, 't', '{}', 't', '{}', 'owner', 'owner', 'x'), (2, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'approved', t.id, 2, 't' FROM titles t LIMIT 1`)
	exec41(t, x.s, `INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (1, 2, 't')`)
	mine := viewAs(t, x, webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Via: "session"})
	if len(mine) != 1 || mine[0].Path != "" {
		t.Fatalf("member row %+v", mine)
	}
	own := viewAs(t, x, webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"})
	if len(own) != 1 || own[0].Path == "" {
		t.Fatalf("owner row %+v", own)
	}
}

// Every answer carries an opener policy, so a page an outside site opened
// cannot be watched or redirected through window.opener handles.
func TestSecureSetsOpenerPolicy(t *testing.T) {
	h := webserver.SecureHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/library/1", nil))
	if got := w.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin-allow-popups" {
		t.Fatalf("COOP %q", got)
	}
}

// A member reading a title does not get the server folder, the Pick rules
// or a Slot's reason; the owner does.
func TestTitleViewTrimmedForMember(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, `UPDATE slots SET last_reason = 'turned down: Secret.Release-GRP'`)
	a := buildHandlers(services{Store: x.s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"})
	ts, err := database.Select[mediafiles.Title](context.Background(), x.s.Read(), "titles", "1=1 LIMIT 1")
	if err != nil || len(ts) != 1 {
		t.Fatal(err)
	}
	view := func(who webserver.Caller) TitleView {
		v, err := a.titleView(webserver.ContextWith(context.Background(), who), ts[0])
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	own := view(webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"})
	if own.FolderPath == "" || len(own.Slots) == 0 || own.Slots[0].Reason == "" {
		t.Fatalf("owner view %+v", own)
	}
	mine := view(webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Via: "session"})
	if mine.FolderPath != "" || mine.PickRulesName != "" || mine.LibraryFolderID != 0 || mine.Slots[0].Reason != "" {
		t.Fatalf("member view %+v", mine)
	}
	for _, ver := range mine.Versions {
		if ver.FolderPath != "" || ver.PickRulesName != "" {
			t.Fatalf("member version %+v", ver)
		}
	}
	f := mediafiles.File{RelativePath: "Movies/Secret (2020)/x.mkv", Size: 9}
	got := discover.TrimForViewer(webserver.ContextWith(context.Background(), webserver.Caller{UserID: 2, Role: webserver.RoleMember, Via: "session"}), TitleView{Slots: []SlotView{{File: &f}}})
	if got.Slots[0].File.RelativePath != "" || got.Slots[0].File.Size != 9 || f.RelativePath == "" {
		t.Fatalf("file %+v", got.Slots[0].File)
	}
}

// Made-up category names stop adding rows once a member's day holds the
// cap; categories already counted that day still count.
func TestExploreStripCountsCapPerDay(t *testing.T) {
	a, s, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	name := func(i int) string { return "x" + strings.Repeat("a", i/26) + string(rune('a'+i%26)) }
	for n := 0; n < 10; n++ {
		var cs []string
		for i := 0; i < discover.StripRowsMax; i++ {
			cs = append(cs, `"`+name(n*discover.StripRowsMax+i)+`"`)
		}
		resp, err := http.Post(srv.URL+"/api/v1/explore/strip/counts", "application/json", strings.NewReader(`{"event":"shown","categories":[`+strings.Join(cs, ",")+`]}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	var rows int
	if err := s.Read().QueryRow("SELECT COUNT(*) FROM explore_category_counts").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows > discover.StripDayCategoriesMax {
		t.Fatalf("%d rows in one day", rows)
	}
	resp, _ := http.Post(srv.URL+"/api/v1/explore/strip/counts", "application/json", strings.NewReader(`{"event":"open","categories":["`+name(0)+`"]}`))
	resp.Body.Close()
	var opened int
	_ = s.Read().QueryRow("SELECT opened FROM explore_category_counts WHERE category_id = ?", name(0)).Scan(&opened)
	if opened != 1 {
		t.Fatalf("known category opened %d", opened)
	}
}

// One person cannot make the server fetch new pictures without end: past
// the minute budget cache misses get 429 and TMDB is not asked; pictures
// already kept are still served.
func TestTMDBArtworkMissBudget(t *testing.T) {
	old := discover.ArtworkMissesPerMinute
	discover.ArtworkMissesPerMinute = 3
	defer func() { discover.ArtworkMissesPerMinute = old }()
	var hits atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = io.WriteString(w, "JPEGBYTES")
	}))
	defer pics.Close()
	u, _ := url.Parse(pics.URL)
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{metadata.ImageBase + "/"}, HTTP: &http.Client{Transport: toServer{u}}}
	srv := httptest.NewServer(buildHandlers(services{Version: "test", Library: &library.Service{Artwork: art}}).Handler())
	defer srv.Close()
	get := func(f string) int {
		res, err := http.Get(srv.URL + "/api/v1/artwork/tmdb/w342/" + f)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	for i := 0; i < 3; i++ {
		if c := get("p" + strconv.Itoa(i) + ".jpg"); c != 200 {
			t.Fatalf("miss %d: %d", i, c)
		}
	}
	if c := get("p9.jpg"); c != http.StatusTooManyRequests {
		t.Fatalf("over budget: %d", c)
	}
	if c := get("p0.jpg"); c != 200 {
		t.Fatalf("kept picture: %d", c)
	}
	if hits.Load() != 3 {
		t.Fatalf("TMDB asked %d times", hits.Load())
	}
}
