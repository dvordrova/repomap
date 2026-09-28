package lines

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	StageSymbols    = "atlas_symbols"
	StageBoundaries = "atlas_boundaries"
	StageZones      = "atlas_zones"
	StageArrows     = "atlas_arrows"
	StageTargets    = "atlas_targets"
	StageJoints     = "atlas_joints"

	symbolsContract    = "repomap.atlas.symbols.v8"
	boundariesContract = "repomap.atlas.boundaries.v8"
	arrowsContract     = "repomap.atlas.arrows.v1"
	targetsContract    = "repomap.atlas.targets.v3"
	jointsContract     = "repomap.atlas.joints.v3"

	// ShortLineRunes bounds the lines of boundaries and symbols; LabelRunes
	// the label of a joint.
	ShortLineRunes = 120
	LabelRunes     = 40

	// PeerNone is the peer choice that says no listed boundary is the
	// counterpart.
	PeerNone = "none"

	maxWitnesses = 3
	maxValues    = 8
)

//go:embed prompts/symbols.md
var symbolsPrompt string

//go:embed prompts/types.md
var typesPrompt string

//go:embed prompts/fixed_boundaries.md
var fixedBoundariesPrompt string

//go:embed prompts/arrows.md
var arrowsPrompt string

//go:embed prompts/targets.md
var targetsPrompt string

//go:embed prompts/target_descriptions.md
var targetDescriptionsPrompt string

//go:embed prompts/joints.md
var jointsPrompt string

// MaxKeysPerFile bounds how many symbols the model may mark as key in one
// file; the code keeps the first by rank.
const MaxKeysPerFile = 5

// AliasColumn is the Symbols and Types cell holding a declaration's short
// English reader label. It is asked only of a name that NeedsAlias; the other
// rows are asked the same table without it.
const AliasColumn = "alias"

// NeedsAlias reports whether a declaration's name is asked an English alias:
// it holds a letter outside the Latin script, as parse한국 does. Digits,
// underscores and punctuation are not letters, so snake_case_ascii needs
// none, and neither does any other Latin-script name, a transliterated one
// included (owner decision 2026-09-26). Code decides this; the model is
// never asked which names are English.
func NeedsAlias(name string) bool {
	for _, r := range name {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return true
		}
	}
	return false
}

func aliasColumn() table.Column {
	return table.Column{Name: AliasColumn, Kind: table.Text, MaxRunes: LabelRunes, EmptyValue: "none", Alone: true,
		Note: "asked only for a name not written in Latin letters: a short English reader label grounded in this declaration; none when the evidence does not establish one"}
}

// Symbols explains the selected declarations displayed in the overview.
func Symbols() table.Definition {
	return table.Definition{
		Stage: StageSymbols, Contract: symbolsContract,
		System: withVocabulary(symbolsPrompt), Memoize: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, Note: "one sentence, what this declaration does or is"},
			aliasColumn(),
		},
	}
}

// Types describes the concept represented by a type using its own interface.
// It shares symbol knowledge and publication; it does not classify operations.
func Types() table.Definition {
	return table.Definition{
		Stage: StageSymbols, Contract: "repomap.atlas.types.v7",
		System: typesPrompt, Memoize: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Prose, Note: "briefly explain what this represents or controls and any consequential documented rule, preserving its conditions; no method inventory or invented effects"},
			aliasColumn(),
		},
	}
}

func TypeRow(place atlas.Place) table.Row {
	decl := place.Symbol.Decl
	fields := []table.Field{{Name: "path", Value: place.Path}, {Name: "name", Value: decl.Name},
		{Name: "signature", Value: decl.Signature}, {Name: "author_doc", Value: decl.Doc}}
	fields = append(fields, table.Field{Name: "owned_declarations", Value: ownedDeclarations(place.Symbol.Members)})
	return table.Row{ID: place.ID, Fields: fields}
}

