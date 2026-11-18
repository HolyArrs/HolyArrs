package metadata

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
)

// tmdbCredential tells TMDB's two credentials apart: a v3 API key is 32 hex
// characters and goes in the query, anything else is a v4 read access token
// and goes in the Authorization header.
func tmdbCredential(tok string) (bearer, apiKey string) {
	t := strings.TrimSpace(tok)
	if len(t) == 32 {
		if _, err := hex.DecodeString(t); err == nil {
			return "", t
		}
	}
	return t, ""
}

// tmdbApply puts the credential where TMDB expects it and returns the bearer
// (empty when the credential went into the query instead). A v3 API key in an
// Authorization header is a 401, which reads as a refused install.
func tmdbApply(u *string, tok string) (bearer string) {
	bearer, apiKey := tmdbCredential(tok)
	if apiKey != "" {
		*u = withQuery(*u, "api_key", apiKey)
	}
	return bearer
}

// withQuery adds one query value to u, keeping the ones already there.
func withQuery(u, key, value string) string {
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + url.Values{key: {value}}.Encode()
}

// setCredential adds the Authorization header, and only for a v4 read token.
func setCredential(req *http.Request, bearer string) {
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
}
