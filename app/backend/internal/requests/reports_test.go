package requests

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// libCopies is a title put straight into the Library: one item per numbers
// entry, one Version per label, and a file in every Slot when withFiles.
type libCopies struct {
	title    int64
	items    []int64
	versions []int64
	files    map[[2]int64]int64 // [item, version] -> file
	slots    map[[2]int64]int64
}

func addCopies(t *testing.T, st *database.Store, kind, name string, numbers []mediafiles.Numbers, labels []string, withFiles bool) libCopies {
	t.Helper()
	c := libCopies{files: map[[2]int64]int64{}, slots: map[[2]int64]int64{}}
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		var folder int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folder); err != nil {
			return err
		}
		rules := &mediafiles.PickRules{Name: "Rules " + name, Kinds: []string{kind}, Revision: 1}
		if _, err := database.Insert(tx, "pick_rules", rules, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: kind, GateDriver: "none", Name: name, SortName: strings.ToLower(name), MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		c.title = ti.ID
		itemKind := map[string]string{"movie": "movie", "show": "episode"}[kind]
		for _, n := range numbers {
			it := &mediafiles.Item{TitleID: ti.ID, Kind: itemKind, Numbers: n, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			c.items = append(c.items, it.ID)
		}
		for i, l := range labels {
			v := &mediafiles.Version{TitleID: ti.ID, Label: l, PickRulesID: rules.ID, LibraryFolderID: folder, FolderName: name + " " + l, KeepUpToDate: true, IsMain: i == 0, AudioLanguages: []string{}}
			if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
				return err
			}
			c.versions = append(c.versions, v.ID)
			for _, it := range c.items {
				sl := &mediafiles.Slot{ItemID: it, VersionID: v.ID, State: mediafiles.SlotWanted}
				if withFiles {
					f := &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: name + "/" + l + "/" + strconv.FormatInt(it, 10) + ".mkv", AddedAt: time.Now().UTC(),
						Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
					if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
						return err
					}
					sl.State, sl.FileID = mediafiles.SlotInLibrary, &f.ID
					c.files[[2]int64{it, v.ID}] = f.ID
				}
				if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
					return err
				}
				c.slots[[2]int64{it, v.ID}] = sl.ID
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func intp(n int) *int { return &n }

func (f *fakeAlerts) all() []alerts.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]alerts.Message(nil), f.sent...)
}