func ownedDeclarations(declarations []atlas.TypeMember) []map[string]any {
	members := make([]map[string]any, 0, len(declarations))
	for _, member := range declarations {
		entry := map[string]any{"path": member.Path, "line": member.Decl.LineNo,
			"name": member.Decl.Name, "kind": member.Decl.Kind, "signature": member.Decl.Signature, "author_doc": member.Decl.Doc}
		if member.Decl.Aliases != "" {
			entry["aliases"] = member.Decl.Aliases
		}
		members = append(members, entry)
	}
	return members
}

// siteCall is one call of a symbol row with every line it occurs on. Calls
// whose rendered evidence is identical apart from their line (the column is
// never rendered) are one entry: nothing that tells them apart is dropped.
// The Line field hides the embedded call's own line, so an entry names its
// sites once, as lines, in call order.
type siteCall struct {
	callEvidence
	Line  int   `json:"line,omitempty"`
	Lines []int `json:"lines"`
}

// SymbolRow builds the row of one candidate symbol. Each call carries its
// evidence, written once with the lines of all its identical sites; an exact
// repository callee is internal delegation, one line of context under
// local_calls. No column selects a call, so calls carry no refs.
func SymbolRow(place atlas.Place, fileLine string) table.Row {
	evidence := EvidenceCatalog{OmitDefaults: true}
	decl := place.Symbol.Decl
	fields := []table.Field{
		{Name: "path", Value: place.Path},
		{Name: "name", Value: decl.Name},
		{Name: "kind", Value: decl.Kind},
	}
	if decl.Signature != "" {
		fields = append(fields, table.Field{Name: "signature", Value: cut(decl.Signature, maxSignature)})
	}
	if decl.Doc != "" {
		fields = append(fields, table.Field{Name: "doc", Value: cut(decl.Doc, maxDoc)})
	}
	if fileLine != "" {
		fields = append(fields, table.Field{Name: "file_hypothesis", Value: fileLine})
	}
	fields = append(fields, table.Field{Name: "callers", Value: decl.FanIn})
	if len(place.Symbol.Bindings) > 0 {
		fields = append(fields, table.Field{Name: "callable_bindings", Value: evidence.Bindings(place.Symbol.Bindings)})
	}
	calls := make([]*siteCall, 0, len(place.Symbol.Calls))
	sites := make(map[string]*siteCall)
	var local []string
	for _, call := range place.Symbol.Calls {
		// A complete exact repository callee is internal delegation at this
		// site: one context line. Possible or unresolved dispatch keeps its
		// full evidence.
		if call.Kind == "calls" && call.Resolution == DefaultResolution && len(call.CalleeIDs) == 1 && call.API == nil {
			local = append(local, localCall(call))
			continue
		}
		rendered := evidence.call(call)
		line := rendered.Line
		rendered.Line = 0
		identity, err := json.Marshal(rendered)
		if site := sites[string(identity)]; err == nil && site != nil {
			site.Lines = append(site.Lines, line)
			continue
		}
		site := &siteCall{callEvidence: rendered, Lines: []int{line}}
		if err == nil {
			sites[string(identity)] = site
		}
		calls = append(calls, site)
	}
	fields = append(fields, table.Field{Name: "calls", Value: calls})
	if len(local) > 0 {
		fields = append(fields, table.Field{Name: "local_calls", Value: local})
	}
	fields = append(fields, evidence.Fields()...)
	return table.Row{ID: place.ID, Fields: fields}
}

// localCall is the context line of an exact repository callee: name@line, a
// non-default invocation, and the control statements whose bodies hold the
// call. The callee's own operation is reviewed on its own row; this row keeps
// what the call says about this declaration, such as a loop that launches it.
func localCall(call atlas.SymbolCall) string {
	line := fmt.Sprintf("%s@%d", call.Name, call.Line)
	if call.Invocation != "" {
		line += " " + call.Invocation
	}
	var statements []string
	for _, observation := range call.Evidence {
		if observation.Extractor == "control_context" {
			statements = append(statements, observation.Label)
		}
	}
	if len(statements) > 0 {
		line += " (" + strings.Join(statements, "; ") + ")"
	}
	return line
}

