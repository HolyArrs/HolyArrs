package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrServiceAccess      = errors.New("metadata service refused installation access")
	ErrServiceUnavailable = errors.New("Metadata server is down, searches and new artwork will return when it's back")
	ErrServiceBusy        = errors.New("metadata service is busy; try again later")
	ErrServiceInvalid     = errors.New("metadata service returned an invalid or expired answer")
	ErrServiceIncomplete  = errors.New("metadata service does not supply the complete requested catalog")
	// ErrServicePending means the server answered 202: the data is still
	// being fetched. It is not a failure and its body is never final.
	ErrServicePending = errors.New("metadata service is still fetching this; try again shortly")
)

// PendingError is ErrServicePending with what the server said about the wait:
// state ("fetching" or "queued"), the place in line and the words to show.
// errors.Is(err, ErrServicePending) holds for it.
type PendingError struct {
	State    string `json:"state"`
	Position int    `json:"position"`
	Message  string `json:"message"`
}

func (e *PendingError) Error() string   { return ErrServicePending.Error() }
func (e *PendingError) Is(t error) bool { return t == ErrServicePending }

// pendingFrom reads a 202 body into a PendingError; a body with no wait
// fields is plain ErrServicePending.
func pendingFrom(body []byte) error {
	var p PendingError
	if json.Unmarshal(body, &p) != nil || p.State == "" && p.Message == "" {
		return ErrServicePending
	}
	if p.State != "queued" {
		p.State = "fetching"
	}
	if p.Position < 0 {
		p.Position = 0
	}
	if len(p.Message) > 200 {
		p.Message = p.Message[:200]
	}
	return &p
}

// ServiceOptions requires the caller's address-policy HTTP client. The command
// layer supplies it so metadata does not depend on the release-source package.
type ServiceOptions struct {
	URL        string
	Access     func(context.Context) (string, error)
	HTTP       *http.Client
	AllowLocal bool
}

// Service speaks the standalone schema-1 API with an installation credential.
// It never holds a provider credential or serves answers from a local cache.
type Service struct {
	base   string
	access func(context.Context) (string, error)
	http   *http.Client
	hold   serviceHold
	// local answers music from the packages kept on disk (SetLocalMusic).
	local atomic.Pointer[localHolder]
}

func NewService(o ServiceOptions) (*Service, error) {
	u, err := url.Parse(o.URL)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(u.Host, "%\\ \t\r\n") || o.HTTP == nil || o.Access == nil {
		return nil, errors.New("metadata service needs a guarded client, separate credential and plain HTTPS origin")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	switch host {
	case "metadata", "metadata.google.internal", "metadata.goog", "instance-data", "instance-data.ec2.internal", "metadata.azure.com", "metadata.platformequinix.com":
		return nil, errors.New("metadata service address refused")
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.Equal(net.ParseIP("100.100.100.200")) || ip.Equal(net.ParseIP("fd00:ec2::254")) || ip.Equal(net.IPv4bcast)) {
		return nil, errors.New("metadata service address refused")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && o.AllowLocal && ip != nil && (ip.IsLoopback() || ip.IsPrivate())) {
		return nil, errors.New("metadata service requires HTTPS; private literal HTTP needs explicit local access")
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, errors.New("metadata service port refused")
		}
	}
	cl := *o.HTTP
	cl.Jar = nil
	cl.Timeout = 18 * time.Second
	if u.String() == DefaultServiceURL {
		cl.Transport = pictureHost{next: cl.Transport, old: OldServiceURL + "/v1/img/", own: DefaultServiceURL + "/v1/img/"}
	}
	cl.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u.Path = ""
	return &Service{base: u.String(), access: o.Access, http: &cl}, nil
}

func serviceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 18*time.Second)
}

func serviceFallback(ctx context.Context, err error) bool {
	return ctx.Err() == nil && (errors.Is(err, ErrServiceUnavailable) || errors.Is(err, ErrServiceBusy) || errors.Is(err, ErrServicePending) || errors.Is(err, ErrServiceIncomplete) || errors.Is(err, outbound.ErrSuppressed))
}

type serviceEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source"`
	SourceID      string          `json:"source_id"`
	FetchedAt     *time.Time      `json:"fetched_at"`
	ExpiresAt     *time.Time      `json:"expires_at"`
	Completeness  string          `json:"completeness"`
	Stale         *bool           `json:"stale"`
	NextCursor    string          `json:"next_cursor"`
	Data          json.RawMessage `json:"data"`
}

type serviceItem struct {
	Source     string          `json:"source"`
	SourceID   string          `json:"source_id"`
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	SourceData json.RawMessage `json:"source_data"`
	// Popularity rides beside an artist's data (the metadata server adds it).
	Popularity *ArtistPopularity `json:"popularity,omitempty"`
	// ReleaseGroupsComplete: the artist's stored list holds every release
	// group, so no MusicBrainz walk is needed.
	ReleaseGroupsComplete bool `json:"release_groups_complete,omitempty"`
}

func (i serviceItem) decode(out any) error {
	var id struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(i.SourceData, &id) != nil || len(id.ID) == 0 {
		return ErrServiceInvalid
	}
	var rawID string
	if json.Unmarshal(id.ID, &rawID) != nil {
		rawID = string(id.ID)
	}
	if !strings.EqualFold(rawID, i.SourceID) || json.Unmarshal(i.SourceData, out) != nil {
		return ErrServiceInvalid
	}
	return nil
}

type servicePagination struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Offset       int `json:"offset"`
	Limit        int `json:"limit"`
}

type serviceSearch struct {
	Items      []serviceItem     `json:"items"`
	Pagination servicePagination `json:"pagination"`
}

func serviceKind(provider, kind string) bool {
	return provider == "tmdb" && (kind == "movie" || kind == "show") || provider == "tvmaze" && kind == "show" || provider == "musicbrainz" && (kind == "artist" || kind == "release-group" || kind == "release" || kind == "recording")
}

