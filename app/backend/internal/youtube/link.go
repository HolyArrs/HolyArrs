package youtube

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Link is a YouTube video, playlist or channel. URL is always rebuilt from
// the checked ID, never copied from what was pasted.
type Link struct {
	Kind, ID, URL string
}

// ErrNotYouTube is returned for anything that is not a plain link to a
// YouTube video, playlist or channel.
var ErrNotYouTube = errors.New("paste a link to a YouTube video, playlist or channel")

var (
	reVideoID    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	rePlaylistID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,64}$`)
	reChannelID  = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	reHandle     = regexp.MustCompile(`^@[A-Za-z0-9._-]{3,30}$`)
	reLegacyName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// youtubeHosts are the only hosts a link may name.
var youtubeHosts = map[string]bool{
	"youtube.com":       true,
	"www.youtube.com":   true,
	"m.youtube.com":     true,
	"music.youtube.com": true,
	"youtu.be":          true,
}

// channelTabs may follow a channel path; the follow covers the channel
// whichever tab was pasted.
var channelTabs = map[string]bool{
	"videos": true, "streams": true, "shorts": true, "featured": true, "live": true,
}

const site = "https://www.youtube.com/"

// ParseLink checks a pasted link and returns what it points at.
func ParseLink(raw string) (Link, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Port() != "" ||
		!youtubeHosts[strings.ToLower(u.Hostname())] {
		return Link{}, ErrNotYouTube
	}
	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if strings.EqualFold(u.Hostname(), "youtu.be") {
		if len(segs) == 1 {
			return videoLink(segs[0])
		}
		return Link{}, ErrNotYouTube
	}
	if len(segs) == 0 {
		return Link{}, ErrNotYouTube
	}
	switch first := segs[0]; {
	case first == "watch" && len(segs) == 1:
		return videoLink(u.Query().Get("v"))
	case first == "playlist" && len(segs) == 1:
		id := u.Query().Get("list")
		if !rePlaylistID.MatchString(id) {
			return Link{}, ErrNotYouTube
		}
		return Link{KindPlaylist, id, site + "playlist?list=" + id}, nil
	case (first == "shorts" || first == "live" || first == "embed") && len(segs) == 2:
		return videoLink(segs[1])
	case strings.HasPrefix(first, "@") && reHandle.MatchString(first) && tabOnly(segs[1:]):
		return Link{KindChannel, first, site + first}, nil
	case first == "channel" && len(segs) >= 2 && reChannelID.MatchString(segs[1]) && tabOnly(segs[2:]):
		return Link{KindChannel, segs[1], site + "channel/" + segs[1]}, nil
	case (first == "c" || first == "user") && len(segs) >= 2 && reLegacyName.MatchString(segs[1]) && tabOnly(segs[2:]):
		id := first + "/" + segs[1]
		return Link{KindChannel, id, site + id}, nil
	}
	return Link{}, ErrNotYouTube
}

func videoLink(id string) (Link, error) {
	if !reVideoID.MatchString(id) {
		return Link{}, ErrNotYouTube
	}
	return Link{KindVideo, id, watchURL(id)}, nil
}

// watchURL is the page of a video id the caller has already checked.
func watchURL(id string) string {
	return site + "watch?v=" + id
}

// canonicalURL rebuilds a link's URL from its kind and checked ID, so a
// Link made anywhere but ParseLink never hands yt-dlp its own URL.
func canonicalURL(l Link) (string, error) {
	switch l.Kind {
	case KindVideo:
		if reVideoID.MatchString(l.ID) {
			return watchURL(l.ID), nil
		}
	case KindPlaylist:
		if rePlaylistID.MatchString(l.ID) {
			return site + "playlist?list=" + l.ID, nil
		}
	case KindChannel:
		if reChannelID.MatchString(l.ID) {
			return site + "channel/" + l.ID, nil
		}
		if reHandle.MatchString(l.ID) {
			return site + l.ID, nil
		}
		for _, p := range []string{"c/", "user/"} {
			if name, ok := strings.CutPrefix(l.ID, p); ok && reLegacyName.MatchString(name) {
				return site + l.ID, nil
			}
		}
	}
	return "", ErrNotYouTube
}

// tabOnly reports whether rest is empty or one known channel tab.
func tabOnly(rest []string) bool {
	return len(rest) == 0 || (len(rest) == 1 && channelTabs[rest[0]])
}
