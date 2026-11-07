package source

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// KindSearchSlot is the Job kind that searches every enabled Source for one Slot.
const KindSearchSlot = "search_slot"

// Publisher sends live events (live.Hub fits).
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Searcher runs search_slot Jobs.
type Searcher struct {
	Store *database.Store
	Live  Publisher // may be nil
	// OnChosen is told when a Release is chosen for a Slot (to send it to a Downloader).
	OnChosen func(ctx context.Context, slotID, releaseID int64)
	// Secrets opens a Source's secret_ref (its API key). Nil means no keys.
	Secrets SecretReader
	// Queue takes the searches a missing search queues.
	Queue *jobs.Queue

	mu         sync.Mutex
	clients    map[int64]*Client
	onAsk      func(name string) // tests: told before each Source is asked
	quietMu    sync.Mutex
	quietCalls map[int64]*quietSourceCalls
}

func (s *Searcher) publish(topic string, data any) {
	if s.Live != nil {
		s.Live.Publish(topic, data)
	}
}

// SecretReader opens stored secrets.
type SecretReader interface {
	Get(ctx context.Context, id int64) (string, error)
}

// client keeps one Client per Source so its limits hold across Jobs.
// A Source whose settings, key or proxy changed gets a new Client.
func (s *Searcher) client(ctx context.Context, src domain.Source) (*Client, error) {
	cfg, err := ConfigFor(ctx, src, s.Secrets)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.clients[src.ID]; c != nil && sameConfig(c.cfg, cfg) {
		return c, nil
	}
	id := src.ID
	cfg.Observe = func(op string, took time.Duration, err error) {
		Record(context.Background(), s.Store, id, op, took, err)
	}
	c := New(cfg)
	if s.clients == nil {
		s.clients = map[int64]*Client{}
	}
	s.clients[src.ID] = c
	return c, nil
}

func sameConfig(a, b Config) bool {
	proxy := func(c Config) string {
		if c.Proxy == nil {
			return ""
		}
		return c.Proxy.String()
	}
	return a.URL == b.URL && a.APIKey == b.APIKey && a.AllowLocal == b.AllowLocal && a.Format == b.Format && a.Protocol == b.Protocol &&
		a.GapMS == b.GapMS && a.PerWindow == b.PerWindow && a.Window == b.Window && a.Timeout == b.Timeout &&
		proxy(a) == proxy(b) && a.ProxyAllowLocal == b.ProxyAllowLocal
}

// hidden marks where a Source key was taken out of a stored link. The
// number says which key value, the letter which spelling (r as it is, q
// query-escaped, p path-escaped).
const hidden = "HOLYARRSKEY"