func serviceID(provider, id string) bool {
	if provider == "musicbrainz" {
		return ValidMBID(id)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}

func (s *Service) search(ctx context.Context, provider, kind, query string, page, limit int) (serviceSearch, error) {
	if !serviceKind(provider, kind) || strings.TrimSpace(query) == "" || len(query) > 256 || page < 1 || page > 100 || limit < 1 || limit > 100 || provider == "tmdb" && limit != 20 || provider == "tvmaze" && page != 1 {
		return serviceSearch{}, ErrServiceInvalid
	}
	q := url.Values{"provider": {provider}, "kind": {kind}, "q": {query}, "page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
	e, err := s.get(ctx, "/v1/search", provider, "", q)
	if err != nil {
		return serviceSearch{}, err
	}
	var out serviceSearch
	if json.Unmarshal(e.Data, &out) != nil || out.Items == nil || out.Pagination.Page != page || out.Pagination.TotalResults < 0 || out.Pagination.TotalResults < len(out.Items) || len(out.Items) > limit {
		return serviceSearch{}, ErrServiceInvalid
	}
	p := out.Pagination
	more := false
	switch provider {
	case "tmdb":
		if p.TotalPages < 0 || p.TotalResults > 0 && (p.TotalPages < page || len(out.Items) == 0) {
			return serviceSearch{}, ErrServiceInvalid
		}
		more = page < p.TotalPages
	case "tvmaze":
		if p.TotalPages != 1 || p.TotalResults != len(out.Items) {
			return serviceSearch{}, ErrServiceInvalid
		}
	case "musicbrainz":
		if p.Limit != limit || p.Offset != (page-1)*limit || p.Offset+len(out.Items) > p.TotalResults || p.Offset < p.TotalResults && len(out.Items) == 0 {
			return serviceSearch{}, ErrServiceInvalid
		}
		more = p.Offset+len(out.Items) < p.TotalResults
	}
	if (more && e.NextCursor != strconv.Itoa(page+1)) || (!more && e.NextCursor != "") {
		return serviceSearch{}, ErrServiceInvalid
	}
	seen := map[string]bool{}
	for _, item := range out.Items {
		if item.Source != provider || item.Kind != kind || !serviceID(provider, item.SourceID) || seen[strings.ToLower(item.SourceID)] {
			return serviceSearch{}, ErrServiceInvalid
		}
		var raw map[string]json.RawMessage
		if item.decode(&raw) != nil {
			return serviceSearch{}, ErrServiceInvalid
		}
		seen[strings.ToLower(item.SourceID)] = true
	}
	return out, nil
}

func (s *Service) detail(ctx context.Context, provider, kind, id string) (serviceItem, error) {
	if !serviceKind(provider, kind) || !serviceID(provider, id) {
		return serviceItem{}, ErrServiceInvalid
	}
	base := "/v1/movies/"
	if kind == "show" {
		base = "/v1/shows/"
	} else if provider == "musicbrainz" {
		base = "/v1/music/" + provider + "/" + kind + "/"
		provider = "musicbrainz"
	}
	path := base + provider + "/" + strings.ToLower(id)
	if provider == "musicbrainz" {
		path = base + strings.ToLower(id)
	}
	e, err := s.get(ctx, path, provider, strings.ToLower(id), nil)
	if err != nil {
		if errors.Is(err, ErrServicePending) {
			// Partial answer: usable for display, never final.
			var part serviceItem
			if json.Unmarshal(e.Data, &part) == nil && part.Source == provider && part.Kind == kind && strings.EqualFold(part.SourceID, id) {
				return part, err
			}
		}
		return serviceItem{}, err
	}
	var item serviceItem
	if json.Unmarshal(e.Data, &item) != nil || item.Source != provider || item.Kind != kind || !strings.EqualFold(item.SourceID, id) {
		return serviceItem{}, ErrServiceInvalid
	}
	var raw map[string]json.RawMessage
	if item.decode(&raw) != nil {
		return serviceItem{}, ErrServiceInvalid
	}
	return item, nil
}

func (s *Service) season(ctx context.Context, id string, number int) (serviceItem, error) {
	if !serviceID("tmdb", id) || number < 0 || number > 1000 {
		return serviceItem{}, ErrServiceInvalid
	}
	e, err := s.get(ctx, "/v1/shows/tmdb/"+id+"/seasons/"+strconv.Itoa(number), "tmdb", id, nil)
	if err != nil {
		return serviceItem{}, err
	}
	var item serviceItem
	if json.Unmarshal(e.Data, &item) != nil || item.Source != "tmdb" || item.Kind != "show" || !serviceID("tmdb", item.SourceID) {
		return serviceItem{}, ErrServiceInvalid
	}
	var raw map[string]json.RawMessage
	if item.decode(&raw) != nil {
		return serviceItem{}, ErrServiceInvalid
	}
	return item, nil
}

func (s *Service) get(ctx context.Context, path, provider, id string, q url.Values) (out serviceEnvelope, err error) {
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if IsAhead(ctx) && s.hold.held(time.Now()) {
		return out, ErrServiceBusy
	}
	tok, e := s.access(ctx)
	if e != nil || tok == "" || len(tok) > 4096 || strings.ContainsAny(tok, "\r\n") {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, ErrServiceAccess
	}
	// Fetch-ahead has its own breaker, so failing warm-ups never hold back
	// a person's page.
	lane := "page"
	if IsAhead(ctx) {
		lane = "ahead"
	}
	op, e := outbound.Begin(ctx, outbound.Target{Kind: "metadata_service", Identity: []string{s.base, tok, lane}})
	if e != nil {
		if errors.Is(e, outbound.ErrSuppressed) {
			return out, ErrServiceUnavailable
		}
		return out, e
	}
	defer func() {
		result := outbound.TerminalFailure
		switch {
		case err == nil:
			result = outbound.Success
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			result = outbound.Canceled
		case errors.Is(err, ErrServiceBusy):
			// "Slow down" (from the server or our own pacing) says nothing about the server being down.
			result = outbound.Canceled
		case errors.Is(err, ErrServiceUnavailable):
			result = outbound.TransientFailure
		}
		op.Finish(result)
	}()
	u := s.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	pending := 0
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if attempt > 0 {
			tok, e = s.access(ctx)
			if e != nil || tok == "" || len(tok) > 4096 || strings.ContainsAny(tok, "\r\n") {
				if ctx.Err() != nil {
					return out, ctx.Err()
				}
				return out, ErrServiceAccess
			}
		}
		rctx, stop := op.AttemptContext(ctx, 16*time.Second)
		req, _ := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Cache-Control", "no-cache")
		aheadRequest(ctx, req)
		resp, e := s.http.Do(req)
		if e != nil {
			stop()
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			return out, ErrServiceUnavailable
		}
		body, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
		_ = resp.Body.Close()
		stop()
		if e != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			return out, ErrServiceUnavailable
		}
		if len(body) > 8<<20 {
			return out, ErrServiceInvalid
		}
		switch resp.StatusCode {
		case http.StatusOK:
			d := json.NewDecoder(bytes.NewReader(body))
			if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF {
				return out, ErrServiceInvalid
			}
			now := time.Now()
			if out.SchemaVersion != 1 || out.Source != provider || out.SourceID != id || out.Completeness != "complete" || out.Stale == nil || *out.Stale || out.FetchedAt == nil || out.FetchedAt.IsZero() || out.ExpiresAt == nil || !now.Before(*out.ExpiresAt) || out.FetchedAt.After(now.Add(time.Minute)) || !out.FetchedAt.Before(*out.ExpiresAt) || len(out.Data) == 0 || out.Data[0] != '{' || path != "/v1/search" && out.NextCursor != "" {
				return out, ErrServiceInvalid
			}
			return out, nil
		case http.StatusAccepted:
			if pending >= pendingTries {
				// Still not ready: hand back the partial data (never final).
				var partial serviceEnvelope
				if json.Unmarshal(body, &partial) == nil {
					out = partial
				}
				return out, pendingFrom(body)
			}
			pending++
			attempt--
			if !pendingSleep(ctx, resp.Header.Get("Retry-After")) {
				return out, ctx.Err()
			}
		case http.StatusUnauthorized, http.StatusForbidden:
			return out, ErrServiceAccess
		case http.StatusNotFound:
			return out, ErrNotFound
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			if attempt == 1 {
				s.hold.set(resp.Header.Get("Retry-After"), body, time.Now())
				if resp.StatusCode == 429 {
					return out, ErrServiceBusy
				}
				return out, ErrServiceUnavailable
			}
			delay := serviceRetryAfter(resp.Header.Get("Retry-After"), body)
			tm := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				tm.Stop()
				return out, ctx.Err()
			case <-tm.C:
			}
		default:
			if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500 && resp.StatusCode <= 599 {
				return out, ErrServiceUnavailable
			}
			return out, ErrServiceInvalid
		}
	}
	return out, ErrServiceUnavailable
}

