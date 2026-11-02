package musictrend

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

// Public, keyless sources.
const (
	ListenBrainzURL = "https://api.listenbrainz.org/1/stats/sitewide"
	DeezerURL       = "https://api.deezer.com"
)

// Entry is one charted artist or album before matching. MBID is set when
// the source already knows the MusicBrainz id (ListenBrainz).
type Entry struct {
	MBID       string
	Name       string // artist name, or album title
	Artist     string // album artist (albums only)
	ArtistMBID string
	CAARelease string // a release with Cover Art Archive art (albums only)
	Listens    int64
	// Cover is a ready picture link from the chart source: the album cover,
	// or the artist picture for artists.
	Cover string
	// Date, Explicit and Genres come from Apple Music charts (Genres
	// without the catch-all "Music").
	Date     string
	Explicit bool
	Genres   []string
	// Type is "ep" or "single" when the source's title said so.
	Type string
	// AppleArtist is the artist's Apple Music id and UPC the album's
	// barcode, as Apple's chart row gives them (rows kept before carry
	// neither; EntryBarcode then reads the cover's file name).
	AppleArtist string `json:",omitempty"`
	// AppleID and AppleKind ("albums", "songs") are the row's Apple Music
	// catalog item; its artist is asked once (fillAppleArtists).
	AppleID   string `json:",omitempty"`
	AppleKind string `json:",omitempty"`
	UPC       string `json:",omitempty"`
	// Codes are an artist row's album barcodes (RankArtists, up to 3).
	Codes []string `json:",omitempty"`
	// Unlinked: no exact evidence and a name several artists carry, so the
	// row links to no artist (shown as plain text). On an artist row that
	// is the row itself (artistKind); an album or song row still needs its
	// own id.
	Unlinked bool `json:"-"`
}

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validMBID(s string) bool { return mbidRe.MatchString(s) }

// ParseLBArtists reads the ListenBrainz sitewide top artists answer.
func ParseLBArtists(r io.Reader) ([]Entry, int64, error) {
	var body struct {
		Payload struct {
			LastUpdated int64 `json:"last_updated"`
			Artists     []struct {
				MBID    string `json:"artist_mbid"`
				Name    string `json:"artist_name"`
				Listens int64  `json:"listen_count"`
			} `json:"artists"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	var out []Entry
	for _, a := range body.Payload.Artists {
		id := strings.ToLower(a.MBID)
		if !validMBID(id) || seen[id] || a.Name == "" {
			continue
		}
		seen[id] = true
		out = append(out, Entry{MBID: id, Name: a.Name, Listens: a.Listens})
	}
	if len(out) == 0 {
		return nil, 0, errors.New("musictrend: no ListenBrainz artists")
	}
	return out, body.Payload.LastUpdated, nil
}

// ParseLBAlbums reads the ListenBrainz sitewide top release groups answer.
func ParseLBAlbums(r io.Reader) ([]Entry, int64, error) {
	var body struct {
		Payload struct {
			LastUpdated   int64 `json:"last_updated"`
			ReleaseGroups []struct {
				MBID        string   `json:"release_group_mbid"`
				Name        string   `json:"release_group_name"`
				Artist      string   `json:"artist_name"`
				ArtistMBIDs []string `json:"artist_mbids"`
				CAARelease  string   `json:"caa_release_mbid"`
				Listens     int64    `json:"listen_count"`
			} `json:"release_groups"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	var out []Entry
	for _, g := range body.Payload.ReleaseGroups {
		id := strings.ToLower(g.MBID)
		if !validMBID(id) || seen[id] || g.Name == "" {
			continue
		}
		seen[id] = true
		e := Entry{MBID: id, Name: g.Name, Artist: g.Artist, Listens: g.Listens}
		if len(g.ArtistMBIDs) > 0 && validMBID(strings.ToLower(g.ArtistMBIDs[0])) {
			e.ArtistMBID = strings.ToLower(g.ArtistMBIDs[0])
		}
		if validMBID(strings.ToLower(g.CAARelease)) {
			e.CAARelease = strings.ToLower(g.CAARelease)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, 0, errors.New("musictrend: no ListenBrainz release groups")
	}
	return out, body.Payload.LastUpdated, nil
}

// countries maps country names to ISO 3166-1 codes; the codes are the
// Apple Music storefronts the charts are kept for.
var countries = map[string]string{
	"Algeria": "DZ", "Argentina": "AR", "Australia": "AU", "Austria": "AT", "Belgium": "BE", "Bolivia": "BO",
	"Brazil": "BR", "Bulgaria": "BG", "Canada": "CA", "Chile": "CL", "Colombia": "CO", "Costa Rica": "CR",
	"Croatia": "HR", "Czech Republic": "CZ", "Denmark": "DK", "Ecuador": "EC", "Egypt": "EG", "El Salvador": "SV",
	"Estonia": "EE", "Finland": "FI", "France": "FR", "Germany": "DE", "Guatemala": "GT", "Honduras": "HN",
	"Hungary": "HU", "Indonesia": "ID", "Ireland": "IE", "Israel": "IL", "Italy": "IT", "Ivory Coast": "CI",
	"Jamaica": "JM", "Japan": "JP", "Jordan": "JO", "Kenya": "KE", "Latvia": "LV", "Lebanon": "LB",
	"Lithuania": "LT", "Malaysia": "MY", "Mexico": "MX", "Morocco": "MA", "Netherlands": "NL", "Nigeria": "NG",
	"Norway": "NO", "Paraguay": "PY", "Peru": "PE", "Philippines": "PH", "Poland": "PL", "Portugal": "PT",
	"Romania": "RO", "Russia": "RU", "Saudi Arabia": "SA", "Senegal": "SN", "Serbia": "RS", "Singapore": "SG",
	"Slovakia": "SK", "Slovenia": "SI", "South Africa": "ZA", "South Korea": "KR", "Spain": "ES", "Sweden": "SE",
	"Switzerland": "CH", "Thailand": "TH", "Tunisia": "TN", "Turkey": "TR", "UK": "GB", "Ukraine": "UA",
	"United Arab Emirates": "AE", "USA": "US", "Venezuela": "VE", "New Zealand": "NZ", "India": "IN",
	"Panama": "PA", "Dominican Republic": "DO", "Uruguay": "UY", "Greece": "GR", "Cyprus": "CY",
	"Iceland": "IS", "Luxembourg": "LU", "Ghana": "GH", "Cameroon": "CM", "Mali": "ML",
}
