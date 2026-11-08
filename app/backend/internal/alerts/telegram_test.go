package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A made-up bot token in the shape @BotFather gives.
var tgToken = strings.Join([]string{"123456789", "AAHstandInTokenForTests_0123456789"}, ":")

type tgCall struct {
	path string
	body map[string]any
}

// fakeTelegram answers the Bot API's sendMessage with answer (code and body)
// and keeps what it was sent. HOLYARRS_TELEGRAM_URL points the sender at it.
func fakeTelegram(t *testing.T, answer func(n int) (int, string)) func() []tgCall {
	var mu sync.Mutex
	var got []tgCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, tgCall{path: r.URL.Path, body: m})
		n := len(got)
		mu.Unlock()
		code, body := answer(n)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOLYARRS_TELEGRAM_URL", srv.URL)
	return func() []tgCall { mu.Lock(); defer mu.Unlock(); return append([]tgCall{}, got...) }
}

func addTelegramVault(t *testing.T, s *database.Store, v *secrets.Vault) domain.Alert {
	t.Helper()
	a, err := Add(context.Background(), s, v, Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "42", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func noToken(t *testing.T, where, s string) {
	t.Helper()
	if strings.Contains(s, tgToken) || strings.Contains(s, "AAHstandIn") {
		t.Fatalf("the bot token leaked into %s: %s", where, s)
	}
}

func TestTelegramSendsPlainText(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	got := fakeTelegram(t, func(int) (int, string) { return 200, `{"ok":true,"result":{}}` })
	a, err := Add(ctx, s, v, Input{Name: "Family Telegram", Kind: "telegram", BotToken: tgToken, ChatID: " -1001234567890 ", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if a.PasswordSecretID == nil || a.ChatID != "-1001234567890" || len(a.Moments) != len(Moments) {
		t.Fatalf("alert %+v", a)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.Send(ctx, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now.\nhttps://example.com/dune",
		People: []Person{{Name: "ana", DiscordID: "123456789012"}}}); err != nil {
		t.Fatal(err)
	}
	calls := got()
	if len(calls) != 1 || calls[0].path != "/bot"+tgToken+"/sendMessage" {
		t.Fatalf("calls %+v", calls)
	}
	b := calls[0].body
	if b["chat_id"] != "-1001234567890" || b["text"] != "Dune is ready\nWatch it now.\nhttps://example.com/dune" || b["disable_web_page_preview"] != true {
		t.Fatalf("body %v", b)
	}
	if _, ok := b["parse_mode"]; ok {
		t.Fatalf("plain text must not set parse_mode: %v", b)
	}
	var fresh domain.Alert
	if err := database.Get(ctx, s.Read(), "alerts", a.ID, &fresh); err != nil || fresh.LastSentAt == nil || fresh.LastError != "" {
		t.Fatalf("result %+v %v", fresh, err)
	}
}

func TestTelegramRefusals(t *testing.T) {
	cases := []struct {
		name, body, want string
		code             int
	}{
		{"refused token", `{"ok":false,"error_code":401,"description":"Unauthorized"}`, "Telegram refused the bot token", 401},
		{"unknown token", `{"ok":false,"error_code":404,"description":"Not Found"}`, "Telegram refused the bot token", 404},
		{"unknown chat", `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, "Telegram could not find that chat", 400},
		{"blocked", `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`, "Telegram would not let the bot write to that chat", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, v := open(t)
			ctx := context.Background()
			got := fakeTelegram(t, func(int) (int, string) { return c.code, c.body })
			a, err := Add(ctx, s, v, Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "42", AllowLocal: &yes}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			// A final refusal is not tried again, even with waits to spare.
			snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{time.Millisecond, time.Millisecond}}
			err = snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"})
			if err == nil || !strings.HasPrefix(err.Error(), c.want) || retryable(err) {
				t.Fatalf("err %v", err)
			}
			if n := len(got()); n != 1 {
				t.Fatalf("tried %d times", n)
			}
			noToken(t, "the error", err.Error())
			var fresh domain.Alert
			_ = database.Get(ctx, s.Read(), "alerts", a.ID, &fresh)
			if !strings.HasPrefix(fresh.LastError, c.want) {
				t.Fatalf("last error %q", fresh.LastError)
			}
			evs, err := database.Select[domain.Event](ctx, s.ReadEvents(), "events", "kind = ?", domain.EventAlertFailed)
			if err != nil || len(evs) != 1 {
				t.Fatalf("events %v %v", evs, err)
			}
			raw, _ := json.Marshal(evs[0])
			noToken(t, "the event", string(raw))
		})
	}
}

func TestTelegramWaitsWhenAsked(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	got := fakeTelegram(t, func(n int) (int, string) {
		if n <= 2 {
			return 429, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`
		}
		return 200, `{"ok":true,"result":{}}`
	})
	a, err := Add(ctx, s, v, Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "@holyarrs_test", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// A test is sent once: the 429 comes back as a failure worth trying again.
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{time.Millisecond}}
	err = snd.TestOne(ctx, a, Message{Moment: MomentTest, Subject: "HolyArrs test", Text: "hi"})
	if err == nil || !retryable(err) || err.Error() != "Telegram asked HolyArrs to wait 1 s" || askedWait(err) != time.Second {
		t.Fatalf("err %v", err)
	}
	// SendOne waits at least what Telegram asked for, then sends.
	start := time.Now()
	if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"}); err != nil {
		t.Fatal(err)
	}
	if n := len(got()); n != 3 {
		t.Fatalf("calls %d", n)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("did not wait the asked second: %v", time.Since(start))
	}
}

func TestTelegramServerErrorIsTriedAgain(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	got := fakeTelegram(t, func(n int) (int, string) {
		if n == 1 {
			return 502, `{"ok":false}`
		}
		return 200, `{"ok":true,"result":{}}`
	})
	a := addTelegramVault(t, s, v)
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{time.Millisecond}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"}); err != nil {
		t.Fatal(err)
	}
	if n := len(got()); n != 2 {
		t.Fatalf("calls %d", n)
	}
}

func TestTelegramNoAnswerKeepsTokenOut(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	t.Setenv("HOLYARRS_TELEGRAM_URL", "http://127.0.0.1:1")
	a := addTelegramVault(t, s, v)
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"})
	if err == nil || !retryable(err) {
		t.Fatalf("err %v", err)
	}
	noToken(t, "the error", err.Error())
	var f *failure
	if errors.As(err, &f) && f.detail != nil {
		noToken(t, "the logged detail", f.detail.Error())
	}
}

func TestTelegramSaveChecks(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	bad := []struct {
		in   Input
		want error
	}{
		{Input{Name: "TG", Kind: "telegram", ChatID: "42"}, ErrTelegramToken},
		{Input{Name: "TG", Kind: "telegram", BotToken: "nope", ChatID: "42"}, ErrTelegramToken},
		{Input{Name: "TG", Kind: "telegram", BotToken: tgToken + "/../x", ChatID: "42"}, ErrTelegramToken},
		{Input{Name: "TG", Kind: "telegram", BotToken: tgToken}, ErrTelegramChat},
		{Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "my chat"}, ErrTelegramChat},
		{Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "@ab"}, ErrTelegramChat},
	}
	for _, c := range bad {
		if _, err := Add(ctx, s, v, c.in, domain.System); !errors.Is(err, c.want) || !IsInputError(err) {
			t.Fatalf("%+v: %v", c.in, err)
		}
	}
	for _, chat := range []string{"42", "-1001234567890", "@holyarrs_news"} {
		if CheckTelegramChat(chat) != nil {
			t.Fatalf("chat %q refused", chat)
		}
	}
	a, err := Add(ctx, s, v, Input{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "42"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	was := *a.PasswordSecretID
	// An empty token on change keeps the stored one; the chat can change.
	b, err := Change(ctx, s, v, a.ID, Input{Name: "TG", ChatID: "-42"}, domain.System)
	if err != nil || b.ChatID != "-42" || b.PasswordSecretID == nil || *b.PasswordSecretID != was {
		t.Fatalf("change %+v %v", b, err)
	}
	if got, _ := v.Get(ctx, *b.PasswordSecretID); got != tgToken {
		t.Fatal("stored token changed")
	}
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "TG", BotToken: "nope", ChatID: "-42"}, domain.System); !errors.Is(err, ErrTelegramToken) {
		t.Fatalf("bad new token: %v", err)
	}
	if err := Remove(ctx, s, a.ID); err != nil {
		t.Fatal(err)
	}
}
