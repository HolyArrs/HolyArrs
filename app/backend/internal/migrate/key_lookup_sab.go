package migrate

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
)

// sabKeyIn finds the API key field on SABnzbd's General settings page.
var sabKeyIn = regexp.MustCompile(`id="apikey"[^>]*value="([0-9a-fA-F]{32})"|value="([0-9a-fA-F]{32})"[^>]*id="apikey"`)

// sabKey reads SABnzbd's General settings page, which shows the API key.
// With a SABnzbd username and password set, it signs in once with the login
// given (never kept); with none given it asks for one.
func (k *keyAsker) sabKey(ctx context.Context, login KeyLogin) KeyResult {
	st, b := k.do(ctx, http.MethodGet, "/config/general/", "", nil, nil, 512<<10)
	if st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	if st != http.StatusOK || sabKeyIn.Find(b) == nil {
		if login.Username == "" || login.Password == "" {
			return KeyResult{NeedLogin: true}
		}
		form := url.Values{"username": {login.Username}, "password": {login.Password}, "remember_me": {"0"}}
		if st, _ = k.do(ctx, http.MethodPost, "/login/", "application/x-www-form-urlencoded", []byte(form.Encode()), nil, 64<<10); st == 0 {
			return KeyResult{Problem: keyUnreachable}
		}
		if st, b = k.do(ctx, http.MethodGet, "/config/general/", "", nil, nil, 512<<10); st != http.StatusOK {
			return KeyResult{Problem: keyWrongLogin}
		}
	}
	m := sabKeyIn.FindSubmatch(b)
	if m == nil {
		return KeyResult{Problem: keyWrongLogin}
	}
	if len(m[1]) > 0 {
		return KeyResult{Key: string(m[1])}
	}
	return KeyResult{Key: string(m[2])}
}
