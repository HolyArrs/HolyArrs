package alerts

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Input is what a person sets on an Alert. URL, Password, Secret, BotToken
// and AppToken are only changed when sent; an empty one keeps what is stored.
type Input struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Enabled      *bool    `json:"enabled"`
	Moments      []string `json:"moments"`
	URL          string   `json:"url"`
	SMTPHost     string   `json:"smtp_host"`
	SMTPPort     int      `json:"smtp_port"`
	SMTPSecurity string   `json:"smtp_security"`
	SMTPFrom     string   `json:"smtp_from"`
	SMTPUser     string   `json:"smtp_user"`
	Password     string   `json:"smtp_pass"`
	StaffTo      string   `json:"staff_to"`
	// Secret signs a webhook's messages or is sent as its bearer token
	// (WebhookAuth: sign, bearer or none).
	Secret      string `json:"secret"`
	WebhookAuth string `json:"webhook_auth"`
	// AllowLocal lets the Alert reach the local network; nil keeps what is
	// stored (off for a new Alert).
	AllowLocal *bool `json:"allow_local_network"`
	// BotToken and ChatID are a Telegram Alert's bot and chat.
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	// ServerURL is an ntfy or Gotify server (ntfy: empty means ntfy.sh),
	// Topic the ntfy topic and UserKey the Pushover user key. AppToken is the
	// ntfy access token, the Gotify app token or the Pushover app token.
	// ClearToken drops a stored ntfy access token (the only optional one)
	// when no new token is sent.
	ServerURL  string `json:"server_url"`
	Topic      string `json:"topic"`
	UserKey    string `json:"user_key"`
	AppToken   string `json:"app_token"`
	ClearToken bool   `json:"clear_app_token"`
}

// fill checks in and copies it onto a. New Alerts speak for every moment
// unless told otherwise.
func fill(in Input, a *domain.Alert, isNew bool) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	if isNew {
		if !slices.Contains([]string{"discord", "email", "webhook", "telegram", "ntfy", "gotify", "pushover"}, in.Kind) {
			return ErrKind
		}
		a.Kind = in.Kind
	}
	if a.WebhookAuth == "" {
		a.WebhookAuth = "sign"
	}
	moments := in.Moments
	if moments == nil {
		moments = a.Moments
		if isNew {
			moments = slices.Clone(Moments)
		}
	}
	for _, m := range moments {
		if !slices.Contains(Moments, m) {
			return ErrMoment
		}
	}
	a.Name, a.Moments = name, moments
	a.Enabled = in.Enabled == nil || *in.Enabled
	if in.AllowLocal != nil {
		a.AllowLocal = *in.AllowLocal
	}
	if a.Kind == "webhook" {
		if a.SMTPSecurity == "" {
			a.SMTPSecurity, a.SMTPPort = "starttls", 587
		}
		if in.WebhookAuth != "" {
			if in.WebhookAuth != "sign" && in.WebhookAuth != "bearer" && in.WebhookAuth != "none" {
				return ErrAuth
			}
			a.WebhookAuth = in.WebhookAuth
		}
		if in.URL != "" && CheckWebhookURL(in.URL, a.AllowLocal) != nil {
			return ErrHook
		}
		if isNew && in.URL == "" {
			return ErrHook
		}
		return nil
	}
	if a.Kind == "telegram" {
		if a.SMTPSecurity == "" {
			a.SMTPSecurity, a.SMTPPort = "starttls", 587
		}
		if (isNew || in.BotToken != "") && CheckTelegramToken(in.BotToken) != nil {
			return ErrTelegramToken
		}
		chat := strings.TrimSpace(in.ChatID)
		if CheckTelegramChat(chat) != nil {
			return ErrTelegramChat
		}
		a.ChatID = chat
		return nil
	}
	if a.Kind == "ntfy" || a.Kind == "gotify" || a.Kind == "pushover" {
		if a.SMTPSecurity == "" {
			a.SMTPSecurity, a.SMTPPort = "starttls", 587
		}
		return fillApp(in, a, isNew)
	}
	if a.Kind == "discord" {
		if a.SMTPSecurity == "" {
			a.SMTPSecurity, a.SMTPPort = "starttls", 587
		}
		if in.URL != "" && CheckDiscordURL(in.URL, a.AllowLocal) != nil {
			return ErrURL
		}
		if isNew && in.URL == "" {
			return ErrURL
		}
		return nil
	}
	sec := in.SMTPSecurity
	if sec == "" {
		sec = "starttls"
	}
	if sec != "starttls" && sec != "tls" && sec != "none" {
		return ErrSecurity
	}
	port := in.SMTPPort
	if port == 0 {
		port = 587
	}
	host := strings.TrimSpace(in.SMTPHost)
	if host == "" || strings.ContainsAny(host, " /:") || port < 1 || port > 65535 || !CheckAddress(strings.TrimSpace(in.SMTPFrom)) {
		return ErrSMTP
	}
	for _, x := range strings.Split(in.StaffTo, ",") {
		if x = strings.TrimSpace(x); x != "" && !CheckAddress(x) {
			return ErrAddress
		}
	}
	a.SMTPHost, a.SMTPPort, a.SMTPSecurity, a.SMTPFrom = host, port, sec, strings.TrimSpace(in.SMTPFrom)
	a.SMTPUser, a.StaffTo = strings.TrimSpace(in.SMTPUser), strings.TrimSpace(in.StaffTo)
	return nil
}

