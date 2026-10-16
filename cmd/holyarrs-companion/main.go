package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// The companion deliberately knows one API route and emits aggregate states only.
func readStates(ctx context.Context, base, key string, client *http.Client) (map[string]int, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("set HOLYARRS_URL to the app address, without credentials or a query")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/health/states"
	u.RawPath = ""
	if key == "" || len(key) > 4096 || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("the companion needs its task-only API key file")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("the app address could not be used")
	}
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Accept", "application/json")
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.New("the app did not answer within the deadline")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("the app refused the companion health read")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("the health answer could not be read")
	}
	var answer struct {
		States []struct {
			State string `json:"state"`
		} `json:"states"`
	}
	if err := json.Unmarshal(b, &answer); err != nil || answer.States == nil {
		return nil, errors.New("the app did not return health states")
	}
	counts := map[string]int{"off": 0, "starting": 0, "running": 0, "retrying": 0, "failed": 0}
	for _, state := range answer.States {
		if _, ok := counts[state.State]; !ok {
			return nil, errors.New("the app returned an unknown health state")
		}
		counts[state.State]++
	}
	return counts, nil
}

func run() error {
	f, err := os.Open("/run/secrets/holyarrs-key")
	if err != nil {
		return errors.New("mount the companion key at /run/secrets/holyarrs-key")
	}
	b, err := io.ReadAll(io.LimitReader(f, 4098))
	_ = f.Close()
	if err != nil || len(b) > 4097 {
		return errors.New("the companion key file could not be read")
	}
	key := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	counts, err := readStates(context.Background(), os.Getenv("HOLYARRS_URL"), key, http.DefaultClient)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"states": counts})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
