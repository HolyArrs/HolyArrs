package discover

import (
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func ItemRefs(items []engine.Item) []metadata.DiscoverItem {
	refs := make([]metadata.DiscoverItem, len(items))
	for i, it := range items {
		refs[i] = metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind}
	}
	return refs
}

// KeepItems keeps the items named in refs, in items' order.
func KeepItems(items []engine.Item, refs []metadata.DiscoverItem) []engine.Item {
	ok := map[string]bool{}
	for _, it := range refs {
		ok[it.Kind+":"+it.TMDBID] = true
	}
	out := []engine.Item{}
	for _, it := range items {
		if ok[it.Key()] {
			out = append(out, it)
		}
	}
	return out
}
