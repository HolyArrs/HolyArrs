package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Show names the show a message is about.
type Show struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Year   *int   `json:"year,omitempty"`
	TVDB   string `json:"tvdb,omitempty"`
	TMDB   string `json:"tmdb,omitempty"`
	TVMaze string `json:"tvmaze,omitempty"`
	IMDb   string `json:"imdb,omitempty"`
}

// Episode names one episode a message is about.
type Episode struct {
	Season int    `json:"season"`
	Number int    `json:"number"`
	Name   string `json:"name,omitempty"`
}

// Label is S01E02.
func (e Episode) Label() string { return fmt.Sprintf("S%02dE%02d", e.Season, e.Number) }

// How a pack import is told (setting pack_alerts).
const (
	PackOne  = "one"  // one Alert per pack import, listing its episodes
	PackEach = "each" // one Alert per episode
)

// DefaultEachLimit is how many per-episode Alerts one pack import sends with
// pack_alerts each before the rest go in one summary Alert.
const DefaultEachLimit = 25

// Settings are the Alert settings that belong to no single Alert (group
// 'alerts'). Older installs may still hold pack_alerts in the 'links' group;
// that row is read until the settings are saved.
type Settings struct {
	PackAlerts string `json:"pack_alerts"`
	EachLimit  int    `json:"each_limit"`
}

// ReadSettings reads the Alert settings; unset or odd ones keep their
// default (one Alert per pack, at most 25 per-episode Alerts).
func ReadSettings(ctx context.Context, db database.Querier) Settings {
	s := Settings{PackAlerts: PackOne, EachLimit: DefaultEachLimit}
	var v string
	if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'pack_alerts' AND grp IN ('alerts', 'links') ORDER BY grp = 'alerts' DESC LIMIT 1").Scan(&v) == nil && (v == PackOne || v == PackEach) {
		s.PackAlerts = v
	}
	if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'alerts' AND key = 'each_limit'").Scan(&v) == nil {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= MaxEachLimit {
			s.EachLimit = n
		}
	}
	return s
}

// MaxEachLimit caps each_limit.
const MaxEachLimit = 500

// SaveSettings stores the Alert settings in the alerts group.
func SaveSettings(ctx context.Context, st *database.Store, s Settings, by domain.Actor) error {
	if s.PackAlerts != PackOne && s.PackAlerts != PackEach {
		return fmt.Errorf("pack_alerts must be %s or %s", PackOne, PackEach)
	}
	if s.EachLimit == 0 { // left out
		s.EachLimit = DefaultEachLimit
	}
	if s.EachLimit < 1 || s.EachLimit > MaxEachLimit {
		return fmt.Errorf("each_limit must be 1 to %d", MaxEachLimit)
	}
	actor, _ := json.Marshal(by)
	now := database.FormatTime(time.Now())
	return st.Write(ctx, func(tx *sql.Tx) error {
		for _, kv := range [][2]string{{"pack_alerts", s.PackAlerts}, {"each_limit", strconv.Itoa(s.EachLimit)}} {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('alerts', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				kv[0], kv[1], now, string(actor), now, string(actor)); err != nil {
				return err
			}
		}
		_, err := tx.Exec("DELETE FROM settings WHERE grp = 'links' AND key = 'pack_alerts'")
		return err
	})
}

// slotIDsOf finds the Slots an Event is about: those it names (an import
// names the ones it placed), else its Download's, else its own Slot.
// SlotIDsOf lists the Slots an Event is about.
func SlotIDsOf(ctx context.Context, db database.Querier, e domain.Event) []int64 {
	return slotIDsOf(ctx, db, e)
}

func slotIDsOf(ctx context.Context, db database.Querier, e domain.Event) []int64 {
	for _, k := range []string{"placed_slot_ids", "slot_ids"} {
		if raw, ok := e.Data[k].([]any); ok && len(raw) > 0 {
			var out []int64
			for _, x := range raw {
				if f, ok := x.(float64); ok {
					out = append(out, int64(f))
				}
			}
			return out
		}
		if ids, ok := e.Data[k].([]int64); ok && len(ids) > 0 {
			return ids
		}
	}
	if e.DownloadID != nil {
		var raw string
		if db.QueryRowContext(ctx, "SELECT slot_ids FROM downloads WHERE id = ?", *e.DownloadID).Scan(&raw) == nil {
			var ids []int64
			if json.Unmarshal([]byte(raw), &ids) == nil && len(ids) > 0 {
				return ids
			}
		}
	}
	if e.SlotID != nil {
		return []int64{*e.SlotID}
	}
	return nil
}

// episodesOf lists the episodes of Slots, in episode order, each once.
func episodesOf(ctx context.Context, db database.Querier, slotIDs []int64) []Episode {
	var out []Episode
	seen := map[[2]int]bool{}
	for _, id := range slotIDs {
		var se, ep sql.NullInt64
		var name sql.NullString
		if db.QueryRowContext(ctx, `SELECT json_extract(i.numbers, '$.season'), json_extract(i.numbers, '$.episode'), i.name FROM slots s
			JOIN items i ON i.id = s.item_id WHERE s.id = ? AND i.kind = 'episode'`, id).Scan(&se, &ep, &name) != nil || !se.Valid || !ep.Valid {
			continue
		}
		k := [2]int{int(se.Int64), int(ep.Int64)}
		if seen[k] {
			continue
		}
		seen[k] = true
		e := Episode{Season: k[0], Number: k[1]}
		if name.Valid && !strings.EqualFold(name.String, "TBA") {
			e.Name = name.String
		}
		out = append(out, e)
	}
	for i := 1; i < len(out); i++ { // few episodes: insertion sort
		for j := i; j > 0 && (out[j].Season < out[j-1].Season || out[j].Season == out[j-1].Season && out[j].Number < out[j-1].Number); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// episodesFromLabels reads "S01E02" labels an Event lists.
func episodesFromLabels(e domain.Event) []Episode {
	raw, _ := e.Data["episodes"].([]any)
	var out []Episode
	for _, x := range raw {
		s, _ := x.(string)
		var ep Episode
		if _, err := fmt.Sscanf(s, "S%02dE%02d", &ep.Season, &ep.Number); err == nil {
			out = append(out, ep)
		}
	}
	return out
}

// episodeWords names episodes for a subject: S01E02, or S01E01 to S01E03
// (3 episodes).
func episodeWords(eps []Episode) string { return episodeWordsIn("", eps) }

// episodeWordsIn is episodeWords in an app language.
func episodeWordsIn(lang string, eps []Episode) string {
	switch len(eps) {
	case 0:
		return ""
	case 1:
		return eps[0].Label()
	}
	return say(lang, "episode_range", "from", eps[0].Label(), "to", eps[len(eps)-1].Label(), "n", strconv.Itoa(len(eps)))
}

// episodeLines lists episodes with their names, one per line.
func episodeLines(eps []Episode) string {
	lines := make([]string, len(eps))
	for i, e := range eps {
		lines[i] = strings.TrimSpace(e.Label() + " " + e.Name)
	}
	return strings.Join(lines, "\n")
}
