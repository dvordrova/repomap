package storefixture

import "net/http"

// ReturnedReadyHandler deliberately has an empty body: registration, not an
// outgoing call inside the callback, makes its native declaration relevant.
func ReturnedReadyHandler() http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

type routeDefinition struct {
	Path   string
	Handle http.HandlerFunc
}

func newRouteDefinition(path string) *routeDefinition {
	return &routeDefinition{Path: path, Handle: ReturnedReadyHandler()}
}

func requireRouteToken(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

func registerRouteDefinition(path string, handler http.HandlerFunc) {
	http.HandleFunc(path, requireRouteToken(handler))
}

type localRouteNames struct{}

func (localRouteNames) HandleFunc(string, http.HandlerFunc) {}

// RegisterHTTPDefinitions retains both actual constructor instances. The
// runtime input and the same-named local method are negative controls.
func RegisterHTTPDefinitions(runtimePath string) {
	mux := http.NewServeMux()
	mux.Handle("GET /health", ReturnedReadyHandler())
	mux.HandleFunc("PATCH\t/tasks/{id}", ReturnedReadyHandler())
	mux.HandleFunc("HEAD status.example/{$}", ReturnedReadyHandler())
	update := newRouteDefinition("/v1/update")
	metrics := newRouteDefinition("/v1/metrics")
	registerRouteDefinition(update.Path, update.Handle)
	registerRouteDefinition(metrics.Path, metrics.Handle)
	direct := newRouteDefinition("/direct-field")
	http.HandleFunc(direct.Path, direct.Handle)
	registerRouteDefinition(runtimePath, ReturnedReadyHandler())
	localRouteNames{}.HandleFunc("GET /not-a-route", ReturnedReadyHandler())
}

func alternativeReadyHandler(choice bool) http.HandlerFunc {
	if choice {
		return func(http.ResponseWriter, *http.Request) {}
	}
	return func(http.ResponseWriter, *http.Request) {}
}

func RegisterAlternativeHTTP(choice bool) {
	http.HandleFunc("/alternative", alternativeReadyHandler(choice))
}

func RegisterSameLineHTTP(once bool) {
	for http.HandleFunc("/same", ReturnedReadyHandler()); once; http.HandleFunc("/same", ReturnedReadyHandler()) {
		once = false
	}
}

func unusedRouteDefinition() *routeDefinition {
	return newRouteDefinition("/never-registered")
}

func RegisterUnknownHTTP(handler http.HandlerFunc) {
	definition := &routeDefinition{Path: "/unknown-handler", Handle: handler}
	http.HandleFunc(definition.Path, definition.Handle)
	changed := newRouteDefinition("/changed-handler")
	changed.Handle = handler
	http.HandleFunc(changed.Path, changed.Handle)
}

// The interface's declared API and unresolved runtime implementation are two
// native views of one source invocation, not two transport calls.
func CallDeclaredHTTPTransport(transport http.RoundTripper, request *http.Request) (*http.Response, error) {
	return transport.RoundTrip(request)
}

type interfaceHTTPApplication struct{}

// The named function conversion and interface return match a handler factory
// whose empty callback is passed to Handle rather than HandleFunc.
func (*interfaceHTTPApplication) health() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

func alternativeInterfaceHandler(choice bool) http.Handler {
	if choice {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

func openInterfaceHandler(choice bool, unknown http.Handler) http.Handler {
	if choice {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	return unknown
}

func RegisterInterfaceHTTP(choice bool, unknown http.Handler) {
	app := &interfaceHTTPApplication{}
	mux := http.NewServeMux()
	mux.Handle("GET /interface-health", app.health())
	mux.Handle("/interface-alternative", alternativeInterfaceHandler(choice))
	mux.Handle("/interface-open", openInterfaceHandler(choice, unknown))
	mux.Handle("/interface-unknown", unknown)
	mux.Handle("/interface-external", http.NewServeMux())
}

// Both functions call Header.Set. The receiver, not that API name, records
// whether the original request or the response is changed. The middleware
// continues the same request through its captured handler.
func withRequestScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Scope-OrgID", "fixture-request")
		next.ServeHTTP(w, r)
	})
}

func writeScopeResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Scope-OrgID", "fixture-response")
	w.WriteHeader(http.StatusNoContent)
}

type methodArgumentApplication struct{}

func (*methodArgumentApplication) first(next http.Handler) http.Handler  { return next }
func (*methodArgumentApplication) second(next http.Handler) http.Handler { return next }

func receiveMethodArguments(values ...func(http.Handler) http.Handler) {}

// These are passed method values, not executions of the methods or a claim
// about the receiving function. Repetition and source order must survive.
func PassMethodArgumentExpressions(app *methodArgumentApplication) {
	receiveMethodArguments(app.first, app.second, app.first, (app.second))
	receiveMethodArguments(app.second, app.first)
	for receiveMethodArguments(app.first); false; receiveMethodArguments(app.second) {
	}
	values := []func(http.Handler) http.Handler{app.first, app.second}
	receiveMethodArguments(values...)
	receiveMethodArguments(func(next http.Handler) http.Handler {
		panic("body-not-provider-evidence")
	})
	receiveMethodArguments((func() *methodArgumentApplication {
		panic("receiver-body-not-provider-evidence")
	})().first)
}