// FixedBoundaries explains a boundary whose existence and kind the facts and
// the symbol roles already gave. An incoming entry also chooses its name among
// the words its registration wrote; an outgoing fact whose address the code
// does not know may still need a destination and an address choice. Each cell
// fails alone: a refused line keeps the fact's given line, a refused name
// leaves the handler's own name, a refused destination or address names none.
func FixedBoundaries(outgoing bool) table.Definition {
	def := table.Definition{
		Stage: StageBoundaries, Contract: boundariesContract + ".fixed",
		System: fixedBoundariesPrompt, Memoize: true,
		Columns: []table.Column{{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, Alone: true,
			Note: "at most ten words, no subject: what this native observation reads, receives or sends; a configuration read is not itself a remote exchange"}},
	}
	if outgoing {
		def.Contract += ".outbound"
		def.Columns = append(def.Columns, destinationColumn(), addressColumn())
		return def
	}
	def.Columns = append(def.Columns, table.Column{Name: "name", Kind: table.Sequence, OptionsFrom: "word_options", WhenOptionsFrom: "word_options", Alone: true,
		Note: "the w* refs of the words that name this entry as its sender names it, in the order they are read; none when no word names it"})
	return def
}

// EntryWord is one word an incoming registration wrote, offered to name its
// entry: a verb, a path, a command name, a topic, an event, an RPC method.
// Which one it is, the model reads; the code keeps the value as written.
type EntryWord struct {
	Ref   string `json:"ref"`
	Value string `json:"value"`
}

// EntryWords are the words an incoming entry may be named by, as w1, w2, ...
// in the order the registration wrote them. A listener names no entry. A word
// that cannot stand in a one-line name as written (a control character such
// as a newline, or space around it) is not offered: it is never trimmed into
// one.
func EntryWords(place atlas.Place) []EntryWord {
	facts := place.Boundary
	if facts == nil || facts.Direction != atlas.DirectionIn || facts.GivenKind == atlas.BoundaryListenAddress {
		return nil
	}
	words := make([]EntryWord, 0, len(facts.Words))
	for _, value := range facts.Words {
		if !nameable(value) {
			continue
		}
		words = append(words, EntryWord{Ref: fmt.Sprintf("w%d", len(words)+1), Value: value})
	}
	return words
}

// NameableWords are the words, as written and in order, that can stand in
// a one-line name: the name of an entry no word was chosen for.
func NameableWords(values []string) []string {
	var words []string
	for _, value := range values {
		if nameable(value) {
			words = append(words, value)
		}
	}
	return words
}

