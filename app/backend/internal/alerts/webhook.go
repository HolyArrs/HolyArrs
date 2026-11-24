package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Webhook headers. The signature is "sha256=" and the hex HMAC-SHA256 of
// the timestamp, a dot and the body, keyed with the Alert's secret.
const (
	HeaderEvent     = "X-HolyArrs-Event"
	HeaderDelivery  = "X-HolyArrs-Delivery"
	HeaderTimestamp = "X-HolyArrs-Timestamp"
	HeaderSignature = "X-HolyArrs-Signature"
)

// Payload is the JSON body a webhook Alert posts.
type Payload struct {
	Moment  string    `json:"moment"`
	Kind    string    `json:"event_kind,omitempty"` // the Event's kind, e.g. import_done
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
	Test    bool      `json:"test,omitempty"`
	Movie   *Movie    `json:"movie,omitempty"`
	// Show and Episodes are set for a moment about a show.
	Show     *Show     `json:"show,omitempty"`
	Episodes []Episode `json:"episodes,omitempty"`
	// Artist and Tracks are set for a moment about an artist.
	Artist *Artist        `json:"artist,omitempty"`
	Tracks []Track        `json:"tracks,omitempty"`
	Data   map[string]any `json:"data,omitempty"` // reasons and paths
	// People are the names of the people who asked (no addresses).
	People []string `json:"people,omitempty"`
	// Report is set for a moment about a problem report.
	Report *Report `json:"report,omitempty"`
}

// Sign returns the signature header value for body sent at ts with the
// delivery id: HMAC-SHA256 of "<ts>.<delivery>.<body>". Receivers must
// refuse old timestamps and delivery ids they have already seen.
func Sign(secret string, ts int64, delivery string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.FormatInt(ts, 10) + "." + delivery + "."))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// CheckWebhookURL accepts an http or https link with a host and no user
// name. Without allowLocal a link to this machine or a private address is
// refused here; names are checked again after DNS when sending.
func CheckWebhookURL(raw string, allowLocal bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ErrHook
	}
	if allowLocal {
		return nil
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return ErrHook
	}
	if ip := net.ParseIP(h); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsLinkLocalMulticast()) {
		return ErrHook
	}
	return nil
}

var reFullPath = regexp.MustCompile(`^(/|\\|[A-Za-z]:[\\/])`)

