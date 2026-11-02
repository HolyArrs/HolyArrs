package imgstore

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Movie and TV pictures stay TMDB links; the store keeps only what TMDB
// does not serve: fanart.tv's logos, banners and a wide background for a
// title (by "movie-{tmdb id}" or "tv-{tmdb id}"), and Wikimedia Commons
// photos of people TMDB has none of (by TMDB person id), with the photo's
// author and license recorded (credits go on the repository).
const (
	KindTitleLogo   = "title-logo"   // PNG as it comes
	KindTitleBanner = "title-banner" // WebP at its own size (fanart.tv: 1000 by 185)
	KindTitleWide   = "title-wide"   // WebP, 1600 wide at most
	KindPerson      = "person"       // WebP 500 and big
)

// Slots of the title pictures.
const (
	SlotBanner    = "banner"
	SlotTitleWide = "1600"
)

var (
	titleID  = regexp.MustCompile(`^(movie|tv)-[1-9][0-9]{0,9}$`)
	personID = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
)

// ValidID reports an ID of the kind's own form: a MusicBrainz ID for
// music, movie-{n} or tv-{n} for titles, a TMDB person id for people.
func ValidID(kind, id string) bool {
	switch kind {
	case KindArtist, KindAlbum, KindWide, KindLogo:
		return ValidMBID(id)
	case KindTitleLogo, KindTitleBanner, KindTitleWide:
		return titleID.MatchString(id)
	case KindPerson:
		return personID.MatchString(id)
	}
	return false
}

// TitleKind answers a title's fanart.tv kind and TMDB id from a title
// picture's ID ("movie-603": movie, 603).
func TitleKind(id string) (kind, tmdb string) {
	kind, tmdb, _ = strings.Cut(id, "-")
	return kind, tmdb
}

// titleArt reads fanart.tv's art for a title (the extras.Art JSON).
func (r *Resolver) titleArt(ctx context.Context, id, kind string) ([]Candidate, error) {
	if r.TitleFanart == nil {
		return nil, nil
	}
	fk, tmdb := TitleKind(id)
	b, err := r.TitleFanart(ctx, fk, tmdb)
	if errors.Is(err, ErrNoPicture) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d struct {
		Logo        string   `json:"logo"`
		Banner      string   `json:"banner"`
		Backgrounds []string `json:"backgrounds"`
	}
	_ = json.Unmarshal(b, &d)
	var u []string
	switch kind {
	case KindTitleLogo:
		u = []string{d.Logo}
	case KindTitleBanner:
		u = []string{d.Banner}
	case KindTitleWide:
		u = d.Backgrounds
	}
	var out []Candidate
	for _, x := range u {
		if strings.HasPrefix(x, "https://") || strings.HasPrefix(x, "http://") {
			out = append(out, Candidate{Provider: "fanart", URL: x})
		}
	}
	return out, nil
}

// commonsThumb matches a Wikimedia Commons thumbnail address: the file
// name, escaped, is its fourth path part.
var commonsThumb = regexp.MustCompile(`^https://upload\.wikimedia\.org/wikipedia/commons/thumb/[0-9a-f]/[0-9a-f]{2}/([^/]+)/\d+px-`)

// CommonsFile answers the file name in a Commons thumbnail address.
func CommonsFile(thumb string) string {
	m := commonsThumb.FindStringSubmatch(thumb)
	if m == nil {
		return ""
	}
	name, err := url.PathUnescape(m[1])
	if err != nil {
		return ""
	}
	return name
}

// personPhoto is a person's Commons photo at 500 and 960 (widths Commons
// serves), with its author and license, re-checked monthly (a deleted file
// answers 404 and is dropped).
func (r *Resolver) personPhoto(ctx context.Context, id string) ([]Candidate, error) {
	if r.PersonPhoto == nil {
		return nil, nil
	}
	file, err := r.PersonPhoto(ctx, id)
	if err != nil || file == "" {
		return nil, err
	}
	q := url.Values{"action": {"query"}, "format": {"json"}, "prop": {"imageinfo"}, "iiprop": {"url|extmetadata"},
		"iiurlwidth": {"500"}, "iiextmetadatafilter": {"Artist|LicenseShortName"}, "titles": {"File:" + file}}
	b, err := r.call(ctx, "wikimedia", newReq(base(r.CommonsAPI, "https://commons.wikimedia.org/w/api.php")+"?"+q.Encode()))
	if err != nil || b == nil {
		return nil, err
	}
	var d struct {
		Query struct {
			Pages map[string]struct {
				Missing   *string `json:"missing"`
				ImageInfo []struct {
					Thumb string `json:"thumburl"`
					Meta  map[string]struct {
						Value string `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	for _, p := range d.Query.Pages {
		if p.Missing != nil || len(p.ImageInfo) == 0 || p.ImageInfo[0].Thumb == "" {
			continue
		}
		ii := p.ImageInfo[0]
		c := Candidate{Provider: "wikimedia", URL: ii.Thumb, License: plainText(ii.Meta["LicenseShortName"].Value),
			Author: plainText(ii.Meta["Artist"].Value), Recheck: ShortRecheck}
		if strings.Contains(ii.Thumb, "/500px-") {
			c.Big = strings.Replace(ii.Thumb, "/500px-", "/960px-", 1)
		}
		return []Candidate{c}, nil
	}
	return nil, nil
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// plainText is Commons metadata without its HTML (author lines are links).
func plainText(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&quot;", "\"", "&#039;", "'", "&lt;", "<", "&gt;", ">", "&nbsp;", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
