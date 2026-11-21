package sources

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// slskd searches the Soulseek network itself, so a slskd Source is a built-in
// site that asks its own slskd. Each user's folder of matching audio files
// becomes one Release, and its link carries the user and the file list for a
// slskd Downloader to fetch.

// SlskdScheme starts every slskd Release link.
const SlskdScheme = "slskd:"

// slskdWait is how long slskd listens for answers after the last one came in.
// slskd counts it in milliseconds.
const slskdWait = 15 * time.Second

// slskdPoll is how often and how long a search is read back before its answers are taken as they stand.
var slskdPoll = struct {
	gap  time.Duration
	most time.Duration
}{time.Second, 45 * time.Second}

// slskdAudio are the file endings a folder's tracks may have. Covers, logs and
// cue sheets are left out of the Release.
var slskdAudio = map[string]bool{"flac": true, "mp3": true, "m4a": true, "aac": true, "ogg": true, "opus": true,
	"wav": true, "aiff": true, "aif": true, "alac": true, "ape": true, "wv": true, "dsf": true, "dff": true}

// SlskdFile is one file a Soulseek user shares, as slskd names it: the user's full path.
type SlskdFile struct {
	Name string `json:"n"`
	Size int64  `json:"s"`
}

// SlskdRelease is what a slskd link holds.
type SlskdRelease struct {
	User  string      `json:"u"`
	Files []SlskdFile `json:"f"`
}

// SlskdLink packs a user's files into an opaque Release link.
func SlskdLink(r SlskdRelease) string {
	b, _ := json.Marshal(r)
	return SlskdScheme + base64.RawURLEncoding.EncodeToString(b)
}

// ParseSlskdLink reads a link SlskdLink made.
func ParseSlskdLink(link string) (SlskdRelease, error) {
	var r SlskdRelease
	raw, ok := strings.CutPrefix(link, SlskdScheme)
	if !ok {
		return r, errors.New("not a slskd link")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(b, &r) != nil || r.User == "" || len(r.Files) == 0 {
		return SlskdRelease{}, errors.New("the slskd link is damaged")
	}
	// One Release is one folder, so every file shares the first one's folder.
	dir := path.Dir(slskdSlash(r.Files[0].Name))
	for _, f := range r.Files {
		if f.Name == "" || f.Size < 0 || path.Dir(slskdSlash(f.Name)) != dir {
			return SlskdRelease{}, errors.New("the slskd link is damaged")
		}
	}
	return r, nil
}

type slskdFileAnswer struct {
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
	Extension  string `json:"extension"`
	BitRate    *int   `json:"bitRate"`
	BitDepth   *int   `json:"bitDepth"`
	SampleRate *int   `json:"sampleRate"`
	IsVBR      *bool  `json:"isVariableBitRate"`
	IsLocked   bool   `json:"isLocked"`
}

type slskdResponse struct {
	Username          string            `json:"username"`
	Files             []slskdFileAnswer `json:"files"`
	HasFreeUploadSlot bool              `json:"hasFreeUploadSlot"`
	UploadSpeed       int64             `json:"uploadSpeed"`
	QueueLength       int64             `json:"queueLength"`
}

func (c *Client) slskdHeader() http.Header {
	return http.Header{"X-API-Key": {c.cfg.APIKey}, "Content-Type": {"application/json"}, "Accept": {"application/json"}}
}

// slskdTest asks slskd for its own state, which any key may read.
func (c *Client) slskdTest(ctx context.Context, base string) error {
	if c.cfg.APIKey == "" {
		return &Error{Kind: KindLoginRefused, Msg: "slskd needs an API key with the readwrite role"}
	}
	raw, err := c.send(ctx, http.MethodGet, base+"/api/v0/application", nil, c.slskdHeader())
	if err != nil {
		return err
	}
	var app struct {
		Server *struct {
			State string `json:"state"`
		} `json:"server"`
	}
	if json.Unmarshal(raw, &app) != nil || app.Server == nil {
		return &Error{Kind: KindBroken, Msg: "the address answered, but not like slskd"}
	}
	if !strings.Contains(app.Server.State, "LoggedIn") {
		return &Error{Kind: KindDown, Msg: "slskd is not signed in to the Soulseek network (" + app.Server.State + ")"}
	}
	return nil
}

// slskdSearch runs one search in slskd, waits for its answers and turns every
// user's folder of audio files into an Item. It has no newest list.
func (c *Client) slskdSearch(ctx context.Context, base, text, artist string) ([]Item, error) {
	if text == "" {
		return nil, nil
	}
	if c.cfg.APIKey == "" {
		return nil, &Error{Kind: KindLoginRefused, Msg: "slskd needs an API key with the readwrite role"}
	}
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{"id": id, "searchText": text, "searchTimeout": slskdWait.Milliseconds(), "responseLimit": 100, "fileLimit": 10000})
	if _, err := c.send(ctx, http.MethodPost, base+"/api/v0/searches", body, c.slskdHeader()); err != nil {
		return nil, err
	}
	// The search is slskd's to keep only while it is read; it goes once read or abandoned.
	defer func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, _ = c.send(dctx, http.MethodDelete, base+"/api/v0/searches/"+id, nil, c.slskdHeader())
	}()
	until := time.Now().Add(slskdPoll.most)
	for {
		raw, err := c.send(ctx, http.MethodGet, base+"/api/v0/searches/"+id, nil, c.slskdHeader())
		if err != nil {
			return nil, err
		}
		var st struct {
			IsComplete bool `json:"isComplete"`
		}
		if json.Unmarshal(raw, &st) != nil {
			return nil, &Error{Kind: KindBroken, Msg: "slskd answered with something that is not a search"}
		}
		if st.IsComplete || time.Now().After(until) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(slskdPoll.gap):
		}
	}
	raw, err := c.send(ctx, http.MethodGet, base+"/api/v0/searches/"+id+"/responses", nil, c.slskdHeader())
	if err != nil {
		return nil, err
	}
	return parseSlskd(raw, artist)
}

