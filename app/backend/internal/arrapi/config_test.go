package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Profilarr's add-time call: the default delay rule with both protocols
// on, torrent first and 600 minutes each. It answers 202 with the rule,
// lists it, and the wait reaches Pick rules this side was sent, before and
// after the call.
func TestConfig_DelayProfile(t *testing.T) {
	h := newHookRig(t)
	_, id := pushAll(t, h)
	rule := `{"id":1,"enableUsenet":true,"enableTorrent":true,"preferredProtocol":"torrent","usenetDelay":600,"torrentDelay":600,
		"bypassIfHighestQuality":false,"bypassIfAboveCustomFormatScore":false,"minimumCustomFormatScore":0,"order":2147483647,"tags":[]}`
	st, out := h.do("radarr", "PUT", "delayprofile/1", rule)
	if got := decodeObj(t, st, out, 202); got["torrentDelay"] != 600.0 || got["preferredProtocol"] != "torrent" {
		t.Fatalf("put: %s", out)
	}
	if l := h.list("radarr", "delayprofile"); len(l) != 1 || l[0]["usenetDelay"] != 600.0 {
		t.Fatalf("list: %v", l)
	}
	want := mediafiles.ProtocolRule{Prefer: "torrent", WaitMinutesTorrent: 600, WaitMinutesUsenet: 600}
	if got := h.rules(id).Body.Protocol; got != want {
		t.Fatalf("pushed rules: %+v", got)
	}
	st, out = h.do("radarr", "POST", "qualityprofile", radarrProfile(0, "Second", nil, nil))
	id2 := int64(decodeObj(t, st, out, 201)["id"].(float64))
	if got := h.rules(id2).Body.Protocol; got != want {
		t.Fatalf("rules pushed after: %+v", got)
	}
	// The other side keeps its own rule.
	if l := h.list("sonarr", "delayprofile"); l[0]["torrentDelay"] != 0.0 {
		t.Fatalf("sonarr: %v", l)
	}
	// Only torrent: torrent is preferred.
	st, _ = h.do("radarr", "PUT", "delayprofile/1", strings.Replace(rule, `"enableUsenet":true`, `"enableUsenet":false`, 1))
	if st != 202 || h.rules(id).Body.Protocol.Prefer != "torrent" {
		t.Fatalf("only torrent: %d %+v", st, h.rules(id).Body.Protocol)
	}
	// Extra rules are kept as sent; the default cannot be removed.
	st, out = h.do("radarr", "POST", "delayprofile", `{"enableUsenet":true,"enableTorrent":true,"preferredProtocol":"usenet","usenetDelay":5,"torrentDelay":5,"tags":[3]}`)
	extra := decodeObj(t, st, out, 201)
	if st, _ := h.do("radarr", "GET", fmt.Sprintf("delayprofile/%v", extra["id"]), ""); st != 200 {
		t.Fatalf("get extra: %d", st)
	}
	if st, _ := h.do("radarr", "DELETE", "delayprofile/1", ""); st == 200 {
		t.Fatal("default removed")
	}
	if st, _ := h.do("radarr", "DELETE", fmt.Sprintf("delayprofile/%v", extra["id"]), ""); st != 200 {
		t.Fatalf("delete extra: %d", st)
	}
	if st, _ := h.do("radarr", "PUT", "delayprofile/77", rule); st != 404 {
		t.Fatalf("missing: %d", st)
	}
}

// Size limits: the whole list comes back with sizes changed on matching
// rows; they become per-minute limits on the matching steps.
func TestConfig_QualityDefinitions(t *testing.T) {
	h := newHookRig(t)
	_, id := pushAll(t, h)
	l := h.list("radarr", "qualitydefinition")
	if len(l) != len(sideQualities["radarr"]) {
		t.Fatalf("rows: %d", len(l))
	}
	for _, row := range l {
		q := row["quality"].(map[string]any)
		switch q["name"] {
		case "Bluray-1080p":
			row["minSize"], row["maxSize"], row["preferredSize"] = 20.0, 120.0, 90.0
		case "WEBDL-1080p":
			row["minSize"], row["maxSize"], row["preferredSize"] = 10.0, nil, nil
		}
	}
	b, _ := json.Marshal(l)
	st, out := h.do("radarr", "PUT", "qualitydefinition/update", string(b))
	if st != 202 || !strings.Contains(out, `"Bluray-1080p"`) {
		t.Fatalf("put: %d %s", st, out)
	}
	limits := h.rules(id).Body.SizeLimits
	mb := int64(1 << 20)
	if got := limits["1080p_bluray"]; got != (mediafiles.SizeLimit{Min: 20 * mb, Max: 120 * mb, PerMinute: true}) {
		t.Fatalf("bluray: %+v", got)
	}
	if got := limits["1080p_web"]; got.Min != 0 || got.Max != 0 {
		// WEBRip-1080p kept its own sizes, so the step takes the wider pair.
		t.Fatalf("web: %+v", got)
	}
	one := h.list("radarr", "qualitydefinition")[0]
	one["minSize"] = 1.0
	b, _ = json.Marshal(one)
	if st, out := h.do("radarr", "PUT", fmt.Sprintf("qualitydefinition/%v", one["id"]), string(b)); st != 202 || !strings.Contains(out, `"minSize":1`) {
		t.Fatalf("put one: %d %s", st, out)
	}
	if st, _ := h.do("radarr", "GET", fmt.Sprintf("qualitydefinition/%v", one["id"]), ""); st != 200 {
		t.Fatalf("get one: %d", st)
	}
}

