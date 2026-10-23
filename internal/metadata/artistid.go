package metadata

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ArtistIDs are an artist's ids at other sites, read from its MusicBrainz
// links. They tie a picture to the artist itself: two artists can share a
// name, never an id.
type ArtistIDs struct {
	Deezer   int64  `json:"deezer,omitempty"`
	Wikidata string `json:"wikidata,omitempty"` // Q and digits
}

// ArtistIDSource reads an artist's ids at other sites by MusicBrainz id.
type ArtistIDSource interface {
	ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error)
}

var (
	deezerArtistURL = regexp.MustCompile(`^https?://(?:www\.)?deezer\.com/(?:[a-z]{2}/)?artist/(\d+)/?$`)
	wikidataURL     = regexp.MustCompile(`^https?://(?:www\.)?wikidata\.org/wiki/(Q\d+)$`)
)

// parseArtistURLs reads the Deezer and Wikidata ids from an artist's links.
func parseArtistURLs(links []string) ArtistIDs {
	var ids ArtistIDs
	for _, l := range links {
		l = strings.TrimSpace(l)
		if m := deezerArtistURL.FindStringSubmatch(l); m != nil && ids.Deezer == 0 {
			ids.Deezer, _ = strconv.ParseInt(m[1], 10, 64)
		}
		if m := wikidataURL.FindStringSubmatch(l); m != nil && ids.Wikidata == "" {
			ids.Wikidata = m[1]
		}
	}
	return ids
}

