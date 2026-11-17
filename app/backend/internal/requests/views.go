package requests

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// StateLabels are the words people see for ask states.
var StateLabels = map[string]string{
	domain.AskAsked: "Waiting for approval", domain.AskApproved: "Approved", domain.AskTurnedDown: "Turned down", domain.AskAvailable: "Ready to watch",
}

// Who names a person.
type Who struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// DisplayName is the person's chosen name, when set (sent for the decider only).
	DisplayName string `json:"display_name,omitempty"`
}

// Asker is someone who asked, and when.
type Asker struct {
	Who
	AskedAt time.Time `json:"asked_at"`
	// FromWatchlist: they asked (or joined) from their Plex watchlist.
	FromWatchlist bool `json:"from_watchlist,omitempty"`
}

// View is an ask as pages show it.
type View struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	TMDBID      string `json:"tmdb_id"`
	Ref         string `json:"ref"`     // the id it is added by (tmdb:1399, tvmaze:82)
	Seasons     []int  `json:"seasons"` // asked-for seasons of a show; empty: the whole show
	SeasonWords string `json:"season_words,omitempty"`
	// Episodes are single episodes asked for, by season (seasons not asked whole).
	Episodes []domain.AskEpisodes `json:"episodes"`
	// FutureOnly: the show's future seasons only.
	FutureOnly bool `json:"future_only"`
	// FourK: a movie or show ask for a 4K copy.
	FourK bool `json:"four_k"`
	// Quality is the line of what people can ask for that was picked
	// (standard, 4k; empty for music), and QualityName the owner's name for
	// it now (empty: the built-in name).
	Quality     string `json:"quality"`
	QualityName string `json:"quality_name"`
	// Albums are the albums of an artist asked for; empty: the whole artist.
	Albums []domain.AskAlbum `json:"albums"`
	// ArtistID is an artist ask's MusicBrainz id, for its page and picture.
	ArtistID   string     `json:"artist_id,omitempty"`
	AlbumWords string     `json:"album_words,omitempty"`
	Name       string     `json:"name"`
	Year       *int       `json:"year,omitempty"`
	Poster     string     `json:"poster,omitempty"`
	State      string     `json:"state"`
	StateLabel string     `json:"state_label"`
	TitleID    *int64     `json:"title_id,omitempty"`
	AskedBy    Who        `json:"asked_by"`
	AskedAt    time.Time  `json:"asked_at"`
	People     []Asker    `json:"people"` // everyone who asked, first one first
	DecidedBy  *Who       `json:"decided_by,omitempty"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	// AlsoDeclined are the other people who declined the same thing on
	// earlier asks for it, newest first (the decider of this ask left out).
	// Filled by AddAlsoDeclined, for people who approve only (a list, maybe
	// empty); left out for everyone else.
	AlsoDeclined *[]Who     `json:"also_declined,omitempty"`
	Note         string     `json:"note,omitempty"`
	AutoApproved bool       `json:"auto_approved"`
	AvailableAt  *time.Time `json:"available_at,omitempty"`
	// Step says where an approved ask is now (filled by AddSteps).
	Step *Step `json:"step,omitempty"`
	// Problem says why an approved ask will not reach the Library, when the
	// server knows (the people who asked see it on Requests).
	Problem string `json:"problem,omitempty"`
	// Mine says the person reading asked for it too (set by the API).
	Mine bool `json:"mine"`
}

// Views reads asks matching where (SQL on asks a), newest first.
func (s *Service) Views(ctx context.Context, where string, args ...any) ([]View, error) {
	db := s.Store.Read()
	as, err := database.Select[domain.Ask](ctx, db, "asks a", where+" ORDER BY CASE a.state WHEN 'asked' THEN 0 ELSE 1 END, a.asked_at DESC, a.id DESC", args...)
	if err != nil {
		return nil, err
	}
	out := []View{}
	names := map[int64]string{}
	userName := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		var n string
		_ = db.QueryRowContext(ctx, viewsSQL, id).Scan(&n)
		names[id] = n
		return n
	}
	shown := map[int64]string{}
	displayName := func(id int64) string {
		if n, ok := shown[id]; ok {
			return n
		}
		var n string
		_ = db.QueryRowContext(ctx, viewsSQL2, id).Scan(&n)
		n = strings.TrimSpace(n)
		shown[id] = n
		return n
	}
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, a := range as {
		v := View{ID: a.ID, Kind: a.Kind, TMDBID: a.TMDBID, Ref: a.Ref, Seasons: a.Seasons, FutureOnly: a.FutureOnly, FourK: a.FourK, Quality: a.Quality, Name: a.Name, Year: a.Year, Poster: a.Poster, State: a.State, StateLabel: StateLabels[a.State],
			TitleID: a.TitleID, AskedBy: Who{ID: a.AskedBy, Name: userName(a.AskedBy)}, AskedAt: a.AskedAt, DecidedAt: a.DecidedAt, Note: a.Note,
			AutoApproved: a.AutoApproved, AvailableAt: a.AvailableAt, People: []Asker{}}
		if v.Seasons == nil {
			v.Seasons = []int{}
		}
		v.Episodes = a.Episodes
		if v.Episodes == nil {
			v.Episodes = []domain.AskEpisodes{}
		}
		if a.Kind == "show" {
			v.SeasonWords = askSeasonWords(a)
		}
		if a.Quality != "" {
			if l, ok := lineByID(set.Qualities.of(a.Kind), a.Quality); ok {
				v.QualityName = l.Name
			} else {
				v.QualityName = a.QualityName
			}
		}
		v.Albums = a.Albums
		if v.Albums == nil {
			v.Albums = []domain.AskAlbum{}
		}
		if a.Kind == "artist" {
			v.AlbumWords = albumWords(a.Albums)
			v.ArtistID = a.SourceIDs["musicbrainz"]
			if v.ArtistID == "" {
				v.ArtistID = strings.TrimPrefix(a.Ref, "mbid:")
			}
			if v.Poster == "" && v.ArtistID != "" {
				v.Poster = "/api/v1/music-trending/art/artist/" + v.ArtistID
			}
			if a.State == domain.AskAvailable {
				v.StateLabel = "Ready to listen"
			}
		}
		// An approved ask whose title was deleted from the Library since will not arrive.
		if a.State == domain.AskApproved && a.TitleID != nil {
			var st string
			_ = db.QueryRowContext(ctx, viewsSQL3, *a.TitleID).Scan(&st)
			if st == "removed" {
				v.Problem = "It was deleted from the Library after it was approved."
			}
		}
		if a.DecidedBy != nil {
			v.DecidedBy = &Who{ID: *a.DecidedBy, Name: userName(*a.DecidedBy), DisplayName: displayName(*a.DecidedBy)}
		}
		ps, err := s.people(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			v.People = append(v.People, Asker{Who{ID: p.ID, Name: p.Name}, p.AskedAt, p.FromWatchlist})
		}
		out = append(out, v)
	}
	return out, nil
}

// AddAlsoDeclined fills each view's AlsoDeclined (always a list, maybe
// empty). It names who turned down other people's asks, so it is for
// people who approve only; members see who decided their own ask.
func (s *Service) AddAlsoDeclined(ctx context.Context, vs []View) {
	db := s.Store.Read()
	shown := map[int64]Who{}
	who := func(id int64) Who {
		if w, ok := shown[id]; ok {
			return w
		}
		w := Who{ID: id}
		_ = db.QueryRowContext(ctx, addAlsoDeclinedSQL, id).Scan(&w.Name)
		_ = db.QueryRowContext(ctx, addAlsoDeclinedSQL2, id).Scan(&w.DisplayName)
		w.DisplayName = strings.TrimSpace(w.DisplayName)
		shown[id] = w
		return w
	}
	for i := range vs {
		list := []Who{}
		if a, err := get(ctx, db, vs[i].ID); err == nil {
			for _, id := range alsoDeclined(ctx, db, a) {
				list = append(list, who(id))
			}
		}
		vs[i].AlsoDeclined = &list
	}
}

// alsoDeclined lists who declined earlier asks for the same thing as a,
// newest first, each person once, without a's own decider. Movies match by
// TMDB id and 4K, shows by any id they share (a show found through TVmaze
// has no TMDB id), artists by MusicBrainz id. A failed read lists nobody.
func alsoDeclined(ctx context.Context, db *sql.DB, a domain.Ask) []int64 {
	var match []string
	var args []any
	switch a.Kind {
	case "artist":
		if mb := a.SourceIDs["musicbrainz"]; mb != "" {
			match, args = []string{"json_extract(source_ids, '$.musicbrainz') = ?"}, []any{mb}
		}
	case "show":
		var or []string
		for _, k := range []string{"tmdb", "tvdb", "tvmaze", "imdb"} {
			if v := a.SourceIDs[k]; v != "" {
				or = append(or, "json_extract(source_ids, '$."+k+"') = ?")
				args = append(args, v)
			}
		}
		if len(or) > 0 {
			match = []string{"(" + strings.Join(or, " OR ") + ")"}
		}
	default:
		if a.TMDBID != "" {
			match, args = []string{"tmdb_id = ?", "four_k = ?"}, []any{a.TMDBID, a.FourK}
		}
	}
	if len(match) == 0 {
		return nil
	}
	q := alsoDeclinedSQL + strings.Join(match, " AND ") + " AND state = 'turned_down' AND decided_by IS NOT NULL AND id != ? ORDER BY decided_at DESC, id DESC"
	args = append([]any{a.Kind}, append(args, a.ID)...)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[int64]bool{}
	if a.DecidedBy != nil && a.State == domain.AskTurnedDown {
		seen[*a.DecidedBy] = true
	}
	var out []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// Message is an in-app message for one person.
type Message struct {
	ID        int64      `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Kind      string     `json:"kind"` // approved, turned_down, ready, fixed (a report)
	Text      string     `json:"text"`
	AskID     *int64     `json:"ask_id,omitempty"`
	TitleID   *int64     `json:"title_id,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	ReportID  *int64     `json:"report_id,omitempty"`
}

// Messages reads a person's newest messages and how many are unread.
func Messages(ctx context.Context, db *sql.DB, userID int64, limit int) ([]Message, int, error) {
	out := []Message{}
	var unread int
	if err := db.QueryRowContext(ctx, messagesSQL, userID).Scan(&unread); err != nil {
		return out, 0, err
	}
	rows, err := db.QueryContext(ctx, messagesSQL2, userID, limit)
	if err != nil {
		return out, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Message
		var at string
		var read sql.NullString
		if err := rows.Scan(&m.ID, &at, &m.Kind, &m.Text, &m.AskID, &m.TitleID, &read, &m.ReportID); err != nil {
			return out, 0, err
		}
		m.CreatedAt, _ = time.Parse(database.TimeLayout, at)
		if read.Valid {
			t, _ := time.Parse(database.TimeLayout, read.String)
			m.ReadAt = &t
		}
		out = append(out, m)
	}
	return out, unread, rows.Err()
}

// MarkRead marks all of a person's messages as read.
func MarkRead(ctx context.Context, st *database.Store, userID int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(markReadSQL, database.FormatTime(time.Now()), userID)
		return err
	})
}
