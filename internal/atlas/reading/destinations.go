package reading

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// DestinationReader traverses the existing source calls, not inferred arrows.
// Each branch retains its own target intersection and source sites. Cycles stop
// explicitly; repository size never silently removes a caller or an address.
// Which value of an outside call names what it reaches is the model's one
// decision per symbol (lines.APIArgument), given in DestinationChoices; no
// list of packages says it.
type DestinationReader struct {
	places         map[string]atlas.Place
	callers        map[string][]destinationCall
	callSites      map[sourcevalue.Anchor][]destinationCall
	owners         map[sourcevalue.Anchor][]atlas.Place
	ownerLines     map[sourcevalue.Anchor][]atlas.Place
	parameterCalls map[sourcevalue.Anchor][]destinationCall
	// environment is, by the site of a call, the environment variable a
	// configuration read there reads (facts config_read, a code fact).
	environment map[sourcevalue.Anchor]string
	choices     DestinationChoices
	// undecided, when set, hears of each outside call whose result a walk
	// reached and whose symbol has no decided argument (readArguments).
	undecided func(symbol string, at atlas.Place, call atlas.SymbolCall)
	// objects, when set, goes on from a field a walk cannot follow to the
	// outside call with a decided argument that made the object it is a
	// field of (destination_objects.go). talks is, by outside symbol, its
	// talks answer: an object an outside call answered another kind made is
	// no exchange's. exchanges indexes, once, where each call's result is
	// handed (Exchange).
	objects   bool
	talks     map[string]string
	exchanges *exchangeIndex
	// order is the places walked, in graph order.
	order []string
}

// DestinationChoices are what the reading decided that a walk reads.
// Arguments are, by outside symbol (package.Receiver.Name), which value of
// a call to it names what the call reaches. Options are, by the site of a
// call whose words an answer made a command-line option, the option's
// name: what such a call returns is that option's value, "{--name}".
type DestinationChoices struct {
	Arguments map[string]ArgumentChoice
	Options   map[sourcevalue.Anchor]string
	// Talks is, by outside symbol, its talks answer: an object an outside
	// call answered another kind made is no exchange's (Exchange).
	Talks map[string]string
}

// ArgumentChoice is which value of a call names what it reaches: its
// receiver, the argument at a position (from 1), or the one passed by a
// keyword. None says that no value does.
type ArgumentChoice struct {
	Receiver bool
	Position int
	Keyword  string
	None     bool
}

// value is the chosen value at one call: the receiver, the argument at the
// position or passed by the keyword, or, when the call passes it the other
// way, by the parameter name its declared type gives that position.
func (choice ArgumentChoice) value(call atlas.SymbolCall) *sourcevalue.Value {
	switch {
	case choice.None:
		return nil
	case choice.Receiver:
		return call.ReceiverValue
	}
	var names []string
	if call.API != nil {
		names = parameterNames(call.API.Signature)
	}
	position, keyword := choice.Position, choice.Keyword
	if keyword == "" && position > 0 && position <= len(names) {
		keyword = names[position-1]
	}
	if position == 0 && keyword != "" {
		for i, name := range names {
			if name == keyword {
				position = i + 1
			}
		}
	}
	for _, argument := range call.SourceArguments {
		if position > 0 && argument.Position == position && argument.Keyword == "" {
			return argument.Origin
		}
	}
	for _, argument := range call.SourceArguments {
		if keyword != "" && argument.Keyword == keyword {
			return argument.Origin
		}
	}
	return nil
}

