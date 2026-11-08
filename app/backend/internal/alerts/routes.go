package alerts

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// alertHelp is the shape of an Alert body, sent back when a body does not parse.
const alertHelp = `send name, kind (discord, email, webhook, telegram, ntfy, gotify or pushover), moments, and for Discord url; for email smtp_host, smtp_port, smtp_security, smtp_from, smtp_user, smtp_pass, staff_to; for a webhook url, secret and webhook_auth (sign, bearer or none); for Telegram bot_token and chat_id; for ntfy server_url (default https://ntfy.sh), topic and app_token (optional access token; clear_app_token drops a stored one); for Gotify server_url and app_token; for Pushover user_key and app_token; allow_local_network lets it reach the local network`

// Handlers serves the Alert routes and the Alert settings.
type Handlers struct {
	Store   *database.Store
	Secrets *secrets.Vault
	Sender  *Sender
	// Caller names the signed-in person (user id and display name).
	Caller func(r *http.Request) (userID int64, name string)
	// OwnEmail reads a person's own email from My account; a test of an email
	// Alert without a staff address goes there.
	OwnEmail func(ctx context.Context, userID int64) (string, error)
}

// View is an Alert as the API shows it: secrets are masked.
type View struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Enabled      bool       `json:"enabled"`
	Moments      []string   `json:"moments"`
	URL          string     `json:"url,omitempty"`
	SMTPHost     string     `json:"smtp_host,omitempty"`
	SMTPPort     int        `json:"smtp_port,omitempty"`
	SMTPSecurity string     `json:"smtp_security,omitempty"`
	SMTPFrom     string     `json:"smtp_from,omitempty"`
	SMTPUser     string     `json:"smtp_user,omitempty"`
	Password     string     `json:"smtp_pass,omitempty"`
	StaffTo      string     `json:"staff_to,omitempty"`
	Secret       string     `json:"secret,omitempty"`
	WebhookAuth  string     `json:"webhook_auth,omitempty"`
	BotToken     string     `json:"bot_token,omitempty"`
	ChatID       string     `json:"chat_id,omitempty"`
	ServerURL    string     `json:"server_url,omitempty"`
	Topic        string     `json:"topic,omitempty"`
	UserKey      string     `json:"user_key,omitempty"`
	AppToken     string     `json:"app_token,omitempty"`
	AllowLocal   bool       `json:"allow_local_network"`
	LastSentAt   *time.Time `json:"last_sent_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// Routes adds the Alert settings routes, and the Alert list routes when a
// secrets vault is there to hold their links and passwords.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/alerts", h.getSettings)
	m.HandleFunc("PUT /api/v1/settings/alerts", h.putSettings)
	if h.Secrets == nil {
		return
	}
	m.HandleFunc("GET /api/v1/alerts", h.list)
	m.HandleFunc("POST /api/v1/alerts", h.add)
	m.HandleFunc("PUT /api/v1/alerts/{id}", h.edit)
	m.HandleFunc("DELETE /api/v1/alerts/{id}", h.delete)
	m.HandleFunc("POST /api/v1/alerts/{id}/test", h.test)
}

// ViewOf masks an Alert's secrets for the API.
func ViewOf(x domain.Alert) View {
	v := View{ID: x.ID, Name: x.Name, Kind: x.Kind, Enabled: x.Enabled, Moments: x.Moments, AllowLocal: x.AllowLocal, LastSentAt: x.LastSentAt, LastError: x.LastError}
	if v.Moments == nil {
		v.Moments = []string{}
	}
	if x.Kind == "telegram" {
		v.ChatID = x.ChatID
		if x.PasswordSecretID != nil {
			v.BotToken = secrets.Masked
		}
		return v
	}
	if x.Kind == "ntfy" || x.Kind == "gotify" || x.Kind == "pushover" {
		v.ServerURL, v.Topic, v.UserKey = x.ServerURL, x.Topic, x.UserKey
		if x.PasswordSecretID != nil {
			v.AppToken = secrets.Masked
		}
		return v
	}
	if x.Kind == "discord" || x.Kind == "webhook" {
		if x.URLSecretID != nil {
			v.URL = secrets.Masked
		}
		if x.Kind == "webhook" {
			v.WebhookAuth = x.WebhookAuth
			if x.PasswordSecretID != nil {
				v.Secret = secrets.Masked
			}
		}
		return v
	}
	v.SMTPHost, v.SMTPPort, v.SMTPSecurity, v.SMTPFrom, v.SMTPUser, v.StaffTo = x.SMTPHost, x.SMTPPort, x.SMTPSecurity, x.SMTPFrom, x.SMTPUser, x.StaffTo
	if x.PasswordSecretID != nil {
		v.Password = secrets.Masked
	}
	return v
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	xs, err := listAll(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []View{}
	for _, x := range xs {
		out = append(out, ViewOf(x))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"alerts": out, "moments": Moments})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	x, err := Add(r.Context(), h.Store, h.Secrets, in, webserver.RequestActor(r))
	if err != nil {
		answerError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, ViewOf(x))
}

func (h *Handlers) edit(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readInput(w, r)
	if !ok {
		return
	}
	x, err := Change(r.Context(), h.Store, h.Secrets, id, in, webserver.RequestActor(r))
	if err != nil {
		answerError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ViewOf(x))
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := Remove(r.Context(), h.Store, id); err != nil {
		answerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// test sends a test message through one Alert. An email Alert without a
// staff address sends it to the person testing, at their own email.
func (h *Handlers) test(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	x, err := get(r.Context(), h.Store.Read(), id)
	if err != nil {
		answerError(w, err)
		return
	}
	m := Message{Moment: MomentWaiting, Subject: "HolyArrs test", Text: "This is a test from HolyArrs. If you can read it, the Alert " + x.Name + " works."}
	if x.Kind == "webhook" {
		m.Moment = MomentTest
	}
	if x.Kind == "email" && strings.TrimSpace(x.StaffTo) == "" {
		userID, name := h.Caller(r)
		email, _ := h.OwnEmail(r.Context(), userID)
		if email == "" {
			webserver.Fail(w, http.StatusBadRequest, "add a staff address to this Alert, or your own email in My account, to send a test")
			return
		}
		m.Moment, m.People = MomentReady, []Person{{Name: name, Email: email}}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.Sender.TestOne(ctx, x, m); err != nil {
		// The reason is logged and kept on the Alert as its last result, so
		// the answer stays the same every time and never echoes a link.
		webserver.Fail(w, http.StatusBadGateway, "the Discord, Telegram, ntfy, Gotify, Pushover, mail or webhook server refused the test")
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// readInput reads an Alert body; a masked placeholder keeps the stored secret.
func readInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	if !webserver.Decode(w, r, &in, alertHelp) {
		return in, false
	}
	if in.URL == secrets.Masked {
		in.URL = ""
	}
	if in.Password == secrets.Masked {
		in.Password = ""
	}
	if in.Secret == secrets.Masked {
		in.Secret = ""
	}
	if in.BotToken == secrets.Masked {
		in.BotToken = ""
	}
	if in.AppToken == secrets.Masked {
		in.AppToken = ""
	}
	return in, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Alert")
		return 0, false
	}
	return id, true
}

func answerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameUsed):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case IsInputError(err):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such Alert")
	default:
		webserver.Internal(w, "api", err)
	}
}
