package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// purgeEdge clears the shared text routes (api.EdgePrefixes but /v1/img/)
// from the Cloudflare cache, for a deploy that changes an answer's shape or
// a cache rule. Pictures are never purged: their addresses carry their
// version, so a changed picture has a new address and Cloudflare's copies
// survive every deploy (-pictures purges them too, for a broken copy). The
// token comes from CLOUDFLARE_API_TOKEN and is never printed.
func purgeEdge(args []string) error {
	fs := flag.NewFlagSet("purge-edge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	base := fs.String("api", "https://api.cloudflare.com/client/v4", "Cloudflare API address")
	zone := fs.String("zone", "coderluii.dev", "zone name")
	host := fs.String("host", "holyarrs-api.coderluii.dev", "public host name")
	pictures := fs.Bool("pictures", false, "purge the picture routes too")
	only := fs.String("only", "", "comma-separated route prefixes (under /v1/) to purge instead, e.g. the music ones")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return errors.New("invalid command arguments")
	}
	token := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if token == "" {
		return errors.New("CLOUDFLARE_API_TOKEN required")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	call := func(method, path string, body any, out any) error {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, strings.TrimRight(*base, "/")+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("cloudflare unreachable")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("cloudflare answered " + resp.Status)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	var zones struct {
		Success bool
		Result  []struct{ ID string }
	}
	if err := call("GET", "/zones?name="+url.QueryEscape(*zone), nil, &zones); err != nil {
		return err
	}
	if !zones.Success || len(zones.Result) != 1 {
		return errors.New("zone not found")
	}
	var prefixes []string
	if *only != "" {
		for _, p := range strings.Split(*only, ",") {
			if p = strings.TrimSpace(p); !strings.HasPrefix(p, "/v1/") {
				return errors.New("-only prefixes start with /v1/")
			}
			prefixes = append(prefixes, *host+p)
		}
	} else {
		for _, p := range api.EdgePrefixes() {
			if p == "/v1/img/" && !*pictures {
				continue
			}
			prefixes = append(prefixes, *host+p)
		}
	}
	// Cloudflare takes at most 30 prefixes per call.
	for len(prefixes) > 0 {
		n := min(len(prefixes), 30)
		var res struct{ Success bool }
		if err := call("POST", "/zones/"+zones.Result[0].ID+"/purge_cache", map[string]any{"prefixes": prefixes[:n]}, &res); err != nil {
			return err
		}
		if !res.Success {
			return errors.New("purge refused")
		}
		prefixes = prefixes[n:]
	}
	return nil
}
