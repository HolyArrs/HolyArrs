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
func languagesOf(codes []string) []language {
	out := []language{}
	for _, c := range codes {
		if n := subtitles.LanguageName(c); n != "" {
			out = append(out, language{Name: n})
		}
	}
	return out
}

// originalLanguage returns a title's original language. It is "Unknown"
// when the metadata gave none, so callers always find the field.
func originalLanguage(code *string) language {
	if code != nil {
		if n := subtitles.LanguageName(*code); n != "" {
			return language{Name: n}
		}
	}
	return language{Name: "Unknown"}
}

// alternateTitle is one other name of a title.
type alternateTitle struct {
	Title string `json:"title"`
}

func alternateTitles(names []string) []alternateTitle {
	out := []alternateTitle{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, alternateTitle{Title: n})
		}
	}
	return out
}