// fillApp checks and copies an ntfy, Gotify or Pushover Alert's fields. A
// token is needed when adding (ntfy: never); an empty one on change keeps
// the stored one.
func fillApp(in Input, a *domain.Alert, isNew bool) error {
	token := strings.TrimSpace(in.AppToken)
	switch a.Kind {
	case "ntfy":
		server := strings.TrimRight(strings.TrimSpace(in.ServerURL), "/")
		if server == "" {
			server = ntfyDefault
		}
		if CheckServerURL(server, a.AllowLocal) != nil {
			return ErrServer
		}
		topic := strings.TrimSpace(in.Topic)
		if CheckNtfyTopic(topic) != nil {
			return ErrNtfyTopic
		}
		if token != "" && checkAppToken(token, ErrNtfyToken) != nil {
			return ErrNtfyToken
		}
		a.ServerURL, a.Topic = server, topic
	case "gotify":
		server := strings.TrimRight(strings.TrimSpace(in.ServerURL), "/")
		if CheckServerURL(server, a.AllowLocal) != nil {
			return ErrServer
		}
		if (isNew || token != "" || in.ClearToken) && checkAppToken(token, ErrGotifyToken) != nil {
			return ErrGotifyToken
		}
		a.ServerURL = server
	case "pushover":
		if (isNew || token != "" || in.ClearToken) && checkPushKey(token, ErrPushToken) != nil {
			return ErrPushToken
		}
		user := strings.TrimSpace(in.UserKey)
		if checkPushKey(user, ErrPushUser) != nil {
			return ErrPushUser
		}
		a.UserKey = user
	}
	return nil
}

// Add stores a new Alert; its secrets go to the vault.
func Add(ctx context.Context, s *database.Store, v *secrets.Vault, in Input, by domain.Actor) (domain.Alert, error) {
	var a domain.Alert
	if err := fill(in, &a, true); err != nil {
		return a, err
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, a.Name, 0); err != nil {
			return err
		}
		if in.URL != "" {
			id, err := v.PutTx(tx, "alert_url", strings.TrimSpace(in.URL), by)
			if err != nil {
				return err
			}
			a.URLSecretID = &id
		}
		pass, kind := secretFor(a.Kind, in)
		if pass != "" {
			id, err := v.PutTx(tx, kind, pass, by)
			if err != nil {
				return err
			}
			a.PasswordSecretID = &id
		}
		_, err := database.Insert(tx, "alerts", &a, by)
		return err
	})
	return a, err
}

