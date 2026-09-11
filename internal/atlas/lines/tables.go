package lines

import (
	_ "embed"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/destinations"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const (
	StageSymbols    = "atlas_symbols"
	StageBoundaries = "atlas_boundaries"
	StageZones      = "atlas_zones"
	StageArrows     = "atlas_arrows"
	StageTargets    = "atlas_targets"
	StageJoints     = "atlas_joints"

	symbolsContract    = "repomap.atlas.symbols.v7"
	boundariesContract = "repomap.atlas.boundaries.v7"
	zonesContract      = "repomap.atlas.zones.v2"
	arrowsContract     = "repomap.atlas.arrows.v1"
	targetsContract    = "repomap.atlas.targets.v2"
	jointsContract     = "repomap.atlas.joints.v3"

	// ShortLineRunes bounds the lines of boundaries and symbols; LabelRunes
	// the label of a joint.
	ShortLineRunes = 120
	LabelRunes     = 40

	// PeerNone is the peer choice that says no listed boundary is the
	// counterpart.
	PeerNone = "none"

	maxWitnesses = 3
	maxZoneBoxes = 8
	maxValues    = 8
)

//go:embed prompts/symbols.md
var symbolsPrompt string

//go:embed prompts/types.md
var typesPrompt string

//go:embed prompts/boundaries.md
var boundariesPrompt string

//go:embed prompts/fixed_boundaries.md
var fixedBoundariesPrompt string

//go:embed prompts/zones.md
var zonesPrompt string

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

// Symbols explains the selected declarations displayed in the overview.
func Symbols() table.Definition {
	return table.Definition{
		Stage: StageSymbols, Contract: symbolsContract,
		System: symbolsPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, Note: "one sentence, what this declaration does or is"},
			{Name: "alias", Kind: table.Text, MaxRunes: LabelRunes, EmptyValue: "none", Note: "short English reader label grounded in this declaration; none when its original name is already clear"},
		},
	}
}

// Types describes the concept represented by a type using its own interface.
// It shares symbol knowledge and publication; it does not classify operations.
func Types() table.Definition {
	return table.Definition{
		Stage: StageSymbols, Contract: "repomap.atlas.types.v7",
		System: typesPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Prose, Note: "briefly explain what this represents or controls and any consequential documented rule, preserving its conditions; no method inventory or invented effects"},
			{Name: "alias", Kind: table.Text, MaxRunes: LabelRunes, EmptyValue: "none", Note: "short English reader label grounded in this declaration; none when its original name is already clear"},
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
		members = append(members, map[string]any{"path": member.Path, "line": member.Decl.LineNo,
			"name": member.Decl.Name, "kind": member.Decl.Kind, "signature": member.Decl.Signature, "author_doc": member.Decl.Doc})
	}
	return members
}

// SymbolRow builds the row of one candidate symbol.
func SymbolRow(place atlas.Place, fileLine string) table.Row {
	var evidence EvidenceCatalog
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
	calls := make([]map[string]any, 0, len(place.Symbol.Calls))
	refs := make([]string, 0, len(place.Symbol.Calls))
	for i, call := range place.Symbol.Calls {
		ref := fmt.Sprintf("c%d", i+1)
		// A complete exact repository callee is internal delegation at this
		// site. Keep the call as context and retain its original c* position;
		// possible or unresolved dispatch is still eligible for review.
		if call.Kind != "calls" || call.Resolution != "exact" || len(call.CalleeIDs) != 1 || call.API != nil {
			refs = append(refs, ref)
		}
		calls = append(calls, map[string]any{"ref": ref, "evidence": evidence.Call(call)})
	}
	fields = append(fields, table.Field{Name: "calls", Value: calls}, table.Field{Name: "call_options", Value: refs}, table.Field{Name: "call_count", Value: len(refs)})
	fields = append(fields, evidence.Fields()...)
	return table.Row{ID: place.ID, Fields: fields}
}

// Boundaries interprets candidate relationships whose role is not a native
// fact. Outgoing mode adds the runtime-system cells. The decision and kind
// lists are column options: every row chooses from the same list, so no row
// repeats it, and an outgoing candidate is offered only the kinds the group
// index keeps as communication.
func Boundaries(outgoing ...bool) table.Definition {
	positive := map[string]string{"decision": "boundary"}
	def := table.Definition{
		Stage: StageBoundaries, Contract: boundariesContract,
		System: boundariesPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{
			{Name: "decision", Kind: table.Choice, Options: []string{"boundary", "none", "unassessed"}, Note: "boundary when this call itself dispatches an exchange or creates/configures the actual remote client instance; none for local helpers, options and preparation; unassessed for insufficient evidence"},
			{Name: "kind", Kind: table.Choice, Options: atlas.BoundaryKinds(), When: positive},
			{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes, When: positive, Note: "at most ten words, no subject: why this component exchanges with the runtime system"},
		},
	}
	if len(outgoing) > 0 && outgoing[0] {
		def.Contract += ".outbound"
		def.Columns[1].Options = atlas.OutgoingBoundaryKinds()
		def.Columns = append(def.Columns,
			destinationColumn(positive),
			table.Column{Name: "basis", Kind: table.Choice, Options: []string{"dispatch", "remote_client_instance"}, When: positive, Note: "dispatch: this call sends the exchange; remote_client_instance: this call itself creates or configures the actual remote client/exporter instance, not an option for a later constructor"},
			addressColumn(positive),
		)
	}
	return def
}

