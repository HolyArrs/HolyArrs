package secretsfile

import (
	"context"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func deps(t *testing.T) Deps {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Store: st, Vault: v, By: domain.System}
}

func parse(t *testing.T, text string) *File {
	t.Helper()
	f, err := Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func results(ls []Line) map[string]string {
	m := map[string]string{}
	for _, l := range ls {
		m[l.Item] = l.Result
	}
	return m
}

// noValues fails when any value of f shows in the lines or in an Event.
func noValues(t *testing.T, d Deps, f *File, ls []Line) {
	t.Helper()
	var all strings.Builder
	for _, l := range ls {
		all.WriteString(l.Item + " " + l.Result + "\n")
	}
	rows, err := d.Store.Read().Query("SELECT summary, data FROM events")
	if err == nil {
		for rows.Next() {
			var s, data string
			_ = rows.Scan(&s, &data)
			all.WriteString(s + data)
		}
		rows.Close()
	}
	for _, v := range f.Values() {
		if strings.Contains(all.String(), v) {
			t.Errorf("a value from the file was said: %q", v)
		}
	}
}

func count(t *testing.T, d Deps, q string) int {
	t.Helper()
	var n int
	if err := d.Store.Read().QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyAndAgain(t *testing.T) {
	d := deps(t)
	ctx := context.Background()
	f := parse(t, fakeFile)

	// A dry run changes nothing.
	dry := Apply(ctx, d, f, true)
	if r := results(dry); r["tvdb"] != WouldSet || !strings.HasPrefix(r["indexers[1]"], WouldSet) {
		t.Fatalf("dry %v", r)
	}
	if count(t, d, "SELECT count(*) FROM sources")+count(t, d, "SELECT count(*) FROM downloaders")+count(t, d, "SELECT (SELECT count(*) FROM move_credentials) + count(*) FROM settings WHERE key IN ('tmdb_token', 'tvdb_key', 'trakt_client_id')")+
		count(t, d, "SELECT count(*) FROM subtitle_sites")+count(t, d, "SELECT count(*) FROM alerts")+count(t, d, "SELECT count(*) FROM media_servers") != 0 {
		t.Fatal("dry run wrote")
	}

	ls := Apply(ctx, d, f, false)
	r := results(ls)
	want := map[string]string{
		"tmdb.read_token": Set, "tmdb.api_key": Empty, "tvdb": Set, "trakt": Set, "opensubtitles": Set, "subdl": Set, "addic7ed": Empty,
		"jackett": Empty, "nzbget": Empty, "emby": Empty, "plex": Empty, "sonarr (old app)": Empty, "discord": Empty,
	}
	for k, v := range want {
		if r[k] != v {
			t.Errorf("%s: %q, want %q", k, r[k], v)
		}
	}
	for _, k := range []string{"indexers[1]", "indexers[2]", "qbittorrent", "sabnzbd", "jellyfin", "radarr (old app)", "prowlarr (old app)", "smtp"} {
		if !strings.HasPrefix(r[k], Set) {
			t.Errorf("%s: %q", k, r[k])
		}
	}
	if !strings.HasPrefix(r["webhook"], "skipped") || !strings.HasPrefix(r["holyarrs"], "skipped") || !strings.HasPrefix(r["prowlarr"], "noted") {
		t.Errorf("webhook %q holyarrs %q prowlarr %q", r["webhook"], r["holyarrs"], r["prowlarr"])
	}
	if !AllOK(ls) {
		t.Fatalf("failures: %v", ls)
	}
	noValues(t, d, f, ls)

	// What was made: paused, local only where the file said so, keys in
	// the vault (the bridge key filled the indexer without one).
	if n := count(t, d, "SELECT count(*) FROM sources WHERE state = 'disabled' AND allow_local_network = 1"); n != 2 {
		t.Errorf("sources paused and local: %d", n)
	}
	var ref int64
	_ = d.Store.Read().QueryRow("SELECT secret_ref FROM sources WHERE name = 'Lab feed'").Scan(&ref)
	if k, _ := d.Vault.Get(ctx, ref); k != "your-prowlarr-key-7" {
		t.Errorf("bridge key not used")
	}
	if n := count(t, d, "SELECT count(*) FROM downloaders WHERE state = 'disabled' AND allow_local_network = 1"); n != 2 {
		t.Errorf("downloaders: %d", n)
	}
	if n := count(t, d, "SELECT count(*) FROM media_servers WHERE enabled = 0"); n != 1 {
		t.Errorf("media servers: %d", n)
	}
	if n := count(t, d, "SELECT count(*) FROM alerts WHERE enabled = 0 AND kind = 'email' AND smtp_security = 'none'"); n != 1 {
		t.Errorf("email alert: %d", n)
	}
	if n := count(t, d, "SELECT count(*) FROM subtitle_sites WHERE kind IN ('opensubtitles', 'subdl') AND secret_ref IS NOT NULL"); n != 2 {
		t.Errorf("subtitle sites: %d", n)
	}
	if tok, _ := d.Vault.Setting(ctx, LinksGroup, TMDBTokenKey); tok != "your-tmdb-read-1" {
		t.Errorf("tmdb token")
	}
	saved, err := ListMoveCredentials(ctx, d.Store, d.Vault)
	if err != nil || len(saved) != 2 || saved[0].Kind != "radarr" || saved[1].Kind != "prowlarr" || saved[0].Address != "http://127.0.0.1:18702" {
		t.Errorf("saved %+v %v", saved, err)
	}

	// Again: everything unchanged.
	again := Apply(ctx, d, f, false)
	for _, l := range again {
		if strings.HasPrefix(l.Result, Set) || l.Failed() {
			t.Errorf("second run: %s %s", l.Item, l.Result)
		}
	}
	if results(again)["qbittorrent"] != Unchanged || results(again)["indexers[1]"] != Unchanged {
		t.Errorf("second run %v", results(again))
	}

	// One changed item: only it is set.
	f2 := parse(t, strings.Replace(fakeFile, "your-tvdb-key-2", "your-tvdb-key-22", 1))
	third := results(Apply(ctx, d, f2, false))
	for k, v := range third {
		if strings.HasPrefix(v, Set) && k != "tvdb" {
			t.Errorf("also set: %s", k)
		}
	}
	if third["tvdb"] != Set {
		t.Errorf("tvdb %q", third["tvdb"])
	}

	// A bad item fails on its own; the others stay as they are.
	f3 := parse(t, strings.Replace(fakeFile, "http://127.0.0.1:18799/api", "ftp://127.0.0.1/api", 1))
	fourth := Apply(ctx, d, f3, false)
	r4 := results(fourth)
	if !strings.HasPrefix(r4["indexers[2]"], "failed: ") || r4["indexers[1]"] != Unchanged || AllOK(fourth) {
		t.Errorf("partial %v", r4)
	}
	noValues(t, d, f3, fourth)

	// Forgetting the move credentials.
	if n, err := ForgetMoveCredential(ctx, d.Store, ""); err != nil || n != 2 {
		t.Fatalf("forget %d %v", n, err)
	}
	if n := count(t, d, "SELECT count(*) FROM secrets WHERE kind = 'move_credential'"); n != 0 {
		t.Errorf("move secrets left: %d", n)
	}
}

func TestApplyNeedsBothParts(t *testing.T) {
	d := deps(t)
	f := parse(t, "version: 1\nsabnzbd:\n  address: http://127.0.0.1:18707\nradarr:\n  api_key: your-x-1\nopensubtitles:\n  api_key: your-x-2\n")
	ls := Apply(context.Background(), d, f, false)
	r := results(ls)
	if !strings.HasPrefix(r["sabnzbd"], "failed: needs") || !strings.HasPrefix(r["radarr (old app)"], "failed: needs") || !strings.HasPrefix(r["opensubtitles"], "failed: ") {
		t.Fatalf("%v", r)
	}
	noValues(t, d, f, ls)
	var n int
	_ = d.Store.Read().QueryRow("SELECT count(*) FROM downloaders").Scan(&n)
	if n != 0 {
		t.Error("half a Downloader made")
	}
}

func TestApplyWithoutVault(t *testing.T) {
	d := deps(t)
	d.Vault = nil
	ls := Apply(context.Background(), d, parse(t, "version: 1\n"), false)
	if len(ls) != 1 || !ls[0].Failed() {
		t.Fatalf("%v", ls)
	}
}
