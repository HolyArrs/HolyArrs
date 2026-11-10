package alerts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Moments an Alert can speak for.
const (
	MomentWaiting     = "ask_waiting"     // to the staff: an ask waits for approval
	MomentApproved    = "ask_approved"    // to the people who asked
	MomentTurnedDown  = "ask_turned_down" // to the people who asked
	MomentReady       = "ask_ready"       // to the people who asked: the movie is in the Library
	MomentNewEpisodes = "new_episodes"    // to the people who asked for or follow a show: new episodes are in the Library
	MomentNewAlbums   = "new_albums"      // to the people who follow an artist: a new album is in the Library
	MomentTest        = "test"            // the Send a test button
	MomentReported    = "report_made"     // to the staff: someone reported a problem with a title
	MomentFixed       = "report_fixed"    // to the person who reported it: the problem was fixed
)

// Library moments, told to the staff.
const (
	MomentGrab     = "grab"           // a Release was sent to a Downloader
	MomentImport   = "import"         // files were placed in the Library
	MomentRename   = "rename"         // files got new names
	MomentAdd      = "add"            // a movie was added
	MomentDelete   = "delete"         // a movie was deleted
	MomentHealth   = "health"         // a health check broke or recovered
	MomentDecision = "needs_decision" // a movie needs a decision
	MomentSubtitle = "subtitle"       // a subtitle was saved or upgraded, could not be saved, or a Subtitle site rests
)

// Moments lists the moments in the order screens show them.
var Moments = []string{MomentWaiting, MomentApproved, MomentTurnedDown, MomentReady, MomentNewEpisodes, MomentNewAlbums,
	MomentReported, MomentFixed, MomentGrab, MomentImport, MomentRename, MomentAdd, MomentDelete, MomentHealth, MomentDecision, MomentSubtitle}

// Person is someone a message is about, with where Alerts reach them.
type Person struct {
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	DiscordID string `json:"discord_id,omitempty"`
}

// Message is one moment to tell.
type Message struct {
	// lang is the app language of the fixed words ("" is English).
	lang    string
	Moment  string
	Subject string
	Text    string
	// People are the people who asked; they get the email for their moments
	// and are named in Discord.
	People []Person
	// Event is what happened, for a Library moment (nil for asks and tests).
	Event *domain.Event
	// Movie is the movie it is about, when there is one.
	Movie *Movie
	// Show and Episodes are the show and episodes it is about.
	Show     *Show
	Episodes []Episode
	// Artist and Tracks are the artist and tracks it is about.
	Artist *Artist
	Tracks []Track
	// Report is the problem report it is about, when there is one.
	Report *Report
}

// Report names the problem report a message is about.
type Report struct {
	ID      int64  `json:"id"`
	TitleID int64  `json:"title_id"`
	Title   string `json:"title"`
	Where   string `json:"where,omitempty"` // S02E03, Season 2, an album or track
	Kind    string `json:"kind"`            // picture, sound, subtitles, wont_play, other
	Note    string `json:"note,omitempty"`
	Reply   string `json:"reply,omitempty"`
}

// Movie names the movie a message is about.
type Movie struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Year *int   `json:"year,omitempty"`
	TMDB string `json:"tmdb,omitempty"`
	IMDb string `json:"imdb,omitempty"`
}