func nameable(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// EntryName restores a name cell: the chosen words as written, joined by one
// space in the order the model wrote them. No word is translated, recased,
// trimmed or composed any other way.
func EntryName(words []EntryWord, cell string) string {
	values := make(map[string]string, len(words))
	for _, word := range words {
		values[word.Ref] = word.Value
	}
	var chosen []string
	for _, ref := range strings.Fields(cell) {
		if value, ok := values[ref]; ok {
			chosen = append(chosen, value)
		}
	}
	return strings.Join(chosen, " ")
}

// DestinationOther prefixes a destination the catalogue does not name.
const DestinationOther = "other: "

// destinationColumn chooses among the names the row's outside packages
// were given (Destinations), or names a system none of them covers.
func destinationColumn() table.Column {
	return table.Column{Name: "destination", Kind: table.Choice, OptionsFrom: "destination_options", Free: DestinationOther, FreeMaxRunes: LabelRunes, Alone: true,
		Note: "the d* ref of the system this call reaches from context.destination_catalog, or other: and its short name when no entry names it"}
}

// addressColumn is asked only where the row carries address candidates: a
// row whose address the code already knows, or that has no candidate
// literal, has no address decision. A missing address is the declared
// unknown, which keeps no address.
func addressColumn() table.Column {
	return table.Column{Name: "address", Kind: table.Choice, OptionsFrom: "address_options", WhenOptionsFrom: "address_options", Missing: "unknown", Alone: true,
		Note: "one supplied a* address value, or unknown when no observed value identifies the destination"}
}

// BoundaryAddress keeps the original supplied bytes and their source context.
// These observations are choices, not locally assigned destination semantics.
type BoundaryAddress struct {
	Ref   string `json:"ref"`
	Value string `json:"value"`
	Call  string `json:"call,omitempty"`
	Line  int    `json:"line,omitempty"`
}

// BoundaryAddresses lists the address candidates of one candidate call: its
// own observed values and the literals of the owner's other calls. Format
// templates and the literals of formatting, error, logging, time, string and
// number-conversion calls are not addresses: Morfeu offered every fmt.Errorf
// message and time.Format layout of the owner, and 25 of 29 accepted rows
// chose unknown.
func BoundaryAddresses(place atlas.Place, owners ...atlas.Place) []BoundaryAddress {
	var values []BoundaryAddress
	seen := make(map[string]bool)
	add := func(value, call string, line int) {
		if value == "" || seen[value] || FormatTemplate(value) {
			return
		}
		seen[value] = true
		values = append(values, BoundaryAddress{Ref: fmt.Sprintf("a%d", len(values)+1), Value: value, Call: call, Line: line})
	}
	for _, value := range place.Boundary.Values {
		add(value, place.Boundary.External, place.LineNo)
	}
	for _, owner := range owners {
		if owner.Symbol == nil {
			continue
		}
		for _, call := range owner.Symbol.Calls {
			if !AddressCandidateCall(call) {
				continue
			}
			for _, value := range call.Values {
				add(value, call.Name, call.Line)
			}
		}
	}
	return values
}

// AddressCandidateCall reports a call whose literals may be addresses: one
// outside the formatting, error, logging, time, string and number-conversion
// packages. The call's package decides, never the literal's text.
func AddressCandidateCall(call atlas.SymbolCall) bool {
	pkg := ""
	if call.API != nil {
		pkg = call.API.Package
	} else if dot := strings.Index(call.Name, "."); dot > 0 {
		pkg = call.Name[:dot]
	}
	if slash := strings.LastIndex(pkg, "/"); slash >= 0 {
		pkg = pkg[slash+1:]
	}
	switch pkg = strings.ToLower(pkg); {
	case pkg == "fmt", pkg == "errors", pkg == "time", pkg == "strings", pkg == "strconv", pkg == "console", strings.HasPrefix(pkg, "log"):
		return false
	}
	return true
}

// FormatTemplate reports a literal that is a formatting template rather than
// an observed value: a % verb with optional flags, width and precision, as
// in "%s: %w", "node-%03d" or "%(name)s". A percent-encoded octet keeps a
// URL an address: %2F is two hex digits, and only a pair of decimal digits
// followed by a letter ("%03d") is read as a width and verb instead. The
// package rule catches the usual sources of templates; this one catches a
// template in a call the package rule keeps.
func FormatTemplate(value string) bool {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	isHex := func(c byte) bool { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
	isLetter := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			continue
		}
		rest := value[i+1:]
		switch {
		case rest == "":
			return false
		case rest[0] == '%':
			i++
			continue
		case len(rest) >= 2 && isHex(rest[0]) && isHex(rest[1]) && !(isDigit(rest[0]) && isDigit(rest[1]) && len(rest) >= 3 && isLetter(rest[2])):
			i += 2
			continue
		}
		j := 0
		if rest[0] == '(' {
			if j = strings.IndexByte(rest, ')'); j < 0 {
				continue
			}
			j++
		}
		for j < len(rest) && strings.IndexByte("-+# 0", rest[j]) >= 0 {
			j++
		}
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j < len(rest) && rest[j] == '.' {
			j++
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
		}
		if j < len(rest) && isLetter(rest[j]) {
			return true
		}
	}
	return false
}

