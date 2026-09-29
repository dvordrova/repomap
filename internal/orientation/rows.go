package orientation

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// memberRow is one declaration a request shows: its own facts and every call
// it makes, in the order they are written in it, one tuple per call
// (encodeCall). Nothing is cut: a declaration's row is complete or absent.
// Who calls it is not listed: inside a request's scope a caller's own row
// already lists the call.
type memberRow struct {
	Ref       string           `json:"ref"`
	Name      string           `json:"name"`
	Kind      string           `json:"kind,omitempty"`
	Anchor    string           `json:"anchor,omitempty"`
	Signature string           `json:"signature,omitempty"`
	AuthorDoc string           `json:"author_doc,omitempty"`
	Calls     []any            `json:"calls,omitempty"`
	Evidence  map[string][]any `json:"evidence,omitempty"`
}

// The words a call's flags may hold, each from its own closed vocabulary.
// The four vocabularies share no word, so a flag names its field. A value
// outside its vocabulary is written under its field name in the call's
// details instead.
var (
	callKinds = []string{
		string(programindex.RelationCalls), string(programindex.RelationImports), string(programindex.RelationImplements),
		string(programindex.RelationDecorates), string(programindex.RelationPassesCallback), string(programindex.RelationBindsImplementation),
		string(programindex.RelationSources), string(programindex.RelationExecutes), string(programindex.RelationReads),
		string(programindex.RelationWrites), string(programindex.RelationInvokesExternal),
	}
	callInvocations = []string{programindex.InvocationDeferred, programindex.InvocationGoroutine, programindex.InvocationAsyncTask, programindex.InvocationConstruct}
	callDispatches  = []string{programindex.DispatchInterface, programindex.DispatchInterfaceMethod, programindex.DispatchFunctionValue}
	callResolutions = []string{string(programindex.ResolutionExact), string(programindex.ResolutionAlternatives), string(programindex.ResolutionUnresolved)}
)

const (
	defaultCallKind   = string(programindex.RelationCalls)
	defaultResolution = string(programindex.ResolutionExact)
	// repositoryCallee stands for callee candidates the repository indexes
	// but that have no declaration place to name.
	repositoryCallee = "(repository)"
)

// rowWriter writes member rows over the source graph. Places are by place
// ID; refs name, by place ID, the declarations the request lists as
// members, so a call into one of them names its ref.
type rowWriter struct {
	places map[string]atlas.Place
	refs   map[string]string
}

func newRowWriter(graph atlas.Graph) *rowWriter {
	writer := &rowWriter{places: make(map[string]atlas.Place, len(graph.Places)), refs: make(map[string]string)}
	for _, place := range graph.Places {
		writer.places[place.ID] = place
	}
	return writer
}

// row writes one member. A member without a declaration place keeps its
// label and has no calls to show.
func (writer *rowWriter) row(ref string, label memberWire, place *atlas.Place) memberRow {
	row := memberRow{Ref: ref, Name: label.Name, Kind: label.Kind, Anchor: label.Anchor}
	if place == nil || place.Symbol == nil {
		return row
	}
	decl := place.Symbol.Decl
	row.Name, row.Anchor = decl.Name, anchorString(place.Path, place.LineNo)
	if decl.Kind != "" {
		row.Kind = decl.Kind
	}
	row.Signature, row.AuthorDoc = decl.Signature, decl.Doc
	evidence := evidenceRefs{}
	for _, call := range lines.WrittenOrder(place.Symbol.Calls) {
		row.Calls = append(row.Calls, writer.call(call, &evidence))
	}
	row.Evidence = evidence.byRef
	return row
}

