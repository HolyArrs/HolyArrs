package downloader

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// TorrentUpload is one .torrent file a person dropped on the Add sheet.
type TorrentUpload struct {
	Name string
	Data []byte
}

// AddBatch is what a person adds by hand: links (magnet, http(s) .torrent
// or a bare info hash) and uploaded files, into one category and folder.
type AddBatch struct {
	Links    []string
	Files    []TorrentUpload
	Category string
	SavePath string
	Paused   bool
}

// AddOutcome says how a hand-made add went. Pending are http links the
// client still fetches in the background: their outcome shows in the list.
type AddOutcome struct {
	Added      int `json:"added"`
	Failed     int `json:"failed"`
	Pending    int `json:"pending"`
	Duplicates int `json:"duplicates"`
	// Bad names the uploaded files that were not torrents.
	Bad []string `json:"bad,omitempty"`
}

// TorrentAdder is the optional part of a torrent engine that adds torrents a
// person picked by hand. Nothing it adds becomes a HolyArrs download.
type TorrentAdder interface {
	AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error)
}

// ErrNothingToAdd is an add with no links and no files.
var ErrNothingToAdd = errors.New("add at least one link or .torrent file")

// linkHash reads the info hash of a magnet link or a bare hash; "" when the
// link does not carry one (an http link to a .torrent file).
func linkHash(link string) string {
	l := strings.TrimSpace(link)
	if source.ValidInfoHash(strings.ToLower(l)) {
		return strings.ToLower(l)
	}
	if !strings.HasPrefix(strings.ToLower(l), "magnet:") {
		return ""
	}
	u, err := url.Parse(l)
	if err != nil {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		v, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:")
		if !ok {
			continue
		}
		if source.ValidInfoHash(v) {
			return v
		}
		if len(v) == 32 {
			if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(v)); err == nil {
				return hex.EncodeToString(b)
			}
		}
	}
	return ""
}

// AddTorrents adds links and files in one torrents/add call. Torrents the
// client already holds are counted as duplicates and not sent. qBittorrent
// before 5.2 answers "Ok." or "Fails." for the whole call; 5.2 answers
// counts, 409 when nothing was added and 415 for a file that is no torrent.
func (q *Qbittorrent) AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error) {
	var out AddOutcome
	have := map[string]bool{}
	if all, err := q.info(ctx, url.Values{}); err == nil {
		for _, j := range all {
			have[j.ID] = true
		}
	}
	seen := map[string]bool{}
	var links []string
	for _, l := range b.Links {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if h := linkHash(l); h != "" {
			if have[h] || seen[h] {
				out.Duplicates++
				continue
			}
			seen[h] = true
		}
		links = append(links, l)
	}
	var files []TorrentUpload
	for _, f := range b.Files {
		h, err := torrentFileHash(f.Data)
		if err != nil {
			out.Bad = append(out.Bad, f.Name)
			out.Failed++
			continue
		}
		if have[h] || seen[h] {
			out.Duplicates++
			continue
		}
		seen[h] = true
		files = append(files, f)
	}
	n := len(links) + len(files)
	if n == 0 {
		if out.Duplicates == 0 && out.Failed == 0 {
			return out, ErrNothingToAdd
		}
		return out, nil
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	field := func(k, v string) { _ = mw.WriteField(k, v) }
	if len(links) > 0 {
		field("urls", strings.Join(links, "\n"))
	}
	if b.Category != "" {
		field("category", b.Category)
	}
	if b.SavePath != "" {
		field("savepath", b.SavePath)
	}
	if b.Paused {
		field("stopped", "true") // 5.x
		field("paused", "true")  // older servers
	}
	for _, f := range files {
		w, err := mw.CreateFormFile("torrents", safeFileName(f.Name))
		if err != nil {
			return out, err
		}
		_, _ = w.Write(f.Data)
	}
	_ = mw.Close()
	code, resp, err := q.callRaw(ctx, "torrents/add", mw.FormDataContentType(), body.Bytes(), http.StatusConflict, http.StatusUnsupportedMediaType)
	if err != nil {
		return out, err
	}
	text := strings.TrimSpace(string(resp))
	switch {
	case code == http.StatusConflict:
		out.Failed += n
	case code == http.StatusUnsupportedMediaType:
		return out, errors.New("qBittorrent could not read one of the .torrent files")
	case text == "Fails.":
		out.Failed += n
	case text == "Ok." || text == "":
		// Before 5.2 there are no counts: http links are fetched later, the rest is in.
		for _, l := range links {
			if linkHash(l) == "" {
				out.Pending++
			} else {
				out.Added++
			}
		}
		out.Added += len(files)
	default:
		var c struct {
			Success int `json:"success_count"`
			Failure int `json:"failure_count"`
			Pending int `json:"pending_count"`
		}
		if json.Unmarshal(resp, &c) != nil {
			return out, fmt.Errorf("qbittorrent torrents/add: unexpected answer")
		}
		out.Added += c.Success
		out.Failed += c.Failure
		out.Pending += c.Pending
	}
	return out, nil
}

// safeFileName keeps an uploaded name to its last part, for the form only.
func safeFileName(n string) string {
	n = n[strings.LastIndexAny(n, `/\`)+1:]
	if n == "" {
		return "upload.torrent"
	}
	return n
}