// Change saves an Alert; an empty URL or password keeps the stored one.
func Change(ctx context.Context, s *database.Store, v *secrets.Vault, id int64, in Input, by domain.Actor) (domain.Alert, error) {
	a, err := get(ctx, s.Read(), id)
	if err != nil {
		return a, err
	}
	old := a
	if err := fill(in, &a, false); err != nil {
		return a, err
	}
	// The stored password never goes to another mail server: when where it
	// is sent changes, it must be typed again.
	if a.Kind == "email" && a.PasswordSecretID != nil && in.Password == "" &&
		(a.SMTPHost != old.SMTPHost || a.SMTPPort != old.SMTPPort || a.SMTPUser != old.SMTPUser || a.SMTPSecurity != old.SMTPSecurity) {
		return old, ErrPasswordAgain
	}
	// Likewise a webhook secret never goes to another host or scheme (never down to plain http).
	if a.Kind == "webhook" && a.PasswordSecretID != nil && in.Secret == "" && in.URL != "" && a.URLSecretID != nil {
		was, err := v.Get(ctx, *a.URLSecretID)
		if err != nil {
			return old, err
		}
		if originOf(was) != originOf(in.URL) {
			return old, ErrSecretAgain
		}
	}
	// And an ntfy or Gotify token never goes to another server.
	dropToken := a.Kind == "ntfy" && in.ClearToken && strings.TrimSpace(in.AppToken) == ""
	if (a.Kind == "ntfy" || a.Kind == "gotify") && a.PasswordSecretID != nil && strings.TrimSpace(in.AppToken) == "" && !dropToken &&
		originOf(a.ServerURL) != originOf(old.ServerURL) {
		return old, ErrTokenAgain
	}
	pass, passKind := secretFor(a.Kind, in)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, a.Name, a.ID); err != nil {
			return err
		}
		put := func(ref **int64, kind, plain string) error {
			if plain == "" {
				return nil
			}
			if *ref != nil {
				if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", **ref); err != nil {
					return err
				}
			}
			nid, err := v.PutTx(tx, kind, plain, by)
			*ref = &nid
			return err
		}
		if err := put(&a.URLSecretID, "alert_url", strings.TrimSpace(in.URL)); err != nil {
			return err
		}
		if err := put(&a.PasswordSecretID, passKind, pass); err != nil {
			return err
		}
		if dropToken && a.PasswordSecretID != nil {
			if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", *a.PasswordSecretID); err != nil {
				return err
			}
			a.PasswordSecretID = nil
		}
		return database.Update(tx, "alerts", &a, by)
	})
	return a, err
}

// secretFor is the secret an Alert keeps under PasswordSecretID, and its
// vault kind: the mail password, the webhook secret, the Telegram bot token
// or the ntfy, Gotify or Pushover token.
func secretFor(kind string, in Input) (string, string) {
	switch kind {
	case "webhook":
		return in.Secret, "alert_webhook_secret"
	case "telegram":
		return strings.TrimSpace(in.BotToken), "alert_telegram_token"
	case "ntfy", "gotify", "pushover":
		return strings.TrimSpace(in.AppToken), "alert_" + kind + "_token"
	}
	return in.Password, "alert_password"
}

// Remove deletes an Alert and its secrets.
func Remove(ctx context.Context, s *database.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, ref := range []*int64{a.URLSecretID, a.PasswordSecretID} {
			if ref != nil {
				if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", *ref); err != nil {
					return err
				}
			}
		}
		return deleteRow(tx, id)
	})
}

// originOf is a link's scheme, host and port, lower case: a secret
// stays only while all three stay (no move to plain http).
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// IsInputError says err is a refusal of what was sent.
func IsInputError(err error) bool {
	for _, e := range []error{ErrName, ErrKind, ErrURL, ErrMoment, ErrSMTP, ErrSecurity, ErrAddress, ErrPasswordAgain, ErrHook, ErrAuth, ErrSecretAgain, ErrTelegramToken, ErrTelegramChat,
		ErrServer, ErrNtfyTopic, ErrNtfyToken, ErrGotifyToken, ErrPushToken, ErrPushUser, ErrTokenAgain} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