func addressCandidateLiterals(call atlas.SymbolCall) bool {
	if !AddressCandidateCall(call) {
		return false
	}
	for _, value := range call.Values {
		if value != "" && !FormatTemplate(value) {
			return true
		}
	}
	return false
}

// BoundaryRow uses original callable observations, independent of its caption.
// The owner is shared by the window (context.owners) and named by owner_ref;
// the address catalogue appears only where an address decision is asked and
// candidates exist. Neighbouring boundary rows have no implied relationship.
func BoundaryRow(place atlas.Place, ownerRef string, addresses []BoundaryAddress, askAddress bool) table.Row {
	facts := place.Boundary
	fields := []table.Field{{Name: "path", Value: place.Path}, {Name: "line", Value: place.LineNo}, {Name: "caller", Value: facts.Caller}}
	if ownerRef == "" && facts.CallerDoc != "" {
		fields = append(fields, table.Field{Name: "caller_doc", Value: facts.CallerDoc})
	}
	fields = append(fields, table.Field{Name: "external", Value: facts.External})
	// An entry shows the words its registration wrote, to be named by; which
	// of them is a verb or a path is the model's reading, not a field.
	if words := EntryWords(place); len(words) > 0 {
		options := make([]string, 0, len(words))
		for _, word := range words {
			options = append(options, word.Ref)
		}
		fields = append(fields, table.Field{Name: "words", Value: words}, table.Field{Name: "word_options", Value: options})
	} else {
		if facts.Method != "" {
			fields = append(fields, table.Field{Name: "method", Value: facts.Method})
		}
		fields = append(fields, table.Field{Name: "values", Value: facts.Values})
	}
	fields = append(fields, table.Field{Name: "direction", Value: facts.Direction})
	if facts.GivenKind != "" {
		fields = append(fields, table.Field{Name: "kind_given", Value: facts.GivenKind})
	}
	if ownerRef != "" {
		fields = append(fields, table.Field{Name: "owner_ref", Value: ownerRef})
	}
	if askAddress && len(addresses) > 0 {
		options := []string{"unknown"}
		for _, address := range addresses {
			options = append(options, address.Ref)
		}
		fields = append(fields, table.Field{Name: "address_catalog", Value: addresses}, table.Field{Name: "address_options", Value: options})
	}
	return table.Row{ID: place.ID, Fields: fields}
}

// OwnerCallSpan is how far from a row's line an owner call still appears in
// the shared owner: the calls beside the candidate, not the whole body.
const OwnerCallSpan = 3

// BoundaryOwner is the window's one view of the declaration whose calls the
// rows are: the declaration, its calls within OwnerCallSpan lines of a row
// and every call whose literals are address candidates (a client constructor
// with its endpoint), its callable bindings, its owned declarations and the
// supplied source context. Morfeu repeated the owner's twelve calls, bindings
// and declarations in each of its rows: four rows spent 51% of a window on
// that repetition.
func BoundaryOwner(ref string, owner atlas.Place, rowLines []int, sourceContext []table.Field) map[string]any {
	evidence := EvidenceCatalog{OmitDefaults: true}
	decl := owner.Symbol.Decl
	calls := []any{}
	for _, call := range owner.Symbol.Calls {
		near := false
		for _, line := range rowLines {
			if call.Line >= line-OwnerCallSpan && call.Line <= line+OwnerCallSpan {
				near = true
				break
			}
		}
		if !near && !addressCandidateLiterals(call) {
			continue
		}
		calls = append(calls, evidence.CallWithOrigins(call))
	}
	value := map[string]any{"ref": ref, "path": owner.Path, "line": owner.LineNo, "name": decl.Name, "kind": decl.Kind,
		"signature": decl.Signature, "author_doc": decl.Doc, "call_span": OwnerCallSpan, "calls": calls,
		"callable_bindings": evidence.Bindings(owner.Symbol.Bindings), "owned_declarations": ownedDeclarations(owner.Symbol.Members)}
	for _, field := range evidence.Fields() {
		value[field.Name] = field.Value
	}
	for _, field := range sourceContext {
		value[field.Name] = field.Value
	}
	return value
}

