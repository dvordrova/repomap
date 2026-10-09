package reading

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// The object an exchange goes through (owner, 2026-09-30: the calls that
// reach the same session or engine reach one destination; skeptic-reviewed
// the same day). An ORM's statement builders (freqtrade's select, text,
// update) name nothing they reach: the model decides no argument of theirs
// names it, so each row's own walk reads nothing, and each had been its own
// destination, named per window ("Freqtrade database" 39 times, "Database"
// 24). The statement is sent through an object:
// `Trade.session.scalars(select(...))`, `connection.execute(text(...))` in
// `with engine.begin() as connection`. That object is followed back, as the
// object an entry is declared on is (declared.go), to the outside call that
// made it; its walk is where the row's destination ends (the destination
// key and the destination's ends, never the row's own address or chain).
// Every row sent through objects create_engine(db_url) made ends where that
// call's walk ends: one key, one name. Code alone decides it.

// exchangeIndex is where each call's result goes, gathered once over the
// program's code outside tests: handed whole as an argument (a local name
// bound to the result is the result), made the receiver of a call, or
// returned by its function to each of that function's calls; and the calls
// handed a parameter whole, by the parameter's declaration and position or
// name.
type exchangeIndex struct {
	handed     map[sourcevalue.Anchor][]destinationCall
	onResult   map[sourcevalue.Anchor][]destinationCall
	returned   map[sourcevalue.Anchor][]destinationCall
	parameters map[parameterKey][]destinationCall
	// fields are the calls handed a field of a call's result
	// (`trades_grouped.c.count`, a column of a statement built as a CTE).
	fields map[sourcevalue.Anchor][]destinationCall
}

type parameterKey struct {
	owner    sourcevalue.Anchor
	position int
	name     string
}

func (d *DestinationReader) exchangeIndex() *exchangeIndex {
	if d.exchanges != nil {
		return d.exchanges
	}
	index := &exchangeIndex{handed: map[sourcevalue.Anchor][]destinationCall{}, onResult: map[sourcevalue.Anchor][]destinationCall{},
		returned: map[sourcevalue.Anchor][]destinationCall{}, parameters: map[parameterKey][]destinationCall{}, fields: map[sourcevalue.Anchor][]destinationCall{}}
	for _, id := range d.order {
		place := d.places[id]
		for _, call := range place.Symbol.Calls {
			item := destinationCall{place, call}
			if receiver := call.ReceiverValue; receiver != nil && receiver.Kind == "call_result" && receiver.Anchor != nil {
				index.onResult[*receiver.Anchor] = append(index.onResult[*receiver.Anchor], item)
			}
			for _, argument := range call.SourceArguments {
				for _, whole := range wholeValues(argument.Origin) {
					switch {
					case whole.Kind == "call_result" && whole.Anchor != nil:
						index.handed[*whole.Anchor] = append(index.handed[*whole.Anchor], item)
					case whole.Kind == "field":
						if base := fieldBase(whole); base != nil {
							index.fields[*base] = append(index.fields[*base], item)
						}
					case whole.Kind == "parameter" && whole.Owner != nil:
						index.parameters[parameterKey{owner: *whole.Owner, position: whole.Position}] = append(index.parameters[parameterKey{owner: *whole.Owner, position: whole.Position}], item)
						if whole.Text != "" {
							index.parameters[parameterKey{owner: *whole.Owner, name: whole.Text}] = append(index.parameters[parameterKey{owner: *whole.Owner, name: whole.Text}], item)
						}
					}
				}
			}
			if len(call.CalleeIDs) == 1 {
				for _, whole := range wholeValues(call.ResultValue) {
					if whole.Kind == "call_result" && whole.Anchor != nil {
						index.returned[*whole.Anchor] = append(index.returned[*whole.Anchor], item)
					}
				}
			}
		}
	}
	d.exchanges = index
	return index
}

