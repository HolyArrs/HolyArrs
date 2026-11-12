package mediaservers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Changes a folder can have.
const (
	Created  = "Created"
	Modified = "Modified"
	Deleted  = "Deleted"
)

// Errors from checking a media server.
var (
	ErrName     = errors.New("a media server needs a name of at most 80 characters")
	ErrNameUsed = errors.New("another media server has that name")
	ErrKind     = errors.New("kind must be plex, jellyfin or emby")
	ErrURL      = errors.New("the address must be an http or https link with a host, like http://192.168.1.10:32400")
	ErrNoKey    = errors.New("paste the server's token (Plex) or API key (Jellyfin, Emby)")
	ErrPathMap  = errors.New("give both sides of the path map, or neither")
	// ErrKeyAgain keeps a stored token from going to another server.
	ErrKeyAgain = errors.New("type the token again when the server address changes")
)

// IsInputError says err is a refusal of what was sent.
func IsInputError(err error) bool {
	for _, e := range []error{ErrName, ErrKind, ErrURL, ErrNoKey, ErrPathMap, ErrKeyAgain} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// Input is what a person sets on a media server. An empty token keeps the
// stored one.
type Input struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Enabled    *bool  `json:"enabled"`
	OnImport   *bool  `json:"on_import"`
	OnRename   *bool  `json:"on_rename"`
	OnDelete   *bool  `json:"on_delete"`
	PathFrom   string `json:"path_from"`
	PathTo     string `json:"path_to"`
	AllowLocal *bool  `json:"allow_local_network"`
}

func orKeep(p *bool, keep bool) bool {
	if p == nil {
		return keep
	}
	return *p
}

func checkURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrURL
	}
	return nil
}

func fill(in Input, m *domain.MediaServer, isNew bool) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return ErrName
	}
	if isNew {
		if in.Kind != "plex" && in.Kind != "jellyfin" && in.Kind != "emby" {
			return ErrKind
		}
		m.Kind = in.Kind
		m.Enabled, m.OnImport, m.OnRename, m.OnDelete, m.AllowLocal = true, true, true, true, true
	}
	if isNew || in.URL != "" {
		if checkURL(in.URL) != nil {
			return ErrURL
		}
		m.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	}
	if isNew && in.Token == "" {
		return ErrNoKey
	}
	from, to := strings.TrimSpace(in.PathFrom), strings.TrimSpace(in.PathTo)
	if (from == "") != (to == "") {
		return ErrPathMap
	}
	m.Name, m.PathFrom, m.PathTo = name, from, to
	m.Enabled = orKeep(in.Enabled, m.Enabled)
	m.OnImport, m.OnRename, m.OnDelete = orKeep(in.OnImport, m.OnImport), orKeep(in.OnRename, m.OnRename), orKeep(in.OnDelete, m.OnDelete)
	m.AllowLocal = orKeep(in.AllowLocal, m.AllowLocal)
	return nil
}

// Add stores a media server; its token goes to the vault.
func Add(ctx context.Context, s *database.Store, v *secrets.Vault, in Input, by domain.Actor) (domain.MediaServer, error) {
	var m domain.MediaServer
	if err := fill(in, &m, true); err != nil {
		return m, err
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, m.Name, 0); err != nil {
			return err
		}
		id, err := v.PutTx(tx, "media_server_token", strings.TrimSpace(in.Token), by)
		if err != nil {
			return err
		}
		m.TokenSecretID = &id
		return insertServer(tx, &m, by)
	})
	return m, err
}

// Change saves a media server; an empty token keeps the stored one, unless
// the address moved to another host or scheme.
func Change(ctx context.Context, s *database.Store, v *secrets.Vault, id int64, in Input, by domain.Actor) (domain.MediaServer, error) {
	m, err := getServer(ctx, s.Read(), id)
	if err != nil {
		return m, err
	}
	old := m
	if err := fill(in, &m, false); err != nil {
		return old, err
	}
	if in.Token == "" && originOf(m.URL) != originOf(old.URL) {
		return old, ErrKeyAgain
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, m.Name, m.ID); err != nil {
			return err
		}
		if in.Token != "" {
			if m.TokenSecretID != nil {
				if err := v.DeleteTx(ctx, tx, *m.TokenSecretID); err != nil {
					return err
				}
			}
			nid, err := v.PutTx(tx, "media_server_token", strings.TrimSpace(in.Token), by)
			if err != nil {
				return err
			}
			m.TokenSecretID = &nid
		}
		return updateServer(tx, &m, by)
	})
	return m, err
}

