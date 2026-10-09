package reading

import (
	"encoding/json"
	"fmt"
	"slices"
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
	// environment is, by the site of a call, the environment variables a
	// configuration read there reads (facts config_read, a code fact): one
	// written key, or each key a wrapper's callers hand it.
	environment map[sourcevalue.Anchor][]string
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
	// order is the places walked, in graph order; byPath indexes them by
	// file, built on first use (ownerAt).
	order  []string
	byPath map[string][]string
	// inWriteValue counts the write values being walked (writeValues): a
	// field met there is named by its write sites alone.
	inWriteValue int
	// fieldWrites are the graph's field writes by field key; writesMemo
	// what reading them gave, by field and walk, each under the bindings
	// and choices the reading consulted (pushConsults); frames are the
	// readings in progress, each recording what it consults.
	fieldWrites map[string][]atlas.FieldWrite
	writesMemo  map[string][]writesReading
	frames      []*consults
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
	// FieldWrites are the graph's field writes (atlas.Graph.FieldWrites):
	// what a field read's field_writes reference names.
	FieldWrites []atlas.FieldWrites
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
	d := &DestinationReader{places: make(map[string]atlas.Place), callers: make(map[string][]destinationCall), callSites: make(map[sourcevalue.Anchor][]destinationCall), owners: make(map[sourcevalue.Anchor][]atlas.Place), ownerLines: make(map[sourcevalue.Anchor][]atlas.Place), parameterCalls: make(map[sourcevalue.Anchor][]destinationCall), environment: make(map[sourcevalue.Anchor][]string), choices: choices, talks: choices.Talks}
	d.fieldWrites = make(map[string][]atlas.FieldWrite, len(choices.FieldWrites))
	for _, writes := range choices.FieldWrites {
		d.fieldWrites[writes.Field] = writes.Writes
	}
	tests := make(map[string]bool)
	for _, place := range places {
		if place.Kind == atlas.PlaceFile && place.File != nil && place.File.Test {
			tests[place.ID] = true
		}
	}
	for _, place := range places {
		if b := place.Boundary; b != nil && b.Source == "fact" && b.GivenKind == atlas.BoundaryConfig && len(b.Values) > 0 {
			at := sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Column}
			if !slices.Contains(d.environment[at], b.Values[0]) {
				d.environment[at] = append(d.environment[at], b.Values[0])
			}
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
	initial := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: append([]string(nil), runningCallTargets(place, call)...), Steps: []atlas.DestinationStep{step}}}
	if call.API != nil {
		initial.kind = d.talks[apiName(*call.API)]
	}
	if value := d.chosen(call); value != nil {
		return publishDestinationPaths(d.value(value, place, initial, make(map[string]bool)))
	}
	initial.Frontier = call.Name
	return []atlas.DestinationUse{initial.DestinationUse}
}

// discover walks what Read walks, for its undecided hearing alone: the
// paths it reaches are neither published nor ordered, which only the
// boundaries and files that read them need.
func (d *DestinationReader) discover(place atlas.Place, call atlas.SymbolCall) {
	if value := d.chosen(call); value != nil {
		initial := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: append([]string(nil), runningCallTargets(place, call)...), Steps: []atlas.DestinationStep{destinationStep(place, call)}}}
		d.value(value, place, initial, make(map[string]bool))
	}
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

// environmentAt is the environment variables a configuration read at a
// call reads: at its column, or on its line when the fact has no column.
func (d *DestinationReader) environmentAt(anchor sourcevalue.Anchor) []string {
	if keys := d.environment[anchor]; len(keys) > 0 {
		return keys
	}
	anchor.Column = 0
	return d.environment[anchor]
}

