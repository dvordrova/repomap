package facts

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

var routeSelectors = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "patch": {}, "delete": {}, "head": {}, "options": {},
	"route": {}, "mount": {}, "handle": {}, "handlefunc": {}, "method": {}, "websocket": {}, "api_route": {},
}

var callSelectors = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "patch": {}, "delete": {}, "request": {}, "fetch": {},
}

// serverPackages are the web frameworks whose route registrations we trust.
var serverPackages = []string{
	"fastapi", "flask", "django", "starlette", "aiohttp",
	"express", "koa", "fastify", "hono",
	"net/http", "github.com/go-chi/chi", "github.com/gin-gonic/gin", "github.com/labstack/echo", "github.com/gofiber/fiber",
}

// clientPackages map an HTTP client package to the symbol names that perform
// requests; nil accepts every selector in callSelectors.
var clientPackages = map[string][]string{
	"axios": nil, "node-fetch": nil, "ky": nil, "got": nil, "superagent": nil,
	"platform:javascript": {"fetch"},
	"requests":            nil, "httpx": nil, "urllib": nil, "aiohttp": nil,
	"net/http": {"Get", "Post", "Head", "PostForm", "Do", "NewRequest", "NewRequestWithContext"},
}

type httpSide struct {
	server  bool
	client  bool
	pkgPath string
}

// classifyHTTP decides whether a pattern is a server registration or a
// client call from the external origin behind it. Packages that serve both
// roles (aiohttp, net/http) are server-side only in decorator form or with a
// registration selector.
func classifyHTTP(origins []programindex.ExternalSymbol, form programindex.PatternForm, selector string) httpSide {
	for _, origin := range origins {
		if names, isClient := clientPackage(origin); isClient {
			if _, isServer := packageMatches(origin.PackagePath, serverPackages...); isServer && registrationForm(form, selector) {
				return httpSide{server: true, pkgPath: origin.PackagePath}
			}
			if names == nil || containsFold(names, origin.Name) {
				return httpSide{client: true, pkgPath: origin.PackagePath}
			}
			continue
		}
		if _, isServer := packageMatches(origin.PackagePath, serverPackages...); isServer {
			return httpSide{server: true, pkgPath: origin.PackagePath}
		}
	}
	return httpSide{}
}

func clientPackage(origin programindex.ExternalSymbol) ([]string, bool) {
	if origin.PackagePath == "platform:javascript" {
		return clientPackages[origin.PackagePath], true
	}
	for candidate, names := range clientPackages {
		if candidate == "platform:javascript" {
			continue
		}
		if _, ok := packageMatches(origin.PackagePath, candidate); ok {
			return names, true
		}
	}
	return nil, false
}

func registrationForm(form programindex.PatternForm, selector string) bool {
	if form == programindex.PatternDecoratorCall {
		return true
	}
	switch selector {
	case "handle", "handlefunc", "mount", "route", "method", "websocket", "api_route":
		return true
	default:
		return false
	}
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func (b *builder) addHTTP(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			selector := strings.ToLower(pattern.Selector)
			_, isRoute := routeSelectors[selector]
			_, isCall := callSelectors[selector]
			if !isRoute && !isCall {
				continue
			}
			side := classifyHTTP(target.externalOrigins(relation, pattern), pattern.Form, selector)
			switch {
			case side.server && isRoute:
				b.addRoute(target, relation, pattern, selector)
			case side.client && isCall && pattern.Form == programindex.PatternCall:
				b.addCall(target, relation, pattern, selector)
			}
		}
	}
}

func (b *builder) addRoute(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, selector string) {
	method, pathValue, templated, ok := routeMethodAndPath(pattern, selector)
	if !ok || !strings.HasPrefix(pathValue, "/") {
		return
	}
	anchor := target.patternAnchor(relation, pattern)
	if anchor == nil {
		return
	}
	if !b.once(strings.Join([]string{string(KindHTTPRoute), anchor.String(), method, pathValue}, "\x00")) {
		return
	}
	symbol, objectID := target.routeHandler(relation, pattern)
	resolution := ResolutionExact
	if templated {
		resolution = ResolutionPossible
	}
	b.add(target.root, Fact{
		Kind:       KindHTTPRoute,
		TargetID:   target.target.ID,
		Anchor:     anchor,
		Method:     method,
		Path:       pathValue,
		Symbol:     symbol,
		ObjectID:   objectID,
		Resolution: resolution,
	}, method, pathValue)
}