// secretValues lists the key values a result link may carry: a
// Torznab/Newznab apikey, or the values in an rss Source's key (its
// address's query, where feeds keep their passkey), then the passkeys
// its address path holds.
func secretValues(cfg Config) []string {
	if cfg.APIKey == "" && len(cfg.PathKeys) == 0 {
		return nil
	}
	var out []string
	if cfg.Format != "rss" {
		if cfg.APIKey != "" {
			out = append(out, cfg.APIKey)
		}
	} else {
		// Feeds separate values with & or ; (u=1;tp=PASSKEY).
		for _, part := range strings.FieldsFunc(cfg.APIKey, func(r rune) bool { return r == '&' || r == ';' }) {
			_, v, _ := strings.Cut(part, "=")
			if u, err := url.QueryUnescape(v); err == nil {
				v = u
			}
			if len(v) >= 4 && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	// Path passkeys come after, so markers stored before keep their number.
	for _, v := range cfg.PathKeys {
		if len(v) >= 4 && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

type spelling struct{ marker, value string }

// spellings lists every key value as it may be written in a link, longest
// first, each with its own marker.
func spellings(keys []string) []spelling {
	var out []spelling
	for i, k := range keys {
		for _, f := range []struct {
			tag string
			v   string
		}{{"r", k}, {"q", url.QueryEscape(k)}, {"p", url.PathEscape(k)}} {
			out = append(out, spelling{fmt.Sprintf("%s%d%s", hidden, i, f.tag), f.v})
		}
	}
	slices.SortStableFunc(out, func(a, b spelling) int { return len(b.value) - len(a.value) })
	return out
}

// cleanLink takes the Source's key values out of a result link (in the
// query or the path), so a stored Release never holds the Source secret;
// GrabLink puts them back. Another apikey parameter is dropped.
func cleanLink(link string, keys []string) string {
	// A marker already in the feed's link was planted by someone else;
	// break it so GrabLink never fills the Source key into it.
	link = hideKeys(strings.ReplaceAll(link, hidden, "HOLYARRS-KEY"), keys)
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	q := u.Query()
	if !q.Has("apikey") || strings.HasPrefix(q.Get("apikey"), hidden) {
		return link
	}
	q.Del("apikey")
	u.RawQuery = q.Encode()
	return u.String()
}

// hideKeys swaps the key values in a link for their markers, keeping the
// markers it already holds.
func hideKeys(link string, keys []string) string {
	for _, sp := range spellings(keys) {
		if sp.value != "" {
			link = strings.ReplaceAll(link, sp.value, sp.marker)
		}
	}
	return link
}

// revealLink puts the key values back into a link cleanLink cleaned.
func revealLink(link string, keys []string) (string, error) {
	if !strings.Contains(link, hidden) {
		return link, nil
	}
	sp := spellings(keys)
	for i := len(sp) - 1; i >= 0; i-- {
		link = strings.ReplaceAll(link, sp[i].marker, sp[i].value)
	}
	if strings.Contains(link, hidden) {
		return "", errors.New("the Release link needs a Source key that has changed or gone; search again")
	}
	return link, nil
}

// GrabLink is the link to hand a Downloader for a Release: the stored link
// with the Source's key, read from the vault, put back.
func GrabLink(ctx context.Context, s *database.Store, sec SecretReader, rel domain.Release) (string, error) {
	if !strings.Contains(rel.LinkRef, hidden) {
		return rel.LinkRef, nil
	}
	var src domain.Source
	if err := database.Get(ctx, s.Read(), "sources", rel.SourceID, &src); err != nil {
		return "", fmt.Errorf("the Release's Source: %w", err)
	}
	cfg, err := ConfigFor(ctx, src, sec)
	if err != nil {
		return "", err
	}
	return revealLink(rel.LinkRef, secretValues(cfg))
}

func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

type found struct {
	src  domain.Source
	item Item
	keys []string // the Source's key values, kept out of stored links
	// relID is the stored Release when one is looked at again (a Held
	// Release during a feed check); it is updated, not stored twice.
	relID int64
}

// Handle searches every enabled Source for the Job's Slot, judges each
// Release with the Version's Pick rules, stores them with their verdicts and
// moves the Slot to chosen, held or back to where it was.
//
// Params: by_hand (a person searched this one Item: the protocol wait and,
// with a warning, the Release gate are skipped), choose (default true;
// false only judges and stores, for a person to choose from), skip_gate
// (a bulk search when the gate setting is off).
func (s *Searcher) Handle(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "slot" {
		return errors.New("search_slot needs a slot subject")
	}
	slotID := r.Job.Subject.ID
	by := jobActor(r)
	c, err := s.loadSlot(ctx, slotID)
	if err != nil {
		return err
	}
	if c.ti.Kind == "artist" {
		// A track is searched as its album.
		return s.queueAlbumFor(ctx, r, c)
	}
	handSearch, _ := r.Job.Params["by_hand"].(bool)
	skipGate, _ := r.Job.Params["skip_gate"].(bool)
	show := false
	if v, ok := r.Job.Params["choose"].(bool); ok && !v {
		show = true
	}
	from := c.sl.State
	switch {
	case show:
		// Showing only: any Slot may be looked at; it does not move.
	case from == domain.SlotNotOutYet && (handSearch || skipGate),
		from == domain.SlotWanted, from == domain.SlotBetterWanted, from == domain.SlotStalled, from == domain.SlotFailed, from == domain.SlotRejected:
		if err := store.MoveSlot(ctx, s.Store, slotID, domain.SlotSearching, "searching", by); err != nil {
			return err
		}
		s.publish("slot", map[string]any{"id": slotID, "state": domain.SlotSearching})
	case from == domain.SlotNeedsADecision && handSearch:
		// A person chose "search again" on Needs a decision (stage 4.1.2).
		if err := store.MoveSlot(ctx, s.Store, slotID, domain.SlotSearching, "searching again by hand", by); err != nil {
			return err
		}
		s.publish("slot", map[string]any{"id": slotID, "state": domain.SlotSearching})
	case from == domain.SlotSearching, from == domain.SlotHeld && handSearch:
	default:
		r.SetResult(map[string]any{"skipped": "slot is " + string(from)})
		return nil
	}

	if !show {
		// A search cut off (a stop or an error) puts the Slot back, so it
		// is not left searching; a stop has ended ctx, so this writes without it.
		defer func() {
			ctx := context.WithoutCancel(ctx)
			if s.slotState(ctx, slotID) != domain.SlotSearching {
				return
			}
			to := restState(c, from)
			if from == domain.SlotNeedsADecision {
				to = from
			}
			if store.MoveSlot(ctx, s.Store, slotID, to, "the search stopped before it finished", by) == nil {
				s.publish("slot", map[string]any{"id": slotID, "state": to})
			}
		}()
	}

	now := time.Now().UTC()
	ti := c.ti
	q := Query{Title: ti.Name, IMDb: ti.SourceIDs["imdb"]}
	if ti.Year != nil {
		q.Year = *ti.Year
	}
	q.TMDb, _ = strconv.Atoi(ti.SourceIDs["tmdb"])
	isShow := ti.Kind == "show"
	var shows showSet
	if isShow {
		var ep *epSlot
		if shows, ep, err = s.episodeSet(ctx, c, from != domain.SlotNotOutYet); err != nil {
			return err
		}
		q = showQuery(ti, ep.ref.Season, ep)
	}
	all, failures, answered, skipped, err := s.askAll(ctx, r, q, handSearch, isShow, ti.Tags)
	if err != nil {
		return err
	}
	jobID := r.Job.ID

	if reason, data, ok, failed := noAnswer(failures, answered, skipped); ok {
		if !show && from != domain.SlotHeld {
			_ = store.MoveSlot(ctx, s.Store, slotID, restState(c, from), reason, by)
			s.publish("slot", map[string]any{"id": slotID, "state": restState(c, from), "reason": reason})
		}
		s.addEvent(ctx, by, &jobID, ti.ID, slotID, EventSearchFailed, reason, data)
		if !failed {
			return nil
		}
		return errors.New(reason)
	}
	all = dedupe(all)
	out := from != domain.SlotNotOutYet
	var rels []judge.Release
	var vs []judge.Verdict
	if isShow {
		rels, vs, err = s.judgeShow(ctx, shows, all, handSearch, skipGate, s.showRules(ctx))
	} else {
		rels, vs, err = s.judgeAll(ctx, c, all, out, handSearch, skipGate)
	}
	if err != nil {
		return err
	}
	winner := -1
	if !show {
		winner = s.pickWinner(ctx, all, vs, now)
	}
	// A Release that fills more Slots (a multi-episode Release) is chosen
	// for all of them.
	pickSlots := []int64{slotID}
	if winner >= 0 && len(vs[winner].SlotIDs) > 1 {
		pickSlots = vs[winner].SlotIDs
	}
	foundBy := "auto_search"
	if handSearch {
		foundBy = "hand_search"
	}
	ids, err := s.storeAll(ctx, by, &jobID, foundBy, slotID, all, rels, vs, show)
	if err != nil {
		return err
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET last_searched_at = ? WHERE id = ?", database.FormatTime(now), c.it.ID)
		return err
	})

	refused := 0
	for _, v := range vs {
		if v.Outcome == "refused" {
			refused++
		}
	}
	result := map[string]any{"releases": len(all), "refused": refused, "sources_answered": answered}
	s.publish("releases", map[string]any{"slot_id": slotID, "count": len(all)})
	switch {
	case show:
		result["shown"] = true
		r.SetResult(result)
	case winner >= 0 && vs[winner].Outcome == "chosen":
		name := all[winner].item.Title
		if err := s.settleChosenMany(ctx, pickSlots, ids[winner], name, by); errors.Is(err, ErrBusy) {
			result["skipped"] = err.Error()
			r.SetResult(result)
			return nil
		} else if err != nil {
			return err
		}
		result["chosen_release_id"] = ids[winner]
		data := map[string]any{"release_id": ids[winner]}
		if !out {
			data["warning"] = "the Release gate was skipped"
		}
		s.addEvent(ctx, by, &jobID, ti.ID, slotID, EventReleaseChosen, fmt.Sprintf("Chose %s out of %d Releases", name, len(all)), data)
		s.publish("slot", map[string]any{"id": slotID, "state": domain.SlotChosen, "reason": "chose " + name})
		r.SetResult(result)
		if s.OnChosen != nil {
			s.OnChosen(ctx, slotID, ids[winner])
		}
	case winner >= 0 && vs[winner].Outcome == "held":
		name := all[winner].item.Title
		if err := s.settleHeldMany(ctx, pickSlots, ids[winner], name, *vs[winner].HoldUntil, by); errors.Is(err, ErrBusy) {
			result["skipped"] = err.Error()
			r.SetResult(result)
			return nil
		} else if err != nil {
			return err
		}
		result["held_release_id"] = ids[winner]
		s.addEvent(ctx, by, &jobID, ti.ID, slotID, EventReleaseHeld, "Holding "+name, map[string]any{"release_id": ids[winner]})
		s.publish("slot", map[string]any{"id": slotID, "state": domain.SlotHeld, "reason": "holding " + name})
		r.SetResult(result)
	default:
		reason := fmt.Sprintf("no Release passed the Pick rules (%d found, %d refused)", len(all), refused)
		if from != domain.SlotHeld {
			to := restState(c, from)
			if from == domain.SlotFailed && c.cur == nil {
				// Its download failed and nothing else is left: a person
				// decides (stage 4.0.3). With a copy in place it keeps
				// looking for a better one instead.
				to = domain.SlotNeedsADecision
				reason = "the download failed and no other Release passed the Pick rules (" + strconv.Itoa(len(all)) + " found, " + strconv.Itoa(refused) + " refused)"
			}
			if from == domain.SlotNeedsADecision {
				// "Search again" from Needs a decision found nothing: it
				// stays there for the person (stage 4.1.2).
				to = domain.SlotNeedsADecision
				reason = "searched again and no Release passed the Pick rules (" + strconv.Itoa(len(all)) + " found, " + strconv.Itoa(refused) + " refused)"
			}
			if err := store.MoveSlot(ctx, s.Store, slotID, to, reason, by); err != nil {
				return err
			}
			s.publish("slot", map[string]any{"id": slotID, "state": to, "reason": reason})
		}
		s.addEvent(ctx, by, &jobID, ti.ID, slotID, EventSearchDone, reason, nil)
		r.SetResult(result)
	}
	return nil
}

func jobActor(r *jobs.Run) domain.Actor { return domain.Actor{Kind: "job", ID: r.Job.ID} }

// turnedDown lists names and info hashes turned down for the Slot.
func (s *Searcher) turnedDown(ctx context.Context, slotID int64) ([]string, error) {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT name, COALESCE(info_hash, '') FROM releases
		WHERE state = 'turned_down' AND EXISTS (SELECT 1 FROM json_each(COALESCE(json_extract(verdict, '$.turned_down_slot_ids'), matched_slot_ids))
			WHERE value = ?) OR EXISTS (SELECT 1 FROM json_each(json_extract(verdict, '$.turned_down_slot_ids')) WHERE value = ?)`, slotID, slotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n, h string
		if err := rows.Scan(&n, &h); err != nil {
			return nil, err
		}
		out = append(out, n)
		if h != "" {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// pause puts a failing Source on a back-off pause (model.md section 2.9).
// Only the health columns change, read fresh in the same write, so a key or
// proxy changed meanwhile stays, and a Source the person turned off stays off.
func (s *Searcher) pause(ctx context.Context, src domain.Source, cause error) {
	kind, detail := KindDown, ""
	var wait time.Duration
	var se *Error
	if errors.As(cause, &se) {
		kind, wait, detail = se.Kind, se.RetryAfter, se.Detail
	}
	var n int
	var until time.Time
	paused := false
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var failures sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT json_extract(extra, '$.failures') FROM sources WHERE id = ? AND state != 'disabled'`, src.ID).Scan(&failures)
		if err != nil {
			return err // gone or turned off
		}
		n, _ = strconv.Atoi(failures.String)
		n++
		until = time.Now().UTC().Add(max(wait, Backoff(n)))
		now := time.Now().UTC()
		// The kind of block (#106) is kept beside the failure count, and cleared by any other failure.
		res, err := tx.ExecContext(ctx, `UPDATE sources SET state = 'paused', pause_reason = ?, paused_until = ?,
			extra = CASE WHEN ? = '' THEN json_remove(json_set(extra, '$.failures', ?), '$.block') ELSE json_set(extra, '$.failures', ?, '$.block', ?) END,
			updated_at = ?, updated_by = ? WHERE id = ? AND state != 'disabled'`,
			kind, database.FormatTime(until), detail, strconv.Itoa(n), strconv.Itoa(n), detail, database.FormatTime(now), systemActor(), src.ID)
		if err != nil {
			return err
		}
		k, _ := res.RowsAffected()
		paused = k == 1
		return nil
	})
	if !paused {
		return
	}
	why := kind
	if detail != "" {
		why = detail
	}
	e := &domain.Event{Kind: "source_paused", Summary: fmt.Sprintf("Paused Source %s until %s (%s)", src.Name, until.Format(time.RFC3339), why),
		Data: map[string]any{"source_id": src.ID, "reason": kind, "failures": n}}
	if detail != "" {
		e.Data["block"] = detail
	}
	if s.Store.AddEvent(ctx, e) == nil {
		s.publish("event", e)
	}
}

