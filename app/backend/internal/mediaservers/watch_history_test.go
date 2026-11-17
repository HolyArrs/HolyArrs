package mediaservers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// watchTitles adds a movie (tmdb 603) and a show (tvdb 100).
func watchTitles(t *testing.T, w *world) (movie, show int64) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, ti := range []*mediafiles.Title{
			{Kind: "movie", Name: "The Matrix", SortName: "matrix", MetadataSource: "tmdb", Status: "released", SourceIDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}},
			{Kind: "show", Name: "Severance", SortName: "severance", MetadataSource: "tvdb", Status: "running", SourceIDs: map[string]string{"tvdb": "100"}},
		} {
			ti.GateDriver, ti.NewPartsPolicy, ti.ReleaseGate, ti.WhyAdded, ti.State = "none", "all", "released", "by_hand", "active"
			ti.Aliases, ti.MusicTypes, ti.Artwork = []string{}, []string{}, []string{}
			if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
				return err
			}
			if ti.Kind == "movie" {
				movie = ti.ID
			} else {
				show = ti.ID
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return movie, show
}

func watchOn(t *testing.T, w *world, on bool) {
	t.Helper()
	if err := SetWatchHistory(context.Background(), w.s, on, domain.System); err != nil {
		t.Fatal(err)
	}
}

func watchRows(t *testing.T, w *world) []string {
	t.Helper()
	rows, err := w.s.Read().Query(`SELECT account_key || '/' || account_name || '/' || kind || '/' || season || 'x' || episode || '/' || plays
		FROM watch_state ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

// plexHistoryServer plays a Plex server: one movie library, one show
// library and a watch history of two accounts. onHistory, when set, runs
// as each history page is asked for.
func plexHistoryServer(t *testing.T, failHistory *atomic.Bool, onHistory func()) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/accounts":
			_, _ = w.Write([]byte(`<MediaContainer><Account id="0" name=""/><Account id="1" name="luis"/><Account id="22" name="ana"/></MediaContainer>`))
		case "/library/sections":
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Movies"/><Directory key="2" type="show" title="TV"/></MediaContainer>`))
		case "/library/sections/1/all":
			_, _ = w.Write([]byte(plexPage(r, `<Video ratingKey="50" guid="plex://movie/x"><Guid id="tmdb://603"/></Video>`, `<Video ratingKey="51" guid="com.plexapp.agents.imdb://tt999?lang=en"/>`)))
		case "/library/sections/2/all":
			_, _ = w.Write([]byte(plexPage(r, `<Directory ratingKey="7" guid="plex://show/abc"><Guid id="tvdb://100"/></Directory>`)))
		case "/status/sessions/history/all":
			if onHistory != nil {
				onHistory()
			}
			if failHistory != nil && failHistory.Load() && r.URL.Query().Get("X-Plex-Container-Start") == "2" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(plexPage(r,
				`<Video type="movie" ratingKey="50" accountID="1" viewedAt="1700000000"/>`,
				`<Video type="episode" ratingKey="70" grandparentKey="/library/metadata/7" parentIndex="1" index="2" accountID="22" viewedAt="1700000100"/>`,
				`<Video type="episode" ratingKey="70" grandparentKey="/library/metadata/7" parentIndex="1" index="2" accountID="22" viewedAt="1700000200"/>`,
				`<Video type="movie" ratingKey="51" accountID="22" viewedAt="1700000300"/>`,
				`<Video type="episode" grandparentKey="/library/metadata/9" parentIndex="1" index="1" accountID="22"/>`)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWatchHistoryOffReadsNothing(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(srv.Close)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	if WatchHistoryOn(ctx, w.s.Read()) {
		t.Fatal("watch history is on by default")
	}
	if err := (&Linker{Store: w.s, Vault: w.v}).ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || len(watchRows(t, w)) != 0 {
		t.Fatalf("calls %d rows %v", calls.Load(), watchRows(t, w))
	}
}

func TestWatchHistoryPlexEveryAccount(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	srv := plexHistoryServer(t, nil, nil)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	if err := (&Linker{Store: w.s, Vault: w.v}).ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"1/luis/movie/0x0/1", "22/ana/episode/1x2/2"}
	if got := watchRows(t, w); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rows %v, want %v", got, want)
	}

	// ana's Plex account is linked to a person; luis is not.
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'Ana B', 'member', '')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (7, 'plex', '22', 'ana', 't')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := Watched(ctx, w.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Enabled || sum.ReadAt == nil || len(sum.People) != 2 {
		t.Fatalf("summary %+v", sum)
	}
	for _, p := range sum.People {
		switch p.Name {
		case "luis":
			if p.Linked || p.Movies != 1 || p.Episodes != 0 || p.Server != "Plex" || p.LastWatchedAt == nil {
				t.Fatalf("luis %+v", p)
			}
		case "ana":
			if !p.Linked || p.UserName != "Ana B" || p.Episodes != 1 || p.Shows != 1 || p.Plays != 2 {
				t.Fatalf("ana %+v", p)
			}
		default:
			t.Fatalf("person %+v", p)
		}
	}

	// Turning it off deletes every row.
	watchOn(t, w, false)
	if rows := watchRows(t, w); len(rows) != 0 {
		t.Fatalf("rows after off %v", rows)
	}
}

