package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Event kinds for subtitle moments.
const (
	EventSaved         = "subtitle_saved"
	EventUpgraded      = "subtitle_upgraded"
	EventNoneGood      = "subtitle_none_good_enough"
	EventFailed        = "subtitle_failed"
	EventSitePaused    = "subtitle_site_paused"
	EventSynced        = "subtitle_synced"
	EventTurnedDown    = "subtitle_turned_down"
	EventHeldWithVideo = "subtitle_held"
)

// Publisher sends live events (live.Hub fits).
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Service runs subtitle work.
type Service struct {
	Store *store.Store
	Live  Publisher // may be nil
	// Vault keeps the account details of sites that need an account.
	Vault *secrets.Vault
	// HTTP replaces every site's HTTP client (tests only).
	HTTP *http.Client
	// Enqueue queues a Job (the jobs queue); nil runs nothing later.
	Enqueue func(ctx context.Context, kind string, subj *domain.Subject, params map[string]any, key string) error

	mu      sync.Mutex
	clients map[int64]cached
}

func (s *Service) event(ctx context.Context, e *domain.Event) {
	if s.Store.AddEvent(ctx, e) == nil && s.Live != nil {
		s.Live.Publish("event", e)
	}
}

// Subtitle is one row of the subtitles table.
type Subtitle struct {
	domain.Meta
	FileID          int64          `db:"file_id" json:"file_id"`
	Language        string         `db:"language" json:"language"`
	Forced          bool           `db:"forced" json:"forced"`
	HearingImpaired bool           `db:"hearing_impaired" json:"hearing_impaired"`
	Place           string         `db:"place" json:"place"` // built_in, beside, wish
	TrackIndex      *int           `db:"track_index" json:"track_index,omitempty"`
	Codec           *string        `db:"codec" json:"codec,omitempty"`
	RelativePath    *string        `db:"relative_path" json:"relative_path,omitempty"`
	SiteID          *int64         `db:"site_id" json:"site_id,omitempty"`
	SiteName        *string        `db:"site_name" json:"site,omitempty"`
	SiteKey         *string        `db:"site_key" json:"site_key,omitempty"`
	Score           *int           `db:"score" json:"score,omitempty"`
	ScoreParts      []Part         `db:"score_parts,json" json:"score_parts"`
	Origin          string         `db:"origin" json:"origin"` // found, fetched, by_hand
	State           string         `db:"state" json:"state"`   // present, replaced, gone, wanted, none_good_enough
	SavedAt         *time.Time     `db:"saved_at" json:"saved_at,omitempty"`
	Tries           int            `db:"tries" json:"tries"`
	FirstTriedAt    *time.Time     `db:"first_tried_at" json:"first_tried_at,omitempty"`
	LastTriedAt     *time.Time     `db:"last_tried_at" json:"last_tried_at,omitempty"`
	NextTryAt       *time.Time     `db:"next_try_at" json:"next_try_at,omitempty"`
	Sync            map[string]any `db:"sync,json" json:"sync"`
}

// fileCtx is everything about one main File a search needs.
type fileCtx struct {
	file    domain.File
	title   domain.Title
	item    domain.Item
	version domain.Version
	lib     string // the Library folder's path
}

// video is the File's full path.
func (fc *fileCtx) video() string {
	return filepath.Join(fc.lib, filepath.FromSlash(fc.file.RelativePath))
}

func (fc *fileCtx) videoFacts() Video {
	v := Video{Kind: fc.item.Kind, Title: fc.title.Name, Aliases: fc.title.Aliases}
	if fc.title.Year != nil {
		v.Year = *fc.title.Year
	}
	if fc.item.Numbers.Season != nil {
		v.Season = *fc.item.Numbers.Season
	}
	if fc.item.Numbers.Episode != nil {
		v.Episode = *fc.item.Numbers.Episode
	}
	v.ReleaseName = strings.TrimSuffix(path.Base(fc.file.RelativePath), path.Ext(fc.file.RelativePath))
	if fc.file.ReleaseName != nil && *fc.file.ReleaseName != "" {
		v.ReleaseName = *fc.file.ReleaseName
	}
	return v
}