// parameterNames are the names a declared type gives its parameters, in
// order ("func(ctx context.Context, url string) error" gives ctx and url),
// or none when it does not name every one.
func parameterNames(signature string) []string {
	open := strings.Index(signature, "(")
	if open < 0 {
		return nil
	}
	var names []string
	named := func(parameter string) bool {
		fields := strings.Fields(parameter)
		if len(fields) < 2 {
			return false
		}
		names = append(names, fields[0])
		return true
	}
	depth, start := 0, open+1
	for i := open; i < len(signature); i++ {
		switch signature[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth--; depth == 0 {
				if strings.TrimSpace(signature[start:i]) != "" && !named(signature[start:i]) {
					return nil
				}
				return names
			}
		case ',':
			if depth == 1 {
				if !named(signature[start:i]) {
					return nil
				}
				start = i + 1
			}
		}
	}
	return nil
}

type destinationCall struct {
	place atlas.Place
	call  atlas.SymbolCall
}

// Branch choices exist only during expression traversal. They are never
// published as source facts or sent to the model.
type destinationPath struct {
	atlas.DestinationUse
	choices map[string]int
	// kind is the talks answer of the call the walk began at: an object
	// made by an outside call answered another kind is not what it reaches.
	kind string
}

// A walk never passes through test code: a test is testing, not the
// program, so a value a test hands the program's code (freqtrade's
// tests/rpc/test_rpc_webhook.py building a Webhook with a URL) is not where
// the program's call goes. Once the tests became freqtrade's own files, its
// webhook calls walked only into them, and the page dropped the calls
// whose every walk ran through a test.
func NewDestinationReader(places []atlas.Place, choices DestinationChoices) *DestinationReader {
	d := &DestinationReader{places: make(map[string]atlas.Place), callers: make(map[string][]destinationCall), callSites: make(map[sourcevalue.Anchor][]destinationCall), owners: make(map[sourcevalue.Anchor][]atlas.Place), ownerLines: make(map[sourcevalue.Anchor][]atlas.Place), parameterCalls: make(map[sourcevalue.Anchor][]destinationCall), environment: make(map[sourcevalue.Anchor]string), choices: choices, talks: choices.Talks}
	tests := make(map[string]bool)
	for _, place := range places {
		if place.Kind == atlas.PlaceFile && place.File != nil && place.File.Test {
			tests[place.ID] = true
		}
	}
	for _, place := range places {
		if b := place.Boundary; b != nil && b.Source == "fact" && b.GivenKind == atlas.BoundaryConfig && len(b.Values) > 0 {
			d.environment[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Column}] = b.Values[0]
		}
		if place.Symbol == nil || tests[place.Parent] {
			continue
		}
		d.places[place.ID] = place
		d.order = append(d.order, place.ID)
		owner := sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}
		d.owners[owner] = append(d.owners[owner], place)
		owner.Column = 0
		d.ownerLines[owner] = append(d.ownerLines[owner], place)
		for _, call := range place.Symbol.Calls {
			item := destinationCall{place, call}
			anchor := sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}
			d.callSites[anchor] = append(d.callSites[anchor], item)
			if call.ResultValue != nil && call.ResultValue.Owner != nil {
				d.parameterCalls[*call.ResultValue.Owner] = append(d.parameterCalls[*call.ResultValue.Owner], item)
			}
			for _, id := range call.CalleeIDs {
				d.callers[id] = append(d.callers[id], item)
			}
		}
	}
	return d
}

// Read follows the value a call's symbol's decided argument names back to
// where it comes from. A call to a symbol without a decided argument, or
// whose decided argument it was not given, stops at its own name.
func (d *DestinationReader) Read(place atlas.Place, call atlas.SymbolCall) []atlas.DestinationUse {
	step := destinationStep(place, call)
	initial := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: append([]string(nil), runningTargets(place)...), Steps: []atlas.DestinationStep{step}}}
	if call.API != nil {
		initial.kind = d.talks[apiName(*call.API)]
	}
	if value := d.chosen(call); value != nil {
		return publishDestinationPaths(d.value(value, place, initial, make(map[string]bool)))
	}
	initial.Frontier = call.Name
	return []atlas.DestinationUse{initial.DestinationUse}
}

