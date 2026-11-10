package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// Register adds the subtitle Job kinds to q and lets the Service use the
// queue to schedule later work.
func (s *Service) Register(q *jobs.Queue) {
	q.Handle(KindSearch, s.handleSearch)
	q.Handle(KindMissing, s.handleMissing)
	q.Handle(KindBetter, s.handleBetter)
	q.Handle(KindLook, s.handleLook)
	if s.Enqueue == nil {
		s.Enqueue = func(ctx context.Context, kind string, subj *domain.Subject, params map[string]any, key string) error {
			_, _, err := q.Enqueue(ctx, kind, subj, params, key, domain.System)
			return err
		}
	}
}

// Queue asks for a search of one File. Each File has at most one live Job.
func (s *Service) Queue(ctx context.Context, fileID int64, params map[string]any) error {
	if s.Enqueue == nil {
		return errors.New("no job queue")
	}
	return s.Enqueue(ctx, KindSearch, &domain.Subject{Type: "file", ID: fileID}, params, fmt.Sprintf("%s:%d", KindSearch, fileID))
}

func jobActor(r *jobs.Run) domain.Actor { return domain.Actor{Kind: "job", ID: r.Job.ID} }

func (s *Service) handleSearch(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "file" {
		return errors.New("subtitle_search needs a file subject")
	}
	if old, ok := r.Job.Params["replaced_file_id"].(float64); ok && old > 0 {
		s.holdWithVideo(ctx, int64(old), r.Job.Subject.ID, jobActor(r))
	}
	better, _ := r.Job.Params["better"].(bool)
	days := 0
	if d, ok := r.Job.Params["better_days"].(float64); ok {
		days = int(d)
	}
	res, err := s.Search(ctx, r.Job.Subject.ID, SearchOptions{Better: better, BetterDays: days}, jobActor(r))
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotMain) || errors.Is(err, ErrMusic) {
		r.SetResult(map[string]any{"skipped": err.Error()})
		return nil
	}
	if err != nil {
		return err
	}
	b, _ := json.Marshal(res.Outcomes)
	var outs []any
	_ = json.Unmarshal(b, &outs)
	r.SetResult(map[string]any{"outcomes": outs, "sites_asked": res.Asked})
	return nil
}

// handleMissing queues searches, at most Batch of them, for Files whose wish
// is unmet and due, and for Files not looked at since their wish changed.
func (s *Service) handleMissing(ctx context.Context, r *jobs.Run) error {
	set := ReadSettings(ctx, s.Store.Read())
	ids, err := s.dueFiles(ctx, set.Batch)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Queue(ctx, id, nil); err != nil {
			return err
		}
	}
	r.SetResult(map[string]any{"queued": len(ids)})
	return nil
}

