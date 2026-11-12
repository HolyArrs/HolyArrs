package downloads

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

func (a *Web) removeDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	st, err := a.readDownloaderSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	var in struct {
		DeleteFiles *bool `json:"delete_files"`
		TurnDown    *bool `json:"turn_down"`
		SearchAgain *bool `json:"search_again"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"delete_files": bool, "turn_down": bool, "search_again": bool}, each optional (defaults in Settings > Downloaders)`) {
		return
	}
	c := RemoveChoice{DeleteFiles: st.RemoveDeleteFiles, TurnDown: st.RemoveTurnDown, SearchAgain: st.RemoveSearchAgain}
	if in.DeleteFiles != nil {
		c.DeleteFiles = *in.DeleteFiles
	}
	if in.TurnDown != nil {
		c.TurnDown = *in.TurnDown
	}
	if in.SearchAgain != nil {
		c.SearchAgain = *in.SearchAgain
	}
	who := ""
	if p, ok := webserver.From(r.Context()); ok {
		who = p.Name
	}
	err = a.Manager().Remove(r.Context(), id, c, actor(r), who)
	switch {
	case errors.Is(err, database.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		webserver.Fail(w, http.StatusNotFound, "no such download")
	case errors.Is(err, ErrNotActive):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		var d domain.Download
		_ = database.Get(r.Context(), a.Store.Read(), "downloads", id, &d)
		if d.ExternalID != "" {
			a.forgetTorrents(r.Context(), d.DownloaderID, []string{d.ExternalID})
		}
		v := DownloadViewOf(d)
		v.Label = PhaseLabels["removed"]
		if d.StateReason != nil {
			v.Text = upperFirst(*d.StateReason) + "."
		}
		webserver.WriteJSON(w, http.StatusOK, v)
	}
}

// DecisionView is one movie waiting for a person on Needs a decision.
type DecisionView struct {
	SlotID      int64     `json:"slot_id"`
	TitleID     int64     `json:"title_id"`
	Title       string    `json:"title"`
	Year        *int      `json:"year,omitempty"`
	Version     string    `json:"version"`
	Versions    int       `json:"versions"` // how many Versions the Title has
	Why         string    `json:"why"`      // ran_out, conflict, not_replaced, not_sent, taken
	Reason      string    `json:"reason"`
	ReleaseID   *int64    `json:"release_id,omitempty"`
	ReleaseName string    `json:"release_name,omitempty"`
	HasCopy     bool      `json:"has_copy"`
	Since       time.Time `json:"since"`
	// Shows: the episode, and the pack it came in (the other episodes of
	// the same download that also wait for a decision).
	// Artists: Episode is the track, Pack every track of the album
	// download, and Album names the album.
	Kind        string        `json:"kind"` // movie, show, artist
	Episode     *EpisodeLine  `json:"episode,omitempty"`
	Album       string        `json:"album,omitempty"`
	DownloadID  *int64        `json:"download_id,omitempty"`
	PackSlotIDs []int64       `json:"pack_slot_ids,omitempty"`
	Pack        []EpisodeLine `json:"pack,omitempty"` // every episode of that download
}

// why sorts a Slot's last reason into the ways it ends up waiting.
func why(reason string) string {
	switch {
	case strings.Contains(reason, "could not be placed") && filesTaken(reason):
		// An older import whose files another app had already taken.
		return "taken"
	case strings.Contains(reason, "not replaced (setting)"):
		return "not_replaced"
	case strings.HasPrefix(reason, "not sent:"):
		// No Downloader takes the title's tags.
		return "not_sent"
	case strings.HasPrefix(reason, "nothing could be placed"), strings.Contains(reason, "import"),
		strings.Contains(reason, "could not be placed"), strings.HasPrefix(reason, "nothing in the download could be matched"),
		// The unpack reasons (importer/unpack.go): the import could not finish, Releases did not run out.
		strings.Contains(reason, "RAR file"), strings.Contains(reason, "archive file"), strings.HasPrefix(reason, "Packed in "), strings.HasPrefix(reason, "Not enough free space to unpack"),
		// The file was refused in place of the copy, or its edition was not settled (importer/upgrade.go, copyfacts.go).
		strings.Contains(reason, "copy in place"), strings.HasPrefix(reason, "edition "):
		return "conflict"
	}
	return "ran_out"
}

// filesTaken reports whether a placing error says the download's file was
// no longer there.
func filesTaken(reason string) bool {
	r := strings.ToLower(reason)
	for _, w := range []string{"no such file or directory", "cannot find the file", "cannot find the path", "not in the download folder any more"} {
		if strings.Contains(r, w) {
			return true
		}
	}
	return false
}

