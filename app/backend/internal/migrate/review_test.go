package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

func noLabels(any) []string { return nil }

// TestLinksInSettingsAreSecret: tokens ride in link paths and free-text
// parameters too, so every link in indexer and notification settings is a
// secret unless it is an indexer's base address, which keeps only its
// scheme, host and API root.
func TestLinksInSettingsAreSecret(t *testing.T) {
	ix := func(kind string, set map[string]any) Provider {
		fields := []any{}
		for k, v := range set {
			fields = append(fields, map[string]any{"name": k, "value": v})
		}
		return provider(map[string]any{"name": kind, "implementation": kind, "fields": fields}, noLabels)
	}
	indexers := linkSafe([]Provider{
		ix("Torznab", map[string]any{"baseUrl": "https://user" + ":" + "PWSECRET1@idx.example/api/9f8e7d6c5b4a39281706f5e4/torznab?api" + "key=QSECRET1&x=QSECRET2", "additionalParameters": "&pass" + "key=APSECRET1"}),
		ix("TorrentRssIndexer", map[string]any{"feedUrl": "https://tracker.example/rss/0123456789abcdef0123456789abcdef"}),
		ix("Newznab", map[string]any{"baseUrl": "http://prowlarr:9696/1/", "apiPath": "/api"}),
		ix("Torznab", map[string]any{"baseUrl": "http://jackett:9117/api/v2.0/indexers/1337x/results/torznab/"}),
	}, false)
	notes := linkSafe([]Provider{
		ix("Webhook", map[string]any{"url": "https://hooks.example.com/services/T000/B000/WHSECRET0123456789", "method": 1}),
		ix("Ntfy", map[string]any{"serverUrl": "https://ntfy.sh", "topics": []any{"TOPICSECRET1"}, "priority": 3}),
		ix("Gotify", map[string]any{"server": "https://gotify.example/GOTSECRET1"}),
	}, true)
	b, _ := json.Marshal(append(indexers, notes...))
	for _, s := range []string{"PWSECRET1", "QSECRET1", "QSECRET2", "APSECRET1", "0123456789abcdef", "9f8e7d6c5b4a", "WHSECRET", "TOPICSECRET1", "GOTSECRET1", "gotify.example", "hooks.example"} {
		if strings.Contains(string(b), s) {
			t.Errorf("%s survived: %s", s, b)
		}
	}
	if got := indexers[0].Settings["baseUrl"]; got != "https://idx.example/api" {
		t.Errorf("base address %v", got)
	}
	if got := indexers[2].Settings["baseUrl"]; got != "http://prowlarr:9696/1/" {
		t.Errorf("a plain Prowlarr address changed: %v", got)
	}
	if got := indexers[3].Settings["baseUrl"]; got != "http://jackett:9117/api/v2.0/indexers/1337x/results/torznab/" {
		t.Errorf("a plain Jackett address changed: %v", got)
	}
	for i, want := range [][]string{{"baseUrl", "additionalParameters"}, {"feedUrl"}, nil, nil} {
		for _, w := range want {
			if !hasField(indexers[i].NeedsReentry, w) {
				t.Errorf("indexer %d needs %v, want %s", i, indexers[i].NeedsReentry, w)
			}
		}
	}
	for i, w := range []string{"url", "serverUrl", "server"} {
		if !hasField(notes[i].NeedsReentry, w) {
			t.Errorf("notification %d needs %v, want %s", i, notes[i].NeedsReentry, w)
		}
	}
	if !hasField(notes[1].NeedsReentry, "topics") {
		t.Errorf("ntfy topics not listed: %v", notes[1].NeedsReentry)
	}
	if notes[1].Settings["priority"] == nil {
		t.Error("a plain setting was dropped")
	}
}

// TestHeldKeyEscaped: a typed key with characters JSON escapes is still found.
func TestHeldKeyEscaped(t *testing.T) {
	s := &Snapshot{Kind: Radarr, Notes: []string{"a<b&c>d1234"}}
	if err := s.mustNotHold("a<b&c>d1234"); !errors.Is(err, ErrHeldKey) {
		t.Fatalf("escaped key passed: %v", err)
	}
}