// chosen is the value the decided argument of an outside call names, nil
// for a call to no outside symbol, to one without a decided argument or
// decided none, or one not given it.
func (d *DestinationReader) chosen(call atlas.SymbolCall) *sourcevalue.Value {
	if call.API == nil {
		return nil
	}
	choice, ok := d.choices.Arguments[apiName(*call.API)]
	if !ok {
		return nil
	}
	return choice.value(call)
}

// environmentAt is the environment variable a configuration read at a call
// reads: at its column, or on its line when the fact has no column.
func (d *DestinationReader) environmentAt(anchor sourcevalue.Anchor) string {
	if key := d.environment[anchor]; key != "" {
		return key
	}
	anchor.Column = 0
	return d.environment[anchor]
}

func sourceArgument(call atlas.SymbolCall, position int) *sourcevalue.Value {
	for _, argument := range call.SourceArguments {
		if argument.Position == position {
			return argument.Origin
		}
	}
	return nil
}

func namedSourceArgument(call atlas.SymbolCall, position int, name string) *sourcevalue.Value {
	if value := sourceArgument(call, position); value != nil {
		return value
	}
	for _, argument := range call.SourceArguments {
		if argument.Keyword == name {
			return argument.Origin
		}
	}
	return nil
}

func (d *DestinationReader) value(value *sourcevalue.Value, owner atlas.Place, use destinationPath, active map[string]bool) []destinationPath {
	if value == nil {
		use.Frontier = "unresolved argument"
		return []destinationPath{use}
	}
	raw, _ := json.Marshal(value)
	key := owner.ID + string(raw)
	if active[key] {
		use.Frontier = "cyclic value"
		return []destinationPath{use}
	}
	active[key] = true
	defer delete(active, key)
	switch value.Kind {
	case "literal":
		use.Address = value.Text
		return []destinationPath{use}
	case "parameter":
		var result []destinationPath
		for _, caller := range d.parameterCallers(value, use) {
			next := cloneDestinationPath(use)
			next.TargetIDs = intersectTargets(use.TargetIDs, runningTargets(caller.place))
			if len(next.TargetIDs) == 0 {
				continue
			}
			next.Steps = appendDestinationStep(next.Steps, destinationStep(caller.place, caller.call))
			result = append(result, d.value(namedSourceArgument(caller.call, value.Position, value.Text), caller.place, next, active)...)
		}
		if len(result) > 0 {
			return result
		}
		use.Frontier = value.Text
		if use.Frontier == "" {
			use.Frontier = fmt.Sprintf("parameter:%d", value.Position)
		}
	case "call_result":
		var result []destinationPath
		if value.Anchor != nil {
			for _, call := range d.callSites[*value.Anchor] {
				next := cloneDestinationPath(use)
				next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
				at := sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column}
				// What a command-line option's declaration or an
				// environment read returns is that setting's value: the
				// configuration names the address, no deployed value.
				if name := d.choices.Options[at]; name != "" {
					next.Address = "{--" + strings.TrimLeft(name, "-") + "}"
					result = append(result, next)
					continue
				}
				if key := d.environmentAt(at); key != "" {
					next.Address = "{env:" + key + "}"
					result = append(result, next)
					continue
				}
				if call.call.API != nil {
					if chosen := d.chosen(call.call); chosen != nil {
						result = append(result, d.value(chosen, call.place, next, active)...)
						continue
					}
					if _, decided := d.choices.Arguments[apiName(*call.call.API)]; !decided && d.undecided != nil {
						d.undecided(apiName(*call.call.API), call.place, call.call)
					}
					next.Frontier = call.call.Name + "()"
					result = append(result, next)
					continue
				}
				if call.call.ResultValue != nil && len(call.call.CalleeIDs) == 1 {
					if callee, ok := d.places[call.call.CalleeIDs[0]]; ok {
						result = append(result, d.value(call.call.ResultValue, callee, next, active)...)
					} else {
						next.Frontier = call.call.Name + "()"
						result = append(result, next)
					}
				} else {
					next.Frontier = call.call.Name + "()"
					result = append(result, next)
				}
			}
		}
		if len(result) > 0 {
			return result
		}
		use.Frontier = value.Text + "()"
	case "concat":
		results := []destinationPath{use}
		for _, part := range value.Parts {
			var combined []destinationPath
			for _, prefix := range results {
				seed := cloneDestinationPath(prefix)
				seed.Address = ""
				seed.Frontier = ""
				for _, suffix := range d.value(&part, owner, seed, active) {
					// A template of known and unread parts is still read.
					suffix.Unread = false
					if prefix.Frontier == "" && suffix.Frontier == "" {
						suffix.Address = prefix.Address + suffix.Address
					} else {
						left, right := prefix.Address, suffix.Address
						if prefix.Frontier != "" {
							left = prefix.Frontier
						}
						if suffix.Frontier != "" {
							right = "{" + templatePart(&part, suffix.Frontier) + "}"
						}
						suffix.Frontier = left + right
						suffix.Address = ""
					}
					combined = append(combined, suffix)
				}
			}
			results = combined
		}
		return results
	case "alternatives":
		var result []destinationPath
		for i, part := range value.Parts {
			if branch, ok := chooseDestinationPart(value, i, owner, use); ok {
				result = append(result, d.value(&part, owner, branch, active)...)
			}
		}
		return result
	case "field":
		result := d.field(&value.Parts[0], value.Text, owner, use, active)
		if value.Initializer != nil && len(result) == 1 && result[0].Address == "" {
			result = d.value(storedValue(value.Initializer), owner, use, active)
			for i := range result {
				if result[i].Address != "" {
					result[i].Frontier = "initializer: " + result[i].Address
					result[i].Address = ""
				}
			}
		}
		return result
	case "record":
		// A request record's native URL field is the supplied destination.
		for _, field := range value.Parts {
			if field.Text == "URL" || field.Text == "url" {
				return d.value(&field.Parts[0], owner, use, active)
			}
		}
		use.Frontier = "request value"
	case "index":
		if value.Parts[1].Kind == "literal" {
			return d.field(&value.Parts[0], value.Parts[1].Text, owner, use, active)
		}
		use.Frontier = sourceValueExpression(value)
	default:
		use.Frontier = value.Text
		if use.Frontier == "" {
			use.Frontier = "computed value"
		}
		// A value the adapter could not read ends the walk with no address
		// established from code; its text is only the expression written
		// there (Redis's connect on `(struct sockaddr*)&sa`).
		use.Unread = value.Kind == "unknown"
	}
	if value.Anchor != nil {
		use.Steps = appendDestinationStep(use.Steps, atlas.DestinationStep{SubjectID: owner.ID, Name: use.Frontier, Path: value.Anchor.Path, Line: value.Anchor.Line, Column: value.Anchor.Column})
	}
	return []destinationPath{use}
}

