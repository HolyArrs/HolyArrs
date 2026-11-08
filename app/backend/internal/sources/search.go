package sources

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
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Searcher runs the search jobs.
type Searcher struct {
	Store *database.Store
	Live  Publisher
	// OnChosen is told when a release is chosen for a Slot, to send it to a Downloader.
	OnChosen func(ctx context.Context, slotID, releaseID int64)
	// Secrets opens a Source's secret_ref, its API key. Nil means no keys.
	Secrets SecretReader
	// Queue takes the searches a missing search queues.
	Queue *jobs.Queue

	mu         sync.Mutex
	clients    map[int64]*Client
	onAsk      func(name string)
	quietMu    sync.Mutex
	quietCalls map[int64]*quietSourceCalls
}

func (s *Searcher) publish(topic string, data any) {
	if s.Live != nil {
		s.Live.Publish(topic, data)
	}
}

// client keeps one Client per Source so its limits hold across jobs. A Source
// whose settings, key or proxy changed gets a new Client.
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

// hidden marks where a Source key was taken out of a stored link. The number
// says which key value and the letter which spelling: r as it is, q
// query-escaped, p path-escaped.
const hidden = "HOLYARRSKEY"

// secretValues lists the key values a result link may carry: a Torznab or
// Newznab apikey, or the values in an rss Source's key, then the passkeys its
// URL path holds.
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
	// Path passkeys come after the others, so stored markers keep their number.
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

// cleanLink takes the Source's key values out of a result link, in the query
// or the path, so a stored release never holds the secret. Another apikey
// parameter is dropped.
func cleanLink(link string, keys []string) string {
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

// hideKeys swaps the key values in a link for their markers and keeps the
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

// GrabLink is the link to hand a Downloader for a release: the stored link
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
	src   domain.Source
	item  Item
	keys  []string
	relID int64
}

// Handle searches every enabled Source for the job's Slot, judges each
// release with the Version's pick rules, stores them with their verdicts and
// moves the Slot to chosen, held or back to where it was.
//
// Params: by_hand (a person asked: the protocol wait is skipped, and the
// release gate with a warning), choose (default true; false only judges and
// stores, for a person to choose from), skip_gate (a bulk search while the
// gate setting is off).
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
	// An artist is searched through its albums.
	if c.ti.Kind == "artist" {
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
	case from == mediafiles.SlotNotOutYet && (handSearch || skipGate),
		from == mediafiles.SlotWanted, from == mediafiles.SlotBetterWanted, from == mediafiles.SlotStalled, from == mediafiles.SlotFailed, from == mediafiles.SlotRejected:
		if err := mediafiles.MoveSlot(ctx, s.Store, slotID, mediafiles.SlotSearching, "searching", by); err != nil {
			return err
		}
		s.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotSearching})
	// A person chose to search again from a decision they had to make.
	case from == mediafiles.SlotNeedsADecision && handSearch:
		if err := mediafiles.MoveSlot(ctx, s.Store, slotID, mediafiles.SlotSearching, "searching again by hand", by); err != nil {
			return err
		}
		s.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotSearching})
	case from == mediafiles.SlotSearching, from == mediafiles.SlotHeld && handSearch:
	default:
		r.SetResult(map[string]any{"skipped": "slot is " + string(from)})
		return nil
	}

	if !show {
		defer func() {
			ctx := context.WithoutCancel(ctx)
			if s.slotState(ctx, slotID) != mediafiles.SlotSearching {
				return
			}
			to := restState(c, from)
			if from == mediafiles.SlotNeedsADecision {
				to = from
			}
			if mediafiles.MoveSlot(ctx, s.Store, slotID, to, "the search stopped before it finished", by) == nil {
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
		if shows, ep, err = s.episodeSet(ctx, c, from != mediafiles.SlotNotOutYet); err != nil {
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
		if !show && from != mediafiles.SlotHeld {
			_ = mediafiles.MoveSlot(ctx, s.Store, slotID, restState(c, from), reason, by)
			s.publish("slot", map[string]any{"id": slotID, "state": restState(c, from), "reason": reason})
		}
		s.addEvent(ctx, by, &jobID, ti.ID, slotID, EventSearchFailed, reason, data)
		if !failed {
			return nil
		}
		return errors.New(reason)
	}
	all = dedupe(all)
	out := from != mediafiles.SlotNotOutYet
	var rels []mediafiles.ReleaseFacts
	var vs []mediafiles.Verdict
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
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error { return mediafiles.TouchItemSearchedTx(tx, c.it.ID, now) })

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
		s.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotChosen, "reason": "chose " + name})
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
		s.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotHeld, "reason": "holding " + name})
		r.SetResult(result)
	default:
		reason := fmt.Sprintf("no Release passed the Pick rules (%d found, %d refused)", len(all), refused)
		if from != mediafiles.SlotHeld {
			to := restState(c, from)
			// A failed download with no other release and no copy in place goes to a person. With a copy it keeps looking for a better one.
			if from == mediafiles.SlotFailed && c.cur == nil {
				to = mediafiles.SlotNeedsADecision
				reason = "the download failed and no other Release passed the Pick rules (" + strconv.Itoa(len(all)) + " found, " + strconv.Itoa(refused) + " refused)"
			}
			if from == mediafiles.SlotNeedsADecision {
				to = mediafiles.SlotNeedsADecision
				reason = "searched again and no Release passed the Pick rules (" + strconv.Itoa(len(all)) + " found, " + strconv.Itoa(refused) + " refused)"
			}
			if err := mediafiles.MoveSlot(ctx, s.Store, slotID, to, reason, by); err != nil {
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
	return turnedDownNames(ctx, s.Store.Read(), slotID)
}

// pause puts a failing Source on a back-off pause. Only the health columns
// change, read fresh in the same write, so a key or proxy edited meanwhile
// stays and a Source the person turned off stays off.
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
		var err error
		n, until, paused, err = pauseSourceTx(tx, ctx, src.ID, kind, detail, wait)
		return err
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
	if need, err := sourceNeedsRecovery(ctx, s.Store.Read(), src.ID); err != nil || !need {
		return
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error { return recoverSourceTx(tx, ctx, src.ID) })
}

// SweepSearching puts Slots left searching longer than olderThan back to
// looking, after a crash cut a search off. It runs at start, before any job.
func (s *Searcher) SweepSearching(ctx context.Context, olderThan time.Duration) (int, error) {
	slots, err := mediafiles.SlotsSearchingBefore(ctx, s.Store.Read(), time.Now().Add(-olderThan))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sl := range slots {
		c, err := s.loadSlot(ctx, sl.ID)
		if err != nil {
			continue
		}
		to := restState(c, mediafiles.SlotSearching)
		if mediafiles.MoveSlot(ctx, s.Store, sl.ID, to, "a search was cut off; looking again", domain.System) == nil {
			n++
		}
	}
	return n, nil
}