func (s *Service) dueFiles(ctx context.Context, batch int) ([]int64, error) {
	db := s.Store.Read()
	var out []int64
	seen := map[int64]bool{}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT file_id FROM subtitles WHERE place = 'wish' AND state IN ('wanted', 'none_good_enough')
		AND (next_try_at IS NULL OR next_try_at <= ?) AND NOT EXISTS (SELECT 1 FROM files f JOIN versions v ON v.id = f.version_id
			JOIN titles t ON t.id = v.title_id WHERE f.id = subtitles.file_id AND t.state = 'removed')
		ORDER BY next_try_at IS NOT NULL, next_try_at LIMIT ?`, database.FormatTime(now()), batch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	rows.Close()
	if len(out) >= batch {
		return out, nil
	}
	return s.staleFiles(ctx, batch, out, seen)
}

// staleFiles appends to out, up to limit, Files that were never looked at or
// were looked at under a different wish than they have now.
func (s *Service) staleFiles(ctx context.Context, limit int, out []int64, seen map[int64]bool) ([]int64, error) {
	db := s.Store.Read()
	// Files never looked at, or looked at under another wish.
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT f.id, i.title_id, f.version_id, i.id, l.wish FROM files f
		JOIN slots s ON s.file_id = f.id JOIN items i ON i.id = s.item_id
		JOIN titles t ON t.id = i.title_id LEFT JOIN subtitle_looks l ON l.file_id = f.id
		WHERE f.role = 'main' AND f.state = 'present' AND i.kind IN ('movie', 'episode') AND t.state != 'removed'
		ORDER BY l.looked_at IS NOT NULL, l.looked_at, f.id`)
	if err != nil {
		return nil, err
	}
	type cand struct {
		file, title, version, item int64
		wish                       sql.NullString
	}
	var cs []cand
	for rows.Next() {
		var c cand
		if rows.Scan(&c.file, &c.title, &c.version, &c.item, &c.wish) == nil {
			cs = append(cs, c)
		}
	}
	rows.Close()
	for _, c := range cs {
		if len(out) >= limit {
			break
		}
		if seen[c.file] {
			continue
		}
		w, err := WishFor(ctx, db, c.title, c.version, c.item)
		if err != nil || len(w.Wants) == 0 {
			continue
		}
		b, _ := json.Marshal(w.Wants)
		if c.wish.Valid && c.wish.String == string(b) {
			continue
		}
		seen[c.file] = true
		out = append(out, c.file)
	}
	return out, nil
}

// QueueLook asks for a look at every File whose wish changed or that was never
// looked at. A look reads what is on disk and lists the missing subtitles
// without asking any site. Only one look Job runs at a time.
func (s *Service) QueueLook(ctx context.Context) error {
	if s.Enqueue == nil {
		return errors.New("no job queue")
	}
	return s.Enqueue(ctx, KindLook, nil, nil, KindLook)
}

// lookBatch caps the Files one look Job reads, so a large library is split
// across several Jobs.
var lookBatch = 500

func (s *Service) handleLook(ctx context.Context, r *jobs.Run) error {
	ids, err := s.staleFiles(ctx, lookBatch, nil, map[int64]bool{})
	if err != nil {
		return err
	}
	failed := 0
	for i, id := range ids {
		if _, err := s.Look(ctx, id, jobActor(r)); err != nil {
			failed++
		}
		if (i+1)%25 == 0 {
			_ = r.Progress(ctx, float64(i+1)/float64(len(ids)), fmt.Sprintf("Looked at %d of %d files", i+1, len(ids)))
		}
	}
	r.SetResult(map[string]any{"looked": len(ids) - failed, "failed": failed})
	if len(ids) == lookBatch && failed < len(ids) {
		// Files remain, so the next Job carries on from here.
		_ = s.Enqueue(ctx, KindLook, nil, nil, fmt.Sprintf("%s:%d", KindLook, r.Job.ID))
	}
	return nil
}

