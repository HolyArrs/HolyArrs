package musictrend

import (
	"regexp"
	"strconv"
)

// Cover sizes the app asks for: 250 for small tiles, 500 for big tiles,
// 1200 for hero art.
var CoverSizes = []int{250, 500, 1200}

var (
	// Cover Art Archive: .../front, .../front-500, .../123-250.jpg
	caaCover = regexp.MustCompile(`^(https?://(?:www\.)?coverartarchive\.org/release(?:-group)?/[^/]+/(?:front|back|\d+))(?:-(?:250|500|1200|small|large))?(?:\.jpg)?$`)
	// Deezer CDN: .../images/cover/<md5>/500x500-000000-80-0-0.jpg
	dzCover = regexp.MustCompile(`^(https?://[a-z0-9-]+\.dzcdn\.net/images/(?:cover|artist)/[0-9a-f]+/)\d+x\d+(-.*)?$`)
	// Deezer API: .../album/123/image?size=big
	dzAPICover = regexp.MustCompile(`^(https?://api\.deezer\.com/(?:album|artist)/\d+/image)(?:\?size=\w+)?$`)
	// Apple Music (chart feeds): .../100x100bb.jpg
	itCover = regexp.MustCompile(`^(https?://[a-z0-9-]+\.mzstatic\.com/image/thumb/.+/)\d+x\d+(bb|cc)?(\.(?:jpg|png|webp))$`)
)

// SizedCover points a known cover link (Cover Art Archive, Deezer, Apple Music)
// at the size px asked for. Other links come back as they are.
func SizedCover(u string, px int) string {
	n := strconv.Itoa(px)
	if m := caaCover.FindStringSubmatch(u); m != nil {
		return m[1] + "-" + n
	}
	if m := dzCover.FindStringSubmatch(u); m != nil {
		return m[1] + n + "x" + n + m[2]
	}
	if m := dzAPICover.FindStringSubmatch(u); m != nil {
		word := map[int]string{250: "medium", 500: "big", 1200: "xl"}[px]
		if word == "" {
			word = "big"
		}
		return m[1] + "?size=" + word
	}
	if m := itCover.FindStringSubmatch(u); m != nil {
		suffix := m[2]
		if suffix == "" {
			suffix = "bb"
		}
		return m[1] + n + "x" + n + suffix + m[3]
	}
	return u
}

// CoverVariants answers the link at each of CoverSizes, or nil when the
// link has no known size form.
func CoverVariants(u string) map[string]string {
	if SizedCover(u, 500) == u && SizedCover(u, 250) == u {
		return nil
	}
	out := map[string]string{}
	for _, s := range CoverSizes {
		out[strconv.Itoa(s)] = SizedCover(u, s)
	}
	return out
}

// CoverArtArchiveURL is the Cover Art Archive, whose release fronts stand in
// for chart albums without a cover link.
const CoverArtArchiveURL = "https://coverartarchive.org"