// BoundaryOwnerContext is the context field carrying one window's owners.
func BoundaryOwnerContext(owners ...map[string]any) table.Field {
	return table.Field{Name: "owners", Value: owners}
}

// BoundarySourceContext supplies purpose clues from the same source graph.
// Parent and native caller identities are the only joins; names and nearby
// paths cannot attach another declaration or README. Caller context stops at
// that declaration, without expanding its calls or following its own callers.
// A call site keeps its receiver and argument origins, not its result value:
// the result trees of six Errorf alternatives explained nothing about the
// owner and filled the window.
func BoundarySourceContext(place, owner atlas.Place, places, declarations map[string]atlas.Place) []table.Field {
	context := make(map[string]any)
	file := places[place.Parent]
	if owner.Symbol != nil {
		file = places[owner.Parent]
	}
	if file.File != nil {
		context["file"] = map[string]any{"path": file.Path, "author_doc": file.File.Doc}
		var ancestors []map[string]any
		for parent := file.Parent; parent != ""; {
			directory, found := places[parent]
			if !found || directory.Directory == nil {
				break
			}
			if directory.Directory.Doc != "" || directory.Directory.Readme != "" {
				ancestors = append(ancestors, map[string]any{"path": directory.Path,
					"author_doc": directory.Directory.Doc, "readme_claim": directory.Directory.Readme})
			}
			parent = directory.Parent
		}
		if len(ancestors) > 0 {
			context["ancestor_directories"] = ancestors
		}
	}
	if owner.Symbol != nil {
		byID := make(map[string]map[string]any)
		for i, caller := range owner.Symbol.CalledBy {
			id := caller.ObjectID
			if caller.PlaceID != "" {
				id = caller.PlaceID
			}
			if id == "" {
				id = fmt.Sprintf("observation:%d", i)
			}
			row := byID[id]
			if row == nil {
				row = map[string]any{"name": caller.Name, "signature": caller.Signature, "path": caller.Path}
				if declaration, found := declarations[id]; found && declaration.Symbol != nil {
					row["author_doc"] = declaration.Symbol.Decl.Doc
					row["declaration_line"] = declaration.LineNo
					evidence := EvidenceCatalog{OmitDefaults: true}
					row["callable_bindings"] = evidence.Bindings(declaration.Symbol.Bindings)
					for _, field := range evidence.Fields() {
						row[field.Name] = field.Value
					}
				}
				byID[id] = row
			}
			sites, _ := row["call_sites"].([]map[string]any)
			var matched []map[string]any
			if declaration := declarations[id]; declaration.Symbol != nil {
				for _, call := range declaration.Symbol.Calls {
					if call.Line != caller.Line || call.Kind != caller.Kind || call.Invocation != caller.Invocation || call.Dispatch != caller.Dispatch || call.Resolution != caller.Resolution || !slices.Contains(call.CalleeIDs, owner.ID) {
						continue
					}
					evidence := EvidenceCatalog{OmitDefaults: true}
					site := call
					site.ResultValue = nil
					entry := map[string]any{"call": evidence.CallWithOrigins(site)}
					for _, field := range evidence.Fields() {
						entry[field.Name] = field.Value
					}
					matched = append(matched, entry)
				}
			}
			if len(matched) == 0 {
				matched = append(matched, map[string]any{"line": caller.Line, "kind": caller.Kind,
					"invocation": caller.Invocation, "dispatch": caller.Dispatch, "resolution": caller.Resolution})
			}
			row["call_sites"] = append(sites, matched...)
		}
		ids := make([]string, 0, len(byID))
		for id := range byID {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var callers []map[string]any
		for _, id := range ids {
			callers = append(callers, byID[id])
		}
		if len(callers) > 0 {
			context["immediate_callers"] = callers
		}
	}
	if len(context) == 0 {
		return nil
	}
	return []table.Field{{Name: "source_context", Value: context}}
}

// BoxSummary describes the endpoints of an architecture connection.
type BoxSummary struct {
	ID    string
	Title string
	Line  string
	Files int
}

// Arrows is the arrow table.
func Arrows() table.Definition {
	return table.Definition{
		Stage: StageArrows, Contract: arrowsContract, Window: WindowRows,
		System:  arrowsPrompt,
		Columns: []table.Column{{Name: "sentence", Kind: table.Text, MaxRunes: LineRunes, Note: "what the first box does with the second"}},
	}
}

// ArrowRow builds the row of one drawn arrow.
func ArrowRow(id string, from, to BoxSummary, witnesses []atlas.Witness, calls int) table.Row {
	pairs := make([]string, 0, min(len(witnesses), maxWitnesses))
	for _, witness := range witnesses {
		pairs = append(pairs, witness.Caller+" "+witnessVerb(witness.Kind)+" "+witness.Callee)
		if len(pairs) == maxWitnesses {
			break
		}
	}
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "from", Value: from.Title + ": " + from.Line},
		{Name: "to", Value: to.Title + ": " + to.Line},
		{Name: "witnesses", Value: pairs},
		{Name: "calls", Value: calls},
	}}
}

