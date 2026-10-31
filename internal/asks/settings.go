package asks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// DefaultPerWeek is each kind's weekly limit when none is set: 10 movies,
// 10 shows and 10 music asks a person.
const DefaultPerWeek = 10

// Settings are the Request settings.
type Settings struct {
	// MoviesPerWeek, ShowsPerWeek and MusicPerWeek are how many asks of
	// each kind a person may make in 7 days, unless their own limit for
	// that kind says otherwise; 0 is no limit. A show ask counts as one
	// whatever its seasons; an artist or album ask as one whatever its
	// albums.
	MoviesPerWeek int `json:"movies_per_week"`
	ShowsPerWeek  int `json:"shows_per_week"`
	MusicPerWeek  int `json:"music_per_week"`
	// LibraryFolderID and PickRulesID are used for approved asks when the
	// approver picks none (0: the add-a-movie defaults).
	LibraryFolderID int64 `json:"library_folder_id"`
	PickRulesID     int64 `json:"pick_rules_id"`
	// SearchOnApprove starts the search as soon as an ask is approved.
	SearchOnApprove bool `json:"search_on_approve"`
	// ShowLibraryFolderID and ShowPickRulesID are used for approved show
	// asks when the approver picks none (0: the add-a-show defaults).
	ShowLibraryFolderID int64 `json:"show_library_folder_id"`
	ShowPickRulesID     int64 `json:"show_pick_rules_id"`
	// MusicLibraryFolderID and MusicPickRulesID are used for approved
	// artist asks when the approver picks none (0: the add-an-artist
	// defaults).
	MusicLibraryFolderID int64 `json:"music_library_folder_id"`
	MusicPickRulesID     int64 `json:"music_pick_rules_id"`
	// ShowWhoAsked is who sees the names of other askers: "approvers" (the
	// owner, admins and managers; the default) or "everyone" (members too,
	// for askers who turned on showing their name). Kids never see others.
	ShowWhoAsked string `json:"show_who_asked"`
}

// ErrShowWhoAsked is a bad ShowWhoAsked value.
var ErrShowWhoAsked = errors.New("show_who_asked must be approvers or everyone")

// PerWeekOf is the default weekly limit for one kind.
func (s Settings) PerWeekOf(kind string) int {
	switch kind {
	case KindShows:
		return s.ShowsPerWeek
	case KindMusic:
		return s.MusicPerWeek
	}
	return s.MoviesPerWeek
}

var settingKeys = []string{"ask_movies_per_week", "ask_library_folder_id", "ask_pick_rules_id", "ask_search_on_approve",
	"ask_show_library_folder_id", "ask_show_pick_rules_id", "ask_shows_per_week", "ask_music_library_folder_id", "ask_music_pick_rules_id", "ask_music_per_week", "ask_show_who_asked"}

// ReadSettings reads the Request settings (10 a week of each kind, search
// on approve).
func ReadSettings(ctx context.Context, db store.Lister) (Settings, error) {
	s := Settings{MoviesPerWeek: DefaultPerWeek, ShowsPerWeek: DefaultPerWeek, MusicPerWeek: DefaultPerWeek, SearchOnApprove: true, ShowWhoAsked: "approvers"}
	args := make([]any, len(settingKeys))
	for i, k := range settingKeys {
		args[i] = k
	}
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = 'this_app' AND key IN (?"+strings.Repeat(", ?", len(settingKeys)-1)+")", args...)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		switch k {
		case "ask_movies_per_week", "ask_shows_per_week", "ask_music_per_week":
			if n >= 0 && n <= 1000 && v != "" {
				*map[string]*int{"ask_movies_per_week": &s.MoviesPerWeek, "ask_shows_per_week": &s.ShowsPerWeek, "ask_music_per_week": &s.MusicPerWeek}[k] = int(n)
			}
		case "ask_library_folder_id":
			s.LibraryFolderID = n
		case "ask_pick_rules_id":
			s.PickRulesID = n
		case "ask_search_on_approve":
			s.SearchOnApprove = v != "false"
		case "ask_show_library_folder_id":
			s.ShowLibraryFolderID = n
		case "ask_show_pick_rules_id":
			s.ShowPickRulesID = n
		case "ask_music_library_folder_id":
			s.MusicLibraryFolderID = n
		case "ask_music_pick_rules_id":
			s.MusicPickRulesID = n
		case "ask_show_who_asked":
			if v == "everyone" {
				s.ShowWhoAsked = v
			}
		}
	}
	return s, rows.Err()
}

