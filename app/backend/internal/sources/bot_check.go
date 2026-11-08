package sources

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A site's bot check is a page meant for a browser, which Cloudflare or DDoS-Guard puts in front of a Source
// in place of the feed. HolyArrs never runs a browser or solves a CAPTCHA, so it says plainly that it
// is blocked instead of calling the key refused, which a bare 403 would suggest.
// A site that bans the address outright is told apart, because nothing on the HolyArrs side fixes it.
// The kind is kept in the Source's extra under "block" while the Source is paused.
const (
	BlockBotCheck = "bot_check"
	BlockSiteBan  = "site_ban"
)

// blockText says what each kind of block means for the person.
var blockText = map[string]string{
	BlockBotCheck: "Blocked by the site's bot check. Reach this site through Jackett or Prowlarr.",
	BlockSiteBan:  "The site's protection refuses HolyArrs's address. Changing anything in HolyArrs does not help; the site has to allow it.",
}

var (
	// checkPage matches words that only a check page shows, in a lower-cased body.
	// The bare challenge-platform path and the Turnstile widget id are left out, because Cloudflare puts
	// them on ordinary pages too, including a site's own error page.
	checkPage = regexp.MustCompile(`<title>\s*(just a moment|ddos-guard|please wait)|/cdn-cgi/challenge-platform/h/|_cf_chl_opt|checking your browser|/\.well-known/ddos-guard/`)
	// banPage matches Cloudflare's access denied and firewall pages.
	// Error codes 1005 to 1012 and 1020 are bans, not checks, and so is "you have been blocked".
	banPage = regexp.MustCompile(`error code:?\s*(100[5-9]|101[0-2]|1020)\b|<title>\s*access denied|sorry, you have been blocked`)
)

// BlockOf returns the kind of block a paused Source is under, or "".
func BlockOf(src domain.Source) string {
	if src.PauseReason == nil || *src.PauseReason != KindBlocked {
		return ""
	}
	if b := src.Extra["block"]; blockText[b] != "" {
		return b
	}
	return ""
}

// botCheck says whether an answer is a site's bot check or ban page, and which.
func botCheck(resp *http.Response, body []byte) string {
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Cf-Mitigated")), "challenge") {
		return BlockBotCheck
	}
	server := strings.ToLower(resp.Header.Get("Server"))
	if !strings.Contains(server, "cloudflare") && !strings.Contains(server, "ddos-guard") {
		return ""
	}
	// A 429 is a rate limit unless the header said challenge.
	// Only an HTML page can be a check page, so a feed's own XML or JSON error stays what it is.
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable ||
		!strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") {
		return ""
	}
	// Only the first 64 KiB is read, since a check page puts its markers near the top.
	low := bytes.ToLower(body[:min(len(body), 64<<10)])
	switch {
	case banPage.Match(low):
		return BlockSiteBan
	case checkPage.Match(low):
		return BlockBotCheck
	}
	return ""
}
