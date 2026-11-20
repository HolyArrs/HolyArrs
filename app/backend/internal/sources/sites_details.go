package sources

import (
	"context"
	"errors"
	"fmt"
	htmlesc "html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Some built-in sites (1337x, Torrent9) list only a page per torrent; its
// magnet is on that page. Reading every page during a search would cost one
// call per result, so the page is read once, when a Release is grabbed.

var pageMagnet = regexp.MustCompile(`href="(magnet:\?xt=urn:btih:[^"]+)"`)

// detailsLink is the magnet for a Release whose link is its site's page, or
// the link as it is for every other Release.
func detailsLink(ctx context.Context, s *database.Store, sec SecretReader, rel domain.Release) (string, error) {
	if strings.HasPrefix(rel.LinkRef, "magnet:") {
		return rel.LinkRef, nil
	}
	var src domain.Source
	if err := database.Get(ctx, s.Read(), "sources", rel.SourceID, &src); errors.Is(err, database.ErrNotFound) {
		// A Source removed since the search: the link goes as it was stored.
		return rel.LinkRef, nil
	} else if err != nil {
		return "", fmt.Errorf("the Release's Source: %w", err)
	}
	site, ok := SiteByID(src.Extra[extraSiteKind])
	if src.Format != "site" || !ok || !site.Details {
		return rel.LinkRef, nil
	}
	cfg, err := ConfigFor(ctx, src, sec)
	if err != nil {
		return "", err
	}
	return New(cfg).pageMagnet(ctx, rel.LinkRef)
}

// pageMagnet reads the magnet off a torrent's page on the Source's own site.
func (c *Client) pageMagnet(ctx context.Context, link string) (string, error) {
	u, err := url.Parse(link)
	base, berr := url.Parse(c.cfg.URL)
	if err != nil || berr != nil || !strings.EqualFold(u.Host, base.Host) {
		// The page must be on the site the Source names, so a stored link can never send HolyArrs elsewhere.
		return "", &Error{Kind: KindBroken, Msg: "the Release's page is not on its Source's site"}
	}
	if err := c.wait(ctx); err != nil {
		return "", err
	}
	page, err := c.send(ctx, http.MethodGet, link, nil, nil)
	if err != nil {
		return "", err
	}
	m := pageMagnet.FindSubmatch(page)
	if m == nil {
		return "", &Error{Kind: KindBroken, Msg: "the site's page for this Release has no magnet link"}
	}
	magnet := htmlesc.UnescapeString(string(m[1]))
	if mu, err := url.Parse(magnet); err != nil || magnetHash(mu.Query().Get("xt")) == "" {
		return "", &Error{Kind: KindBroken, Msg: "the site's page for this Release has no usable magnet link"}
	}
	return magnet, nil
}