// storedValue is what a field initializer stores: the value of its one
// store, or, when the field's class and the classes deriving from it each
// store it (a Python base-class method reading self._url), the value of each
// store as one alternative.
func storedValue(initial *sourcevalue.Value) *sourcevalue.Value {
	switch initial.Kind {
	case "field_value":
		return &initial.Parts[0]
	case "alternatives":
		stored := *initial
		stored.Parts = make([]sourcevalue.Value, len(initial.Parts))
		for i := range initial.Parts {
			stored.Parts[i] = *storedValue(&initial.Parts[i])
		}
		return &stored
	}
	return initial
}

func (d *DestinationReader) parameterCallers(value *sourcevalue.Value, use destinationPath) []destinationCall {
	owners := d.valueOwners(value.Owner)
	var ownerIDs []string
	for _, parameterOwner := range owners {
		ownerIDs = append(ownerIDs, parameterOwner.ID)
	}
	if chosen := d.chosenCallers(use, ownerIDs, value.Owner); len(chosen) > 0 {
		var selected []destinationCall
		for anchor := range chosen {
			for _, call := range d.callSites[anchor] {
				if matchesDestinationOwner(call.call, ownerIDs, value.Owner) {
					selected = append(selected, call)
				}
			}
		}
		return bindingCalls(selected)
	}
	var callers []destinationCall
	for _, id := range ownerIDs {
		callers = append(callers, d.callers[id]...)
	}
	// A construction binds its constructor's formals by its result's owner.
	if value.Owner != nil {
		callers = append(callers, d.parameterCalls[*value.Owner]...)
	}
	return bindingCalls(callers)
}

