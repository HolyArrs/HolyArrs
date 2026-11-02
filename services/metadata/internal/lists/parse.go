package lists

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// Entry is one row of a chart or a new-release list, in our own words.
// Ext ids are the source's own ids; they stay inside this package and
// are never served.
type Entry struct {
	Rank        int    `json:"rank"`
	Prev        int    `json:"prev,omitempty"`
	Peak        int    `json:"peak,omitempty"`
	PeakDate    string `json:"peak_date,omitempty"`
	Days        int    `json:"days,omitempty"`
	Streams     int64  `json:"streams,omitempty"`
	Status      string `json:"status,omitempty"` // up, down, same, new, back
	EntryDate   string `json:"entry_date,omitempty"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	ExtArtist   string `json:"ext_artist,omitempty"`
	ExtAlbum    string `json:"ext_album,omitempty"`
	Album       string `json:"album,omitempty"`      // a list song's album name
	AlbumType   string `json:"album_type,omitempty"` // album, single, ep, compilation
	Image       string `json:"image,omitempty"`      // the artwork's own id (joins songs to albums)
	ReleaseDate string `json:"release_date,omitempty"`
}

type chartRow struct {
	Entry struct {
		Current     int    `json:"currentRank"`
		Previous    int    `json:"previousRank"`
		Peak        int    `json:"peakRank"`
		PeakDate    string `json:"peakDate"`
		Consecutive int    `json:"consecutiveAppearancesOnChart"`
		Status      string `json:"entryStatus"`
		EntryDate   string `json:"entryDate"`
		Metric      struct {
			Value string `json:"value"`
		} `json:"rankingMetric"`
	} `json:"chartEntryData"`
	Track *struct {
		Name    string     `json:"trackName"`
		Image   string     `json:"displayImageUri"`
		Artists []extNamed `json:"artists"`
		Release string     `json:"releaseDate"`
	} `json:"trackMetadata"`
	Album *struct {
		Name    string     `json:"albumName"`
		URI     string     `json:"albumUri"`
		Image   string     `json:"displayImageUri"`
		Artists []extNamed `json:"artists"`
		Release string     `json:"releaseDate"`
	} `json:"albumMetadata"`
	Artist *struct {
		Name  string `json:"artistName"`
		URI   string `json:"artistUri"`
		Image string `json:"displayImageUri"`
	} `json:"artistMetadata"`
}

type extNamed struct {
	Name string `json:"name"`
	URI  string `json:"spotifyUri"`
}

// ImageID is the artwork's hash without the size prefix, so the same
// cover at two sizes compares equal.
func ImageID(u string) string { return imageID(u) }

func imageID(u string) string {
	if i := strings.LastIndex(u, "/"); i >= 0 {
		u = u[i+1:]
	}
	if len(u) > 40 {
		u = u[len(u)-40:]
	}
	return u
}

// lastPart is the id after the last ':' of a source URI.
func lastPart(u string) string {
	if i := strings.LastIndex(u, ":"); i >= 0 {
		return u[i+1:]
	}
	return u
}

var statusWords = map[string]string{"MOVED_UP": "up", "MOVED_DOWN": "down", "NO_CHANGE": "same", "NEW_ENTRY": "new", "RE_ENTRY": "back"}

// ParseChart reads a song, album or artist chart.
func ParseChart(r io.Reader) ([]Entry, error) {
	var rows []chartRow
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, x := range rows {
		e := Entry{Rank: x.Entry.Current, Prev: x.Entry.Previous, Peak: x.Entry.Peak, PeakDate: x.Entry.PeakDate, Days: x.Entry.Consecutive, Status: statusWords[x.Entry.Status], EntryDate: x.Entry.EntryDate}
		e.Streams, _ = strconv.ParseInt(x.Entry.Metric.Value, 10, 64)
		switch {
		case x.Track != nil:
			e.Title, e.Image, e.ReleaseDate = x.Track.Name, imageID(x.Track.Image), x.Track.Release
			if len(x.Track.Artists) > 0 {
				e.Artist, e.ExtArtist = x.Track.Artists[0].Name, lastPart(x.Track.Artists[0].URI)
			}
		case x.Album != nil:
			e.Title, e.ExtAlbum, e.Image, e.ReleaseDate, e.AlbumType = x.Album.Name, lastPart(x.Album.URI), imageID(x.Album.Image), x.Album.Release, "album"
			if len(x.Album.Artists) > 0 {
				e.Artist, e.ExtArtist = x.Album.Artists[0].Name, lastPart(x.Album.Artists[0].URI)
			}
		case x.Artist != nil:
			e.Artist, e.ExtArtist, e.Image = x.Artist.Name, lastPart(x.Artist.URI), imageID(x.Artist.Image)
		default:
			continue
		}
		if e.Rank > 0 && (e.Title != "" || e.Artist != "") {
			out = append(out, e)
		}
	}
	return out, nil
}

// ParseNew reads one page of a new-release list.
func ParseNew(r io.Reader) ([]Entry, error) {
	var body struct {
		Albums struct {
			Offset int `json:"offset"`
			Items  []struct {
				Type    string `json:"album_type"`
				ID      string `json:"id"`
				Name    string `json:"name"`
				Release string `json:"release_date"`
				Images  []struct {
					URL string `json:"url"`
				} `json:"images"`
				Artists []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(body.Albums.Items))
	for i, it := range body.Albums.Items {
		e := Entry{Rank: body.Albums.Offset + i + 1, Title: it.Name, ExtAlbum: it.ID, AlbumType: it.Type, ReleaseDate: it.Release}
		if len(it.Artists) > 0 {
			e.Artist, e.ExtArtist = it.Artists[0].Name, it.Artists[0].ID
		}
		if len(it.Images) > 0 {
			e.Image = imageID(it.Images[0].URL)
		}
		if e.Title != "" && e.Artist != "" {
			out = append(out, e)
		}
	}
	return out, nil
}

// ParseCategory reads a browse category page: the ids of its editorial
// lists in page order (sections top to bottom), each once. Only ids of the
// source's own editorial lists are kept; user lists and albums are skipped.
func ParseCategory(r io.Reader) ([]Entry, error) {
	var body struct {
		Sections []struct {
			Items []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"items"`
		} `json:"sections"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, err
	}
	var out []Entry
	seen := map[string]bool{}
	for _, s := range body.Sections {
		for _, it := range s.Items {
			if it.Type != "playlist" || !strings.HasPrefix(it.ID, "37i9dQZF1D") || seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, Entry{Rank: len(out) + 1, ExtAlbum: it.ID})
		}
	}
	return out, nil
}

// ParsePlaylist reads one page of a list's songs: each song with its
// album (name, id, artwork) and first artist, in list order.
func ParsePlaylist(r io.Reader) ([]Entry, error) {
	var body struct {
		Items []struct {
			Track *struct {
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					ID     string `json:"id"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"track"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, err
	}
	var out []Entry
	for _, it := range body.Items {
		t := it.Track
		if t == nil || t.Name == "" || len(t.Artists) == 0 || t.Artists[0].Name == "" {
			continue
		}
		e := Entry{Rank: len(out) + 1, Title: t.Name, Artist: t.Artists[0].Name, ExtArtist: t.Artists[0].ID, Album: t.Album.Name, ExtAlbum: t.Album.ID}
		if len(t.Album.Images) > 0 {
			e.Image = imageID(t.Album.Images[0].URL)
		}
		out = append(out, e)
	}
	return out, nil
}
