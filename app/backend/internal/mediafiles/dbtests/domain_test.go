package dbtests

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func ptr[T any](v T) *T { return &v }

var owner = domain.Actor{Kind: "user", ID: 1}

func roundTrip(t *testing.T, s *database.Store, table string, v any, fresh any) {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = database.Insert(tx, table, v, owner)
		return err
	})
	if err != nil {
		t.Fatalf("%s: insert: %v", table, err)
	}
	if id == 0 {
		t.Fatalf("%s: no id", table)
	}
	db := s.Read()
	if table == "events" {
		db = s.ReadEvents()
	}
	if err := database.Get(ctx, db, table, id, fresh); err != nil {
		t.Fatalf("%s: get: %v", table, err)
	}
	if !reflect.DeepEqual(v, fresh) {
		t.Fatalf("%s: read back differs\n got  %+v\n want %+v", table, fresh, v)
	}
}

func TestSeededGoodEveryday(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var pr mediafiles.PickRules
	if err := database.Get(context.Background(), s.Read(), "pick_rules", id, &pr); err != nil {
		t.Fatal(err)
	}
	if !pr.Preset || pr.Revision != 1 {
		t.Fatalf("preset flags wrong: %+v", pr)
	}
	if !reflect.DeepEqual(pr.Body, mediafiles.GoodEveryday()) {
		t.Fatalf("seeded body differs from mediafiles.GoodEveryday()\n got  %+v\n want %+v", pr.Body, mediafiles.GoodEveryday())
	}
}