func (a *Web) decisions(ctx context.Context, slotID int64) ([]DecisionView, error) {
	q := `SELECT s.id, t.id, t.name, t.year, v.label, (SELECT count(*) FROM versions v2 WHERE v2.title_id = t.id),
		COALESCE(s.last_reason, ''), s.file_id IS NOT NULL, s.updated_at,
		(SELECT d.release_id FROM downloads d WHERE EXISTS (SELECT 1 FROM json_each(d.slot_ids) WHERE value = s.id) ORDER BY d.id DESC LIMIT 1),
		t.kind, (SELECT d.id FROM downloads d WHERE EXISTS (SELECT 1 FROM json_each(d.slot_ids) WHERE value = s.id) ORDER BY d.id DESC LIMIT 1)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id JOIN versions v ON v.id = s.version_id
		WHERE s.state = 'needs_a_decision' AND t.state != 'removed'`
	args := []any{}
	if slotID > 0 {
		q += " AND s.id = ?"
		args = append(args, slotID)
	}
	rows, err := a.Store.Read().QueryContext(ctx, q+" ORDER BY s.updated_at DESC, s.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DecisionView{}
	for rows.Next() {
		var dv DecisionView
		var year, rel, dl sql.NullInt64
		var since string
		if err := rows.Scan(&dv.SlotID, &dv.TitleID, &dv.Title, &year, &dv.Version, &dv.Versions, &dv.Reason, &dv.HasCopy, &since, &rel, &dv.Kind, &dl); err != nil {
			return nil, err
		}
		if dl.Valid {
			dv.DownloadID = &dl.Int64
		}
		if year.Valid {
			y := int(year.Int64)
			dv.Year = &y
		}
		if rel.Valid {
			dv.ReleaseID = &rel.Int64
		}
		dv.Since, _ = time.Parse(database.TimeLayout, since)
		dv.Why = why(dv.Reason)
		out = append(out, dv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Kind == "artist" {
			_ = a.Store.Read().QueryRowContext(ctx, `SELECT p.name FROM slots s JOIN items i ON i.id = s.item_id JOIN parts p ON p.id = i.part_id
				WHERE s.id = ?`, out[i].SlotID).Scan(&out[i].Album)
		}
		if out[i].Kind == "show" || out[i].Kind == "artist" {
			if eps := episodeLines(ctx, a.Store.Read(), []int64{out[i].SlotID}); len(eps) == 1 {
				out[i].Episode = &eps[0]
			}
			if out[i].DownloadID != nil {
				var d domain.Download
				if database.Get(ctx, a.Store.Read(), "downloads", *out[i].DownloadID, &d) == nil && len(d.SlotIDs) > 1 {
					out[i].Pack = episodeLines(ctx, a.Store.Read(), d.SlotIDs)
					for _, e := range out[i].Pack {
						if e.State == string(mediafiles.SlotNeedsADecision) {
							out[i].PackSlotIDs = append(out[i].PackSlotIDs, e.SlotID)
						}
					}
				}
			}
		}
		if out[i].ReleaseID != nil {
			_ = a.Store.Read().QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *out[i].ReleaseID).Scan(&out[i].ReleaseName)
		}
		if out[i].Reason != "" {
			out[i].Reason = upperFirst(out[i].Reason) + "."
		}
	}
	return out, nil
}

func (a *Web) needsDecision(w http.ResponseWriter, r *http.Request) {
	list, err := a.decisions(r.Context(), 0)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"items": list, "count": len(list)})
}

// decide runs a person's choice for a Slot that needs a decision:
// search (search again by hand), turn_down (turn its last Release down,
// then search again) or stop (stop looking: Keep up to date off for that
// Version, the Slot rests). Choosing a Release by hand and trying another
// Version happen on the Movie page.
// errMovedOn is a Slot that left Needs a decision while a choice was made.
var errMovedOn = errors.New("the slot no longer needs a decision")

// BeforeDecide lets tests change a Slot between the read and the write.
var BeforeDecide func()

