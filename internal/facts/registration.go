package facts

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A registration is a call that hands something over to a receiver that
// keeps or runs it: a repository callable, a repository value under an
// address, or an address on its own. The receiver is code the repository
// does not own (this file: route registrations, message consumers, timers,
// plugin hooks, client requests), or the repository's own where its native
// evidence shows it keeps what it is handed: a function that stores the
// callable parameter in a field or a module-level variable (registrar.go,
// the program index's parameter stores: Lua's lua_cpcall keeps pmain in
// CCallS.func, then runs it), a row of a module-level table (tableRow:
// luaL_Reg's {"assert", luaB_assert}), or a statement that starts the
// callable to run on its own (started.go). A repository function that runs
// a callable in place and keeps nothing is delegation, a call the walk
// follows, never a registration (owner decision 2026-09-27 "a"). Which kind
// of entry a registration is, the reading stage decides. Nothing here knows
// a framework.
//
// The shape keeps everything a reader would use: the call word as written,
// every string literal in order, the address literal with the mount prefixes
// observed for its receiver, the verb the word or a literal states, the
// callable handed over, and the external symbol behind the call.

func (b *builder) addRegistrations(target *targetContext) {
	values := target.values()
	originsByValue := target.routeValueOrigins()
	prefixes := target.prefixesByObject()
	starts := target.newStartReader()
	var shapes []registrationShape
	for _, relation := range target.input.Index.Relations {
		if target.unreachable(relation) {
			continue
		}
		// A call starting the repository's own callable to run on its own
		// hands it over (started.go): the one exception, beside table rows,
		// to a call the repository declares being delegation.
		if started := starts.shapes(relation); len(started) > 0 {
			shapes = append(shapes, started...)
			continue
		}
		if target.ownsCallee(relation) && !target.tableRow(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			if target.ownsReceiver(pattern) {
				continue
			}
			shape := target.registrationShape(relation, pattern, originsByValue, values)
			if handed := target.keywordHandoffs(shape); len(handed) > 0 {
				shapes = append(shapes, handed...)
				continue
			}
			shapes = append(shapes, shape)
		}
	}
	shapes = oneShapePerTableRow(target, shapes)
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
		// A started callable is handed to no router: it has no address.
		if shape.invocation != "" {
			b.addRegistration(target, shape, nil, values)
			continue
		}
		owner := shape.pattern.ReceiverID
		if len(prefixes[owner]) == 0 {
			owner = shape.relation.FromID
		}
		b.addRegistration(target, shape, prefixes[owner], values)
	}
}

// oneShapePerTableRow keeps one registration per row of a table the
// repository owns. A row that stores two callables ({"zunion", zunionCommand,
// ..., zunionInterBlockClientOnSwappedKeys}) reaches the index as one
// construction per stored callable, each with the row's literals. Those
// literals, written once at their own source positions, are the row's
// identity: the row is one registration, handing over the callable it writes
// first, and the others stay callbacks the row stores, never inputs of their
// own. This is identity the code carries, not a decision about what a field
// means.
func oneShapePerTableRow(target *targetContext, shapes []registrationShape) []registrationShape {
	first := make(map[string]int)
	var keys []string
	for position, shape := range shapes {
		key := tableRowKey(target, shape)
		keys = append(keys, key)
		if key == "" {
			continue
		}
		if kept, seen := first[key]; !seen || locationBefore(shape.pattern.Location, shapes[kept].pattern.Location) {
			first[key] = position
		}
	}
	result := shapes[:0:0]
	for position, shape := range shapes {
		if keys[position] == "" || first[keys[position]] == position {
			result = append(result, shape)
		}
	}
	return result
}