// handleBetter queues a search for a better subtitle on Files whose subtitle
// came from a search in the last BetterDays days and scored under 99.
func (s *Service) handleBetter(ctx context.Context, r *jobs.Run) error {
	set := ReadSettings(ctx, s.Store.Read())
	if !set.Better {
		r.SetResult(map[string]any{"skipped": "better subtitles are off"})
		return nil
	}
	since := database.FormatTime(now().AddDate(0, 0, -set.BetterDays))
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT DISTINCT file_id FROM subtitles WHERE place = 'beside' AND state = 'present' AND origin = 'fetched'
		AND score < 99 AND saved_at >= ? ORDER BY saved_at LIMIT ?`, since, set.Batch)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if err := s.Queue(ctx, id, map[string]any{"better": true, "better_days": set.BetterDays}); err != nil {
			return err
		}
	}
	r.SetResult(map[string]any{"queued": len(ids)})
	return nil
}

// OnEvent starts a search when an Import places a File or a better copy
// replaces one; in the second case the old subtitles are held aside with the
// old video. Wire it to database.OnEvent.
func (s *Service) OnEvent(e domain.Event) {
	ctx := context.Background()
	switch e.Kind {
	case "file_replaced":
		newID, _ := num(e.Data["new_file_id"])
		oldID, _ := num(e.Data["old_file_id"])
		if oldID > 0 {
			// This runs here rather than in the Job because the import's own Event may
			// already have queued a search for the new File.
			s.holdWithVideo(ctx, oldID, newID, domain.System)
		}
		if newID > 0 {
			if err := s.Queue(ctx, newID, nil); err != nil {
				log.Printf("subtitles: queue search for file %d: %v", newID, err)
			}
		}
	case "adopted":
		// Files adopted from disk get their missing subtitles listed now; the
		// schedule searches for them later.
		if err := s.QueueLook(ctx); err != nil {
			log.Printf("subtitles: queue look after adopting: %v", err)
		}
	case "import_done", "import_partly_done":
		for _, id := range s.placedFiles(ctx, e) {
			if err := s.Queue(ctx, id, nil); err != nil {
				log.Printf("subtitles: queue search for file %d: %v", id, err)
			}
		}
	}
}

func num(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	}
	return 0, false
}

// placedFiles returns the main Files that the Event's Slots now hold.
func (s *Service) placedFiles(ctx context.Context, e domain.Event) []int64 {
	var slots []int64
	for _, k := range []string{"placed_slot_ids", "slot_ids"} {
		if raw, ok := e.Data[k].([]any); ok {
			for _, x := range raw {
				if id, ok := num(x); ok {
					slots = append(slots, id)
				}
			}
		}
		if ids, ok := e.Data[k].([]int64); ok {
			slots = append(slots, ids...)
		}
	}
	if len(slots) == 0 && e.SlotID != nil {
		slots = []int64{*e.SlotID}
	}
	var out []int64
	seen := map[int64]bool{}
	for _, id := range slots {
		var f sql.NullInt64
		var kind string
		if s.Store.Read().QueryRowContext(ctx, `SELECT s.file_id, i.kind FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?`, id).Scan(&f, &kind) == nil &&
			f.Valid && !seen[f.Int64] && (kind == "movie" || kind == "episode") {
			seen[f.Int64] = true
			out = append(out, f.Int64)
		}
	}
	return out
}

// libRel returns p relative to the Library folder lib with forward slashes,
// so Events and History never reveal where the library lives on disk.
func libRel(lib, p string) string {
	r, err := filepath.Rel(lib, p)
	if err != nil || !filepath.IsLocal(r) {
		return filepath.Base(p)
	}
	return filepath.ToSlash(r)
}

// holdWithVideo moves the beside subtitles the app saved for a replaced video
// into the holding folder along with it, and marks their rows replaced.
func (s *Service) holdWithVideo(ctx context.Context, oldFileID, newFileID int64, by domain.Actor) {
	rows, err := database.Select[Subtitle](ctx, s.Store.Read(), "subtitles",
		"file_id = ? AND place = 'beside' AND state = 'present' AND origin IN ('fetched', 'by_hand')", oldFileID)
	if err != nil || len(rows) == 0 {
		return
	}
	var lib string
	if s.Store.Read().QueryRowContext(ctx, `SELECT lf.path FROM files f JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE f.id = ?`, oldFileID).Scan(&lib) != nil {
		return
	}
	for _, r := range rows {
		if r.RelativePath == nil {
			continue
		}
		to, err := importer.HoldPath(lib, *r.RelativePath, now())
		if err == nil {
			if err = os.MkdirAll(filepath.Dir(to), 0o777); err == nil {
				err = os.Rename(filepath.Join(lib, filepath.FromSlash(*r.RelativePath)), to)
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("subtitles: hold %s: %v", *r.RelativePath, err)
			continue
		}
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "UPDATE subtitles SET state = 'replaced', updated_at = ? WHERE id = ?", database.FormatTime(now()), r.ID)
			return err
		})
		var titleID int64
		_ = s.Store.Read().QueryRowContext(ctx, "SELECT v.title_id FROM files f JOIN versions v ON v.id = f.version_id WHERE f.id = ?", newFileID).Scan(&titleID)
		s.event(ctx, &domain.Event{Actor: by, Kind: EventHeldWithVideo, TitleID: &titleID, Summary: "Held " + filepath.Base(*r.RelativePath) + " aside with the replaced video",
			Data: map[string]any{"file_id": oldFileID, "new_file_id": newFileID, "moved_to": libRel(lib, to)}, GroupKey: "subtitles:" + itoa(newFileID)})
	}
}

// MissingRow is one entry in the missing subtitles list.
type MissingRow struct {
	FileID    int64      `json:"file_id"`
	ItemID    int64      `json:"item_id"`
	TitleID   int64      `json:"title_id"`
	Title     string     `json:"title"`
	Kind      string     `json:"kind"`
	Season    *int       `json:"season,omitempty"`
	Episode   *int       `json:"episode,omitempty"`
	Language  string     `json:"language"`
	Forced    bool       `json:"forced"`
	HI        bool       `json:"hearing_impaired"`
	State     string     `json:"state"`
	Tries     int        `json:"tries"`
	NextTryAt *time.Time `json:"next_try_at,omitempty"`
}

// MissingFilter narrows the missing subtitles list. An empty field matches all.
type MissingFilter struct {
	Language string // two-letter language code
	Kind     string // movie or episode
	TitleID  int64  // limits the list to one movie or show
}

const missingFrom = ` FROM subtitles s JOIN files f ON f.id = s.file_id JOIN slots sl ON sl.id = json_extract(f.slot_ids, '$[0]')
		JOIN items i ON i.id = sl.item_id JOIN titles t ON t.id = i.title_id WHERE s.place = 'wish' AND s.state IN ('wanted', 'none_good_enough') AND f.state = 'present'
		AND t.state != 'removed'`

// MissingTitle is a movie or show missing at least one subtitle.
type MissingTitle struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// MissingTitles lists by name the movies and shows missing a wanted subtitle,
// with a count for each. It feeds the title filter of the missing list.
func MissingTitles(ctx context.Context, db *sql.DB) ([]MissingTitle, error) {
	rows, err := db.QueryContext(ctx, "SELECT t.id, t.name, t.kind, count(*)"+missingFrom+" GROUP BY t.id ORDER BY t.sort_name, t.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MissingTitle{}
	for rows.Next() {
		var m MissingTitle
		if err := rows.Scan(&m.ID, &m.Name, &m.Kind, &m.Count); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MissingList lists wanted subtitles that are not present yet. It can narrow
// to one language, kind or title, and sorts by title name, then episode.
func MissingList(ctx context.Context, db *sql.DB, mf MissingFilter, limit, offset int) ([]MissingRow, int, error) {
	where := ""
	var args []any
	if mf.Language != "" {
		where += " AND s.language = ?"
		args = append(args, mf.Language)
	}
	if mf.Kind != "" {
		where += " AND i.kind = ?"
		args = append(args, mf.Kind)
	}
	if mf.TitleID > 0 {
		where += " AND t.id = ?"
		args = append(args, mf.TitleID)
	}
	from := missingFrom + where
	var total int
	if err := db.QueryRowContext(ctx, "SELECT count(*)"+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT s.file_id, i.id, t.id, t.name, i.kind, json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode'),
		s.language, s.forced, s.hearing_impaired, s.state, s.tries, s.next_try_at`+from+
		` ORDER BY t.sort_name, json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode'), s.language LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []MissingRow{}
	for rows.Next() {
		var m MissingRow
		var next sql.NullString
		if err := rows.Scan(&m.FileID, &m.ItemID, &m.TitleID, &m.Title, &m.Kind, &m.Season, &m.Episode, &m.Language, &m.Forced, &m.HI, &m.State, &m.Tries, &next); err != nil {
			return nil, 0, err
		}
		if next.Valid {
			if t, err := time.Parse(database.TimeLayout, next.String); err == nil {
				m.NextTryAt = &t
			}
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}
