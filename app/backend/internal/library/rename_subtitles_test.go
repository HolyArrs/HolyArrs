package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// putSubRow records a beside subtitle of a File, as the subtitle search does.
func putSubRow(t *testing.T, s *database.Store, fileID int64, rel, lang string) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, relative_path, origin, state)
			VALUES ('2026-10-13T00:00:00Z', '{}', '2026-10-13T00:00:00Z', '{}', ?, ?, 'beside', ?, 'fetched', 'present')`, fileID, lang, rel)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func subRows(t *testing.T, s *database.Store, fileID int64) map[string]string {
	t.Helper()
	rows, err := s.Read().Query("SELECT relative_path, state FROM subtitles WHERE file_id = ?", fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, st string
		if err := rows.Scan(&p, &st); err != nil {
			t.Fatal(err)
		}
		out[p] = st
	}
	return out
}

const subDir = "Dune - Part Two (2024)/"

// 4.16 gap (a): the subtitle files beside a renamed video follow it; the
// preview lists them and the rename does exactly that.
func TestRenameMovesSubtitlesBesideTheVideo(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	for _, n := range []string{"x.en.srt", "x.es.hi.srt", "x.fr.forced.ass", "x.extra.srt", "y.en.srt", "x.en.txt"} {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+n)), "sub:"+n)
	}
	putSubRow(t, s, f.ID, subDir+"x.es.hi.srt", "es")
	plan, hash, err := PlanRenameHashed(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	newBase := subDir + "Dune - Part Two (2024) 2160p WEB"
	want := map[string]string{subDir + "x.en.srt": newBase + ".en.srt", subDir + "x.es.hi.srt": newBase + ".es.hi.srt", subDir + "x.fr.forced.ass": newBase + ".fr.forced.ass"}
	if len(plan[0].Subtitles) != len(want) {
		t.Fatalf("subtitles %+v", plan[0].Subtitles)
	}
	for _, m := range plan[0].Subtitles {
		if want[m.From] != m.To || m.Skip != "" {
			t.Fatalf("move %+v", m)
		}
	}
	// The hash binds the subtitle moves too: a new one changes it.
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.de.srt")), "sub")
	if _, h2, _ := PlanRenameHashed(ctx, s.Read(), []int64{id}); h2 == hash {
		t.Fatal("a new subtitle did not change the plan hash")
	}
	_ = os.Remove(filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.de.srt")))

	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	for from, to := range want {
		if onDisk(lf, from) || !onDisk(lf, to) {
			t.Fatalf("%s did not follow to %s", from, to)
		}
		if b, _ := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(to))); string(b) != "sub:"+filepath.Base(from) {
			t.Fatalf("content of %s: %q", to, b)
		}
	}
	for _, m := range plan[0].Subtitles {
		if m.Result != "renamed" {
			t.Fatalf("result %+v", m)
		}
	}
	for _, other := range []string{"x.extra.srt", "y.en.srt", "x.en.txt"} {
		if !onDisk(lf, subDir+other) {
			t.Fatalf("%s is not a subtitle of x.mkv but moved", other)
		}
	}
	if rows := subRows(t, s, f.ID); rows[newBase+".es.hi.srt"] != "present" {
		t.Fatalf("subtitle row did not follow: %v", rows)
	}
}

// A subtitle whose new name is taken is skipped and stays; the video
// still moves, and a file at that name is never replaced.
func TestRenameSubtitleClashSkipsOnlyThatSubtitle(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	newBase := subDir + "Dune - Part Two (2024) 2160p WEB"
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt")), "mine")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(newBase+".en.srt")), "someone else's")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || len(plan[0].Subtitles) != 1 || !strings.Contains(plan[0].Subtitles[0].Skip, "already there") {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(newBase+".en.srt"))); string(b) != "someone else's" {
		t.Fatal("a file was replaced")
	}
	if !onDisk(lf, subDir+"x.en.srt") {
		t.Fatal("the skipped subtitle moved")
	}
}

// Case-only: a video renamed only in case takes its subtitles along.
func TestRenameCaseOnlyTakesSubtitles(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	rel := subDir + "Dune - Part Two (2024) 2160p web.mkv"
	putFile(t, s, lf, id, rel, "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"Dune - Part Two (2024) 2160p web.en.srt")), "sub")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || len(plan[0].Subtitles) != 1 || plan[0].Subtitles[0].Skip != "" {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	ents, _ := os.ReadDir(filepath.Join(lf.Path, "Dune - Part Two (2024)"))
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if !strings.Contains(strings.Join(names, "|"), "Dune - Part Two (2024) 2160p WEB.en.srt") {
		t.Fatalf("names on disk %v", names)
	}
}

// A rename cut off after the video moved finishes its subtitles next run;
// one cut off before keeps them pending until the video moves.
func TestRenameCrashFinishesSubtitles(t *testing.T) {
	for _, stage := range []string{"before_move", "after_move"} {
		t.Run(stage, func(t *testing.T) {
			svc, s, lf, id := renameWorld(t)
			ctx := context.Background()
			putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
			writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt")), "sub")
			plan, _ := PlanRename(ctx, s.Read(), []int64{id})
			renameCrash = func(at string) error {
				if at == stage {
					return errors.New("power cut")
				}
				return nil
			}
			_, err := svc.ApplyRename(ctx, plan, 0, domain.System)
			renameCrash = nil
			if err == nil {
				t.Fatal("no crash")
			}
			if !onDisk(lf, subDir+"x.en.srt") {
				t.Fatal("the subtitle moved before its video was done")
			}
			if _, err := svc.RecoverRenames(ctx, domain.System); err != nil {
				t.Fatal(err)
			}
			if onDisk(lf, subDir+"x.en.srt") || !onDisk(lf, plan[0].Subtitles[0].To) {
				t.Fatal("recovery did not finish the subtitle")
			}
		})
	}
}

func TestMissingSubtitleProofDoesNotStopUnrelatedRename(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	a := putFile(t, s, lf, id, subDir+"a.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")
	b := putFile(t, s, lf, id, subDir+"b.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "imported")
	sub := filepath.Join(lf.Path, filepath.FromSlash(subDir+"a.en.srt"))
	writeFile(t, sub, "subtitle")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan %+v: %v", plan, err)
	}
	if err := os.Remove(sub); err != nil {
		t.Fatal(err)
	}
	n, err := svc.ApplyRename(ctx, plan, 0, domain.System)
	if err == nil || n != 1 {
		t.Fatalf("renamed %d: %v", n, err)
	}
	results := map[int64]string{}
	for _, st := range plan {
		results[st.FileID] = st.Result
	}
	if !strings.HasPrefix(results[a.ID], "failed") || results[b.ID] != "renamed" {
		t.Fatalf("results %v", results)
	}
	if !onDisk(lf, subDir+"a.mkv") || fileRow(t, s, b.ID).RelativePath == b.RelativePath {
		t.Fatal("subtitle proof failure changed the failed File or stopped the good File")
	}
	var failedSteps int
	if err := s.Read().QueryRow("SELECT count(*) FROM rename_steps WHERE file_id = ?", a.ID).Scan(&failedSteps); err != nil {
		t.Fatal(err)
	}
	if failedSteps != 0 {
		t.Fatalf("failed File has %d orphan rename steps", failedSteps)
	}
}

// Review 4.16 (1): recovery reads the video step's state from the database.
// A crash after the video step was marked failed but before its subtitle
// steps were dropped must not move the subtitles alone; a video step that is
// done lets them finish; one still pending keeps them pending.
func TestRecoverRenamesChecksTheVideoStepInTheDatabase(t *testing.T) {
	for _, videoState := range []string{"failed", "done", "pending"} {
		t.Run(videoState, func(t *testing.T) {
			svc, s, lf, id := renameWorld(t)
			ctx := context.Background()
			f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
			writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt")), "sub")
			from := filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt"))
			to := filepath.Join(lf.Path, filepath.FromSlash(subDir+"y.en.srt"))
			vFrom := filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.mkv"))
			vTo := vFrom
			if videoState == "pending" {
				// A pending video whose source is gone fails in recovery.
				vFrom = filepath.Join(lf.Path, filepath.FromSlash(subDir+"gone.mkv"))
				vTo = filepath.Join(lf.Path, filepath.FromSlash(subDir+"y.mkv"))
			}
			err := s.Write(ctx, func(tx *sql.Tx) error {
				if _, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state) VALUES ('2026-10-13T00:00:00Z', ?, ?, ?, ?, ?)`,
					f.ID, vFrom, vTo, subDir+"y.mkv", videoState); err != nil {
					return err
				}
				_, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state, part) VALUES ('2026-10-13T00:00:00Z', ?, ?, ?, ?, 'pending', 'subtitle')`,
					f.ID, from, to, subDir+"y.en.srt")
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = svc.RecoverRenames(ctx, domain.System)
			moved := onDisk(lf, subDir+"y.en.srt")
			if videoState == "done" && (!moved || onDisk(lf, subDir+"x.en.srt")) {
				t.Fatal("subtitle of a moved video did not finish")
			}
			if videoState != "done" && (moved || !onDisk(lf, subDir+"x.en.srt")) {
				t.Fatalf("subtitle moved while its video step is %s", videoState)
			}
			var st string
			_ = s.Read().QueryRow("SELECT state FROM rename_steps WHERE part = 'subtitle'").Scan(&st)
			if videoState != "done" && st != "failed" {
				t.Fatalf("subtitle step %s, want failed", st)
			}
		})
	}
}

// Deleting a movie's files holds the subtitles beside them too, and the
// preview lists them.
func TestDeleteHoldsSubtitlesWithTheVideo(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "imported")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt")), "sub")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"notes.txt")), "keep")
	putSubRow(t, s, f.ID, subDir+"x.en.srt", "en")
	preview, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, false, domain.System)
	if err != nil || len(preview.Held) != 1 || len(preview.Held[0].Subtitles) != 1 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	plan, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	m := plan.Held[0].Subtitles[0]
	if m.To != preview.Held[0].Subtitles[0].To || m.Result != "held" || !strings.HasPrefix(m.To, importer.HoldingDir+"/") {
		t.Fatalf("held subtitle %+v", m)
	}
	if onDisk(lf, subDir+"x.en.srt") || !onDisk(lf, m.To) || !onDisk(lf, subDir+"notes.txt") {
		t.Fatal("subtitle not held, or another file touched")
	}
	if rows := subRows(t, s, f.ID); rows[subDir+"x.en.srt"] != "gone" {
		t.Fatalf("rows %v", rows)
	}
}

// vobsub is the start of a picture (VobSub) .sub: an MPEG pack header.
const vobsub = "\x00\x00\x01\xba\x44\x00\x04\x00\x04\x01\x01\x89\xc3\xf8"

// Review 4.16 (2): a VobSub .sub moves only with its .idx, as a pair, in a
// rename and in a delete; a .sub with no .idx moves only when it is clearly
// text (MicroDVD); a picture .sub alone is left where it is.
func TestRenameMovesVobSubAsAPair(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.idx")), "# VobSub index file, v7\nid: en, index: 0\n")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.fr.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.de.sub")), "{1}{25}Hallo\n")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	got := map[string]bool{}
	for _, m := range plan[0].Subtitles {
		got[path.Base(m.From)] = true
	}
	if !got["x.en.sub"] || !got["x.en.idx"] || !got["x.de.sub"] || got["x.fr.sub"] {
		t.Fatalf("moves %+v", plan[0].Subtitles)
	}
	if _, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	newBase := subDir + "Dune - Part Two (2024) 2160p WEB"
	for _, n := range []string{".en.sub", ".en.idx", ".de.sub"} {
		if !onDisk(lf, newBase+n) {
			t.Fatalf("%s did not follow", n)
		}
	}
	if !onDisk(lf, subDir+"x.fr.sub") {
		t.Fatal("a picture .sub without its .idx moved")
	}
}

// A pair whose .idx cannot take the new name stays together: the .sub is
// skipped too.
func TestRenameVobSubPairClashSkipsBoth(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	newBase := subDir + "Dune - Part Two (2024) 2160p WEB"
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.idx")), "idx")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(newBase+".en.idx")), "someone else's")
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || len(plan[0].Subtitles) != 2 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	for _, m := range plan[0].Subtitles {
		if m.Skip == "" {
			t.Fatalf("half a pair would move: %+v", plan[0].Subtitles)
		}
	}
	if _, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	if !onDisk(lf, subDir+"x.en.sub") || !onDisk(lf, subDir+"x.en.idx") {
		t.Fatal("the pair split")
	}
}

// A delete holds the pair under matching names, even when the holding
// folder already has one of them.
func TestDeleteHoldsVobSubPairTogether(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "imported")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.idx")), "idx")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.fr.sub")), vobsub)
	day := importer.HoldingDir + "/" + now.UTC().Format("2006-01-02") + "/"
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(day+subDir+"x.en.idx")), "held earlier")
	plan, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil || len(plan.Held) != 1 || len(plan.Held[0].Subtitles) != 2 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	var sub, idx string
	for _, m := range plan.Held[0].Subtitles {
		if m.Result != "held" {
			t.Fatalf("%+v", m)
		}
		if strings.HasSuffix(m.To, ".sub") {
			sub = strings.TrimSuffix(m.To, ".sub")
		} else {
			idx = strings.TrimSuffix(m.To, ".idx")
		}
	}
	if sub == "" || sub != idx {
		t.Fatalf("pair held under different names: %+v", plan.Held[0].Subtitles)
	}
	if !onDisk(lf, subDir+"x.fr.sub") {
		t.Fatal("a picture .sub without its .idx was held")
	}
}

// Recovery keeps a VobSub pair whole: a .sub whose .idx step failed stays.
func TestRecoverRenamesKeepsVobSubPairWhole(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.idx")), "idx")
	at := func(n string) string { return filepath.Join(lf.Path, filepath.FromSlash(subDir+n)) }
	err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, st := range [][]string{{at("x.mkv"), at("x.mkv"), "done", "video"}, {at("x.en.idx"), at("y.en.idx"), "failed", "subtitle"}, {at("x.en.sub"), at("y.en.sub"), "pending", "subtitle"}} {
			if _, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state, part) VALUES ('2026-10-13T00:00:00Z', ?, ?, ?, 'r', ?, ?)`,
				f.ID, st[0], st[1], st[2], st[3]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.RecoverRenames(ctx, domain.System)
	if !onDisk(lf, subDir+"x.en.sub") || onDisk(lf, subDir+"y.en.sub") {
		t.Fatal("the .sub moved without its .idx")
	}
}

