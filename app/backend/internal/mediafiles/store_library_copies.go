package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"golang.org/x/text/unicode/norm"
)

const libraryCopyCodecSQL = `CASE
	WHEN lower(coalesce(json_extract(f.media_info, '$.inspection_status'), '')) IN ('complete', 'partial')
	 AND lower(coalesce(json_extract(f.media_info, '$.video_codec'), '')) IN ('h264', 'hevc', 'av1', 'vp9', 'mpeg2', 'xvid', 'vc1')
	THEN lower(json_extract(f.media_info, '$.video_codec'))
	ELSE 'unknown' END`

var (
	ErrLibraryCopyFilter = errors.New("invalid Library copy filter")
	ErrLibraryCopyStale  = errors.New("the selected Library copy changed or no longer exists")
)

type LibraryCopyFilter struct {
	Query  string
	Codec  string
	Sort   string
	Limit  int
	Offset int
}

type LibraryCopySelection struct {
	FileID    int64 `json:"file_id"`
	VersionID int64 `json:"version_id"`
}

type LibraryCopy struct {
	FileID                int64      `json:"file_id"`
	VersionID             int64      `json:"version_id"`
	TitleID               int64      `json:"title_id"`
	TitleName             string     `json:"title"`
	Year                  *int       `json:"year,omitempty"`
	VersionLabel          string     `json:"version"`
	Edition               string     `json:"edition"`
	VersionAudioLanguages []string   `json:"version_audio_languages"`
	RelativePath          string     `json:"relative_path"`
	Protected             bool       `json:"protected"`
	Codec                 string     `json:"codec"`
	InspectionStatus      string     `json:"inspection_status"`
	Container             string     `json:"container"`
	AudioLanguages        []string   `json:"audio_languages"`
	AudioTracks           int        `json:"audio_tracks"`
	AudioUnknown          bool       `json:"audio_unknown"`
	AddedAt               time.Time  `json:"added_at"`
	CheckedAt             *time.Time `json:"checked_at,omitempty"`
	FileUpdatedAt         time.Time  `json:"-"`
	VersionUpdatedAt      time.Time  `json:"-"`
	PickRulesID           int64      `json:"-"`
}

