package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
)

// jackettKeyShape is Jackett's API key: 32 lowercase letters and digits.
var jackettKeyShape = regexp.MustCompile(`^[a-z0-9]{32}$`)

// jackettKey reads Jackett's own settings call, which its dashboard uses and
// which answers without a sign-in only when Jackett has no admin password.
// No login is ever sent: with a password set, Jackett keeps its key and the
// owner copies it from the dashboard.
func (k *keyAsker) jackettKey(ctx context.Context) KeyResult {
	// Jackett with no admin password signs a visitor in on its login page in
	// two steps: a test cookie first, then its own cookie once the page sees
	// the test one come back. Its settings call only answers with that cookie.
	if st, _ := k.do(ctx, http.MethodGet, "/UI/Login", "", nil, nil, 64<<10); st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	k.do(ctx, http.MethodGet, "/UI/Login?cookiesChecked=1", "", nil, nil, 64<<10)
	st, b := k.do(ctx, http.MethodGet, "/api/v2.0/server/config", "", nil, nil, 64<<10)
	if st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	var c struct {
		APIKey string `json:"api_key"`
	}
	if st == http.StatusOK && json.Unmarshal(b, &c) == nil && jackettKeyShape.MatchString(c.APIKey) {
		return KeyResult{Key: c.APIKey}
	}
	return KeyResult{}
}