// Remove deletes a media server and its token.
func Remove(ctx context.Context, s *database.Store, v *secrets.Vault, id int64) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		m, err := getServer(ctx, tx, id)
		if err != nil {
			return err
		}
		if m.TokenSecretID != nil {
			if err := v.DeleteTx(ctx, tx, *m.TokenSecretID); err != nil {
				return err
			}
		}
		return deleteServer(tx, id)
	})
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// Linker tells media servers about changed movie, show and music folders.
type Linker struct {
	Store *database.Store
	Vault *secrets.Vault
	// Delay gathers changes before they are sent, so a rename of many files
	// is one call per folder (default 10 s).
	Delay time.Duration
	// Paused (safe mode) keeps ReadWatchHistorySoon from reading.
	Paused bool

	// reading lets one watch history pass run at a time; stopRead ends a
	// background one.
	reading  sync.Mutex
	stopRead context.CancelFunc

	mu      sync.Mutex
	pending map[string]string // folder -> change
	kinds   map[string]string // folder -> title kind (movie, show, artist)
	timer   *time.Timer
	wg      sync.WaitGroup
}

// Folders returns the movie folders of a Title (one per Version).
func (l *Linker) Folders(ctx context.Context, titleID int64) []string {
	out, _ := mediafiles.TitleFolders(ctx, l.Store.Read(), titleID)
	return out
}

// ChangeOf says which change an Event is for media servers ("" for none).
// A delete counts only when files were deleted.
func ChangeOf(e domain.Event) string {
	switch e.Kind {
	case "import_done", "import_partly_done":
		return Created
	case domain.EventRenamed:
		return Modified
	case domain.EventTitleRemoved, domain.EventEpisodesRemoved:
		if del, _ := e.Data["delete_files"].(bool); del {
			return Deleted
		}
	}
	return ""
}

// Watch listens to stored Events and gathers the folders to tell about.
func (l *Linker) Watch(ctx context.Context) {
	l.Store.OnEvent(func(e domain.Event) {
		ch := ChangeOf(e)
		if ch == "" || e.TitleID == nil {
			return
		}
		kind := mediafiles.TitleKind(ctx, l.Store.Read(), *e.TitleID)
		for _, f := range l.FoldersOf(ctx, e) {
			l.queue(ctx, f, ch, kind)
		}
	})
}

