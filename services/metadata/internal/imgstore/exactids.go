package imgstore

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
)

// How an artist's outside id was found, after MusicBrainz's own link
// (MatchLink) and before a name search (MatchSearch).
const (
	MatchBarcode  = "barcode"
	MatchWikidata = "wikidata"
)

// Via names, as kept with each match (Store.SetVia).
const (
	ViaLink     = "mb-link"
	ViaBarcode  = "barcode"
	ViaWikidata = "wikidata"
	ViaName     = "name"
)

// viaOf turns a Deezer or Spotify "how" into its via name.
func viaOf(how string) string {
	switch how {
	case MatchLink:
		return ViaLink
	case MatchBarcode:
		return ViaBarcode
	case MatchWikidata:
		return ViaWikidata
	case MatchSearch:
		return ViaName
	}
	return ""
}

// wikidataLink is a MusicBrainz link to the artist's Wikidata item.
var wikidataLink = regexp.MustCompile(`^https?://(?:www\.)?wikidata\.org/wiki/(Q[0-9]+)$`)

// LinkedQID answers the Wikidata item MusicBrainz links ("" when none).
func LinkedQID(links []string) string {
	for _, l := range links {
		if m := wikidataLink.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// via notes how a match was found.
func (r *Resolver) via(mbid, source, via string) {
	if r.Via != nil && via != "" {
		r.Via(KindArtist, mbid, source, via)
	}
}

// ExactID finds the artist's id at a source by exact ids, after its
// MusicBrainz link and before any name search: first the barcodes of its
// own releases, then the ids its MusicBrainz-linked Wikidata item names.
// A barcode counts for the album artist named like the artist (a duo's
// album also lists its members, an alias's the person): one such barcode
// is enough when no other points elsewhere; without a name match (another
// script) two barcodes must agree on one artist. store is Apple's
// storefront where the barcode was found ("" otherwise). "" when neither
// step gives one.
func (r *Resolver) ExactID(ctx context.Context, source, mbid, name, qid, country string) (id, store, how string, err error) {
	if r.Barcodes != nil {
		named := map[string]string{} // id: storefront
		bare := map[string]int{}     // id: barcodes naming it without a name match
		bareStore := map[string]string{}
		for _, code := range r.Barcodes(ctx, mbid, 3) {
			if !ValidBarcode(code) {
				continue // crowd-sourced: never sent on unchecked
			}
			artists, st, err := r.upcArtists(ctx, source, code, country)
			if err != nil {
				return "", "", "", err
			}
			hit := ""
			for _, a := range artists {
				if ValidSourceID(source, a.id) && a.name != "" && SameName(a.name, name) {
					hit = a.id
					break
				}
			}
			switch {
			case hit != "":
				named[hit] = st
			case len(artists) > 0 && ValidSourceID(source, artists[0].id):
				bare[artists[0].id]++
				bareStore[artists[0].id] = st
			}
		}
		if len(named) == 1 {
			for id, st := range named {
				if n := len(bare); n == 0 || n == 1 && bare[id] > 0 {
					return id, st, MatchBarcode, nil
				}
			}
		}
		if len(named) == 0 && len(bare) == 1 {
			for id, n := range bare {
				if n >= 2 {
					return id, bareStore[id], MatchBarcode, nil
				}
			}
		}
	}
	if r.WikidataIDs != nil && qid != "" {
		apple, deezer, spotify, err := r.WikidataIDs(ctx, qid)
		if err != nil {
			return "", "", "", err
		}
		if v := map[string]string{"applemusic": apple, "deezer": deezer, "spotify": spotify}[source]; ValidSourceID(source, v) {
			return v, "", MatchWikidata, nil
		}
	}
	return "", "", "", nil
}

// albumArtist is one of an album's artists at a source.
type albumArtist struct{ id, name string }

// upcArtists answers the artists of the album a barcode names at a source,
// the main one first (none when the source has no such album).
func (r *Resolver) upcArtists(ctx context.Context, source, code, country string) ([]albumArtist, string, error) {
	var out []albumArtist
	switch source {
	case "applemusic":
		if r.AppleMusic == nil {
			return nil, "", nil
		}
		for _, sf := range r.AppleMusic.storefronts(country) {
			var d struct {
				Data []struct {
					Relationships struct {
						Artists struct {
							Data []struct {
								ID         string `json:"id"`
								Attributes struct {
									Name string `json:"name"`
								} `json:"attributes"`
							} `json:"data"`
						} `json:"artists"`
					} `json:"relationships"`
				} `json:"data"`
			}
			ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(sf)+"/albums", url.Values{"filter[upc]": {code}, "include": {"artists"}}, &d)
			if err != nil {
				return nil, "", err
			}
			if ok && len(d.Data) > 0 {
				for _, a := range d.Data[0].Relationships.Artists.Data {
					out = append(out, albumArtist{a.ID, a.Attributes.Name})
				}
				if len(out) > 0 {
					return out, sf, nil
				}
			}
		}
	case "deezer":
		type dzRef struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		}
		var d struct {
			Artist       dzRef   `json:"artist"`
			Contributors []dzRef `json:"contributors"`
		}
		ok, err := r.deezerGet(ctx, "/album/upc:"+url.PathEscape(code), nil, &d)
		if err != nil || !ok {
			return nil, "", err
		}
		seen := map[string]bool{}
		for _, a := range append([]dzRef{d.Artist}, d.Contributors...) {
			if s := a.ID.String(); s != "" && s != "0" && !seen[s] {
				seen[s] = true
				out = append(out, albumArtist{s, a.Name})
			}
		}
	case "spotify":
		var d struct {
			Albums struct {
				Items []struct {
					Artists []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"artists"`
				} `json:"items"`
			} `json:"albums"`
		}
		ok, err := r.spotifyGet(ctx, "/v1/search?"+url.Values{"type": {"album"}, "q": {"upc:" + code}, "limit": {"1"}}.Encode(), &d)
		if err != nil || !ok {
			return nil, "", err
		}
		if len(d.Albums.Items) > 0 {
			for _, a := range d.Albums.Items[0].Artists {
				out = append(out, albumArtist{a.ID, a.Name})
			}
		}
	}
	return out, "", nil
}

// Shapes of the ids and barcodes taken from crowd-sourced data (Wikidata,
// MusicBrainz): anything else is skipped, never sent on.
var (
	digitsID   = regexp.MustCompile(`^[0-9]{1,20}$`)
	spotifyID  = regexp.MustCompile(`^[0-9A-Za-z]{22}$`)
	barcodeFmt = regexp.MustCompile(`^[0-9]{8,14}$`)
)

// ValidSourceID reports an id of the right shape for its source: Apple
// Music and Deezer ids are digits, Spotify ids 22 letters and digits.
func ValidSourceID(source, id string) bool {
	switch source {
	case "applemusic", "deezer":
		return digitsID.MatchString(id)
	case "spotify":
		return spotifyID.MatchString(id)
	}
	return false
}

// ValidBarcode reports a barcode of 8 to 14 digits.
func ValidBarcode(code string) bool { return barcodeFmt.MatchString(code) }
