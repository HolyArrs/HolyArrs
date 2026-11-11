package downloads

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// rtorrentNoHash is a target no torrent has. The connection test sends write
// calls to it to learn whether rTorrent allows them, without changing anything.
const rtorrentNoHash = "0000000000000000000000000000000000000000"

// rtorrentConfirm is how often and how long an add is looked for. rTorrent
// answers every load with 0, so only finding the hash proves the add.
var rtorrentConfirm = struct {
	tries int
	every time.Duration
}{10, 500 * time.Millisecond}

// rtorrentAnswerMax caps an answer. A large rTorrent lists thousands of torrents
// in one d.multicall2, as large as qBittorrent's full list.
const rtorrentAnswerMax = 64 << 20

// rtorrentFields are the d.multicall2 fields, in the order rtRow reads them.
var rtorrentFields = []string{
	"d.hash=", "d.name=", "d.custom1=", "d.size_bytes=", "d.completed_bytes=", "d.down.rate=", "d.up.rate=", "d.up.total=",
	"d.ratio=", "d.state=", "d.is_open=", "d.is_active=", "d.complete=", "d.hashing=", "d.message=", "d.base_path=",
	"d.directory=", "d.is_multi_file=", "d.peers_connected=", "d.peers_complete=", "d.timestamp.finished=",
}

// RTorrent is a Client for rTorrent's XML-RPC interface, reached over HTTP: the
// /RPC2 mount of the web server in front of it, or ruTorrent's RPC plugins.
type RTorrent struct {
	Category string
	Others   []string // more categories that are also ours
	// PathMap and SaveRoot tell where HolyArrs sees the files, so a remove that
	// asks for the files can delete them here. rTorrent cannot delete them.
	PathMap  []domain.PathMap
	SaveRoot string
	// Keep lists folders that are never deleted, nor anything in or around
	// them: the Library folders. Without it nothing is deleted.
	Keep func(ctx context.Context) ([]string, error)

	endpoint, user, pass string
	http                 *http.Client
	now                  func() time.Time
}

// rtRow is one torrent as d.multicall2 or a single-torrent read reports it.
type rtRow struct {
	hash, name, label, message, basePath, dir                   string
	size, done, down, up, upTotal, ratio, finished              int64
	state, open, active, complete, hashing, multi, conns, seeds int64
}

// NewRTorrent returns a client for the rTorrent at base. An address with no
// path gets /RPC2, the usual mount.
func NewRTorrent(base, user, pass string, h *http.Client) *RTorrent {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	base = strings.TrimSpace(base)
	if u, err := url.Parse(base); err == nil && u.Host != "" && (u.Path == "" || u.Path == "/") {
		u.Path = "/RPC2"
		base = u.String()
	}
	return &RTorrent{endpoint: base, user: user, pass: pass, http: h, now: time.Now}
}

// Health reports whether rTorrent answers, accepts the sign-in and allows the
// calls HolyArrs makes.
func (r *RTorrent) Health(ctx context.Context) (Health, error) {
	v, err := r.call(ctx, "system.client_version", "")
	if err != nil {
		return Health{}, err
	}
	version := asStr(v)
	if version == "" {
		return Health{}, errors.New("the address answered XML-RPC, but did not say an rTorrent version")
	}
	major, rest, _ := strings.Cut(version, ".")
	minor, _, _ := strings.Cut(rest, ".")
	if a, err := strconv.Atoi(major); err == nil {
		if b, err := strconv.Atoi(minor); err == nil && a == 0 && b < 9 {
			return Health{}, fmt.Errorf("rTorrent %s is too old; HolyArrs needs 0.9 or newer", version)
		}
	}
	if _, err := r.rows(ctx); err != nil {
		return Health{}, err
	}
	answers, err := r.multicall(ctx, [][]any{{"d.custom1.set", rtorrentNoHash, "holyarrs"}, {"d.stop", rtorrentNoHash}, {"d.erase", rtorrentNoHash}})
	if err != nil {
		return Health{}, err
	}
	for _, a := range answers {
		var f *xmlrpcFault
		if e, ok := a.(error); ok && errors.As(e, &f) && f.untrusted() {
			return Health{}, untrustedError(f)
		}
	}
	return Health{Version: version, At: time.Now().UTC()}, nil
}

