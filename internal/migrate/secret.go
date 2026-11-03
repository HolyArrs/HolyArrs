package migrate

import (
	"net/url"
	"strings"
)

// secretWords mark a setting name as a secret. A name holding one of them
// is never copied: it goes to NeedsReentry.
var secretWords = []string{
	"key", "pass", "token", "secret", "cookie", "auth", "hash", "session",
	"credential", "salt", "pin", "signature", "private", "guid", "webhook",
}

// plainNames are names that hold a secret word but carry no secret.
var plainNames = map[string]bool{
	"keyword": true, "keywords": true, "authtype": true, "authenticationmethod": true,
	"authentication": true, "pinned": true, "pinyin": true, "monkeys": true,
	// authuser is the Trakt account name a Radarr Trakt list signed in as.
	"authuser": true,
}

// IsSecretName reports whether a setting called name may hold a secret.
func IsSecretName(name string) bool {
	n := strings.ToLower(name)
	if i := strings.LastIndexAny(n, ".\\/"); i >= 0 {
		n = n[i+1:]
	}
	n = strings.NewReplacer("_", "", "-", "").Replace(n)
	if plainNames[n] {
		return false
	}
	for _, w := range secretWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// masked is the arr apps' mask for a secret they will not send.
const masked = "********"

// plainSettings keeps the plain values of a settings map: strings, numbers,
// booleans and lists of those. A secret-named setting with a value is left
// behind and its name returned; addresses lose any user, password and
// secret-named query value they carry.
func plainSettings(in map[string]any) (map[string]any, []string) {
	out := map[string]any{}
	var gone []string
	for k, v := range in {
		// An on/off setting holds no secret, whatever its name says
		// ("rejectBlocklistedTorrentHashes..." only names a switch).
		if _, isBool := v.(bool); IsSecretName(k) && !isBool {
			if !empty(v) {
				gone = append(gone, k)
			}
			continue
		}
		if sv, ok := v.(string); ok && otherScheme(sv) {
			gone = append(gone, k) // apprise-style links carry their tokens
			continue
		}
		if pv, ok := plainValue(normalize(v)); ok {
			out[k] = pv
		}
	}
	if len(out) == 0 {
		out = nil
	}
	return out, noteList(gone)
}

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// plainValue keeps scalars and lists of scalars; strings are cleaned.
func plainValue(v any) (any, bool) {
	switch x := v.(type) {
	case string:
		if x == masked {
			return nil, false
		}
		return cleanString(x), true
	case bool, float64, int, int64:
		return x, true
	case []any:
		var out []any
		for _, e := range x {
			switch e.(type) {
			case string, bool, float64, int, int64:
				pv, _ := plainValue(e)
				out = append(out, pv)
			default:
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// otherScheme reports a link that is not http or https (such links,
// like discord://id/token, carry their secrets in the path).
func otherScheme(s string) bool {
	i := strings.Index(s, "://")
	if i <= 0 {
		return false
	}
	scheme := strings.ToLower(s[:i])
	return scheme != "http" && scheme != "https"
}

// cleanString strips the user, password and secret-named query values
// from an address; any other string stays as it is.
func cleanString(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "(address left behind)"
	}
	u.User = nil
	q := u.Query()
	changed := false
	for k := range q {
		if IsSecretName(k) {
			q.Del(k)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	u.Fragment = ""
	return u.String()
}

// linkSafe treats every link in indexer and notification settings as a
// secret: tokens ride in link paths (webhook addresses, ntfy and Gotify
// servers and topics, RSS feeds with a passkey in the path) and in
// free-text extra parameters, where no name says so. An indexer's base
// address is the one link kept, cut to its scheme, host and API root (the
// path up to the first part that looks like a token, no query); the rest,
// with ntfy topics and "additionalParameters", is dropped and named under
// needs re-entry. notification says the providers are notifications, whose
// links are all dropped.
func linkSafe(ps []Provider, notification bool) []Provider {
	for i := range ps {
		p := &ps[i]
		for k, v := range p.Settings {
			base := strings.TrimPrefix(strings.ToLower(k), "link.")
			drop := base == "additionalparameters" || notification && strings.Contains(base, "topic")
			if !drop && hasLink(v) {
				if !notification && base == "baseurl" {
					sv, _ := v.(string)
					if root, cut := apiRoot(sv); root != "" {
						p.Settings[k] = root
						if cut {
							p.NeedsReentry = append(p.NeedsReentry, k)
						}
						continue
					}
				}
				drop = true
			}
			if drop {
				delete(p.Settings, k)
				if !empty(v) {
					p.NeedsReentry = append(p.NeedsReentry, k)
				}
			}
		}
		if len(p.Settings) == 0 {
			p.Settings = nil
		}
		p.NeedsReentry = noteList(p.NeedsReentry)
	}
	return ps
}

// hasLink reports a string, or a list holding a string, with a link in it.
func hasLink(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.Contains(x, "://")
	case []any:
		for _, e := range x {
			if hasLink(e) {
				return true
			}
		}
	}
	return false
}

// apiRoot cuts an http(s) address to scheme, host and the path parts before
// the first one that looks like a token; cut says something was left out.
func apiRoot(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	cut := u.User != nil || u.RawQuery != "" || u.Fragment != ""
	var keep []string
	parts := strings.Split(u.EscapedPath(), "/")
	for i, part := range parts {
		if tokenLike(part) {
			cut = true
			break
		}
		if i > 0 || part != "" {
			keep = append(keep, part)
		}
	}
	out := u.Scheme + "://" + u.Host
	if len(keep) > 0 {
		out += "/" + strings.Join(keep, "/")
	}
	if cut {
		out = strings.TrimSuffix(out, "/")
	}
	return out, cut
}

// tokenLike says a path part looks like a key: long, and letters mixed
// with digits, or very long.
func tokenLike(s string) bool {
	if len(s) < 16 {
		return false
	}
	letters, digits := false, false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			letters = true
		}
	}
	return letters && digits || len(s) >= 32
}

// fieldsToMap turns the arr apps' fields list ([{name, value}]) into a map.
func fieldsToMap(fields []any) map[string]any {
	m := map[string]any{}
	for _, f := range fields {
		fm, _ := f.(map[string]any)
		name, _ := fm["name"].(string)
		if name == "" {
			continue
		}
		// A field the app marks as private is a secret whatever its name.
		if p, _ := fm["privacy"].(string); p != "" && p != "normal" {
			if !empty(fm["value"]) {
				m["\x00"+name] = fm["value"]
			}
			continue
		}
		m[name] = fm["value"]
	}
	return m
}

// providerSettings splits a settings map into plain settings and the names
// of secrets left behind (including private fields from fieldsToMap).
func providerSettings(in map[string]any) (map[string]any, []string) {
	var forced []string
	rest := map[string]any{}
	for k, v := range in {
		if strings.HasPrefix(k, "\x00") {
			forced = append(forced, k[1:])
			continue
		}
		rest[k] = v
	}
	out, gone := plainSettings(rest)
	return out, noteList(append(gone, forced...))
}