func (a *Web) decide(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "slot")
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
		Pack   bool   `json:"pack"`
	}
	if !webserver.Decode(w, r, &in, `body is {"action": "search" | "turn_down" | "stop", "pack": bool (optional, for every episode of the same download)}`) {
		return
	}
	list, err := a.decisions(r.Context(), id)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if len(list) == 0 {
		webserver.Fail(w, http.StatusNotFound, "this movie or episode does not need a decision")
		return
	}
	dv := list[0]
	noun := "movie"
	many := dv.Kind == "show" || dv.Kind == "artist" // one Slot of many in a download
	switch dv.Kind {
	case "show":
		noun = "episode"
	case "artist":
		noun = "track"
	}
	targets := []DecisionView{dv}
	if in.Pack {
		if !many || len(dv.PackSlotIDs) == 0 {
			webserver.Fail(w, http.StatusBadRequest, "this "+noun+" did not come in a pack")
			return
		}
		targets = nil
		for _, sid := range dv.PackSlotIDs {
			if l, err := a.decisions(r.Context(), sid); err == nil && len(l) == 1 {
				targets = append(targets, l[0])
			}
		}
	}
	if BeforeDecide != nil {
		BeforeDecide()
	}
	by := actor(r)
	who := "someone"
	if p, ok := webserver.From(r.Context()); ok {
		who = p.Name
	}
	switch in.Action {
	case "turn_down":
		if dv.ReleaseID == nil {
			webserver.Fail(w, http.StatusConflict, "there is no Release to turn down for this "+noun)
			return
		}
		reason := "turned down by " + who + " on Needs a decision"
		// An episode turns the Release down for that episode only; a pack
		// (or a movie) for everything it holds.
		if many && !in.Pack {
			err = a.searcher().TurnDownFor(r.Context(), *dv.ReleaseID, dv.SlotID, reason, by)
		} else {
			err = a.searcher().TurnDown(r.Context(), *dv.ReleaseID, reason, by)
		}
		if err != nil {
			sources.SearchError(w, err)
			return
		}
		fallthrough
	case "search":
		if a.Queue == nil {
			webserver.Fail(w, http.StatusServiceUnavailable, "searches are not running")
			return
		}
		var queued []int64
		for _, t := range targets {
			jid, _, err := a.Queue.Enqueue(r.Context(), jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: t.SlotID}, map[string]any{"by_hand": true},
				"hand_search_slot:"+itoa(t.SlotID), by)
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			queued = append(queued, jid)
		}
		webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": queued[0], "job_ids": queued, "action": in.Action})
	case "stop":
		to := mediafiles.SlotWanted
		if dv.HasCopy {
			to = mediafiles.SlotBetterWanted
		}
		reason := "stopped looking (" + who + ")"
		var stopped []DecisionView
		err := a.Store.Write(r.Context(), func(tx *sql.Tx) error {
			stopped = nil
			for _, t := range targets {
				// The Slot may have moved on since it was read: only one that
				// still needs a decision stops, and nothing is written otherwise.
				var state string
				if err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", t.SlotID).Scan(&state); err != nil {
					return err
				}
				if state != string(mediafiles.SlotNeedsADecision) {
					if t.SlotID == id {
						return errMovedOn
					}
					continue
				}
				// A movie stops for its Version; an episode for itself, so
				// the rest of the show keeps going.
				q := "UPDATE versions SET keep_up_to_date = 0, updated_at = ? WHERE id = (SELECT version_id FROM slots WHERE id = ?)"
				if t.Kind == "show" || t.Kind == "artist" {
					q = "UPDATE items SET keep_up_to_date = 0, updated_at = ? WHERE id = (SELECT item_id FROM slots WHERE id = ?)"
				}
				if _, err := tx.Exec(q, database.FormatTime(time.Now()), t.SlotID); err != nil {
					return err
				}
				st := mediafiles.SlotWanted
				if t.HasCopy {
					st = mediafiles.SlotBetterWanted
				}
				if err := mediafiles.MoveSlotTx(tx, t.SlotID, st, reason, by); err != nil {
					return err
				}
				stopped = append(stopped, t)
			}
			return nil
		})
		if errors.Is(err, errMovedOn) {
			webserver.Fail(w, http.StatusConflict, "this "+noun+" no longer needs a decision")
			return
		}
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for _, t := range stopped {
			tid, sid := t.TitleID, t.SlotID
			name := t.Title + " (" + t.Version + ")"
			if t.Episode != nil {
				name = t.Title + " " + t.Episode.Label
			}
			e := &domain.Event{Actor: by, Kind: "stopped_looking", Summary: "Stopped looking for " + name, TitleID: &tid, SlotID: &sid}
			if a.Store.AddEvent(r.Context(), e) == nil && a.Live != nil {
				a.Live.Publish("event", e)
			}
			st := mediafiles.SlotWanted
			if t.HasCopy {
				st = mediafiles.SlotBetterWanted
			}
			if a.Live != nil {
				a.Live.Publish("slot", map[string]any{"id": sid, "state": st, "reason": reason})
			}
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"action": "stop", "state": to, "stopped": len(stopped)})
	default:
		webserver.Fail(w, http.StatusBadRequest, `action is "search", "turn_down" or "stop"`)
	}
}

// clearNow tries every imported download still waiting to be cleared from
// its Downloader at once, and answers how many still wait.
func (a *Web) clearNow(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, map[string]int{"left": a.Manager().ClearNow(r.Context())})
}