// ErrNotMain refuses Files that are not a present main video of a movie
// or an episode.
var ErrNotMain = errors.New("not a video of a movie or an episode in the Library")

func (s *Service) load(ctx context.Context, fileID int64) (*fileCtx, error) {
	db := s.Store.Read()
	fc := &fileCtx{}
	if err := store.Get(ctx, db, "files", fileID, &fc.file); err != nil {
		return nil, ErrNotFound
	}
	if fc.file.Role != "main" || fc.file.State != "present" || len(fc.file.SlotIDs) == 0 {
		return nil, ErrNotMain
	}
	var itemID int64
	if err := db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", fc.file.SlotIDs[0]).Scan(&itemID); err != nil {
		return nil, ErrNotMain
	}
	if err := store.Get(ctx, db, "items", itemID, &fc.item); err != nil {
		return nil, err
	}
	if fc.item.Kind != "movie" && fc.item.Kind != "episode" {
		return nil, ErrMusic
	}
	if err := store.Get(ctx, db, "versions", fc.file.VersionID, &fc.version); err != nil {
		return nil, err
	}
	if err := store.Get(ctx, db, "titles", fc.item.TitleID, &fc.title); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, "SELECT path FROM library_folders WHERE id = ?", fc.version.LibraryFolderID).Scan(&fc.lib); err != nil {
		return nil, err
	}
	return fc, nil
}

// Have is one subtitle that is there for a File.
type Have struct {
	Language        string `json:"language"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearing_impaired"`
	Place           string `json:"place"`
	Picture         bool   `json:"picture,omitempty"`
	Path            string `json:"path,omitempty"` // beside: relative to the Library folder
	Codec           string `json:"codec,omitempty"`
	Track           int    `json:"track,omitempty"`
}

// meets reports whether h is what w wishes for (design section 2).
func meets(w Want, h Have) bool {
	if h.Language != w.Language || h.Forced != w.Forced {
		return false
	}
	return w.HearingImpaired != HIOnly || h.HearingImpaired
}

// Missing lists the wants no Have meets.
func Missing(wants []Want, haves []Have) []Want {
	out := []Want{}
	for _, w := range wants {
		met := false
		for _, h := range haves {
			met = met || meets(w, h)
		}
		if !met {
			out = append(out, w)
		}
	}
	return out
}

// builtIn reads the subtitle tracks inside the video.
func builtIn(video string, ignorePictures bool) ([]Have, error) {
	tracks, err := container.Probe(video)
	if err != nil {
		if errors.Is(err, container.ErrUnknownFormat) {
			return nil, nil
		}
		return nil, err
	}
	var out []Have
	for _, t := range tracks {
		lang := NormLanguage(t.Language)
		if lang == "" || (ignorePictures && t.Picture) {
			continue
		}
		name := strings.ToLower(t.Name)
		hi := t.HearingImpaired || hasWord(name, "sdh") || hasWord(name, "hi") || hasWord(name, "cc")
		forced := t.Forced || strings.Contains(name, "forced")
		out = append(out, Have{Language: lang, Forced: forced, HearingImpaired: hi, Place: "built_in", Picture: t.Picture, Codec: t.Codec, Track: t.Index})
	}
	return out, nil
}

func hasWord(s, w string) bool {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') }) {
		if f == w {
			return true
		}
	}
	return false
}

// besideName reads a subtitle file name next to video base: base.LANG
// [.hi|.cc|.sdh][.forced].EXT. ok is false for other files.
func besideName(base, name string) (Have, bool) {
	ext := strings.ToLower(path.Ext(name))
	known := false
	for _, f := range subtext.Formats {
		known = known || ext == "."+f
	}
	if !known || len(name) <= len(base)+1 || !strings.EqualFold(name[:len(base)+1], base+".") {
		return Have{}, false
	}
	tags := strings.Split(strings.ToLower(strings.TrimSuffix(name[len(base)+1:], path.Ext(name))), ".")
	if len(tags) == 0 || len(tags) > 4 {
		return Have{}, false
	}
	h := Have{Language: NormLanguage(tags[0]), Place: "beside"}
	if h.Language == "" {
		return Have{}, false
	}
	for _, t := range tags[1:] {
		switch t {
		case "hi", "cc", "sdh":
			h.HearingImpaired = true
		case "forced", "foreign":
			h.Forced = true
		default:
			return Have{}, false
		}
	}
	return h, true
}

