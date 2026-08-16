package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The group prefix has to be spliced after the method verb: Go 1.22 patterns
// are "METHOD /path", so naive concatenation would produce "GET /api/v1GET ...".
func TestRouteGroupPattern(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		input  string
		want   string
	}{
		{name: "method and path", prefix: "/api/v1", input: "GET /usage", want: "GET /api/v1/usage"},
		{name: "post route", prefix: "/api/v1", input: "POST /recommendations", want: "POST /api/v1/recommendations"},
		{name: "nested webhook path", prefix: "/api/v1", input: "POST /billing/webhook", want: "POST /api/v1/billing/webhook"},
		{name: "pattern without a method", prefix: "/api/v1", input: "/feedback", want: "/api/v1/feedback"},
		{name: "empty prefix keeps the pattern", prefix: "", input: "GET /healthz", want: "GET /healthz"},
		{name: "empty prefix subtree", prefix: "", input: "/legal/", want: "/legal/"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			group := RouteGroup{router: NewRouter(), prefix: testCase.prefix}
			if diff := cmp.Diff(testCase.want, group.pattern(testCase.input)); diff != "" {
				t.Errorf("unexpected pattern (-want +got):\n%s", diff)
			}
		})
	}
}

// Middleware coverage is structural: everything registered through a group runs
// the group's chain, outermost first, and the router remembers every pattern so
// a caller can enumerate them (for example, to verify each one carries the
// middleware it is supposed to).
func TestRouteGroupAppliesChainInOrderAndRecordsPatterns(t *testing.T) {
	var callOrder []string
	tag := func(label string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callOrder = append(callOrder, label)
				next.ServeHTTP(w, r)
			})
		}
	}

	routerUnderTest := NewRouter()
	public := routerUnderTest.Group("")
	public.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	api := routerUnderTest.Group("/api/v1", tag("outer"), tag("inner"))
	api.HandleFunc("GET /usage", func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, "handler")
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	routerUnderTest.Mux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if diff := cmp.Diff([]string{"outer", "inner", "handler"}, callOrder); diff != "" {
		t.Errorf("unexpected middleware order (-want +got):\n%s", diff)
	}
	wantPatterns := []string{"GET /healthz", "GET /api/v1/usage"}
	if diff := cmp.Diff(wantPatterns, routerUnderTest.Patterns()); diff != "" {
		t.Errorf("unexpected registered patterns (-want +got):\n%s", diff)
	}

	publicRecorder := httptest.NewRecorder()
	routerUnderTest.Mux().ServeHTTP(publicRecorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if publicRecorder.Code != http.StatusOK {
		t.Errorf("public route status = %d, want %d", publicRecorder.Code, http.StatusOK)
	}
	if len(callOrder) != 3 {
		t.Errorf("public route ran the api chain: %v", callOrder)
	}
}

// Mutating the slice Patterns returns must not change the router's own
// record: the method hands callers a copy, not the internal slice.
func TestRouterPatternsReturnsACopy(t *testing.T) {
	routerUnderTest := NewRouter()
	routerUnderTest.Group("").HandleFunc("GET /healthz", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	patterns := routerUnderTest.Patterns()
	patterns[0] = "GET /tampered"

	if diff := cmp.Diff([]string{"GET /healthz"}, routerUnderTest.Patterns()); diff != "" {
		t.Errorf("mutating the returned slice changed the router's record (-want +got):\n%s", diff)
	}
}
