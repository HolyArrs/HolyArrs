package refresh

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MetaBrainz replication endpoints; the token is sent as a query parameter
// and never logged (errors carry no URL).
const (
	MBInfoURL   = "https://metabrainz.org/api/musicbrainz/replication-info"
	MBPacketURL = "https://metabrainz.org/api/musicbrainz/replication-%d.tar.bz2"
	mbCursor    = "cursor:musicbrainz"
)

// MBFeed applies MusicBrainz replication packets (hourly): every row change
// on an entity table carries the entity's gid (MBID), which marks the held
// rows "musicbrainz:<mbid>" stale. Off without Token.
type MBFeed struct {
	Tracker   *Tracker
	HTTP      *http.Client
	Token     string
	InfoURL   string
	PacketURL string // with one %d for the packet number
	UserAgent string
	Now       func() time.Time
	// MaxPackets bounds packets applied per run (default 48).
	MaxPackets int
	// OnChanged, when set, gets every packet's changed MBIDs too (the
	// discography store marks the lists and albums they touch).
	OnChanged func(ctx context.Context, mbids []string)
}

// Enabled reports whether the token is set.
func (f *MBFeed) Enabled() bool { return f != nil && strings.TrimSpace(f.Token) != "" }

var (
	mbPacketNumber = regexp.MustCompile(`replication-([0-9]+)`)
	mbGID          = regexp.MustCompile(`"gid"='([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})'`)
	mbTables       = map[string]bool{"artist": true, "release_group": true, "release": true, "recording": true, "label": true, "work": true}
)

// Poll applies every packet after the saved one, oldest first. The cursor
// moves after each packet, so a failure resumes where it stopped.
func (f *MBFeed) Poll(ctx context.Context) (marked int, err error) {
	if !f.Enabled() {
		return 0, nil
	}
	last, err := f.lastPacket(ctx)
	if err != nil {
		f.Tracker.Set(ctx, "feed:musicbrainz", "failed "+f.Now().UTC().Format(time.RFC3339))
		return 0, err
	}
	saved, _ := strconv.Atoi(f.Tracker.Get(ctx, mbCursor))
	if saved <= 0 || saved > last {
		f.Tracker.Set(ctx, "feed:musicbrainz", "ok "+f.Now().UTC().Format(time.RFC3339))
		return 0, f.Tracker.Set(ctx, mbCursor, strconv.Itoa(last))
	}
	limit := f.MaxPackets
	if limit <= 0 {
		limit = 48
	}
	for n := saved + 1; n <= last && n <= saved+limit; n++ {
		gids, err := f.packet(ctx, n)
		if err != nil {
			f.Tracker.Set(ctx, "feed:musicbrainz", "failed "+f.Now().UTC().Format(time.RFC3339))
			return marked, err
		}
		refs := make([]string, 0, len(gids))
		ids := make([]string, 0, len(gids))
		for g := range gids {
			refs = append(refs, "musicbrainz:"+g)
			ids = append(ids, g)
		}
		if f.OnChanged != nil {
			f.OnChanged(ctx, ids)
		}
		m, err := f.Tracker.MarkRefs(ctx, refs)
		if err != nil {
			return marked, err
		}
		marked += m
		if err := f.Tracker.Set(ctx, mbCursor, strconv.Itoa(n)); err != nil {
			return marked, err
		}
	}
	f.Tracker.Set(ctx, "feed:musicbrainz", "ok "+f.Now().UTC().Format(time.RFC3339))
	return marked, nil
}

func (f *MBFeed) get(ctx context.Context, base string) (*http.Response, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, errors.New("musicbrainz feed: bad url")
	}
	q := u.Query()
	q.Set("token", strings.TrimSpace(f.Token))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("musicbrainz feed: bad request")
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("musicbrainz feed: unavailable")
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, errors.New("musicbrainz feed: status " + strconv.Itoa(resp.StatusCode))
	}
	return resp, nil
}

func (f *MBFeed) lastPacket(ctx context.Context) (int, error) {
	base := f.InfoURL
	if base == "" {
		base = MBInfoURL
	}
	resp, err := f.get(ctx, base)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var info struct {
		LastPacket string `json:"last_packet"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil {
		return 0, errors.New("musicbrainz feed: bad info")
	}
	m := mbPacketNumber.FindStringSubmatch(info.LastPacket)
	if m == nil {
		return 0, errors.New("musicbrainz feed: bad info")
	}
	return strconv.Atoi(m[1])
}

func (f *MBFeed) packet(ctx context.Context, n int) (map[string]bool, error) {
	base := f.PacketURL
	if base == "" {
		base = MBPacketURL
	}
	resp, err := f.get(ctx, strings.Replace(base, "%d", strconv.Itoa(n), 1))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ParsePacket(io.LimitReader(resp.Body, 2<<30))
}

// ParsePacket reads a replication packet (tar.bz2, or a plain tar) and
// returns the MBIDs of changed entity rows.
func ParsePacket(r io.Reader) (map[string]bool, error) {
	br := bufio.NewReader(r)
	var src io.Reader = br
	if magic, _ := br.Peek(3); string(magic) == "BZh" {
		src = bzip2.NewReader(br)
	}
	tr := tar.NewReader(src)
	tables := map[string]string{} // seq id -> table
	var data [][2]string          // seq id, row data
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("musicbrainz feed: bad packet")
		}
		switch strings.TrimPrefix(h.Name, "./") {
		case "mbdump/dbmirror_pending":
			sc := bufio.NewScanner(tr)
			sc.Buffer(make([]byte, 1<<16), 1<<24)
			for sc.Scan() {
				p := strings.Split(sc.Text(), "\t")
				if len(p) >= 2 {
					name := p[1]
					if i := strings.LastIndex(name, "."); i >= 0 {
						name = name[i+1:]
					}
					tables[p[0]] = strings.Trim(name, `"`)
				}
			}
			if sc.Err() != nil {
				return nil, errors.New("musicbrainz feed: bad packet")
			}
		case "mbdump/dbmirror_pendingdata":
			sc := bufio.NewScanner(tr)
			sc.Buffer(make([]byte, 1<<16), 1<<24)
			for sc.Scan() {
				p := strings.SplitN(sc.Text(), "\t", 3)
				if len(p) == 3 {
					data = append(data, [2]string{p[0], p[2]})
				}
			}
			if sc.Err() != nil {
				return nil, errors.New("musicbrainz feed: bad packet")
			}
		}
	}
	out := map[string]bool{}
	for _, d := range data {
		if !mbTables[tables[d[0]]] {
			continue
		}
		if m := mbGID.FindStringSubmatch(d[1]); m != nil {
			out[m[1]] = true
		}
	}
	return out, nil
}
