package webserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Confirms holds the confirmation ids of the two-step controls.
type Confirms struct {
	mu  sync.Mutex
	ids map[string]confirmFor
}

type confirmFor struct {
	what    string // method and path
	user    int64
	expires time.Time
}

// MaxConfirmsPerUser caps the open confirmation ids of one person.
const MaxConfirmsPerUser = 100

// ConfirmFor is how long a confirmation id stays good.
const ConfirmFor = 60 * time.Second

// ConfirmNow is the clock of the confirmation ids (tests move it).
var ConfirmNow = time.Now

// Confirmed runs the two-step confirm: without a confirm id in the body
// it answers 202 with a new id (valid ConfirmFor) and returns false; with
// a good id for the same person and request it returns true. what says in
// plain words what the second request will do.
func (c *Confirms) Confirmed(w http.ResponseWriter, r *http.Request, user int64, what string) bool {
	var body struct {
		Confirm string `json:"confirm"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<10))
	if err != nil || (len(strings.TrimSpace(string(raw))) > 0 && json.Unmarshal(raw, &body) != nil) {
		Fail(w, http.StatusBadRequest, `send {} to ask, then {"confirm": "<id>"} within 60 seconds`)
		return false
	}
	key := r.Method + " " + r.URL.Path
	now := ConfirmNow()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = map[string]confirmFor{}
	}
	for id, cf := range c.ids {
		if now.After(cf.expires) {
			delete(c.ids, id)
		}
	}
	if body.Confirm == "" {
		// At most MaxConfirmsPerUser open ids per person: the oldest goes.
		var oldest string
		n := 0
		for id, cf := range c.ids {
			if cf.user != user {
				continue
			}
			n++
			if oldest == "" || cf.expires.Before(c.ids[oldest].expires) {
				oldest = id
			}
		}
		if n >= MaxConfirmsPerUser {
			delete(c.ids, oldest)
		}
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		c.ids[id] = confirmFor{what: key, user: user, expires: now.Add(ConfirmFor)}
		WriteJSON(w, http.StatusAccepted, map[string]any{"confirm": id, "expires_in_s": int(ConfirmFor.Seconds()),
			"text": "To " + what + ", send the same request again within 60 seconds with {\"confirm\": \"" + id + "\"}."})
		return false
	}
	cf, ok := c.ids[body.Confirm]
	if !ok || cf.what != key || cf.user != user {
		Fail(w, http.StatusConflict, "that confirmation id is unknown, used, expired or for another control; ask again")
		return false
	}
	delete(c.ids, body.Confirm)
	return true
}

// Len is the number of confirmation ids held now.
func (c *Confirms) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ids)
}
