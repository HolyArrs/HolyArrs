package downloads

import (
	"path"
	"strings"
	"unicode"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// seenName names a download by the files its Downloader held, for when its
// Release is gone: their top folder, or a lone file without its extension.
// "" when the files say nothing.
func seenName(files []domain.SeenFile) string {
	if len(files) == 0 {
		return ""
	}
	p := strings.Trim(strings.ReplaceAll(files[0].Name, `\`, "/"), "/")
	if top, _, ok := strings.Cut(p, "/"); ok {
		p = top
	} else if len(files) > 1 {
		return ""
	} else {
		p = strings.TrimSuffix(p, fileExt(p))
	}
	if p == "." || p == ".." {
		return ""
	}
	return p
}

// fileExt is a name's file extension (".mkv"): a short run of letters and
// digits with a letter in it, so "Movie.2020" and "x.H264-GRP" have none.
func fileExt(name string) string {
	ext := path.Ext(name)
	if len(ext) < 2 || len(ext) > 5 {
		return ""
	}
	letter := false
	for _, r := range ext[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ""
		}
		letter = letter || unicode.IsLetter(r)
	}
	if !letter {
		return ""
	}
	return ext
}