// call writes one call as a tuple:
//
//	"name@line -> callee | callee"   the called name, its line, and each
//	                                 declaration it may reach: a listed
//	                                 member's ref, or "name (path:line)"
//	"flags"                          its non-default kind, invocation,
//	                                 dispatch and resolution words
//	{details}                        detail, args, receiver, result, values,
//	                                 arguments, api, evidence_refs
//
// A call with neither flags nor details is its head string alone. An
// origin is [kind, text] or [kind, text, initializer or 0, part, ...]. A
// call's values are left out only when they are exactly its literal
// arguments' texts in order. The call's column and each callee's column stay
// local, as the evidence rows keep them.
func (writer *rowWriter) call(call atlas.SymbolCall, evidence *evidenceRefs) any {
	head := call.Name + "@" + strconv.Itoa(call.Line)
	var callees []string
	for _, id := range call.CalleeIDs {
		if ref := writer.refs[id]; ref != "" {
			callees = append(callees, ref)
			continue
		}
		if place, known := writer.places[id]; known && place.Symbol != nil {
			callees = append(callees, place.Symbol.Decl.Name+" ("+anchorString(place.Path, place.LineNo)+")")
		}
	}
	if len(callees) == 0 && len(call.CalleeIDs) > 0 {
		callees = []string{repositoryCallee}
	}
	if len(callees) > 0 {
		head += " -> " + strings.Join(callees, " | ")
	}
	details := map[string]any{}
	var flags []string
	flag := func(field, value, fallback string, vocabulary []string) {
		switch {
		case value == "" || value == fallback:
		case slices.Contains(vocabulary, value):
			flags = append(flags, value)
		default:
			details[field] = value
		}
	}
	flag("kind", call.Kind, defaultCallKind, callKinds)
	if call.Kind == "" {
		// An empty kind is not the default one.
		details["kind"] = ""
	}
	flag("invocation", call.Invocation, "", callInvocations)
	flag("dispatch", call.Dispatch, "", callDispatches)
	flag("resolution", call.Resolution, defaultResolution, callResolutions)
	if call.Resolution == "" {
		details["resolution"] = ""
	}
	if call.Detail != "" {
		details["detail"] = call.Detail
	}
	if len(call.SourceArguments) > 0 {
		args := make([]any, 0, len(call.SourceArguments))
		for _, argument := range call.SourceArguments {
			switch {
			case argument.Keyword == "":
				args = append(args, []any{argument.Position, originTuple(argument.Origin, 0)})
			case argument.Position == 0:
				args = append(args, []any{argument.Keyword, originTuple(argument.Origin, 0)})
			default:
				args = append(args, []any{argument.Position, argument.Keyword, originTuple(argument.Origin, 0)})
			}
		}
		details["args"] = args
	}
	if call.ReceiverValue != nil {
		details["receiver"] = originTuple(call.ReceiverValue, 0)
	}
	if call.ResultValue != nil {
		details["result"] = originTuple(call.ResultValue, 0)
	}
	if values := literalTexts(call.SourceArguments); !slices.Equal(call.Values, values) {
		details["values"] = append([]string{}, call.Values...)
	}
	if len(call.Arguments) > 0 {
		details["arguments"] = call.Arguments
	}
	if call.API != nil {
		details["api"] = []string{call.API.Package, call.API.Receiver, call.API.Name, call.API.Signature}
	}
	if refs := evidence.of(call.Evidence); len(refs) > 0 {
		details["evidence_refs"] = refs
	}
	tuple := []any{head}
	if len(flags) > 0 {
		tuple = append(tuple, strings.Join(flags, " "))
	}
	if len(details) > 0 {
		tuple = append(tuple, details)
	}
	if len(tuple) == 1 {
		return head
	}
	return tuple
}

// literalTexts are the texts of a call's literal arguments, in order, each
// once: what the call's values hold when they say nothing more.
func literalTexts(arguments []atlas.SourceArgument) []string {
	var texts []string
	for _, argument := range arguments {
		if argument.Origin != nil && argument.Origin.Kind == "literal" && argument.Origin.Text != "" && !slices.Contains(texts, argument.Origin.Text) {
			texts = append(texts, argument.Origin.Text)
		}
	}
	return texts
}

// originTuple writes a source origin as [kind, text], or [kind, text,
// initializer or 0, part, ...] when it has an initializer or parts, down to
// lines.OriginDepth levels below its root as every evidence row keeps it.
func originTuple(value *sourcevalue.Value, level int) any {
	if value == nil {
		return nil
	}
	tuple := []any{value.Kind, value.Text}
	if level >= lines.OriginDepth || value.Initializer == nil && len(value.Parts) == 0 {
		return tuple
	}
	if value.Initializer != nil {
		tuple = append(tuple, originTuple(value.Initializer, level+1))
	} else {
		tuple = append(tuple, 0)
	}
	for i := range value.Parts {
		tuple = append(tuple, originTuple(&value.Parts[i], level+1))
	}
	return tuple
}

// evidenceRefs shares a row's repeated source observations: refs e1, e2, ...
// in encounter order, local to the row, each [extractor, label, path, line].
type evidenceRefs struct {
	refs  map[atlas.EdgeEvidence]string
	byRef map[string][]any
}

func (catalogue *evidenceRefs) of(evidence []atlas.EdgeEvidence) []string {
	var refs []string
	for _, observation := range evidence {
		if catalogue.refs == nil {
			catalogue.refs = make(map[atlas.EdgeEvidence]string)
			catalogue.byRef = make(map[string][]any)
		}
		ref, known := catalogue.refs[observation]
		if !known {
			ref = fmt.Sprintf("e%d", len(catalogue.refs)+1)
			catalogue.refs[observation] = ref
			catalogue.byRef[ref] = []any{observation.Extractor, observation.Label, observation.Path, observation.LineNo}
		}
		refs = append(refs, ref)
	}
	return refs
}
