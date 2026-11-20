package arrapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"reflect"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The settings objects of the old apps that Profilarr pushes: delay rules,
// the size table, media switches and naming. Each is kept as sent, so an
// app reads back what it wrote. Where HolyArrs has the same setting, it is
// applied too:
//
//   - the default delay rule (id 1) sets the protocol preference, both
//     waits and the skip-wait switch of the Pick rules sent to this side
//     (only one protocol on becomes a preference for it);
//   - the size table sets per-minute size limits per quality step on those
//     Pick rules (several old qualities on one step take the widest pair);
//   - the repack choice of the media switches sets their fixed re-release
//     rule;
//   - naming sets HolyArrs's rename switch and its colon, illegal-character
//     and multi-episode choices, for both sides.
//
// Extra delay rules, rename patterns (written in the old apps' words) and
// every other media switch are kept and answered back but change nothing.

type obj = map[string]any

// readJSONBody reads a JSON body (object or list) into v. It returns false
// after sending a 400.
func readJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyMax))
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if o, ok := v.(*obj); err == nil && ok && *o == nil {
		err = io.ErrUnexpectedEOF // null: there is no object to keep
	}
	if err != nil {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return false
	}
	return true
}

// num reads a JSON number, or a number written as text; 0 otherwise.
func num(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return fieldNum(v)
}

func flag(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// sameID reports whether a stored object has this id.
func sameID(o obj, id int64) bool { return int64(num(o["id"])) == id }

// sideConfig is what this side was sent that changes Pick rules; nil when
// never sent.
type sideConfig struct {
	delay obj
	sizes []obj
	media obj
}

func (h *handler) loadConfig(ctx context.Context) (sideConfig, error) {
	var c sideConfig
	var rules []obj
	if _, err := readConfig(ctx, h.Store, h.side, "delay", &rules); err != nil {
		return c, err
	}
	for _, o := range rules {
		if sameID(o, 1) {
			c.delay = o
		}
	}
	if _, err := readConfig(ctx, h.Store, h.side, "sizes", &c.sizes); err != nil {
		return c, err
	}
	_, err := readConfig(ctx, h.Store, h.side, "media", &c.media)
	return c, err
}

// fold puts what c maps onto into a Pick rules body.
func (c sideConfig) fold(b *mediafiles.PickRulesBody) {
	if d := c.delay; d != nil {
		usenet, torrent := flag(d["enableUsenet"], true), flag(d["enableTorrent"], true)
		prefer, _ := d["preferredProtocol"].(string)
		switch {
		case usenet && !torrent:
			prefer = "usenet"
		case torrent && !usenet:
			prefer = "torrent"
		case prefer != "usenet" && prefer != "torrent":
			prefer = "either"
		}
		b.Protocol = mediafiles.ProtocolRule{Prefer: prefer, WaitMinutesTorrent: waitOf(d["torrentDelay"]),
			WaitMinutesUsenet: waitOf(d["usenetDelay"]), SkipWaitAtGoodEnough: flag(d["bypassIfHighestQuality"], false)}
	}
	if c.sizes != nil {
		limits := map[string]mediafiles.SizeLimit{}
		for k, v := range b.SizeLimits {
			limits[k] = v
		}
		for step, l := range stepLimits(c.sizes) {
			limits[step] = l
		}
		b.SizeLimits = limits
	}
	if c.media != nil {
		if mode, ok := properModes[strOf(c.media["downloadPropersAndRepacks"])]; ok {
			b.ProperPolicy = mediafiles.ProperPolicy{Mode: mode}
		}
	}
}

// properModes maps the repack choice to the fixed re-release rule.
var properModes = map[string]string{"preferAndUpgrade": "take_any", "doNotUpgrade": "prefer_only", "doNotPrefer": "ignore"}

func strOf(v any) string { s, _ := v.(string); return s }

// waitOf reads a wait in minutes, held to what Pick rules allow (a week).
func waitOf(v any) int { return int(math.Max(0, math.Min(10080, num(v)))) }

// stepLimits turns size-table rows (MB a minute, null or 0 largest for no
// limit) into per-minute limits per quality step.
func stepLimits(rows []obj) map[string]mediafiles.SizeLimit {
	sizes := make([]library.OldSize, 0, len(rows))
	for _, row := range rows {
		q, _ := row["quality"].(obj)
		sizes = append(sizes, library.OldSize{Name: cmpOr(strOf(q["name"]), strOf(row["title"])), Min: num(row["minSize"]), Max: num(row["maxSize"])})
	}
	return library.OldSizeLimits(sizes)
}

// refold applies this side's settings to every Pick rules body it was sent.
// A body the checks refuse is logged and left as it was.
func (h *handler) refold(ctx context.Context, r *http.Request) error {
	c, err := h.loadConfig(ctx)
	if err != nil {
		return err
	}
	ps, pushed, err := h.listedProfiles(ctx)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if pp, ok := pushed[p.ID]; !ok || pp.Side != h.side {
			continue
		}
		body := p.Body
		c.fold(&body)
		if reflect.DeepEqual(body, p.Body) {
			continue
		}
		if _, err := library.UpdatePickRules(ctx, h.Store, p.ID, library.PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: body}, actor(r)); err != nil {
			log.Printf("arrapi: settings left Pick rules %d as they were: %v", p.ID, err)
		}
	}
	return nil
}

// saveAndFold stores one settings object and applies the side's settings.
func (h *handler) saveAndFold(w http.ResponseWriter, r *http.Request, kind string, v any) bool {
	if err := saveConfig(r.Context(), h.Store, h.side, kind, v, actor(r)); err != nil {
		internalError(w, err)
		return false
	}
	if err := h.refold(r.Context(), r); err != nil {
		internalError(w, err)
		return false
	}
	return true
}
