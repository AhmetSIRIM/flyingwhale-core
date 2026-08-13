package httpx

import (
	"net/http"
	"strings"
)

// Router wraps an http.ServeMux and records every pattern registered through
// it, so a caller can enumerate the routes it exposes after setup.
type Router struct {
	mux      *http.ServeMux
	patterns []string
}

// NewRouter returns an empty Router ready to register route groups on.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Mux returns the underlying http.ServeMux, for serving the router or for
// wrapping it in server-level middleware.
func (rt *Router) Mux() *http.ServeMux {
	return rt.mux
}

// Patterns returns every pattern registered through the router's route
// groups, in registration order.
func (rt *Router) Patterns() []string {
	return rt.patterns
}

// Group returns a RouteGroup that prefixes every pattern it registers with
// prefix and wraps every handler in chain, outermost middleware first.
func (rt *Router) Group(prefix string, chain ...Middleware) *RouteGroup {
	return &RouteGroup{router: rt, prefix: prefix, chain: chain}
}

// RouteGroup registers handlers that share a path prefix and a middleware
// chain, and reports each registered pattern back to its Router.
type RouteGroup struct {
	router *Router
	prefix string
	chain  []Middleware
}

// Handle registers handler for pattern under the group's prefix, wrapped in
// the group's middleware chain, and records the resulting pattern on the
// group's Router.
func (group RouteGroup) Handle(pattern string, handler http.Handler) {
	full := group.pattern(pattern)
	group.router.patterns = append(group.router.patterns, full)
	group.router.mux.Handle(full, wrapChain(handler, group.chain...))
}

// HandleFunc is Handle for a plain http.HandlerFunc.
func (group RouteGroup) HandleFunc(pattern string, handler http.HandlerFunc) {
	group.Handle(pattern, handler)
}

// pattern splices the group's prefix into pattern after the method verb, so
// a Go 1.22+ pattern like "GET /path" becomes "GET /prefix/path" rather than
// "GET /prefixGET /path".
func (group RouteGroup) pattern(pattern string) string {
	method, path, hasMethod := strings.Cut(pattern, " ")
	if !hasMethod {
		return group.prefix + pattern
	}
	return method + " " + group.prefix + path
}

// wrapChain wraps handler in chain, applying the chain outermost first so
// chain[0] is the first middleware a request reaches.
func wrapChain(handler http.Handler, chain ...Middleware) http.Handler {
	wrapped := handler
	for index := len(chain) - 1; index >= 0; index-- {
		wrapped = chain[index](wrapped)
	}
	return wrapped
}
