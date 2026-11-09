package webserver

import (
	"net/http"
	"slices"
)

// Route is one registered route: its pattern and what the caller needs.
type Route struct {
	Pattern string // "METHOD /path", the form http.ServeMux matches
	Perm    Perm
	Part    string // the part that must be on for the route to answer; empty if none
	Flag    string // the feature flag that must be on; empty if none
}

// RouteOption adds a gate to one registration.
type RouteOption func(*Route)

// Router collects the routes of every module. It sits on a standard mux, so
// matching, path values and 405 answers stay what net/http does.
type Router struct {
	mux    *http.ServeMux
	routes []Route
	byPat  map[string]int
}

// Part gates a route on a part of the app being switched on.
func Part(name string) RouteOption { return func(r *Route) { r.Part = name } }

// Flag gates a route on a feature flag.
func Flag(name string) RouteOption { return func(r *Route) { r.Flag = name } }

// NewRouter returns an empty router.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux(), byPat: map[string]int{}}
}

// Get registers a GET route that needs perm.
func (r *Router) Get(path string, perm Perm, h http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodGet+" "+path, perm, h, opts...)
}

// Post registers a POST route that needs perm.
func (r *Router) Post(path string, perm Perm, h http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodPost+" "+path, perm, h, opts...)
}

// Put registers a PUT route that needs perm.
func (r *Router) Put(path string, perm Perm, h http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodPut+" "+path, perm, h, opts...)
}

// Delete registers a DELETE route that needs perm.
func (r *Router) Delete(path string, perm Perm, h http.HandlerFunc, opts ...RouteOption) {
	r.Handle(http.MethodDelete+" "+path, perm, h, opts...)
}

// Handle registers a handler under a full "METHOD /path" pattern. Registering
// the same pattern twice panics, as the mux does.
func (r *Router) Handle(pattern string, perm Perm, h http.Handler, opts ...RouteOption) {
	r.mux.Handle(pattern, h)
	rt := Route{Pattern: pattern, Perm: perm}
	for _, o := range opts {
		o(&rt)
	}
	r.byPat[pattern] = len(r.routes)
	r.routes = append(r.routes, rt)
}

// Mux is the underlying mux. Routes that do not declare their permission yet
// register on it directly and are left out of Routes.
func (r *Router) Mux() *http.ServeMux { return r.mux }

// ServeHTTP lets the router stand in for its mux.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.mux.ServeHTTP(w, req) }

// Routes lists the declared routes in registration order.
func (r *Router) Routes() []Route { return slices.Clone(r.routes) }

// Route returns the declared route for a pattern.
func (r *Router) Route(pattern string) (Route, bool) {
	i, ok := r.byPat[pattern]
	if !ok {
		return Route{}, false
	}
	return r.routes[i], true
}
