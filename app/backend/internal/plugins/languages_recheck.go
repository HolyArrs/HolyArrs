package plugins

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// blockedPack says whether the blocklist names this pack, by hash or by
// id (and version when the entry gives one).
func blockedPack(block []Entry, en Entry) bool {
	for _, b := range block {
		if (b.SHA256 != "" && b.SHA256 == en.SHA256) || (b.ID != "" && b.ID == en.ID && (b.Version == "" || b.Version == en.Version)) {
			return true
		}
	}
	return false
}

// safeVersion keeps a list version usable as a file name.
func safeVersion(v string) bool {
	return v != "" && !strings.ContainsAny(v, `/\`) && !strings.Contains(v, "..")
}

// Recheck fetches the signed language list and blocklist and drops every
// installed pack the blocklist names or the list no longer carries, so a
// pack withdrawn after install stops being served. Nothing is fetched
// when no pack is installed.
func (l *Languages) Recheck(ctx context.Context) error {
	e := l.Engine
	l.mu.Lock()
	st := l.load()
	n := len(st.Packs)
	last, lastBlock := st.ListCounter, st.BlockCounter
	l.mu.Unlock()
	if n == 0 || e == nil || len(e.Keys) == 0 {
		return nil
	}
	list, block, err := l.fetchLists(ctx, last, lastBlock)
	if err != nil {
		return err
	}
	l.mu.Lock()
	st = l.load()
	st.ListCounter = max(st.ListCounter, list.Counter)
	st.BlockCounter = max(st.BlockCounter, block.Counter)
	var gone []string
	for code, p := range st.Packs {
		en := Entry{ID: "lang-" + code, Version: p.Version, SHA256: p.SHA256}
		listed := false
		for _, x := range list.Entries {
			if x.ID == en.ID && x.Version == en.Version && x.SHA256 == en.SHA256 {
				listed = true
				break
			}
		}
		if !listed || blockedPack(block.Entries, en) {
			gone = append(gone, code)
		}
	}
	err = l.saveLocked(st)
	l.mu.Unlock()
	for _, code := range gone {
		log.Printf("languages: %s pack withdrawn by the signed lists, dropped", code)
		l.drop(code)
	}
	return err
}

// fetchLists fetches and checks the signed languages list and blocklist.
// The blocklist may be no older than the one the plugin engine has seen,
// nor older than the list asks for.
func (l *Languages) fetchLists(ctx context.Context, last, lastBlock uint64) (*List, *List, error) {
	e := l.Engine
	listRaw, err := e.fetch(ctx, "languages.json")
	if err != nil {
		return nil, nil, err
	}
	list, _, err := e.verify(listRaw, KindLanguages, last)
	if err != nil {
		return nil, nil, fmt.Errorf("language list: %w", err)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return nil, nil, err
	}
	block, _, err := e.verify(blockRaw, KindBlocklist, max(lastBlock, e.blockFloor()))
	if err != nil {
		return nil, nil, fmt.Errorf("blocklist: %w", err)
	}
	if block.Counter < *list.MinBlocklistCounter {
		return nil, nil, fmt.Errorf("%w: counter %d, need %d", ErrStaleBlock, block.Counter, *list.MinBlocklistCounter)
	}
	return list, block, nil
}