// IsBeside says whether name is a subtitle file of the video named base
// (its name without the extension): base.lang[.hi|.cc|.sdh|.forced].ext.
func IsBeside(base, name string) bool {
	_, ok := besideName(base, name)
	return ok
}

// besideFiles lists subtitle files next to the video.
func besideFiles(fc *fileCtx) ([]Have, error) {
	video := fc.video()
	dir := filepath.Dir(video)
	base := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	relDir := path.Dir(fc.file.RelativePath)
	var out []Have
	for _, e := range ents {
		if !e.Type().IsRegular() {
			continue
		}
		h, ok := besideName(base, e.Name())
		if !ok {
			continue
		}
		h.Path = path.Join(relDir, e.Name())
		out = append(out, h)
	}
	return out, nil
}

// Status is what a File has and still wants.
type Status struct {
	FileID  int64    `json:"file_id"`
	Wish    Resolved `json:"wish"`
	Haves   []Have   `json:"haves"`
	Missing []Want   `json:"missing"`
}

// Look reads what a File has (built in and beside), records it, and
// brings the File's wish rows up to date. It reaches no site.
func (s *Service) Look(ctx context.Context, fileID int64, by domain.Actor) (Status, error) {
	fc, err := s.load(ctx, fileID)
	if err != nil {
		return Status{}, err
	}
	return s.look(ctx, fc, by)
}

