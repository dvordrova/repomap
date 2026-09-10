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
type DestinationReader struct {
	places         map[string]atlas.Place
	callers        map[string][]destinationCall
	callSites      map[sourcevalue.Anchor][]destinationCall
	owners         map[sourcevalue.Anchor][]atlas.Place
	ownerLines     map[sourcevalue.Anchor][]atlas.Place
	parameterCalls map[sourcevalue.Anchor][]destinationCall
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
}

func NewDestinationReader(places []atlas.Place) *DestinationReader {
	d := &DestinationReader{places: make(map[string]atlas.Place), callers: make(map[string][]destinationCall), callSites: make(map[sourcevalue.Anchor][]destinationCall), owners: make(map[sourcevalue.Anchor][]atlas.Place), ownerLines: make(map[sourcevalue.Anchor][]atlas.Place), parameterCalls: make(map[sourcevalue.Anchor][]destinationCall)}
	for _, place := range places {
		if place.Symbol == nil {
			continue
		}
		d.places[place.ID] = place
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

func (d *DestinationReader) Read(place atlas.Place, call atlas.SymbolCall) []atlas.DestinationUse {
	position, purpose := destinationArgument(call.API)
	step := destinationStep(place, call)
	initial := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: append([]string(nil), place.TargetIDs...), Steps: []atlas.DestinationStep{step}}}
	if purpose == "options" {
		var result []destinationPath
		for _, argument := range call.SourceArguments {
			if argument.Origin == nil || argument.Origin.Kind != "call_result" || argument.Origin.Anchor == nil {
				continue
			}
			for _, option := range d.callSites[*argument.Origin.Anchor] {
				if _, kind := destinationArgument(option.call.API); kind == "endpoint" {
					result = append(result, d.value(argument.Origin, place, initial, make(map[string]bool))...)
				}
			}
		}
		if len(result) > 0 {
			return publishDestinationPaths(result)
		}
	}
	if position == 0 {
		initial.Frontier = call.Name
		return []atlas.DestinationUse{initial.DestinationUse}
	}
	if value := destinationSourceArgument(call, position); value != nil {
		return publishDestinationPaths(d.value(value, place, initial, make(map[string]bool)))
	}
	initial.Frontier = call.Name
	return []atlas.DestinationUse{initial.DestinationUse}
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

// Keyword arguments retain their written names; the native API contract owns
// the URL parameter name, just as it owns its positional slot.
func destinationSourceArgument(call atlas.SymbolCall, position int) *sourcevalue.Value {
	if call.API != nil {
		switch call.API.Package {
		case "requests", "requests.api", "requests.sessions", "requests.models", "httpx", "httpx._api", "httpx._client", "aiohttp", "aiohttp.client":
			return namedSourceArgument(call, position, "url")
		}
	}
	return sourceArgument(call, position)
}

// The closed mechanisms identify where a supplied value goes. They never
// promote a wrapper, import or request builder into a communication operation.
func destinationArgument(api *atlas.CallAPI) (int, string) {
	if api == nil {
		return 0, ""
	}
	name := api.Name
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	switch api.Package {
	case "net/http":
		switch name {
		case "Get", "Head", "Post", "PostForm", "Do", "RoundTrip":
			return 1, "exchange"
		case "NewRequest":
			return 2, "request"
		case "NewRequestWithContext":
			return 3, "request"
		case "WithContext", "Clone":
			return -1, "receiver"
		}
	case "net/url":
		if name == "String" {
			return -1, "receiver"
		}
		if name == "Parse" {
			return 1, "endpoint"
		}
	case "flag":
		if name == "String" {
			return 1, "flag"
		}
	case "os":
		if name == "Getenv" || name == "LookupEnv" || name == "getenv" {
			return 1, "environment"
		}
	case "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp", "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc", "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp", "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc":
		switch name {
		case "New", "NewClient":
			return 0, "options"
		case "WithEndpoint", "WithEndpointURL":
			return 1, "endpoint"
		}
	case "platform:javascript":
		switch name {
		case "fetch":
			return 1, "exchange"
		case "Request":
			return 1, "request"
		}
	case "requests", "requests.api", "requests.sessions", "requests.models", "httpx", "httpx._api", "httpx._client", "aiohttp", "aiohttp.client", "axios", "node-fetch", "ky", "got":
		if name == "Request" {
			return 2, "request"
		}
		switch strings.ToLower(name) {
		case "get", "post", "put", "patch", "delete", "head", "fetch":
			return 1, "exchange"
		case "request":
			return 2, "exchange"
		}
	}
	return 0, ""
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
			next.TargetIDs = intersectTargets(use.TargetIDs, caller.place.TargetIDs)
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
				position, kind := destinationArgument(call.call.API)
				argument := destinationSourceArgument(call.call, position)
				if (kind == "flag" || kind == "environment") && argument != nil && argument.Kind == "literal" {
					prefix := "env:"
					if kind == "flag" {
						prefix = "--"
					}
					next.Address = "{" + prefix + argument.Text + "}"
					result = append(result, next)
				} else if (kind == "request" || kind == "endpoint") && argument != nil {
					if kind == "request" {
						if method := sourceArgument(call.call, position-1); method != nil && method.Kind == "literal" {
							next.Method = method.Text
						}
					}
					branches := d.value(argument, call.place, next, active)
					if kind == "request" {
						if method := sourceArgument(call.call, position-1); method != nil {
							for i := range branches {
								methodUse := cloneDestinationPath(branches[i])
								methodUse.Address = ""
								methodUse.Frontier = ""
								methods := d.value(method, call.place, methodUse, active)
								if len(methods) == 1 && methods[0].Frontier == "" {
									branches[i].Method = methods[0].Address
								}
							}
						}
					}
					result = append(result, branches...)
				} else if kind == "receiver" && call.call.ReceiverValue != nil {
					result = append(result, d.value(call.call.ReceiverValue, call.place, next, active)...)
				} else if call.call.ResultValue != nil && len(call.call.CalleeIDs) == 1 {
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
					if prefix.Frontier == "" && suffix.Frontier == "" {
						suffix.Address = prefix.Address + suffix.Address
					} else {
						left, right := prefix.Address, suffix.Address
						if prefix.Frontier != "" {
							left = prefix.Frontier
						}
						if suffix.Frontier != "" {
							right = "{" + suffix.Frontier + "}"
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
			initial := value.Initializer
			if initial.Kind == "field_value" {
				initial = &initial.Parts[0]
			}
			result = d.value(initial, owner, use, active)
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
	}
	if value.Anchor != nil {
		use.Steps = appendDestinationStep(use.Steps, atlas.DestinationStep{SubjectID: owner.ID, Name: use.Frontier, Path: value.Anchor.Path, Line: value.Anchor.Line, Column: value.Anchor.Column})
	}
	return []destinationPath{use}
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
		return selected
	}
	var callers []destinationCall
	for _, id := range ownerIDs {
		callers = append(callers, d.callers[id]...)
	}
	// Constructors can bind formals without a synthetic edge to __init__.
	if value.Owner != nil {
		callers = append(callers, d.parameterCalls[*value.Owner]...)
	}
	return callers
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
			callers := d.callers[owners[0].ID]
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
				next.TargetIDs = intersectTargets(use.TargetIDs, call.place.TargetIDs)
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
			next.TargetIDs = intersectTargets(use.TargetIDs, call.place.TargetIDs)
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