// SaveSettings checks and stores the Request settings.
func SaveSettings(ctx context.Context, st *store.Store, s Settings, by domain.Actor) error {
	for _, n := range []int{s.MoviesPerWeek, s.ShowsPerWeek, s.MusicPerWeek} {
		if n < 0 || n > 1000 {
			return ErrPerWeek
		}
	}
	if s.ShowWhoAsked == "" {
		s.ShowWhoAsked = "approvers"
	}
	if s.ShowWhoAsked != "approvers" && s.ShowWhoAsked != "everyone" {
		return ErrShowWhoAsked
	}
	db := st.Read()
	if s.LibraryFolderID != 0 {
		var lf domain.LibraryFolder
		if err := store.Get(ctx, db, "library_folders", s.LibraryFolderID, &lf); errors.Is(err, store.ErrNotFound) {
			return library.ErrNoFolder
		} else if err != nil {
			return err
		}
		if lf.Kind != "movie" {
			return library.ErrNotMovieFolder
		}
	}
	if s.ShowLibraryFolderID != 0 {
		var lf domain.LibraryFolder
		if err := store.Get(ctx, db, "library_folders", s.ShowLibraryFolderID, &lf); errors.Is(err, store.ErrNotFound) {
			return library.ErrNoFolder
		} else if err != nil {
			return err
		}
		if lf.Kind != "show" {
			return library.ErrNotShowFolder
		}
	}
	if s.MusicLibraryFolderID != 0 {
		var lf domain.LibraryFolder
		if err := store.Get(ctx, db, "library_folders", s.MusicLibraryFolderID, &lf); errors.Is(err, store.ErrNotFound) {
			return library.ErrNoFolder
		} else if err != nil {
			return err
		}
		if lf.Kind != "music" {
			return library.ErrNotMusicFolder
		}
	}
	for _, id := range []int64{s.PickRulesID, s.ShowPickRulesID, s.MusicPickRulesID} {
		if id == 0 {
			continue
		}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pick_rules WHERE id = ?", id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return library.ErrNoPickRules
		}
	}
	who, _ := json.Marshal(by)
	return st.Write(ctx, func(tx *sql.Tx) error {
		now := store.FormatTime(time.Now())
		vals := []string{strconv.Itoa(s.MoviesPerWeek), strconv.FormatInt(s.LibraryFolderID, 10), strconv.FormatInt(s.PickRulesID, 10), strconv.FormatBool(s.SearchOnApprove),
			strconv.FormatInt(s.ShowLibraryFolderID, 10), strconv.FormatInt(s.ShowPickRulesID, 10), strconv.Itoa(s.ShowsPerWeek),
			strconv.FormatInt(s.MusicLibraryFolderID, 10), strconv.FormatInt(s.MusicPickRulesID, 10), strconv.Itoa(s.MusicPerWeek), s.ShowWhoAsked}
		for i, k := range settingKeys {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, vals[i], now, string(who), now, string(who))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// PersonLimit is one person's own weekly limit per kind; nil uses that
// kind's default.
type PersonLimit struct {
	Movies *int `json:"movies_per_week"`
	Shows  *int `json:"shows_per_week"`
	Music  *int `json:"music_per_week"`
}

// SetPersonLimit sets one person's own weekly limits, one per kind; nil
// goes back to that kind's default. It replaces the older all-kinds value.
func SetPersonLimit(ctx context.Context, st *store.Store, userID int64, l PersonLimit) error {
	for _, n := range []*int{l.Movies, l.Shows, l.Music} {
		if n != nil && (*n < 0 || *n > 1000) {
			return ErrPerWeek
		}
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM users WHERE id = ?", userID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return store.ErrNotFound
		}
		_, err := tx.Exec(`INSERT INTO user_ask_settings (user_id, per_week, movies_per_week, shows_per_week, music_per_week) VALUES (?, NULL, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET per_week = NULL, movies_per_week = excluded.movies_per_week,
				shows_per_week = excluded.shows_per_week, music_per_week = excluded.music_per_week`, userID, l.Movies, l.Shows, l.Music)
		return err
	})
}

// Targets is where Alerts reach a person.
type Targets struct {
	Email     string `json:"email"`
	DiscordID string `json:"discord_id"`
	// NewEpisodes: tell this person when new episodes of a show they asked
	// for arrive (on unless turned off).
	NewEpisodes *bool `json:"new_episodes"`
}

// Errors from saving Targets.
var (
	ErrEmail     = errors.New("that email address does not look right")
	ErrDiscordID = errors.New("a Discord user id is a long number (turn on Developer Mode in Discord, then Copy User ID)")
)

// ReadTargets reads where Alerts reach a person.
func ReadTargets(ctx context.Context, db store.Querier, userID int64) (Targets, error) {
	var t Targets
	on := true
	t.NewEpisodes = &on
	err := db.QueryRowContext(ctx, "SELECT email, discord_id, new_episodes = 1 FROM user_ask_settings WHERE user_id = ?", userID).Scan(&t.Email, &t.DiscordID, &on)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return t, err
}

// SaveTargets stores where Alerts reach a person.
func SaveTargets(ctx context.Context, st *store.Store, userID int64, t Targets) error {
	if t.Email != "" && !alerts.CheckAddress(t.Email) {
		return ErrEmail
	}
	if !alerts.CheckDiscordID(t.DiscordID) {
		return ErrDiscordID
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_ask_settings (user_id, email, discord_id) VALUES (?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET email = excluded.email, discord_id = excluded.discord_id`, userID, t.Email, t.DiscordID)
		if err == nil && t.NewEpisodes != nil {
			_, err = tx.Exec("UPDATE user_ask_settings SET new_episodes = ? WHERE user_id = ?", *t.NewEpisodes, userID)
		}
		return err
	})
}
