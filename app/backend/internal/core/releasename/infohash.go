package releasename

import "regexp"

var infoHashRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// ValidInfoHash says whether s is a lower-case v1 (40 hex) or v2 (64 hex)
// torrent info hash.
func ValidInfoHash(s string) bool { return infoHashRe.MatchString(s) }