func TestCreateAndReadEveryRecord(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 9, 30, 0, 123456789, time.UTC)

	roundTrip(t, s, "users", &domain.User{Name: "luis", Role: "owner", PasswordHash: "x", State: "active"}, &domain.User{})

	var prID int64
	_ = s.Read().QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&prID)
	body := mediafiles.GoodEveryday()
	body.MinPoints = -5
	roundTrip(t, s, "pick_rules", &mediafiles.PickRules{Name: "Mine", Kinds: []string{"movie"}, Revision: 2, Body: body}, &mediafiles.PickRules{})

	lf := &mediafiles.LibraryFolder{Path: "/movies", Kind: "movie", Label: "Movies", DefaultPickRules: &prID,
		FreeSpace: 1 << 40, Health: "ok", HealthCheckedAt: &now, SameDiskAs: []int64{1}}
	roundTrip(t, s, "library_folders", lf, &mediafiles.LibraryFolder{})

	ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "The Lantern Keepers", SortName: "lantern keepers", Year: ptr(2021),
		OriginalLanguage: ptr("en"), Aliases: []string{"Lantern Keepers"}, SourceIDs: map[string]string{"tmdb": "1"},
		MetadataSource: "local", KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released",
		WhyAdded: "by_hand", Status: "released", State: "active", RefreshedAt: &now}
	roundTrip(t, s, "titles", ti, &mediafiles.Title{})

	pa := &mediafiles.Part{TitleID: ti.ID, Kind: "season", Number: ptr(1), Name: "Season 1", ReleaseDate: &now,
		SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{{Name: "CD", TrackCount: 10, Preferred: true}}}
	roundTrip(t, s, "parts", pa, &mediafiles.Part{})

	it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", Runtime: ptr(118), KeepUpToDate: true, AirDate: &now}
	roundTrip(t, s, "items", it, &mediafiles.Item{})

	ve := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: prID, LibraryFolderID: lf.ID,
		FolderName: "The Lantern Keepers (2021)", KeepUpToDate: true, IsMain: true}
	roundTrip(t, s, "versions", ve, &mediafiles.Version{})

	so := &domain.Source{Name: "fake", Protocol: "torrent", Format: "torznab", URL: "http://fake:9117",
		UseFor: domain.SourceUse{AutoSearch: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
		MinSeeders: ptr(1), Priority: 25, State: "enabled"}
	roundTrip(t, s, "sources", so, &domain.Source{})

	jb := &domain.Job{Kind: "search_slot", Subject: &domain.Subject{Type: "slot", ID: 1}, Params: map[string]any{"x": "y"},
		Priority: "normal", State: "queued", MaxAttempts: 5, RunAfter: now, AskedBy: owner, DedupeKey: ptr("search_slot:1")}
	roundTrip(t, s, "jobs", jb, &domain.Job{})

	re := &domain.Release{SourceID: so.ID, Name: "The.Lantern.Keepers.2021.1080p.WEB-DL-QUILL", LinkRef: "r1",
		Protocol: "torrent", Size: 4 << 30, PublishedAt: &now, Seeders: ptr(85), Flags: []string{},
		Parsed: map[string]any{"title": "The Lantern Keepers"}, MatchedSlotIDs: []int64{1},
		Verdict: map[string]any{"outcome": "chosen", "passed": true}, FoundBy: "auto_search", JobID: &jb.ID, State: "judged"}
	roundTrip(t, s, "releases", re, &domain.Release{})

	dl := &domain.Downloader{Name: "qbit", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://qbit:8080",
		Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{{Theirs: "/downloads", Ours: "/data/downloads"}},
		AfterImport: "leave", SeedGoal: domain.SeedGoal{Ratio: ptr(1.0)}, State: "ok"}
	roundTrip(t, s, "downloaders", dl, &domain.Downloader{})

	dn := &domain.Download{DownloaderID: dl.ID, ReleaseID: &re.ID, SlotIDs: []int64{1}, ExternalID: "abc",
		Progress: 0.5, Speed: 1000, State: "downloading", FilesSeen: []domain.SeenFile{{Name: "a.mkv", Size: 1}},
		SavePath: "/downloads", StartedAt: &now, StartedBy: owner}
	roundTrip(t, s, "downloads", dn, &domain.Download{})

	im := &mediafiles.Import{DownloadID: &dn.ID, FromPath: "/downloads/a", Kind: "movie", Mode: "auto",
		Plan:      []mediafiles.PlanStep{{SourceFile: "a.mkv", TargetSlotIDs: []int64{1}, TargetPath: "x/a.mkv", Method: "hardlink", Confidence: 0.98, Why: []string{"title"}}},
		Result:    []mediafiles.PlaceResult{{SourceFile: "a.mkv", Outcome: "placed", PlacedBy: "copy", FellBack: true}},
		DecidedBy: &owner, State: "done"}
	roundTrip(t, s, "imports", im, &mediafiles.Import{})

	fi := &mediafiles.File{SlotIDs: []int64{1}, VersionID: ve.ID, RelativePath: "x/a.mkv", Size: 1, AddedAt: now,
		Quality: mediafiles.Quality{Resolution: 1080, Source: "web_dl"}, Languages: []string{"en"}, MediaInfo: map[string]any{},
		ReleaseName: ptr(re.Name), Group: ptr("QUILL"), Origin: "imported", PlacedBy: "copy", FellBack: true, Role: "main", State: "present"}
	roundTrip(t, s, "files", fi, &mediafiles.File{})

	sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ve.ID, State: mediafiles.SlotInLibrary, FileID: &fi.ID, GoodEnough: true,
		ActiveDownloadID: &dn.ID, LastReason: ptr("placed"), NextTryAt: &now}
	roundTrip(t, s, "slots", sl, &mediafiles.Slot{})

	roundTrip(t, s, "schedules", &domain.Schedule{Kind: "round_trip_check", EverySeconds: 900, NextRun: &now, Enabled: true}, &domain.Schedule{})
	roundTrip(t, s, "settings", &domain.Setting{Group: "this_app", Key: "url_base", Value: "/"}, &domain.Setting{})

	ev := &domain.Event{At: now, Actor: owner, Kind: "added", TitleID: &ti.ID, Summary: "Added The Lantern Keepers",
		Data: map[string]any{"why": "by_hand"}, GroupKey: "t1"}
	var evID int64
	err := s.WriteEvents(ctx, func(tx *sql.Tx) error {
		var err error
		evID, err = database.Insert(tx, "events", ev, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var back domain.Event
	if err := database.Get(ctx, s.ReadEvents(), "events", evID, &back); err != nil || !reflect.DeepEqual(*ev, back) {
		t.Fatalf("event round trip: %v\n got %+v\n want %+v", err, back, *ev)
	}
}

func TestOneOwner(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "users", &domain.User{Name: "a", Role: "owner", PasswordHash: "x", State: "active"}, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "users", &domain.User{Name: "b", Role: "owner", PasswordHash: "x", State: "active"}, domain.System)
		return err
	})
	if err == nil {
		t.Fatal("a second owner was accepted")
	}
}