func (s *Service) look(ctx context.Context, fc *fileCtx, by domain.Actor) (Status, error) {
	st := Status{FileID: fc.file.ID, Haves: []Have{}}
	// Recover an interrupted save before reading beside files. Recovery may
	// restore a held old subtitle; scanning first would use a stale empty view
	// and immediately mark that restored row gone.
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		return s.finishSaves(ctx, tx, fc)
	}); err != nil {
		return st, err
	}
	wish, err := WishFor(ctx, s.Store.Read(), fc.title.ID, fc.version.ID, fc.item.ID)
	if err != nil {
		return st, err
	}
	st.Wish = wish
	set := ReadSettings(ctx, s.Store.Read())
	// An unchanged video (same size, modified time and built-in settings
	// as the last look) is not read again: its recorded tracks stand.
	stamp, stampOK := stampOf(fc.video(), set)
	was, _, wasOK := storedStamp(ctx, s.Store.Read(), fc.file.ID)
	same := stampOK && wasOK && was == stamp
	var inside []Have
	if same {
		if inside, err = storedBuiltIn(ctx, s.Store.Read(), fc.file.ID); err != nil {
			return st, err
		}
	} else if set.UseBuiltIn {
		inside, err = builtIn(fc.video(), set.IgnorePictureTracks)
		if err != nil {
			inside = nil // an unreadable video has no built-in subtitles we can trust
			stampOK = false
		}
	}
	beside, err := besideFiles(fc)
	if err != nil {
		return st, err
	}
	st.Haves = append(append(st.Haves, inside...), beside...)
	st.Missing = Missing(wish.Wants, st.Haves)
	wishJSON, _ := json.Marshal(wish.Wants)
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if !same { // else the recorded built-in rows stand
			if _, err := tx.ExecContext(ctx, "DELETE FROM subtitles WHERE file_id = ? AND place = 'built_in'", fc.file.ID); err != nil {
				return err
			}
			for _, h := range inside {
				idx, codec := h.Track, h.Codec
				if _, err := store.Insert(tx, "subtitles", &Subtitle{FileID: fc.file.ID, Language: h.Language, Forced: h.Forced, HearingImpaired: h.HearingImpaired,
					Place: "built_in", TrackIndex: &idx, Codec: &codec, Origin: "found", State: "present", Sync: map[string]any{}}, by); err != nil {
					return err
				}
			}
		}
		// Beside files: new ones are found; recorded ones no longer there are gone.
		rows, err := store.Select[Subtitle](ctx, tx, "subtitles", "file_id = ? AND place = 'beside' AND state = 'present'", fc.file.ID)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, r := range rows {
			there := false
			for _, h := range beside {
				there = there || (r.RelativePath != nil && *r.RelativePath == h.Path)
			}
			if !there {
				if _, err := tx.ExecContext(ctx, "UPDATE subtitles SET state = 'gone', updated_at = ? WHERE id = ?", store.FormatTime(now()), r.ID); err != nil {
					return err
				}
				continue
			}
			known[*r.RelativePath] = true
		}
		for _, h := range beside {
			if known[h.Path] {
				continue
			}
			p := h.Path
			if _, err := store.Insert(tx, "subtitles", &Subtitle{FileID: fc.file.ID, Language: h.Language, Forced: h.Forced, HearingImpaired: h.HearingImpaired,
				Place: "beside", RelativePath: &p, Origin: "found", State: "present", Sync: map[string]any{}}, by); err != nil {
				return err
			}
		}
		// Wish rows: one per missing want; met ones go.
		wishRows, err := store.Select[Subtitle](ctx, tx, "subtitles", "file_id = ? AND place = 'wish'", fc.file.ID)
		if err != nil {
			return err
		}
		for _, r := range wishRows {
			still := false
			for _, m := range st.Missing {
				still = still || (m.Language == r.Language && m.Forced == r.Forced)
			}
			if !still {
				if _, err := tx.ExecContext(ctx, "DELETE FROM subtitles WHERE id = ?", r.ID); err != nil {
					return err
				}
			}
		}
		for _, m := range st.Missing {
			if _, err := tx.ExecContext(ctx, `INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, forced, hearing_impaired, place, origin, state)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'wish', 'fetched', 'wanted')
				ON CONFLICT (file_id, language, forced) WHERE place = 'wish' DO UPDATE SET hearing_impaired = excluded.hearing_impaired`,
				store.FormatTime(now()), actorJSON(by), store.FormatTime(now()), actorJSON(by), fc.file.ID, m.Language, m.Forced, m.HearingImpaired == HIOnly); err != nil {
				return err
			}
		}
		// The video's stamp; a changed video drops its stored hash.
		var size, mod, read any
		if stampOK {
			size, mod, read = stamp.Size, stamp.Modified, stamp.Read
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO subtitle_looks (file_id, looked_at, wish, video_size, video_modified, video_read) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (file_id) DO UPDATE SET looked_at = excluded.looked_at, wish = excluded.wish,
				video_hash = CASE WHEN video_size IS excluded.video_size AND video_modified IS excluded.video_modified THEN video_hash END,
				video_size = excluded.video_size, video_modified = excluded.video_modified, video_read = excluded.video_read`,
			fc.file.ID, store.FormatTime(now()), string(wishJSON), size, mod, read)
		return err
	})
	sort.SliceStable(st.Haves, func(i, j int) bool { return st.Haves[i].Language < st.Haves[j].Language })
	return st, err
}

func actorJSON(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

// FilesOf lists the present main Files of a title (optionally one Item).
func FilesOf(ctx context.Context, db *sql.DB, titleID, itemID int64) ([]int64, error) {
	q := `SELECT DISTINCT f.id FROM files f JOIN slots s ON s.file_id = f.id JOIN items i ON i.id = s.item_id
		WHERE i.title_id = ? AND f.role = 'main' AND f.state = 'present' AND i.kind IN ('movie', 'episode')`
	args := []any{titleID}
	if itemID != 0 {
		q += " AND i.id = ?"
		args = append(args, itemID)
	}
	rows, err := db.QueryContext(ctx, q+" ORDER BY f.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListFor lists the subtitle rows of a File, newest first within a place.
func ListFor(ctx context.Context, db *sql.DB, fileID int64) ([]Subtitle, error) {
	rs, err := store.Select[Subtitle](ctx, db, "subtitles", "file_id = ? ORDER BY place, language, id DESC", fileID)
	if rs == nil {
		rs = []Subtitle{}
	}
	return rs, err
}

func fmtWant(w Want) string {
	s := w.Language
	if w.Forced {
		s += " (forced)"
	}
	if w.HearingImpaired == HIOnly {
		s += " (hearing-impaired)"
	}
	return s
}