// List returns the torrents with the category's label. An empty category
// returns every torrent.
func (r *RTorrent) List(ctx context.Context, category string) ([]Job, error) {
	rows, err := r.rows(ctx)
	if err != nil {
		return nil, err
	}
	jobs := []Job{}
	for _, row := range rows {
		if category == "" || labelIs(row.label, category) {
			jobs = append(jobs, r.job(row))
		}
	}
	return jobs, nil
}

// Get returns the torrent with the given id, with its files.
func (r *RTorrent) Get(ctx context.Context, id string) (Job, error) {
	row, err := r.row(ctx, id)
	if err != nil {
		return Job{}, err
	}
	j := r.job(row)
	v, err := r.call(ctx, "f.multicall", strings.ToUpper(row.hash), "", "f.path=", "f.size_bytes=", "f.completed_chunks=", "f.size_chunks=")
	if err != nil {
		return Job{}, rtNotFound(err)
	}
	files, _ := v.([]any)
	for _, f := range files {
		cols, _ := f.([]any)
		if len(cols) < 4 {
			continue
		}
		name := safeRelName(asStr(cols[0]))
		if name == "" {
			continue
		}
		if row.multi == 1 {
			if folder := path.Base(row.content()); row.content() != "" {
				name = folder + "/" + name
			}
		}
		progress := float64(0)
		if total := asInt(cols[3]); total > 0 {
			progress = max(0, min(1, float64(asInt(cols[2]))/float64(total)))
		}
		j.Files = append(j.Files, File{Name: name, Size: max(0, asInt(cols[1])), Progress: progress})
	}
	return j, nil
}

// Add sends a torrent with the category's label and returns its hash. HolyArrs
// fetches a .torrent itself and sends its bytes, so rTorrent never contacts the
// Source.
func (r *RTorrent) Add(ctx context.Context, req AddRequest) (string, error) {
	if req.Category == "" {
		return "", ErrCategory
	}
	expected := strings.ToLower(req.InfoHash)
	if expected != "" && !releasename.ValidInfoHash(expected) {
		return "", errBadHash
	}
	if req.ReconcileOnly {
		if expected == "" || req.Skip[expected] {
			return "", ErrUncertain
		}
		j, err := r.Get(ctx, expected)
		if err != nil || !labelIs(j.Category, req.Category) || req.Skip[j.ID] {
			return "", errors.Join(ErrUncertain, err)
		}
		return j.ID, nil
	}
	var method string
	var payload any
	if h, ok := magnetHash(req.URL); ok {
		if expected != "" && expected != h {
			return "", errors.New("the magnet and the expected info hash do not match")
		}
		expected, method, payload = h, "load.start", req.URL
	} else {
		b, err := req.File, error(nil)
		if len(b) == 0 {
			b, err = fetchRelease(ctx, req.URL, req.AllowLocal, torrentFetchMax)
		}
		if err != nil {
			return "", err
		}
		if !req.AllowLocal {
			if b, err = safeTorrent(b); err != nil {
				return "", err
			}
		}
		h, err := torrentFileHash(b)
		if err != nil {
			return "", err
		}
		if expected != "" && expected != h {
			return "", errors.New("the torrent file does not match its info hash")
		}
		expected, method, payload = h, "load.raw_start", b
	}
	if req.Skip[expected] {
		return "", errors.New("the torrent belongs to another download")
	}
	if j, err := r.Get(ctx, expected); err == nil {
		if !labelIs(j.Category, req.Category) {
			return "", errors.New("the torrent is already in rTorrent under another label")
		}
		return j.ID, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	args := []any{"", payload, "d.custom1.set=" + labelValue(req.Category)}
	if dir, ok := r.saveDir(req.SavePath); ok {
		args = append(args, `d.directory.set="`+dir+`"`)
	}
	_, sendErr := r.call(ctx, method, args...)
	var fault *xmlrpcFault
	if errors.As(sendErr, &fault) {
		if fault.untrusted() {
			return "", untrustedError(fault)
		}
		return "", sendErr
	}
	var refusedSize rtorrentTooLarge
	if errors.Is(sendErr, ErrAuth) || errors.Is(sendErr, outbound.ErrAddressRefused) || errors.As(sendErr, &refusedSize) {
		return "", sendErr
	}
	for i := 0; i < rtorrentConfirm.tries && ctx.Err() == nil; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(rtorrentConfirm.every):
			}
		}
		row, err := r.row(ctx, expected)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return "", errors.Join(ErrUncertain, sendErr, err)
		}
		if row.label == "" {
			// Seen without the label that went with the load: put it back.
			if _, err := r.call(ctx, "d.custom1.set", strings.ToUpper(expected), labelValue(req.Category)); err != nil {
				return "", errors.Join(ErrUncertain, err)
			}
			row.label = labelValue(req.Category)
		}
		if !labelIs(row.label, req.Category) {
			return "", errors.Join(ErrUncertain, errors.New("the torrent is in rTorrent under another label"))
		}
		return expected, nil
	}
	return "", errors.Join(ErrUncertain, sendErr, errors.New("rTorrent did not show the torrent yet"))
}

