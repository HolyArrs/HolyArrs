package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// CheckTyped tests the values typed in a saved Downloader's Change form
// without saving them, as the old apps test a form. A key or password left
// masked or empty uses the saved one, but never on a new host. Nothing is
// recorded: the saved Downloader and its health stay as they were.
func (m *Manager) CheckTyped(ctx context.Context, saved domain.Downloader, in Input) (domain.Downloader, Health, error) {
	if in.Secret == secrets.Masked {
		in.Secret = ""
	}
	d := saved
	if err := fill(in, &d, false); err != nil {
		return d, Health{}, typedError{err}
	}
	useSaved := in.Secret == "" && !in.ClearSecret
	if useSaved && d.SecretRef != nil && !sameTarget(d, saved) {
		return d, Health{}, typedError{ErrTypeKey}
	}
	var e Engine
	var err error
	switch {
	case m.Engine != nil:
		e, err = m.Engine(d)
	case useSaved:
		e, err = m.EngineFor(ctx, d)
	default:
		e, err = m.engineWith(d, in.Secret)
	}
	if err != nil {
		return d, Health{}, err
	}
	h, err := e.Health(ctx)
	if err != nil {
		return d, h, err
	}
	// SABnzbd only, as in Check, but a missing category is only named here.
	if s, ok := rawEngine(e).(*Sabnzbd); ok {
		for _, c := range d.Categories() {
			if c == "" || c == "*" {
				continue
			}
			if has, cerr := s.HasCategory(ctx, c); cerr == nil && !has {
				return d, h, fmt.Errorf("%w (%s)", ErrNoCategory, c)
			}
		}
	}
	return d, h, nil
}

// testTyped answers a Test sent with the Change form's values.
func (a *Web) testTyped(w http.ResponseWriter, r *http.Request, saved domain.Downloader, raw []byte) {
	var in Input
	if json.Unmarshal(raw, &in) != nil {
		webserver.Fail(w, http.StatusBadRequest, downloaderHelp)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	start := time.Now()
	d, h, err := a.Manager().CheckTyped(ctx, saved, in)
	var bad typedError
	if errors.As(err, &bad) {
		downloaderError(w, bad.error)
		return
	}
	out := map[string]any{"ok": err == nil, "ms": time.Since(start).Milliseconds(), "health": HealthOf(saved, time.Now())}
	if err != nil {
		cause, text := explainDownloader(err, d.URL, d.AllowLocal)
		out["text"], out["cause"] = text, cause
		missingCategory(out, err, d)
	} else {
		out["text"] = "It answered: " + d.Adapter + " " + h.Version + "."
		out["version"] = h.Version
		if h.Note != "" {
			out["text"] = out["text"].(string) + " " + h.Note
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// ErrTypeKey refuses to send the saved key or password anywhere but where it
// was saved for.
var ErrTypeKey = errors.New("the address or user name differs from the saved one; type the key or password again to test it")

// sameTarget says whether d goes to the same scheme, host, port, path and
// user name as saved, so the saved key may be sent with it.
func sameTarget(d, saved domain.Downloader) bool {
	a, errA := url.Parse(strings.TrimSpace(d.URL))
	b, errB := url.Parse(strings.TrimSpace(saved.URL))
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && a.Port() == b.Port() &&
		strings.TrimRight(a.Path, "/") == strings.TrimRight(b.Path, "/") && d.Username == saved.Username
}

// typedError is a fault in the typed values themselves, answered like a
// refused save.
type typedError struct{ error }

func (e typedError) Unwrap() error { return e.error }

// readTestBody is the Test request's body, empty when the form sent none.
func readTestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		webserver.Fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		return nil, false
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil, true
	}
	return raw, true
}