// recovered moves a paused Source back to enabled after it answered and
// clears its failure count. A Source the person turned off stays off.
func (s *Searcher) recovered(ctx context.Context, src domain.Source) {
	var need bool
	if err := s.Store.Read().QueryRowContext(ctx, `SELECT state = 'paused' OR json_extract(extra, '$.failures') IS NOT NULL OR json_extract(extra, '$.block') IS NOT NULL FROM sources WHERE id = ?`,
		src.ID).Scan(&need); err != nil || !need {
		return // read fresh: the copy may be older than a pause from another search
	}
	now := database.FormatTime(time.Now().UTC())
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE sources SET state = 'enabled', pause_reason = NULL, paused_until = NULL,
			updated_at = ?, updated_by = ? WHERE id = ? AND state = 'paused'`, now, systemActor(), src.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_remove(extra, '$.failures', '$.block'), updated_at = ?, updated_by = ?
			WHERE id = ? AND (json_extract(extra, '$.failures') IS NOT NULL OR json_extract(extra, '$.block') IS NOT NULL)`, now, systemActor(), src.ID)
		return err
	})
}

func systemActor() string {
	b, _ := json.Marshal(domain.System)
	return string(b)
}

// SweepSearching puts Slots left searching longer than olderThan back to
// looking (a search cut off by a crash). It runs at start, before any Job.
func (s *Searcher) SweepSearching(ctx context.Context, olderThan time.Duration) (int, error) {
	slots, err := database.Select[domain.Slot](ctx, s.Store.Read(), "slots", "state = 'searching' AND updated_at < ?",
		database.FormatTime(time.Now().Add(-olderThan)))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sl := range slots {
		c, err := s.loadSlot(ctx, sl.ID)
		if err != nil {
			continue
		}
		to := restState(c, domain.SlotSearching)
		if store.MoveSlot(ctx, s.Store, sl.ID, to, "a search was cut off; looking again", domain.System) == nil {
			n++
		}
	}
	return n, nil
}