func TestSlotStateCheckAndMoves(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	var slotID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var prID int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&prID)
		lf := &mediafiles.LibraryFolder{Path: "/m", Kind: "movie", Label: "M", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "A", SortName: "a", MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active"}
		for _, x := range []struct {
			t string
			v any
		}{{"library_folders", lf}, {"titles", ti}} {
			if _, err := database.Insert(tx, x.t, x.v, owner); err != nil {
				return err
			}
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, owner); err != nil {
			return err
		}
		ve := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: prID, LibraryFolderID: lf.ID, FolderName: "A"}
		if _, err := database.Insert(tx, "versions", ve, owner); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: ve.ID, State: "bogus"}, owner); err == nil {
			return errors.New("bogus slot state accepted by the table")
		}
		var err error
		slotID, err = database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: ve.ID, State: mediafiles.SlotWanted}, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mediafiles.MoveSlot(ctx, s, slotID, mediafiles.SlotInLibrary, "", owner); err == nil {
		t.Fatal("wanted -> in_library accepted")
	}
	if err := mediafiles.MoveSlot(ctx, s, slotID, mediafiles.SlotSearching, "search started", owner); err != nil {
		t.Fatal(err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, s.Read(), "slots", slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotSearching || sl.LastReason == nil || *sl.LastReason != "search started" {
		t.Fatalf("slot after move: %+v", sl)
	}
	if err := mediafiles.MoveSlot(ctx, s, 9999, mediafiles.SlotSearching, "", owner); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("missing slot: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	st := &domain.Setting{Group: "this_app", Key: "k", Value: "1"}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "settings", st, domain.System); err != nil {
			return err
		}
		st.Value = "2"
		return database.Update(tx, "settings", st, owner)
	})
	if err != nil {
		t.Fatal(err)
	}
	var back domain.Setting
	_ = database.Get(ctx, s.Read(), "settings", st.ID, &back)
	if back.Value != "2" || back.UpdatedBy != owner || back.CreatedBy != domain.System {
		t.Fatalf("update wrong: %+v", back)
	}
	if err := database.Get(ctx, s.Read(), "settings", 999, &back); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("want database.ErrNotFound, got %v", err)
	}
}

func TestSeededFourK(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var pr mediafiles.PickRules
	if err := database.Get(context.Background(), s.Read(), "pick_rules", id, &pr); err != nil {
		t.Fatal(err)
	}
	if !pr.Preset || !reflect.DeepEqual(pr.Body, mediafiles.FourK()) {
		t.Fatalf("seeded 4K preset differs from mediafiles.FourK()\n got  %+v\n want %+v", pr.Body, mediafiles.FourK())
	}
}

func TestSeededSaveSpaceAndBest(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	for name, want := range map[string]mediafiles.PickRulesBody{mediafiles.SaveSpaceName: mediafiles.SaveSpace(), mediafiles.BestName: mediafiles.Best()} {
		var id int64
		if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", name).Scan(&id); err != nil {
			t.Fatal(name, err)
		}
		var pr mediafiles.PickRules
		if err := database.Get(context.Background(), s.Read(), "pick_rules", id, &pr); err != nil {
			t.Fatal(err)
		}
		if !pr.Preset || !reflect.DeepEqual(pr.Body, want) {
			t.Fatalf("seeded %s preset differs\n got  %+v\n want %+v", name, pr.Body, want)
		}
	}
}

func TestSeededMusicPresets(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	for name, want := range map[string]mediafiles.PickRulesBody{mediafiles.LosslessFirstName: mediafiles.LosslessFirst(),
		mediafiles.Lossy320Name: mediafiles.Lossy320(), mediafiles.AnyMusicName: mediafiles.AnyMusic()} {
		var id int64
		if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", name).Scan(&id); err != nil {
			t.Fatal(name, err)
		}
		var pr mediafiles.PickRules
		if err := database.Get(context.Background(), s.Read(), "pick_rules", id, &pr); err != nil {
			t.Fatal(err)
		}
		if !pr.Preset || !reflect.DeepEqual(pr.Kinds, []string{"music"}) || !reflect.DeepEqual(pr.Body, want) || !mediafiles.IsMusicRules(pr.Body) {
			t.Fatalf("seeded %s differs\n got  %+v\n want %+v", name, pr.Body, want)
		}
	}
	if mediafiles.IsMusicRules(mediafiles.GoodEveryday()) || mediafiles.IsMusicRules(mediafiles.FourK()) {
		t.Fatal("video presets read as music")
	}
}