// tableRowKey is a table row's identity: the table, the record type and each
// literal of the row where it is written. Empty for any other shape, and for
// a row whose literals carry no source position.
func tableRowKey(target *targetContext, shape registrationShape) string {
	if shape.handlerID == "" || !target.tableRow(shape.relation) {
		return ""
	}
	parts := []string{shape.relation.FromID, strings.Join(shape.relation.ToIDs, ",")}
	for _, argument := range shape.pattern.Arguments {
		value, _, literal := literalValue(argument)
		if !literal {
			continue
		}
		if argument.Origin == nil || argument.Origin.Anchor == nil {
			return ""
		}
		at := argument.Origin.Anchor
		parts = append(parts, argument.Keyword+"="+value+"@"+at.Path+":"+strconv.Itoa(at.Line)+":"+strconv.Itoa(at.Column))
	}
	if len(parts) == 2 {
		return ""
	}
	sort.Strings(parts[2:])
	return strings.Join(parts, "\x00")
}

func locationBefore(a, b *programindex.Location) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	case a.Path != b.Path:
		return a.Path < b.Path
	case a.Line != b.Line:
		return a.Line < b.Line
	default:
		return a.Column < b.Column
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
	// constructed reports an instance the repository built (new(DNS)) or a
	// module handed over: what a registering symbol may make an entry of.
	// A value a repository call returned is data, not something registered.
	constructed bool
	// holder is the value the call acts on, at the call that produced it;
	// holderPrefixes are the path literals of the calls between the holder
	// and this value (Group("/articles")), root first.
	holder         *Anchor
	holderPrefixes []string
	origin         string
	originKnown    bool
	method         string
	// invocation is the shared invocation word of a call that starts its
	// repository callee to run on its own (started.go).
	invocation string
	// keyword is the keyword a call hands its callable over under, when it
	// hands several (keywordHandoffs), and at where that entry is written.
	keyword string
	at      *Anchor
}

// keywordHandoffs are the registrations of a call handing several repository
// callables over, each under a keyword of its own: Quil's (q/sketch :setup
// setup :key-pressed on-key), a map of handlers given to a server, a struct
// of an outside type with two function fields, a Python call with two
// callable keyword arguments. The call alone names no one handler, so each
// keyword entry is one registration: its handler is that callable, its
// registrar the call's symbol with the keyword, as a table row's registrar is
// its record type with the field (quil.core.sketch.key-pressed), and its
// place where the entry is written. A callable handed by position beside
// them stays with none; a call handing one callable keeps its one
// registration.
func (target *targetContext) keywordHandoffs(shape registrationShape) []registrationShape {
	if shape.handlerID != "" || shape.relation.Kind == programindex.RelationDecorates {
		return nil
	}
	callables := make(map[string]bool)
	var result []registrationShape
	for _, argument := range shape.pattern.Arguments {
		id := target.argumentCallable(argument)
		if id == "" {
			continue
		}
		callables[id] = true
		if argument.Keyword == "" {
			continue
		}
		object, _ := target.object(id)
		handed := shape
		// The keyword is the word the code wrote for this entry, first
		// among the call's words: the reading names the entry from them.
		handed.literals = append([]string{argument.Keyword}, shape.literals...)
		handed.handlerName, handed.handlerID, handed.handed, handed.keyword = object.Name, object.ID, true, argument.Keyword
		if shape.originKnown {
			handed.origin = shape.origin + "." + argument.Keyword
		}
		if at := argument.Origin; at != nil && at.Anchor != nil {
			handed.at = &Anchor{Path: at.Anchor.Path, Line: at.Anchor.Line, Column: at.Anchor.Column}
		}
		result = append(result, handed)
	}
	if len(callables) < 2 {
		return nil
	}
	return result
}