// Errors from checking an Alert.
var (
	ErrName     = errors.New("an Alert needs a name of at most 80 characters")
	ErrNameUsed = errors.New("another Alert has that name")
	ErrKind     = errors.New("kind must be discord, email, webhook, telegram, ntfy, gotify or pushover")
	ErrHook     = errors.New("paste the webhook link (http:// or https:// with a host); a server on the local network needs May reach the local network")
	ErrAuth     = errors.New("webhook_auth must be sign, bearer or none")
	// ErrSecretAgain keeps a webhook secret from going to another server:
	// changing the link's host needs the secret typed again.
	ErrSecretAgain = errors.New("type the webhook secret again when the webhook link points to another server")
	ErrURL         = errors.New("paste the Discord link (it starts with https://discord.com/); a server on the local network needs May reach the local network")
	ErrMoment      = errors.New("unknown moment")
	ErrSMTP        = errors.New("an email Alert needs the mail server name, a port between 1 and 65535 and a From address")
	ErrSecurity    = errors.New("security must be starttls, tls or none")
	ErrAddress     = errors.New("an email address does not look right")
	// ErrPasswordAgain keeps the stored mail password from going to another
	// server: changing the server, port, sign-in name or security needs it
	// typed again.
	ErrPasswordAgain = errors.New("type the mail password again when the mail server, port, sign-in name or security changes")
)

// failure is a send error: public is safe to store and show (no links,
// addresses or secrets); detail is only logged. retry marks a failure worth
// trying again (no answer, too busy, a server error).
type failure struct {
	public string
	detail error
	retry  bool
	// wait is how long the service asked us to wait before trying again.
	wait time.Duration
}

func (f *failure) Error() string { return f.public }
func (f *failure) Unwrap() error { return f.detail }

func fail(public string, detail error) error { return &failure{public: public, detail: detail} }

// again is a failure worth trying again.
func again(public string, detail error) error {
	return &failure{public: public, detail: detail, retry: true}
}

// retryable says whether err is worth another try.
func retryable(err error) bool {
	var f *failure
	return errors.As(err, &f) && f.retry
}

// askedWait is how long a failed service asked us to wait (at most five
// minutes), or 0.
func askedWait(err error) time.Duration {
	var f *failure
	if errors.As(err, &f) && f.wait > 0 {
		return min(f.wait, 5*time.Minute)
	}
	return 0
}

// statusFailure turns an HTTP answer into a failure: 408, 429 and 5xx are
// worth trying again, a redirect is never followed.
func statusFailure(who string, code int) error {
	if code/100 == 3 {
		return fail(fmt.Sprintf("%s answered %d (a redirect, not followed)", who, code), nil)
	}
	return &failure{public: fmt.Sprintf("%s answered %d", who, code), retry: code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500}
}

// DefaultBackoff is how long a Sender waits before each new try.
var DefaultBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

// Sender sends messages through every Alert that is on for the moment.
type Sender struct {
	Store *database.Store
	Vault *secrets.Vault
	// OnResult is told when an Alert's last result changes (may be nil).
	OnResult func(domain.Alert)
	// Backoff is the wait before each new try of a failed send; nil means
	// DefaultBackoff, an empty list never tries again.
	Backoff []time.Duration
	// Lang is the owner's app language for the fixed words of a message
	// (nil or "" means English).
	Lang func(context.Context) string

	wg sync.WaitGroup
}

// dialer connects only to public addresses unless the Alert may reach the
// local network; the check runs on each address after DNS. Link-local and
// cloud metadata addresses are refused even then.
func dialer(a domain.Alert) *net.Dialer {
	if a.AllowLocal {
		return outbound.PolicyDialer(outbound.NotMetadata)
	}
	return outbound.PolicyDialer(outbound.PublicOnly)
}

// client sends Discord messages for one Alert. Redirects are never
// followed, so the message (and the link's token) cannot be sent on.
func client(a domain.Alert) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // the policy must see the real address
	t.DialContext = dialer(a).DialContext
	return &http.Client{Timeout: 15 * time.Second, Transport: t,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Send tells every enabled Alert that speaks for m.Moment. One Alert
// failing does not stop the others; the errors are joined, recorded on the
// Alert and written as an Event.
func (s *Sender) Send(ctx context.Context, m Message) error {
	as, err := listEnabled(ctx, s.Store.Read())
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range as {
		if !slices.Contains(a.Moments, m.Moment) {
			continue
		}
		if err := s.SendOne(ctx, a, m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Name, err))
		}
	}
	return errors.Join(errs...)
}

