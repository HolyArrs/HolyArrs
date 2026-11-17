package sources

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// NoDownloader says in plain words that nothing can take a protocol's
// downloads: "no Usenet Downloader is set up".
func NoDownloader(protocol string) string {
	if protocol == "usenet" {
		return "no Usenet Downloader is set up"
	}
	return "no torrent Downloader is set up"
}

// noDownloader passes over a Release no Downloader could take, so it is never
// chosen and shown as sent while nothing can download it. It returns true when
// it did.
func (s *Searcher) noDownloader(ctx context.Context, protocol string, v *mediafiles.Verdict) bool {
	if s.HasDownloader == nil || s.HasDownloader(ctx, protocol) {
		return false
	}
	v.Outcome = "passed_over"
	v.HoldUntil = nil
	v.Reasons = append(v.Reasons, mediafiles.Reason{Step: "rank", Rule: "no_downloader", Effect: "rank", Detail: NoDownloader(protocol)})
	return true
}

// blockedWhy is the reason a search chose nothing when the only passing
// Releases had no Downloader to go to, or "".
func blockedWhy(vs []mediafiles.Verdict) string {
	for _, v := range vs {
		for _, r := range v.Reasons {
			if r.Rule == "no_downloader" {
				return r.Detail
			}
		}
	}
	return ""
}