// FoldersOf returns the folders an Event changed: for an artist, the album
// folders of the tracks it names (an import's placed tracks, a rename's
// new name, a delete's folders), so a media server looks at one album, not
// the whole artist; otherwise, and when no album folder is known, the
// title's folders.
func (l *Linker) FoldersOf(ctx context.Context, e domain.Event) []string {
	db := l.Store.Read()
	kind, root, artist, err := mediafiles.MainFolder(ctx, db, *e.TitleID)
	if err != nil || kind != "artist" || artist == "" {
		return l.Folders(ctx, *e.TitleID)
	}
	var rels []string
	if raw, ok := e.Data["folders"].([]any); ok {
		for _, x := range raw {
			if s, ok := x.(string); ok {
				rels = append(rels, s)
			}
		}
	}
	if fs, ok := e.Data["folders"].([]string); ok {
		rels = append(rels, fs...)
	}
	if to, ok := e.Data["to"].(string); ok {
		rels = append(rels, to)
	}
	if len(rels) == 0 {
		for _, k := range []string{"placed_slot_ids", "slot_ids"} {
			for _, id := range idList(e.Data[k]) {
				if rel, err := mediafiles.SlotPath(ctx, db, id); err == nil {
					rels = append(rels, rel)
				}
			}
			if len(rels) > 0 {
				break
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, rel := range rels {
		parts := strings.Split(rel, "/")
		// A backslash is a separator on Windows and on a Windows media
		// server: such a path is never sent.
		if strings.Contains(rel, "\\") || len(parts) < 2 || parts[0] != artist || parts[1] == "" || parts[1] == "." || parts[1] == ".." {
			continue
		}
		dir := filepath.Join(root, parts[0], parts[1])
		if len(parts) == 2 {
			dir = filepath.Join(root, parts[0]) // a loose file in the artist folder
		}
		if !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	if len(out) == 0 {
		return l.Folders(ctx, *e.TitleID)
	}
	sort.Strings(out)
	return out
}

// idList reads a list of ids from Event data (numbers from JSON, or ids).
func idList(v any) []int64 {
	switch x := v.(type) {
	case []int64:
		return x
	case []any:
		var out []int64
		for _, y := range x {
			if f, ok := y.(float64); ok {
				out = append(out, int64(f))
			}
		}
		return out
	}
	return nil
}

// Queue gathers one folder change; they are sent together after Delay.
func (l *Linker) Queue(ctx context.Context, folder, change string) {
	l.queue(ctx, folder, change, "")
}

// queue gathers a folder change with the kind of title it belongs to, so a
// Plex scan with no library holding the folder stays with that kind.
func (l *Linker) queue(ctx context.Context, folder, change, kind string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending == nil {
		l.pending, l.kinds = map[string]string{}, map[string]string{}
	}
	if kind != "" {
		l.kinds[folder] = kind
	}
	if was, ok := l.pending[folder]; ok && was != change {
		change = Modified // several kinds piled up
	}
	l.pending[folder] = change
	if l.timer == nil {
		d := l.Delay
		if d <= 0 {
			d = 10 * time.Second
		}
		l.wg.Add(1)
		l.timer = time.AfterFunc(d, func() {
			defer l.wg.Done()
			jobs.Guard("media_servers", func() { l.Flush(context.WithoutCancel(ctx)) })
		})
	}
}

// Wait waits for a gathered batch to be sent.
func (l *Linker) Wait() { l.wg.Wait() }

// Stop sends a gathered batch now (within 10 s) instead of after the
// delay, so a shutdown does not wait the whole delay and the batch is
// never sent after the store is closed. Wait waits for it.
func (l *Linker) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopRead != nil {
		l.stopRead()
	}
	if l.timer == nil || !l.timer.Stop() {
		return // nothing gathered, or the send already started
	}
	go func() {
		defer l.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		jobs.Guard("media_servers", func() { l.Flush(ctx) })
	}()
}

// Flush sends the gathered changes to every enabled media server that
// wants them.
func (l *Linker) Flush(ctx context.Context) {
	l.mu.Lock()
	batch, kinds := l.pending, l.kinds
	l.pending, l.kinds, l.timer = nil, nil, nil
	l.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		log.Printf("media servers: %v", err)
		return
	}
	folders := make([]string, 0, len(batch))
	for f := range batch {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	for _, m := range ms {
		var send []Update
		for _, f := range folders {
			ch := batch[f]
			if (ch == Created && !m.OnImport) || (ch == Modified && !m.OnRename) || (ch == Deleted && !m.OnDelete) {
				continue
			}
			send = append(send, Update{Path: MapPath(m, f), Change: ch, Kind: kinds[f]})
		}
		if len(send) == 0 {
			continue
		}
		err := l.Tell(ctx, m, send)
		l.record(ctx, m, send, err)
	}
}

// Update is one folder to look at again, as the media server sees it.
type Update struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	// Kind is the title kind (movie, show, artist); empty counts as movie.
	Kind string `json:"-"`
}

// MapPath turns a HolyArrs folder into the media server's path with the
// server's path map.
func MapPath(m domain.MediaServer, p string) string {
	if m.PathFrom == "" {
		return p
	}
	from := strings.TrimRight(m.PathFrom, `/\`)
	// A Windows path (drive letter or \\server) ignores case.
	if len(p) < len(from) || (p[:len(from)] != from && !(winPath(from) && strings.EqualFold(p[:len(from)], from))) ||
		(len(p) > len(from) && p[len(from)] != '/' && p[len(from)] != '\\') {
		return p
	}
	rest := p[len(from):]
	to := strings.TrimRight(m.PathTo, `/\`)
	if strings.Contains(to, "/") || strings.HasPrefix(to, "/") {
		rest = strings.ReplaceAll(rest, `\`, "/")
	} else {
		rest = strings.ReplaceAll(rest, "/", `\`)
	}
	return to + rest
}

// winPath says whether p is a Windows path: a drive letter or \\server.
func winPath(p string) bool {
	return strings.HasPrefix(p, `\\`) || len(p) >= 2 && p[1] == ':' && (p[0]|0x20) >= 'a' && (p[0]|0x20) <= 'z'
}

func (l *Linker) record(ctx context.Context, m domain.MediaServer, sent []Update, sendErr error) {
	now := database.FormatTime(time.Now())
	msg := ""
	if sendErr != nil {
		msg = sendErr.Error()
	}
	if sendErr != nil {
		markError(ctx, l.Store, m.ID, msg)
	} else {
		markOK(ctx, l.Store, m.ID, now)
	}
	paths := []string{}
	for _, u := range sent {
		paths = append(paths, u.Path)
	}
	e := &domain.Event{Kind: domain.EventMediaRefreshed, Summary: fmt.Sprintf("Told %s about %d folders", m.Name, len(sent)),
		Data: map[string]any{"media_server_id": m.ID, "paths": paths, "change": sent[0].Change}, GroupKey: "media_server:" + strconv.FormatInt(m.ID, 10)}
	if sendErr != nil {
		e.Kind, e.Summary = domain.EventMediaFailed, "Could not tell "+m.Name+": "+msg
		e.Data["error"] = msg
		log.Printf("media servers: %s: %s", m.Name, msg)
	}
	_ = l.Store.AddEvent(context.WithoutCancel(ctx), e)
}

func (l *Linker) token(ctx context.Context, m domain.MediaServer) (string, error) {
	if m.TokenSecretID == nil || l.Vault == nil {
		return "", ErrNoKey
	}
	t, err := l.Vault.Get(ctx, *m.TokenSecretID)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("the token could not be read")
	}
	return t, nil
}

// client never follows redirects (the token stays with the server) and
// reaches the local network only when the server may.
func client(m domain.MediaServer) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	if m.AllowLocal {
		// The local network, but never link-local or cloud metadata.
		t.DialContext = outbound.PolicyDialer(outbound.NotMetadata).DialContext
	} else {
		t.DialContext = outbound.PolicyDialer(outbound.PublicOnly).DialContext
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: t,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// maxAnswer is the most bytes of one media server answer read.
const maxAnswer = 4 << 20

// do sends one request with the server's token and returns the body. The
// error never holds the address or the token.
func (l *Linker) do(ctx context.Context, m domain.MediaServer, method, path string, body []byte) ([]byte, error) {
	tok, err := l.token(ctx, m)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "media_server", Identity: []string{strconv.FormatInt(m.ID, 10), m.Kind, m.URL, tok}})
	if err != nil {
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, method, m.URL+path, rd)
	if err != nil {
		outcome = outbound.TerminalFailure
		return nil, ErrURL
	}
	req.Header.Set("Accept", "application/json")
	if m.Kind == "plex" {
		req.Header.Set("X-Plex-Token", tok)
		req.Header.Set("X-Plex-Client-Identifier", "holyarrs")
		req.Header.Set("Accept", "application/xml")
	} else if m.Kind == "jellyfin" {
		req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Client="HolyArrs", Device="HolyArrs", DeviceId="holyarrs", Version="1", Token=%q`, tok))
	} else {
		req.Header.Set("X-MediaBrowser-Token", tok)
		req.Header.Set("X-Emby-Token", tok)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client(m).Do(req)
	if err != nil {
		outcome = outbound.HTTPOutcome(0, err, ctx)
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return nil, errors.New("the server is on the local network, which it may not reach")
		}
		return nil, errors.New("the server did not answer")
	}
	b, readErr := io.ReadAll(io.LimitReader(resp.Body, maxAnswer+1))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		outcome = outbound.HTTPOutcome(resp.StatusCode, readErr, ctx)
		return nil, errors.New("reading the server answer failed")
	}
	if len(b) > maxAnswer {
		// A cut answer would read as a short list.
		outcome = outbound.TerminalFailure
		return nil, errors.New("the server's answer was too big to read")
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.New("the server turned down the token")
	case resp.StatusCode/100 != 2:
		return nil, fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	return b, nil
}