// FallbackSentence is the arrow's sentence when the model has not spoken. It
// names the callees of the witnesses, most observed first, each once: three
// callers of addReply make one addReply, and the next name takes the place.
func FallbackSentence(from, to BoxSummary, witnesses []atlas.Witness) string {
	names := make([]string, 0, maxWitnesses)
	for _, witness := range witnesses {
		if slices.Contains(names, witness.Callee) {
			continue
		}
		names = append(names, witness.Callee)
		if len(names) == maxWitnesses {
			break
		}
	}
	if len(names) == 0 {
		return from.Title + " uses " + to.Title + "."
	}
	return from.Title + " calls " + to.Title + ": " + strings.Join(names, ", ") + "."
}

// Targets is the portfolio table.
func Targets(rolesBound ...bool) table.Definition {
	definition := table.Definition{
		Stage: StageTargets, Contract: targetsContract, Window: WindowRows,
		System: targetsPrompt,
		Columns: []table.Column{
			// Each cell fails alone: a target keeps its fallback role or an
			// empty line for a refused one.
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what this target is and does", Alone: true},
			{Name: "role", Kind: table.Choice, Options: atlas.Roles(), Alone: true},
		},
	}
	if len(rolesBound) > 0 && rolesBound[0] {
		definition.Contract += ".selected-role-v1"
		definition.Columns = definition.Columns[:1]
		definition.System = targetDescriptionsPrompt
	}
	return definition
}

// TargetSummary is what a portfolio row says about a target.
type TargetSummary struct {
	SelectedRole string
	ID           string
	Name         string
	Root         string
	Language     string
	Kind         string
	Readme       string
	Entrypoint   string
	Files        int
	Dirs         int
	Boundaries   map[string]int
	Operations   []string
	Parts        []string
}

// TargetRow builds the row of one target.
func TargetRow(target TargetSummary) table.Row {
	fields := []table.Field{
		{Name: "name", Value: target.Name},
		{Name: "root", Value: target.Root},
		{Name: "language", Value: target.Language},
		{Name: "kind", Value: target.Kind},
	}
	if target.SelectedRole != "" {
		fields = append(fields, table.Field{Name: "selected_role", Value: target.SelectedRole})
	}
	if target.Readme != "" {
		fields = append(fields, table.Field{Name: "readme", Value: target.Readme})
	}
	if target.Entrypoint != "" {
		fields = append(fields, table.Field{Name: "entrypoint", Value: target.Entrypoint})
	}
	fields = append(fields,
		table.Field{Name: "files", Value: target.Files},
		table.Field{Name: "dirs", Value: target.Dirs},
	)
	if len(target.Operations) > 0 {
		fields = append(fields, table.Field{Name: "operation_hypotheses", Value: target.Operations})
	}
	if len(target.Parts) > 0 {
		fields = append(fields, table.Field{Name: "responsibility_hypotheses", Value: target.Parts})
	}
	if len(target.Boundaries) > 0 {
		kinds := make([]string, 0, len(target.Boundaries))
		for kind := range target.Boundaries {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		counts := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			counts = append(counts, fmt.Sprintf("%s %d", kind, target.Boundaries[kind]))
		}
		fields = append(fields, table.Field{Name: "boundaries", Value: counts})
	}
	return table.Row{ID: target.ID, Fields: fields}
}

