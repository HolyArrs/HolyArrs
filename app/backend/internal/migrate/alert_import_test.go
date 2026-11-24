package migrate

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// An imported Alert brings its ntfy or Gotify server (scheme and host only,
// never a user, path or query), the old app's moments, a plain line for
// what it still needs, and one line for the settings HolyArrs has no place
// for. No token comes over and no raw field name is asked for.
func TestAlertImportDetails(t *testing.T) {
	raw := func(name, kind string, set map[string]any, top map[string]any) map[string]any {
		fields := []any{}
		for k, v := range set {
			fields = append(fields, map[string]any{"name": k, "value": v})
		}
		m := map[string]any{"name": name, "implementation": kind, "fields": fields}
		for k, v := range top {
			m[k] = v
		}
		return m
	}
	// Stand-in secrets, put together so no line holds one.
	fake := func(s string) string { return "SECRET" + s }
	var ps []Provider
	for _, m := range []map[string]any{
		raw("Tele", "Telegram", map[string]any{"botToken": "123456:" + fake("BOT"), "chatId": "-1001234567890", "topicId": 7, "sendSilently": true},
			map[string]any{"onGrab": true, "onDownload": true, "onUpgrade": false, "onHealthIssue": false}),
		raw("Ntfy", "Ntfy", map[string]any{"serverUrl": "https://bob:" + fake("PW") + "@ntfy.example.com/sub?x=" + fake("Q"), "topics": []any{"SECRETTOPIC"},
			"priority": 4, "tags": []any{"tv"}, "userName": "bob", "password": fake("PW2"), "accessToken": "tk_" + fake("TK")}, nil),
		raw("Gotify", "Gotify", map[string]any{"server": "http://192.168.1.5:8080", "appToken": fake("APP"), "priority": 5}, map[string]any{"onHealthIssue": true}),
		raw("Meta", "Gotify", map[string]any{"server": "http://169.254.169.254", "appToken": fake("APP2")}, nil),
		raw("Push", "Pushover", map[string]any{"apiKey": fake("API"), "userKey": fake("USER"), "devices": []any{"phone"}, "priority": 2, "sound": "siren"}, nil),
	} {
		ps = append(ps, provider(m, noLabels))
	}
	s := &Snapshot{Kind: Sonarr, Notifications: linkSafe(ps, true)}
	s.finish()
	if slices.ContainsFunc(s.NeedsReentry, func(n string) bool { return strings.Contains(n, "topicId") }) {
		t.Errorf("the Telegram topic is asked for as a key: %v", s.NeedsReentry)
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	b, _ := json.Marshal(p)
	for _, secret := range []string{"SECRET", "bob:", "/sub", "x="} {
		if strings.Contains(string(b), secret) {
			t.Errorf("%s reached the plan: %s", secret, b)
		}
	}
	for _, n := range p.NeedsInput {
		if strings.Contains(n, `notification "`) {
			t.Errorf("a raw field name is asked for: %v", p.NeedsInput)
		}
	}
	for _, c := range []struct {
		key, server string
		local       bool
		needs       string
		moments     []string
	}{
		{"alert:tele", "", false, "its bot token", []string{"grab", "import"}},
		{"alert:ntfy", "https://ntfy.example.com", false, "its topic", nil},
		{"alert:gotify", "http://192.168.1.5:8080", true, "its app token", []string{"health"}},
		{"alert:meta", "", false, "its server and app token", nil},
		{"alert:push", "", false, "its user key and app token", nil},
	} {
		it := item(t, p, GroupAlerts, c.key)
		if it.Alert.ServerURL != c.server || it.Alert.AllowLocal != c.local || it.Needs != c.needs || !slices.Equal(it.Alert.Moments, c.moments) {
			t.Errorf("%s: %+v needs %q", c.key, it.Alert, it.Needs)
		}
	}
	left := map[string]string{}
	for _, n := range p.NotMoved {
		if n.Group == GroupAlerts {
			if _, dup := left[n.What]; dup {
				t.Errorf("two lines for %s", n.What)
			}
			left[n.What] = n.Why
		}
	}
	for name, want := range map[string]string{
		"Tele":   "HolyArrs has no place for: the Telegram topic; sending silently",
		"Ntfy":   "HolyArrs has no place for: the ntfy priority; the ntfy tags; the user name bob (HolyArrs signs in to ntfy with an access token)",
		"Push":   "HolyArrs has no place for: the Pushover devices; the Pushover priority; the Pushover sound",
		"Gotify": "",
	} {
		if left[name] != want {
			t.Errorf("%s left %q, want %q", name, left[name], want)
		}
	}
}