// bindingCalls keeps one caller per source site. Two calls written at one
// site are one call: a Python construction is the class's call, with its
// arguments, and the call of the __init__ it runs, recorded without them.
// Where one of them records arguments, the other passes nothing of its own.
func bindingCalls(callers []destinationCall) []destinationCall {
	type site struct {
		place        string
		line, column int
	}
	withArguments := make(map[site]bool)
	for _, caller := range callers {
		if len(caller.call.SourceArguments) > 0 {
			withArguments[site{caller.place.ID, caller.call.Line, caller.call.Column}] = true
		}
	}
	result := make([]destinationCall, 0, len(callers))
	for _, caller := range callers {
		if len(caller.call.SourceArguments) == 0 && withArguments[site{caller.place.ID, caller.call.Line, caller.call.Column}] {
			continue
		}
		result = append(result, caller)
	}
	return result
}

func (d *DestinationReader) valueOwners(owner *sourcevalue.Anchor) []atlas.Place {
	if owner == nil {
		return nil
	}
	if exact := d.owners[*owner]; len(exact) > 0 {
		return exact
	}
	line := *owner
	line.Column = 0
	if candidates := d.ownerLines[line]; len(candidates) == 1 {
		return candidates
	}
	return nil
}

func matchesDestinationOwner(call atlas.SymbolCall, callees []string, formal *sourcevalue.Anchor) bool {
	for _, callee := range callees {
		if contains(call.CalleeIDs, callee) {
			return true
		}
	}
	return formal != nil && call.ResultValue != nil && call.ResultValue.Owner != nil && *call.ResultValue.Owner == *formal
}

// Two parameters of the same invocation must use that same caller row.
// Independent expansion would invent cross-products of URL bases and suffixes
// that were never passed together by any source call.
func (d *DestinationReader) chosenCallers(use destinationPath, callees []string, formal *sourcevalue.Anchor) map[sourcevalue.Anchor]bool {
	chosen := make(map[sourcevalue.Anchor]bool)
	for _, step := range use.Steps {
		anchor := sourcevalue.Anchor{Path: step.Path, Line: step.Line, Column: step.Column}
		for _, call := range d.callSites[anchor] {
			if matchesDestinationOwner(call.call, callees, formal) {
				chosen[anchor] = true
			}
		}
	}
	return chosen
}

// templatePart is how a template writes a part whose walk established no
// address: the value as the code wrote it there when it names one (a
// field, a parameter, a receiver, an element), else where its walk ended.
// litestream's WAL file is db.path + "-wal": "{db.path}-wal", however deep
// the walk of db.path went, and no longer "-wal" alone.
func templatePart(part *sourcevalue.Value, frontier string) string {
	switch part.Kind {
	case "field", "parameter", "receiver", "index":
		if expression := sourceValueExpression(part); expression != "" && !strings.Contains(expression, "?") {
			return expression
		}
	}
	return frontier
}

func sourceValueExpression(value *sourcevalue.Value) string {
	switch value.Kind {
	case "literal":
		return fmt.Sprintf("%q", value.Text)
	case "field":
		return sourceValueExpression(&value.Parts[0]) + "." + value.Text
	case "index":
		return sourceValueExpression(&value.Parts[0]) + "[" + sourceValueExpression(&value.Parts[1]) + "]"
	case "call_result":
		return value.Text + "()"
	}
	if value.Text != "" {
		return value.Text
	}
	return "?"
}