func serviceRetryAfter(value string, body []byte) time.Duration {
	delay := time.Second
	if seconds, e := strconv.Atoi(value); e == nil && seconds >= 0 {
		delay = time.Duration(min(seconds, 10)) * time.Second
	} else if stamp, e := http.ParseTime(value); e == nil {
		delay = max(0, min(time.Until(stamp), 10*time.Second))
	} else {
		var b struct {
			Error struct {
				Retry int `json:"retry_after_seconds"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &b) == nil && b.Error.Retry > 0 {
			delay = time.Duration(min(b.Error.Retry, 10)) * time.Second
		}
	}
	return delay + time.Duration(rand.IntN(100))*time.Millisecond
}

// pendingTries is how many times a 202 is re-asked inside one request.
const pendingTries = 2

// pendingDelay reads Retry-After for a 202, capped to 3 s so a page never
// waits long on the server.
func pendingDelay(value string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 {
		n = 2
	}
	return time.Duration(min(n, 3))*time.Second + time.Duration(rand.IntN(100))*time.Millisecond
}

func pendingSleep(ctx context.Context, retryAfter string) bool {
	t := time.NewTimer(pendingDelay(retryAfter))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// doPending sends req and, on 202, re-asks up to pendingTries times after
// Retry-After. A 202 left over is returned unread for the caller to map to
// ErrServicePending.
func (s *Service) doPending(ctx context.Context, req *http.Request) (*http.Response, error) {
	for i := 0; ; i++ {
		resp, err := s.http.Do(req)
		if err != nil || resp.StatusCode != http.StatusAccepted || i >= pendingTries {
			return resp, err
		}
		ra := resp.Header.Get("Retry-After")
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		if !pendingSleep(ctx, ra) {
			return nil, ctx.Err()
		}
	}
}