func TestReportFixAndReopen(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c := addCopies(t, r.st, "movie", "Arrival", []mediafiles.Numbers{{}}, []string{"HD"}, true)

	v, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: c.title, Kind: "sound", Note: "  no sound after 20 minutes "})
	if err != nil {
		t.Fatal(err)
	}
	key := [2]int64{c.items[0], c.versions[0]}
	if v.State != domain.ReportOpen || v.KindLabel != "Sound" || v.Note != "no sound after 20 minutes" || v.MadeBy == nil || v.MadeBy.Name != "ana" ||
		v.ItemID == nil || *v.ItemID != c.items[0] || v.FileID == nil || *v.FileID != c.files[key] || v.VersionID == nil || v.TitleName != "Arrival" {
		t.Fatalf("report %+v", v)
	}
	e := hasEvent(r.events(t), domain.EventReportMade)
	if e == nil || e.TitleID == nil || *e.TitleID != c.title || e.SlotID == nil || *e.SlotID != c.slots[key] || e.Data["kind"] != "sound" || e.Summary != "ana reported a problem with Arrival: Sound" {
		t.Fatalf("made event %+v", e)
	}
	r.svc.Wait()
	if last := r.fake.last(); last.Moment != alerts.MomentReported || len(last.People) != 0 || last.Report == nil || last.Report.Kind != "sound" {
		t.Fatalf("staff alert %+v", last)
	}

	// The same open problem twice is refused; another person or kind is its own report.
	if _, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: c.title, Kind: "sound"}); !errors.Is(err, ErrReportedAlready) {
		t.Fatalf("twice: %v", err)
	}
	bo, err := r.svc.Report(ctx, r.bo, ReportInput{TitleID: c.title, Kind: "sound"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: c.title, Kind: "picture"}); err != nil {
		t.Fatal(err)
	}
	if mine, _ := r.svc.ListReports(ctx, r.ana, false, "", 0, 50); len(mine) != 2 {
		t.Fatalf("ana's reports %+v", mine)
	}
	if all, _ := r.svc.ListReports(ctx, r.luis, true, domain.ReportOpen, c.title, 50); len(all) != 3 {
		t.Fatalf("all reports %+v", all)
	}

	// Fixing with a reply tells only ana: a message and the Alert.
	before := len(r.fake.all())
	fixed, err := r.svc.FixReport(ctx, r.luis, v.ID, " swapped the file ")
	if err != nil {
		t.Fatal(err)
	}
	if fixed.State != domain.ReportFixed || fixed.ClosedBy == nil || fixed.ClosedBy.Name != "luis" || fixed.ClosedAt == nil || fixed.Reply != "swapped the file" {
		t.Fatalf("fixed %+v", fixed)
	}
	r.svc.Wait()
	sent := r.fake.all()[before:]
	if len(sent) != 1 || sent[0].Moment != alerts.MomentFixed || len(sent[0].People) != 1 || sent[0].People[0].Name != "ana" || sent[0].Report.Reply != "swapped the file" {
		t.Fatalf("fixed alerts %+v", sent)
	}
	ms, unread, _ := Messages(ctx, r.st.Read(), r.ana.UserID, 10)
	if unread != 1 || ms[0].Kind != "fixed" || ms[0].ReportID == nil || *ms[0].ReportID != v.ID || !strings.Contains(ms[0].Text, "Reply: swapped the file") {
		t.Fatalf("ana messages %+v", ms)
	}
	if ms, _, _ := Messages(ctx, r.st.Read(), r.bo.UserID, 10); len(ms) != 0 {
		t.Fatalf("bo messages %+v", ms)
	}
	if e := hasEvent(r.events(t), domain.EventReportFixed); e == nil || e.Data["reply"] != "swapped the file" || e.Data["made_by"] != "ana" {
		t.Fatalf("fixed event %+v", e)
	}
	if _, err := r.svc.FixReport(ctx, r.luis, v.ID, ""); !errors.Is(err, ErrReportFixed) {
		t.Fatalf("fix twice: %v", err)
	}
	if n, _ := OpenReports(ctx, r.st.Read()); n != 2 {
		t.Fatalf("open %d", n)
	}

	// The sound is gone again: ana reports again, so the old one cannot
	// open again until the new one is fixed.
	again, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: c.title, Kind: "sound"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.ReopenReport(ctx, r.luis, v.ID); !errors.Is(err, ErrReportedAlready) {
		t.Fatalf("reopen beside an open one: %v", err)
	}
	if _, err := r.svc.FixReport(ctx, r.luis, again.ID, ""); err != nil {
		t.Fatal(err)
	}
	reopened, err := r.svc.ReopenReport(ctx, r.luis, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State != domain.ReportOpen || reopened.ClosedBy != nil || reopened.Reply != "" {
		t.Fatalf("reopened %+v", reopened)
	}
	if e := hasEvent(r.events(t), domain.EventReportReopened); e == nil || e.Data["reopened_by"] != "luis" {
		t.Fatalf("reopened event %+v", e)
	}
	if _, err := r.svc.ReopenReport(ctx, r.luis, bo.ID); !errors.Is(err, ErrReportOpen) {
		t.Fatalf("reopen an open one: %v", err)
	}
	if _, err := r.svc.FixReport(ctx, r.luis, 9999, ""); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("fix missing: %v", err)
	}
}