// The Plex server's owner is account 1 on the server, while their Plex
// link keeps their plex.tv id; their Plex name still links them.
func TestWatchHistoryPlexOwnerLinkedByName(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	srv := plexHistoryServer(t, nil, nil)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	if err := (&Linker{Store: w.s, Vault: w.v}).ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (8, 't', '{}', 't', '{}', 'Luis R', 'member', '')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (8, 'plex', '9876543', 'Luis', 't')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := Watched(ctx, w.s.Read())
	if err != nil || len(sum.People) != 2 {
		t.Fatalf("summary %+v %v", sum, err)
	}
	for _, p := range sum.People {
		switch {
		case p.Name == "luis" && (!p.Linked || p.UserName != "Luis R" || p.UserID == nil || *p.UserID != 8):
			t.Fatalf("the server owner %+v", p)
		case p.Name == "ana" && p.Linked:
			t.Fatalf("ana has no link %+v", p)
		}
	}
}

// Only the server's owner (account 1) is linked by Plex name: another
// account with the same name stays unlinked.
func TestWatchHistoryPlexNameLinksOnlyTheOwner(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	srv := plexHistoryServer(t, nil, nil)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	if err := (&Linker{Store: w.s, Vault: w.v}).ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (9, 't', '{}', 't', '{}', 'Other Ana', 'member', '')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (9, 'plex', '555', 'ana', 't')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sum, err := Watched(ctx, w.s.Read())
	if err != nil || len(sum.People) != 2 {
		t.Fatalf("summary %+v %v", sum, err)
	}
	for _, p := range sum.People {
		if p.Name == "ana" && p.Linked {
			t.Fatalf("account 22 linked by name to another Plex account %+v", p)
		}
	}
}

// A failed page keeps the rows of the last whole pass.
func TestWatchHistoryFailedPageKeepsRows(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	var fail atomic.Bool
	srv := plexHistoryServer(t, &fail, nil)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	before := watchRows(t, w)
	fail.Store(true)
	if err := l.ReadWatchHistory(ctx); err == nil {
		t.Fatal("a failed page was not an error")
	}
	if after := watchRows(t, w); strings.Join(after, ",") != strings.Join(before, ",") || len(after) == 0 {
		t.Fatalf("rows changed: %v -> %v", before, after)
	}
}

// Turning watch history off while a pass reads leaves no rows behind.
func TestWatchHistoryOffDuringAPassKeepsNoRows(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	var once sync.Once
	srv := plexHistoryServer(t, nil, func() {
		once.Do(func() {
			if err := SetWatchHistory(ctx, w.s, false, domain.System); err != nil {
				t.Error(err)
			}
		})
	})
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	if err := (&Linker{Store: w.s, Vault: w.v}).ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if rows := watchRows(t, w); len(rows) != 0 || WatchHistoryOn(ctx, w.s.Read()) {
		t.Fatalf("rows after off %v", rows)
	}
}