// SendOne sends m through one Alert, trying again after each Backoff wait
// while the failure is worth it, and records the result on the Alert.
func (s *Sender) SendOne(ctx context.Context, a domain.Alert, m Message) error {
	op, err := outbound.Begin(ctx, alertTarget(a))
	if err != nil {
		return err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	waits := s.Backoff
	if waits == nil {
		waits = DefaultBackoff
	}
	err = s.attempt(ctx, op, a, m)
	for i := 0; err != nil && retryable(err) && i < len(waits); i++ {
		t := time.NewTimer(max(waits[i], askedWait(err)))
		select {
		case <-ctx.Done():
			t.Stop()
			s.record(ctx, a, m, err)
			return err
		case <-t.C:
		}
		err = s.attempt(ctx, op, a, m)
	}
	outcome = alertOutcome(ctx, err)
	s.record(ctx, a, m, err)
	return err
}

// TestOne sends m once, with no new tries (the Send a test button).
func (s *Sender) TestOne(ctx context.Context, a domain.Alert, m Message) error {
	op, err := outbound.Begin(ctx, alertTarget(a))
	if err != nil {
		return err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	err = s.attempt(ctx, op, a, m)
	outcome = alertOutcome(ctx, err)
	s.record(ctx, a, m, err)
	return err
}

func (s *Sender) attempt(ctx context.Context, op *outbound.Operation, a domain.Alert, m Message) error {
	caller := ctx
	ctx, cancel := op.AttemptContext(ctx, 30*time.Second)
	defer cancel()
	var err error
	switch a.Kind {
	case "discord":
		err = s.discord(ctx, a, m)
	case "email":
		err = s.email(ctx, a, m)
	case "webhook":
		err = s.webhook(ctx, a, m)
	case "telegram":
		err = s.telegram(ctx, a, m)
	case "ntfy":
		err = s.ntfy(ctx, a, m)
	case "gotify":
		err = s.gotify(ctx, a, m)
	case "pushover":
		err = s.pushover(ctx, a, m)
	default:
		return ErrKind
	}
	if caller.Err() != nil {
		return caller.Err()
	}
	if ctx.Err() != nil {
		return again("the outside service did not answer in time", ctx.Err())
	}
	return err
}

func alertTarget(a domain.Alert) outbound.Target {
	ids := []string{strconv.FormatInt(a.ID, 10), a.Kind, a.SMTPHost, strconv.Itoa(a.SMTPPort), a.SMTPSecurity, a.SMTPUser, a.ServerURL}
	for _, id := range []*int64{a.URLSecretID, a.PasswordSecretID} {
		if id == nil {
			ids = append(ids, "")
		} else {
			ids = append(ids, strconv.FormatInt(*id, 10))
		}
	}
	return outbound.Target{Kind: "alert", Identity: ids}
}

func alertOutcome(ctx context.Context, err error) outbound.Outcome {
	if ctx.Err() != nil {
		return outbound.Canceled
	}
	if err == nil {
		return outbound.Success
	}
	if retryable(err) {
		return outbound.TransientFailure
	}
	return outbound.TerminalFailure
}

func (s *Sender) record(ctx context.Context, a domain.Alert, m Message, sendErr error) {
	now := database.FormatTime(time.Now())
	msg := ""
	if sendErr != nil {
		msg = sendErr.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
	}
	err := s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		if sendErr != nil {
			return markFailed(tx, a.ID, msg)
		}
		return markSent(tx, a.ID, now)
	})
	if err != nil {
		log.Printf("alerts: record %s: %v", a.Name, err)
	}
	if sendErr != nil {
		detail := sendErr
		if f := (*failure)(nil); errors.As(sendErr, &f) && f.detail != nil {
			detail = fmt.Errorf("%s: %w", f.public, f.detail)
		}
		log.Printf("alerts: %s (%s): %s", a.Name, m.Moment, redact(detail.Error()))
		_ = s.Store.AddEvent(context.WithoutCancel(ctx), &domain.Event{Kind: domain.EventAlertFailed, Summary: "Alert " + a.Name + " could not be sent",
			Data: map[string]any{"alert_id": a.ID, "moment": m.Moment, "error": msg}, GroupKey: "alert:" + strconv.FormatInt(a.ID, 10)})
	}
	if s.OnResult != nil {
		if fresh, err := get(ctx, s.Store.Read(), a.ID); err == nil {
			s.OnResult(fresh)
		}
	}
}