// FixedBoundaries explains an existing native observation. Its existence and
// kind are input facts, not mandatory one-option model decisions, and the
// short prompt asks for the line alone. An outgoing HTTP fact whose address
// the code does not know may still need a destination and an address choice.
func FixedBoundaries(outgoing bool) table.Definition {
	def := table.Definition{
		Stage: StageBoundaries, Contract: boundariesContract + ".fixed",
		System: fixedBoundariesPrompt, Independent: true, Memoize: true,
		Columns: []table.Column{{Name: "line", Kind: table.Text, MaxRunes: ShortLineRunes,
			Note: "at most ten words, no subject: what this native observation reads, receives or sends; a configuration read is not itself a remote exchange"}},
	}
	if outgoing {
		def.Contract += ".outbound"
		def.Columns = append(def.Columns, destinationColumn(nil), addressColumn(nil))
	}
	return def
}

// DestinationOther prefixes a destination the catalogue does not list.
const DestinationOther = "other: "

func destinationColumn(when map[string]string) table.Column {
	return table.Column{Name: "destination", Kind: table.Choice, OptionsFrom: "destination_options", Free: DestinationOther, FreeMaxRunes: LabelRunes, When: when,
		Note: "one d* ref from context.destination_catalog, or other: followed by the runtime system's short name; never a host, URL or address"}
}

// addressColumn is asked only where the row carries address candidates: a
// row whose address the code already knows, or that has no candidate
// literal, has no address decision.
func addressColumn(when map[string]string) table.Column {
	return table.Column{Name: "address", Kind: table.Choice, OptionsFrom: "address_options", WhenOptionsFrom: "address_options", When: when,
		Note: "one supplied a* address value, or unknown when no observed value identifies the destination"}
}

// DestinationFields are the window's closed list of runtime systems.
func DestinationFields(catalog []destinations.Entry) []table.Field {
	return []table.Field{{Name: "destination_catalog", Value: catalog}, {Name: "destination_options", Value: destinations.Refs(catalog)}}
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
	if facts.Method != "" {
		fields = append(fields, table.Field{Name: "method", Value: facts.Method})
	}
	fields = append(fields, table.Field{Name: "values", Value: facts.Values}, table.Field{Name: "direction", Value: facts.Direction})
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
	var evidence EvidenceCatalog
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
					var evidence EvidenceCatalog
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
					if call.Line != caller.Line || call.Kind != caller.Kind || call.Invocation != caller.Invocation || call.Resolution != caller.Resolution || !slices.Contains(call.CalleeIDs, owner.ID) {
						continue
					}
					var evidence EvidenceCatalog
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
					"invocation": caller.Invocation, "resolution": caller.Resolution})
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

// ZoneNames asks for exactly want part names of one target in one row: a
// cell per part, so the answer cannot be one name per box. ZoneAssign then
// places every top box against that closed list; ZoneLines gives each part
// its sentence. All three share one prompt and one stage.
func ZoneNames(want int) table.Definition {
	columns := make([]table.Column, 0, want)
	for i := 1; i <= want; i++ {
		columns = append(columns, table.Column{
			Name: fmt.Sprintf("part_%d", i), Kind: table.Text, MaxRunes: TitleRunes,
			Note: "the name of one part, two to four words, distinct from the others",
		})
	}
	return table.Definition{
		Stage: StageZones, Contract: fmt.Sprintf("%s.names.%d", zonesContract, want), Window: 1,
		System: zonesPrompt, Columns: columns,
	}
}

// ZoneNamesRow is the one row of the names question: the target's largest
// boxes, each with its title, line and size.
func ZoneNamesRow(targetID string, boxes []BoxSummary) table.Row {
	entries := make([]string, 0, len(boxes))
	for _, box := range boxes {
		entries = append(entries, fmt.Sprintf("%s: %s (%s)", box.Title, box.Line, FileCount(box.Files)))
	}
	return table.Row{ID: targetID, Fields: []table.Field{{Name: "boxes", Value: entries}}}
}

// FileCount spells a number of files: "1 file", "12 files".
func FileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// PartName reads the i-th part cell of a names answer.
func PartName(answer map[string]string, i int) string {
	return answer[fmt.Sprintf("part_%d", i+1)]
}