// argumentCallable is the one repository callable an argument hands over:
// the callback the adapter recorded for it, or the one callable it names.
func (target *targetContext) argumentCallable(argument programindex.PatternArgument) string {
	if handlerID, observed := target.callbacks[argument.ID]; observed {
		if object, ok := target.object(handlerID); ok && isCallable(object) {
			return handlerID
		}
		return ""
	}
	if len(argument.ObjectIDs) != 1 || argument.ObjectsOmitted != 0 {
		return ""
	}
	if object, ok := target.object(argument.ObjectIDs[0]); ok && isCallable(object) {
		return object.ID
	}
	return ""
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
	// addressed reports an address among the call's literals, a field's
	// included ({Method: "GET", Path: "/users"}): a verb literal beside it
	// states the method it is asked with.
	addressed := false
	for position := range pattern.Arguments {
		argument := &pattern.Arguments[position]
		if value, _, literal := literalValue(*argument); literal {
			if value == "" {
				continue
			}
			shape.literals = append(shape.literals, value)
			addressed = addressed || isAddressLiteral(value)
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
			// A module-level value or a module is the repository's own thing
			// handed over; a local variable (an error, a counter) is not.
			if object, ok := target.object(id); ok && (object.Kind == programindex.ObjectModule || object.Kind == programindex.ObjectVariable && target.moduleLevel(object)) {
				shape.handedValue = true
				shape.constructed = shape.constructed || object.Kind == programindex.ObjectModule
			}
		}
		// A value the repository made: a record it built (new(DNS)) or the
		// result of its own call. What a library returned (an error, a
		// token) is the library's value, not something handed over.
		if len(argument.ObjectIDs) == 0 && argument.Origin != nil {
			switch {
			case argument.Origin.Kind == "record":
				produced, shape.constructed = true, true
			case argument.Origin.Kind == "call_result" && argument.Origin.Anchor != nil && target.producesRepositoryType(*argument.Origin.Anchor):
				produced = true
			}
		}
	}
	// The symbol behind the call: the callee itself, or the type of the
	// value it is called on with the call word (flask.Blueprint.route).
	for _, id := range relation.ToIDs {
		if object, ok := target.object(id); ok && object.Kind == programindex.ObjectExternalSymbol && object.External != nil && object.External.RepositoryPath == "" {
			shape.originKnown = true
			if shape.origin == "" {
				shape.origin = externalSymbolName(*object.External)
			}
		}
	}
	if !shape.originKnown {
		for _, origin := range target.callOrigins(relation, pattern, originsByValue) {
			shape.originKnown = true
			if shape.origin == "" {
				shape.origin = externalSymbolName(origin) + "." + pattern.Selector
			}
		}
	}
	if !shape.originKnown && target.tableRow(relation) {
		shape.origin, shape.originKnown = target.tableRowRegistrar(relation, pattern, shape.handlerID)
	}
	shape.holder, shape.holderPrefixes = target.holderRoot(pattern)
	// A value set on a parameter no repository caller supplies
	// (c.Set("user", model) on the request's context) is state of one call
	// the outside made, not something registered with a holder.
	if receiver := pattern.ReceiverValue; receiver != nil && receiver.Kind == "parameter" && shape.holder == nil {
		produced, shape.handedValue, shape.constructed = false, false, false
	}
	shape.handed = shape.handlerID != "" || relation.Kind == programindex.RelationDecorates || produced || shape.handedValue
	shape.method = statedMethod(pattern, addressed || shape.address != nil)
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

// producesRepositoryType reports a call whose repository callee returns a
// value of a repository type: an instance handed over, not an error or a
// string a helper computed.
func (target *targetContext) producesRepositoryType(at sourcevalue.Anchor) bool {
	for _, id := range target.producerObjects(at) {
		object, ok := target.object(id)
		if !ok {
			continue
		}
		for _, result := range object.Results {
			if result.TypeID != "" {
				return true
			}
		}
	}
	return false
}

// moduleLevel reports a variable declared directly in a module or package.
func (target *targetContext) moduleLevel(object programindex.Object) bool {
	for _, id := range []string{object.ContainerID, object.OwnerID} {
		if owner, ok := target.object(id); ok && (owner.Kind == programindex.ObjectModule || owner.Kind == programindex.ObjectPackage) {
			return true
		}
	}
	return false
}

// holderRoot is the value a call acts on, followed back through the calls
// outside the repository that produced it: a route put into a group made
// from a router is held by the router's value. The receiver gives the
// value; a produced value handed as an argument (ListenAndServe(":8080",
// mux)) gives it when the receiver does not. A value the code did not
// produce by a call has no holder.
func (target *targetContext) holderRoot(pattern programindex.RelationPattern) (*Anchor, []string) {
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
		start, _ = target.parameterValue(pattern.ReceiverValue, make(map[parameterSlot]bool))
	}
	if start == nil {
		return nil, nil
	}
	at := *start
	var prefixes []string
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
					// The call that made this value from its holder names the
					// place the value stands under: Group("/articles").
					if prefix, _, ok := statedPrefix(produced); ok && next != nil {
						prefixes = append([]string{prefix}, prefixes...)
					}
				}
			}
		}
		if next == nil {
			break
		}
		at = *next
	}
	return &Anchor{Path: at.Path, Line: at.Line, Column: at.Column}, prefixes
}