// redact drops anything that looks like a link from a logged error, so a
// Discord token never reaches the log.
func redact(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if strings.Contains(w, "://") {
			words[i] = "[link]"
		}
	}
	return strings.Join(words, " ")
}

func (s *Sender) secret(ctx context.Context, id *int64) (string, error) {
	if id == nil || s.Vault == nil {
		return "", nil
	}
	v, err := s.Vault.Get(ctx, *id)
	if err != nil {
		return "", fail("the Alert's secret could not be read", err)
	}
	return v, nil
}

// forPeople says whether a moment is told to the people who asked.
func forPeople(moment string) bool {
	return moment == MomentApproved || moment == MomentTurnedDown || moment == MomentReady || moment == MomentNewEpisodes || moment == MomentNewAlbums ||
		moment == MomentFixed
}

func (s *Sender) discord(ctx context.Context, a domain.Alert, m Message) error {
	link, err := s.secret(ctx, a.URLSecretID)
	if err != nil {
		return err
	}
	if CheckDiscordURL(link, a.AllowLocal) != nil {
		return ErrURL
	}
	var mentions, ids []string
	if forPeople(m.Moment) {
		for _, p := range m.People {
			if validDiscordID(p.DiscordID) {
				mentions = append(mentions, "<@"+p.DiscordID+">")
				ids = append(ids, p.DiscordID)
			}
		}
	}
	content := "**" + m.Subject + "**\n" + m.Text
	if len(mentions) > 0 {
		content = strings.Join(mentions, " ") + " " + content
	}
	content = cutRunes(content, discordMax)
	body, _ := json.Marshal(map[string]any{"username": "HolyArrs", "content": content,
		"allowed_mentions": map[string]any{"parse": []string{}, "users": append([]string{}, ids...)}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, bytes.NewReader(body))
	if err != nil {
		return ErrURL
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client(a).Do(req)
	if err != nil {
		// The link holds the Discord token: never keep it in the error.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return fail("Discord link points to the local network, which this Alert may not reach", err)
		}
		return again("Discord did not answer", err)
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return again("Discord's answer could not be read", readErr)
	}
	if resp.StatusCode/100 != 2 {
		return statusFailure("Discord", resp.StatusCode)
	}
	return nil
}

func validDiscordID(id string) bool {
	if len(id) < 5 || len(id) > 25 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// CheckDiscordURL accepts an https link on discord.com or discordapp.com
// (or one of their subdomains). With allowLocal (the Alert may reach the
// local network) any http or https link with a host is accepted, for a
// Discord-like server at home.
func CheckDiscordURL(raw string, allowLocal bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return ErrURL
	}
	if allowLocal {
		if u.Scheme != "https" && u.Scheme != "http" {
			return ErrURL
		}
		return nil
	}
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
		return ErrURL
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range []string{"discord.com", "discordapp.com"} {
		if h == d || strings.HasSuffix(h, "."+d) {
			return nil
		}
	}
	return ErrURL
}

// CheckDiscordID accepts an empty id or a Discord user id (digits only).
func CheckDiscordID(id string) bool { return id == "" || validDiscordID(id) }

// CheckAddress accepts one plain email address.
func CheckAddress(a string) bool {
	if a == "" || strings.ContainsAny(a, "\r\n<>,") {
		return false
	}
	p, err := mail.ParseAddress(a)
	return err == nil && p.Address == a
}

// recipients lists the addresses a moment goes to.
func recipients(a domain.Alert, m Message) []string {
	var to []string
	add := func(x string) {
		x = strings.TrimSpace(x)
		if CheckAddress(x) && !slices.Contains(to, x) {
			to = append(to, x)
		}
	}
	if forPeople(m.Moment) {
		for _, p := range m.People {
			add(p.Email)
		}
	} else {
		for _, x := range strings.Split(a.StaffTo, ",") {
			add(x)
		}
	}
	return to
}

// Discord takes at most 2000 characters in content; mail subjects are kept
// well under the 998-octet line limit once encoded.
const (
	discordMax = 2000
	subjectMax = 200
)

// cutRunes keeps at most n letters of s, ending a cut one with an ellipsis
// at a whole letter.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:n-1]), " \n") + "…"
}

