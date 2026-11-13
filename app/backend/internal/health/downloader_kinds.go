package health

import (
	"context"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// noDownloaderLine says that Sources find a protocol's downloads that no
// Downloader can take, so none of them is ever sent.
func noDownloaderLine(protocol string) string {
	if protocol == "usenet" {
		return "Sources find Usenet downloads, but no Usenet Downloader is set up, so none can be sent"
	}
	return "Sources find torrent downloads, but no torrent Downloader is set up, so none can be sent"
}

// noDownloaderFix says what to add.
func noDownloaderFix(protocol string) string {
	if protocol == "usenet" {
		return "add a Usenet Downloader (such as SABnzbd or NZBGet) in Settings, Downloaders, or turn off the Usenet Sources"
	}
	return "add a torrent Downloader (such as qBittorrent) in Settings, Downloaders, or turn off the torrent Sources"
}

// downloaderKinds warns for each protocol the enabled Sources give that no
// Downloader takes. With the Sources part off there is nothing to say.
func (e *Engine) downloaderKinds(ctx context.Context) ([]Result, error) {
	if e.SourcesOn != nil && !e.SourcesOn() {
		return nil, nil
	}
	gaps, err := downloaderGaps(ctx, e.Store.Read())
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, g := range gaps {
		r := Result{Key: "no_downloader:" + g.Protocol, Kind: "no_downloader", Subject: g.Protocol, State: OK}
		if !g.Has {
			r.State, r.Message = Warning, noDownloaderLine(g.Protocol)
		}
		out = append(out, r)
	}
	return out, nil
}

// downloaderKindChecks is the same check for doctor.
func downloaderKindChecks(ctx context.Context, r *Report, db database.Lister) {
	gaps, err := downloaderGaps(ctx, db)
	if err != nil {
		r.add("Sources and Downloaders", StatusWarn, "could not read the Sources and Downloaders: "+err.Error(), "")
		return
	}
	for _, g := range gaps {
		if !g.Has {
			r.add("Sources and Downloaders", StatusWarn, noDownloaderLine(g.Protocol), noDownloaderFix(g.Protocol))
		}
	}
}