// parameterValue follows a receiver that is a parameter of its function to
// the value its repository callers pass in that position, when every caller
// passes the same one: ArticlesRegister(v1.Group("/articles")) mounts every
// route the function registers on its router parameter. A caller handing on
// a parameter this search has already reached (a function passing its own
// parameter to itself, functions passing it round, two paths meeting) adds
// no value of its own; what reaches that parameter is counted where it was
// first reached. known is false when a caller passes an unknown or a
// different value, or no caller passes one.
func (target *targetContext) parameterValue(value *sourcevalue.Value, reached map[parameterSlot]bool) (passed *sourcevalue.Anchor, known bool) {
	if value == nil || value.Kind != "parameter" || value.Owner == nil || value.Position == 0 {
		return nil, false
	}
	slot := parameterSlot{owner: *value.Owner, position: value.Position}
	if reached[slot] {
		return nil, true
	}
	reached[slot] = true
	var owner string
	for _, object := range target.input.Index.Objects {
		if isCallable(object) && object.Location != nil && object.Location.Path == value.Owner.Path && object.Location.Line == value.Owner.Line {
			owner = object.ID
			break
		}
	}
	if owner == "" {
		return nil, false
	}
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || len(relation.ToIDs) != 1 || relation.ToIDs[0] != owner {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.Position != value.Position {
					continue
				}
				known = true
				anchor := producedAt(argument.Origin)
				if anchor == nil {
					handed, handedKnown := target.parameterValue(argument.Origin, reached)
					if !handedKnown {
						return nil, false
					}
					if anchor = handed; anchor == nil {
						continue
					}
				}
				if passed != nil && *passed != *anchor {
					return nil, false
				}
				passed = anchor
			}
		}
	}
	return passed, known
}

// parameterSlot is one parameter position of the callable declared at owner.
type parameterSlot struct {
	owner    sourcevalue.Anchor
	position int
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
	_, _, ok := methodPattern(value)
	return ok
}

// methodPattern splits a "VERB host/path" pattern (net/http's ServeMux
// syntax) whose method is an HTTP verb as the syntax writes it, in capitals:
// "GET /health", "HEAD status.example/{$}". Other text with a word, a space
// and a slash is prose ("open /dev/null: %s", "failed to read a/b"), not an
// address.
func methodPattern(value string) (method, path string, ok bool) {
	method, path, ok = goServeMuxMethodAndPath(value)
	if !ok || method != strings.ToUpper(method) || !isHTTPVerb(method) {
		return "", "", false
	}
	return method, path, true
}

// resolvesToAddress follows a dynamic argument to the literals it can carry.
func resolvesToAddress(values *routeValueReader, argument programindex.PatternArgument) bool {
	for _, literal := range values.addresses(argument) {
		if isAddressLiteral(literal.text) {
			return true
		}
	}
	return false
}

