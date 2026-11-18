package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate/faketranslator"
)

// dirtySRT has a credit first, a sound line, a speaker name and a line of
// dialogue, in Windows-1252 (the \xe9 is é), as many sites still send.
const dirtySRT = "1\r\n00:00:01,000 --> 00:00:02,000\r\nSubtitles by Lab Team\r\n\r\n" +
	"2\r\n00:00:03,000 --> 00:00:04,000\r\n[door creaks]\r\n\r\n" +
	"3\r\n00:00:05,000 --> 00:00:06,000\r\nMAN: The caf\xe9 is closed.\r\n\r\n" +
	"4\r\n00:00:07,000 --> 00:00:08,000\r\nThen we wait.\r\n"

var allSteps = subtext.CleanupOptions{RemoveHI: true, RemoveTags: true, RemoveEmoji: true, OCRFixes: true, CommonFixes: true,
	FixUppercase: true, RemoveAds: true, FixOverlaps: true}

// setCleanup stores rows as the owner's cleanup settings and returns them.
func setCleanup(t *testing.T, f *fix, rows map[string]subtext.CleanupOptions) Settings {
	t.Helper()
	set := ReadSettings(context.Background(), f.st.Read())
	set.Cleanup = rows
	if err := SaveSettings(context.Background(), f.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	return ReadSettings(context.Background(), f.st.Read())
}

// heldFiles lists the files in the Library's holding folder, relative to it.
func heldFiles(t *testing.T, f *fix) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(filepath.Join(f.lib, importer.HoldingDir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(f.lib, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// A site's SRT is cleaned with its language's steps and the original is held;
// a language without a row of its own is saved as it came.
func TestCleanupSave_SiteSubtitle(t *testing.T) {
	f := newFix(t)
	site := fakesite.Lab()
	site.Subs = append(site.Subs, fakesite.Sub{ID: "lantern-en", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", ByID: true, Trusted: true, Format: "srt", Text: dirtySRT})
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"}, Want{Language: "fr"})
	setCleanup(t, f, map[string]subtext.CleanupOptions{"en": {RemoveAds: true, RemoveHI: true}})
	res, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 2 || res.Outcomes[0].Result != "saved" || res.Outcomes[1].Result != "saved" {
		t.Fatalf("%+v", res.Outcomes)
	}
	en, err := os.ReadFile(f.abs(res.Outcomes[0].Path))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:05,000 --> 00:00:06,000\nThe café is closed.\n\n2\n00:00:07,000 --> 00:00:08,000\nThen we wait.\n\n"
	if string(en) != want {
		t.Fatalf("cleaned file %q", en)
	}
	fr, _ := os.ReadFile(f.abs(res.Outcomes[1].Path))
	if !strings.Contains(string(fr), "\r\n") || !strings.Contains(string(fr), "Made-up line") {
		t.Fatalf("French file was touched: %q", fr)
	}

	// The Event says what changed and where the original is kept.
	var note map[string]any
	for _, e := range eventsOf(t, f.st, EventSaved) {
		if e.Data["language"] == "en" {
			note, _ = e.Data["cleanup"].(map[string]any)
		} else if e.Data["cleanup"] != nil {
			t.Errorf("French event has a cleanup: %v", e.Data)
		}
	}
	if note == nil || note["removed"] != 2.0 || note["changed"] != 1.0 || !relInLib(note["held"]) {
		t.Fatalf("event note %v", note)
	}
	held, err := os.ReadFile(f.abs(note["held"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	// The held copy is the download after decoding, so it is UTF-8 too.
	if string(held) != strings.ReplaceAll(dirtySRT, "\xe9", "é") || !utf8.Valid(en) {
		t.Fatalf("held %q", held)
	}
	if hs := heldFiles(t, f); len(hs) != 1 || !strings.HasPrefix(hs[0], importer.HoldingDir+"/") {
		t.Fatalf("held files %v", hs)
	}
}

// A hand-picked subtitle is cleaned too.
func TestCleanupSave_Choose(t *testing.T) {
	f := newFix(t)
	site := fakesite.Lab()
	site.Subs = append(site.Subs, fakesite.Sub{ID: "lantern-en", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010,
		Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Format: "srt", Text: dirtySRT})
	s, _ := svc(t, f, site)
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	setCleanup(t, f, map[string]subtext.CleanupOptions{"*": {RemoveAds: true}})
	rows, err := Sites(ctx, f.st.Read())
	if err != nil || len(rows) != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	p, err := s.Choose(ctx, f.movieFile, rows[0].Name, "lantern-en", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.abs(p))
	if strings.Contains(string(b), "Subtitles by") || !strings.Contains(string(b), "[door creaks]") {
		t.Fatalf("picked file %q", b)
	}
	if len(heldFiles(t, f)) != 1 {
		t.Fatalf("held %v", heldFiles(t, f))
	}
}

// saveWith saves text for the movie with the given cleanup and row.
func saveWith(t *testing.T, f *fix, s *Service, set Settings, row *Subtitle, text, format string) (string, map[string]any) {
	t.Helper()
	ctx := context.Background()
	fc, err := s.load(ctx, row.FileID)
	if err != nil {
		t.Fatal(err)
	}
	p, note, err := s.save(ctx, fc, row, text, format, set, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return p, note
}

func TestCleanupSave_HearingImpairedRules(t *testing.T) {
	hiText := "1\n00:00:01,000 --> 00:00:02,000\n[thunder]\nRun!\n"
	for _, c := range []struct {
		name     string
		wish     Want
		forced   bool
		wantPath string
		wantHI   bool
		kept     bool
	}{
		{"normal wish: lines go, file is normal", Want{Language: "en"}, false, "BluRay.en.srt", false, false},
		{"hi wish: lines stay", Want{Language: "en", HearingImpaired: HIOnly}, false, "BluRay.en.hi.srt", true, true},
		{"forced: lines stay", Want{Language: "en", Forced: true}, true, "BluRay.en.forced.srt", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFix(t)
			s, _ := svc(t, f, fakesite.Lab())
			wishAll(t, f, c.wish)
			set := setCleanup(t, f, map[string]subtext.CleanupOptions{"en": {RemoveHI: true}})
			row := fetchedRow(f.movieFile, "en")
			row.HearingImpaired, row.Forced = true, c.forced
			p, _ := saveWith(t, f, s, set, row, hiText, "srt")
			b, _ := os.ReadFile(f.abs(p))
			if !strings.HasSuffix(p, c.wantPath) || strings.Contains(string(b), "[thunder]") != c.kept {
				t.Fatalf("%s: %q", p, b)
			}
			rows, _ := ListFor(context.Background(), f.st.Read(), f.movieFile)
			for _, r := range rows {
				if r.Place == "beside" && r.HearingImpaired != c.wantHI {
					t.Fatalf("row hearing impaired %v", r.HearingImpaired)
				}
			}
			// The wish is met either way, so nothing is searched again.
			if st, _ := s.Look(context.Background(), f.movieFile, domain.System); len(st.Missing) != 0 {
				t.Fatalf("missing %+v", st.Missing)
			}
		})
	}
}

// A forced subtitle keeps its first cue and its capitals, but other steps run.
func TestCleanupSave_ForcedSkipsAdsAndCapitals(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	wishAll(t, f, Want{Language: "en", Forced: true})
	set := setCleanup(t, f, map[string]subtext.CleanupOptions{"*": allSteps})
	row := fetchedRow(f.movieFile, "en")
	row.Forced = true
	text := "1\n00:00:01,000 --> 00:00:02,000\nPOLICE STATION\n\n2\n00:00:03,000 --> 00:00:04,000\n<i>Visit www.example.com</i>\n"
	p, _ := saveWith(t, f, s, set, row, text, "srt")
	b, _ := os.ReadFile(f.abs(p))
	if string(b) != "1\n00:00:01,000 --> 00:00:02,000\nPOLICE STATION\n\n2\n00:00:03,000 --> 00:00:04,000\nVisit www.example.com\n\n" {
		t.Fatalf("%q", b)
	}
}

// Only SRT is cleaned. Other formats, and SRT with nothing to clean or every
// step off, are saved byte for byte with nothing held.
func TestCleanupSave_LeftAsTheyCame(t *testing.T) {
	ass := "[Script Info]\nTitle: x\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Subtitles by Lab Team\nDialogue: 0,0:00:03.00,0:00:04.00,Default,,0,0,0,,[noise] HELLO THERE\n"
	vtt := "WEBVTT\n\n00:01.000 --> 00:02.000 align:start\nSubtitles by Lab Team\n\n00:03.000 --> 00:04.000\n[noise] HELLO THERE\n"
	sub := "{25}{50}Subtitles by Lab Team\n{75}{100}[noise] HELLO THERE\n"
	clean := "7\r\n00:00:01,000 --> 00:00:02,000\r\nHello there.\r\n"
	for _, c := range []struct {
		name, format, text string
		cleanup            map[string]subtext.CleanupOptions
		skipped            bool
	}{
		{"ass", "ass", ass, map[string]subtext.CleanupOptions{"*": allSteps}, true},
		{"vtt", "vtt", vtt, map[string]subtext.CleanupOptions{"*": allSteps}, true},
		{"sub", "sub", sub, map[string]subtext.CleanupOptions{"*": allSteps}, true},
		{"srt with nothing to clean", "srt", clean, map[string]subtext.CleanupOptions{"*": allSteps}, false},
		{"srt with every step off", "srt", dirtySRT, map[string]subtext.CleanupOptions{"en": {}}, false},
		{"srt in a language without a row", "srt", dirtySRT, map[string]subtext.CleanupOptions{"fr": allSteps}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFix(t)
			s, _ := svc(t, f, fakesite.Lab())
			wishAll(t, f, Want{Language: "en"})
			set := setCleanup(t, f, c.cleanup)
			p, note := saveWith(t, f, s, set, fetchedRow(f.movieFile, "en"), c.text, c.format)
			b, _ := os.ReadFile(f.abs(p))
			if string(b) != c.text {
				t.Fatalf("file changed: %q", b)
			}
			if (note["skipped"] != nil) != c.skipped || note["held"] != nil {
				t.Fatalf("note %v", note)
			}
			if hs := heldFiles(t, f); len(hs) != 0 {
				t.Fatalf("held %v", hs)
			}
		})
	}
}

// A save that fails leaves no held original behind, and a file HolyArrs did
// not save is never touched or held.
func TestCleanupSave_FailuresHoldNothing(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	wishAll(t, f, Want{Language: "en"})
	set := setCleanup(t, f, map[string]subtext.CleanupOptions{"*": allSteps})
	ctx := context.Background()
	fc, _ := s.load(ctx, f.movieFile)

	renameInto = func(from, to string) error { return errors.New("disk full") }
	_, _, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), dirtySRT, "srt", set, nil, domain.System)
	renameInto = os.Rename
	if err == nil {
		t.Fatal("no error")
	}
	if hs := heldFiles(t, f); len(hs) != 0 {
		t.Fatalf("held after a failed save: %v", hs)
	}

	mine := f.abs(SubtitleName(movieRel, "en", false, false, "hi", "srt"))
	if err := os.WriteFile(mine, []byte(dirtySRT), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.save(ctx, fc, fetchedRow(f.movieFile, "en"), dirtySRT, "srt", set, nil, domain.System); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("not refused: %v", err)
	}
	if b, _ := os.ReadFile(mine); string(b) != dirtySRT {
		t.Fatalf("person's file changed: %q", b)
	}
	if hs := heldFiles(t, f); len(hs) != 0 {
		t.Fatalf("held after a refused save: %v", hs)
	}
}

// Every Version's File is cleaned, a 4K one included.
func TestCleanupSave_FourKVersion(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, fakesite.Lab())
	wishAll(t, f, Want{Language: "en"})
	set := setCleanup(t, f, map[string]subtext.CleanupOptions{"en": {RemoveAds: true}})
	rel := "Quiet Harbour (2019) [4K]/Season 01/Quiet Harbour - S01E02 2160p.mkv"
	ts := database.FormatTime(now())
	var fileID int64
	err := f.st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`,
			ts, sysActor, ts, sysActor, f.ep1, f.show4K)
		if err != nil {
			return err
		}
		slot, _ := res.LastInsertId()
		res, err = tx.Exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, release_name, origin, placed_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy')`, ts, sysActor, ts, sysActor, "["+itoa(slot)+"]", f.show4K, rel, ts, "Quiet.Harbour.S01E02.2160p.WEB.h265-GRP")
		if err != nil {
			return err
		}
		fileID, _ = res.LastInsertId()
		_, err = tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", fileID, slot)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(f.abs(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.abs(rel), []byte("not a real video"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, note := saveWith(t, f, s, set, fetchedRow(fileID, "en"), dirtySRT, "srt")
	b, _ := os.ReadFile(f.abs(p))
	if !strings.HasPrefix(p, "Quiet Harbour (2019) [4K]/") || strings.Contains(string(b), "Subtitles by") || note["removed"] != 1 {
		t.Fatalf("%s %q %v", p, b, note)
	}
}

// Ad cues in a translation's source are dropped before the translator sees
// them, and the source file stays as it was.
func TestCleanupSave_TranslationSourceAds(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := newFix(t)
		ctx := context.Background()
		text := fakesite.Cue("Subtitles by Lab Team", 1000) + "\n" + fakesite.Cue("Hello there.", 3000) + "\n" + fakesite.Cue("Then we wait.", 6000)
		en := f.abs("The Lantern (2010)/The Lantern (2010) 1080p BluRay.en.srt")
		if err := os.WriteFile(en, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		ft := &faketranslator.OpenAI{}
		s, _ := withTranslator(t, f, ft)
		setCleanup(t, f, map[string]subtext.CleanupOptions{"pt": {RemoveAds: on}})
		res, err := s.Translate(ctx, f.movieFile, TranslateRequest{Language: "pt-br", From: "en", ByHand: true}, domain.System)
		if err != nil || res.Path == "" {
			t.Fatalf("%+v %v", res, err)
		}
		if strings.Contains(ft.User, "Subtitles by") == on {
			t.Fatalf("ads on %v: translator was asked %q", on, ft.User)
		}
		got := readCues(t, f.abs(res.Path))
		if on && (len(got) != 2 || got[0].Text != "PT: Hello there.") {
			t.Fatalf("cues %+v", got)
		}
		if b, _ := os.ReadFile(en); string(b) != text {
			t.Fatalf("source changed: %q", b)
		}
		es := eventsOf(t, f.st, EventTranslated)
		if len(es) != 1 {
			t.Fatalf("%d events", len(es))
		}
		note, _ := es[0].Data["cleanup"].(map[string]any)
		if on && note["source_ads_removed"] != 1.0 || !on && note != nil {
			t.Fatalf("ads on %v: note %v", on, es[0].Data)
		}
	}
}