func TestRecoverRenamesRollsBackVobSubIndexWhenSubDestinationIsOccupied(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "adopted")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"y.en.idx")), "idx")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.sub")), vobsub)
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"y.en.sub")), "unrelated")
	at := func(n string) string { return filepath.Join(lf.Path, filepath.FromSlash(subDir+n)) }
	err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, st := range [][]string{
			{at("x.mkv"), at("x.mkv"), subDir + "x.mkv", "done", "video"},
			{at("x.en.idx"), at("y.en.idx"), subDir + "y.en.idx", "done", "subtitle"},
			{at("x.en.sub"), at("y.en.sub"), subDir + "y.en.sub", "pending", "subtitle"},
		} {
			if _, err := tx.Exec(`INSERT INTO rename_steps (created_at, file_id, from_path, to_path, to_rel, state, part) VALUES ('2026-10-13T00:00:00Z', ?, ?, ?, ?, ?, ?)`,
				f.ID, st[0], st[1], st[2], st[3], st[4]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecoverRenames(ctx, domain.System); err == nil {
		t.Fatal("occupied .sub destination did not fail recovery")
	}
	if !onDisk(lf, subDir+"x.en.idx") || !onDisk(lf, subDir+"x.en.sub") || onDisk(lf, subDir+"y.en.idx") {
		t.Fatal("recovery left the VobSub pair split across basenames")
	}
	if got, _ := os.ReadFile(at("y.en.sub")); string(got) != "unrelated" {
		t.Fatalf("occupied destination changed: %q", got)
	}
	rows, err := s.Read().Query("SELECT state FROM rename_steps WHERE part = 'subtitle' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var states []string
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			t.Fatal(err)
		}
		states = append(states, state)
	}
	if strings.Join(states, ",") != "failed,failed" {
		t.Fatalf("subtitle step states %v, want both failed", states)
	}
}

// Review 4.16 (8): a delete marks a subtitle's row gone only once that
// subtitle has moved; one that could not move keeps its row present.
func TestDeleteSubtitleRowsFollowEachMove(t *testing.T) {
	svc, s, lf, id := renameWorld(t)
	ctx := context.Background()
	f := putFile(t, s, lf, id, subDir+"x.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-HOLYARRS", "imported")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.en.srt")), "sub")
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(subDir+"x.fr.srt")), "sub")
	putSubRow(t, s, f.ID, subDir+"x.en.srt", "en")
	putSubRow(t, s, f.ID, subDir+"x.fr.srt", "fr")
	moveSubtitle = func(from, to string) error {
		if strings.HasSuffix(from, "x.fr.srt") {
			return errors.New("disk full")
		}
		return moveFile(from, to)
	}
	defer func() { moveSubtitle = moveFile }()
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	rows := subRows(t, s, f.ID)
	if rows[subDir+"x.en.srt"] != "gone" || rows[subDir+"x.fr.srt"] != "present" || !onDisk(lf, subDir+"x.fr.srt") {
		t.Fatalf("rows %v", rows)
	}
}
