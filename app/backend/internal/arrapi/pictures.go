package arrapi

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// coverSizes are the smaller poster copies a cover name may ask for, as
// in poster-250.jpg. Bazarr shows 250 and swaps in 500. Any other size
// gets the stored size, so a caller cannot fill the cache with copies.
var coverSizes = map[string]int{"250": 250, "500": 500}

// coverName reads a cover file name loosely, such as poster.jpg,
// poster-250.jpg or FANART.png. The query is only a cache buster and is
// never part of it.
func coverName(file string) (kind string, px int) {
	stem := strings.ToLower(strings.TrimSuffix(file, path.Ext(file)))
	kind, size, _ := strings.Cut(stem, "-")
	return kind, coverSizes[size]
}

// posterOf returns the title's stored poster ("poster:<cache name>"), or "".
func posterOf(t mediafiles.Title) string {
	for _, a := range t.Artwork {
		if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
			return a
		}
	}
	return ""
}

// libraryImages lists a Library title's pictures, poster first and the
// wide one second, because Bazarr reads them in that order. url points at
// this side's covers route, as the old apps point at their own cover
// cache. remoteUrl keeps the original address when one is known. A stored
// poster has none, so it is empty there.
func (h *handler) libraryImages(ctx context.Context, t mediafiles.Title) []image {
	out := []image{}
	at := h.base(ctx) + "/MediaCover/" + itoa(t.ID) + "/"
	if posterOf(t) != "" {
		out = append(out, image{CoverType: "poster", URL: at + "poster.jpg"})
	}
	if t.Backdrop != "" {
		out = append(out, image{CoverType: "fanart", URL: at + "fanart.jpg", RemoteURL: t.Backdrop})
	}
	return out
}

// cover sends a Library title's poster or wide picture. A removed title,
// a title of the other side, an unknown id or an unknown picture answers
// 404.
func (h *handler) cover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.title(r.Context(), id, h.folderKind())
	if err != nil {
		internalError(w, err)
		return
	}
	src := ""
	kind, px := coverName(r.PathValue("file"))
	switch kind {
	case "poster":
		src = posterOf(t)
	case "fanart":
		src, px = t.Backdrop, 0
	}
	if src == "" || h.Picture == nil {
		notFound(w)
		return
	}
	h.Picture(w, r, src, px)
}

// language is a language as the old apps write it. HolyArrs keeps no
// number for a language, so id is always 0. Callers read the name.
type language struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// languagesOf names a file's language codes. Unknown codes are left out.
func languagesOf(side string, codes []string) []language {
	out := []language{}
	for _, c := range codes {
		if n := subtitles.LanguageName(c); n != "" {
			out = append(out, languageOf(side, n))
		}
	}
	return out
}

// languageIDs are each side's language numbers, as Sonarr 4 and Radarr 6
// list them under /language. A name not listed is 0 (Unknown).
var languageIDs = map[string]map[string]int{
	"sonarr": {"English": 1, "French": 2, "Spanish": 3, "German": 4, "Italian": 5, "Danish": 6, "Dutch": 7, "Japanese": 8, "Icelandic": 9, "Chinese": 10, "Russian": 11, "Polish": 12, "Vietnamese": 13, "Swedish": 14, "Norwegian": 15, "Finnish": 16, "Turkish": 17, "Portuguese": 18, "Flemish": 19, "Greek": 20, "Korean": 21, "Hungarian": 22, "Hebrew": 23, "Lithuanian": 24, "Czech": 25, "Arabic": 26, "Hindi": 27, "Bulgarian": 28, "Malayalam": 29, "Ukrainian": 30, "Slovak": 31, "Thai": 32, "Portuguese (Brazil)": 33, "Spanish (Latino)": 34, "Romanian": 35, "Latvian": 36, "Persian": 37, "Catalan": 38, "Croatian": 39, "Serbian": 40, "Bosnian": 41, "Estonian": 42, "Tamil": 43, "Indonesian": 44, "Macedonian": 45, "Slovenian": 46},
	"radarr": {"English": 1, "French": 2, "Spanish": 3, "German": 4, "Italian": 5, "Danish": 6, "Dutch": 7, "Japanese": 8, "Icelandic": 9, "Chinese": 10, "Russian": 11, "Polish": 12, "Vietnamese": 13, "Swedish": 14, "Norwegian": 15, "Finnish": 16, "Turkish": 17, "Portuguese": 18, "Flemish": 19, "Greek": 20, "Korean": 21, "Hungarian": 22, "Hebrew": 23, "Lithuanian": 24, "Czech": 25, "Hindi": 26, "Romanian": 27, "Thai": 28, "Bulgarian": 29, "Portuguese (Brazil)": 30, "Arabic": 31, "Ukrainian": 32, "Persian": 33, "Bengali": 34, "Slovak": 35, "Latvian": 36, "Spanish (Latino)": 37, "Catalan": 38, "Croatian": 39, "Serbian": 40, "Bosnian": 41, "Estonian": 42, "Tamil": 43, "Indonesian": 44, "Telugu": 45, "Macedonian": 46, "Slovenian": 47, "Malayalam": 48, "Kannada": 49, "Albanian": 50, "Afrikaans": 51, "Marathi": 52, "Tagalog": 53, "Urdu": 54, "Romansh": 55, "Mongolian": 56, "Georgian": 57},
}

// languageOf is a language by name with this side's number.
func languageOf(side, name string) language {
	return language{ID: languageIDs[side][name], Name: name}
}

// originalLanguage returns a title's original language. It is "Unknown"
// when the metadata gave none, so callers always find the field.
func originalLanguage(side string, code *string) language {
	if code != nil {
		if n := subtitles.LanguageName(*code); n != "" {
			return languageOf(side, n)
		}
	}
	return language{Name: "Unknown"}
}

// alternateTitle is one other name of a title.
// alternateTitle carries the fields each side sends with one: Sonarr a
// season number (-1: every season), Radarr where it came from and ids.
type alternateTitle struct {
	Title           string `json:"title"`
	SeasonNumber    *int   `json:"seasonNumber,omitempty"`
	SourceType      string `json:"sourceType,omitempty"`
	MovieMetadataID int64  `json:"movieMetadataId,omitempty"`
	ID              int64  `json:"id,omitempty"`
}

func alternateTitles(side string, titleID int64, names []string) []alternateTitle {
	out := []alternateTitle{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			a := alternateTitle{Title: n}
			if side == "sonarr" {
				a.SeasonNumber = ptr(-1)
			} else {
				a.SourceType, a.MovieMetadataID, a.ID = "tmdb", titleID, titleID*1000+int64(len(out))+1
			}
			out = append(out, a)
		}
	}
	return out
}
