package downloads

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ErrNothingToAdd means the batch had no links and no files.
var ErrNothingToAdd = errors.New("add at least one link or .torrent file")

// TorrentUpload is a .torrent file the user picked.
type TorrentUpload struct {
	Name string
	Data []byte
}

// AddBatch is what the user adds by hand from the Downloads screen.
type AddBatch struct {
	Links      []string
	Files      []TorrentUpload
	Category   string
	SavePath   string
	Paused     bool
	AllowLocal bool
}

// AddOutcome counts what happened to each item of a batch. Bad names the items that could not be read; Dropped counts tracker addresses removed from magnets as unsafe.
type AddOutcome struct {
	Added      int      `json:"added"`
	Failed     int      `json:"failed"`
	Pending    int      `json:"pending"`
	Duplicates int      `json:"duplicates"`
	Bad        []string `json:"bad,omitempty"`
	Dropped    int      `json:"dropped,omitempty"`
}

// TorrentAdder is the optional part of an engine that takes manual adds.
type TorrentAdder interface {
	AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error)
}

// SaveBase holds the client's default save folder and each category's own folder.
type SaveBase struct {
	Default    string
	Categories map[string]string
}

// TorrentFolders is the optional part of an engine that tells where it saves.
type TorrentFolders interface {
	SaveBase(ctx context.Context) (SaveBase, error)
}

// AddTorrents sends a batch to qBittorrent. Items it already holds are skipped by info hash, so a repeat is a duplicate and not an error. Web links are fetched here and uploaded as files, so the client never contacts the Source itself.
func (q *Qbittorrent) AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error) {
	var out AddOutcome
	// Ids already in the client. If the list call fails the client still dedupes on its own.
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
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			data, err := fetchTorrentLink(ctx, l, b.AllowLocal)
			if err != nil {
				out.Bad = append(out.Bad, linkName(l))
				out.Failed++
				continue
			}
			b.Files = append(b.Files, TorrentUpload{Name: linkName(l), Data: data})
			continue
		}
		if strings.HasPrefix(low, "magnet:") {
			clean, n, err := safeMagnet(l, b.AllowLocal)
			if err != nil {
				out.Bad = append(out.Bad, "a magnet link")
				out.Failed++
				continue
			}
			l = clean
			out.Dropped += n
		}
		// Dedupe inside the batch too.
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
	// Old and new qBittorrent versions name the pause flag differently, so send both.
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
		field("stopped", "true")
		field("paused", "true")
	}
	for _, f := range files {
		w, err := mw.CreateFormFile("torrents", safeFileName(f.Name))
		if err != nil {
			return out, err
		}
		_, _ = w.Write(f.Data)
	}
	_ = mw.Close()
	// 409 and 415 are answers to read, not transport errors.
	code, resp, err := q.callRaw(ctx, "torrents/add", mw.FormDataContentType(), body.Bytes(), http.StatusConflict, http.StatusUnsupportedMediaType)
	if err != nil {
		return out, err
	}
	text := strings.TrimSpace(string(resp))
	switch {
	// 409 means the client refused every item.
	case code == http.StatusConflict:
		out.Failed += n
	case code == http.StatusUnsupportedMediaType:
		return out, errors.New("qBittorrent could not read one of the .torrent files")
	// Older clients answer in plain text; newer ones answer with counts.
	case text == "Fails.":
		out.Failed += n
	case text == "Ok." || text == "":
		out.Added += len(links) + len(files)
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

// SaveBase reads the default and per-category save folders.
func (q *Qbittorrent) SaveBase(ctx context.Context) (SaveBase, error) {
	out := SaveBase{Categories: map[string]string{}}
	b, err := q.call(ctx, http.MethodGet, "app/preferences", nil)
	if err != nil {
		return out, err
	}
	var p struct {
		SavePath string `json:"save_path"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return out, fmt.Errorf("qbittorrent app/preferences: %w", err)
	}
	out.Default = p.SavePath
	b, err = q.call(ctx, http.MethodGet, "torrents/categories", nil)
	if err != nil {
		return out, err
	}
	var cats map[string]struct {
		SavePath string `json:"savePath"`
	}
	if err := json.Unmarshal(b, &cats); err != nil {
		return out, fmt.Errorf("qbittorrent torrents/categories: %w", err)
	}
	for name, c := range cats {
		out.Categories[name] = c.SavePath
	}
	return out, nil
}

// linkHash returns the lowercase hex info hash of a bare hash or magnet link, or empty. Base32 hashes are converted since some magnets use them.
func linkHash(link string) string {
	l := strings.TrimSpace(link)
	if releasename.ValidInfoHash(strings.ToLower(l)) {
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
		if releasename.ValidInfoHash(v) {
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

// safeFileName drops any folder part of an uploaded name so it cannot steer the client.
func safeFileName(n string) string {
	n = n[strings.LastIndexAny(n, `/\`)+1:]
	if n == "" {
		return "upload.torrent"
	}
	return n
}

// linkName gives a short label for a link without its query, which may hold a key.
func linkName(l string) string {
	u, err := url.Parse(l)
	if err != nil {
		return "link"
	}
	return u.Host + "/" + safeFileName(u.Path)
}
