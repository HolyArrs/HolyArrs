package alerts

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Input is what a person sets on an Alert. URL and Password are only
// changed when sent; an empty one keeps what is stored.
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
}

// fill checks in and copies it onto a. New Alerts speak for every moment
// unless told otherwise.
func fill(in Input, a *domain.Alert, isNew bool) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	if isNew {
		if in.Kind != "discord" && in.Kind != "email" && in.Kind != "webhook" {
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

func nameUsed(tx *sql.Tx, name string, id int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM alerts WHERE name = ? COLLATE NOCASE AND id != ?", name, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNameUsed
	}
	return nil
}

// Add stores a new Alert; its secrets go to the vault.
func Add(ctx context.Context, s *store.Store, v *secrets.Vault, in Input, by domain.Actor) (domain.Alert, error) {
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
		pass, kind := in.Password, "alert_password"
		if a.Kind == "webhook" {
			pass, kind = in.Secret, "alert_webhook_secret"
		}
		if pass != "" {
			id, err := v.PutTx(tx, kind, pass, by)
			if err != nil {
				return err
			}
			a.PasswordSecretID = &id
		}
		_, err := store.Insert(tx, "alerts", &a, by)
		return err
	})
	return a, err
}

// Change saves an Alert; an empty URL or password keeps the stored one.
func Change(ctx context.Context, s *store.Store, v *secrets.Vault, id int64, in Input, by domain.Actor) (domain.Alert, error) {
	var a domain.Alert
	if err := store.Get(ctx, s.Read(), "alerts", id, &a); err != nil {
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
	pass, passKind := in.Password, "alert_password"
	if a.Kind == "webhook" {
		pass, passKind = in.Secret, "alert_webhook_secret"
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
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
		return store.Update(tx, "alerts", &a, by)
	})
	return a, err
}

// Remove deletes an Alert and its secrets.
func Remove(ctx context.Context, s *store.Store, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		var a domain.Alert
		if err := store.Get(ctx, tx, "alerts", id, &a); err != nil {
			return err
		}
		for _, ref := range []*int64{a.URLSecretID, a.PasswordSecretID} {
			if ref != nil {
				if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", *ref); err != nil {
					return err
				}
			}
		}
		res, err := tx.Exec("DELETE FROM alerts WHERE id = ?", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
		return nil
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
	for _, e := range []error{ErrName, ErrKind, ErrURL, ErrMoment, ErrSMTP, ErrSecurity, ErrAddress, ErrPasswordAgain, ErrHook, ErrAuth, ErrSecretAgain} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