// ArtistIDs reads an artist's Deezer and Wikidata ids from its MusicBrainz
// links.
func (m *MusicBrainz) ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error) {
	if !ValidMBID(mbid) {
		return ArtistIDs{}, ErrArtistNotFound
	}
	b, err := m.get(ctx, "/ws/2/artist/"+strings.ToLower(mbid), url.Values{"inc": {"url-rels"}})
	if err != nil {
		return ArtistIDs{}, err
	}
	var a struct {
		Relations []struct {
			URL struct {
				Resource string `json:"resource"`
			} `json:"url"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return ArtistIDs{}, fmt.Errorf("MusicBrainz: odd answer")
	}
	links := make([]string, 0, len(a.Relations))
	for _, r := range a.Relations {
		links = append(links, r.URL.Resource)
	}
	return parseArtistURLs(links), nil
}

// ArtistIDs asks the direct MusicBrainz client, when it can.
func (m *serviceMusic) ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error) {
	g, ok := m.direct.(ArtistIDSource)
	if !ok {
		return ArtistIDs{}, ErrArtistNotFound
	}
	return g.ArtistIDs(ctx, mbid)
}

// deezerPicture is a usable picture link of a Deezer artist ("" for the
// empty picture Deezer gives an artist without one).
func deezerPicture(list ...string) string {
	for _, p := range list {
		if p != "" && !strings.Contains(p, "/artist//") {
			return p
		}
	}
	return ""
}

// DeezerPicture is the picture of the Deezer artist with this id.
func (d *ArtistImages) DeezerPicture(ctx context.Context, id int64) (string, error) {
	if d == nil || id <= 0 {
		return "", nil
	}
	var a struct {
		ID     int64  `json:"id"`
		XL     string `json:"picture_xl"`
		Big    string `json:"picture_big"`
		Medium string `json:"picture_medium"`
	}
	if err := d.json(ctx, d.base()+"/artist/"+strconv.FormatInt(id, 10), &a); err != nil {
		return "", fmt.Errorf("Deezer artist: %w", err)
	}
	if a.ID != id {
		return "", nil
	}
	return deezerPicture(a.XL, a.Big, a.Medium), nil
}

// WikidataBase is Wikidata's API; CommonsThumbBase serves its pictures.
const (
	WikidataBase     = "https://www.wikidata.org/w/api.php"
	CommonsThumbBase = "https://upload.wikimedia.org/wikipedia/commons/thumb"
)

var (
	wdOnce sync.Once
	wdGet  *getter
)

// WikidataPicture is the picture (P18) of the Wikidata item, as a 600
// pixel wide Wikimedia Commons thumbnail ("" when it has none).
func (d *ArtistImages) WikidataPicture(ctx context.Context, qid string) (string, error) {
	if d == nil || !regexp.MustCompile(`^Q\d+$`).MatchString(qid) {
		return "", nil
	}
	wdOnce.Do(func() {
		wdGet = &getter{name: "Wikidata", http: d.HTTP, timeout: d.Timeout, gap: 200 * time.Millisecond, ttl: 7 * 24 * time.Hour, agent: d.UserAgent}
	})
	base := WikidataBase
	if d.WikidataBase != "" {
		base = strings.TrimRight(d.WikidataBase, "/")
	}
	u := base + "?" + url.Values{"action": {"wbgetclaims"}, "entity": {qid}, "property": {"P18"}, "format": {"json"}}.Encode()
	b, err := wdGet.do(ctx, []string{base}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	if err != nil {
		return "", fmt.Errorf("Wikidata: %w", err)
	}
	var out struct {
		Claims map[string][]struct {
			Rank     string `json:"rank"`
			Mainsnak struct {
				Datavalue struct {
					Value any `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("Wikidata: odd answer")
	}
	for _, c := range out.Claims["P18"] {
		if c.Rank == "deprecated" {
			continue
		}
		if file, ok := c.Mainsnak.Datavalue.Value.(string); ok {
			if t := commonsThumb(file, 600); t != "" {
				return t, nil
			}
		}
	}
	return "", nil
}

// commonsThumb is the Wikimedia Commons thumbnail link of a file (photos
// only: jpg, png, webp).
func commonsThumb(file string, width int) string {
	file = strings.ReplaceAll(strings.TrimSpace(file), " ", "_")
	low := strings.ToLower(file)
	if file == "" || !(strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg") || strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".webp")) {
		return ""
	}
	sum := md5.Sum([]byte(file))
	h := hex.EncodeToString(sum[:])
	enc := url.PathEscape(file)
	return CommonsThumbBase + "/" + h[:1] + "/" + h[:2] + "/" + enc + "/" + strconv.Itoa(width) + "px-" + enc
}

// PictureByIDs is the artist's picture from its own ids: Deezer's artist,
// else Wikidata's picture. Never a name search, never an album cover.
func (d *ArtistImages) PictureByIDs(ctx context.Context, ids ArtistIDs) string {
	if d == nil {
		return ""
	}
	if p, _ := d.DeezerPicture(ctx, ids.Deezer); p != "" {
		return p
	}
	p, _ := d.WikidataPicture(ctx, ids.Wikidata)
	return p
}

// ProfileFor reads the Deezer profile of the artist: by the Deezer id from
// its MusicBrainz links when known, else by a name search that needs the
// exact name and at least two album titles shared with MusicBrainz (one
// when MusicBrainz lists fewer than three). A zero ID means no sure match.
func (d *ArtistImages) ProfileFor(ctx context.Context, name string, deezerID int64, albums []string) (DeezerProfile, error) {
	if d == nil {
		return DeezerProfile{}, nil
	}
	if deezerID > 0 {
		return d.profileByID(ctx, deezerID)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return DeezerProfile{}, nil
	}
	var found struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := d.json(ctx, d.base()+"/search/artist?"+url.Values{"q": {name}, "limit": {"10"}}.Encode(), &found); err != nil {
		return DeezerProfile{}, fmt.Errorf("Deezer artist search: %w", err)
	}
	mine := map[string]bool{}
	for _, a := range albums {
		if k := SongKey(a); k != "" {
			mine[k] = true
		}
	}
	need := 2
	if len(mine) < 3 {
		need = 1
	}
	if len(mine) == 0 {
		return DeezerProfile{}, nil
	}
	tried := 0
	for _, a := range found.Data {
		if tried >= 3 {
			break
		}
		if !sameArtistName(a.Name, name, false) {
			continue
		}
		tried++
		p, err := d.profileByID(ctx, a.ID)
		if err != nil {
			continue
		}
		shared := map[string]bool{}
		for _, al := range p.Albums {
			if k := SongKey(al.Title); mine[k] {
				shared[k] = true
			}
		}
		if len(shared) >= need {
			return p, nil
		}
	}
	return DeezerProfile{}, nil
}

// profileByID reads one Deezer artist's picture, fans, top songs and albums.
func (d *ArtistImages) profileByID(ctx context.Context, id int64) (DeezerProfile, error) {
	var a struct {
		ID   int64  `json:"id"`
		XL   string `json:"picture_xl"`
		Big  string `json:"picture_big"`
		Fans int64  `json:"nb_fan"`
	}
	if err := d.json(ctx, d.base()+"/artist/"+strconv.FormatInt(id, 10), &a); err != nil {
		return DeezerProfile{}, fmt.Errorf("Deezer artist: %w", err)
	}
	if a.ID != id {
		return DeezerProfile{}, nil
	}
	p := DeezerProfile{ID: id, Fans: a.Fans, Picture: deezerPicture(a.XL, a.Big)}
	d.fillProfile(ctx, &p)
	return p, nil
}