// Media switches: read, two keys changed, sent back. The repack choice
// becomes the proper policy; every other key is kept as sent.
func TestConfig_MediaManagement(t *testing.T) {
	h := newHookRig(t)
	_, id := pushAll(t, h)
	st, out := h.do("sonarr", "GET", "config/mediamanagement", "")
	m := decodeObj(t, st, out, 200)
	m["downloadPropersAndRepacks"], m["enableMediaInfo"], m["madeUp"] = "doNotUpgrade", false, "kept"
	b, _ := json.Marshal(m)
	if st, out := h.do("radarr", "PUT", fmt.Sprintf("config/mediamanagement/%v", m["id"]), string(b)); st != 202 || !strings.Contains(out, `"madeUp":"kept"`) {
		t.Fatalf("put: %d %s", st, out)
	}
	if got := h.rules(id).Body.ProperPolicy.Mode; got != "prefer_only" {
		t.Fatalf("policy: %s", got)
	}
	if st, out := h.do("radarr", "GET", "config/mediamanagement", ""); st != 200 || !strings.Contains(out, `"enableMediaInfo":false`) {
		t.Fatalf("read back: %d %s", st, out)
	}
}

// Naming: the rename switch, colon and illegal-character choices reach
// HolyArrs naming; patterns (written in the old apps' words) are kept and
// read back but not used.
func TestConfig_Naming(t *testing.T) {
	h := newHookRig(t)
	st, out := h.do("sonarr", "GET", "config/naming", "")
	m := decodeObj(t, st, out, 200)
	m["renameEpisodes"], m["colonReplacementFormat"], m["multiEpisodeStyle"], m["replaceIllegalCharacters"] = false, 1.0, 3.0, false
	m["standardEpisodeFormat"] = "{Series Title} - S{season:00}E{episode:00}"
	b, _ := json.Marshal(m)
	if st, out := h.do("sonarr", "PUT", fmt.Sprintf("config/naming/%v", m["id"]), string(b)); st != 202 {
		t.Fatalf("put: %d %s", st, out)
	}
	n, err := mediafiles.ReadNaming(context.Background(), h.st.Read())
	if err != nil || n.RenameOnImport || n.Colon != mediafiles.ColonDash || n.Illegal != mediafiles.IllegalRemove || n.MultiEpisodeStyle != mediafiles.MultiEachDash {
		t.Fatalf("naming: %+v %v", n, err)
	}
	if n.EpisodePattern == m["standardEpisodeFormat"] {
		t.Fatal("old-app pattern used")
	}
	st, out = h.do("sonarr", "GET", "config/naming", "")
	if back := decodeObj(t, st, out, 200); back["standardEpisodeFormat"] != m["standardEpisodeFormat"] || back["renameEpisodes"] != false {
		t.Fatalf("read back: %s", out)
	}
	st, out = h.do("radarr", "GET", "config/naming", "")
	r := decodeObj(t, st, out, 200)
	if r["renameMovies"] != false || r["colonReplacementFormat"] != "dash" {
		t.Fatalf("radarr view: %s", out)
	}
	r["colonReplacementFormat"], r["renameMovies"] = "spaceDashSpace", true
	b, _ = json.Marshal(r)
	if st, _ := h.do("radarr", "PUT", "config/naming", string(b)); st != 202 {
		t.Fatalf("radarr put: %d", st)
	}
	if n, _ := mediafiles.ReadNaming(context.Background(), h.st.Read()); n.Colon != mediafiles.ColonSpaceDashSpace || !n.RenameOnImport {
		t.Fatalf("radarr naming: %+v", n)
	}
}

// Writes need a key whose maker may change settings; reads do not.
func TestConfig_NeedSettingsPermission(t *testing.T) {
	h := newHookRig(t)
	h.caller = webserver.Caller{UserID: 2, Name: "staff", Via: "api_key", Role: "member", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	for _, c := range [][2]string{{"PUT", "delayprofile/1"}, {"POST", "delayprofile"}, {"PUT", "qualitydefinition/update"},
		{"PUT", "config/mediamanagement"}, {"PUT", "config/naming"}} {
		if st, _ := h.do("radarr", c[0], c[1], `{}`); st != 403 {
			t.Errorf("%s %s: %d", c[0], c[1], st)
		}
	}
	for _, p := range []string{"delayprofile", "qualitydefinition", "config/mediamanagement", "config/naming"} {
		if st, _ := h.do("radarr", "GET", p, ""); st != 200 {
			t.Errorf("get %s: %d", p, st)
		}
	}
}

func TestConfig_OddBodies(t *testing.T) {
	h := newHookRig(t)
	pushAll(t, h)
	for _, path := range []string{"delayprofile", "delayprofile/1", "config/mediamanagement", "config/naming"} {
		method := "PUT"
		if path == "delayprofile" {
			method = "POST"
		}
		if st, out := h.do("radarr", method, path, "null"); st != 400 {
			t.Fatalf("%s %s null: %d %s", method, path, st, out)
		}
	}
	// An id no row has, but a quality name one row has.
	st, out := h.do("radarr", "PUT", "qualitydefinition/999", `{"quality":{"name":"Bluray-1080p"},"minSize":5}`)
	if st != 202 || !strings.Contains(out, `"Bluray-1080p"`) {
		t.Fatalf("by name: %d %s", st, out)
	}
}