func (d *DestinationReader) field(receiver *sourcevalue.Value, name string, owner atlas.Place, use destinationPath, active map[string]bool) []destinationPath {
	raw, _ := json.Marshal(receiver)
	key := "field:" + owner.ID + ":" + name + string(raw)
	if active[key] {
		use.Frontier = "cyclic field " + name
		return []destinationPath{use}
	}
	active[key] = true
	defer delete(active, key)
	switch receiver.Kind {
	case "record":
		for _, field := range receiver.Parts {
			if field.Text == name {
				if field.Anchor != nil {
					use.Steps = appendDestinationStep(use.Steps, atlas.DestinationStep{SubjectID: owner.ID, Name: field.Text, Path: field.Anchor.Path, Line: field.Anchor.Line, Column: field.Anchor.Column})
				}
				return d.value(&field.Parts[0], owner, use, active)
			}
		}
	case "receiver":
		var result []destinationPath
		owners := d.valueOwners(receiver.Owner)
		if len(owners) == 1 {
			chosen := d.chosenCallers(use, []string{owners[0].ID}, receiver.Owner)
			// A call resolved to alternatives is made on an object of one of
			// several classes (an element of RPCManager's registered
			// handlers): which object's field it is, it cannot say.
			var callers []destinationCall
			for _, call := range d.callers[owners[0].ID] {
				if call.call.Resolution != "alternatives" {
					callers = append(callers, call)
				}
			}
			if len(chosen) > 0 {
				callers = nil
				for anchor := range chosen {
					for _, call := range d.callSites[anchor] {
						if matchesDestinationOwner(call.call, []string{owners[0].ID}, receiver.Owner) {
							callers = append(callers, call)
						}
					}
				}
			}
			for _, call := range callers {
				if call.call.ReceiverValue == nil {
					continue
				}
				next := cloneDestinationPath(use)
				next.TargetIDs = intersectTargets(use.TargetIDs, runningTargets(call.place))
				if len(next.TargetIDs) == 0 {
					continue
				}
				next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
				result = append(result, d.field(call.call.ReceiverValue, name, call.place, next, active)...)
			}
		}
		if len(result) > 0 {
			return result
		}
	case "parameter":
		var result []destinationPath
		for _, call := range d.parameterCallers(receiver, use) {
			argument := namedSourceArgument(call.call, receiver.Position, receiver.Text)
			if argument == nil {
				continue
			}
			next := cloneDestinationPath(use)
			next.TargetIDs = intersectTargets(use.TargetIDs, runningTargets(call.place))
			if len(next.TargetIDs) == 0 {
				continue
			}
			next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
			result = append(result, d.field(argument, name, call.place, next, active)...)
		}
		if len(result) > 0 {
			return result
		}
	case "call_result":
		var result []destinationPath
		if receiver.Anchor != nil {
			for _, call := range d.callSites[*receiver.Anchor] {
				if call.call.ResultValue == nil {
					continue
				}
				next := cloneDestinationPath(use)
				next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
				callee := call.place
				if len(call.call.CalleeIDs) == 1 {
					if place, ok := d.places[call.call.CalleeIDs[0]]; ok {
						callee = place
					}
				}
				result = append(result, d.field(call.call.ResultValue, name, callee, next, active)...)
			}
		}
		if len(result) > 0 {
			return result
		}
	case "alternatives":
		var result []destinationPath
		for i, part := range receiver.Parts {
			if branch, ok := chooseDestinationPart(receiver, i, owner, use); ok {
				result = append(result, d.field(&part, name, owner, branch, active)...)
			}
		}
		return result
	}
	// A field of an object the walk cannot follow is part of that object:
	// what the outside call with a decided argument that made the object
	// reaches, when one did (Trade.session.bind, of the session made from
	// create_engine(db_url)). An object no decided call made gives nothing:
	// two values read from one configuration stay apart.
	if d.objects {
		if found := d.object(receiver, owner, use, false, active); len(found) > 0 {
			return found
		}
	}
	use.Frontier = sourceValueExpression(receiver) + "." + name
	if receiver.Anchor != nil {
		use.Steps = appendDestinationStep(use.Steps, atlas.DestinationStep{SubjectID: owner.ID, Name: use.Frontier, Path: receiver.Anchor.Path, Line: receiver.Anchor.Line, Column: receiver.Anchor.Column})
	}
	return []destinationPath{use}
}

