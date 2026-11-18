package updates

import (
	"net/url"
	"os"
)

// Lab builds only (set with -ldflags -X): a throwaway public key and one
// plain-http host for a local release server. Release builds leave both
// empty, so only the pinned keys and the listed https hosts count.
var (
	labKey  = ""
	labHost = ""
)

// labAllowed says link is on the lab build's release server.
func labAllowed(link string) bool {
	if labHost == "" {
		return false
	}
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Host == labHost
}

// LabListURL is the update list address of a lab build ("" otherwise),
// read from HOLYARRS_UPDATE_LIST.
func LabListURL() string {
	if labHost == "" || labKey == "" {
		return ""
	}
	if v := os.Getenv("HOLYARRS_UPDATE_LIST"); labAllowed(v) {
		return v
	}
	return ""
}
