package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// findGap is the least time between two network searches.
const findGap = 2 * time.Second

// findWaitMax is how long a search waits for the one before it.
const findWaitMax = 20 * time.Second

// findWaiters is how many searches may wait their turn at once.
const findWaiters = 4

// find searches this server's network for old apps (owner only, on a
// click). One search runs at a time, at most one per findGap.
func (h *Handlers) find(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	// A search asked for while another runs (or just ran) waits its turn
	// instead of failing: setup starts one as a screen opens.
	// Only a few may wait at once; more are refused, so held requests
	// cannot pile up.
	if h.findWaiting.Add(1) > findWaiters {
		h.findWaiting.Add(-1)
		webserver.Fail(w, http.StatusTooManyRequests, "too many tries; wait a little and try again")
		return
	}
	defer h.findWaiting.Add(-1)
	for waited := time.Duration(0); ; waited += 250 * time.Millisecond {
		h.findMu.Lock()
		if !h.finding && time.Since(h.findAt) >= findGap {
			h.finding = true
			h.findMu.Unlock()
			break
		}
		h.findMu.Unlock()
		if waited >= findWaitMax {
			webserver.Fail(w, http.StatusTooManyRequests, "too many tries; wait a little and try again")
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	defer func() {
		h.findMu.Lock()
		h.finding, h.findAt = false, time.Now()
		h.findMu.Unlock()
	}()
	hints, only := h.findHints(r)
	found, searched := Find(r.Context(), hints, only...)
	if found == nil {
		found = []Found{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"found": found, "searched": searched})
}

// findHints are the addresses whose /24 a search covers first: the host the
// browser used to reach HolyArrs (sent in the body, then the Host header),
// the addresses typed in this setup, then the apps and Downloaders already
// added. In Docker this server's own /24 is the bridge, not the home network.
func (h *Handlers) findHints(r *http.Request) ([]string, []Kind) {
	var in struct {
		Host      string   `json:"host"`
		Addresses []string `json:"addresses"`
		Kinds     []string `json:"kinds"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in)
	hints := []string{in.Host, r.Host}
	if len(in.Addresses) > 20 {
		in.Addresses = in.Addresses[:20]
	}
	hints = append(hints, in.Addresses...)
	if h.Store != nil {
		hints = append(hints, knownAddresses(r.Context(), h.Store.Read())...)
	}
	// Only kinds this search knows; anything else is ignored (an empty list searches every app).
	var only []Kind
	for _, k := range in.Kinds {
		if _, ok := dockerNames[Kind(k)]; ok {
			only = append(only, Kind(k))
		}
	}
	return hints, only
}

// linked lists the apps an Overseerr, Seerr or Prowlarr is set up with
// (never their keys). With add, it reads each named one with the key the
// old app holds and keeps that key in the vault, as a typed read does.
func (h *Handlers) linked(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in MigrateSourceInput
	src, _, ok := h.sourceFrom(w, r, &in)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	apps, err := ReadLinked(ctx, src)
	if err != nil {
		fail(w, err)
		return
	}
	if apps == nil {
		apps = []Linked{}
	}
	if len(in.Add) == 0 {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"apps": apps})
		return
	}
	want := map[string]bool{}
	for _, a := range in.Add {
		want[InstanceAddress(a)] = true
	}
	type added struct {
		Kind     Kind     `json:"kind"`
		Snapshot Saved    `json:"snapshot"`
		Reentry  []string `json:"needs_reentry,omitempty"`
		Notes    []string `json:"notes,omitempty"`
		Replaced []int64  `json:"replaced,omitempty"`
	}
	type failed struct {
		Kind    Kind   `json:"kind"`
		Address string `json:"address"`
		Error   string `json:"error"`
	}
	read, bad := []added{}, []failed{}
	for _, l := range apps {
		if !want[l.Address] || !l.HasKey {
			continue
		}
		child := l.source(src)
		snap, err := Read(ctx, child)
		if err != nil {
			bad = append(bad, failed{Kind: l.Kind, Address: l.Address, Error: err.Error()})
			continue
		}
		snap.Address = InstanceAddress(child.URL)
		older := h.sameAppReads(r.Context(), child, snap.Address)
		var saved Saved
		err = h.Store.Write(r.Context(), func(tx *sql.Tx) error {
			var err error
			saved, err = SaveTx(r.Context(), tx, actor(r).ID, snap)
			return err
		})
		if err != nil {
			bad = append(bad, failed{Kind: l.Kind, Address: l.Address, Error: ErrTooBig.Error()})
			continue
		}
		if h.Secrets != nil {
			h.keepMoveKey(r, child, secretsfile.MoveCredentialVersion{})
		}
		read = append(read, added{Kind: l.Kind, Snapshot: saved, Reentry: snap.NeedsReentry, Notes: snap.Notes, Replaced: h.dropSameApp(r.Context(), older)})
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"apps": apps, "read": read, "failed": bad})
}
