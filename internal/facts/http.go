package facts

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

var routeSelectors = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "patch": {}, "delete": {}, "head": {}, "options": {},
	"route": {}, "mount": {}, "handle": {}, "handlefunc": {}, "method": {}, "websocket": {}, "api_route": {},
	"path": {},
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
	values := newRouteValueReader(target)
	originsByValue := target.routeValueOrigins()
	prefixes := target.prefixesByObject(originsByValue)
	for _, relation := range target.input.Index.Relations {
		for _, pattern := range relation.Patterns {
			selector := strings.ToLower(pattern.Selector)
			_, isRoute := routeSelectors[selector]
			_, isCall := callSelectors[selector]
			if !isRoute && !isCall {
				continue
			}
			origins := target.externalOrigins(relation, pattern)
			origins = append(origins, originsByValue[pattern.ReceiverID]...)
			for _, id := range pattern.ReceiverOriginIDs {
				if origin, ok := target.inheritedMethodOrigin(id, pattern.Selector); ok {
					origins = append(origins, origin)
				}
			}
			side := classifyHTTP(origins, pattern.Form, selector)
			switch {
			case side.server && isRoute:
				// Mount registrations describe a router, not another handler.
				if target.isRouterMount(relation, pattern, origins) {
					continue
				}
				owner := pattern.ReceiverID
				if len(prefixes[owner]) == 0 {
					owner = relation.FromID
				}
				b.addRoute(target, relation, pattern, selector, prefixes[owner], values)
			case side.client && isCall && pattern.Form == programindex.PatternCall:
				b.addCall(target, relation, pattern, selector)
			}
		}
	}
}

// inheritedMethodOrigin follows the adapter's observed Python base classes.
// A local override, incomplete base or multiple inheritance needs method
// resolution evidence we do not have. Neither a class name nor an interface
// implementation is evidence that a method belongs to an external framework.
func (target *targetContext) inheritedMethodOrigin(id, selector string) (programindex.ExternalSymbol, bool) {
	seen := make(map[string]bool)
	for !seen[id] {
		seen[id] = true
		object, ok := target.objects[id]
		if !ok {
			break
		}
		if object.Kind == programindex.ObjectExternalSymbol && object.External != nil && object.External.RepositoryPath == "" {
			return *object.External, true
		}
		if object.Kind != programindex.ObjectType || target.classMembers[id][selector] {
			break
		}
		bases := target.classBases[id]
		if len(bases) != 1 {
			break
		}
		id = bases[0]
	}
	return programindex.ExternalSymbol{}, false
}

// addRoute emits one fact per path this registration answers on. A router
// mounted under three prefixes registers the same handler on three paths, and
// printing only the un-prefixed one would print a path nobody can call.
func (b *builder) addRoute(
	target *targetContext,
	relation programindex.Relation,
	pattern programindex.RelationPattern,
	selector string,
	prefixes []routePrefix,
	values *routeValueReader,
) {
	method, position, ok := routeMethod(pattern, selector)
	if !ok {
		return
	}
	argument, found := positionalArgument(pattern, position)
	if !found {
		return
	}
	paths := make(map[string]routeLiteral)
	for _, path := range values.argument(argument) {
		if previous, exists := paths[path.text]; exists {
			path.evidence = mergeRouteEvidence(previous.evidence, path.evidence)
			path.possible = path.possible || previous.possible
		}
		paths[path.text] = path
	}
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		b.addResolvedRoute(target, relation, pattern, selector, prefixes, method, paths[path])
	}
}

