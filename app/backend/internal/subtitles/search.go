package subtitles

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// SearchOptions changes how one search behaves.
type SearchOptions struct {
	// Better also searches again for wanted languages whose saved subtitle
	// scored under 99 and was saved within BetterDays days (0 means any age).
	Better     bool
	BetterDays int
	// Show only lists the candidates for a person to pick; nothing is saved.
	Show bool
	// NoTranslate never queues a translation; the search a failed translation
	// falls back to sets it.
	NoTranslate bool
}

// Outcome is what a search did for one wanted language.
type Outcome struct {
	Want    Want     `json:"want"`
	Result  string   `json:"result"` // saved, upgraded, none_good_enough, failed, kept or translating
	Path    string   `json:"path,omitempty"`
	Score   int      `json:"score,omitempty"`
	Site    string   `json:"site,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Made    string   `json:"made,omitempty"`    // a machine-made file stood in, see Subtitle.Made
	Choices []Scored `json:"choices,omitempty"` // filled only for Show searches
}

// SearchResult is the outcome of searching one File.
type SearchResult struct {
	FileID   int64     `json:"file_id"`
	Status   Status    `json:"status"`
	Outcomes []Outcome `json:"outcomes"`
	Asked    []string  `json:"sites_asked"`
}

// target is one language a search looks for, with the score it must beat.
type target struct {
	want  Want
	beat  int       // a candidate must score above this; -1 means anything over the pass mark
	old   *Subtitle // the saved subtitle a better one would replace
	isNew bool
}

// Search looks on every usable site for the File's missing subtitles, plus
// improvable ones when Better is set, and saves the best one over the pass mark.
func (s *Service) Search(ctx context.Context, fileID int64, opt SearchOptions, by domain.Actor) (SearchResult, error) {
	res := SearchResult{FileID: fileID, Outcomes: []Outcome{}, Asked: []string{}}
	fc, err := s.load(ctx, fileID)
	if err != nil {
		return res, err
	}
	st, err := s.look(ctx, fc, by)
	if err != nil {
		return res, err
	}
	res.Status = st
	var targets []target
	for _, m := range st.Missing {
		targets = append(targets, target{want: m, beat: -1, isNew: true})
	}
	if opt.Better || opt.Show {
		// A person choosing by hand may swap their own earlier pick too; timed
		// better searches leave hand picks alone.
		origins := "origin = 'fetched'"
		if opt.Show {
			origins = "origin IN ('fetched', 'by_hand')"
		}
		saved, err := database.Select[Subtitle](ctx, s.Store.Read(), "subtitles",
			"file_id = ? AND place = 'beside' AND state = 'present' AND "+origins+" AND score IS NOT NULL", fileID)
		if err != nil {
			return res, err
		}
		for i := range saved {
			sv := &saved[i]
			// A machine-made subtitle is looked past at any age, and any file made
			// by a person that passes replaces it.
			stop := sv.Made != nil
			if !opt.Show && !stop && (*sv.Score >= 99 || (opt.BetterDays > 0 && sv.SavedAt != nil && sv.SavedAt.Before(now().AddDate(0, 0, -opt.BetterDays)))) {
				continue
			}
			for _, w := range st.Wish.Wants {
				if meets(w, Have{Language: sv.Language, Forced: sv.Forced, HearingImpaired: sv.HearingImpaired}) {
					t := target{want: w, beat: *sv.Score, old: sv}
					if stop {
						t.beat = -1
					}
					targets = append(targets, t)
					break
				}
			}
		}
	}
	if len(targets) == 0 {
		return res, nil
	}
	set := ReadSettings(ctx, s.Store.Read())
	pass := set.PassMark(fc.item.Kind)
	v := fc.videoFacts()
	q := sites.Query{Kind: fc.item.Kind, Title: v.Title, Year: v.Year, Season: v.Season, Episode: v.Episode, ReleaseName: v.ReleaseName,
		IMDb: fc.title.SourceIDs["imdb"], TMDB: fc.title.SourceIDs["tmdb"], TVDB: fc.title.SourceIDs["tvdb"]}
	if h, err := s.videoHash(ctx, fc, set); err == nil && h != "" {
		q.Hash = h // hash-matching sites use it and the rest ignore it
		if st, err := os.Stat(fc.video()); err == nil {
			q.Size = st.Size()
		}
	}
	sc := &searchCtx{fc: fc, status: st, set: set, opt: opt, pass: pass, video: v, bySite: map[string]SiteRow{}, by: by}
	seenLang := map[string]bool{}
	ask := func(l string) {
		if !seenLang[l] {
			seenLang[l] = true
			q.Languages = append(q.Languages, l)
		}
	}
	for _, t := range targets {
		ask(t.want.Language)
		if t.isNew && t.want.TranslateFrom != "" && !opt.Show && !opt.NoTranslate {
			if !sc.translator.Ready() {
				sc.translator = s.Translator(ctx)
			}
			if sc.translator.Ready() {
				ask(t.want.TranslateFrom) // a source to translate from, in the same call
			}
		}
	}
	turned := s.turnedDown(ctx, fileID)
	sc.turned = turned

	// Each usable site is asked once for all languages to keep traffic low.
	rows, err := usable(ctx, s.Store.Read(), now())
	if err != nil {
		return res, err
	}
	var all []Scored
	bySite := sc.bySite
	for order, r := range rows {
		k, _ := sites.KindOf(r.Kind)
		if (fc.item.Kind == "movie" && !k.Movies) || (fc.item.Kind == "episode" && !k.Shows) {
			continue
		}
		site, err := s.client(ctx, r)
		if err != nil {
			continue
		}
		res.Asked = append(res.Asked, r.Name)
		cands, err := site.Search(ctx, q)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if err != nil {
			s.pause(ctx, r, err)
			continue
		}
		s.recovered(ctx, r)
		bySite[r.Name] = r
		for _, c := range cands[:min(len(cands), MaxCandidatesPerSite)] {
			if c.MachineMade != "" && !r.MachineMade {
				continue // the site's machine-made switch is off
			}
			c.Site = r.Name
			all = append(all, Scored{Candidate: c, Priority: r.Priority, Order: order})
		}
	}
	sc.all = all

	for _, t := range targets {
		var choices []Scored
		for _, c := range all {
			if !LangMatches(t.want.Language, c.Language) || turned[c.Site+"\x00"+c.Key] {
				continue
			}
			c.Score = ScoreCandidate(v, t.want, c.Candidate)
			choices = append(choices, c)
		}
		Rank(choices)
		out := Outcome{Want: t.want}
		if opt.Show {
			out.Result, out.Choices = "shown", choices
			res.Outcomes = append(res.Outcomes, out)
			continue
		}
		out.Result = "none_good_enough"
		for _, c := range choices {
			if c.MachineMade != "" {
				continue // used only when nothing made by a person passes
			}
			if c.Score.Refused != "" || c.Score.Total < pass || c.Score.Total <= t.beat {
				break // the list is ranked, so nothing after this is better
			}
			p, err := s.fetchAndSave(ctx, fc, t, c, bySite[c.Site], set, by)
			if err != nil {
				out.Reason = err.Error()
				continue
			}
			out.Result, out.Path, out.Score, out.Site, out.Reason = "saved", p, c.Score.Total, c.Site, ""
			if t.old != nil {
				out.Result = "upgraded"
			}
			break
		}
		if t.isNew && out.Result == "none_good_enough" {
			s.stopgap(ctx, sc, t, choices, &out)
		}
		if t.old != nil && out.Result == "none_good_enough" {
			out.Result = "kept"
			if t.old.Made != nil {
				s.touchStopgap(ctx, t.old.ID)
			}
		}
		s.afterTarget(ctx, fc, t, &out, set, by)
		res.Outcomes = append(res.Outcomes, out)
	}
	return res, nil
}

// afterTarget records how a target ended: its wish row state and an Event.
func (s *Service) afterTarget(ctx context.Context, fc *fileCtx, t target, out *Outcome, set Settings, by domain.Actor) {
	titleID, fileID := fc.title.ID, fc.file.ID
	slotID := fc.file.SlotIDs[0]
	switch out.Result {
	case "saved", "upgraded":
		kind, verb := EventSaved, "Saved"
		if out.Result == "upgraded" {
			kind, verb = EventUpgraded, "Saved a better"
		}
		s.event(ctx, &domain.Event{Actor: by, Kind: kind, TitleID: &titleID, SlotID: &slotID,
			Summary: fmt.Sprintf("%s %s subtitle from %s (score %d)", verb, fmtWant(t.want), out.Site, out.Score),
			Data:    map[string]any{"file_id": fileID, "language": t.want.Language, "path": out.Path, "score": out.Score, "site": out.Site}, GroupKey: "subtitles:" + itoa(fileID)})
	case "none_good_enough", "translating":
		next := now().Add(time.Duration(set.SearchHours) * time.Hour)
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			var first sql.NullString
			_ = tx.QueryRowContext(ctx, "SELECT first_tried_at FROM subtitles WHERE file_id = ? AND place = 'wish' AND language = ? AND forced = ?",
				fileID, t.want.Language, t.want.Forced).Scan(&first)
			if set.Adaptive && first.Valid {
				if ft, err := time.Parse(database.TimeLayout, first.String); err == nil && now().Sub(ft) > 21*24*time.Hour {
					next = now().Add(7 * 24 * time.Hour)
				}
			}
			_, err := tx.ExecContext(ctx, `UPDATE subtitles SET state = 'none_good_enough', tries = tries + 1, first_tried_at = COALESCE(first_tried_at, ?),
				last_tried_at = ?, next_try_at = ?, updated_at = ? WHERE file_id = ? AND place = 'wish' AND language = ? AND forced = ?`,
				database.FormatTime(now()), database.FormatTime(now()), database.FormatTime(next), database.FormatTime(now()), fileID, t.want.Language, t.want.Forced)
			return err
		})
		if out.Result == "translating" {
			return // the translation Job writes its own Event
		}
		s.event(ctx, &domain.Event{Actor: by, Kind: EventNoneGood, TitleID: &titleID, SlotID: &slotID,
			Summary: fmt.Sprintf("No %s subtitle reached the Pass mark", fmtWant(t.want)),
			Data:    map[string]any{"file_id": fileID, "language": t.want.Language, "reason": out.Reason, "next_try_at": next}, GroupKey: "subtitles:" + itoa(fileID)})
	}
}

// fetchAndSave downloads a candidate, checks it and saves it beside the video,
// holding any replaced subtitle aside. It returns the saved path.
func (s *Service) fetchAndSave(ctx context.Context, fc *fileCtx, t target, c Scored, r SiteRow, set Settings, by domain.Actor) (string, error) {
	site, err := s.client(ctx, r)
	if err != nil {
		return "", err
	}
	if !s.countDownload(ctx, r) {
		return "", errors.New("the site's downloads for today are used up")
	}
	data, err := site.Download(ctx, c.Candidate)
	if err != nil {
		s.pause(ctx, r, err)
		return "", err
	}
	entries, err := subtext.Unpack(data, c.Key+"."+c.Format)
	if err != nil {
		s.failed(ctx, fc, t.want, c.Site, err, by)
		return "", err
	}
	e := pickEntry(entries)
	text, err := subtext.Clean(e.Data, e.Format)
	if err != nil {
		s.failed(ctx, fc, t.want, c.Site, err, by)
		return "", err
	}
	siteID := r.ID
	// The file keeps the language the site gave, so a pt-BR file found for a pt
	// wish is named and recorded as pt-BR.
	lang := t.want.Language
	if LangMatches(lang, c.Language) {
		lang = c.Language
	}
	row := Subtitle{FileID: fc.file.ID, Language: lang, Forced: t.want.Forced, HearingImpaired: c.HearingImpaired, Place: "beside",
		SiteName: &c.Site, SiteKey: &c.Key, Score: &c.Score.Total, ScoreParts: c.Score.Parts, Origin: "fetched", State: "present", Sync: map[string]any{}}
	if c.MachineMade != "" {
		// A machine-made file is marked as such and keeps a low score, so a
		// later search replaces it with a file made by a person.
		made, score := c.MachineMade, stopgapScore(c.MachineMade, c.Score.Total)
		row.Made, row.Score = &made, &score
	}
	if siteID != 0 {
		row.SiteID = &siteID
	}
	p, err := s.save(ctx, fc, &row, text, e.Format, set, t.old, by)
	if err != nil {
		s.failed(ctx, fc, t.want, c.Site, err, by)
	}
	return p, err
}

// pickEntry prefers an SRT entry and falls back to the first one.
func pickEntry(es []subtext.Entry) subtext.Entry {
	for _, e := range es {
		if e.Format == subtext.SRT {
			return e
		}
	}
	return es[0]
}

func (s *Service) failed(ctx context.Context, fc *fileCtx, w Want, site string, err error, by domain.Actor) {
	titleID, slotID, fileID := fc.title.ID, fc.file.SlotIDs[0], fc.file.ID
	// Disk errors carry full paths. Events and the live stream must only show
	// paths inside the Library folder.
	msg := err.Error()
	if lib := filepath.Clean(fc.lib); lib != "." && lib != "" {
		msg = strings.ReplaceAll(msg, lib+string(filepath.Separator), "")
		msg = strings.ReplaceAll(msg, lib, "the Library folder")
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: EventFailed, TitleID: &titleID, SlotID: &slotID,
		Summary: fmt.Sprintf("A %s subtitle from %s could not be saved: %s", fmtWant(w), site, msg),
		Data:    map[string]any{"file_id": fileID, "language": w.Language, "site": site, "error": msg}, GroupKey: "subtitles:" + itoa(fileID)})
}

// SubtitleName builds a subtitle file name for a video in the form
// base.lang[.hi|.forced].format.
func SubtitleName(videoRel, lang string, hi, forced bool, hiSuffix, format string) string {
	base := strings.TrimSuffix(path.Base(videoRel), path.Ext(videoRel))
	name := base + "." + LanguageTag(lang)
	switch {
	case forced:
		name += ".forced"
	case hi:
		name += "." + hiSuffix
	}
	return path.Join(path.Dir(videoRel), name+"."+format)
}

// ErrNotOurs refuses to overwrite a subtitle file the app did not save, so a
// person's own file is never lost.
var ErrNotOurs = errors.New("a subtitle file HolyArrs did not save is at that name; it is left alone")

// save writes text beside the video under its language-tagged name, using the
// importer's safe-write rules. The path must stay inside the Library folder
// with links followed. The text goes to a temporary file that is synced and
// then renamed into place. A file already at the name is held aside in the
// holding folder if the app saved it; otherwise the save is refused.
func (s *Service) save(ctx context.Context, fc *fileCtx, row *Subtitle, text, format string, set Settings, old *Subtitle, by domain.Actor) (string, error) {
	rel := SubtitleName(fc.file.RelativePath, row.Language, row.HearingImpaired, row.Forced, set.HISuffix, format)
	return s.writeAt(ctx, fc, row, text, rel, old, by)
}

type heldSubtitle struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// saveNote adds the exact held-aside files to the saved subtitle row, so an
// interrupted save can be undone. Notes without them still decode.
type saveNote struct {
	Subtitle
	Held     []heldSubtitle `json:"_held,omitempty"`
	NewProof string         `json:"_new_proof,omitempty"`
}

// writeAt is save at a path relative to the Library folder. A file already at
// rel must belong to old or be one the app saved, and it is held aside.
func (s *Service) writeAt(ctx context.Context, fc *fileCtx, row *Subtitle, text, rel string, old *Subtitle, by domain.Actor) (string, error) {
	full := filepath.Join(fc.lib, filepath.FromSlash(rel))
	dir := filepath.Dir(full)
	if !strings.HasPrefix(full, filepath.Clean(fc.lib)+string(filepath.Separator)) || !importer.InsideReal(fc.lib, dir) {
		return "", fmt.Errorf("%s is outside the Library folder", rel)
	}
	// Collect what this save replaces: the previous saved subtitle when this is a
	// better one, and whatever sits at the new name.
	var replace []Subtitle
	if old != nil {
		replace = append(replace, *old)
	}
	if fi, err := os.Lstat(full); err == nil {
		if !fi.Mode().IsRegular() {
			return "", fmt.Errorf("%s is not a regular file", rel)
		}
		if old == nil || old.RelativePath == nil || *old.RelativePath != rel {
			mine, err := database.Select[Subtitle](ctx, s.Store.Read(), "subtitles",
				"file_id = ? AND place = 'beside' AND state = 'present' AND relative_path = ? AND origin IN ('fetched', 'by_hand')", fc.file.ID, rel)
			if err != nil {
				return "", err
			}
			if len(mine) == 0 {
				return "", ErrNotOurs
			}
			replace = append(replace, mine[0])
		}
	}
	tmp, err := importer.CreateTemp(dir, ".holyarrs-sub-", "")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // already gone after the rename; removes the leftover on failure
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// Record the save before touching the disk, so a save cut off after its file
	// is in place gets finished by the next look.
	t := now()
	row.RelativePath, row.SavedAt = &rel, &t
	// Choose and record the exact holding names before moving anything. If the
	// process stops after a hold but before the new file lands, recovery can put
	// the old subtitle back instead of losing its only usable name.
	var held []heldSubtitle
	seen := map[string]bool{}
	for _, r := range replace {
		if r.RelativePath == nil {
			continue
		}
		from := filepath.Join(fc.lib, filepath.FromSlash(*r.RelativePath))
		if seen[from] {
			continue
		}
		seen[from] = true
		fi, statErr := os.Lstat(from)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return "", statErr
		}
		if !fi.Mode().IsRegular() {
			return "", fmt.Errorf("%s is not a regular file", *r.RelativePath)
		}
		to, err := importer.HoldPath(fc.lib, *r.RelativePath, now())
		if err != nil {
			return "", err
		}
		held = append(held, heldSubtitle{From: from, To: to})
	}
	rowJSON, err := json.Marshal(saveNote{Subtitle: *row, Held: held, NewProof: bytesProof([]byte(text))})
	if err != nil {
		return "", err
	}
	var replaceIDs []int64
	for _, r := range replace {
		replaceIDs = append(replaceIDs, r.ID)
	}
	idsJSON, _ := json.Marshal(replaceIDs)
	var saveID int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT INTO subtitle_saves (created_at, file_id, relative_path, row, replace_ids) VALUES (?, ?, ?, ?, ?)",
			database.FormatTime(t), row.FileID, rel, string(rowJSON), string(idsJSON))
		if err == nil {
			saveID, err = res.LastInsertId()
		}
		return err
	})
	if err != nil {
		return "", err
	}
	forget := func() {
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "DELETE FROM subtitle_saves WHERE id = ?", saveID)
			return err
		})
	}
	putBack := func() {
		for i := len(held) - 1; i >= 0; i-- {
			_ = os.Rename(held[i].To, held[i].From)
		}
	}
	for i, h := range held {
		err := os.MkdirAll(filepath.Dir(h.To), 0o777)
		if err != nil {
			putBack()
			forget()
			return "", err
		}
		if err := os.Rename(h.From, h.To); err != nil {
			putBack()
			forget()
			return "", err
		}
		if saveHoldCrash != nil {
			if err := saveHoldCrash(i + 1); err != nil {
				return "", err
			}
		}
	}
	if saveHoldCrash != nil && len(held) == 0 {
		if err := saveHoldCrash(0); err != nil {
			return "", err
		}
	}
	if err := renameInto(tmpName, full); err != nil {
		// Move the held files back to their names so nothing has changed.
		putBack()
		forget()
		return "", err
	}
	if saveCrash != nil {
		if err := saveCrash(); err != nil {
			return "", err
		}
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		return s.recordSave(ctx, tx, row, replaceIDs, saveID, by)
	})
	return rel, err
}

// renameInto puts a written subtitle at its final name. Tests swap it to fail.
var renameInto = os.Rename

// saveCrash lets tests stop a save after its file is in place and before its
// row is written, as if the process died at that point.
var saveCrash func() error

// saveHoldCrash lets tests stop a save after old subtitles were held and
// before the new file was installed, as if the process died at that point.
var saveHoldCrash func(held int) error

// recordSave writes the row of a finished save. It marks the rows it replaces
// as replaced, removes the wish it meets and deletes its subtitle_saves note.
func (s *Service) recordSave(ctx context.Context, tx *sql.Tx, row *Subtitle, replaceIDs []int64, saveID int64, by domain.Actor) error {
	ts := database.FormatTime(now())
	for _, id := range replaceIDs {
		if _, err := tx.ExecContext(ctx, "UPDATE subtitles SET state = 'replaced', updated_at = ? WHERE id = ?", ts, id); err != nil {
			return err
		}
	}
	// Any other present row at the same name is replaced too, since one file has one row.
	if _, err := tx.ExecContext(ctx, "UPDATE subtitles SET state = 'replaced', updated_at = ? WHERE file_id = ? AND place = 'beside' AND state = 'present' AND relative_path = ?",
		ts, row.FileID, *row.RelativePath); err != nil {
		return err
	}
	if _, err := database.Insert(tx, "subtitles", row, by); err != nil {
		return err
	}
	// The saved file meets the wish for its own language and for its base
	// language, as a pt-BR file meets a pt wish.
	if _, err := tx.ExecContext(ctx, "DELETE FROM subtitles WHERE file_id = ? AND place = 'wish' AND language IN (?, ?) AND forced = ?",
		row.FileID, row.Language, baseOf(row.Language), row.Forced); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM subtitle_saves WHERE id = ?", saveID)
	return err
}

// finishSaves completes a File's saves that were cut off. A save whose file is
// in place gets its row, marked as the app's own rather than found. A save
// whose file is absent is dropped.
func (s *Service) finishSaves(ctx context.Context, tx *sql.Tx, fc *fileCtx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id, relative_path, row, replace_ids FROM subtitle_saves WHERE file_id = ? ORDER BY id", fc.file.ID)
	if err != nil {
		return err
	}
	type save struct {
		id            int64
		rel, row, ids string
	}
	var ss []save
	for rows.Next() {
		var v save
		if err := rows.Scan(&v.id, &v.rel, &v.row, &v.ids); err != nil {
			rows.Close()
			return err
		}
		ss = append(ss, v)
	}
	rows.Close()
	for _, v := range ss {
		var note saveNote
		var ids []int64
		if json.Unmarshal([]byte(v.row), &note) != nil || json.Unmarshal([]byte(v.ids), &ids) != nil ||
			note.RelativePath == nil || *note.RelativePath != v.rel {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subtitle_saves WHERE id = ?", v.id); err != nil {
				return err
			}
			continue
		}
		target := filepath.Join(fc.lib, filepath.FromSlash(v.rel))
		installed, err := savedSubtitleInstalled(fc.lib, target, note)
		if err != nil {
			return err
		}
		if !installed {
			if err := restoreHeldSubtitles(fc.lib, note.Held); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM subtitle_saves WHERE id = ?", v.id); err != nil {
				return err
			}
			continue
		}
		note.ID = 0
		if err := s.recordSave(ctx, tx, &note.Subtitle, ids, v.id, domain.System); err != nil {
			return err
		}
	}
	return nil
}

func bytesProof(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%d:%s", len(b), hex.EncodeToString(sum[:]))
}

func fileMatchesProof(name, proof string) bool {
	sizeText, wantHash, ok := strings.Cut(proof, ":")
	if !ok {
		return false
	}
	wantSize, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || wantSize < 0 || wantSize > subtext.MaxEntryText {
		return false
	}
	st, err := os.Lstat(name)
	if err != nil || !st.Mode().IsRegular() || st.Size() != wantSize {
		return false
	}
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, wantSize+1))
	return err == nil && n == wantSize && hex.EncodeToString(h.Sum(nil)) == wantHash
}

func savedSubtitleInstalled(lib, target string, note saveNote) (bool, error) {
	if err := validateHeldSubtitles(lib, note.Held); err != nil {
		return false, err
	}
	if !fileMatchesProof(target, note.NewProof) {
		return false, nil
	}
	for _, h := range note.Held {
		_, fromErr := os.Lstat(h.From)
		_, toErr := os.Lstat(h.To)
		if filepath.Clean(h.From) == filepath.Clean(target) {
			if fromErr == nil && toErr == nil {
				continue
			}
			return false, nil
		}
		if errors.Is(fromErr, os.ErrNotExist) && toErr == nil {
			continue
		}
		return false, nil
	}
	return true, nil
}

func validateHeldSubtitles(lib string, held []heldSubtitle) error {
	lib = filepath.Clean(lib)
	holding := filepath.Join(lib, importer.HoldingDir)
	for _, h := range held {
		if !strings.HasPrefix(filepath.Clean(h.From), lib+string(filepath.Separator)) ||
			!strings.HasPrefix(filepath.Clean(h.To), holding+string(filepath.Separator)) ||
			!importer.InsideReal(lib, filepath.Dir(h.From)) || !importer.InsideReal(lib, filepath.Dir(h.To)) {
			return errors.New("a held subtitle path leaves the Library folder")
		}
	}
	return nil
}

func restoreHeldSubtitles(lib string, held []heldSubtitle) error {
	if err := validateHeldSubtitles(lib, held); err != nil {
		return err
	}
	for i := len(held) - 1; i >= 0; i-- {
		h := held[i]
		_, fromErr := os.Lstat(h.From)
		_, toErr := os.Lstat(h.To)
		switch {
		case fromErr == nil && errors.Is(toErr, os.ErrNotExist):
			continue // already put back
		case errors.Is(fromErr, os.ErrNotExist) && errors.Is(toErr, os.ErrNotExist):
			continue // the file vanished before it could be held
		case errors.Is(fromErr, os.ErrNotExist) && toErr == nil:
			if err := os.Rename(h.To, h.From); err != nil {
				return err
			}
		case fromErr == nil && toErr == nil:
			return fmt.Errorf("both the subtitle and its held copy exist at %s", filepath.Base(h.From))
		default:
			return errors.Join(fromErr, toErr)
		}
	}
	return nil
}

func (s *Service) turnedDown(ctx context.Context, fileID int64) map[string]bool {
	out := map[string]bool{}
	rows, err := s.Store.Read().QueryContext(ctx, "SELECT site_name, site_key FROM subtitle_turned_down WHERE file_id = ?", fileID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if rows.Scan(&a, &b) == nil {
			out[a+"\x00"+b] = true
		}
	}
	return out
}