// routeMethodAndPath maps a selector to its HTTP method. Registration
// selectors without a verb (route, handle, mount...) accept any method, and
// a Go "Method(verb, path, handler)" call carries the verb as its first
// literal argument.
func routeMethodAndPath(pattern programindex.RelationPattern, selector string) (method, pathValue string, templated, ok bool) {
	pathPosition := 1
	switch selector {
	case "get", "post", "put", "patch", "delete", "head", "options":
		method = strings.ToUpper(selector)
	case "websocket":
		method = "WEBSOCKET"
	case "method":
		verb, verbOK := positionalArgument(pattern, 1)
		verbValue, _, literal := literalValue(verb)
		if !verbOK || !literal || !isHTTPVerb(verbValue) {
			return "", "", false, false
		}
		method = strings.ToUpper(verbValue)
		pathPosition = 2
	default:
		method = "ANY"
	}
	argument, found := positionalArgument(pattern, pathPosition)
	if !found {
		return "", "", false, false
	}
	pathValue, templated, ok = literalValue(argument)
	return method, pathValue, templated, ok
}

func isHTTPVerb(value string) bool {
	switch strings.ToUpper(value) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

// routeHandler finds the function bound to a route: the decorated function,
// the callback argument recorded by a passes_callback relation, or a single
// function object referenced by an argument.
func (target *targetContext) routeHandler(relation programindex.Relation, pattern programindex.RelationPattern) (string, string) {
	if relation.Kind == programindex.RelationDecorates {
		if object, ok := target.object(relation.FromID); ok {
			return object.Name, object.ID
		}
		return "", ""
	}
	for _, argument := range pattern.Arguments {
		if handlerID, ok := target.callbacks[argument.ID]; ok {
			if object, found := target.object(handlerID); found {
				return object.Name, object.ID
			}
		}
	}
	for _, argument := range pattern.Arguments {
		if len(argument.ObjectIDs) != 1 {
			continue
		}
		object, found := target.object(argument.ObjectIDs[0])
		if found && (object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod || object.Kind == programindex.ObjectLambda) {
			return object.Name, object.ID
		}
	}
	return "", ""
}

func (b *builder) addCall(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, selector string) {
	argument, found := positionalArgument(pattern, 1)
	if !found {
		return
	}
	pathValue, templated, ok := literalValue(argument)
	if !ok || !isRequestPath(pathValue) {
		return
	}
	anchor := target.patternAnchor(relation, pattern)
	if anchor == nil {
		return
	}
	method := callMethod(pattern, selector)
	if !b.once(strings.Join([]string{string(KindHTTPCall), anchor.String(), method, pathValue}, "\x00")) {
		return
	}
	symbol, objectID := target.enclosingSymbol(relation.FromID)
	resolution := ResolutionExact
	if templated {
		resolution = ResolutionPossible
	}
	b.add(target.root, Fact{
		Kind:       KindHTTPCall,
		TargetID:   target.target.ID,
		Anchor:     anchor,
		Method:     method,
		Path:       pathValue,
		Symbol:     symbol,
		ObjectID:   objectID,
		Resolution: resolution,
	}, method, pathValue)
}

// callMethod resolves the verb of a client call; fetch/request default to GET
// unless a literal "method" option or second positional verb says otherwise.
func callMethod(pattern programindex.RelationPattern, selector string) string {
	switch selector {
	case "get", "post", "put", "patch", "delete":
		return strings.ToUpper(selector)
	}
	if argument, ok := keywordArgument(pattern, "method"); ok {
		if value, _, literal := literalValue(argument); literal && isHTTPVerb(value) {
			return strings.ToUpper(value)
		}
	}
	if argument, ok := positionalArgument(pattern, 2); ok {
		if value, _, literal := literalValue(argument); literal && isHTTPVerb(value) {
			return strings.ToUpper(value)
		}
	}
	return "GET"
}

func isRequestPath(value string) bool {
	return strings.HasPrefix(value, "/") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