// TestMovedLinksAlwaysPaused: Sources and Downloaders come over paused and
// without local-network reach, whatever their keys; the owner turns each on.
func TestMovedLinksAlwaysPaused(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Indexers = []Provider{
			{Name: "Keyless", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://192.168.1.1/api"}},
			{Name: "Meta", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://169.254.169.254/latest"}},
		}
		radarr.DownloadClients = []Provider{{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1, Settings: map[string]any{"host": "qbit", "port": int64(8080)}}}
	})
	mv := l.move(t, Choices{})
	if n := l.count(t, "SELECT count(*) FROM sources WHERE name = 'Keyless' AND state = 'disabled' AND allow_local_network = 0"); n != 1 {
		t.Fatalf("keyless Source not paused or local on: %+v", lineOf(t, mv.Report, "source:keyless"))
	}
	if n := l.count(t, "SELECT count(*) FROM sources WHERE name = 'Meta'"); n != 0 {
		t.Fatal("a link-local Source was moved")
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE state = 'disabled' AND allow_local_network = 0"); n != 1 {
		t.Fatal("password-less Downloader not paused or local on")
	}
	found := false
	for _, n := range mv.Report.Next {
		if strings.Contains(n, "local network") {
			found = true
		}
	}
	if !found {
		t.Fatalf("next steps do not say how to turn them on: %v", mv.Report.Next)
	}
}