// jellyHistoryServer plays Jellyfin or Emby (recorded answer shapes). big
// makes the second user's page larger than an answer may be: an empty list
// padded past the cap, which cut at the cap would read as nothing played.
func jellyHistoryServer(t *testing.T, kind string, big *atomic.Bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		okTok := r.Header.Get("X-Emby-Token") == tok
		if kind == "jellyfin" {
			okTok = strings.Contains(r.Header.Get("Authorization"), `Token="`+tok+`"`)
		}
		if !okTok || r.Method != http.MethodGet {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		page := func(all ...string) string {
			start, _ := strconv.Atoi(q.Get("StartIndex"))
			limit, err := strconv.Atoi(q.Get("Limit"))
			if err != nil {
				limit = len(all)
			}
			start = min(start, len(all))
			return `{"Items":[` + strings.Join(all[start:min(start+limit, len(all))], ",") + `],"TotalRecordCount":` + strconv.Itoa(len(all)) + `}`
		}
		switch r.URL.Path {
		case "/Items":
			_, _ = rw.Write([]byte(page(`{"Id":"s1","ProviderIds":{"Tvdb":"100"}}`)))
		case "/Users":
			_, _ = rw.Write([]byte(`[{"Id":"u1","Name":"ana"},{"Id":"u2","Name":"kid"}]`))
		case "/Users/u1/Items":
			if q.Get("IsPlayed") != "true" {
				rw.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = rw.Write([]byte(page(
				`{"Type":"Movie","Id":"m1","ProviderIds":{"Tmdb":"603"},"UserData":{"PlayCount":3,"LastPlayedDate":"2025-01-02T03:04:05.0000000Z"}}`,
				`{"Type":"Episode","Id":"e1","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":1,"UserData":{"PlayCount":1}}`,
				`{"Type":"Episode","Id":"e9","SeriesId":"zz","ParentIndexNumber":1,"IndexNumber":1,"UserData":{"PlayCount":1}}`)))
		case "/Users/u2/Items":
			if big != nil && big.Load() {
				_, _ = rw.Write([]byte(`{"Items":[],"TotalRecordCount":0}` + strings.Repeat(" ", 5<<20)))
				return
			}
			_, _ = rw.Write([]byte(page(`{"Type":"Episode","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":3,"UserData":{"PlayCount":0,"Played":true}}`)))
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWatchHistoryJellyfinAndEmby(t *testing.T) {
	for _, kind := range []string{"jellyfin", "emby"} {
		t.Run(kind, func(t *testing.T) {
			smallPages(t)
			w := newWorld(t)
			ctx := context.Background()
			watchTitles(t, w)
			var big atomic.Bool
			srv := jellyHistoryServer(t, kind, &big)
			if _, err := Add(ctx, w.s, w.v, Input{Name: "Home", Kind: kind, URL: srv.URL, Token: tok}, domain.System); err != nil {
				t.Fatal(err)
			}
			watchOn(t, w, true)
			l := &Linker{Store: w.s, Vault: w.v}
			if err := l.ReadWatchHistory(ctx); err != nil {
				t.Fatal(err)
			}
			want := []string{"u1/ana/episode/1x1/1", "u1/ana/movie/0x0/3", "u2/kid/episode/1x3/1"}
			got := watchRows(t, w)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("rows %v, want %v", got, want)
			}
			// An answer past the size cap must not look like a short list.
			big.Store(true)
			if err := l.ReadWatchHistory(ctx); err == nil {
				t.Fatal("a capped answer was not an error")
			}
			if after := watchRows(t, w); strings.Join(after, ",") != strings.Join(want, ",") {
				t.Fatalf("rows changed after a capped answer: %v", after)
			}
		})
	}
}

// A whole pass that matches nothing in the Library still counts as read:
// the summary carries when, with nobody in it. A failed pass does not
// count, and turning watch history off forgets the time.
func TestWatchHistoryReadWithNoMatchKeepsTheReadTime(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	var fail atomic.Bool
	fail.Store(true)
	srv := plexHistoryServer(t, &fail, nil)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.ReadWatchHistory(ctx); err == nil {
		t.Fatal("a failed page was not an error")
	}
	if sum, err := Watched(ctx, w.s.Read()); err != nil || sum.ReadAt != nil {
		t.Fatalf("a failed pass counted as read: %+v %v", sum, err)
	}
	fail.Store(false)
	if err := l.ReadWatchHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if rows := watchRows(t, w); len(rows) != 0 {
		t.Fatalf("rows %v", rows)
	}
	sum, err := Watched(ctx, w.s.Read())
	if err != nil || !sum.Enabled || sum.ReadAt == nil || len(sum.People) != 0 {
		t.Fatalf("summary %+v %v", sum, err)
	}
	watchOn(t, w, false)
	watchOn(t, w, true)
	if sum, err := Watched(ctx, w.s.Read()); err != nil || sum.ReadAt != nil {
		t.Fatalf("read time kept after off: %+v %v", sum, err)
	}
}

// Turning watch history on reads right away in the background, and so does
// adding a media server while it is on.
func TestWatchHistoryReadsRightAway(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	watchTitles(t, w)
	srv := plexHistoryServer(t, nil, nil)
	l := &Linker{Store: w.s, Vault: w.v}
	m := http.NewServeMux()
	(&Handlers{Store: w.s, Vault: w.v, Linker: l}).Routes(m)
	send := func(method, path, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code/100 != 2 {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
	}
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	send("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`)
	l.Wait()
	if rows := watchRows(t, w); len(rows) != 2 {
		t.Fatalf("rows after turning on %v", rows)
	}

	jelly := jellyHistoryServer(t, "jellyfin", nil)
	send("POST", "/api/v1/media-servers", `{"name":"Home","kind":"jellyfin","url":"`+jelly.URL+`","token":"`+tok+`"}`)
	l.Wait()
	if rows := watchRows(t, w); len(rows) != 5 {
		t.Fatalf("rows after adding a server %v", rows)
	}

	// Safe mode reads nothing in the background.
	watchOn(t, w, false)
	l.Paused = true
	send("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`)
	l.Wait()
	if rows := watchRows(t, w); len(rows) != 0 {
		t.Fatalf("rows in safe mode %v", rows)
	}
}

// Stop ends a background read, so a shutdown does not wait for it.
func TestStopEndsABackgroundRead(t *testing.T) {
	smallPages(t)
	w := newWorld(t)
	ctx := context.Background()
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	watchOn(t, w, true)
	l := &Linker{Store: w.s, Vault: w.v}
	l.ReadWatchHistorySoon()
	<-started
	begin := time.Now()
	l.Stop()
	l.Wait()
	if time.Since(begin) > 5*time.Second {
		t.Fatalf("Stop waited %s", time.Since(begin))
	}
}