// Remove takes the torrent out of rTorrent. rTorrent keeps the files, so when
// deleteFiles is set HolyArrs deletes them itself where it can see them.
func (r *RTorrent) Remove(ctx context.Context, id string, deleteFiles bool) error {
	row, err := r.row(ctx, id)
	if err != nil {
		return err
	}
	if r.Category != "" && !labelIs(row.label, r.Category) && !slices.ContainsFunc(r.Others, func(c string) bool { return labelIs(row.label, c) }) {
		return errors.New("refusing to remove a torrent outside the configured category")
	}
	var target filesTarget
	why := ""
	if deleteFiles {
		others, err := r.rows(ctx)
		if err != nil {
			return err
		}
		var keep []string
		if r.Keep == nil {
			why = "HolyArrs does not know its Library folders here"
		} else if keep, err = r.Keep(ctx); err != nil {
			why = "HolyArrs could not read its Library folders"
		}
		if why == "" {
			target, why = r.deleteTarget(row, others, keep)
		}
	}
	if _, err := r.call(ctx, "d.erase", strings.ToUpper(row.hash)); err != nil {
		var f *xmlrpcFault
		if !(errors.As(err, &f) && f.unknownHash()) {
			if _, getErr := r.row(ctx, row.hash); !errors.Is(getErr, ErrNotFound) {
				return err
			}
		}
	}
	if !deleteFiles {
		return nil
	}
	if why == "" {
		why = target.remove()
	}
	if why != "" {
		logf("rtorrent: removed %s but kept its files: %s", row.hash, why)
	}
	return nil
}

func (r *RTorrent) row(ctx context.Context, id string) (rtRow, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return rtRow{}, errBadHash
	}
	calls := make([][]any, len(rtorrentFields))
	for i, f := range rtorrentFields {
		calls[i] = []any{strings.TrimSuffix(f, "="), strings.ToUpper(id)}
	}
	answers, err := r.multicall(ctx, calls)
	if err != nil {
		return rtRow{}, err
	}
	for _, a := range answers {
		if e, ok := a.(error); ok {
			return rtRow{}, rtNotFound(e)
		}
	}
	return rowOf(answers)
}