// TestUndoKeepsReusedRows: SQLite reuses a deleted row's id; undo must not
// delete the owner's own row that took the id.
func TestUndoKeepsReusedRows(t *testing.T) {
	l := newMoveLab(t)
	mv := l.move(t, Choices{})
	ctx := context.Background()
	ids := map[string]int64{}
	for _, tb := range []string{"sources", "alerts", "never_add", "asks", "subtitle_sites", "downloaders"} {
		ids[tb] = int64(l.count(t, "SELECT max(id) FROM "+tb))
	}
	ana := int64(l.count(t, "SELECT id FROM users WHERE name = 'Ana'"))
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		later := database.FormatTime(time.Now().Add(time.Hour))
		for tb, id := range ids {
			if _, err := tx.Exec("UPDATE "+tb+" SET created_at = ?, name = name || ' mine' WHERE id = ?", later, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE users SET created_at = ?, name = 'Bob' WHERE id = ?", later, ana)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u := l.undo(t, mv.ID)
	if u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	for tb, id := range ids {
		if n := l.count(t, "SELECT count(*) FROM "+tb+" WHERE id = ?", id); n != 1 {
			t.Errorf("undo removed the owner's own row %d in %s", id, tb)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Bob'"); n != 1 {
		t.Error("undo removed the owner's own person")
	}
	if u.Report.Counts[ResKept] < len(ids)+1 {
		t.Fatalf("kept %+v", u.Report.Counts)
	}
}

// TestUndoKeepsLaterSettings: naming and subtitle languages changed after
// the move stay as the owner left them.
func TestUndoKeepsLaterSettings(t *testing.T) {
	l := newMoveLab(t)
	mv := l.move(t, Choices{})
	ctx := context.Background()
	ns, _ := mediafiles.ReadNaming(ctx, l.st.Read())
	ns.FilePattern = "{title} mine"
	if err := mediafiles.PutNamingSettings(ctx, l.st, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	ss := subtitles.ReadSettings(ctx, l.st.Read())
	ss.Languages = []subtitles.Want{{Language: "fr", HearingImpaired: "either"}}
	if err := subtitles.SaveSettings(ctx, l.st, ss, domain.System); err != nil {
		t.Fatal(err)
	}
	u := l.undo(t, mv.ID)
	if ns, _ := mediafiles.ReadNaming(ctx, l.st.Read()); ns.FilePattern != "{title} mine" {
		t.Fatalf("undo put naming back over a later change: %s", ns.FilePattern)
	}
	if langs := subtitles.ReadSettings(ctx, l.st.Read()).Languages; len(langs) != 1 || langs[0].Language != "fr" {
		t.Fatalf("undo put languages back over a later change: %+v", langs)
	}
	for _, k := range []string{"naming:radarr", "subtitles:languages"} {
		if got := lineOf(t, u.Report, k); got.Result != ResKept || !strings.Contains(got.Detail, "changed it since") {
			t.Errorf("%s: %+v", k, got)
		}
	}
}

// TestPeopleNeedTheOwnersWord: roles cap at member, the old owner is the
// owner here only when confirmed, a same-name person is never merged
// silently, and a bad email is not moved.
func TestPeopleNeedTheOwnersWord(t *testing.T) {
	l := newMoveLabWith(t, func(_, _, seerr *Snapshot) {
		seerr.Users = append(seerr.Users, User{SourceID: 3, Name: "Boss", Admin: true, Permissions: []string{"ADMIN", "MANAGE_SETTINGS"}},
			User{SourceID: 4, Name: "Mallory", Email: "a@b@c\r\nBcc: x@y"})
	})
	ctx := context.Background()
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "users", &domain.User{Name: "ana", Role: "member", PasswordHash: "x", State: "active", Perms: "[]"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	l.people = map[string]string{}
	p := l.plan(t)
	if it := item(t, p, GroupPeople, "person:seerr:3"); it.Person.Role != "member" || !it.Person.WasAdmin {
		t.Fatalf("admin not capped: %+v", it.Person)
	}
	if it := item(t, p, GroupPeople, "person:seerr:1"); it.Person.Role != "member" || !it.Person.WasOwner {
		t.Fatalf("owner without confirmation: %+v", it.Person)
	}
	rep := l.mv.Run(ctx, 0, p, true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, "person:seerr:2"); got.Result != ResSkipped || got.Choose != ChooseSameName {
		t.Fatalf("same name: %+v", got)
	}
	if !hasNext(rep, "Some people need a choice") {
		t.Fatalf("no choice line in next: %q", rep.Next)
	}
	// A flag on a person who was not the owner there does nothing.
	l.people = map[string]string{"person:seerr:3": "owner"}
	if it := item(t, l.plan(t), GroupPeople, "person:seerr:3"); it.Person.Role != "member" {
		t.Fatalf("owner from a non-owner: %+v", it.Person)
	}
	mv := l.move(t, Choices{People: map[string]string{"person:seerr:1": "owner", "person:seerr:2": "merge", "person:seerr:3": "admin"}})
	if got := lineOf(t, mv.Report, "person:seerr:2"); got.Result != ResMatched || got.Choose != ChooseSameName {
		t.Fatalf("merge: %+v", got)
	}
	if hasNext(mv.Report, "Some people need a choice") {
		t.Fatalf("a made choice still asks for one: %q", mv.Report.Next)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Ana' COLLATE NOCASE"); n != 1 {
		t.Fatal("a second Ana was made")
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE name = 'Boss' AND role = 'admin' AND state = 'disabled'"); n != 1 {
		t.Fatal("confirmed admin not an admin placeholder")
	}
	if n := l.count(t, "SELECT count(*) FROM users u JOIN user_ask_settings s ON s.user_id = u.id WHERE u.name = 'Mallory' AND s.email = ''"); n != 1 {
		t.Fatal("a bad email came over")
	}
	bad := false
	for _, n := range mv.Report.NotMoved {
		if strings.Contains(n.What, "Mallory") && strings.Contains(n.Why, "email") {
			bad = true
		}
	}
	if !bad {
		t.Fatalf("bad email not listed: %+v", mv.Report.NotMoved)
	}
}

// TestInsertAndMarkTogether: a row whose mark cannot be kept is not made.
func TestInsertAndMarkTogether(t *testing.T) {
	l := newMoveLab(t)
	err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TRIGGER no_mark BEFORE INSERT ON move_made WHEN NEW.what IN ('never_add', 'downloader', 'source', 'alert', 'user', 'ask', 'subtitle_site', 'folder')
			BEGIN SELECT RAISE(ABORT, 'no mark'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	for _, tb := range []string{"never_add", "downloaders", "sources", "alerts", "asks", "subtitle_sites", "library_folders"} {
		if n := l.count(t, "SELECT count(*) FROM "+tb); n != 0 {
			t.Errorf("%s: %d rows made without their mark", tb, n)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM users"); n != 1 {
		t.Errorf("people made without their mark: %d", n)
	}
	if mv.Report.Counts[ResFailed] == 0 {
		t.Fatalf("no failures reported: %+v", mv.Report.Counts)
	}
}

// TestStuckMoveRecovered: a move left running by a crash, with no live Job,
// is failed at start and does not block a new move.
func TestStuckMoveRecovered(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state, job_id) VALUES (?, 1, 'running', 4242)", time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO moves (created_at, owner_id, state, undo_state, undo_job_id) VALUES (?, 1, 'done', 'running', 4243)", time.Now().UTC().Format(time.RFC3339))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.mv.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM moves WHERE state = 'failed' AND report LIKE '%stopped%'"); n != 1 {
		t.Fatal("stuck move not failed")
	}
	if n := l.count(t, "SELECT count(*) FROM moves WHERE undo_state = 'failed'"); n != 1 {
		t.Fatal("stuck undo not failed")
	}
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("new move after recovery: %+v", mv)
	}
}