func destinationStep(place atlas.Place, call atlas.SymbolCall) atlas.DestinationStep {
	return atlas.DestinationStep{SubjectID: place.ID, Name: place.Symbol.Decl.Name, Path: place.Path, Line: call.Line, Column: call.Column}
}

func intersectTargets(a, b []string) []string {
	var result []string
	for _, id := range a {
		if contains(b, id) {
			result = append(result, id)
		}
	}
	return result
}

func appendDestinationStep(steps []atlas.DestinationStep, step atlas.DestinationStep) []atlas.DestinationStep {
	for _, prior := range steps {
		if prior == step {
			return steps
		}
	}
	return append(steps, step)
}

func cloneDestinationPath(use destinationPath) destinationPath {
	choices := make(map[string]int, len(use.choices))
	for key, choice := range use.choices {
		choices[key] = choice
	}
	use.choices = choices
	use.TargetIDs = append([]string(nil), use.TargetIDs...)
	use.Steps = append([]atlas.DestinationStep(nil), use.Steps...)
	return use
}

func cloneDestinationUse(use atlas.DestinationUse) atlas.DestinationUse {
	use.TargetIDs = append([]string(nil), use.TargetIDs...)
	use.Steps = append([]atlas.DestinationStep(nil), use.Steps...)
	return use
}

func chooseDestinationPart(value *sourcevalue.Value, part int, owner atlas.Place, use destinationPath) (destinationPath, bool) {
	branch := cloneDestinationPath(use)
	if value.Anchor == nil {
		return branch, true
	}
	encoded, _ := json.Marshal(value)
	key := owner.ID + string(encoded)
	if chosen, ok := branch.choices[key]; ok && chosen != part {
		return branch, false
	}
	branch.choices[key] = part
	return branch, true
}

func publishDestinationPaths(paths []destinationPath) []atlas.DestinationUse {
	uses := make([]atlas.DestinationUse, 0, len(paths))
	for _, path := range paths {
		uses = append(uses, path.DestinationUse)
	}
	return canonicalDestinationUses(uses)
}

func canonicalDestinationUses(uses []atlas.DestinationUse) []atlas.DestinationUse {
	byKey := make(map[string]atlas.DestinationUse)
	for _, use := range uses {
		sort.Strings(use.TargetIDs)
		raw, _ := json.Marshal(use)
		byKey[string(raw)] = use
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]atlas.DestinationUse, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func destinationEvidence(uses []atlas.DestinationUse) []map[string]any {
	var result []map[string]any
	for _, use := range uses {
		var steps []map[string]any
		for _, step := range use.Steps {
			steps = append(steps, map[string]any{"name": step.Name, "path": step.Path, "line": step.Line})
		}
		result = append(result, map[string]any{"address": use.Address, "unresolved_expression": use.Frontier, "method": use.Method, "source_chain": steps})
	}
	return result
}

func destinationAddresses(uses []atlas.DestinationUse) []lines.BoundaryAddress {
	var result []lines.BoundaryAddress
	seen := make(map[string]bool)
	for _, use := range uses {
		if use.Address == "" || seen[use.Address] {
			continue
		}
		seen[use.Address] = true
		result = append(result, lines.BoundaryAddress{Ref: fmt.Sprintf("a%d", len(result)+1), Value: use.Address})
	}
	return result
}
