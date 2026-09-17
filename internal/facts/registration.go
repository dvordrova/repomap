package facts

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A registration is the shape of a call the repository does not own and
// that hands something to the outside: a repository callable, a repository
// value under an address, or an address on its own. Route registrations,
// message consumers, timers, plugin hooks and client requests all have this
// shape; which of them a registration is, the reading stage decides. Nothing
// here knows a framework.
//
// The shape keeps everything a reader would use: the call word as written,
// every string literal in order, the address literal with the mount prefixes
// observed for its receiver, the verb the word or a literal states, the
// callable handed over, and the external symbol behind the call.

func (b *builder) addRegistrations(target *targetContext) {
	values := newRouteValueReader(target)
	originsByValue := target.routeValueOrigins()
	prefixes := target.prefixesByObject()
	var shapes []registrationShape
	for _, relation := range target.input.Index.Relations {
		if target.ownsCallee(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			if target.ownsReceiver(pattern) {
				continue
			}
			shapes = append(shapes, target.registrationShape(relation, pattern, originsByValue, values))
		}
	}
	// A value that received a callable holds work; a literal handed to that
	// value afterwards (Start(":8080") on the router holding the routes) is
	// a registration too, and the reading stage says what it does.
	holders := make(map[Anchor]bool)
	for _, shape := range shapes {
		if shape.handlerID != "" && shape.holder != nil {
			holders[*shape.holder] = true
		}
	}
	for _, shape := range shapes {
		if !shape.accept(holders) {
			continue
		}
		owner := shape.pattern.ReceiverID
		if len(prefixes[owner]) == 0 {
			owner = shape.relation.FromID
		}
		b.addRegistration(target, shape, prefixes[owner], values)
	}
}

type registrationShape struct {
	relation programindex.Relation
	pattern  programindex.RelationPattern
	word     string
	literals []string
	// address is the positional argument carrying the address literal, when
	// one exists; its value may come through variables and parameters.
	address *programindex.PatternArgument
	// firstLiteral is the first positional literal that states no verb; it
	// names what is handed over when no address does.
	firstLiteral *programindex.PatternArgument
	handlerName  string
	handlerID    string
	handedValue  bool
	// handed reports that the call receives something of the repository's:
	// a callable, the decorated declaration, a value, a produced value.
	handed bool
	// holder is the value the call acts on, at the call that produced it.
	holder      *Anchor
	origin      string
	originKnown bool
	method      string
}

// registrationShape reads what a call outside the repository is given: its
// literals, the address among them, the callable, the value it acts on and
// the external symbol behind it. accept then says whether that is a
// registration.
func (target *targetContext) registrationShape(relation programindex.Relation, pattern programindex.RelationPattern, originsByValue map[string][]programindex.ExternalSymbol, values *routeValueReader) registrationShape {
	shape := registrationShape{relation: relation, pattern: pattern, word: pattern.Selector}
	if relation.Kind == programindex.RelationDecorates {
		if object, ok := target.object(relation.FromID); ok {
			shape.handlerName, shape.handlerID = object.Name, object.ID
		}
	} else {
		shape.handlerName, shape.handlerID = target.routeHandler(relation, pattern)
	}
	produced := false
	for position := range pattern.Arguments {
		argument := &pattern.Arguments[position]
		if value, _, literal := literalValue(*argument); literal {
			shape.literals = append(shape.literals, value)
			// The first positional literal names what is handed over. Keyword
			// literals (Command{Use: "serve", Short: "…"}) have no order the
			// index keeps, so they stay values without one being the address.
			if argument.Keyword == "" && shape.firstLiteral == nil && !isHTTPVerb(value) {
				shape.firstLiteral = argument
			}
			if shape.address == nil && argument.Keyword == "" && isAddressLiteral(value) {
				shape.address = argument
			}
			continue
		}
		if argument.Kind != programindex.PatternDynamic || argument.Keyword != "" {
			continue
		}
		callable := false
		for _, id := range argument.ObjectIDs {
			if object, ok := target.object(id); ok && isCallable(object) {
				callable = true
			}
		}
		if shape.address == nil && !callable && resolvesToAddress(values, *argument) {
			// An address given through a variable, field or parameter.
			shape.address = argument
			continue
		}
		for _, id := range argument.ObjectIDs {
			if object, ok := target.object(id); ok && (object.Kind == programindex.ObjectVariable || object.Kind == programindex.ObjectModule) {
				shape.handedValue = true
			}
		}
		if len(argument.ObjectIDs) == 0 && argument.Origin != nil && argument.Origin.Kind == "call_result" {
			produced = true
		}
	}
	for _, origin := range target.callOrigins(relation, pattern, originsByValue) {
		shape.originKnown = true
		if shape.origin == "" {
			shape.origin = externalSymbolName(origin)
		}
	}
	shape.handed = shape.handlerID != "" || relation.Kind == programindex.RelationDecorates || produced || shape.handedValue
	shape.holder = target.holderRoot(pattern)
	shape.method = statedMethod(pattern)
	return shape
}