// fieldBase is the site of the call a field, or a field of a field, is
// read from, nil when it is read from anything else.
func fieldBase(value *sourcevalue.Value) *sourcevalue.Anchor {
	for value.Kind == "field" && len(value.Parts) == 1 {
		value = &value.Parts[0]
	}
	if value.Kind == "call_result" {
		return value.Anchor
	}
	return nil
}

// wholeValues are the values a value is as a whole: itself, or each of its
// alternatives. A part of a text, a field or an element is none.
func wholeValues(value *sourcevalue.Value) []*sourcevalue.Value {
	if value == nil {
		return nil
	}
	if value.Kind != "alternatives" {
		return []*sourcevalue.Value{value}
	}
	var result []*sourcevalue.Value
	for i := range value.Parts {
		result = append(result, wholeValues(&value.Parts[i])...)
	}
	return result
}

// sendersOf are the calls an exchange's statement, the result of the call
// at site, is sent by: a call to no known function it is handed whole
// (`Trade.session.scalars(stmt)`, `connection.execute(stmt)`), and an
// outside call of the statement's kind made on an object it is handed
// whole. The statement is followed on through a call of its kind it is
// handed to or made on (`select(...).filter(...)`, a subquery handed to
// where), through a call answered nothing made on it (`.order_by(...)`,
// as a part handed on is), through the result of a call to no known
// function it is handed (a column's `not_in(subquery)`), into a
// repository function it is handed to and back out of one returning it,
// and into a call of its kind handed a column of it (a CTE's). A call of
// another kind it is handed to (a logger's, a request's) sends nothing.
func (d *DestinationReader) sendersOf(site sourcevalue.Anchor, kind string) []destinationCall {
	index := d.exchangeIndex()
	var senders []destinationCall
	seen := map[sourcevalue.Anchor]bool{}
	var follow func(site sourcevalue.Anchor)
	var handed func(call destinationCall, position int, keyword string)
	handed = func(call destinationCall, position int, keyword string) {
		switch {
		case call.call.API == nil && len(call.call.CalleeIDs) == 0:
			// It sends the statement through the object it is made on, or,
			// made on none a call made (a column's not_in(subquery)), its
			// result carries the statement on.
			senders = append(senders, call)
			follow(sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column})
		case call.call.API != nil:
			if d.talks[apiName(*call.call.API)] != kind {
				return
			}
			if receiver := call.call.ReceiverValue; receiver != nil && !d.sameKindResult(receiver, kind, runningCallTargets(call.place, call.call)) {
				senders = append(senders, call)
			}
			follow(sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column})
		case len(call.call.CalleeIDs) == 1:
			callee, ok := d.places[call.call.CalleeIDs[0]]
			if !ok {
				return
			}
			owner := sourcevalue.Anchor{Path: callee.Path, Line: callee.LineNo, Column: callee.Symbol.Decl.Column}
			keys := []parameterKey{{owner: owner, position: position}}
			if keyword != "" {
				keys = []parameterKey{{owner: owner, name: keyword}}
			}
			for _, key := range keys {
				for _, inner := range index.parameters[key] {
					for _, argument := range inner.call.SourceArguments {
						for _, whole := range wholeValues(argument.Origin) {
							if whole.Kind == "parameter" && whole.Owner != nil && *whole.Owner == owner && (whole.Position == position && keyword == "" || whole.Text == keyword && keyword != "") {
								handed(inner, argument.Position, argument.Keyword)
							}
						}
					}
				}
			}
		}
	}
	follow = func(site sourcevalue.Anchor) {
		if seen[site] {
			return
		}
		seen[site] = true
		for _, call := range index.onResult[site] {
			if call.call.API == nil {
				continue
			}
			if talks := d.talks[apiName(*call.call.API)]; talks == kind || talks == "" {
				follow(sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column})
			}
		}
		for _, call := range index.handed[site] {
			for _, argument := range call.call.SourceArguments {
				for _, whole := range wholeValues(argument.Origin) {
					if whole.Kind == "call_result" && whole.Anchor != nil && *whole.Anchor == site {
						handed(call, argument.Position, argument.Keyword)
					}
				}
			}
		}
		for _, call := range index.returned[site] {
			follow(sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column})
		}
		// A column of the statement handed to a call building one of its
		// kind is a part of that statement (a CTE's columns in select).
		for _, call := range index.fields[site] {
			if call.call.API != nil && d.talks[apiName(*call.call.API)] == kind {
				follow(sourcevalue.Anchor{Path: call.place.Path, Line: call.call.Line, Column: call.call.Column})
			}
		}
	}
	follow(site)
	return senders
}

