package metadata

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// pictureHost makes the server's answers name pictures on the address this
// install talks to. The server still writes them with its first address;
// the rest of the app only takes pictures under its own.
type pictureHost struct {
	next     http.RoundTripper
	old, own string
}

func (p pictureHost) RoundTrip(req *http.Request) (*http.Response, error) {
	next := p.next
	if next == nil {
		next = http.DefaultTransport
	}
	resp, err := next.RoundTrip(req)
	if err != nil || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return resp, err
	}
	// Answers are small; one too big to hold is passed on as it came.
	const most = 32 << 20
	head, rerr := io.ReadAll(io.LimitReader(resp.Body, most+1))
	if rerr != nil || len(head) > most {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}
		if rerr != nil {
			_ = resp.Body.Close()
			return nil, rerr
		}
		return resp, nil
	}
	_ = resp.Body.Close()
	body := bytes.ReplaceAll(head, []byte(p.old), []byte(p.own))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return resp, nil
}

// OldPictures is the first address's picture store when base is the
// official server (a dead address otherwise): pictures saved before the address
// changed still load from it.
func OldPictures(base string) string {
	if base != DefaultServiceURL {
		return "https://invalid.invalid/"
	}
	return OldServiceURL + "/v1/img/"
}