// plexSection is one Plex library and its folders.
type plexSection struct {
	Key       string `xml:"key,attr"`
	Type      string `xml:"type,attr"`
	Title     string `xml:"title,attr"`
	Locations []struct {
		Path string `xml:"path,attr"`
	} `xml:"Location"`
}

func (l *Linker) plexSections(ctx context.Context, m domain.MediaServer) ([]plexSection, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/library/sections", nil)
	if err != nil {
		return nil, err
	}
	var mc struct {
		Directories []plexSection `xml:"Directory"`
	}
	if err := xml.Unmarshal(b, &mc); err != nil {
		return nil, errors.New("the server's answer was not a Plex library list")
	}
	var out []plexSection
	for _, d := range mc.Directories {
		if d.Type == "movie" || d.Type == "show" || d.Type == "artist" {
			out = append(out, d)
		}
	}
	return out, nil
}

// plexType is the Plex library type for a title kind.
func plexType(kind string) string {
	if kind == "show" || kind == "artist" {
		return kind
	}
	return "movie"
}

// Tell asks the server to look at each folder again. Plex scans the movie,
// TV or music library that holds the folder, only that folder; with no
// library holding it, every library of the title's type is scanned. Jellyfin and Emby get one media
// updated call with every folder and its change. Jellyfin then gets one full
// library refresh for the batch because Jellyfin 12.2 accepts the path report
// without reliably indexing a newly created folder.
func (l *Linker) Tell(ctx context.Context, m domain.MediaServer, us []Update) error {
	if len(us) == 0 {
		return nil
	}
	if m.Kind == "plex" {
		secs, err := l.plexSections(ctx, m)
		if err != nil {
			return err
		}
		for _, u := range us {
			matched := false
			for _, s := range secs {
				for _, loc := range s.Locations {
					root := strings.TrimRight(loc.Path, `/\`)
					if u.Path == root || strings.HasPrefix(u.Path, root+"/") || strings.HasPrefix(u.Path, root+`\`) {
						matched = true
						if _, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/refresh?path="+url.QueryEscape(u.Path), nil); err != nil {
							return err
						}
					}
				}
			}
			if !matched {
				for _, s := range secs {
					if s.Type != plexType(u.Kind) {
						continue
					}
					if _, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/refresh", nil); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	type upd struct {
		Path       string `json:"Path"`
		UpdateType string `json:"UpdateType"`
	}
	var body struct {
		Updates []upd `json:"Updates"`
	}
	for _, u := range us {
		body.Updates = append(body.Updates, upd{Path: u.Path, UpdateType: u.Change})
	}
	b, _ := json.Marshal(body)
	if _, err := l.do(ctx, m, http.MethodPost, "/Library/Media/Updated", b); err != nil {
		return err
	}
	if m.Kind == "jellyfin" {
		_, err := l.do(ctx, m, http.MethodPost, "/Library/Refresh", nil)
		return err
	}
	return nil
}

// Test checks the address and token: Plex must list at least one movie,
// TV or music library; Jellyfin and Emby must answer their system info.
func (l *Linker) Test(ctx context.Context, m domain.MediaServer) (string, error) {
	var res string
	var err error
	if m.Kind == "plex" {
		var secs []plexSection
		secs, err = l.plexSections(ctx, m)
		if err == nil && len(secs) == 0 {
			err = errors.New("the server has no movie, TV or music library")
		}
		if err == nil {
			names := []string{}
			for _, s := range secs {
				names = append(names, s.Title)
			}
			res = "Libraries: " + strings.Join(names, ", ")
		}
	} else {
		var b []byte
		b, err = l.do(ctx, m, http.MethodGet, "/System/Info", nil)
		if err == nil {
			var info struct {
				ServerName string `json:"ServerName"`
				Version    string `json:"Version"`
			}
			_ = json.Unmarshal(b, &info)
			res = strings.TrimSpace(info.ServerName + " " + info.Version)
		}
	}
	now := database.FormatTime(time.Now())
	if err != nil {
		log.Printf("media servers: %s: test: %v", m.Name, err)
		markError(ctx, l.Store, m.ID, err.Error())
	} else {
		markOK(ctx, l.Store, m.ID, now)
	}
	return res, err
}