func parseSlskd(raw []byte, artist string) ([]Item, error) {
	var rs []slskdResponse
	if err := json.Unmarshal(raw, &rs); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "slskd answered with something that is not its search answers"}
	}
	var out []Item
	for _, r := range rs {
		if r.Username == "" {
			continue
		}
		folders := map[string][]slskdFileAnswer{}
		for _, f := range r.Files {
			ext := strings.ToLower(strings.TrimPrefix(f.Extension, "."))
			if ext == "" {
				ext = strings.ToLower(strings.TrimPrefix(path.Ext(slskdSlash(f.Filename)), "."))
			}
			if f.IsLocked || !slskdAudio[ext] || f.Size <= 0 {
				continue
			}
			dir := path.Dir(slskdSlash(f.Filename))
			folders[dir] = append(folders[dir], f)
		}
		dirs := make([]string, 0, len(folders))
		for d := range folders {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			files := folders[dir]
			sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
			rel := SlskdRelease{User: r.Username}
			var size int64
			for _, f := range files {
				rel.Files = append(rel.Files, SlskdFile{Name: f.Filename, Size: f.Size})
				size += f.Size
			}
			// A free slot means it starts now; the queue stands in for peers so a busy user ranks lower.
			seeds, peers := 0, int(min(r.QueueLength, 1_000_000))
			if r.HasFreeUploadSlot {
				seeds = 1
			}
			out = append(out, Item{Title: slskdName(dir, artist, files), GUID: "slskd:" + r.Username + "/" + dir, Link: SlskdLink(rel),
				Size: size, Protocol: "soulseek", Files: len(files), Seeders: &seeds, Peers: &peers})
		}
	}
	return out, nil
}

// slskdSlash turns a Windows share path into a slash path; most Soulseek users share from Windows.
func slskdSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// slskdName names a folder like a music release, "Artist - Album (Year) [FLAC 24bit]",
// so the name reader finds the album and the quality. A folder often holds only
// the album, so the folder above it (usually the artist) leads when the folder
// does not name the searched artist.
func slskdName(dir, artist string, files []slskdFileAnswer) string {
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && !strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(artist))) {
		name = parts[len(parts)-2] + " - " + name
	}
	if tag := slskdQuality(files); tag != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(strings.Fields(tag)[0])) {
		name += " [" + tag + "]"
	}
	return name
}

// slskdQuality is the format of most of the files, with the bit depth for
// lossless and the bitrate for MP3: "FLAC 24bit", "MP3 320", "MP3 VBR".
func slskdQuality(files []slskdFileAnswer) string {
	count := map[string]int{}
	for _, f := range files {
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(slskdSlash(f.Filename)), "."))
		count[ext]++
	}
	best := ""
	for ext, n := range count {
		if n > count[best] || n == count[best] && ext < best {
			best = ext
		}
	}
	if best == "" {
		return ""
	}
	tag := strings.ToUpper(best)
	switch best {
	case "flac", "alac", "wav", "aiff", "aif", "ape", "wv":
		for _, f := range files {
			if f.BitDepth != nil && *f.BitDepth >= 24 {
				return tag + " 24bit"
			}
		}
	case "mp3":
		vbr, rate := false, 0
		for _, f := range files {
			if f.IsVBR != nil && *f.IsVBR {
				vbr = true
			}
			if f.BitRate != nil && (rate == 0 || *f.BitRate < rate) {
				rate = *f.BitRate
			}
		}
		switch {
		case vbr:
			return tag + " VBR"
		case rate > 0:
			return fmt.Sprintf("%s %d", tag, rate)
		}
	}
	return tag
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