// settingEnds reads the key an environment read is handed: the walk of its
// decided argument, bound to the callers this walk came through, ends at
// the key its caller names (casdoor's GetConfigString(key) reading
// os.LookupEnv(key)), and an end that is one of the keys facts record read
// there is that setting. Any other end stays as the walk left it. False
// when no end is a key read there: the read's one written key, if it has
// one, is the setting.
func (d *DestinationReader) settingEnds(call destinationCall, use destinationPath, keys []string, active map[string]bool) ([]destinationPath, bool) {
	chosen := d.chosen(call.call)
	if chosen == nil {
		return nil, false
	}
	ends := d.value(chosen, call.place, use, active)
	named := false
	for i := range ends {
		if ends[i].Frontier == "" && slices.Contains(keys, ends[i].Address) {
			ends[i].Address = "{env:" + ends[i].Address + "}"
			named = true
		}
	}
	return ends, named
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
		d.consultCut()
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
			next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(caller.place, caller.call))
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
				next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(call.place, call.call))
				if len(next.TargetIDs) == 0 {
					continue
				}
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
				if keys := d.environmentAt(at); len(keys) > 0 {
					if ends, ok := d.settingEnds(call, next, keys, active); ok {
						result = append(result, ends...)
						continue
					}
					// The read's one key is its setting only when the call
					// writes that key: a key handed to it that the walk
					// cannot read is no neighbour's key (get("KNOWN") beside
					// get(dynamicKey)).
					if len(keys) == 1 && d.writesKey(call.call, keys[0]) {
						next.Address = "{env:" + keys[0] + "}"
						result = append(result, next)
						continue
					}
				}
				if call.call.API != nil {
					if chosen := d.chosen(call.call); chosen != nil {
						result = append(result, d.value(chosen, call.place, next, active)...)
						continue
					}
					if _, decided := d.choices.Arguments[apiName(*call.call.API)]; !decided && d.undecided != nil {
						// A reading that heard this is not reused: a later
						// consumer must hear it too.
						d.consultCut()
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
			if d.outsideItsBranch(&part, owner, use) {
				continue
			}
			if branch, ok := d.chooseDestinationPart(value, i, owner, d.entryOf(use, owner), use); ok {
				result = append(result, d.value(&part, owner, branch, active)...)
			}
		}
		return result
	case "field":
		result := d.field(&value.Parts[0], value.Text, owner, use, active, nil)
		if value.Initializer != nil && !anyAddress(result) {
			// The instance the walk could not follow may hold what any
			// write of the field put there. Each unresolved instance walk
			// stays. A field whose writes the graph keeps (field_writes):
			// from each write the written value is walked as any value
			// (parameters to callers, literals, setting reads), and a
			// field that walk reaches again is named by its write sites and
			// values, followed no further (owner, 2026-10-04: one hop, as a
			// reader follows where a field is set; field-to-field chains
			// are what multiplied). A field initializer kept inline is the
			// instance's own class's stores, one alternative per class it
			// may be (Python's __init__ stores): it is read on in place of
			// the unresolved instance, which stays only where it gives
			// nothing. An address it reaches is a possible source, not the
			// call's established value.
			var stored, kept []destinationPath
			for _, base := range result {
				from := cloneDestinationPath(base)
				from.Frontier, from.Unread = "", false
				if value.Initializer.Kind == "field_writes" {
					if d.inWriteValue > 0 {
						stored = append(stored, d.writeSites(value.Initializer.Text, from)...)
					} else {
						stored = append(stored, d.writeValues(value.Initializer.Text, from, active)...)
					}
					continue
				}
				found := d.initializer(value.Initializer, owner, from, active, writesWalk{})
				if len(found) == 0 {
					kept = append(kept, base)
				}
				stored = append(stored, found...)
			}
			for i := range stored {
				if stored[i].Address != "" {
					stored[i].Frontier = "initializer: " + stored[i].Address
					stored[i].Address = ""
				}
			}
			if value.Initializer.Kind != "field_writes" {
				result = kept
			}
			result = append(result, stored...)
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
			return d.field(&value.Parts[0], value.Parts[1].Text, owner, use, active, nil)
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

// initializer reads on from what a field's writes stored (a field
// initializer): each field_value is read from its write, in the function
// that makes it, with a step at the write, and alternatives are each one;
// leaf, when set, reads it as the object walk does.
// writesWalk is how a field's writes are read on: by the value walk (the
// zero value), or by the object walk, with its itself setting.
type writesWalk struct {
	object, itself bool
}

func (walk writesWalk) leaf(d *DestinationReader, active map[string]bool) fieldLeaf {
	if !walk.object {
		return nil
	}
	return func(field *sourcevalue.Value, at atlas.Place, from destinationPath) []destinationPath {
		return d.object(field, at, from, walk.itself, active)
	}
}

func (d *DestinationReader) initializer(initial *sourcevalue.Value, owner atlas.Place, use destinationPath, active map[string]bool, walk writesWalk) []destinationPath {
	switch initial.Kind {
	case "field_writes":
		// A reference to the field's writes, kept once in the graph: each
		// write its own targets' and read on with this walk's bindings, so
		// a caller the walk passed still binds the write's parameters. A
		// field this walk is already reading is not read again (a cycle).
		// What the writes give is read once for every walk with the same
		// bindings where the reading consulted them (writesMemo).
		key := "writes:" + initial.Text
		if active[key] {
			d.consultCut()
			return nil
		}
		memoKey := fmt.Sprintf("%s\x00%s\x00%t\x00%t", initial.Text, use.kind, walk.object, walk.itself)
		if paths, ok := d.reuseWrites(memoKey, use); ok {
			return paths
		}
		active[key] = true
		frame := d.pushConsults()
		var result []destinationPath
		for i, write := range d.fieldWrites[initial.Text] {
			path := cloneDestinationPath(use)
			path.TargetIDs = intersectTargets(use.TargetIDs, write.TargetIDs)
			if len(path.TargetIDs) == 0 {
				continue
			}
			result = append(result, d.initializer(&d.fieldWrites[initial.Text][i].Value, owner, path, active, walk)...)
		}
		delete(active, key)
		d.popConsults(frame, memoKey, use, result)
		return result
	case "field_value":
		if len(initial.Parts) == 0 {
			return nil
		}
		writer := owner
		next := cloneDestinationPath(use)
		if initial.Anchor != nil {
			if place, ok := d.ownerAt(*initial.Anchor); ok {
				writer = place
			}
			next.Steps = appendDestinationStep(next.Steps, atlas.DestinationStep{SubjectID: writer.ID, Name: initial.Text, Path: initial.Anchor.Path, Line: initial.Anchor.Line, Column: initial.Anchor.Column})
		}
		if leaf := walk.leaf(d, active); leaf != nil {
			return leaf(&initial.Parts[0], writer, next)
		}
		return d.value(&initial.Parts[0], writer, next, active)
	case "alternatives":
		var result []destinationPath
		for i := range initial.Parts {
			result = append(result, d.initializer(&initial.Parts[i], owner, cloneDestinationPath(use), active, walk)...)
		}
		return result
	}
	if leaf := walk.leaf(d, active); leaf != nil {
		return leaf(initial, owner, use)
	}
	return d.value(initial, owner, use, active)
}

// writeValues walk the value each write of a field stored, from the write
// in the function that makes it, each its own targets', with a step at its
// site: a field that walk reaches is named by its write sites alone
// (writeSites).
func (d *DestinationReader) writeValues(field string, use destinationPath, active map[string]bool) []destinationPath {
	// What the writes' values give is the same for every walk binding the
	// functions the walk consulted alike (writesMemo): read once for them.
	memoKey := "values\x00" + field + "\x00" + use.kind
	if paths, ok := d.reuseWrites(memoKey, use); ok {
		return paths
	}
	d.inWriteValue++
	frame := d.pushConsults()
	defer func() { d.inWriteValue-- }()
	var result []destinationPath
	for i, write := range d.fieldWrites[field] {
		path := cloneDestinationPath(use)
		path.TargetIDs = intersectTargets(use.TargetIDs, write.TargetIDs)
		if len(path.TargetIDs) == 0 || write.Value.Kind != "field_value" || len(write.Value.Parts) == 0 {
			continue
		}
		writer := atlas.Place{}
		if anchor := write.Value.Anchor; anchor != nil {
			if place, ok := d.ownerAt(*anchor); ok {
				writer = place
			}
			path.Steps = appendDestinationStep(path.Steps, atlas.DestinationStep{SubjectID: writer.ID, Name: write.Value.Text, Path: anchor.Path, Line: anchor.Line, Column: anchor.Column})
		}
		// A write made by a function this walk passed through (deliver
		// writing c.URL = u before its own request) is read on, so its
		// caller binds what it wrote; a write made anywhere else is named
		// by its site and value (writeSites): a reader follows a field's
		// write along the path at hand, and lists the others.
		if !onPath(path, writer.ID) {
			result = append(result, d.siteOf(&d.fieldWrites[field][i], path)...)
			continue
		}
		result = append(result, d.value(&d.fieldWrites[field][i].Value.Parts[0], writer, path, active)...)
	}
	d.popConsults(frame, memoKey, use, result)
	return result
}

// onPath says a walk passed through a declaration.
func onPath(use destinationPath, id string) bool {
	if id == "" {
		return false
	}
	for _, step := range use.Steps[:max(0, len(use.Steps)-1)] {
		if step.SubjectID == id {
			return true
		}
	}
	return false
}

// siteOf is one write as a possible origin, its step already on path: its
// value as written, read no further.
func (d *DestinationReader) siteOf(write *atlas.FieldWrite, path destinationPath) []destinationPath {
	stored := &write.Value.Parts[0]
	switch stored.Kind {
	case "literal":
		path.Address = stored.Text
	case "unknown":
		path.Frontier = stored.Text
		path.Unread = true
	default:
		path.Frontier = "initializer: " + sourceValueExpression(stored)
	}
	return []destinationPath{path}
}

// writeSites are a field's writes as possible origins of a value walk that
// reached the field: each write its own targets', a step at its site, and
// its value as written (a literal's text, any other value's expression),
// never read further.
func (d *DestinationReader) writeSites(field string, use destinationPath) []destinationPath {
	var result []destinationPath
	for _, write := range d.fieldWrites[field] {
		path := cloneDestinationPath(use)
		path.TargetIDs = intersectTargets(use.TargetIDs, write.TargetIDs)
		if len(path.TargetIDs) == 0 || write.Value.Kind != "field_value" || len(write.Value.Parts) == 0 {
			continue
		}
		stored := &write.Value.Parts[0]
		if anchor := write.Value.Anchor; anchor != nil {
			writer := atlas.Place{}
			if place, ok := d.ownerAt(*anchor); ok {
				writer = place
			}
			path.Steps = appendDestinationStep(path.Steps, atlas.DestinationStep{SubjectID: writer.ID, Name: write.Value.Text, Path: anchor.Path, Line: anchor.Line, Column: anchor.Column})
		}
		switch stored.Kind {
		case "literal":
			path.Address = stored.Text
		case "unknown":
			path.Frontier = stored.Text
			path.Unread = true
		default:
			path.Frontier = "initializer: " + sourceValueExpression(stored)
		}
		result = append(result, path)
	}
	return result
}

// ownerAt is the innermost declaration of the walk's places whose lines
// hold a site.
func (d *DestinationReader) ownerAt(at sourcevalue.Anchor) (atlas.Place, bool) {
	if d.byPath == nil {
		d.byPath = make(map[string][]string)
		for _, id := range d.order {
			place := d.places[id]
			d.byPath[place.Path] = append(d.byPath[place.Path], id)
		}
	}
	var found atlas.Place
	ok := false
	for _, id := range d.byPath[at.Path] {
		place := d.places[id]
		end := place.Symbol.Decl.EndLine
		if end < place.LineNo {
			end = place.LineNo
		}
		if place.LineNo <= at.Line && at.Line <= end && (!ok || place.LineNo > found.LineNo) {
			found, ok = place, true
		}
	}
	return found, ok
}

// writesKey says a call writes a key as its key argument: the argument
// decided to name what it reads, else its first, a literal of that key. A
// call whose arguments carry no value is read by its one literal. Any other
// literal (a default, os.getenv(key, "KNOWN")) proves no key.
func (d *DestinationReader) writesKey(call atlas.SymbolCall, key string) bool {
	argument := d.chosen(call)
	if argument == nil {
		argument = sourceArgument(call, 1)
	}
	if argument != nil {
		return argument.Kind == "literal" && argument.Text == key
	}
	return len(call.SourceArguments) == 0 && len(call.Values) == 1 && call.Values[0] == key
}

func anyAddress(paths []destinationPath) bool {
	for _, path := range paths {
		if path.Address != "" {
			return true
		}
	}
	return false
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
// that were never passed together by any source call. The row is the
// innermost call the walk entered the callee through: one value made of two
// calls of one function (dataSourceName + GetConfigString("dbName")) reads
// each call's own parameter, never the earlier call's.
func (d *DestinationReader) chosenCallers(use destinationPath, callees []string, formal *sourcevalue.Anchor) map[sourcevalue.Anchor]bool {
	chosen := d.bindingOf(use, callees, formal)
	d.consultBinding(callees, formal, chosen)
	return chosen
}

// bindingOf is chosenCallers without recording the consult.
func (d *DestinationReader) bindingOf(use destinationPath, callees []string, formal *sourcevalue.Anchor) map[sourcevalue.Anchor]bool {
	chosen := make(map[sourcevalue.Anchor]bool)
	for position := len(use.Steps) - 1; position >= 0; position-- {
		step := use.Steps[position]
		anchor := sourcevalue.Anchor{Path: step.Path, Line: step.Line, Column: step.Column}
		for _, call := range d.callSites[anchor] {
			if matchesDestinationOwner(call.call, callees, formal) {
				chosen[anchor] = true
			}
		}
		if len(chosen) > 0 {
			return chosen
		}
	}
	return chosen
}

// entryOf is the innermost call the walk entered owner through, as text,
// empty when it read owner's body from the start: a branch of a join is
// chosen once per entry, so two calls of one function may take two.
func (d *DestinationReader) entryOf(use destinationPath, owner atlas.Place) string {
	for anchor := range d.chosenCallers(use, []string{owner.ID}, nil) {
		return fmt.Sprintf("%s:%d:%d", anchor.Path, anchor.Line, anchor.Column)
	}
	return ""
}

// outsideItsBranch says a part of a join in owner's body is no value of the
// call the walk entered owner through: it lies in the branch of a case
// comparing a parameter with words and that call supplies the parameter
// another word (sourcevalue.OutsideItsBranch).
func (d *DestinationReader) outsideItsBranch(part *sourcevalue.Value, owner atlas.Place, use destinationPath) bool {
	if part.Anchor == nil || owner.Symbol == nil || len(owner.Symbol.Comparisons) == 0 {
		return false
	}
	chosen := d.chosenCallers(use, []string{owner.ID}, nil)
	if len(chosen) != 1 {
		return false
	}
	var call atlas.SymbolCall
	for anchor := range chosen {
		for _, site := range d.callSites[anchor] {
			if matchesDestinationOwner(site.call, []string{owner.ID}, nil) {
				call = site.call
			}
		}
	}
	var compared []sourcevalue.Compared
	for _, comparison := range owner.Symbol.Comparisons {
		item := sourcevalue.Compared{}
		if origin := comparison.Origin; origin != nil && origin.Kind == "parameter" {
			item.Position, item.Name = origin.Position, origin.Text
		}
		for _, written := range comparison.Cases {
			item.Cases = append(item.Cases, sourcevalue.ComparedCase{Words: written.Words, Line: written.BranchLine, EndLine: written.BranchEnd,
				Column: written.BranchColumn, EndColumn: written.BranchEndColumn, Exclusive: written.Exclusive})
		}
		compared = append(compared, item)
	}
	return sourcevalue.OutsideItsBranch(part.Anchor, owner.Path, compared, func(position int, name string) (string, bool) {
		supplied := namedSourceArgument(call, position, name)
		if supplied == nil || supplied.Kind != "literal" {
			return "", false
		}
		return supplied.Text, true
	})
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

// fieldLeaf reads on from the value an instance's field holds, when the
// walk found the instance: the value walk (nil) or the object walk.
type fieldLeaf func(value *sourcevalue.Value, owner atlas.Place, use destinationPath) []destinationPath

func (d *DestinationReader) field(receiver *sourcevalue.Value, name string, owner atlas.Place, use destinationPath, active map[string]bool, leaf fieldLeaf) []destinationPath {
	raw, _ := json.Marshal(receiver)
	key := "field:" + owner.ID + ":" + name + string(raw)
	if leaf != nil {
		key = "object " + key
	}
	if active[key] {
		d.consultCut()
		if leaf != nil {
			return nil
		}
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
				if leaf != nil {
					return leaf(&field.Parts[0], owner, use)
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
				next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(call.place, call.call))
				if len(next.TargetIDs) == 0 {
					continue
				}
				next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
				result = append(result, d.field(call.call.ReceiverValue, name, call.place, next, active, leaf)...)
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
			next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(call.place, call.call))
			if len(next.TargetIDs) == 0 {
				continue
			}
			next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
			result = append(result, d.field(argument, name, call.place, next, active, leaf)...)
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
				result = append(result, d.field(call.call.ResultValue, name, callee, next, active, leaf)...)
			}
		}
		if len(result) > 0 {
			return result
		}
	case "alternatives":
		var result []destinationPath
		for i, part := range receiver.Parts {
			if branch, ok := d.chooseDestinationPart(receiver, i, owner, d.entryOf(use, owner), use); ok {
				result = append(result, d.field(&part, name, owner, branch, active, leaf)...)
			}
		}
		return result
	}
	if leaf != nil {
		return nil
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

func (d *DestinationReader) chooseDestinationPart(value *sourcevalue.Value, part int, owner atlas.Place, entry string, use destinationPath) (destinationPath, bool) {
	branch := cloneDestinationPath(use)
	if value.Anchor == nil {
		return branch, true
	}
	encoded, _ := json.Marshal(value)
	key := owner.ID + "\x00" + entry + "\x00" + string(encoded)
	chosen, ok := branch.choices[key]
	d.consultChoice(key, chosen, ok)
	if ok && chosen != part {
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

// destinationEnds are a call's walks as they are saved: each end once (its
// address or frontier, whether unread, its method, targets and the source
// location it ends at), in the order first reached, with its shortest
// chain, when several reach it the number of routes, and the graph steps
// on any of its routes; and the graph itself, every distinct step and
// transition once. casdoor's avatar download had saved 77,299 chains to
// 54 ends; its graph is a few hundred steps and a few thousand edges.
func destinationEnds(uses []atlas.DestinationUse) ([]atlas.DestinationUse, *atlas.DestinationGraph) {
	type end struct {
		address, frontier, method, targets string
		unread                             bool
		at                                 string
	}
	graph := &atlas.DestinationGraph{}
	stepAt := map[atlas.DestinationStep]int{}
	edgeSeen := map[[2]int]bool{}
	position := func(step atlas.DestinationStep) int {
		if at, ok := stepAt[step]; ok {
			return at
		}
		stepAt[step] = len(graph.Steps)
		graph.Steps = append(graph.Steps, step)
		return stepAt[step]
	}
	at := map[end]int{}
	var result []atlas.DestinationUse
	through := map[int]map[int]bool{}
	for _, use := range uses {
		key := end{use.Address, use.Frontier, use.Method, strings.Join(use.TargetIDs, " "), use.Unread, ""}
		if n := len(use.Steps); n > 0 {
			last := use.Steps[n-1]
			key.at = fmt.Sprintf("%s:%d:%d", last.Path, last.Line, last.Column)
		}
		previous := -1
		var onRoute []int
		for _, step := range use.Steps {
			current := position(step)
			onRoute = append(onRoute, current)
			if previous >= 0 && !edgeSeen[[2]int{previous, current}] {
				edgeSeen[[2]int{previous, current}] = true
				graph.Edges = append(graph.Edges, [2]int{previous, current})
			}
			previous = current
		}
		routes := max(1, use.Routes)
		index, seen := at[key]
		if !seen {
			index = len(result)
			at[key] = index
			use.Routes = routes
			result = append(result, use)
			through[index] = map[int]bool{}
		} else {
			result[index].Routes += routes
			if len(use.Steps) < len(result[index].Steps) {
				use.Routes = result[index].Routes
				result[index] = use
			}
		}
		for _, step := range onRoute {
			through[index][step] = true
		}
		for _, step := range use.Through {
			through[index][step] = true
		}
	}
	for i := range result {
		if result[i].Routes == 1 {
			result[i].Routes = 0
		}
		result[i].Through = nil
		if result[i].Routes > 1 {
			for step := range through[i] {
				result[i].Through = append(result[i].Through, step)
			}
			slices.Sort(result[i].Through)
		}
	}
	if len(graph.Steps) == 0 {
		graph = nil
	}
	return result, graph
}

// destinationEvidence is each place a call's value ends, once: its address
// or unresolved expression, method and the step it ends at, with its
// shortest chain from the call and, when several routes reach it, how many
// (`routes`). The routes differ only on the way; where the value comes
// from is the end. casdoor's avatar download had sent 77,299 chains
// (68 MB) for 215 ends, a row no provider window holds.
func destinationEvidence(uses []atlas.DestinationUse) []map[string]any {
	type end struct {
		address, frontier, method string
		last                      atlas.DestinationStep
	}
	var order []end
	shortest := map[end]atlas.DestinationUse{}
	routes := map[end]int{}
	for _, use := range uses {
		key := end{address: use.Address, frontier: use.Frontier, method: use.Method}
		if n := len(use.Steps); n > 0 {
			key.last = use.Steps[n-1]
		}
		if kept, seen := shortest[key]; !seen || len(use.Steps) < len(kept.Steps) {
			if !seen {
				order = append(order, key)
			}
			shortest[key] = use
		}
		routes[key]++
	}
	var result []map[string]any
	for _, key := range order {
		var steps []map[string]any
		for _, step := range shortest[key].Steps {
			steps = append(steps, map[string]any{"name": step.Name, "path": step.Path, "line": step.Line})
		}
		evidence := map[string]any{"address": key.address, "unresolved_expression": key.frontier, "method": key.method, "source_chain": steps}
		if routes[key] > 1 {
			evidence["routes"] = routes[key]
		}
		result = append(result, evidence)
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