// subjectHeader writes a Subject value: no line breaks, at most subjectMax
// letters, non-ASCII as RFC 2047 words, each on its own folded line.
func subjectHeader(s string) string {
	return strings.ReplaceAll(mime.QEncoding.Encode("utf-8", cutRunes(clean(s), subjectMax)), "?= =?", "?=\r\n =?")
}

func clean(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }

func (s *Sender) email(ctx context.Context, a domain.Alert, m Message) error {
	to := recipients(a, m)
	if len(to) == 0 {
		return nil // nobody to reach for this moment
	}
	if a.SMTPHost == "" || a.SMTPPort < 1 || a.SMTPPort > 65535 || !CheckAddress(a.SMTPFrom) {
		return ErrSMTP
	}
	pass, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(a.SMTPHost, strconv.Itoa(a.SMTPPort))
	d := dialer(a)
	var conn net.Conn
	tc := &tls.Config{ServerName: a.SMTPHost, MinVersion: tls.VersionTLS12}
	if a.SMTPSecurity == "tls" {
		conn, err = (&tls.Dialer{NetDialer: d, Config: tc}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		if errors.Is(err, outbound.ErrAddressRefused) {
			return fail("mail server is on the local network, which this Alert may not reach", err)
		}
		return again("mail server did not answer", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	c, err := smtp.NewClient(conn, a.SMTPHost)
	if err != nil {
		conn.Close()
		return again("mail server did not greet", err)
	}
	defer c.Close()
	if a.SMTPSecurity == "starttls" {
		if err := c.StartTLS(tc); err != nil {
			return fail("mail server refused a secure link", err)
		}
	}
	if a.SMTPUser != "" {
		if err := c.Auth(smtp.PlainAuth("", a.SMTPUser, pass, a.SMTPHost)); err != nil {
			return fail("mail server refused the sign-in", err)
		}
	}
	if err := c.Mail(a.SMTPFrom); err != nil {
		return fail("mail server refused the From address", err)
	}
	for _, x := range to {
		if err := c.Rcpt(x); err != nil {
			return fail("mail server refused a recipient", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fail("mail server refused the message", err)
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	var b strings.Builder
	fmt.Fprintf(&b, "From: HolyArrs <%s>\r\n", a.SMTPFrom)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	// An episode name can hold anything (subjectHeader).
	fmt.Fprintf(&b, "Subject: %s\r\n", subjectHeader(m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@holyarrs>\r\n", hex.EncodeToString(id))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	// The DATA writer doubles leading dots and ends lines with CRLF.
	b.WriteString(strings.ReplaceAll(m.Text, "\r", "") + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fail("mail server refused the message", err)
	}
	if err := w.Close(); err != nil {
		return fail("mail server refused the message", err)
	}
	if err := c.Quit(); err != nil {
		return fail("mail server did not say goodbye", err)
	}
	return nil
}

func (s *Sender) lang(ctx context.Context) string {
	if s.Lang == nil {
		return ""
	}
	return s.Lang(ctx)
}
