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
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// edgeRule compares the committed Cloudflare cache rule (internal/api/
// edgerule.json, kept equal to EdgePrefixes by a test) with the live rule
// in the zone's http_request_cache_settings entrypoint. "check" fails on
// any drift; "apply" writes the committed rule (other rules are left alone).
// The token comes from CLOUDFLARE_API_TOKEN and is never printed.
func edgeRule(args []string) error {
	if len(args) == 0 || (args[0] != "check" && args[0] != "apply") {
		return errors.New("expected edge-rule check|apply")
	}
	mode := args[0]
	fs := flag.NewFlagSet("edge-rule", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	base := fs.String("api", "https://api.cloudflare.com/client/v4", "Cloudflare API address")
	zone := fs.String("zone", "coderluii.dev", "zone name")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return errors.New("invalid command arguments")
	}
	token := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if token == "" {
		return errors.New("CLOUDFLARE_API_TOKEN required")
	}
	want, err := api.CommittedEdgeRule()
	if err != nil {
		return errors.New("committed edge rule unreadable")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	call := func(method, path string, body, out any) error {
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
		if out == nil {
			return nil
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
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
	zid := zones.Result[0].ID
	type liveRule struct {
		ID               string          `json:"id"`
		Description      string          `json:"description"`
		Enabled          *bool           `json:"enabled"`
		Action           string          `json:"action"`
		ActionParameters json.RawMessage `json:"action_parameters"`
		Expression       string          `json:"expression"`
	}
	var set struct {
		Success bool
		Result  struct {
			ID    string     `json:"id"`
			Rules []liveRule `json:"rules"`
		}
	}
	if err := call("GET", "/zones/"+zid+"/rulesets/phases/http_request_cache_settings/entrypoint", nil, &set); err != nil {
		return err
	}
	var cur *liveRule
	for i := range set.Result.Rules {
		if set.Result.Rules[i].Description == want.Description {
			cur = &set.Result.Rules[i]
		}
	}
	diffs := edgeRuleDiff(want, cur == nil, func() (bool, string, json.RawMessage, string) {
		return cur.Enabled == nil || *cur.Enabled, cur.Action, cur.ActionParameters, cur.Expression
	})
	if len(diffs) == 0 {
		os.Stdout.WriteString("edge rule matches (" + strconv.Itoa(len(want.Paths)) + " paths)\n")
		return nil
	}
	if mode == "check" {
		return errors.New("EDGE RULE DRIFT: " + strings.Join(diffs, "; "))
	}
	body := map[string]any{"description": want.Description, "enabled": want.Enabled, "action": want.Action,
		"action_parameters": want.ActionParameters, "expression": want.Expression()}
	method, path := "POST", "/zones/"+zid+"/rulesets/"+set.Result.ID+"/rules"
	if cur != nil {
		method, path = "PATCH", path+"/"+cur.ID
	}
	if err := call(method, path, body, nil); err != nil {
		return err
	}
	os.Stdout.WriteString("edge rule applied: " + strings.Join(diffs, "; ") + "\n")
	return nil
}

// edgeRuleDiff names each way the live rule differs from the committed one.
func edgeRuleDiff(want api.EdgeRule, missing bool, live func() (bool, string, json.RawMessage, string)) []string {
	if missing {
		return []string{"rule \"" + want.Description + "\" missing"}
	}
	enabled, action, params, expr := live()
	var d []string
	if enabled != want.Enabled {
		d = append(d, "enabled differs")
	}
	if action != want.Action {
		d = append(d, "action differs")
	}
	var a, b any
	_ = json.Unmarshal(want.ActionParameters, &a)
	_ = json.Unmarshal(params, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		d = append(d, "action_parameters differ")
	}
	if expr != want.Expression() {
		var missingPaths []string
		for _, p := range want.Paths {
			if !strings.Contains(expr, `"`+p+`"`) {
				missingPaths = append(missingPaths, p)
			}
		}
		msg := "expression differs"
		if len(missingPaths) > 0 {
			msg += " (live lacks " + strings.Join(missingPaths, ", ") + ")"
		}
		d = append(d, msg)
	}
	return d
}