// accept recognizes a registration: a call with something handed over (a
// repository callable or the decorated declaration, an address, or a
// repository value or produced value together with a literal naming it, as
// Register("k6/x/dns", new(DNS))), an address on its own, or a literal
// handed to a value that holds callables. A call given only data literals
// (Sprintf, log, a field path) is not a registration.
func (shape *registrationShape) accept(holders map[Anchor]bool) bool {
	switch {
	case shape.handed && shape.address != nil:
	case shape.handed && shape.firstLiteral != nil:
		// What is handed over is registered under the first literal, whatever
		// its shape: path("health", view), subscribe("orders.created", handle).
		shape.address = shape.firstLiteral
	case shape.handed && shape.relation.Kind == programindex.RelationDecorates, shape.handlerID != "":
	case shape.address != nil:
	case shape.holder != nil && holders[*shape.holder] && shape.firstLiteral != nil:
		shape.address = shape.firstLiteral
	default:
		return false
	}
	return true
}

// holderRoot is the value a call acts on, followed back through the calls
// outside the repository that produced it: a route put into a group made
// from a router is held by the router's value. The receiver gives the
// value; a produced value handed as an argument (ListenAndServe(":8080",
// mux)) gives it when the receiver does not. A value the code did not
// produce by a call has no holder.
func (target *targetContext) holderRoot(pattern programindex.RelationPattern) *Anchor {
	start := producedAt(pattern.ReceiverValue)
	for position := range pattern.Arguments {
		argument := pattern.Arguments[position]
		if start != nil || argument.Kind != programindex.PatternDynamic {
			break
		}
		callable := false
		for _, id := range argument.ObjectIDs {
			if object, ok := target.object(id); ok && isCallable(object) {
				callable = true
			}
		}
		if !callable {
			start = producedAt(argument.Origin)
		}
	}
	if start == nil {
		return nil
	}
	at := *start
	for seen := map[sourcevalue.Anchor]bool{}; !seen[at]; {
		seen[at] = true
		var next *sourcevalue.Anchor
		for _, producer := range target.producersAt(at) {
			if target.ownsCallee(producer) {
				continue
			}
			for _, produced := range producer.Patterns {
				if location := produced.Location; location != nil && location.Path == at.Path && location.Line == at.Line && location.Column == at.Column {
					next = producedAt(produced.ReceiverValue)
				}
			}
		}
		if next == nil {
			break
		}
		at = *next
	}
	return &Anchor{Path: at.Path, Line: at.Line, Column: at.Column}
}

func producedAt(value *sourcevalue.Value) *sourcevalue.Anchor {
	if value == nil || value.Kind != "call_result" || value.Anchor == nil {
		return nil
	}
	return value.Anchor
}

// isAddressLiteral is the one shape rule about a value: a path, a URL, or a
// "VERB host/path" pattern is an address; a message, a format, a key is not.
func isAddressLiteral(value string) bool {
	if strings.HasPrefix(value, "/") || strings.Contains(value, "://") {
		return true
	}
	method, _, ok := goServeMuxMethodAndPath(value)
	return ok && method != "ANY"
}

