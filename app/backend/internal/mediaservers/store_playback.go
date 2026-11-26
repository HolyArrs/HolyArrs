package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

type playbackState struct {
	generation int64
	lastSeen   *string
	stale      bool
}

type PlaybackEntry struct {
	ID         int64          `db:"id" json:"id"`
	Session    string         `db:"session_key" json:"-"`
	Account    string         `db:"account_key" json:"account"`
	Item       string         `db:"item_key" json:"-"`
	TitleID    *int64         `db:"title_id" json:"title_id,omitempty"`
	Title      string         `db:"title" json:"title"`
	Person     string         `db:"person" json:"person"`
	Device     string         `db:"device" json:"device"`
	Method     string         `db:"method" json:"method"`
	State      string         `db:"state" json:"state"`
	PositionMS int64          `db:"position_ms" json:"position_ms"`
	DurationMS int64          `db:"duration_ms" json:"duration_ms"`
	WatchMS    int64          `db:"watch_ms" json:"watch_ms"`
	StartedAt  time.Time      `db:"started_at" json:"started_at"`
	SeenAt     time.Time      `db:"seen_at" json:"seen_at"`
	EndedAt    *time.Time     `db:"ended_at" json:"ended_at,omitempty"`
	Incomplete bool           `db:"incomplete" json:"incomplete"`
	Details    *streamDetails `db:"stream_details,json" json:"details,omitempty"`
	Thumb      string         `db:"plex_thumb" json:"-"`
	SourcePart string         `db:"source_part" json:"-"`
	FileID     *int64         `db:"file_id" json:"file_id,omitempty"`
	VersionID  *int64         `db:"version_id" json:"version_id,omitempty"`
}

type playbackSource struct {
	domain.MediaServer
	Generation int64 `db:"playback_generation"`
}

// Read the connection and generation in the same query. A settings change
// between them could let a snapshot from the old connection use the new generation.
func playbackServers(ctx context.Context, q database.Lister) ([]playbackSource, error) {
	return database.Select[playbackSource](ctx, q, `(SELECT media_servers.*, COALESCE(plex_playback_state.generation, 0) AS playback_generation
		FROM media_servers LEFT JOIN plex_playback_state ON media_server_id = media_servers.id)`, "kind = 'plex' AND monitor_playback = 1")
}

func playbackStateFor(ctx context.Context, q database.Querier, id int64) (playbackState, error) {
	var state playbackState
	err := q.QueryRowContext(ctx, "SELECT generation, last_seen_at, stale FROM plex_playback_state WHERE media_server_id = ?", id).
		Scan(&state.generation, &state.lastSeen, &state.stale)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return state, err
}

// resetPlayback invalidates a snapshot already being fetched. Existing plays
// keep their last observed time when a server stops being monitored.
func resetPlayback(ctx context.Context, tx *sql.Tx, id int64, clear bool) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO plex_playback_state(media_server_id, generation) VALUES (?, random())
		ON CONFLICT(media_server_id) DO UPDATE SET generation = excluded.generation, last_seen_at = NULL, stale = 0`, id); err != nil {
		return err
	}
	if clear {
		_, err := tx.ExecContext(ctx, "DELETE FROM plex_playbacks WHERE media_server_id = ?", id)
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE plex_playbacks SET ended_at = seen_at, incomplete = 1 WHERE media_server_id = ? AND ended_at IS NULL", id)
	return err
}

func currentPlayback(ctx context.Context, tx *sql.Tx, id, generation int64) (playbackState, bool, error) {
	var enabled bool
	err := tx.QueryRowContext(ctx, "SELECT kind = 'plex' AND monitor_playback = 1 FROM media_servers WHERE id = ?", id).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return playbackState{}, false, nil
	}
	if err != nil || !enabled {
		return playbackState{}, false, err
	}
	state, err := playbackStateFor(ctx, tx, id)
	return state, state.generation == generation, err
}

func storePlaybackUnavailable(ctx context.Context, store *database.Store, id, generation int64) error {
	return store.Write(ctx, func(tx *sql.Tx) error {
		state, current, err := currentPlayback(ctx, tx, id, generation)
		if err != nil || !current || state.stale {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO plex_playback_state(media_server_id, stale) VALUES (?, 1)
			ON CONFLICT(media_server_id) DO UPDATE SET stale = 1`, id)
		return err
	})
}

