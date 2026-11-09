package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Errors from checking an ntfy, Gotify or Pushover Alert.
var (
	ErrServer      = errors.New("paste the server address (http:// or https:// with a host, no ? or #); a server on the local network needs May reach the local network")
	ErrNtfyTopic   = errors.New("the ntfy topic is 1 to 64 letters, digits, - or _")
	ErrNtfyToken   = errors.New("the ntfy access token looks wrong (it starts with tk_); leave it empty if the topic needs none")
	ErrGotifyToken = errors.New("paste the Gotify app token (in Gotify: Apps, then the app's token)")
	ErrPushToken   = errors.New("paste the Pushover app token (on pushover.net: your app's API Token)")
	ErrPushUser    = errors.New("paste your Pushover user key (on pushover.net, at the top of your dashboard)")
	// ErrTokenAgain keeps an app token from going to another server:
	// changing the server address needs the token typed again.
	ErrTokenAgain = errors.New("type the token again when the server address points to another server")
)

// ntfyDefault is the public ntfy server, used when none is given.
const ntfyDefault = "https://ntfy.sh"

// What each app takes in one message.
const (
	ntfyMax      = 4096
	gotifyMax    = 4096
	pushMax      = 1024
	pushTitleMax = 250
	pushURLMax   = 512
)

var (
	ntfyTopicRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// Tokens go into a header or a form, so only plain letters reach them.
	appTokenRe = regexp.MustCompile(`^[A-Za-z0-9._-]{5,256}$`)
	pushKeyRe  = regexp.MustCompile(`^[A-Za-z0-9]{20,50}$`)
)

// CheckServerURL accepts an ntfy or Gotify server: http or https with a
// host, maybe a path, but no user name, query or fragment. Without
// allowLocal a server on this machine or a private address is refused here;
// names are checked again after DNS when sending.
func CheckServerURL(raw string, allowLocal bool) error {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "?#") || CheckWebhookURL(raw, allowLocal) != nil {
		return ErrServer
	}
	return nil
}

// CheckNtfyTopic accepts an ntfy topic name; it goes into the request path.
func CheckNtfyTopic(t string) error {
	if !ntfyTopicRe.MatchString(strings.TrimSpace(t)) {
		return ErrNtfyTopic
	}
	return nil
}

func checkAppToken(t string, err error) error {
	if !appTokenRe.MatchString(strings.TrimSpace(t)) {
		return err
	}
	return nil
}

func checkPushKey(k string, err error) error {
	if !pushKeyRe.MatchString(strings.TrimSpace(k)) {
		return err
	}
	return nil
}

// pushoverBase is where the Pushover API answers; HOLYARRS_PUSHOVER_URL
// points it at a stand-in for tests.
func pushoverBase() string {
	if b := strings.TrimRight(strings.TrimSpace(os.Getenv("HOLYARRS_PUSHOVER_URL")), "/"); b != "" {
		return b
	}
	return "https://api.pushover.net"
}

// firstLink is the first http or https link in s, or "".
func firstLink(s string) string {
	for _, w := range strings.Fields(s) {
		if strings.HasPrefix(w, "https://") || strings.HasPrefix(w, "http://") {
			if u, err := url.Parse(w); err == nil && u.Host != "" {
				return w
			}
		}
	}
	return ""
}

// postApp sends req for an Alert of the app who and reads at most 64 KiB of
// the answer. The address never goes into the error: it may hold a topic.
func postApp(a domain.Alert, who string, req *http.Request) (int, []byte, error) {
	resp, err := client(a).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return 0, nil, fail("the "+who+" server is on the local network, which this Alert may not reach", err)
		}
		return 0, nil, again(who+" did not answer", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return 0, nil, again(who+"'s answer could not be read", readErr)
	}
	return resp.StatusCode, raw, nil
}

// scrub keeps the app's own reason for the log, without any of the secrets.
func scrub(reason string, secrets ...string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	for _, s := range secrets {
		if s != "" {
			reason = strings.ReplaceAll(reason, s, "[secret]")
		}
	}
	return errors.New(cutRunes(reason, 300))
}

