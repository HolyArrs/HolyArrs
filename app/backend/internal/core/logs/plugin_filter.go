package logs

import (
	"regexp"
	"strings"
)

var (
	pluginTok = regexp.MustCompile(`hap_[A-Za-z0-9]+`)
	ipv4      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6      = regexp.MustCompile(`\[?\b(?:[0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\b\]?`)
	email     = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	quoted    = regexp.MustCompile(`"[^"]{1,80}"`)
)

// privateWords mark a line about sign-in, people or keys: a plugin never
// gets such a line at all.
var privateWords = []string{"auth:", "password", "passphrase", "sign-in", "signed in", "signin", "login", "invite", "two-factor",
	"totp", "token", "api key", "key ", "secret", "cookie", "session", "oidc", "owner reset", "user "}

// ForPlugin filters log lines for a plugin with system.logs.read: on top of
// the masking every line already has, lines about sign-in, people, keys and
// secrets are left out, and addresses, e-mail addresses, plugin tokens and
// quoted names are hidden in the rest.
func (l *Log) ForPlugin(lines []Line) []Line {
	out := make([]Line, 0, len(lines))
	for _, ln := range lines {
		low := strings.ToLower(ln.Text)
		skip := false
		for _, w := range privateWords {
			if strings.Contains(low, w) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		t := ln.Text
		if l != nil {
			t = l.redact(t)
		} else {
			t = redactPatterns(t)
		}
		t = pluginTok.ReplaceAllString(t, Hidden)
		t = email.ReplaceAllString(t, Hidden)
		t = ipv4.ReplaceAllString(t, Hidden)
		t = ipv6.ReplaceAllStringFunc(t, func(m string) string {
			if strings.Count(m, ":") >= 2 && strings.ContainsAny(m, "0123456789abcdefABCDEF") {
				return Hidden
			}
			return m
		})
		t = quoted.ReplaceAllString(t, `"`+Hidden+`"`)
		ln.Text = t
		out = append(out, ln)
	}
	return out
}