func savePlaybackSnapshot(ctx context.Context, store *database.Store, id, generation int64, samples map[playbackKey]playbackSample, at time.Time, seenBefore bool) (bool, error) {
	applied := false
	err := store.Write(ctx, func(tx *sql.Tx) error {
		state, current, err := currentPlayback(ctx, tx, id, generation)
		if err != nil || !current {
			return err
		}
		open, err := database.Select[PlaybackEntry](ctx, tx, "plex_playbacks", "media_server_id = ? AND ended_at IS NULL", id)
		if err != nil {
			return err
		}
		server, err := getServer(ctx, tx, id)
		if err != nil {
			return err
		}
		atText := database.FormatTime(at)
		for _, entry := range open {
			key := playbackKey{Session: entry.Session, Account: entry.Account, Item: entry.Item}
			sample, found := samples[key]
			elapsed := at.Sub(entry.SeenAt)
			gap := !seenBefore || state.stale || elapsed < 0 || elapsed > playbackMaxGap
			if !found {
				if _, err := tx.ExecContext(ctx, "UPDATE plex_playbacks SET ended_at = seen_at, incomplete = incomplete OR ? WHERE id = ?", gap, entry.ID); err != nil {
					return err
				}
				continue
			}
			var observed int64
			if !gap && entry.State == "playing" && sample.state == "playing" {
				observed = elapsed.Milliseconds()
			}
			details := sample.details
			if entry.Details != nil {
				details.BufferMS, details.BufferEvents = entry.Details.BufferMS, entry.Details.BufferEvents
			}
			if sample.state == "buffering" {
				if entry.State != "buffering" || entry.Details == nil {
					details.BufferEvents++
				} else if !gap {
					details.BufferMS += elapsed.Milliseconds()
				}
			}
			bound := entry
			if sample.file != "" && sample.file != entry.SourcePart {
				bound.SourcePart, bound.FileID, bound.VersionID = sample.file, nil, nil
				if err := bindPlaybackFile(ctx, tx, server, &bound); err != nil {
					return err
				}
			}
			raw, err := json.Marshal(details)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE plex_playbacks SET state = ?, position_ms = ?, duration_ms = ?, method = ?,
				seen_at = ?, watch_ms = watch_ms + ?, incomplete = incomplete OR ?, stream_details = ?,
				plex_thumb = COALESCE(NULLIF(?, ''), plex_thumb), source_part = ?, file_id = ?, version_id = ?, title_id = ? WHERE id = ?`,
				sample.state, sample.position, sample.duration, sample.method, atText, observed, gap, string(raw),
				sample.thumb, bound.SourcePart, bound.FileID, bound.VersionID, bound.TitleID, entry.ID); err != nil {
				return err
			}
			delete(samples, key)
		}
		if len(samples) > 0 {
			keys, err := mediafiles.TitlesBySourceID(ctx, tx)
			if err != nil {
				return err
			}
			for key, sample := range samples {
				details := sample.details
				if sample.state == "buffering" {
					details.BufferEvents = 1
				}
				raw, err := json.Marshal(details)
				if err != nil {
					return err
				}
				bound := PlaybackEntry{TitleID: playbackTitle(sample, keys), SourcePart: sample.file}
				if err := bindPlaybackFile(ctx, tx, server, &bound); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO plex_playbacks(media_server_id, session_key, account_key, item_key, title_id,
					title, person, device, method, state, position_ms, duration_ms, started_at, seen_at, stream_details, plex_thumb, source_part, file_id, version_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, key.Session, key.Account, key.Item, bound.TitleID, playbackText(sample.title, 500), playbackText(sample.person, 160),
					playbackText(sample.device, 160), sample.method, sample.state, sample.position, sample.duration, atText, atText,
					string(raw), sample.thumb, bound.SourcePart, bound.FileID, bound.VersionID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plex_playback_state(media_server_id, last_seen_at) VALUES (?, ?)
			ON CONFLICT(media_server_id) DO UPDATE SET last_seen_at = excluded.last_seen_at, stale = 0`, id, atText); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied && err == nil, err
}

type playbackPerson struct {
	Account string `db:"account_key" json:"account"`
	Name    string `db:"name" json:"name"`
}

type playbackPage struct {
	Name       string              `json:"name"`
	Enabled    bool                `json:"enabled"`
	Stale      bool                `json:"stale"`
	LastSeenAt *string             `json:"last_seen_at"`
	Active     []playbackViewEntry `json:"active"`
	History    []playbackViewEntry `json:"history"`
	People     []playbackPerson    `json:"people"`
	Plays      int                 `json:"plays"`
	WatchMS    int64               `json:"watch_ms"`
	Viewers    int                 `json:"viewers"`
	Methods    []playbackMethod    `json:"methods"`
	TopTitles  []playbackRank      `json:"top_titles"`
	TopViewers []playbackRank      `json:"top_viewers"`
}

type playbackMethod struct {
	Method string `db:"method" json:"method"`
	Plays  int    `db:"plays" json:"plays"`
}

type playbackRank struct {
	Key     string `db:"rank_key" json:"key"`
	Name    string `db:"name" json:"name"`
	Plays   int    `db:"plays" json:"plays"`
	WatchMS int64  `db:"watch_ms" json:"watch_ms"`
}

type playbackFilter struct {
	account, title, from, until string
	offset                      int
}

const playbackPageSize = 50

type playbackViewEntry struct {
	PlaybackEntry
	Poster  string `db:"poster" json:"poster,omitempty"`
	Version string `db:"version_label" json:"version,omitempty"`
}

const playbackView = `(SELECT p.*, COALESCE(json_extract(t.artwork, '$[0]'), '') AS poster, COALESCE(v.label, '') AS version_label
	FROM plex_playbacks p LEFT JOIN titles t ON t.id = p.title_id LEFT JOIN versions v ON v.id = p.version_id)`

func playbackPosters(rows []playbackViewEntry, server int64) {
	prefix := "/api/v1/media-servers/" + strconv.FormatInt(server, 10) + "/playback/"
	for i := range rows {
		if rows[i].Poster == "" && rows[i].Thumb != "" {
			rows[i].Poster = prefix + strconv.FormatInt(rows[i].ID, 10) + "/poster"
		}
	}
}

func readPlayback(ctx context.Context, q watchReader, m domain.MediaServer, filter playbackFilter, now time.Time) (playbackPage, error) {
	out := playbackPage{Name: m.Name, Enabled: m.MonitorPlayback}
	state, err := playbackStateFor(ctx, q, m.ID)
	if err != nil {
		return out, err
	}
	out.LastSeenAt = state.lastSeen
	out.Stale = state.stale
	if state.lastSeen != nil {
		at, err := time.Parse(database.TimeLayout, *state.lastSeen)
		out.Stale = out.Stale || err != nil || now.Sub(at) > playbackMaxGap
	}
	out.Active, err = database.Select[playbackViewEntry](ctx, q, playbackView, "media_server_id = ? AND ended_at IS NULL ORDER BY started_at, id", m.ID)
	if err != nil {
		return out, err
	}
	out.People, err = database.Select[playbackPerson](ctx, q,
		"(SELECT account_key, MAX(person) AS name FROM plex_playbacks WHERE media_server_id = ? GROUP BY account_key)", "1=1 ORDER BY name, account_key", m.ID)
	if err != nil {
		return out, err
	}
	where := "media_server_id = ? AND ended_at IS NOT NULL"
	args := []any{m.ID}
	if filter.account != "" {
		where += " AND account_key = ?"
		args = append(args, filter.account)
	}
	if filter.title != "" {
		where += " AND instr(lower(title), lower(?)) > 0"
		args = append(args, filter.title)
	}
	if filter.from != "" {
		where += " AND started_at >= ?"
		args = append(args, filter.from)
	}
	if filter.until != "" {
		where += " AND started_at < ?"
		args = append(args, filter.until)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(watch_ms), 0), COUNT(DISTINCT account_key) FROM plex_playbacks WHERE "+where, args...).Scan(&out.Plays, &out.WatchMS, &out.Viewers); err != nil {
		return out, err
	}
	out.Methods, err = database.Select[playbackMethod](ctx, q, "(SELECT method, COUNT(*) AS plays FROM plex_playbacks WHERE "+where+" GROUP BY method)", "1=1 ORDER BY plays DESC, method", args...)
	if err != nil {
		return out, err
	}
	rank := func(key, name string) ([]playbackRank, error) {
		return database.Select[playbackRank](ctx, q, "(SELECT "+key+" AS rank_key, MAX("+name+") AS name, COUNT(*) AS plays, COALESCE(SUM(watch_ms), 0) AS watch_ms FROM plex_playbacks WHERE "+where+
			" GROUP BY "+key+")", "1=1 ORDER BY watch_ms DESC, plays DESC, rank_key LIMIT 5", args...)
	}
	out.TopTitles, err = rank("item_key", "title")
	if err != nil {
		return out, err
	}
	out.TopViewers, err = rank("account_key", "person")
	if err != nil {
		return out, err
	}
	out.History, err = database.Select[playbackViewEntry](ctx, q, playbackView, where+" ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?", append(args, playbackPageSize, filter.offset)...)
	playbackPosters(out.Active, m.ID)
	playbackPosters(out.History, m.ID)
	return out, err
}
