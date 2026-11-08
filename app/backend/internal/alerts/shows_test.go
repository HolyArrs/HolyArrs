package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"mime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// showWorld writes Copper Valley with episodes S01E01 to S01E03 (one Slot
// each, in a main Version).
func showWorld(t *testing.T, s *database.Store) (titleID int64, slots []int64) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		y := 2025
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Copper Valley", SortName: "copper valley", Year: &y, MetadataSource: "tmdb", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "running", State: "active", SourceIDs: map[string]string{"tmdb": "100", "tvdb": "200"},
			Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		var err error
		if titleID, err = database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		var lf, pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		res, err := tx.Exec(`INSERT INTO library_folders (path, kind, label, created_at, created_by, updated_at, updated_by) VALUES ('/shows', 'show', 'Shows', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		lf, _ = res.LastInsertId()
		v := &mediafiles.Version{TitleID: titleID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Copper Valley (2025)", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		for n, name := range []string{"The Mill", "Flood", "TBA"} {
			se, ep, nm := 1, n+1, name
			it := &mediafiles.Item{TitleID: titleID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &se, Episode: &ep}, Name: &nm, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotDownloading}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return titleID, slots
}

// Show moments name the show and its episodes; a pack import is one
// Alert listing its episodes, or one per episode with pack_alerts each.
func TestShowMomentsNameEpisodes(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid, slots := showWorld(t, s)
	dl := int64(7)
	one := slots[1]

	grab := domain.Event{Kind: "download_sent", TitleID: &tid, SlotID: &slots[0], DownloadID: &dl, Summary: "Sent Copper.Valley.S01.1080p to qBit",
		Data: map[string]any{"slot_ids": slots}}
	m, ok := snd.MessageFor(ctx, grab)
	if !ok || m.Moment != MomentGrab || m.Subject != "Sent to download: Copper Valley S01E01 to S01E03 (3 episodes)" || m.Movie != nil ||
		m.Show == nil || m.Show.TVDB != "200" || len(m.Episodes) != 3 || !strings.Contains(m.Text, "S01E02 Flood\nS01E03") {
		t.Fatalf("grab %+v", m)
	}
	imp := domain.Event{Kind: "import_partly_done", TitleID: &tid, SlotID: &slots[0], DownloadID: &dl, Summary: "Import partly_done: 2 placed (S01E01, S01E02)",
		Data: map[string]any{"slot_ids": []any{float64(slots[0]), float64(slots[1]), float64(slots[2])}, "placed_slot_ids": []any{float64(slots[0]), float64(slots[1])}}}
	ms := snd.MessagesFor(ctx, imp)
	if len(ms) != 1 || ms[0].Subject != "In the Library: Copper Valley S01E01 to S01E02 (2 episodes)" || len(ms[0].Episodes) != 2 {
		t.Fatalf("pack %+v", ms)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackEach}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: "some"}, domain.System); err == nil {
		t.Fatal("an odd setting was saved")
	}
	ms = snd.MessagesFor(ctx, imp)
	if len(ms) != 2 || ms[0].Subject != "In the Library: Copper Valley S01E01" || ms[1].Subject != "In the Library: Copper Valley S01E02" ||
		ms[1].Text != "S01E02 Flood is in the Library" || len(ms[1].Episodes) != 1 {
		t.Fatalf("each %+v", ms)
	}
	dec := domain.Event{Kind: domain.EventNeedsDecision, TitleID: &tid, SlotID: &one, Summary: "Needs a decision: Copper Valley"}
	if m, _ := snd.MessageFor(ctx, dec); m.Subject != "Needs a decision: Copper Valley S01E02" {
		t.Errorf("decision %q", m.Subject)
	}
	ren := domain.Event{Kind: domain.EventRenamed, TitleID: &tid, Summary: "Renamed a.mkv to b.mkv", Data: map[string]any{"slot_ids": []any{float64(one)}}}
	if m, _ := snd.MessageFor(ctx, ren); m.Subject != "Renamed: Copper Valley S01E02" {
		t.Errorf("rename %q", m.Subject)
	}
	del := domain.Event{Kind: domain.EventEpisodesRemoved, TitleID: &tid, Summary: "Deleted Copper Valley season 1 from the Library",
		Data: map[string]any{"episodes": []any{"S01E01", "S01E02"}}}
	if m, ok := snd.MessageFor(ctx, del); !ok || m.Moment != MomentDelete || m.Subject != "Deleted: Copper Valley S01E01 to S01E02 (2 episodes)" {
		t.Errorf("delete %+v", m)
	}
	whole := domain.Event{Kind: domain.EventTitleRemoved, TitleID: &tid, Summary: "Deleted Copper Valley from the Library"}
	if m, _ := snd.MessageFor(ctx, whole); m.Subject != "Deleted: Copper Valley" || m.Show == nil {
		t.Errorf("show delete %+v", m)
	}
	b, _ := json.Marshal(Payload{Show: ms[0].Show, Episodes: ms[0].Episodes})
	if !strings.Contains(string(b), `"show":{"id":`) || !strings.Contains(string(b), `"episodes":[{"season":1,"number":1,"name":"The Mill"}]`) {
		t.Errorf("payload %s", b)
	}
}

// With pack_alerts each, a big pack sends at most each_limit
// per-episode Alerts and one summary Alert for the rest.
func TestPackEachIsCapped(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid, slots := showWorld(t, s)
	dl := int64(7)
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackEach, EachLimit: 1}, domain.System); err != nil {
		t.Fatal(err)
	}
	imp := domain.Event{Kind: "import_done", TitleID: &tid, SlotID: &slots[0], DownloadID: &dl, Summary: "Imported",
		Data: map[string]any{"placed_slot_ids": []any{float64(slots[0]), float64(slots[1]), float64(slots[2])}}}
	ms := snd.MessagesFor(ctx, imp)
	if len(ms) != 2 || ms[0].Subject != "In the Library: Copper Valley S01E01" ||
		ms[1].Subject != "In the Library: Copper Valley S01E02 to S01E03 (2 episodes)" || len(ms[1].Episodes) != 2 ||
		!strings.Contains(ms[1].Text, "2 more episodes") || !strings.Contains(ms[1].Text, "S01E02 Flood\nS01E03") {
		t.Fatalf("capped %+v", ms)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackEach, EachLimit: MaxEachLimit + 1}, domain.System); err == nil {
		t.Fatal("a limit over the cap was saved")
	}
	if st := ReadSettings(ctx, s.Read()); st.EachLimit != 1 {
		t.Fatalf("limit %+v", st)
	}
}

// pack_alerts lives in the alerts group; an old row in links is
// still read until it is saved again.
func TestAlertSettingsGroup(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if st := ReadSettings(ctx, s.Read()); st.PackAlerts != PackOne || st.EachLimit != DefaultEachLimit {
		t.Fatalf("defaults %+v", st)
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'pack_alerts', 'each', '', '{}', '', '{}')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if st := ReadSettings(ctx, s.Read()); st.PackAlerts != PackEach {
		t.Fatalf("old row not read %+v", st)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackOne, EachLimit: 5}, domain.System); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT COUNT(*) FROM settings WHERE grp = 'links' AND key = 'pack_alerts'").Scan(&n)
	var g string
	_ = s.Read().QueryRow("SELECT grp FROM settings WHERE key = 'pack_alerts'").Scan(&g)
	if n != 0 || g != "alerts" {
		t.Fatalf("saved to %q, old rows %d", g, n)
	}
	if st := ReadSettings(ctx, s.Read()); st.PackAlerts != PackOne || st.EachLimit != 5 {
		t.Fatalf("after save %+v", st)
	}
}

// An episode Alert with a long pack of non-ASCII episode
// names stays within Discord's 2000 characters with whole letters, and an
// episode name cannot add a mail header.
func TestEpisodeAlertFormatting(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, posts := fakeDiscord(t)
	if _, err := Add(ctx, s, v, Input{Name: "Discord", Kind: "discord", URL: srv.URL + "/api/webhooks/1/abc", AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	port, mails := fakeSMTP(t)
	if _, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPSecurity: "none",
		SMTPFrom: "holyarrs@example.test", StaffTo: "luis@example.test", AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	var eps []Episode
	for i := 1; i <= 120; i++ {
		eps = append(eps, Episode{Season: 1, Number: i, Name: "Ünïcödé chapter with a long name ✓"})
	}
	subject := "In the Library: Copper Valley\r\nBcc: evil@example.test\n " + episodeWords(eps) + " Ünïcödé " + strings.Repeat("✓", 300)
	if err := snd.Send(ctx, Message{Moment: MomentImport, Subject: subject, Text: episodeLines(eps), Show: &Show{Name: "Copper Valley"}, Episodes: eps}); err != nil {
		t.Fatal(err)
	}
	p := posts()
	if len(p) != 1 {
		t.Fatalf("posts %v", p)
	}
	c, _ := p[0]["content"].(string)
	if n := utf8.RuneCountInString(c); n > 2000 || !utf8.ValidString(c) || strings.ContainsRune(c, utf8.RuneError) || !strings.HasSuffix(c, "…") {
		t.Fatalf("discord content %d runes, ends %q", n, c[len(c)-20:])
	}
	m := mails()
	if len(m) != 1 {
		t.Fatalf("mails %v", m)
	}
	head, _, _ := strings.Cut(m[0].data, "\r\n\r\n")
	for _, line := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") || len(line) > 998 {
			t.Fatalf("header line %q", line)
		}
		for _, r := range line {
			if r > 127 {
				t.Fatalf("raw non-ASCII in header %q", line)
			}
		}
	}
	dec := new(mime.WordDecoder)
	_, raw, _ := strings.Cut(head, "\r\nSubject: ")
	raw, _, _ = strings.Cut(raw, "\r\nDate:")
	subj, err := dec.DecodeHeader(strings.ReplaceAll(raw, "\r\n ", " "))
	if err != nil || utf8.RuneCountInString(subj) > 200 {
		t.Fatalf("subject %q %v", subj, err)
	}
	if !strings.Contains(subj, "Copper Valley") || !strings.Contains(subj, "Ünïcödé") || strings.ContainsAny(subj, "\r\n") {
		t.Fatalf("subject %q", subj)
	}
}