func TestReportChecksAndEpisodes(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	movie := addCopies(t, r.st, "movie", "Arrival", []mediafiles.Numbers{{}}, []string{"HD"}, true)
	empty := addCopies(t, r.st, "movie", "Wanted Only", []mediafiles.Numbers{{}}, []string{"HD"}, false)
	show := addCopies(t, r.st, "show", "Severance", []mediafiles.Numbers{{Season: intp(2), Episode: intp(3)}, {Season: intp(2), Episode: intp(4)}}, []string{"HD", "4K"}, true)

	for _, tc := range []struct {
		in   ReportInput
		want error
	}{
		{ReportInput{TitleID: movie.title, Kind: "smell"}, ErrReportKind},
		{ReportInput{TitleID: movie.title, Kind: "other", Note: strings.Repeat("x", 501)}, ErrNoteLength},
		{ReportInput{TitleID: empty.title, Kind: "other"}, ErrNotInLibrary},
		{ReportInput{TitleID: 9999, Kind: "other"}, database.ErrNotFound},
		{ReportInput{TitleID: movie.title, Kind: "other", ItemID: show.items[0]}, ErrReportPart},
		{ReportInput{TitleID: movie.title, Kind: "other", Season: intp(1)}, ErrReportPart},
		{ReportInput{TitleID: show.title, Kind: "other", ItemID: show.items[0], Season: intp(1)}, ErrReportPart},
		{ReportInput{TitleID: show.title, Kind: "other", VersionID: movie.versions[0]}, ErrReportPart},
		{ReportInput{TitleID: show.title, Kind: "other", Season: intp(7)}, ErrReportPart},
	} {
		if _, err := r.svc.Report(ctx, r.ana, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%+v: %v, want %v", tc.in, err, tc.want)
		}
	}

	// An episode with an HD and a 4K copy: no Version picked keeps no file.
	v, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: show.title, ItemID: show.items[0], Kind: "wont_play"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Season == nil || *v.Season != 2 || v.Where != "S02E03" || v.FileID != nil || v.VersionID != nil || v.KindLabel != "Won't play" {
		t.Fatalf("episode report %+v", v)
	}
	v, err = r.svc.Report(ctx, r.ana, ReportInput{TitleID: show.title, ItemID: show.items[0], VersionID: show.versions[1], Kind: "subtitles"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Where != "S02E03 (4K)" || v.FileID == nil || *v.FileID != show.files[[2]int64{show.items[0], show.versions[1]}] {
		t.Fatalf("4K episode report %+v", v)
	}
	// The same problem on the episode's other Version is its own report.
	if _, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: show.title, ItemID: show.items[0], VersionID: show.versions[0], Kind: "subtitles"}); err != nil {
		t.Fatalf("HD episode report: %v", err)
	}
	// A whole season.
	v, err = r.svc.Report(ctx, r.ana, ReportInput{TitleID: show.title, Season: intp(2), Kind: "sound"})
	if err != nil || v.Where != "Season 2" || v.ItemID != nil {
		t.Fatalf("season report %+v %v", v, err)
	}
	// The note limit counts characters, not bytes.
	if _, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: movie.title, Kind: "other", Note: strings.Repeat("ü", 400)}); err != nil {
		t.Fatalf("400 umlauts: %v", err)
	}
}

func TestReportOpenCap(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	show := addCopies(t, r.st, "show", "Severance", []mediafiles.Numbers{{Season: intp(1), Episode: intp(1)}, {Season: intp(1), Episode: intp(2)}}, []string{"HD"}, true)
	n := 0
	for _, kind := range domain.ReportKinds {
		for _, in := range []ReportInput{{}, {Season: intp(1)}, {ItemID: show.items[0]}, {ItemID: show.items[1]}} {
			in.TitleID, in.Kind = show.title, kind
			if _, err := r.svc.Report(ctx, r.ana, in); err != nil {
				t.Fatalf("report %d: %v", n, err)
			}
			n++
		}
	}
	if n != maxOpenReports {
		t.Fatalf("made %d reports, cap is %d", n, maxOpenReports)
	}
	if _, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: show.title, Kind: "other", Note: "one more"}); !errors.Is(err, ErrTooManyReports) {
		t.Fatalf("over the cap: %v", err)
	}
	// Someone else is not held by ana's cap.
	if _, err := r.svc.Report(ctx, r.bo, ReportInput{TitleID: show.title, Kind: "other"}); err != nil {
		t.Fatal(err)
	}
}
