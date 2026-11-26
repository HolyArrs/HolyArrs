package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The botcheck hook (docs/dev/plugins.md, "Bot-check solver hook"): the
// plugin asks HolyArrs for work with GET, which waits up to 25 seconds and
// answers 200 with a job or 204 with none, and posts each answer back.
const hookPath = "/api/v1/plugin-hooks/botcheck"

const (
	pollWait   = 40 * time.Second // HolyArrs holds a poll up to 25 seconds
	answerWait = 30 * time.Second
	// answerMax is the most HolyArrs reads of one answer (20 MB).
	answerMax = 20 << 20
	// hookFirst and hookMost bound the wait when HolyArrs does not answer.
	hookFirst = time.Second
	hookMost  = time.Minute
	// pollGuard is the least time between two empty polls, should HolyArrs
	// ever answer 204 at once.
	pollGuard = time.Second
	// solving is how many pages are opened at once.
	solving = 3
)

// job is one page HolyArrs hands over.
type job struct {
	ID      string          `json:"id"`
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// errRefused is HolyArrs saying no to the plugin itself: its token, or the
// sources.botcheck action not approved.
var errRefused = errors.New("refused")

type plugin struct {
	target string
	token  string
	client *http.Client
	fs     *flareSolverr // nil when the address is not set or not usable
	fsErr  error         // why fs is nil
	log    *log.Logger
	slots  chan struct{}
	// pause waits d, or until ctx ends; false when it ended.
	pause func(ctx context.Context, d time.Duration) bool
}

func newPlugin(target, token, address string, l *log.Logger) *plugin {
	p := &plugin{target: strings.TrimRight(target, "/"), token: token, log: l, slots: make(chan struct{}, solving), pause: sleep,
		// HolyArrs is on loopback, which is never sent through the proxy.
		client: &http.Client{Transport: &http.Transport{Proxy: nil}}}
	base, err := parseAddress(address)
	switch {
	case err != nil:
		p.fsErr = err
	case base == "":
		p.fsErr = errNoAddress
	default:
		p.fs = newFlareSolverr(base)
	}
	return p
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// run polls for jobs until ctx ends. It never stops on an error: when
// HolyArrs does not answer it waits 1 second, doubling up to a minute.
func (p *plugin) run(ctx context.Context) {
	if p.fsErr != nil {
		p.log.Print(p.fsErr)
	}
	var wait time.Duration
	for ctx.Err() == nil {
		began := time.Now()
		j, err := p.next(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if errors.Is(err, errRefused) {
				wait = hookMost
				p.log.Printf("HolyArrs refused the plugin; approve its sources.botcheck action on the Plugins page. Asking again in %s", wait)
			} else {
				wait = min(max(2*wait, hookFirst), hookMost)
				p.log.Printf("HolyArrs did not answer (%v); asking again in %s", err, wait)
			}
			if !p.pause(ctx, wait) {
				return
			}
			continue
		}
		wait = 0
		if j == nil {
			if time.Since(began) < pollGuard && !p.pause(ctx, pollGuard) {
				return
			}
			continue
		}
		select {
		case p.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func() {
			defer func() { <-p.slots }()
			p.handle(ctx, *j)
		}()
	}
}

// next asks HolyArrs for one job; nil when none came.
func (p *plugin) next(ctx context.Context) (*job, error) {
	cctx, cancel := context.WithTimeout(ctx, pollWait)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, p.target+hookPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, plain(err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errRefused
	default:
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var j job
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&j); err != nil || j.ID == "" {
		return nil, errors.New("a job could not be read")
	}
	return &j, nil
}

// handle opens one page and answers its job. What is logged is only the
// reason, never the page's address, its proxy or the cookies.
func (p *plugin) handle(ctx context.Context, j job) {
	a, err := p.open(ctx, j)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		p.log.Printf("a page could not be opened: %v", err)
		p.reply(ctx, j.ID, nil, err.Error())
		return
	}
	p.reply(ctx, j.ID, &a, "")
}

func (p *plugin) open(ctx context.Context, j job) (answer, error) {
	if j.Op != "open" {
		return answer{}, fmt.Errorf("this plugin only opens pages, not %q", clean(j.Op, ""))
	}
	var in ask
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return answer{}, errors.New("the job could not be read")
	}
	if u, err := url.Parse(in.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return answer{}, errors.New("the job has no web page to open")
	}
	if p.fs == nil {
		return answer{}, p.fsErr
	}
	return p.fs.open(ctx, in)
}

// reply posts {"job", "data"} or {"job", "error"}. A page too large to send
// is answered with an error instead.
func (p *plugin) reply(ctx context.Context, id string, data *answer, why string) {
	body, err := encode(id, data, why)
	if err == nil && body.Len() > answerMax-1024 {
		body, err = encode(id, nil, errTooLarge.Error())
	}
	if err != nil {
		p.log.Printf("an answer could not be written: %v", err)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, answerWait)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, p.target+hookPath, body)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			p.log.Printf("the answer did not reach HolyArrs: %v", plain(err))
		}
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		p.log.Print("HolyArrs had stopped waiting for this answer")
	} else if resp.StatusCode/100 != 2 {
		p.log.Printf("HolyArrs refused the answer: status %d", resp.StatusCode)
	}
}

func encode(id string, data *answer, why string) (*bytes.Buffer, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	// The page goes as it is: escaping < and > would make it larger.
	e.SetEscapeHTML(false)
	err := e.Encode(struct {
		Job   string  `json:"job"`
		Data  *answer `json:"data,omitempty"`
		Error string  `json:"error,omitempty"`
	}{id, data, why})
	return &b, err
}

// plain is a connection error without the address it was made to.
func plain(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