// sameKindResult reports a value that is what an outside call answered
// kind returned: a statement being built, not the object it is sent
// through.
func (d *DestinationReader) sameKindResult(value *sourcevalue.Value, kind string, targets []string) bool {
	if value.Kind != "call_result" || value.Anchor == nil {
		return false
	}
	for _, call := range d.callSites[*value.Anchor] {
		if len(intersectTargets(targets, runningCallTargets(call.place, call.call))) == 0 {
			continue
		}
		if call.call.API != nil && d.talks[apiName(*call.call.API)] == kind {
			return true
		}
	}
	return false
}

// Exchange walks an outgoing call whose own walk reads nothing (its symbol
// decides no argument names what it reaches) from the object its exchange
// goes through: the object each call sending it is made on (sendersOf),
// else the object the call itself is made on. Empty when no outside call
// made that object: the call then stays its own destination.
func (d *DestinationReader) Exchange(place atlas.Place, call atlas.SymbolCall, kind string) []atlas.DestinationUse {
	initial := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: append([]string(nil), runningCallTargets(place, call)...), Steps: []atlas.DestinationStep{destinationStep(place, call)}}, kind: kind}
	active := make(map[string]bool)
	var result []destinationPath
	for _, sender := range d.sendersOf(sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}, kind) {
		next := cloneDestinationPath(initial)
		next.TargetIDs = intersectTargets(initial.TargetIDs, runningCallTargets(sender.place, sender.call))
		if len(next.TargetIDs) == 0 {
			continue
		}
		next.Steps = appendDestinationStep(next.Steps, destinationStep(sender.place, sender.call))
		result = append(result, d.object(sender.call.ReceiverValue, sender.place, next, true, active)...)
	}
	if len(result) == 0 && call.ReceiverValue != nil && !d.sameKindResult(call.ReceiverValue, kind, initial.TargetIDs) {
		result = d.object(call.ReceiverValue, place, initial, true, active)
	}
	if len(result) == 0 {
		return nil
	}
	return publishDestinationPaths(result)
}

