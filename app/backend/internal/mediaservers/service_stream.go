package mediaservers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// doStream is do for an answer read as it arrives: read gets at most limit
// bytes of a good answer, plus one so it can tell a cut answer.
func (l *Linker) doStream(ctx context.Context, m domain.MediaServer, method, path string, body []byte, limit int64, read func(io.Reader) error) error {
	tok, err := l.token(ctx, m)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "media_server", Identity: []string{strconv.FormatInt(m.ID, 10), m.Kind, m.URL, tok}})
	if err != nil {
		return err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, method, m.URL+path, rd)
	if err != nil {
		outcome = outbound.TerminalFailure
		return ErrURL
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
		if errors.Is(err, outbound.ErrMetadataRefused) {
			return errors.New("the address is a cloud metadata address, which HolyArrs never connects to")
		}
		// The line never holds the address or the token.
		if c := doctor.At(doctor.Of(err), err.Error(), m.URL, dialer(m)); c != doctor.Unknown {
			return doctor.Mark(c, nil)
		}
		return errors.New("the server did not answer")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// A refused answer is short: read a little of it so the doctor can
		// tell another app, a sign-in refusal or HTTP sent to HTTPS.
		eb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
		if doctor.Other(resp.Header, eb, m.Kind, ownAlso[m.Kind]) != nil {
			return doctor.Mark(doctor.WrongApp, nil)
		}
		if a := doctor.Answer(resp.StatusCode, eb); a != nil {
			return a
		}
		return fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	switch {
	case resp.StatusCode/100 != 2:
		outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
		return fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	lr := &io.LimitedReader{R: resp.Body, N: limit + 1}
	if readErr := read(lr); readErr != nil || lr.N == 0 {
		if lr.N == 0 {
			// A cut answer would read as a short list.
			outcome = outbound.TerminalFailure
			return errors.New("the server's answer was too big to read")
		}
		outcome = outbound.HTTPOutcome(resp.StatusCode, readErr, ctx)
		return errors.New("reading the server answer failed")
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
	return nil
}