// FallbackRole preserves the native program kind while semantic roles await
// selection. A source directory cannot classify a fixture, tool or example.
func FallbackRole(kind string) string {
	if strings.Contains(kind, "library") {
		return atlas.RoleLibrary
	}
	return atlas.RoleProduct
}

// Joints is the joint table; Peers the blind form of it.
func Joints() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".joints", Window: WindowRows,
		System: jointsPrompt,
		Columns: []table.Column{
			{Name: "same", Kind: table.Choice, Options: []string{"yes", "no"}},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Missing: "-", Note: "at most six words, or - when same is no"},
		},
	}
}

func Peers() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".peers", Window: WindowRows,
		System: jointsPrompt,
		Columns: []table.Column{
			{Name: "peer", Kind: table.Choice, OptionsFrom: "peer_options", Note: "a ref from context.peers, or none"},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Missing: "-", Note: "at most six words, or - when peer is none"},
		},
	}
}

// BoundarySide is one side of a joint as the model sees it.
type BoundarySide struct {
	Target    string
	Line      string
	Path      string
	Caller    string
	Source    string
	External  string
	Method    string
	Values    []string
	Signature string
	CallerDoc string
}

func sideValue(side BoundarySide) map[string]any {
	value := map[string]any{"target": side.Target, "line": side.Line, "values": bounded(side.Values, maxValues)}
	if side.Path != "" {
		value["path"], value["caller"], value["source"] = side.Path, side.Caller, side.Source
	}
	if side.External != "" {
		value["external"] = side.External
	}
	if side.Signature != "" {
		value["caller_signature"] = side.Signature
	}
	if side.CallerDoc != "" {
		value["caller_doc"] = side.CallerDoc
	}
	if side.Method != "" {
		value["method"] = side.Method
	}
	return value
}

// JointRow builds the row of one candidate joint.
func JointRow(id, value string, a, b BoundarySide) table.Row {
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "value", Value: value},
		{Name: "a", Value: sideValue(a)},
		{Name: "b", Value: sideValue(b)},
	}}
}

// PeerRow builds the row of one blind outgoing boundary; refs are the
// window's peer refs.
func PeerRow(id string, side BoundarySide, refs []string) table.Row {
	options := append([]string{PeerNone}, refs...)
	return table.Row{ID: id, Fields: []table.Field{
		{Name: "a", Value: sideValue(side)},
		{Name: "peer_options", Value: options},
	}}
}

// PeerContext is the closed list of incoming boundaries a peers window
// chooses from, as the context field.
func PeerContext(refs []string, sides []BoundarySide) table.Field {
	peers := make([]map[string]any, 0, len(refs))
	for i, ref := range refs {
		peer := sideValue(sides[i])
		peer["ref"] = ref
		peers = append(peers, peer)
	}
	return table.Field{Name: "peers", Value: peers}
}

// witnessVerb says what one declaration does to another, by the relation the
// graph recorded: a call, a callback handed over, an implementation supplied.
func witnessVerb(kind string) string {
	switch kind {
	case "passes_callback":
		return "passes as a callback"
	case "binds_implementation":
		return "supplies as the implementation of"
	case "decorates":
		return "is decorated by"
	case "executes":
		return "runs"
	case "imports":
		return "imports"
	default:
		return "calls"
	}
}