// object walks the object a value is back to the outside call that made
// it: a parameter to each caller's argument, a field to the value its one
// store gives it, what entering a context manager gives to the context
// manager, a repository call to what its function returns, and a call to
// no known function giving no words (engine.begin()) to the object it is
// made on. An outside call with a decided argument made the object: the
// walk goes on through that argument (create_engine(db_url)). An outside
// call never asked which argument names what it reaches and giving no
// words is followed to the object it is made from (producedObject:
// scoped_session(sessionmaker(bind=engine))). Any other call is the
// object itself, keyed by its site (a call answered none, or giving words:
// boto3.client("s3") and boto3.client("sqs") stay two), and only when
// itself is set; an object an outside call answered another kind made is
// none. A field no store gives a value, an unknown name and a receiver
// give none: they say nothing of which object it is.
func (d *DestinationReader) object(value *sourcevalue.Value, owner atlas.Place, use destinationPath, itself bool, active map[string]bool) []destinationPath {
	if value == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	key := "object:" + owner.ID + string(raw)
	if active[key] {
		d.consultCut()
		return nil
	}
	active[key] = true
	defer delete(active, key)
	var result []destinationPath
	switch value.Kind {
	case "call_result":
		if value.Anchor == nil {
			return nil
		}
		for _, call := range d.callSites[*value.Anchor] {
			next := cloneDestinationPath(use)
			next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(call.place, call.call))
			if len(next.TargetIDs) == 0 {
				continue
			}
			next.Steps = appendDestinationStep(next.Steps, destinationStep(call.place, call.call))
			switch {
			case call.call.API != nil:
				symbol := apiName(*call.call.API)
				if talks := d.talks[symbol]; talks != "" && talks != use.kind {
					continue
				}
				if chosen := d.chosen(call.call); chosen != nil {
					result = append(result, d.value(chosen, call.place, next, active)...)
					continue
				}
				if _, decided := d.choices.Arguments[symbol]; !decided && len(call.call.Values) == 0 {
					if found := d.object(producedObject(call.call), call.place, next, itself, active); len(found) > 0 {
						result = append(result, found...)
						continue
					}
				}
				if itself {
					next.Frontier = call.call.Name + "()"
					result = append(result, next)
				}
			case len(call.call.CalleeIDs) == 1 && call.call.ResultValue != nil:
				if callee, ok := d.places[call.call.CalleeIDs[0]]; ok {
					result = append(result, d.object(call.call.ResultValue, callee, next, itself, active)...)
				}
			case len(call.call.CalleeIDs) == 0:
				if len(call.call.Values) == 0 {
					if found := d.object(call.call.ReceiverValue, call.place, next, itself, active); len(found) > 0 {
						result = append(result, found...)
						continue
					}
				}
				if itself {
					next.Frontier = call.call.Name + "()"
					result = append(result, next)
				}
			}
		}
	case "parameter":
		for _, caller := range d.parameterCallers(value, use) {
			next := cloneDestinationPath(use)
			next.TargetIDs = intersectTargets(use.TargetIDs, runningCallTargets(caller.place, caller.call))
			if len(next.TargetIDs) == 0 {
				continue
			}
			next.Steps = appendDestinationStep(next.Steps, destinationStep(caller.place, caller.call))
			result = append(result, d.object(namedSourceArgument(caller.call, value.Position, value.Text), caller.place, next, itself, active)...)
		}
	case "field":
		// The instance the walk follows first, as the value walk does;
		// only an instance it cannot follow reads what the field's writes
		// stored (its initializer), each from its write.
		leaf := func(field *sourcevalue.Value, at atlas.Place, from destinationPath) []destinationPath {
			return d.object(field, at, from, itself, active)
		}
		if found := d.field(&value.Parts[0], value.Text, owner, use, active, leaf); len(found) > 0 {
			return found
		}
		if value.Initializer != nil {
			// Which instance it is, the walk could not say: what any write
			// of the field stored is a possible origin, an end at its site
			// that establishes no address (two configurations' engines must
			// not become one destination through a field they share).
			possible := d.initializer(value.Initializer, owner, use, active, writesWalk{object: true, itself: itself})
			for i := range possible {
				if possible[i].Address != "" {
					possible[i].Frontier = "initializer: " + possible[i].Address
					possible[i].Address = ""
				}
			}
			return possible
		}
	case "entered":
		return d.object(&value.Parts[0], owner, use, itself, active)
	case "alternatives":
		for i, part := range value.Parts {
			if branch, ok := d.chooseDestinationPart(value, i, owner, d.entryOf(use, owner), use); ok {
				result = append(result, d.object(&part, owner, branch, itself, active)...)
			}
		}
	}
	return result
}

// producedObject is the object an outside call is made from: its
// receiver, else its first argument another call made or a caller hands
// (scoped_session(sessionmaker(...)) is made from the sessionmaker,
// sessionmaker(autoflush=False, bind=engine) from the engine), never a
// literal, a text or a field of something.
func producedObject(call atlas.SymbolCall) *sourcevalue.Value {
	if call.ReceiverValue != nil {
		return call.ReceiverValue
	}
	for _, argument := range call.SourceArguments {
		if origin := argument.Origin; origin != nil && (origin.Kind == "call_result" || origin.Kind == "parameter" || origin.Kind == "entered") {
			return origin
		}
	}
	return nil
}