func ZoneAssign(parts []string) table.Definition {
	return table.Definition{
		Stage: StageZones, Contract: zonesContract + ".assign", Window: WindowRows,
		System: zonesPrompt, Independent: true,
		Columns: []table.Column{{Name: "part", Kind: table.Choice, Options: parts, Note: "one of context.parts"}},
	}
}

func ZoneLines() table.Definition {
	return table.Definition{
		Stage: StageZones, Contract: zonesContract + ".lines", Window: WindowRows,
		System: zonesPrompt, Independent: true,
		Columns: []table.Column{{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what this part does"}},
	}
}

// BoxSummary is what a zone row says about a box.
type BoxSummary struct {
	ID    string
	Title string
	Line  string
	Files int
}

// ZoneBoxRow is a row of the names or assign question.
func ZoneBoxRow(box BoxSummary) table.Row {
	return table.Row{ID: box.ID, Fields: []table.Field{
		{Name: "title", Value: box.Title},
		{Name: "line", Value: box.Line},
		{Name: "files", Value: box.Files},
	}}
}

// ZoneLineRow is a row of the lines question.
func ZoneLineRow(zoneID, title string, boxes []BoxSummary) table.Row {
	titles := make([]string, 0, min(len(boxes), maxZoneBoxes))
	files := 0
	for _, box := range boxes {
		files += box.Files
		if len(titles) < maxZoneBoxes {
			titles = append(titles, box.Title)
		}
	}
	return table.Row{ID: zoneID, Fields: []table.Field{
		{Name: "part", Value: title},
		{Name: "boxes", Value: titles},
		{Name: "files", Value: files},
	}}
}

// WantZones says how many parts a target of n top boxes should have: the
// square root, held between four and eight.
func WantZones(n int) int {
	want := 0
	for want*want < n {
		want++
	}
	if want < 4 {
		want = 4
	}
	if want > 8 {
		want = 8
	}
	if want > n {
		want = n
	}
	return want
}

// Arrows is the arrow table.
func Arrows() table.Definition {
	return table.Definition{
		Stage: StageArrows, Contract: arrowsContract, Window: WindowRows,
		System: arrowsPrompt, Independent: true,
		Columns: []table.Column{{Name: "sentence", Kind: table.Text, MaxRunes: LineRunes, Note: "what the first box does with the second"}},
	}
}

// ArrowRow builds the row of one drawn arrow.
func ArrowRow(id string, from, to BoxSummary, witnesses []atlas.Witness, calls int) table.Row {
	pairs := make([]string, 0, min(len(witnesses), maxWitnesses))
	for _, witness := range witnesses {
		pairs = append(pairs, witness.Caller+" calls "+witness.Callee)
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

// FallbackSentence is the arrow's sentence when the model has not spoken.
func FallbackSentence(from, to BoxSummary, witnesses []atlas.Witness) string {
	names := make([]string, 0, maxWitnesses)
	for _, witness := range witnesses {
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
		System: targetsPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "line", Kind: table.Text, MaxRunes: LineRunes, Note: "one sentence, what this target is and does"},
			{Name: "role", Kind: table.Choice, Options: atlas.Roles()},
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

// FallbackRole guesses a target's role from its path and kind when the
// model has not spoken.
func FallbackRole(root, kind string) string {
	lower := strings.ToLower(root)
	for _, marker := range []string{"testdata", "fixture", "fixtures", "test", "tests"} {
		if strings.HasPrefix(lower, marker+"/") || strings.Contains(lower, "/"+marker+"/") || lower == marker {
			return atlas.RoleFixture
		}
	}
	for _, marker := range []string{"example", "examples", "samples", "demo"} {
		if strings.HasPrefix(lower, marker+"/") || strings.Contains(lower, "/"+marker+"/") || lower == marker {
			return atlas.RoleExample
		}
	}
	if strings.Contains(kind, "library") {
		return atlas.RoleLibrary
	}
	return atlas.RoleProduct
}

// Joints is the joint table; Peers the blind form of it.
func Joints() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".joints", Window: WindowRows,
		System: jointsPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "same", Kind: table.Choice, Options: []string{"yes", "no"}},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Note: "at most six words, or - when same is no"},
		},
	}
}

func Peers() table.Definition {
	return table.Definition{
		Stage: StageJoints, Contract: jointsContract + ".peers", Window: WindowRows,
		System: jointsPrompt, Independent: true,
		Columns: []table.Column{
			{Name: "peer", Kind: table.Choice, OptionsFrom: "peer_options", Note: "a ref from context.peers, or none"},
			{Name: "label", Kind: table.Text, MaxRunes: LabelRunes, Note: "at most six words, or - when peer is none"},
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
