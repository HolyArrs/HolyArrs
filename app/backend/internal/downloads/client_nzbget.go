package downloads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

const (
	nzbgetMarker = "HolyArrsID"
	nzbgetMaxNZB = 16 << 20
)

var (
	errNZBGetAmbiguousMarker = errors.New("NZBGet has multiple jobs with the same HolyArrs marker")
	errNZBGetForeignMarker   = errors.New("the NZBGet job marker belongs to another category")
)

// NZBGet is a Client for the NZBGet JSON-RPC API.
type NZBGet struct {
	base       string
	user, pass string
	http       *http.Client
	Category   string
	Others     []string // more categories that are also ours
}

type nzbgetRPCError struct {
	Name    string `json:"name"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type nzbParam struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type nzbgetRow struct {
	NZBID           int        `json:"NZBID"`
	NZBFilename     string     `json:"NZBFilename"`
	NZBName         string     `json:"NZBName"`
	Category        string     `json:"Category"`
	DestDir         string     `json:"DestDir"`
	FinalDir        string     `json:"FinalDir"`
	FileSizeLo      uint64     `json:"FileSizeLo"`
	FileSizeHi      uint64     `json:"FileSizeHi"`
	RemainingSizeLo uint64     `json:"RemainingSizeLo"`
	RemainingSizeHi uint64     `json:"RemainingSizeHi"`
	Status          string     `json:"Status"`
	ParStatus       string     `json:"ParStatus"`
	ExParStatus     string     `json:"ExParStatus"`
	UnpackStatus    string     `json:"UnpackStatus"`
	MoveStatus      string     `json:"MoveStatus"`
	ScriptStatus    string     `json:"ScriptStatus"`
	DeleteStatus    string     `json:"DeleteStatus"`
	URLStatus       string     `json:"UrlStatus"`
	MarkStatus      string     `json:"MarkStatus"`
	HistoryTime     int64      `json:"HistoryTime"`
	MinPostTime     int64      `json:"MinPostTime"`
	Parameters      []nzbParam `json:"Parameters"`
}

type nzbgetFile struct {
	Filename        string `json:"Filename"`
	FileSizeLo      uint64 `json:"FileSizeLo"`
	FileSizeHi      uint64 `json:"FileSizeHi"`
	RemainingSizeLo uint64 `json:"RemainingSizeLo"`
	RemainingSizeHi uint64 `json:"RemainingSizeHi"`
	Progress        int64  `json:"Progress"`
}

type nzbgetMatch struct {
	row     nzbgetRow
	history bool
}

// NewNZBGet returns a client for the NZBGet at base, using the given login and h.
func NewNZBGet(base, user, pass string, h *http.Client) *NZBGet {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	return &NZBGet{base: strings.TrimRight(base, "/"), user: user, pass: pass, http: h}
}

// Health reports whether NZBGet answers and accepts the login.
func (n *NZBGet) Health(ctx context.Context) (Health, error) {
	var version string
	if err := n.rpc(ctx, "version", nil, &version); err != nil {
		return Health{}, err
	}
	if version == "" {
		return Health{}, errors.New("the address answered, but not like NZBGet")
	}
	return Health{Version: version, At: time.Now().UTC()}, nil
}

// Add queues an NZB and returns its job id.
func (n *NZBGet) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.ReconcileOnly && r.AttemptMarker == "" {
		return "", ErrUncertain
	}
	active, history, err := n.rows(ctx)
	if err != nil {
		return "", err
	}
	attempt := 0
	for {
		id := nzbgetID(r, attempt)
		match, found, err := nzbgetFind(active, history, id, r.Category)
		if err != nil {
			return "", err
		}
		if r.AttemptMarker != "" {
			if r.Skip[id] {
				return "", errors.New("the NZBGet marker belongs to another Download")
			}
			if found {
				return id, nil
			}
			if r.ReconcileOnly {
				return "", ErrUncertain
			}
			break
		}
		if !r.Skip[id] {
			if found && match.row.job(match.history).State != StateFailed {
				return id, nil
			} else if !found {
				break
			}
		}
		attempt++
	}
	id := nzbgetID(r, attempt)
	payload, err := fetchRelease(ctx, r.URL, r.AllowLocal, nzbgetMaxNZB)
	if err != nil {
		return "", err
	}
	if !validNZB(payload) {
		return "", errors.New("the Release file is not an NZB document")
	}
	name := r.Name
	if !strings.HasSuffix(strings.ToLower(name), ".nzb") {
		name += ".nzb"
	}
	params := []any{name, base64.StdEncoding.EncodeToString(payload), r.Category, 0, false, false, "", 0, "SCORE", false,
		[][]string{{nzbgetMarker, id}}}
	var native int
	if err := n.rpc(ctx, "append", params, &native); err != nil {
		if ctx.Err() == nil {
			_, found, getErr := n.lookup(ctx, id, r.Category)
			if errors.Is(getErr, errNZBGetAmbiguousMarker) || errors.Is(getErr, errNZBGetForeignMarker) {
				return "", errors.Join(ErrUncertain, getErr)
			}
			if getErr == nil && found {
				return id, nil
			}
		}
		return "", errors.Join(ErrUncertain, err)
	}
	if native <= 0 {
		return "", errors.New("NZBGet refused the NZB")
	}
	return id, nil
}

// List returns the jobs in category.
func (n *NZBGet) List(ctx context.Context, category string) ([]Job, error) {
	active, history, err := n.rows(ctx)
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, r := range active {
		if id := nzbgetMarkerID(r.Parameters); id != "" && (category == "" || strings.EqualFold(r.Category, category)) {
			ids[id] = true
		}
	}
	for _, r := range history {
		if id := nzbgetMarkerID(r.Parameters); id != "" && (category == "" || strings.EqualFold(r.Category, category)) {
			ids[id] = true
		}
	}
	out := make([]Job, 0, len(ids))
	for id := range ids {
		match, found, err := nzbgetFind(active, history, id, category)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, match.row.job(match.history))
		}
	}
	n.shareSpeed(ctx, out)
	return out, nil
}

// Get returns the job with the given id.
func (n *NZBGet) Get(ctx context.Context, id string) (Job, error) {
	match, found, err := n.find(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !found {
		return Job{}, ErrNotFound
	}
	r := match.row
	if match.history {
		return r.anyJob(true), nil
	}
	j := r.anyJob(false)
	var fs []nzbgetFile
	if err := n.rpc(ctx, "listfiles", []any{0, 0, r.NZBID}, &fs); err != nil {
		return Job{}, err
	}
	for _, f := range fs {
		size := nzbgetSize(f.FileSizeLo, f.FileSizeHi)
		p := min(max(float64(f.Progress)/1000, 0), 1)
		if size > 0 {
			p = min(max(float64(size-nzbgetSize(f.RemainingSizeLo, f.RemainingSizeHi))/float64(size), 0), 1)
		}
		j.Files = append(j.Files, File{Name: nzbgetRelative(r.DestDir, f.Filename), Size: size, Progress: p})
	}
	return j, nil
}

// Remove deletes the job, and its files when deleteFiles is set.
func (n *NZBGet) Remove(ctx context.Context, id string, deleteFiles bool) error {
	match, found, err := n.find(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	command := "GroupFinalDelete"
	if !deleteFiles {
		command = "GroupParkDelete"
	}
	if match.history {
		command = "HistoryFinalDelete"
		if !deleteFiles {
			command = "HistoryDelete"
		}
	}
	var ok bool
	if err := n.rpc(ctx, "editqueue", []any{command, "", []int{match.row.NZBID}}, &ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("NZBGet refused to remove the job")
	}
	return nil
}

func (n *NZBGet) rpc(ctx context.Context, method string, params []any, out any) error {
	in := struct {
		Version string `json:"version"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
		ID      int    `json:"id"`
	}{Version: "1.1", Method: method, Params: params, ID: 1}
	b, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("nzbget %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.base+"/jsonrpc", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("nzbget %s: bad address", method)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(n.user, n.pass)
	resp, err := n.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return outbound.ErrAddressRefused
		}
		return errDownloaderUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuth
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return adapterHTTPError{resp.StatusCode}
	}
	var answer struct {
		Result json.RawMessage `json:"result"`
		Error  *nzbgetRPCError `json:"error"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return fmt.Errorf("nzbget %s: the address did not answer like NZBGet", method)
	}
	if answer.Error != nil {
		if answer.Error.Code == http.StatusUnauthorized {
			return ErrAuth
		}
		return fmt.Errorf("nzbget %s: RPC error %d", method, answer.Error.Code)
	}
	if len(answer.Result) == 0 || string(answer.Result) == "null" {
		return fmt.Errorf("nzbget %s: empty result", method)
	}
	if err := json.Unmarshal(answer.Result, out); err != nil {
		return fmt.Errorf("nzbget %s: invalid result", method)
	}
	return nil
}

func nzbgetID(r AddRequest, attempt int) string {
	if r.AttemptMarker != "" {
		return r.AttemptMarker
	}
	key := r.OperationKey
	if key == "" {
		key = strings.ToLower(r.Name)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(r.Category) + "\x00" + key + "\x00" + strconv.Itoa(attempt)))
	return "nzbget-" + hex.EncodeToString(sum[:])
}

func validNZB(b []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	depth, roots := 0, 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return roots == 1 && depth == 0
		}
		if err != nil {
			return false
		}
		switch x := tok.(type) {
		case xml.Directive:
			if dir := strings.TrimSpace(string(x)); !strings.HasPrefix(dir, "DOCTYPE ") || strings.ContainsAny(dir, "[]<>%") {
				return false
			}
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || x.Name.Local != "nzb" {
					return false
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(x)) != "" {
				return false
			}
		}
	}
}

func nzbgetFind(active, history []nzbgetRow, id, category string) (nzbgetMatch, bool, error) {
	var match nzbgetMatch
	found := false
	add := func(r nzbgetRow, isHistory bool) error {
		if nzbgetMarkerID(r.Parameters) != id {
			return nil
		}
		if category != "" && !strings.EqualFold(r.Category, category) {
			return errNZBGetForeignMarker
		}
		if !found {
			match, found = nzbgetMatch{row: r, history: isHistory}, true
			return nil
		}
		if match.row.NZBID <= 0 || r.NZBID <= 0 || match.row.NZBID != r.NZBID {
			return errNZBGetAmbiguousMarker
		}
		if isHistory {
			match = nzbgetMatch{row: r, history: true}
		}
		return nil
	}
	for _, r := range active {
		if err := add(r, false); err != nil {
			return nzbgetMatch{}, false, err
		}
	}
	for _, r := range history {
		if err := add(r, true); err != nil {
			return nzbgetMatch{}, false, err
		}
	}
	return match, found, nil
}

func nzbgetSize(lo, hi uint64) int64 {
	n := (hi << 32) | (lo & 0xffffffff)
	if n > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1)
	}
	return int64(n)
}

func nzbgetMarkerID(ps []nzbParam) string {
	for _, p := range ps {
		if p.Name == nzbgetMarker && strings.HasPrefix(p.Value, "nzbget-") {
			return p.Value
		}
	}
	return ""
}

func nzbgetActiveState(s string) string {
	switch s {
	case "QUEUED", "FETCHING", "PP_QUEUED":
		return StateQueued
	case "PAUSED":
		return StatePaused
	}
	return StateDownloading
}

func nzbgetPostProcess(s string) bool {
	switch s {
	case "LOADING_PARS", "VERIFYING_SOURCES", "REPAIRING", "VERIFYING_REPAIRED", "RENAMING", "UNPACKING", "MOVING", "POST_UNPACK_RENAMING", "EXECUTING_SCRIPT", "PP_FINISHED":
		return true
	}
	return false
}

func nzbgetHistoryFailed(r nzbgetRow) bool {
	if r.Status == "WARNING/SPACE" || r.UnpackStatus == "SPACE" {
		return false
	}
	if strings.HasPrefix(r.Status, "FAILURE/") || strings.HasPrefix(r.Status, "WARNING/") || strings.HasPrefix(r.Status, "DELETED/") {
		return true
	}
	return r.ParStatus == "FAILURE" || r.UnpackStatus == "FAILURE" || r.MoveStatus == "FAILURE" ||
		r.ExParStatus == "FAILURE" || r.ScriptStatus == "FAILURE" || r.URLStatus == "FAILURE" ||
		r.URLStatus == "SCAN_FAILURE" || r.MarkStatus == "BAD" || (r.DeleteStatus != "" && r.DeleteStatus != "NONE")
}

func nzbgetRelative(dest, name string) string {
	dest = strings.TrimRight(strings.ReplaceAll(dest, "\\", "/"), "/")
	name = strings.ReplaceAll(name, "\\", "/")
	if dest != "" && strings.HasPrefix(name, dest+"/") {
		name = strings.TrimPrefix(name, dest+"/")
	}
	name = strings.TrimLeft(name, "/")
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return path.Base(name)
	}
	return name
}

func (r nzbgetRow) job(history bool) Job {
	size := nzbgetSize(r.FileSizeLo, r.FileSizeHi)
	left := nzbgetSize(r.RemainingSizeLo, r.RemainingSizeHi)
	p := float64(0)
	if size > 0 {
		p = min(max(float64(size-left)/float64(size), 0), 1)
	}
	name := r.NZBName
	if name == "" {
		name = strings.TrimSuffix(r.NZBFilename, ".nzb")
	}
	j := Job{ID: nzbgetMarkerID(r.Parameters), Name: name, Category: r.Category, SavePath: r.DestDir,
		Size: size, Progress: p, State: nzbgetActiveState(r.Status), Detail: r.Status, PostedAt: max(r.MinPostTime, 0)}
	if history {
		j.Progress, j.DoneAt = 1, max(r.HistoryTime, 0)
		switch {
		case nzbgetHistoryFailed(r):
			j.State, j.Detail = StateFailed, nzbgetReason(r)
		case r.Status == "WARNING/SPACE" || r.UnpackStatus == "SPACE":
			j.State, j.Detail = StatePaused, nzbgetReason(r)
		case strings.HasPrefix(r.Status, "SUCCESS/"):
			j.State, j.ContentPath = StateFinished, r.FinalDir
			if j.ContentPath == "" {
				j.ContentPath = r.DestDir
			}
		default:
			j.State = StateDownloading
		}
	} else if nzbgetPostProcess(r.Status) {
		j.Progress = 1
	}
	return j
}

func (n *NZBGet) rows(ctx context.Context) ([]nzbgetRow, []nzbgetRow, error) {
	var active []nzbgetRow
	if err := n.rpc(ctx, "listgroups", []any{0}, &active); err != nil {
		return nil, nil, err
	}
	var history []nzbgetRow
	if err := n.rpc(ctx, "history", []any{false}, &history); err != nil {
		return nil, nil, err
	}
	return active, history, nil
}

func (n *NZBGet) lookup(ctx context.Context, id, category string) (nzbgetMatch, bool, error) {
	active, history, err := n.rows(ctx)
	if err != nil {
		return nzbgetMatch{}, false, err
	}
	return nzbgetFind(active, history, id, category)
}

func (n *NZBGet) shareSpeed(ctx context.Context, jobs []Job) {
	moving := 0
	for _, j := range jobs {
		if j.State == StateDownloading {
			moving++
		}
	}
	if moving == 0 {
		return
	}
	var st struct {
		DownloadRate int64 `json:"DownloadRate"`
	}
	if err := n.rpc(ctx, "status", nil, &st); err != nil || st.DownloadRate <= 0 {
		return
	}
	for i := range jobs {
		if jobs[i].State == StateDownloading {
			jobs[i].Speed = st.DownloadRate / int64(moving)
		}
	}
}
