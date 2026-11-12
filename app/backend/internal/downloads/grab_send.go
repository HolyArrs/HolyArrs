package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// send hands a Release to one Downloader and returns its id there. own is false for a torrent
// another app added (same info hash, not in HolyArrs's category), which HolyArrs never changes.
func (m *Manager) send(ctx context.Context, d domain.Downloader, rel domain.Release, slotID int64, hash string, stopped bool) (ext string, own bool, err error) {
	a, err := m.claimAttempt(ctx, d, rel.ID, slotID)
	if err != nil {
		return "", false, err
	}
	if a != nil {
		if a.State == "resolved" {
			return a.marker, true, nil
		}
		d.Category = a.category
		defer func() {
			if err == nil {
				return // Grab resolves it atomically with the Download row.
			}
			if !a.created || errors.Is(err, ErrUncertain) {
				err = errors.Join(ErrUncertain, err)
				return
			}
			// A definite refusal before the append may be retried. A failed cleanup is
			// treated conservatively: keep the intent and suppress failover.
			cleanupErr := m.resolveAttempt(ctx, a.ID)
			if cleanupErr != nil {
				err = errors.Join(ErrUncertain, err, cleanupErr)
			}
		}()
	}
	e, err := m.engine(ctx, d)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", d.Name, err)
	}
	// A job found again is taken as this Release only when no other Download row
	// still holds its id; a torrent whose old row is over may be sent again.
	skip, err := sendSkip(ctx, m.Store.Read(), d.ID, rel.ID, slotID, d.Protocol == "torrent")
	if err != nil {
		return "", false, err
	}
	if a != nil && !a.created {
		// A recovery is only a lookup. It needs neither the original Source nor a
		// send-spacing reservation, and may never fetch or append.
		ext, err = e.Add(ctx, AddRequest{Name: rel.Name, Category: a.category, Skip: skip, AttemptMarker: a.marker, InfoHash: a.expectedID, ReconcileOnly: true})
		if err != nil {
			if unreachable(err) {
				m.downloaderFailed(ctx, d, err)
			}
			return "", false, err
		}
		if ext == "" {
			return "", false, ErrUncertain
		}
		m.downloaderOK(ctx, d)
		return ext, true, nil
	}
	if hash != "" {
		if skip[hash] {
			return "", false, errors.New("the torrent belongs to another download")
		}
		if j, err := e.Get(ctx, hash); err == nil {
			if skip[j.ID] || (d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent" || d.Adapter == "aria2") && !sameCategory(j.Category, d.Category) {
				return "", false, errors.New("the torrent belongs to another download or category")
			}
			if j.ID != "" {
				hash = j.ID
			}
			return hash, d.Category != "" && sameCategory(j.Category, d.Category), nil
		}
	}
	link := rel.LinkRef
	if m.Link != nil {
		if link, err = m.Link(ctx, rel); err != nil {
			return "", false, err
		}
	}
	var src domain.Source
	if err := database.Get(ctx, m.Store.Read(), "sources", rel.SourceID, &src); err != nil {
		return "", false, errors.New("the Release's Source could not be read")
	}
	allowLocal := src.AllowLocal
	if d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent" || d.Adapter == "aria2" || d.Adapter == "nzbget" {
		if !strings.HasPrefix(link, "magnet:") && src.ProxyURL != "" {
			return "", false, errors.New("this Downloader cannot yet fetch Release files through a Source proxy")
		}
	}
	if link, err = grabLinkAllowed(d.Adapter, link, allowLocal); err != nil {
		return "", false, err
	}
	// qBittorrent and SABnzbd would fetch a link themselves from inside the home
	// network, where a name leading home is never checked. Without local network
	// access HolyArrs fetches the file and hands it over; a link that redirects to a
	// magnet sends the magnet. A Source with a proxy is fetched through it, with the
	// same checks. A qBittorrent Release with no info hash at all (an http link to a
	// .torrent, as private sites through Jackett or Prowlarr give) is fetched the
	// same way, even from a local Source: the hash comes from the file, so the
	// download can be followed.
	var file []byte
	noHash := d.Adapter == "qbittorrent" && hash == "" && linkHash(link) == ""
	if (d.Adapter == "qbittorrent" || d.Adapter == "sabnzbd") && (!allowLocal || noHash) && !strings.HasPrefix(link, "magnet:") {
		var rt http.RoundTripper
		if src.ProxyURL != "" {
			if m.SourceTransport == nil {
				return "", false, errors.New("this Downloader cannot yet fetch Release files through a Source proxy")
			}
			if rt, err = m.SourceTransport(ctx, src); err != nil {
				return "", false, err
			}
		}
		limit := addFetchMax
		if d.Adapter == "sabnzbd" {
			limit = sabMaxNZB
		}
		var magnet string
		if file, magnet, err = fetchGrabLink(ctx, link, limit, rt, sourceLocalHost(src.URL, allowLocal)); err == nil && magnet != "" {
			link, _, err = safeMagnet(magnet, allowLocal)
		}
		if err != nil {
			return "", false, err
		}
	}
	// Prowlarr and Jackett answer magnet-only sites with a redirect to a magnet,
	// which Transmission, Deluge, rTorrent, aria2 and some qBittorrent stand-ins cannot follow. A
	// torrent link not read above (a home Source whose Release has a hash) is read
	// here and the magnet or file is sent; if that fails the link is sent as before.
	torrentAdapter := d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent" || d.Adapter == "aria2" || d.Adapter == "qbittorrent" && src.ProxyURL == ""
	if torrentAdapter && file == nil && !strings.HasPrefix(link, "magnet:") {
		link, file = prefetchTorrent(ctx, link, allowLocal, sourceLocalHost(src.URL, allowLocal))
		if hash == "" {
			hash = linkHash(link)
		}
	}
	if d.Adapter == "qbittorrent" && file != nil {
		// Some trackers print a newline after the file; qBittorrent reads past it.
		file = bytes.TrimRight(file, " \t\r\n")
		// Trackers, web seeds and DHT nodes in the file that point at home are left
		// out, and the info hash stays the same. A Source allowed on the local network
		// keeps them: its file is fetched only to learn the hash, and a tracker at home
		// is then the user's own.
		if !allowLocal {
			if file, err = safeTorrent(file); err != nil {
				return "", false, errors.New("the Release file is not a torrent")
			}
		}
		h, err := torrentFileHash(file)
		if err != nil {
			// Often a one-off page (a limit or a check) instead of the file, so try again
			// and do not turn the Release down.
			return "", false, errors.New("the Release file is not a torrent")
		}
		if hash != "" && !strings.EqualFold(hash, h) {
			return "", false, releaseRefused{"the torrent file does not match its info hash"}
		}
		if hash == "" {
			// The hash came from the file, so the earlier look for it in qBittorrent did
			// not run, and adding a torrent it already has would fail.
			if j, err := e.Get(ctx, h); err == nil {
				if skip[j.ID] {
					return "", false, errors.New("the torrent belongs to another download or category")
				}
				if j.ID != "" {
					h = j.ID
				}
				return h, d.Category != "" && j.Category == d.Category, nil
			}
		}
		hash = h
	}
	// qBittorrent answers an add without an id, so HolyArrs follows the torrent by
	// its info hash, from the Release or its magnet link. Without one the torrent
	// would download unseen, so it is not sent.
	if d.Adapter == "qbittorrent" && hash == "" {
		if hash = linkHash(link); hash == "" {
			return "", false, releaseRefused{"the Release has no info hash, so " + d.Name + "'s download of it could not be followed"}
		}
	}
	done, err := m.space(ctx, d.URL)
	if err != nil {
		return "", false, err
	}
	req := AddRequest{URL: link, File: file, Name: rel.Name, Category: d.Category, SavePath: m.SavePath, InfoHash: hash, Skip: skip,
		AllowLocal: allowLocal, OperationKey: fmt.Sprintf("%d:%d:%d", d.ID, rel.ID, slotID), Stopped: stopped}
	if a != nil {
		req.AttemptMarker, req.ReconcileOnly = a.marker, !a.created
	}
	ext, err = e.Add(ctx, req)
	done()
	if err != nil {
		// Only a Downloader that did not answer, or turned down the sign-in, rests. One
		// that answered and refused this Release (a bad link, no category) stays ready
		// for the next one.
		if unreachable(err) {
			m.downloaderFailed(ctx, d, err)
		}
		return "", false, fmt.Errorf("sending to %s: %w", d.Name, err)
	}
	if ext == "" {
		if a != nil {
			return "", false, ErrUncertain
		}
		return "", false, fmt.Errorf("%s gave no id for the Release (no info hash)", d.Name)
	}
	m.downloaderOK(ctx, d)
	return ext, true, nil
}

func jsonIDs(ids []int64) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

// grabLinkAllowed applies the Source's network rule to a feed link before a
// Downloader gets it. Magnets are rebuilt by safeMagnet, and an address that
// qBittorrent or SABnzbd would fetch itself may not name a literal local,
// link-local or metadata host unless the Source may reach the local network.
func grabLinkAllowed(adapter, link string, allowLocal bool) (string, error) {
	if len(link) >= 7 && strings.EqualFold(link[:7], "magnet:") {
		clean, _, err := safeMagnet(link, allowLocal)
		return clean, err
	}
	if (adapter == "qbittorrent" || adapter == "sabnzbd") && !trackerAllowed(link, allowLocal) {
		return "", errors.New("the Release link points at a local or blocked address")
	}
	return link, nil
}