// statedMethod is the HTTP verb the call states itself: as its word (get,
// post), inside a pattern literal ("GET /health"), or as a literal of its
// own beside the address it qualifies (NewRequest("GET", url),
// Handle("POST", "/items", h), a route record's {Method: "GET", Path:
// "/users"} or C table row {"GET", "/health", health}). A verb-shaped
// literal with no address beside it names what is handed over, as a command
// table's {"get", getCommand} row does, and states no verb.
func statedMethod(pattern programindex.RelationPattern, hasAddress bool) string {
	if isHTTPVerb(pattern.Selector) {
		return strings.ToUpper(pattern.Selector)
	}
	for _, argument := range pattern.Arguments {
		value, _, literal := literalValue(argument)
		if !literal {
			continue
		}
		if hasAddress && isHTTPVerb(value) {
			return strings.ToUpper(value)
		}
		if method, _, ok := methodPattern(value); ok {
			return method
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
	if shape.at != nil {
		anchor = shape.at
	}
	if anchor == nil {
		return
	}
	if len(prefixes) == 0 && len(shape.holderPrefixes) > 0 {
		prefixes = []routePrefix{{path: joinRoutePaths(shape.holderPrefixes)}}
	}
	addresses := []routePrefix{{}}
	if shape.address == nil && len(prefixes) > 0 && shape.handlerID != "" {
		// POST("", handler) on a group: the handler answers on the group's path.
		addresses = addresses[:0]
		for _, prefix := range prefixes {
			addresses = append(addresses, prefix)
		}
	}
	if shape.address != nil {
		addresses = addresses[:0]
		for _, observed := range addressLiterals(values, *shape.address) {
			path := observed.text
			if path == "" {
				continue
			}
			if _, rest, ok := methodPattern(path); ok {
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
	_, ownerID := target.enclosingSymbol(shape.relation.FromID)
	for _, address := range addresses {
		if !b.once(strings.Join([]string{string(KindRegistration), target.target.ID, anchor.String(), strconv.Itoa(anchor.Column), shape.word, shape.keyword, address.path}, "\x00")) {
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
			OwnerID:    ownerID,
			Handed:     shape.constructed && shape.handlerID == "",
			Text:       shape.origin,
			Invocation: shape.invocation,
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

// tableRow reports a row of a table the repository owns: a record that a
// module-level variable's initializer builds, handing over a callable beside a
// string literal ({"get", getCommand} in a command table). The owner decided
// on 2026-09-26 that the literal names the callable as a registration does,
// although the repository owns the record; which kind of input it is, the
// reading stage decides.
func (target *targetContext) tableRow(relation programindex.Relation) bool {
	if relation.Invocation != programindex.InvocationConstruct {
		return false
	}
	table, ok := target.object(relation.FromID)
	if !ok || table.Kind != programindex.ObjectVariable || !target.moduleLevel(table) {
		return false
	}
	for _, pattern := range relation.Patterns {
		named, callable := false, false
		for _, argument := range pattern.Arguments {
			if value, _, literal := literalValue(argument); literal && value != "" {
				named = true
			}
			for _, id := range argument.ObjectIDs {
				if object, ok := target.object(id); ok && argument.Kind == programindex.PatternDynamic && isCallable(object) {
					callable = true
				}
			}
		}
		if named && callable {
			return true
		}
	}
	return false
}

// tableRowRegistrar names what a table row registers its callable with: the
// row's record type, as the file that declares it and its name, and the field
// the callable is stored in, as a row writes it (redis.c.redisCommand.proc).
// Every row of one record type shares it, as every call of one outside
// symbol shares that symbol; which kind of entry it makes, the reading asks.
func (target *targetContext) tableRowRegistrar(relation programindex.Relation, pattern programindex.RelationPattern, handlerID string) (string, bool) {
	if len(relation.ToIDs) != 1 {
		return "", false
	}
	record, ok := target.object(relation.ToIDs[0])
	if !ok || record.Kind != programindex.ObjectType || record.Location == nil {
		return "", false
	}
	name := record.Location.Path + "." + record.Name
	for _, argument := range pattern.Arguments {
		if argument.Keyword == "" || handlerID == "" {
			continue
		}
		if target.callbacks[argument.ID] == handlerID || len(argument.ObjectIDs) == 1 && argument.ObjectIDs[0] == handlerID {
			return name + "." + argument.Keyword, true
		}
	}
	return name, true
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

// joinRoutePaths composes mount prefixes root first.
func joinRoutePaths(prefixes []string) string {
	path := ""
	for _, prefix := range prefixes {
		if path == "" {
			path = prefix
			continue
		}
		path = joinRoutePath(path, prefix)
	}
	return path
}