func (r *RTorrent) rows(ctx context.Context) ([]rtRow, error) {
	args := []any{"", "main"}
	for _, f := range rtorrentFields {
		args = append(args, f)
	}
	v, err := r.call(ctx, "d.multicall2", args...)
	if err != nil {
		return nil, err
	}
	list, ok := v.([]any)
	if !ok {
		return nil, errXMLRPCAnswer
	}
	out := make([]rtRow, 0, len(list))
	for _, item := range list {
		cols, _ := item.([]any)
		row, err := rowOf(cols)
		if err != nil || !releasename.ValidInfoHash(row.hash) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func rowOf(cols []any) (rtRow, error) {
	if len(cols) < len(rtorrentFields) {
		return rtRow{}, errXMLRPCAnswer
	}
	n := func(i int) int64 { return asInt(cols[i]) }
	return rtRow{
		hash: strings.ToLower(asStr(cols[0])), name: asStr(cols[1]), label: asStr(cols[2]), size: n(3), done: n(4),
		down: n(5), up: n(6), upTotal: n(7), ratio: n(8), state: n(9), open: n(10), active: n(11), complete: n(12),
		hashing: n(13), message: asStr(cols[14]), basePath: asStr(cols[15]), dir: asStr(cols[16]), multi: n(17),
		conns: n(18), seeds: n(19), finished: n(20),
	}, nil
}

// content is where the torrent's data is, as rTorrent sees it. Older builds
// leave d.base_path empty for a stopped torrent, so it is rebuilt from the
// folder and the name.
func (row rtRow) content() string {
	p := row.basePath
	if p == "" && row.dir != "" {
		p = row.dir
		if row.multi != 1 && row.name != "" {
			p = path.Join(row.dir, row.name)
		}
	}
	if p == "" || !path.IsAbs(p) {
		return ""
	}
	return path.Clean(p)
}

func (r *RTorrent) job(row rtRow) Job {
	content := row.content()
	state, detail := rtState(row, content)
	progress := float64(0)
	if row.size > 0 {
		progress = max(0, min(1, float64(row.done)/float64(row.size)))
	}
	if row.complete == 1 {
		progress = 1
	}
	eta := int64(0)
	if row.down > 0 && row.size > row.done {
		eta = (row.size - row.done) / row.down
	}
	j := Job{ID: row.hash, Name: row.name, Category: labelText(row.label), ContentPath: content, Size: max(0, row.size),
		Progress: progress, Speed: max(0, row.down), UpSpeed: max(0, row.up), Seeds: count32(row.seeds),
		Peers: count32(row.conns - row.seeds), ETA: eta, State: state, Detail: detail, Ratio: float64(max(0, row.ratio)) / 1000,
		Uploaded: max(0, row.upTotal)}
	if content != "" {
		j.SavePath = path.Dir(content)
	}
	if row.complete == 1 && row.finished > 0 {
		j.DoneAt = row.finished
		if now := r.now().Unix(); now > row.finished {
			j.SeedingTime = now - row.finished
		}
	}
	if strings.HasPrefix(strings.ToLower(row.message), "tracker") {
		j.TrackerProblem = "warning"
	}
	return j
}

// rtState maps rTorrent's flags onto ours. rTorrent has no failed state; a
// storage error closes the torrent and leaves the reason in d.message.
func rtState(row rtRow, content string) (string, string) {
	msg := strings.ToLower(row.message)
	switch {
	case row.hashing != 0:
		return StateQueued, "checking"
	case row.complete == 0 && (row.size == 0 || strings.HasSuffix(row.name, ".meta")):
		return StateDownloading, "metadata"
	case row.state == 1 && row.open == 0 && row.message != "" && !strings.HasPrefix(msg, "tracker"):
		return StatePaused, "error: " + redactURLs(cleanDetail(row.message))
	case row.complete == 1 && content != "":
		if row.state == 1 && row.active == 1 {
			return StateFinished, "seeding"
		}
		return StateFinished, "complete"
	case row.state == 0 || row.open == 0:
		return StatePaused, "stopped"
	case row.active == 0:
		return StatePaused, "paused"
	}
	return StateDownloading, "downloading"
}

// saveDir is the folder sent with a load. A Downloader with path pairs sees
// other paths than HolyArrs, so it keeps rTorrent's own folder instead.
func (r *RTorrent) saveDir(p string) (string, bool) {
	if p == "" || len(r.PathMap) > 0 || !path.IsAbs(p) || strings.ContainsAny(p, "\"\\") {
		return "", false
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f {
			return "", false
		}
	}
	return p, true
}

// labelValue is the label as written to d.custom1. ruTorrent writes labels
// percent-encoded, so HolyArrs does too and a label with spaces reads the same
// in both.
func labelValue(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

// labelText is the label as shown: decoded when it was percent-encoded.
func labelText(raw string) string {
	if s, err := url.PathUnescape(raw); err == nil {
		return s
	}
	return raw
}

// labelIs reports whether a stored label is the category, written plainly (as
// another app may have) or percent-encoded.
func labelIs(raw, category string) bool {
	return category != "" && (raw == category || labelText(raw) == category)
}

// rtorrentTooLarge is a call refused for its size before rTorrent took it.
type rtorrentTooLarge struct{ error }

// Unwrap gives the cause.
func (e rtorrentTooLarge) Unwrap() error { return e.error }

func rtNotFound(err error) error {
	var f *xmlrpcFault
	if errors.As(err, &f) && f.unknownHash() {
		return ErrNotFound
	}
	return err
}

func untrustedError(f *xmlrpcFault) error {
	return fmt.Errorf("rTorrent refused a call HolyArrs needs (%s); update ruTorrent, or point HolyArrs at rTorrent's /RPC2 address", redactURLs(cleanDetail(f.Message)))
}

// call sends one XML-RPC call and returns its value.
func (r *RTorrent) call(ctx context.Context, method string, params ...any) (any, error) {
	body, err := xmlrpcCall(method, params...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("rtorrent: invalid address")
	}
	req.Header.Set("Content-Type", "text/xml")
	if r.user != "" || r.pass != "" {
		req.SetBasicAuth(r.user, r.pass)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return nil, outbound.ErrAddressRefused
		}
		return nil, errDownloaderUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuth
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil, fmt.Errorf("%w: nothing answers XML-RPC at this path; crazymax images serve it on port 8000 at /RPC2 (8080 is the ruTorrent page), and through ruTorrent it is /plugins/httprpc/action.php", adapterHTTPError{resp.StatusCode})
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return nil, rtorrentTooLarge{fmt.Errorf("%w: the call is over the web server's or rTorrent's size limit; raise network.xmlrpc.size_limit or the web server's body size limit", ErrTooLarge)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, adapterHTTPError{resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, rtorrentAnswerMax+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errDownloaderUnavailable
	}
	if len(b) > rtorrentAnswerMax {
		return nil, ErrTooLarge
	}
	v, err := xmlrpcResponse(b)
	var f *xmlrpcFault
	if errors.As(err, &f) && f.tooLarge() {
		return nil, rtorrentTooLarge{fmt.Errorf("%w: raise rTorrent's network.xmlrpc.size_limit (crazymax: XMLRPC_SIZE_LIMIT)", ErrTooLarge)}
	}
	if errors.Is(err, errXMLRPCAnswer) {
		return nil, errors.New("this address answered, but not with rTorrent's XML-RPC; check the path (usually /RPC2)")
	}
	return v, err
}

// multicall sends calls in one system.multicall. Each answer is the call's
// value, or an error for a call rTorrent refused.
func (r *RTorrent) multicall(ctx context.Context, calls [][]any) ([]any, error) {
	list := make([]any, len(calls))
	for i, c := range calls {
		params := []any{}
		for _, p := range c[1:] {
			params = append(params, p)
		}
		list[i] = map[string]any{"methodName": c[0], "params": params}
	}
	v, err := r.call(ctx, "system.multicall", list)
	if err != nil {
		return nil, err
	}
	answers, ok := v.([]any)
	if !ok || len(answers) != len(calls) {
		return nil, errXMLRPCAnswer
	}
	out := make([]any, len(answers))
	for i, a := range answers {
		switch x := a.(type) {
		case []any:
			if len(x) > 0 {
				out[i] = x[0]
			}
		case map[string]any:
			f := &xmlrpcFault{Message: asStr(x["faultString"])}
			f.Code = asInt(x["faultCode"])
			out[i] = error(f)
		}
	}
	return out, nil
}

func asStr(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimRight(x, "\n")
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return ""
}

func asInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case bool:
		if x {
			return 1
		}
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	}
	return 0
}

// count32 keeps a peer count inside int on every build, 32-bit ones included.
func count32(v int64) int {
	if v <= 0 {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(v)
}
