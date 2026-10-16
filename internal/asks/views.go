package asks

import (
	"context"
	"database/sql"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// StateLabels are the words people see for ask states.
var StateLabels = map[string]string{
	domain.AskAsked: "Waiting for approval", domain.AskApproved: "Approved", domain.AskTurnedDown: "Turned down", domain.AskAvailable: "Ready to watch",
}

// Who names a person.
type Who struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Asker is someone who asked, and when.
type Asker struct {
	Who
	AskedAt time.Time `json:"asked_at"`
}

// View is an ask as pages show it.
type View struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	TMDBID      string `json:"tmdb_id"`
	Ref         string `json:"ref"`     // the id it is added by (tmdb:1399, tvmaze:82)
	Seasons     []int  `json:"seasons"` // asked-for seasons of a show; empty: the whole show
	SeasonWords string `json:"season_words,omitempty"`
	// Albums are the albums of an artist asked for; empty: the whole artist.
	Albums       []domain.AskAlbum `json:"albums"`
	AlbumWords   string            `json:"album_words,omitempty"`
	Name         string            `json:"name"`
	Year         *int              `json:"year,omitempty"`
	Poster       string            `json:"poster,omitempty"`
	State        string            `json:"state"`
	StateLabel   string            `json:"state_label"`
	TitleID      *int64            `json:"title_id,omitempty"`
	AskedBy      Who               `json:"asked_by"`
	AskedAt      time.Time         `json:"asked_at"`
	People       []Asker           `json:"people"` // everyone who asked, first one first
	DecidedBy    *Who              `json:"decided_by,omitempty"`
	DecidedAt    *time.Time        `json:"decided_at,omitempty"`
	Note         string            `json:"note,omitempty"`
	AutoApproved bool              `json:"auto_approved"`
	AvailableAt  *time.Time        `json:"available_at,omitempty"`
	// Mine says the person reading asked for it too (set by the API).
	Mine bool `json:"mine"`
}

// Views reads asks matching where (SQL on asks a), newest first.
func (s *Service) Views(ctx context.Context, where string, args ...any) ([]View, error) {
	db := s.Store.Read()
	as, err := store.Select[domain.Ask](ctx, db, "asks a", where+" ORDER BY CASE a.state WHEN 'asked' THEN 0 ELSE 1 END, a.asked_at DESC, a.id DESC", args...)
	if err != nil {
		return nil, err
	}
	out := []View{}
	userName := func(id int64) string {
		var n string
		_ = db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", id).Scan(&n)
		return n
	}
	for _, a := range as {
		v := View{ID: a.ID, Kind: a.Kind, TMDBID: a.TMDBID, Ref: a.Ref, Seasons: a.Seasons, Name: a.Name, Year: a.Year, Poster: a.Poster, State: a.State, StateLabel: StateLabels[a.State],
			TitleID: a.TitleID, AskedBy: Who{a.AskedBy, userName(a.AskedBy)}, AskedAt: a.AskedAt, DecidedAt: a.DecidedAt, Note: a.Note,
			AutoApproved: a.AutoApproved, AvailableAt: a.AvailableAt, People: []Asker{}}
		if v.Seasons == nil {
			v.Seasons = []int{}
		}
		if a.Kind == "show" {
			v.SeasonWords = seasonWords(a.Seasons)
		}
		v.Albums = a.Albums
		if v.Albums == nil {
			v.Albums = []domain.AskAlbum{}
		}
		if a.Kind == "artist" {
			v.AlbumWords = albumWords(a.Albums)
			if a.State == domain.AskAvailable {
				v.StateLabel = "Ready to listen"
			}
		}
		if a.DecidedBy != nil {
			v.DecidedBy = &Who{*a.DecidedBy, userName(*a.DecidedBy)}
		}
		ps, err := s.people(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			v.People = append(v.People, Asker{Who{p.ID, p.Name}, p.AskedAt})
		}
		out = append(out, v)
	}
	return out, nil
}

// Message is an in-app message for one person.
type Message struct {
	ID        int64      `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Kind      string     `json:"kind"` // approved, turned_down, ready
	Text      string     `json:"text"`
	AskID     *int64     `json:"ask_id,omitempty"`
	TitleID   *int64     `json:"title_id,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// Messages reads a person's newest messages and how many are unread.
func Messages(ctx context.Context, db *sql.DB, userID int64, limit int) ([]Message, int, error) {
	out := []Message{}
	var unread int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM messages WHERE user_id = ? AND read_at IS NULL", userID).Scan(&unread); err != nil {
		return out, 0, err
	}
	rows, err := db.QueryContext(ctx, "SELECT id, created_at, kind, text, ask_id, title_id, read_at FROM messages WHERE user_id = ? ORDER BY id DESC LIMIT ?", userID, limit)
	if err != nil {
		return out, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Message
		var at string
		var read sql.NullString
		if err := rows.Scan(&m.ID, &at, &m.Kind, &m.Text, &m.AskID, &m.TitleID, &read); err != nil {
			return out, 0, err
		}
		m.CreatedAt, _ = time.Parse(store.TimeLayout, at)
		if read.Valid {
			t, _ := time.Parse(store.TimeLayout, read.String)
			m.ReadAt = &t
		}
		out = append(out, m)
	}
	return out, unread, rows.Err()
}

// MarkRead marks all of a person's messages as read.
func MarkRead(ctx context.Context, st *store.Store, userID int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE messages SET read_at = ? WHERE user_id = ? AND read_at IS NULL", store.FormatTime(time.Now()), userID)
		return err
	})
}
