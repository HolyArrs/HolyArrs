package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func secretRows(t *testing.T, s *database.Store, where string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRowContext(context.Background(), "SELECT count(*) FROM secrets WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRemoveDropsSecretsAndRow(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	a, err := Add(ctx, s, v, Input{Name: "H", Kind: "webhook", URL: "https://x.example.com/h", Secret: "k", Moments: []string{MomentReady}}, domain.System)
	if err != nil || a.URLSecretID == nil || a.PasswordSecretID == nil {
		t.Fatalf("add %+v %v", a, err)
	}
	ids := []int64{*a.URLSecretID, *a.PasswordSecretID}
	if err := Remove(ctx, s, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if n := secretRows(t, s, "id = ?", id); n != 0 {
			t.Fatalf("secret %d left behind", id)
		}
	}
	if err := Remove(ctx, s, a.ID); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
	h := &Handlers{Store: s, Secrets: v}
	mux := http.NewServeMux()
	h.Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/alerts/"+strconv.FormatInt(a.ID, 10), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete unknown over HTTP: %d", rec.Code)
	}
}

func TestChangeReplacesSecretRow(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	urlA, urlB := "https://discord.com/api/webhooks/1/a", "https://discord.com/api/webhooks/2/b"
	a, err := Add(ctx, s, v, Input{Name: "D", Kind: "discord", URL: urlA, Moments: []string{MomentReady}}, domain.System)
	if err != nil || a.URLSecretID == nil {
		t.Fatalf("add %+v %v", a, err)
	}
	old := *a.URLSecretID
	b, err := Change(ctx, s, v, a.ID, Input{Name: "D", URL: urlB}, domain.System)
	if err != nil || b.URLSecretID == nil {
		t.Fatalf("change %+v %v", b, err)
	}
	// The id can come back (no AUTOINCREMENT), so check the old value is gone.
	if *b.URLSecretID != old && secretRows(t, s, "id = ?", old) != 0 {
		t.Fatal("old link row left behind")
	}
	if got, err := v.Get(ctx, *b.URLSecretID); err != nil || got != urlB {
		t.Fatalf("stored %q %v", got, err)
	}
	if n := secretRows(t, s, "kind = 'alert_url'"); n != 1 {
		t.Fatalf("alert_url rows %d", n)
	}
	if x := mustGet(t, s, a.ID); x.URLSecretID == nil || *x.URLSecretID != *b.URLSecretID {
		t.Fatalf("row points at %+v", x.URLSecretID)
	}
}

func TestEditWithMaskedSecretsKeepsStored(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	mux := http.NewServeMux()
	(&Handlers{Store: s, Secrets: v}).Routes(mux)
	do := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec
	}
	cases := []struct {
		field, plain string
		body         map[string]any
	}{
		{"secret", "s3cret", map[string]any{"name": "W", "kind": "webhook", "url": "https://x.example.com/h", "secret": "s3cret", "webhook_auth": "sign"}},
		{"bot_token", tgToken, map[string]any{"name": "T", "kind": "telegram", "bot_token": tgToken, "chat_id": "42"}},
		{"smtp_pass", "mailpw", map[string]any{"name": "M", "kind": "email", "smtp_host": "mail.example.com", "smtp_port": 587, "smtp_security": "starttls", "smtp_from": "h@a.test", "smtp_user": "h", "smtp_pass": "mailpw", "staff_to": "staff@a.test"}},
		{"app_token", ntfyToken, map[string]any{"name": "N", "kind": "ntfy", "server_url": "https://ntfy.example.com", "topic": "holyarrs", "app_token": ntfyToken}},
	}
	for _, c := range cases {
		c.body["moments"] = []string{MomentReady}
		rec := do(http.MethodPost, "/api/v1/alerts", c.body)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("%s add: %d %s", c.field, rec.Code, rec.Body)
		}
		var view View
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		edit := map[string]any{}
		for k, x := range c.body {
			edit[k] = x
		}
		edit[c.field] = secrets.Masked
		if _, ok := edit["url"]; ok {
			edit["url"] = secrets.Masked
		}
		if rec := do(http.MethodPut, "/api/v1/alerts/"+strconv.FormatInt(view.ID, 10), edit); rec.Code != http.StatusOK {
			t.Fatalf("%s edit: %d %s", c.field, rec.Code, rec.Body)
		}
		a := mustGet(t, s, view.ID)
		if a.PasswordSecretID == nil {
			t.Fatalf("%s: secret dropped", c.field)
		}
		if got, err := v.Get(ctx, *a.PasswordSecretID); err != nil || got != c.plain {
			t.Fatalf("%s: stored %q %v", c.field, got, err)
		}
	}
}
