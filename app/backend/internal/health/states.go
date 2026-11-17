package health

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
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

// SourcesOffReason is why every Source shows as off while the Sources part
// is off.
const SourcesOffReason = "Sources are turned off in Settings > Parts."

// Links returns the state of every linked app: each Source, Downloader,
// subtitle site and media server, in the five state names. Paused by a
// person is off; waiting to try again is retrying; a refused sign-in or a
// broken answer is failed; one never checked yet is starting.
func (e *Engine) Links(ctx context.Context) ([]jobs.Status, error) {
	db := e.Store.Read()
	var out []jobs.Status
	add := func(kind string, id int64, name string, st jobs.State, reason string) {
		out = append(out, jobs.Status{Key: "link:" + kind + ":" + strconv.FormatInt(id, 10), Kind: kind, Name: name, State: st, Reason: reason})
	}
	ss, err := database.Select[domain.Source](ctx, db, "sources", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	off := e.SourcesOn != nil && !e.SourcesOn()
	for _, s := range ss {
		h := sources.HealthOf(s)
		switch {
		case off:
			add("source", s.ID, s.Name, jobs.Off, SourcesOffReason)
		case h.State == sources.StateDisabled:
			add("source", s.ID, s.Name, jobs.Off, h.Text)
		case h.State == sources.StateAuthFailed:
			add("source", s.ID, s.Name, jobs.Failed, h.Text)
		case s.PauseReason != nil && *s.PauseReason == sources.KindBroken, s.PauseReason != nil && *s.PauseReason == sources.KindBlocked:
			add("source", s.ID, s.Name, jobs.Failed, h.Text)
		case h.State == sources.StateUnreachable:
			add("source", s.ID, s.Name, jobs.Retrying, h.Text)
		default:
			add("source", s.ID, s.Name, jobs.Running, "")
		}
	}
	ds, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		// Reasons are fixed plain words: the last error (addresses,
		// paths, raw answers) stays in the log, where it was written.
		switch d.State {
		case "disabled":
			add("downloader", d.ID, d.Name, jobs.Off, "Paused by you.")
		case "unreachable":
			add("downloader", d.ID, d.Name, jobs.Retrying, "It does not answer; HolyArrs tries again on its own. The log has the details.")
		case "auth_failed":
			add("downloader", d.ID, d.Name, jobs.Failed, "It turned down the sign-in. Check its user name and password or key; the log has the details.")
		default:
			add("downloader", d.ID, d.Name, jobs.Running, "")
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
			add("subtitle_site", id, name, jobs.Off, "Paused by you.")
		case state == "paused" && (reason.String == "login_refused" || reason.String == "blocked" || reason.String == "unreadable"):
			add("subtitle_site", id, name, jobs.Failed, siteReason[reason.String])
		case state == "paused":
			text := siteReason[reason.String]
			if text == "" {
				text = siteReason["down"]
			}
			add("subtitle_site", id, name, jobs.Retrying, text)
		default:
			add("subtitle_site", id, name, jobs.Running, "")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ms, err := database.Select[domain.MediaServer](ctx, db, "media_servers", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		switch {
		case !m.Enabled:
			add("media_server", m.ID, m.Name, jobs.Off, "Turned off.")
		case strings.TrimSpace(m.LastError) != "":
			add("media_server", m.ID, m.Name, jobs.Failed, "The last refresh failed. The log has the details.")
		case m.LastOKAt == nil:
			add("media_server", m.ID, m.Name, jobs.Starting, "Not told about a change yet.")
		default:
			add("media_server", m.ID, m.Name, jobs.Running, "")
		}
	}
	outsideStates := e.Outside
	if outsideStates == nil {
		outsideStates = outbound.States
	}
	// The breaker's ID is a keyed fingerprint of the call (its address and
	// sign-in): it means nothing to a person and stays on the server, so the
	// row says what kind of service it is and the key only counts.
	seen := map[string]int{}
	for _, s := range outsideStates() {
		seen[s.Kind]++
		add("outside_call", 0, outsideName(s.Kind), jobs.Retrying, s.Reason)
		out[len(out)-1].Key = "outside:" + s.Kind + ":" + strconv.Itoa(seen[s.Kind])
	}
	return out, nil
}

// outsideName is the plain name of an outside call's kind.
func outsideName(kind string) string {
	switch {
	case strings.HasPrefix(kind, "metadata"):
		return "Metadata server"
	}
	switch kind {
	case "downloader":
		return "Downloader"
	case "media_server":
		return "Media server"
	case "source":
		return "Source"
	case "subtitle_site":
		return "Subtitle site"
	case "outside_list":
		return "Outside list"
	case "plex_watchlist":
		return "Plex watchlist"
	case "oidc":
		return "Single sign-on"
	case "alert":
		return "Alerts"
	case "update_feed":
		return "Update check"
	}
	return "Outside service"
}