type LibraryCopyPage struct {
	Copies []LibraryCopy  `json:"copies"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
	Codecs map[string]int `json:"codecs"`
}

func libraryCopyWhere(query string) (string, []any, error) {
	where := `t.kind = 'movie' AND t.state != 'removed' AND f.state = 'present' AND f.role = 'main'`
	if utf8.RuneCountInString(query) > 256 {
		return "", nil, ErrTitleSearchQuery
	}
	if strings.TrimSpace(query) == "" {
		return where, nil, nil
	}
	words := strings.FieldsFunc(norm.NFC.String(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	terms := words[:0]
	for _, word := range words {
		if strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			terms = append(terms, `"`+word+`"*`)
		}
	}
	if len(terms) > 16 {
		return "", nil, ErrTitleSearchQuery
	}
	if len(terms) == 0 {
		return where + " AND 0", nil, nil
	}
	return where + " AND t.id IN (SELECT rowid FROM title_search WHERE title_search MATCH ?)", []any{strings.Join(terms, " AND ")}, nil
}

func normalizeLibraryCopyFilter(f LibraryCopyFilter) (LibraryCopyFilter, error) {
	f.Codec = strings.ToLower(strings.TrimSpace(f.Codec))
	f.Sort = strings.ToLower(strings.TrimSpace(f.Sort))
	if f.Codec == "all" {
		f.Codec = ""
	}
	if f.Sort == "" {
		f.Sort = "title"
	}
	validCodec := map[string]bool{"": true, "unknown": true, "h264": true, "hevc": true, "av1": true, "vp9": true, "mpeg2": true, "xvid": true, "vc1": true}
	if !validCodec[f.Codec] || (f.Sort != "title" && f.Sort != "codec" && f.Sort != "added") || f.Offset < 0 || f.Offset > 1_000_000 {
		return f, ErrLibraryCopyFilter
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	return f, nil
}

// LibraryCopies lists present main movie files and their exact Version.
func LibraryCopies(ctx context.Context, s *database.Store, filter LibraryCopyFilter) (LibraryCopyPage, error) {
	f, err := normalizeLibraryCopyFilter(filter)
	if err != nil {
		return LibraryCopyPage{}, err
	}
	where, args, err := libraryCopyWhere(f.Query)
	if err != nil {
		return LibraryCopyPage{}, err
	}
	page := LibraryCopyPage{Copies: []LibraryCopy{}, Limit: f.Limit, Offset: f.Offset, Codecs: map[string]int{}}
	if err := s.Read().QueryRowContext(ctx, `SELECT count(*) FROM files f JOIN versions v ON v.id=f.version_id JOIN titles t ON t.id=v.title_id WHERE `+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	countRows, err := s.Read().QueryContext(ctx, `SELECT `+libraryCopyCodecSQL+`, count(*) FROM files f JOIN versions v ON v.id=f.version_id JOIN titles t ON t.id=v.title_id WHERE `+where+` GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return page, err
	}
	for countRows.Next() {
		var codec string
		var n int
		if err := countRows.Scan(&codec, &n); err != nil {
			countRows.Close()
			return page, err
		}
		page.Codecs[codec] = n
	}
	if err := countRows.Close(); err != nil {
		return page, err
	}

	listWhere, listArgs := where, append([]any{}, args...)
	if f.Codec != "" {
		listWhere += " AND " + libraryCopyCodecSQL + " = ?"
		listArgs = append(listArgs, f.Codec)
	}
	if f.Codec != "" {
		if err := s.Read().QueryRowContext(ctx, `SELECT count(*) FROM files f JOIN versions v ON v.id=f.version_id JOIN titles t ON t.id=v.title_id WHERE `+listWhere, listArgs...).Scan(&page.Total); err != nil {
			return page, err
		}
	}
	order := "t.sort_name, t.id, v.id, f.id"
	if f.Sort == "codec" {
		order = libraryCopyCodecSQL + ", " + order
	}
	if f.Sort == "added" {
		order = "f.added_at DESC, " + order
	}
	listArgs = append(listArgs, f.Limit, f.Offset)
	rows, err := s.Read().QueryContext(ctx, libraryCopySelectSQL+` WHERE `+listWhere+` ORDER BY `+order+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		copy, err := scanLibraryCopy(rows)
		if err != nil {
			return page, err
		}
		page.Copies = append(page.Copies, copy)
	}
	return page, rows.Err()
}

const libraryCopySelectSQL = `SELECT f.id, v.id, t.id, t.name, t.year, v.label, v.edition, v.audio_languages,
	f.relative_path, f.protected, ` + libraryCopyCodecSQL + `,
	coalesce(json_extract(f.media_info, '$.inspection_status'), ''), coalesce(json_extract(f.media_info, '$.container'), ''),
	coalesce(json_extract(f.media_info, '$.audio_languages'), '[]'), coalesce(json_extract(f.media_info, '$.audio_tracks'), 0),
	coalesce(json_extract(f.media_info, '$.audio_unknown'), 1), f.added_at, f.checked_at, f.updated_at, v.updated_at, v.pick_rules_id
	FROM files f JOIN versions v ON v.id=f.version_id JOIN titles t ON t.id=v.title_id`

type rowScanner interface{ Scan(...any) error }

func scanLibraryCopy(row rowScanner) (LibraryCopy, error) {
	var c LibraryCopy
	var versionAudio, audio, added, fileUpdated, versionUpdated string
	var checked sql.NullString
	if err := row.Scan(&c.FileID, &c.VersionID, &c.TitleID, &c.TitleName, &c.Year, &c.VersionLabel, &c.Edition, &versionAudio,
		&c.RelativePath, &c.Protected, &c.Codec, &c.InspectionStatus, &c.Container, &audio, &c.AudioTracks, &c.AudioUnknown,
		&added, &checked, &fileUpdated, &versionUpdated, &c.PickRulesID); err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(versionAudio), &c.VersionAudioLanguages); err != nil {
		return c, fmt.Errorf("Version audio languages: %w", err)
	}
	if err := json.Unmarshal([]byte(audio), &c.AudioLanguages); err != nil {
		return c, fmt.Errorf("inspected audio languages: %w", err)
	}
	var err error
	if c.AddedAt, err = time.Parse(database.TimeLayout, added); err != nil {
		return c, err
	}
	if c.FileUpdatedAt, err = time.Parse(database.TimeLayout, fileUpdated); err != nil {
		return c, err
	}
	if c.VersionUpdatedAt, err = time.Parse(database.TimeLayout, versionUpdated); err != nil {
		return c, err
	}
	if checked.Valid {
		t, err := time.Parse(database.TimeLayout, checked.String)
		if err != nil {
			return c, err
		}
		c.CheckedAt = &t
	}
	return c, nil
}