// payloadData copies an Event's data for a webhook with every full path of
// this machine (a download folder, a Library folder, a source file) cut to
// its last name; paths inside a Library folder are relative and stay.
func payloadData(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = payloadData(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = payloadData(e)
		}
		return out
	case []string:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = payloadData(e)
		}
		return out
	case string:
		if reFullPath.MatchString(x) {
			t := strings.TrimRight(x, `/\`)
			if i := strings.LastIndexAny(t, `/\`); i >= 0 {
				return t[i+1:]
			}
			return t
		}
	}
	return v
}

func (s *Sender) webhook(ctx context.Context, a domain.Alert, m Message) error {
	link, err := s.secret(ctx, a.URLSecretID)
	if err != nil {
		return err
	}
	if CheckWebhookURL(link, a.AllowLocal) != nil {
		return ErrHook
	}
	secret, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	p := payloadOf(m)
	for _, x := range m.People {
		p.People = append(p.People, x.Name)
	}
	body, _ := json.Marshal(p)
	return s.postWebhook(ctx, a, m, link, secret, body)
}

// PluginPayload is what plugins are told: the webhook payload for m
// without the people who asked or a problem report, with every link in the
// text and data replaced by "[link]" (an error may quote an indexer link
// with its key) and every full path of this machine cut to its last name.
// A plugin is someone else's program; a webhook goes where the owner chose.
func PluginPayload(m Message) Payload {
	p := payloadOf(m)
	p.Report, p.People = nil, nil
	p.Subject, p.Text = plainText(p.Subject), plainText(p.Text)
	if p.Data != nil {
		p.Data = noPeople(noLinks(p.Data)).(map[string]any)
	}
	return p
}

// peopleKeys are Event data keys that name people or hold what they wrote;
// a plugin never gets them, even if a future Event carries one.
var peopleKeys = []string{"asked_by", "asked_by_ids", "made_by", "decided_by", "by", "user", "user_id", "people", "note", "reply", "email"}

// reSecretWord matches a word quoting a key or a login: apikey=, token=,
// passkey=, password=, secret=, auth=, or user:pass@host.
var reSecretWord = regexp.MustCompile(`(?i)(key|token|pass\w*|secret|auth\w*)=|[^\s:/@]+:[^\s/@]+@`)

// noPeople is v without peopleKeys at any depth.
func noPeople(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range peopleKeys {
			delete(x, k)
		}
		for k, e := range x {
			x[k] = noPeople(e)
		}
	case []any:
		for i, e := range x {
			x[i] = noPeople(e)
		}
	}
	return v
}

// plainText replaces each word holding a link with "[link]" and cuts each
// full path to its last name, keeping the lines.
func plainText(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		words := strings.Split(l, " ")
		for j, w := range words {
			if strings.Contains(w, "://") || reSecretWord.MatchString(w) {
				words[j] = "[link]"
			} else if reFullPath.MatchString(w) {
				words[j] = payloadData(w).(string)
			}
		}
		lines[i] = strings.Join(words, " ")
	}
	return strings.Join(lines, "\n")
}

// noLinks is v with every string holding a link replaced by plainText's.
func noLinks(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = noLinks(e)
		}
	case []any:
		for i, e := range x {
			x[i] = noLinks(e)
		}
	case string:
		if strings.Contains(x, "://") || reSecretWord.MatchString(x) {
			return plainText(x)
		}
	}
	return v
}

// payloadOf is the webhook payload for m, without the people who asked.
func payloadOf(m Message) Payload {
	p := Payload{Moment: m.Moment, Subject: m.Subject, Text: m.Text, At: time.Now().UTC(), Test: m.Moment == MomentTest, Movie: m.Movie, Show: m.Show, Episodes: m.Episodes,
		Artist: m.Artist, Tracks: m.Tracks, Report: m.Report}
	if m.Event != nil {
		p.Kind, p.At = m.Event.Kind, m.Event.At
		// The Event holds Go values (lists of import steps and results); as
		// plain JSON maps and lists, payloadData reaches every path in them.
		var plain map[string]any
		if raw, err := json.Marshal(m.Event.Data); err == nil && json.Unmarshal(raw, &plain) == nil {
			if d, ok := payloadData(plain).(map[string]any); ok {
				p.Data = d
			}
		}
	}
	return p
}

func (s *Sender) postWebhook(ctx context.Context, a domain.Alert, m Message, link, secret string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, bytes.NewReader(body))
	if err != nil {
		return ErrHook
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HolyArrs")
	req.Header.Set(HeaderEvent, m.Moment)
	delivery := hex.EncodeToString(id)
	req.Header.Set(HeaderDelivery, delivery)
	if secret != "" {
		switch a.WebhookAuth {
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+secret)
		case "none":
		default:
			ts := time.Now().Unix()
			req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
			req.Header.Set(HeaderSignature, Sign(secret, ts, delivery, body))
		}
	}
	resp, err := client(a).Do(req)
	if err != nil {
		// The link may hold a token: never keep it in the error.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrMetadataRefused) {
			return fail("the address is a cloud metadata address, which HolyArrs never connects to", err)
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return fail("webhook link points to the local network, which this Alert may not reach", err)
		}
		return again("the webhook server did not answer", err)
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return again("the webhook server's answer could not be read", readErr)
	}
	if resp.StatusCode/100 != 2 {
		return statusFailure("the webhook server", resp.StatusCode)
	}
	return nil
}
