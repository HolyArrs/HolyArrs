package source

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// A site's bot check (#106): Cloudflare or DDoS-Guard in front of a Source
// answers with a check page meant for a browser, not the feed. HolyArrs
// cannot pass it (it never runs a browser or solves a CAPTCHA), so it says
// so plainly instead of calling the key refused, which a 403 alone would
// mean. A site that bans the address outright is told apart, since nothing
// on the HolyArrs side fixes that. The kind is kept in the Source's extra
// ("block") while it is paused (research/integrations/cloudflare-proxies.md).
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
	// Words only a check page shows; the body is read in lower case. Not the
	// bare /cdn-cgi/challenge-platform/ path or a Turnstile widget id:
	// Cloudflare puts those on ordinary pages too, a site's own error page
	// among them.
	checkPage = regexp.MustCompile(`<title>\s*(just a moment|ddos-guard|please wait)|/cdn-cgi/challenge-platform/h/|_cf_chl_opt|checking your browser|/\.well-known/ddos-guard/`)
	// Cloudflare's "access denied" and firewall pages: 1005 to 1012 and 1020
	// are bans, not checks, and so is "you have been blocked".
	banPage = regexp.MustCompile(`error code:?\s*(100[5-9]|101[0-2]|1020)\b|<title>\s*access denied|sorry, you have been blocked`)
)

// botCheck says whether an answer is a site's bot check or ban page, and which.
func botCheck(resp *http.Response, body []byte) string {
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Cf-Mitigated")), "challenge") {
		return BlockBotCheck
	}
	server := strings.ToLower(resp.Header.Get("Server"))
	if !strings.Contains(server, "cloudflare") && !strings.Contains(server, "ddos-guard") {
		return ""
	}
	// A 429 is a rate limit unless the header above said challenge; only an
	// HTML page can be a check page, so a feed's own XML or JSON error stays one.
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable ||
		!strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") {
		return ""
	}
	low := bytes.ToLower(body[:min(len(body), 64<<10)])
	switch {
	case banPage.Match(low):
		return BlockSiteBan
	case checkPage.Match(low):
		return BlockBotCheck
	}
	return ""
}

// BlockOf is the kind of block a paused Source is under, or "".
func BlockOf(src domain.Source) string {
	if src.PauseReason == nil || *src.PauseReason != KindBlocked {
		return ""
	}
	if b := src.Extra["block"]; blockText[b] != "" {
		return b
	}
	return ""
}