func (b *builder) addResolvedRoute(target *targetContext, relation programindex.Relation, pattern programindex.RelationPattern, selector string, prefixes []routePrefix, method string, observed routeLiteral) {
	pathValue := observed.text
	serveMux := false
	if target.target.Language == "go" && (selector == "handle" || selector == "handlefunc") {
		for _, origin := range target.externalOrigins(relation, pattern) {
			serveMux = serveMux || origin.PackagePath == "net/http"
		}
	}
	if serveMux {
		var ok bool
		method, pathValue, ok = goServeMuxMethodAndPath(pathValue)
		if !ok {
			return
		}
	}
	if selector == "path" {
		if _, django := packageMatches(classifyHTTP(target.externalOrigins(relation, pattern), pattern.Form, selector).pkgPath, "django"); !django {
			return
		}
		pathValue = "/" + strings.TrimPrefix(pathValue, "/")
	}
	if !serveMux && !strings.HasPrefix(pathValue, "/") {
		return
	}
	anchor := target.patternAnchor(relation, pattern)
	if anchor == nil {
		return
	}
	symbol, objectID := target.routeHandler(relation, pattern)
	resolution := ResolutionExact
	if observed.possible {
		resolution = ResolutionPossible
	}
	paths := []routePrefix{{path: pathValue, evidence: observed.evidence}}
	if len(prefixes) > 0 {
		paths = paths[:0]
		for _, prefix := range prefixes {
			paths = append(paths, routePrefix{path: joinRoutePath(prefix.path, pathValue), evidence: mergeRouteEvidence(prefix.evidence, observed.evidence)})
		}
	}
	for _, resolved := range paths {
		if !b.once(strings.Join([]string{string(KindHTTPRoute), target.target.ID, anchor.String(), strconv.Itoa(anchor.Column), method, resolved.path}, "\x00")) {
			continue
		}
		b.add(target.root, Fact{
			Kind:       KindHTTPRoute,
			TargetID:   target.target.ID,
			Anchor:     anchor,
			Method:     method,
			Path:       resolved.path,
			Symbol:     symbol,
			ObjectID:   objectID,
			Resolution: resolution,
			Evidence:   resolved.evidence,
		}, method, resolved.path)
	}
}

// net/http's ServeMux owns the optional method prefix. Keep host/path and
// wildcards exactly as written; a GET pattern also serves HEAD by its native
// contract, without manufacturing a second registration. Other routers do not
// inherit this syntax merely because their method is named Handle.
func goServeMuxMethodAndPath(pattern string) (method, path string, ok bool) {
	method, path = "ANY", pattern
	if i := strings.IndexAny(pattern, " \t"); i >= 0 {
		method, path = pattern[:i], strings.TrimLeft(pattern[i+1:], " \t")
		if method == "" {
			return "", "", false
		}
		for _, r := range method {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
				return "", "", false
			}
		}
	}
	i := strings.IndexByte(path, '/')
	return method, path, i >= 0 && !strings.Contains(path[:i], "{")
}

// routeMethodAndPath maps a selector to its HTTP method. Registration
// selectors without a verb (route, handle, mount...) accept any method, and
// a Go "Method(verb, path, handler)" call carries the verb as its first
// literal argument.
func routeMethodAndPath(pattern programindex.RelationPattern, selector string) (method, pathValue string, templated, ok bool) {
	method, pathPosition, ok := routeMethod(pattern, selector)
	if !ok {
		return "", "", false, false
	}
	argument, found := positionalArgument(pattern, pathPosition)
	if !found {
		return "", "", false, false
	}
	pathValue, templated, ok = literalValue(argument)
	return method, pathValue, templated, ok
}

func routeMethod(pattern programindex.RelationPattern, selector string) (method string, pathPosition int, ok bool) {
	pathPosition = 1
	switch selector {
	case "get", "post", "put", "patch", "delete", "head", "options":
		method = strings.ToUpper(selector)
	case "websocket":
		method = "WEBSOCKET"
	case "method":
		verb, verbOK := positionalArgument(pattern, 1)
		verbValue, _, literal := literalValue(verb)
		if !verbOK || !literal || !isHTTPVerb(verbValue) {
			return "", 0, false
		}
		method = strings.ToUpper(verbValue)
		pathPosition = 2
	default:
		method = "ANY"
	}
	return method, pathPosition, true
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
	candidates := make(map[string]programindex.Object)
	add := func(id string) {
		object, found := target.object(id)
		if found && (object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod || object.Kind == programindex.ObjectLambda) {
			candidates[id] = object
		}
	}
	for _, argument := range pattern.Arguments {
		if handlerID, observed := target.callbacks[argument.ID]; observed {
			if handlerID == "" {
				return "", ""
			}
			add(handlerID)
		}
		if len(argument.ObjectIDs) != 1 || argument.ObjectsOmitted != 0 {
			continue
		}
		add(argument.ObjectIDs[0])
	}
	if len(candidates) == 1 {
		for _, object := range candidates {
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
