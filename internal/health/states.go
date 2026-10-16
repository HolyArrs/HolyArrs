package health

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/outside"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// siteReason says why a subtitle site waits, by pause reason.
var siteReason = map[string]string{
	"down":           "HolyArrs could not reach it; it tries again on its own.",
	"rate_limited":   "It asked HolyArrs to slow down; it tries again on its own.",
	"download_limit": "Its download limit for today is used up; it starts again tomorrow.",
	"blocked":        "Its address is on the local network, which this site may not reach.",
	"unreadable":     "It answered with something HolyArrs could not read.",
	"login_refused":  "It turned down the sign-in. Check the account, then test it.",
}

// Links returns the state of every linked app: each Source, Downloader,
// subtitle site and media server, in the five state names. Paused by a
// person is off; waiting to try again is retrying; a refused sign-in or a
// broken answer is failed; one never checked yet is starting.
func (e *Engine) Links(ctx context.Context) ([]supervisor.Status, error) {
	db := e.Store.Read()
	var out []supervisor.Status
	add := func(kind string, id int64, name string, st supervisor.State, reason string) {
		out = append(out, supervisor.Status{Key: "link:" + kind + ":" + strconv.FormatInt(id, 10), Kind: kind, Name: name, State: st, Reason: reason})
	}
	ss, err := store.Select[domain.Source](ctx, db, "sources", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	for _, s := range ss {
		h := source.HealthOf(s)
		switch {
		case h.State == source.StateDisabled:
			add("source", s.ID, s.Name, supervisor.Off, h.Text)
		case h.State == source.StateAuthFailed:
			add("source", s.ID, s.Name, supervisor.Failed, h.Text)
		case s.PauseReason != nil && *s.PauseReason == source.KindBroken, s.PauseReason != nil && *s.PauseReason == source.KindBlocked:
			add("source", s.ID, s.Name, supervisor.Failed, h.Text)
		case h.State == source.StateUnreachable:
			add("source", s.ID, s.Name, supervisor.Retrying, h.Text)
		default:
			add("source", s.ID, s.Name, supervisor.Running, "")
		}
	}
	ds, err := store.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		// Reasons are fixed plain words: the last error (addresses,
		// paths, raw answers) stays in the log, where it was written.
		switch d.State {
		case "disabled":
			add("downloader", d.ID, d.Name, supervisor.Off, "Paused by you.")
		case "unreachable":
			add("downloader", d.ID, d.Name, supervisor.Retrying, "It does not answer; HolyArrs tries again on its own. The log has the details.")
		case "auth_failed":
			add("downloader", d.ID, d.Name, supervisor.Failed, "It turned down the sign-in. Check its user name and password or key; the log has the details.")
		default:
			add("downloader", d.ID, d.Name, supervisor.Running, "")
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT id, name, state, pause_reason FROM subtitle_sites ORDER BY priority, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, state string
		var reason sql.NullString
		if err := rows.Scan(&id, &name, &state, &reason); err != nil {
			return nil, err
		}
		switch {
		case state == "disabled":
			add("subtitle_site", id, name, supervisor.Off, "Paused by you.")
		case state == "paused" && (reason.String == "login_refused" || reason.String == "blocked" || reason.String == "unreadable"):
			add("subtitle_site", id, name, supervisor.Failed, siteReason[reason.String])
		case state == "paused":
			text := siteReason[reason.String]
			if text == "" {
				text = siteReason["down"]
			}
			add("subtitle_site", id, name, supervisor.Retrying, text)
		default:
			add("subtitle_site", id, name, supervisor.Running, "")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ms, err := store.Select[domain.MediaServer](ctx, db, "media_servers", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		switch {
		case !m.Enabled:
			add("media_server", m.ID, m.Name, supervisor.Off, "Turned off.")
		case strings.TrimSpace(m.LastError) != "":
			add("media_server", m.ID, m.Name, supervisor.Failed, "The last refresh failed. The log has the details.")
		case m.LastOKAt == nil:
			add("media_server", m.ID, m.Name, supervisor.Starting, "Not told about a change yet.")
		default:
			add("media_server", m.ID, m.Name, supervisor.Running, "")
		}
	}
	for _, s := range outside.States() {
		add("outside_call", 0, s.Kind+" "+s.ID, supervisor.Retrying, s.Reason)
		out[len(out)-1].Key = "outside:" + s.ID
	}
	return out, nil
}