// resolvesToAddress follows a dynamic argument to the literals it can carry.
func resolvesToAddress(values *routeValueReader, argument programindex.PatternArgument) bool {
	for _, literal := range values.argument(argument) {
		if isAddressLiteral(literal.text) {
			return true
		}
	}
	return false
}

// statedMethod is the HTTP verb the call states itself: as its word (get,
// post), as its first literal (Method("GET", …)), or inside a pattern
// literal ("GET /health"). Any other word states no verb.
func statedMethod(pattern programindex.RelationPattern) string {
	if isHTTPVerb(pattern.Selector) {
		return strings.ToUpper(pattern.Selector)
	}
	for _, argument := range pattern.Arguments {
		value, _, literal := literalValue(argument)
		if !literal {
			continue
		}
		if isHTTPVerb(value) {
			return strings.ToUpper(value)
		}
		if method, _, ok := goServeMuxMethodAndPath(value); ok && method != "ANY" && isHTTPVerb(method) {
			return strings.ToUpper(method)
		}
	}
	return ""
}

func isHTTPVerb(value string) bool {
	switch strings.ToUpper(value) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

// addRegistration emits one fact per address the registration answers on. A
// router mounted under three prefixes registers the same handler on three
// paths; a registration without an address is one fact.
func (b *builder) addRegistration(target *targetContext, shape registrationShape, prefixes []routePrefix, values *routeValueReader) {
	anchor := target.patternAnchor(shape.relation, shape.pattern)
	if anchor == nil {
		return
	}
	addresses := []routePrefix{{}}
	if shape.address != nil {
		addresses = addresses[:0]
		for _, observed := range addressLiterals(values, *shape.address) {
			path := observed.text
			if method, rest, ok := goServeMuxMethodAndPath(path); ok && method != "ANY" {
				path = rest
			}
			if len(prefixes) == 0 {
				addresses = append(addresses, routePrefix{path: path, evidence: observed.evidence, possible: observed.possible})
				continue
			}
			for _, prefix := range prefixes {
				addresses = append(addresses, routePrefix{path: joinRoutePath(prefix.path, path), evidence: mergeRouteEvidence(prefix.evidence, observed.evidence), possible: prefix.possible || observed.possible})
			}
		}
	}
	for _, address := range addresses {
		if !b.once(strings.Join([]string{string(KindRegistration), target.target.ID, anchor.String(), strconv.Itoa(anchor.Column), shape.word, address.path}, "\x00")) {
			continue
		}
		resolution := ResolutionExact
		if address.possible || !shape.originKnown {
			resolution = ResolutionPossible
		}
		b.add(target.root, Fact{
			Kind:       KindRegistration,
			TargetID:   target.target.ID,
			Anchor:     anchor,
			Holder:     shape.holder,
			Key:        shape.word,
			Values:     shape.literals,
			Path:       address.path,
			Method:     shape.method,
			Symbol:     shape.handlerName,
			ObjectID:   shape.handlerID,
			Text:       shape.origin,
			Resolution: resolution,
			Evidence:   address.evidence,
		}, shape.word, address.path, shape.handlerName)
	}
}

// addressLiterals reads an address argument through the variables and
// parameters it may come from, deduplicating equal texts.
func addressLiterals(values *routeValueReader, argument programindex.PatternArgument) []routeLiteral {
	byText := make(map[string]routeLiteral)
	for _, literal := range values.argument(argument) {
		if previous, exists := byText[literal.text]; exists {
			literal.evidence = mergeRouteEvidence(previous.evidence, literal.evidence)
			literal.possible = literal.possible || previous.possible
		}
		byText[literal.text] = literal
	}
	keys := make([]string, 0, len(byText))
	for text := range byText {
		keys = append(keys, text)
	}
	sort.Strings(keys)
	result := make([]routeLiteral, 0, len(keys))
	for _, text := range keys {
		result = append(result, byText[text])
	}
	return result
}

// ownsCallee reports a call whose target is a declaration of this
// repository, including a workspace package the compiler resolved here. Such
// a call is internal delegation whatever its name.
func (target *targetContext) ownsCallee(relation programindex.Relation) bool {
	if len(relation.ToIDs) == 0 {
		return false
	}
	for _, id := range relation.ToIDs {
		if !target.ownsObject(id) {
			return false
		}
	}
	return true
}

// ownsReceiver reports a call on a value the repository produced itself: the
// result of a workspace package's factory, or of any repository call, is
// repository code, not a framework.
func (target *targetContext) ownsReceiver(pattern programindex.RelationPattern) bool {
	for _, id := range pattern.ReceiverOriginIDs {
		if object, ok := target.object(id); ok && object.Kind == programindex.ObjectExternalSymbol && target.ownsObject(id) {
			return true
		}
	}
	value := pattern.ReceiverValue
	if value == nil || value.Kind != "call_result" || value.Anchor == nil {
		return false
	}
	// A repository function produced the value, or a repository class that
	// declares the called member itself. An instance of a class that inherits
	// the member from outside is not owned.
	for _, id := range target.producerObjects(*value.Anchor) {
		object, ok := target.object(id)
		if !ok {
			continue
		}
		if isCallable(object) || object.Kind == programindex.ObjectExternalSymbol && target.ownsObject(id) ||
			object.Kind == programindex.ObjectType && target.classMembers[id][pattern.Selector] {
			return true
		}
	}
	return false
}

// producerObjects are the resolved callees of the call at an anchor.
func (target *targetContext) producerObjects(at sourcevalue.Anchor) []string {
	var result []string
	for _, producer := range target.producersAt(at) {
		if target.ownsCallee(producer) {
			result = append(result, producer.ToIDs...)
		}
	}
	return result
}

func (target *targetContext) producersAt(at sourcevalue.Anchor) []programindex.Relation {
	if target.producers == nil {
		target.producers = make(map[sourcevalue.Anchor][]programindex.Relation)
		for _, relation := range target.input.Index.Relations {
			for _, pattern := range relation.Patterns {
				if pattern.Location != nil {
					key := sourcevalue.Anchor{Path: pattern.Location.Path, Line: pattern.Location.Line, Column: pattern.Location.Column}
					target.producers[key] = append(target.producers[key], relation)
				}
			}
		}
	}
	return target.producers[at]
}

func (target *targetContext) ownsObject(id string) bool {
	object, ok := target.object(id)
	if !ok {
		return false
	}
	return object.Kind != programindex.ObjectExternalSymbol || object.External != nil && object.External.RepositoryPath != ""
}

// callOrigins are the external symbols behind a call: its target, its
// receiver's origin, or the single external base class a local receiver type
// inherits the method from.
func (target *targetContext) callOrigins(relation programindex.Relation, pattern programindex.RelationPattern, originsByValue map[string][]programindex.ExternalSymbol) []programindex.ExternalSymbol {
	origins := target.externalOrigins(relation, pattern)
	origins = append(origins, originsByValue[pattern.ReceiverID]...)
	receiverTypes := append([]string(nil), pattern.ReceiverOriginIDs...)
	if value := pattern.ReceiverValue; value != nil && value.Kind == "call_result" && value.Anchor != nil {
		receiverTypes = append(receiverTypes, target.producerObjects(*value.Anchor)...)
	}
	for _, id := range receiverTypes {
		if origin, ok := target.inheritedMethodOrigin(id, pattern.Selector); ok {
			origins = append(origins, origin)
		}
	}
	return origins
}

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

func externalSymbolName(symbol programindex.ExternalSymbol) string {
	name := symbol.Name
	if symbol.Receiver != "" {
		name = strings.TrimPrefix(symbol.Receiver, "*") + "." + name
	}
	return symbol.PackagePath + "." + name
}

// routeHandler finds the callable handed over: the callback the adapter
// recorded for an argument, or a single callable object an argument names.
func (target *targetContext) routeHandler(relation programindex.Relation, pattern programindex.RelationPattern) (string, string) {
	candidates := make(map[string]programindex.Object)
	add := func(id string) {
		if object, found := target.object(id); found && isCallable(object) {
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

func isCallable(object programindex.Object) bool {
	return object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod || object.Kind == programindex.ObjectLambda
}

// net/http's ServeMux pattern syntax: an optional method, a space, then the
// host/path. Kept exactly; a GET pattern also serves HEAD natively.
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
