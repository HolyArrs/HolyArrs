package hooks

import (
	"context"
	"slices"
	"sort"
	"sync"
)

// Registry is everything plugins provide right now: the Broker's work
// lanes plus the tools, Home widget and pages each plugin declared. It is
// all in memory; a plugin declares again on every start.
type Registry struct {
	Broker *Broker
	// Alive ends when a plugin's token does (accounts.PluginTokenContext);
	// nil, or a nil answer, means the plugin is not running.
	Alive func(plugin string) context.Context

	mu      sync.Mutex
	tools   map[string][]Tool
	widgets map[string]Card
	pages   map[string][]Page
	streams *streams
}

// NewRegistry makes an empty Registry around b.
func NewRegistry(b *Broker) *Registry {
	return &Registry{Broker: b, tools: map[string][]Tool{}, widgets: map[string]Card{}, pages: map[string][]Page{}, streams: newStreams()}
}

// Drop forgets all a plugin provides and ends its waiting work and its
// media streams, at once. Called when its token ends.
func (r *Registry) Drop(plugin string) {
	r.Broker.Drop(plugin)
	r.mu.Lock()
	delete(r.tools, plugin)
	delete(r.widgets, plugin)
	delete(r.pages, plugin)
	r.mu.Unlock()
	r.streams.drop(plugin)
}

// granted says whether plugin holds scope now.
func (r *Registry) granted(plugin, scope string) bool {
	b := r.Broker
	return b.Granted != nil && b.Covers != nil && b.Covers(b.Granted(plugin), scope)
}

func (r *Registry) setTools(plugin string, ts []Tool) {
	r.mu.Lock()
	r.tools[plugin] = ts
	r.mu.Unlock()
}

func (r *Registry) setWidget(plugin string, c Card) {
	r.mu.Lock()
	r.widgets[plugin] = c
	r.mu.Unlock()
}

func (r *Registry) setPages(plugin string, ps []Page) {
	r.mu.Lock()
	r.pages[plugin] = ps
	r.mu.Unlock()
}

// Widget is one plugin's Home card.
type Widget struct {
	Plugin string `json:"plugin"`
	Card
}

// Widgets are the Home cards of plugins that still hold pages.home-widget.
func (r *Registry) Widgets() []Widget {
	r.mu.Lock()
	ids := make([]string, 0, len(r.widgets))
	for id := range r.widgets {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	sort.Strings(ids)
	var out []Widget
	for _, id := range ids {
		if !r.granted(id, "pages.home-widget") {
			continue
		}
		r.mu.Lock()
		c, ok := r.widgets[id]
		r.mu.Unlock()
		if ok {
			out = append(out, Widget{Plugin: id, Card: c})
		}
	}
	return out
}

// PluginPage is one page in the plugins menu.
type PluginPage struct {
	Plugin string `json:"plugin"`
	Page
}

// Pages lists the pages of plugins that still hold pages.add.
func (r *Registry) Pages() []PluginPage {
	r.mu.Lock()
	var out []PluginPage
	for id, ps := range r.pages {
		for _, p := range ps {
			out = append(out, PluginPage{Plugin: id, Page: p})
		}
	}
	r.mu.Unlock()
	out = slices.DeleteFunc(out, func(p PluginPage) bool { return !r.granted(p.Plugin, "pages.add") })
	sort.Slice(out, func(i, j int) bool { return out[i].Plugin+out[i].ID < out[j].Plugin+out[j].ID })
	return out
}

func (r *Registry) page(plugin, id string) (Page, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.pages[plugin] {
		if p.ID == id {
			return p, true
		}
	}
	return Page{}, false
}
