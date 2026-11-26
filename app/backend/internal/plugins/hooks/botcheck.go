package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// BotcheckProxy is the Source's own proxy, so the solver's browser opens
// the page from the same address as every other call to that Source.
type BotcheckProxy struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// BotcheckAsk is one page a bot-check plugin opens (op "open"): a plain GET
// of URL, which can hold the Source's key.
type BotcheckAsk struct {
	URL   string         `json:"url"`
	Proxy *BotcheckProxy `json:"proxy,omitempty"`
}

// BotcheckCookie is one cookie the solver's browser got.
type BotcheckCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BotcheckAnswer is the page the solver ended on, with what passed the
// check: its cookies and the browser's User-Agent.
type BotcheckAnswer struct {
	URL       string           `json:"url"`
	Status    int              `json:"status"`
	Body      string           `json:"body"`
	Cookies   []BotcheckCookie `json:"cookies,omitempty"`
	UserAgent string           `json:"user_agent,omitempty"`
}

const (
	botcheckWait    = 75 * time.Second
	botcheckCookies = 50
)

// SolveBotcheck hands plugin one page to open and checks the answer's
// shape. Whether the page it ended on may be used is the caller's call.
func SolveBotcheck(ctx context.Context, b *Broker, plugin string, ask BotcheckAsk) (BotcheckAnswer, error) {
	raw, err := b.Call(ctx, plugin, Botcheck, "open", ask, botcheckWait)
	if err != nil {
		return BotcheckAnswer{}, err
	}
	var a BotcheckAnswer
	if err := json.Unmarshal(raw, &a); err != nil || a.Status < 0 || a.Status > 599 || a.Status > 0 && a.Status < 100 {
		return BotcheckAnswer{}, errors.New("the plugin's answer could not be read")
	}
	if len(a.Body) > feedMax {
		return BotcheckAnswer{}, errors.New("the plugin's answer is too large")
	}
	if len(a.UserAgent) > 512 || strings.ContainsAny(a.UserAgent, "\r\n\x00") {
		a.UserAgent = ""
	}
	a.URL = clip(a.URL, 4096)
	a.Cookies = a.Cookies[:min(len(a.Cookies), botcheckCookies)]
	return a, nil
}
