package main

// stage and block build the unsigned payloads for a publish. They read the
// lists that are live now (verified against the pinned keys), so the counters
// always go up and nothing already listed is lost.
//
//	pluginkey stage -id <id> -version <v> -meta <plugin.json> -builds <dir> -current <dir> -out <dir>
//	pluginkey block -id <id> [-version <v>] -current <dir> -out <dir>
//
// -builds holds one folder per platform (linux-amd64, linux-arm64,
// linux-armv7), each with the built "plugin" file. -current holds the live
// plugins.json and blocklist.json; leave it empty for the very first list.
// Both write plugins.payload.json, blocklist.payload.json and counters.env
// (LIST_MIN and BLOCK_MIN, the -min-counter values for sign) into -out.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// trustKeys are the keys live lists must be signed with; tests swap it.
var trustKeys = plugins.PinnedKeys

var platforms = []string{"linux-amd64", "linux-arm64", "linux-armv7"}

// current is what is live now; zero lists when nothing was published yet.
type current struct {
	list, block plugins.List
}

func readCurrent(dir string) (*current, error) {
	c := &current{list: plugins.List{Kind: plugins.KindPlugins}, block: plugins.List{Kind: plugins.KindBlocklist}}
	for _, f := range []struct {
		name, kind string
		into       *plugins.List
	}{{"plugins.json", plugins.KindPlugins, &c.list}, {"blocklist.json", plugins.KindBlocklist, &c.block}} {
		data, err := os.ReadFile(filepath.Join(dir, f.name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		l, err := verifyPinned(data, f.kind)
		if err != nil {
			return nil, fmt.Errorf("live %s: %w", f.name, err)
		}
		*f.into = *l
	}
	return c, nil
}

// verifyPinned accepts a list signed by any of the keys the app pins.
func verifyPinned(data []byte, kind string) (*plugins.List, error) {
	var last error = errors.New("no pinned keys")
	for _, k := range trustKeys() {
		l, err := plugins.VerifyList(k, data, kind, 0)
		if err == nil {
			return l, nil
		}
		last = err
	}
	return nil, last
}

func runStage(args []string) error {
	fs := flag.NewFlagSet("stage", flag.ExitOnError)
	id := fs.String("id", "", "plugin id")
	version := fs.String("version", "", "plugin version")
	meta := fs.String("meta", "", "plugin.json with name, author, description, icon, access, min_app_version")
	builds := fs.String("builds", "", "folder with one sub-folder per platform")
	cur := fs.String("current", "", "folder with the live plugins.json and blocklist.json")
	out := fs.String("out", "", "folder to write the files to publish")
	_ = fs.Parse(args)
	if *id == "" || *version == "" || *meta == "" || *builds == "" || *out == "" {
		return errors.New("-id, -version, -meta, -builds and -out are required")
	}
	c, err := readCurrent(*cur)
	if err != nil {
		return err
	}
	for _, en := range c.list.Entries {
		if en.ID == *id && en.Version == *version {
			return fmt.Errorf("%s %s is already on the list; publish a new version", *id, *version)
		}
	}
	m, err := readMeta(*meta)
	if err != nil {
		return err
	}
	m["id"], m["version"] = *id, *version
	var added []plugins.Entry
	for _, p := range platforms {
		file, err := os.ReadFile(filepath.Join(*builds, p, "plugin"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		m["sha256"] = sum(file)
		man, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		man = append(man, '\n')
		if _, err := plugins.ParseManifest(man); err != nil {
			return fmt.Errorf("%s manifest: %w", p, err)
		}
		dir := filepath.Join(*out, "plugins", *id, *version, p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin"), file, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), man, 0o644); err != nil {
			return err
		}
		added = append(added, plugins.Entry{ID: *id, Version: *version, SHA256: sum(file), ManifestSHA256: sum(man), Platform: p})
	}
	if len(added) == 0 {
		return fmt.Errorf("no builds found in %s (expected %v)", *builds, platforms)
	}
	list := c.list
	list.Entries = append(append([]plugins.Entry{}, list.Entries...), added...)
	sortEntries(list.Entries)
	block := c.block
	block.Counter++
	return writePayloads(*out, c, list, block)
}

func runBlock(args []string) error {
	fs := flag.NewFlagSet("block", flag.ExitOnError)
	id := fs.String("id", "", "plugin id")
	version := fs.String("version", "", "one version; leave empty to block every version")
	cur := fs.String("current", "", "folder with the live plugins.json and blocklist.json")
	out := fs.String("out", "", "folder to write the files to publish")
	_ = fs.Parse(args)
	if *id == "" || *cur == "" || *out == "" {
		return errors.New("-id, -current and -out are required")
	}
	c, err := readCurrent(*cur)
	if err != nil {
		return err
	}
	if c.list.Counter == 0 {
		return errors.New("no live plugin list found in -current")
	}
	bad := plugins.Entry{ID: *id, Version: *version}
	block := c.block
	block.Entries = append(append([]plugins.Entry{}, block.Entries...), bad)
	block.Counter++
	// The blocked versions also leave the list, so no one can install them.
	list := c.list
	list.Entries = nil
	for _, en := range c.list.Entries {
		if !(en.ID == *id && (*version == "" || en.Version == *version)) {
			list.Entries = append(list.Entries, en)
		}
	}
	if len(list.Entries) == len(c.list.Entries) {
		fmt.Fprintf(os.Stderr, "note: %s %s is not on the list; blocking it anyway\n", *id, *version)
	}
	return writePayloads(*out, c, list, block)
}

// writePayloads raises the list counter and ties the list to the new
// blocklist, so an older blocklist can never be served with it.
func writePayloads(out string, c *current, list, block plugins.List) error {
	list.Kind, block.Kind = plugins.KindPlugins, plugins.KindBlocklist
	list.Counter = c.list.Counter + 1
	if block.Counter <= c.block.Counter {
		block.Counter = c.block.Counter + 1
	}
	minBlock := block.Counter
	list.MinBlocklistCounter = &minBlock
	if list.Entries == nil {
		list.Entries = []plugins.Entry{}
	}
	if block.Entries == nil {
		block.Entries = []plugins.Entry{}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for name, l := range map[string]plugins.List{"plugins.payload.json": list, "blocklist.payload.json": block} {
		b, err := json.MarshalIndent(l, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, name), append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	env := fmt.Sprintf("LIST_MIN=%d\nBLOCK_MIN=%d\n", c.list.Counter, c.block.Counter)
	if err := os.WriteFile(filepath.Join(out, "counters.env"), []byte(env), 0o644); err != nil {
		return err
	}
	fmt.Printf("Staged: plugin list counter %d (%d entries), blocklist counter %d (%d entries)\n",
		list.Counter, len(list.Entries), block.Counter, len(block.Entries))
	return nil
}

// readMeta reads the plugin's own description file; id, version and sha256
// are filled in by stage.
func readMeta(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, k := range []string{"id", "version", "sha256"} {
		if _, ok := m[k]; ok {
			return nil, fmt.Errorf("%s: leave %q out; it is filled in when publishing", path, k)
		}
	}
	return m, nil
}

func sortEntries(es []plugins.Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].ID != es[j].ID {
			return es[i].ID < es[j].ID
		}
		if es[i].Version != es[j].Version {
			return es[i].Version < es[j].Version
		}
		return es[i].Platform < es[j].Platform
	})
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// runCheck verifies a signed list against the pinned keys, the way the app does.
func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	in := fs.String("in", "", "signed list")
	kind := fs.String("kind", "", "plugins or blocklist")
	_ = fs.Parse(args)
	data, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	l, err := verifyPinned(data, *kind)
	if err != nil {
		return err
	}
	fmt.Printf("OK (pinned key): %s list, counter %d, %d entries\n", l.Kind, l.Counter, len(l.Entries))
	return nil
}
