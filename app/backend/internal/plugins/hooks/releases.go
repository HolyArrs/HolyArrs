package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// FeedCall is one request HolyArrs makes of a search-source plugin: a
// Torznab or Newznab call (path /api) or a release file (any other path),
// at the plugin's own address plugin://<id>.
type FeedCall struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
}

// FeedAnswer is the plugin's reply: an HTTP-like status, type and body.
type FeedAnswer struct {
	Status int    `json:"status"`
	Type   string `json:"type,omitempty"`
	Body   []byte `json:"body,omitempty"`
}

const feedMax = 16 << 20

// ReleaseTransport carries a plugin search source's calls through the
// Broker. It only ever reaches that plugin: a result's link to any other
// address is refused, so a plugin can't make HolyArrs fetch an inside URL.
type ReleaseTransport struct {
	Broker *Broker
	Plugin string
	Wait   time.Duration
}

// RoundTrip sends one call to the plugin.
func (t *ReleaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "plugin" || req.URL.Host != t.Plugin {
		return nil, errors.New("a plugin source's links can only lead back to that plugin")
	}
	w := t.Wait
	if w <= 0 {
		w = 60 * time.Second
	}
	raw, err := t.Broker.Call(req.Context(), t.Plugin, Releases, "fetch",
		FeedCall{Method: req.Method, Path: req.URL.Path, Query: req.URL.RawQuery}, w)
	if err != nil {
		return nil, err
	}
	var a FeedAnswer
	if err := json.Unmarshal(raw, &a); err != nil || a.Status < 100 || a.Status > 599 {
		return nil, errors.New("the plugin's answer could not be read")
	}
	if len(a.Body) > feedMax {
		return nil, errors.New("the plugin's answer is too large")
	}
	h := http.Header{}
	if a.Type != "" {
		h.Set("Content-Type", clip(a.Type, 100))
	}
	h.Set("Content-Length", strconv.Itoa(len(a.Body)))
	return &http.Response{StatusCode: a.Status, Status: strconv.Itoa(a.Status) + " " + http.StatusText(a.Status),
		Header: h, Body: io.NopCloser(bytes.NewReader(a.Body)), ContentLength: int64(len(a.Body)),
		Request: req, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}, nil
}

// PluginOf is the plugin id of a plugin source address, or "".
func PluginOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "plugin" {
		return ""
	}
	return u.Host
}