// SelectedLibraryCopies re-reads exact file/Version pairs for review or execution.
func SelectedLibraryCopies(ctx context.Context, s *database.Store, selections []LibraryCopySelection) ([]LibraryCopy, error) {
	if len(selections) == 0 || len(selections) > 100 {
		return nil, ErrLibraryCopyFilter
	}
	clauses := make([]string, 0, len(selections))
	args := make([]any, 0, len(selections)*2)
	seen := map[LibraryCopySelection]bool{}
	for _, sel := range selections {
		if sel.FileID <= 0 || sel.VersionID <= 0 || seen[sel] {
			return nil, ErrLibraryCopyFilter
		}
		seen[sel] = true
		clauses = append(clauses, "(f.id=? AND v.id=?)")
		args = append(args, sel.FileID, sel.VersionID)
	}
	where, _, _ := libraryCopyWhere("")
	rows, err := s.Read().QueryContext(ctx, libraryCopySelectSQL+` WHERE `+where+` AND (`+strings.Join(clauses, " OR ")+`) ORDER BY t.sort_name,t.id,v.id,f.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LibraryCopy, 0, len(selections))
	for rows.Next() {
		c, err := scanLibraryCopy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(selections) {
		return nil, ErrLibraryCopyStale
	}
	return out, nil
}

// LibraryCopySlots returns only movie Slots belonging to the selected Version.
func LibraryCopySlots(ctx context.Context, s *database.Store, selection LibraryCopySelection) ([]int64, error) {
	rows, err := s.Read().QueryContext(ctx, `SELECT s.id FROM slots s JOIN items i ON i.id=s.item_id JOIN versions v ON v.id=s.version_id JOIN files f ON f.id=? AND f.version_id=v.id
		WHERE v.id=? AND i.title_id=v.title_id AND i.kind='movie' ORDER BY s.id`, selection.FileID, selection.VersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrLibraryCopyStale
	}
	return ids, nil
}

// TitleCopy is a movie's main file as the Library list shows it: the codec,
// container and audio languages, and the ids a copy search is asked with.
type TitleCopy struct {
	FileID         int64    `json:"file_id"`
	VersionID      int64    `json:"version_id"`
	Codec          string   `json:"codec"`
	Container      string   `json:"container"`
	AudioLanguages []string `json:"audio_languages"`
	Protected      bool     `json:"protected"`
}

// TitleCopies reads every movie's main file (the main Version first) in one query.
func TitleCopies(ctx context.Context, db database.Lister) (map[int64]TitleCopy, error) {
	where, _, _ := libraryCopyWhere("")
	rows, err := db.QueryContext(ctx, `SELECT t.id, f.id, v.id, `+libraryCopyCodecSQL+`,
		coalesce(json_extract(f.media_info, '$.container'), ''), coalesce(json_extract(f.media_info, '$.audio_languages'), '[]'), f.protected
		FROM files f JOIN versions v ON v.id=f.version_id JOIN titles t ON t.id=v.title_id WHERE `+where+`
		ORDER BY t.id, v.is_main DESC, v.id, f.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]TitleCopy{}
	for rows.Next() {
		var id int64
		var c TitleCopy
		var audio string
		if err := rows.Scan(&id, &c.FileID, &c.VersionID, &c.Codec, &c.Container, &audio, &c.Protected); err != nil {
			return nil, err
		}
		if _, ok := out[id]; ok {
			continue
		}
		if json.Unmarshal([]byte(audio), &c.AudioLanguages) != nil || c.AudioLanguages == nil {
			c.AudioLanguages = []string{}
		}
		out[id] = c
	}
	return out, rows.Err()
}