// ntfy posts the message to the Alert's topic: the text as the body, the
// subject as the Title header and the first link as Click.
func (s *Sender) ntfy(ctx context.Context, a domain.Alert, m Message) error {
	token, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	server := strings.TrimRight(strings.TrimSpace(a.ServerURL), "/")
	if CheckServerURL(server, a.AllowLocal) != nil {
		return ErrServer
	}
	if CheckNtfyTopic(a.Topic) != nil {
		return ErrNtfyTopic
	}
	if token != "" && checkAppToken(token, ErrNtfyToken) != nil {
		return fail("the ntfy access token is broken; type it again", nil)
	}
	text := cutRunes(strings.TrimSpace(m.Text), ntfyMax)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/"+strings.TrimSpace(a.Topic), strings.NewReader(text))
	if err != nil {
		return fail("the ntfy address does not look right", nil)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	// A title with letters outside ASCII goes as RFC 2047 words, which ntfy
	// reads; line breaks would end the header.
	req.Header.Set("Title", mime.BEncoding.Encode("utf-8", cutRunes(clean(m.Subject), subjectMax)))
	if link := firstLink(m.Text); link != "" {
		req.Header.Set("Click", link)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	code, raw, err := postApp(a, "ntfy", req)
	if err != nil {
		return err
	}
	if code/100 == 2 {
		return nil
	}
	var ans struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &ans)
	detail := scrub(ans.Error, token)
	switch {
	case code == http.StatusTooManyRequests:
		return again("ntfy asked HolyArrs to slow down", detail)
	case code >= 500:
		return again(fmt.Sprintf("ntfy answered %d", code), detail)
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return fail("ntfy refused the access token", detail)
	case code == http.StatusNotFound:
		return fail("ntfy answered 404 (check the server address)", detail)
	}
	return statusFailure("ntfy", code)
}

// gotify posts the message to the Gotify server as the Alert's app.
func (s *Sender) gotify(ctx context.Context, a domain.Alert, m Message) error {
	token, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if checkAppToken(token, ErrGotifyToken) != nil {
		return fail("the Gotify app token is missing; type it again", nil)
	}
	server := strings.TrimRight(strings.TrimSpace(a.ServerURL), "/")
	if CheckServerURL(server, a.AllowLocal) != nil {
		return ErrServer
	}
	body, _ := json.Marshal(map[string]any{"title": cutRunes(clean(m.Subject), subjectMax), "message": cutRunes(strings.TrimSpace(m.Text), gotifyMax), "priority": 5})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/message", bytes.NewReader(body))
	if err != nil {
		return fail("the Gotify address does not look right", nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", token)
	code, raw, err := postApp(a, "Gotify", req)
	if err != nil {
		return err
	}
	if code/100 == 2 {
		return nil
	}
	var ans struct {
		Error       string `json:"error"`
		Description string `json:"errorDescription"`
	}
	_ = json.Unmarshal(raw, &ans)
	detail := scrub(strings.TrimSpace(ans.Error+" "+ans.Description), token)
	switch {
	case code == http.StatusTooManyRequests:
		return again("Gotify asked HolyArrs to slow down", detail)
	case code >= 500:
		return again(fmt.Sprintf("Gotify answered %d", code), detail)
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return fail("Gotify refused the app token", detail)
	case code == http.StatusBadRequest:
		return fail("Gotify refused the message", detail)
	}
	return statusFailure("Gotify", code)
}

// pushover sends the message through the Pushover API to the Alert's user.
func (s *Sender) pushover(ctx context.Context, a domain.Alert, m Message) error {
	token, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	user := strings.TrimSpace(a.UserKey)
	if checkPushKey(token, ErrPushToken) != nil {
		return fail("the Pushover app token is missing; type it again", nil)
	}
	if checkPushKey(user, ErrPushUser) != nil {
		return ErrPushUser
	}
	form := url.Values{"token": {token}, "user": {user}, "title": {cutRunes(clean(m.Subject), pushTitleMax)}, "message": {cutRunes(strings.TrimSpace(m.Text), pushMax)}}
	if link := firstLink(m.Text); link != "" && len(link) <= pushURLMax {
		form.Set("url", link)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushoverBase()+"/1/messages.json", strings.NewReader(form.Encode()))
	if err != nil {
		return fail("the Pushover address does not look right", nil)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	code, raw, err := postApp(a, "Pushover", req)
	if err != nil {
		return err
	}
	if code/100 == 2 {
		return nil
	}
	var ans struct {
		Errors []string `json:"errors"`
	}
	_ = json.Unmarshal(raw, &ans)
	reasons := strings.Join(ans.Errors, "; ")
	detail := scrub(reasons, token, user)
	switch {
	case code == http.StatusTooManyRequests:
		return again("Pushover asked HolyArrs to slow down", detail)
	case code >= 500:
		return again(fmt.Sprintf("Pushover answered %d", code), detail)
	case code == http.StatusBadRequest:
		low := strings.ToLower(reasons)
		if len(ans.Errors) == 0 || strings.Contains(low, "token") || strings.Contains(low, "user") {
			return fail("Pushover refused the app token or user key", detail)
		}
		return fail("Pushover refused the message", detail)
	}
	return statusFailure("Pushover", code)
}
