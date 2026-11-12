package downloads

import (
	"slices"
	"strings"
)

// sameCategory reports whether two category names are the same. Clients
// change the case of a category (NZBGet answers "Movies" for movies, SABnzbd
// keeps names in lower case), so case never counts.
func sameCategory(a, b string) bool {
	return strings.EqualFold(a, b)
}

// hasCategory reports whether cats holds c, in any case.
func hasCategory(cats []string, c string) bool {
	return slices.ContainsFunc(cats, func(x string) bool { return sameCategory(x, c) })
}
