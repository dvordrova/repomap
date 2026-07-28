package semanticmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"
	"testing"
)

const (
	maxStructureSidecarBytes = 32 << 10
	maxStructureSymbols      = 24
	maxStructureCalls        = 16
	maxIntersectingSymbols   = 4
	maxUnresolvedCalls       = 4
	maxStructureScalarBytes  = 240
	maxStructureSymbolLines  = 4096
)

type structurePacket struct {
	CaseID              string                 `json:"case_id"`
	Repository          repository             `json:"repository"`
	Question            string                 `json:"question"`
	SourceSlices        []structureSourceSlice `json:"source_slices"`
	StructureProvenance structureProvenance    `json:"structure_provenance"`
	Symbols             []structureSymbol      `json:"symbols"`
	StaticCalls         []structureCall        `json:"static_calls"`
	UnresolvedCalls     []unresolvedCall       `json:"unresolved_calls"`
}

type structureSourceSlice struct {
	Path                  string   `json:"path"`
	StartLine             int      `json:"start_line"`
	EndLine               int      `json:"end_line"`
	EnclosingSymbolID     string   `json:"enclosing_symbol_id"`
	IntersectingSymbolIDs []string `json:"intersecting_symbol_ids"`
	ScopeCoverage         string   `json:"scope_coverage"`
	Text                  string   `json:"text"`
}

type structureProvenance struct {
	Analyzer             string   `json:"analyzer"`
	Version              string   `json:"version"`
	CollectorVersion     string   `json:"collector_version"`
	AdapterIdentity      string   `json:"adapter_identity"`
	Operations           []string `json:"operations"`
	RepositoryRevision   string   `json:"repository_revision"`
	Scenario             string   `json:"scenario"`
	SymbolCoverage       string   `json:"symbol_coverage"`
	CallCoverage         string   `json:"call_coverage"`
	SyntheticScopePolicy string   `json:"synthetic_scope_policy"`
}

type structureSymbol struct {
	ID                        string `json:"id"`
	Path                      string `json:"path"`
	Kind                      string `json:"kind"`
	Name                      string `json:"name"`
	StartLine                 int    `json:"start_line"`
	StartColumn               int    `json:"start_column"`
	EndLine                   int    `json:"end_line"`
	EndColumn                 int    `json:"end_column"`
	ParentSymbolID            string `json:"parent_symbol_id"`
	ContainmentParentSymbolID string `json:"containment_parent_symbol_id"`
}

type structureCall struct {
	CallerSymbolID string `json:"caller_symbol_id"`
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	StartColumn    int    `json:"start_column"`
	EndLine        int    `json:"end_line"`
	EndColumn      int    `json:"end_column"`
	CalleeSymbolID string `json:"callee_symbol_id"`
}

type unresolvedCall struct {
	FactID         string `json:"fact_id"`
	CallerSymbolID string `json:"caller_symbol_id"`
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	EndLine        int    `json:"end_line"`
	Expression     string `json:"expression"`
	Reason         string `json:"reason"`
}

type structureProjection struct {
	CaseID              string              `json:"case_id"`
	Repository          repository          `json:"repository"`
	StructureProvenance structureProvenance `json:"structure_provenance"`
	Symbols             []structureSymbol   `json:"symbols"`
	StaticCalls         []structureCall     `json:"static_calls"`
	UnresolvedCalls     []unresolvedCall    `json:"unresolved_calls"`
}

var sourceTextHashes = map[string]string{
	"caddy": "9a9ad33a17fc0de599c2ab1fd1d0fcae575a93f0fe7035f8fd5c430e27e825eb",
	"beets": "7760583b8899c26ba68b7c70d50352edcfc2814ab99dc0c5854911cdf364b21e",
}

// UnmarshalJSON keeps the older sourcePacket test view strict while allowing
// the explicitly versioned structure metadata carried by these two packets.
func (packet *sourcePacket) UnmarshalJSON(data []byte) error {
	var wire structurePacket
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}

	slices := make([]sourceSlice, len(wire.SourceSlices))
	for i, sourceSlice := range wire.SourceSlices {
		slices[i] = sourceSlice.base()
	}
	*packet = sourcePacket{
		CaseID:       wire.CaseID,
		Repository:   wire.Repository,
		Question:     wire.Question,
		SourceSlices: slices,
	}
	return nil
}

func (value structureSourceSlice) base() sourceSlice {
	return sourceSlice{
		Path:      value.Path,
		StartLine: value.StartLine,
		EndLine:   value.EndLine,
		Text:      value.Text,
	}
}

func TestRecordedAdapterStructureReproducesDeterministically(t *testing.T) {
	for _, name := range []string{"caddy", "beets"} {
		t.Run(name, func(t *testing.T) {
			packetBytes := readBoundedFile(t, name+".source-slices.json", maxSourcePacketBytes)
			sidecarBytes := readBoundedFile(t, name+".adapter-structure.json", maxStructureSidecarBytes)
			packet := decodeStrict[structurePacket](t, packetBytes)
			sidecar := decodeStrict[structureProjection](t, sidecarBytes)

			validateStructurePacket(t, name, packet)
			projected := projectStructure(packet)
			if !reflect.DeepEqual(sidecar, projected) {
				t.Fatalf("recorded structure differs from packet projection")
			}

			first := encodeStructure(t, projected)
			second := encodeStructure(t, projectStructure(packet))
			if !bytes.Equal(first, second) {
				t.Fatal("structure projection is not byte-identical across runs")
			}
			if !bytes.Equal(sidecarBytes, first) {
				t.Fatalf("%s sidecar differs from deterministic encoding", name)
			}
		})
	}
}

func validateStructurePacket(t *testing.T, name string, packet structurePacket) {
	t.Helper()
	baseSlices := make([]sourceSlice, len(packet.SourceSlices))
	hash := sha256.New()
	for i, sourceSlice := range packet.SourceSlices {
		baseSlices[i] = sourceSlice.base()
		_, _ = hash.Write([]byte(sourceSlice.Text))
	}
	validateSourcePacket(t, sourcePacket{
		CaseID:       packet.CaseID,
		Repository:   packet.Repository,
		Question:     packet.Question,
		SourceSlices: baseSlices,
	})
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != sourceTextHashes[name] {
		t.Fatalf("source text hash = %s, want %s", got, sourceTextHashes[name])
	}

	provenance := packet.StructureProvenance
	validateText(t, "structure_provenance.analyzer", provenance.Analyzer, maxStructureScalarBytes)
	validateText(t, "structure_provenance.version", provenance.Version, maxStructureScalarBytes)
	if provenance.CollectorVersion != "semantic-map-structure-v1" {
		t.Fatalf("collector version = %q", provenance.CollectorVersion)
	}
	validateText(t, "structure_provenance.adapter_identity", provenance.AdapterIdentity, maxStructureScalarBytes)
	validateText(t, "structure_provenance.repository_revision", provenance.RepositoryRevision, maxStructureScalarBytes)
	validateText(t, "structure_provenance.scenario", provenance.Scenario, maxStructureScalarBytes)
	if provenance.RepositoryRevision != packet.Repository.Revision {
		t.Fatal("structure provenance revision does not match packet revision")
	}
	if len(provenance.Operations) == 0 || len(provenance.Operations) > 4 {
		t.Fatalf("structure provenance operations = %d, want 1..4", len(provenance.Operations))
	}
	for i, operation := range provenance.Operations {
		validateText(t, fmt.Sprintf("structure_provenance.operations[%d]", i), operation, maxStructureScalarBytes)
	}
	if provenance.SymbolCoverage != "retained_scopes_only" {
		t.Fatalf("symbol coverage = %q", provenance.SymbolCoverage)
	}
	if provenance.CallCoverage != "selected_symbol_targets_only" {
		t.Fatalf("call coverage = %q", provenance.CallCoverage)
	}

	symbols := validateStructureSymbols(t, packet.Symbols)
	syntheticScopes := 0
	for _, symbol := range packet.Symbols {
		if symbol.Kind == "synthetic_scope" {
			syntheticScopes++
		}
	}
	switch provenance.SyntheticScopePolicy {
	case "none":
		if syntheticScopes != 0 {
			t.Fatalf("synthetic scope policy is none, found %d", syntheticScopes)
		}
	case "anonymous_cross-slice_container":
		if syntheticScopes == 0 {
			t.Fatal("synthetic scope policy requires an anonymous container")
		}
	default:
		t.Fatalf("synthetic scope policy = %q", provenance.SyntheticScopePolicy)
	}
	validateStructureSlices(t, packet.SourceSlices, packet.Symbols, symbols)
	validateStructureCalls(t, packet.StaticCalls, symbols, baseSlices)
	validateUnresolvedCalls(t, packet.UnresolvedCalls, symbols, baseSlices)
	validateUnresolvedFactReferences(t, name, packet.UnresolvedCalls)
	validateRequiredStructure(t, name, packet)
}

func validateStructureSymbols(t *testing.T, values []structureSymbol) map[string]structureSymbol {
	t.Helper()
	if len(values) == 0 || len(values) > maxStructureSymbols {
		t.Fatalf("symbols count = %d, want 1..%d", len(values), maxStructureSymbols)
	}
	known := make(map[string]structureSymbol, len(values))
	previous := ""
	for i, symbol := range values {
		prefix := fmt.Sprintf("symbols[%d]", i)
		validateText(t, prefix+".id", symbol.ID, maxStructureScalarBytes)
		validateStructureSymbolRange(t, prefix, symbol)
		if symbol.StartColumn < 1 || symbol.EndColumn < 1 ||
			(symbol.EndLine == symbol.StartLine && symbol.EndColumn <= symbol.StartColumn) {
			t.Fatalf("%s has invalid end-exclusive columns", prefix)
		}
		switch symbol.Kind {
		case "function", "method", "class":
			validateText(t, prefix+".name", symbol.Name, maxStructureScalarBytes)
		case "synthetic_scope":
			if symbol.Name != "" || symbol.ParentSymbolID != "" || symbol.ContainmentParentSymbolID != "" {
				t.Fatalf("%s synthetic scope must be anonymous and root-level", prefix)
			}
		default:
			t.Fatalf("%s.kind = %q", prefix, symbol.Kind)
		}
		if _, duplicate := known[symbol.ID]; duplicate {
			t.Fatalf("duplicate symbol id %q", symbol.ID)
		}
		known[symbol.ID] = symbol

		orderKey := fmt.Sprintf("%s\x00%09d\x00%09d\x00%s", symbol.Path, symbol.StartLine, symbol.StartColumn, symbol.ID)
		if i > 0 && orderKey < previous {
			t.Fatalf("%s is not deterministically sorted", prefix)
		}
		previous = orderKey
	}

	for i, symbol := range values {
		for field, parentID := range map[string]string{
			"parent_symbol_id":             symbol.ParentSymbolID,
			"containment_parent_symbol_id": symbol.ContainmentParentSymbolID,
		} {
			if parentID == "" {
				continue
			}
			parent, ok := known[parentID]
			if !ok {
				t.Fatalf("symbols[%d].%s = %q, want known symbol", i, field, parentID)
			}
			if parent.Path != symbol.Path || !rangeContains(parent, symbol.StartLine, symbol.EndLine) {
				t.Fatalf("symbols[%d].%s does not geometrically contain child", i, field)
			}
		}
	}
	return known
}

func validateStructureSymbolRange(t *testing.T, field string, symbol structureSymbol) {
	t.Helper()
	validateText(t, field+".path", symbol.Path, maxPathBytes)
	if path.IsAbs(symbol.Path) ||
		path.Clean(symbol.Path) != symbol.Path ||
		symbol.Path == "." ||
		strings.HasPrefix(symbol.Path, "../") ||
		strings.Contains(symbol.Path, `\`) ||
		strings.Contains(symbol.Path, ":") {
		t.Fatalf("%s.path = %q, want canonical repository-relative path", field, symbol.Path)
	}
	if symbol.StartLine < 1 ||
		symbol.EndLine < symbol.StartLine ||
		symbol.EndLine-symbol.StartLine+1 > maxStructureSymbolLines {
		t.Fatalf(
			"%s lines = %d..%d, want a positive range within %d lines",
			field,
			symbol.StartLine,
			symbol.EndLine,
			maxStructureSymbolLines,
		)
	}
}

func validateStructureSlices(
	t *testing.T,
	slices []structureSourceSlice,
	symbolValues []structureSymbol,
	symbols map[string]structureSymbol,
) {
	t.Helper()
	syntheticUses := make(map[string]int)
	for i, sourceSlice := range slices {
		prefix := fmt.Sprintf("source_slices[%d]", i)
		enclosing, ok := symbols[sourceSlice.EnclosingSymbolID]
		if !ok {
			t.Fatalf("%s.enclosing_symbol_id = %q, want known symbol", prefix, sourceSlice.EnclosingSymbolID)
		}
		if enclosing.Path != sourceSlice.Path ||
			!rangeContains(enclosing, sourceSlice.StartLine, sourceSlice.EndLine) {
			t.Fatalf("%s enclosing symbol does not contain the full source slice", prefix)
		}
		if enclosing.Kind == "synthetic_scope" {
			syntheticUses[enclosing.ID]++
			lines := strings.Split(strings.TrimSuffix(sourceSlice.Text, "\n"), "\n")
			if sourceSlice.ScopeCoverage != "partial" ||
				enclosing.StartLine != sourceSlice.StartLine ||
				enclosing.StartColumn != 1 ||
				enclosing.EndLine != sourceSlice.EndLine ||
				enclosing.EndColumn != len(lines[len(lines)-1])+1 {
				t.Fatalf("%s synthetic container must exactly wrap one partial source slice", prefix)
			}
		}
		if len(sourceSlice.IntersectingSymbolIDs) == 0 ||
			len(sourceSlice.IntersectingSymbolIDs) > maxIntersectingSymbols {
			t.Fatalf("%s intersecting symbols = %d, want 1..%d", prefix, len(sourceSlice.IntersectingSymbolIDs), maxIntersectingSymbols)
		}
		seen := make(map[string]struct{}, len(sourceSlice.IntersectingSymbolIDs))
		for j, symbolID := range sourceSlice.IntersectingSymbolIDs {
			symbol, ok := symbols[symbolID]
			if !ok {
				t.Fatalf("%s.intersecting_symbol_ids[%d] = %q, want known symbol", prefix, j, symbolID)
			}
			if _, duplicate := seen[symbolID]; duplicate {
				t.Fatalf("%s repeats intersecting symbol %q", prefix, symbolID)
			}
			seen[symbolID] = struct{}{}
			if symbol.Path != sourceSlice.Path ||
				symbol.EndLine < sourceSlice.StartLine ||
				symbol.StartLine > sourceSlice.EndLine {
				t.Fatalf("%s intersecting symbol %q does not intersect the slice", prefix, symbolID)
			}
		}
		expected := make([]string, 0, maxIntersectingSymbols)
		for _, symbol := range symbolValues {
			if symbol.Kind == "synthetic_scope" ||
				symbol.Path != sourceSlice.Path ||
				symbol.EndLine < sourceSlice.StartLine ||
				symbol.StartLine > sourceSlice.EndLine {
				continue
			}
			expected = append(expected, symbol.ID)
		}
		if !reflect.DeepEqual(sourceSlice.IntersectingSymbolIDs, expected) {
			t.Fatalf(
				"%s.intersecting_symbol_ids = %#v, want complete ordered selected-symbol intersection %#v",
				prefix,
				sourceSlice.IntersectingSymbolIDs,
				expected,
			)
		}
		if enclosing.Kind == "synthetic_scope" && len(expected) < 2 {
			t.Fatalf("%s synthetic container must cross at least two selected named symbols", prefix)
		}
		switch sourceSlice.ScopeCoverage {
		case "partial":
		case "complete":
			if enclosing.Kind == "synthetic_scope" ||
				enclosing.StartLine != sourceSlice.StartLine ||
				enclosing.EndLine != sourceSlice.EndLine {
				t.Fatalf("%s complete scope does not preserve the full named range", prefix)
			}
		default:
			t.Fatalf("%s.scope_coverage = %q", prefix, sourceSlice.ScopeCoverage)
		}
	}
	for _, symbol := range symbolValues {
		if symbol.Kind == "synthetic_scope" && syntheticUses[symbol.ID] != 1 {
			t.Fatalf(
				"synthetic scope %q is used by %d source slices, want exactly one",
				symbol.ID,
				syntheticUses[symbol.ID],
			)
		}
	}
}

func validateStructureCalls(
	t *testing.T,
	calls []structureCall,
	symbols map[string]structureSymbol,
	slices []sourceSlice,
) {
	t.Helper()
	if len(calls) == 0 || len(calls) > maxStructureCalls {
		t.Fatalf("static calls count = %d, want 1..%d", len(calls), maxStructureCalls)
	}
	previous := ""
	seen := make(map[string]struct{}, len(calls))
	for i, call := range calls {
		prefix := fmt.Sprintf("static_calls[%d]", i)
		caller, callerOK := symbols[call.CallerSymbolID]
		if !callerOK {
			t.Fatalf("%s.caller_symbol_id = %q, want known symbol", prefix, call.CallerSymbolID)
		}
		callee, calleeOK := symbols[call.CalleeSymbolID]
		if !calleeOK {
			t.Fatalf("%s.callee_symbol_id = %q, want known symbol", prefix, call.CalleeSymbolID)
		}
		validateSource(t, prefix, source{
			Path:      call.Path,
			StartLine: call.StartLine,
			EndLine:   call.EndLine,
		})
		if call.StartColumn < 1 || call.EndColumn <= call.StartColumn ||
			call.StartLine != call.EndLine {
			t.Fatalf("%s has invalid single-line end-exclusive columns", prefix)
		}
		if caller.Path != call.Path || !rangeContains(caller, call.StartLine, call.EndLine) {
			t.Fatalf("%s callsite is outside its caller symbol", prefix)
		}
		if !containedBySourceSlice(source{
			Path:      call.Path,
			StartLine: call.StartLine,
			EndLine:   call.EndLine,
		}, slices) {
			t.Fatalf("%s callsite is outside retained source", prefix)
		}
		token, ok := retainedTokenAt(
			slices,
			call.Path,
			call.StartLine,
			call.StartColumn,
			call.EndColumn,
		)
		if !ok || token != callee.Name {
			t.Fatalf(
				"%s callsite token = %q, want exact callee name %q",
				prefix,
				token,
				callee.Name,
			)
		}

		orderKey := fmt.Sprintf(
			"%s\x00%09d\x00%09d\x00%s\x00%s",
			call.Path,
			call.StartLine,
			call.StartColumn,
			call.CallerSymbolID,
			call.CalleeSymbolID,
		)
		if i > 0 && orderKey < previous {
			t.Fatalf("%s is not deterministically sorted", prefix)
		}
		if _, duplicate := seen[orderKey]; duplicate {
			t.Fatalf("%s duplicates a static call", prefix)
		}
		seen[orderKey] = struct{}{}
		previous = orderKey
	}
}

func retainedTokenAt(
	slices []sourceSlice,
	pathValue string,
	line, startColumn, endColumn int,
) (string, bool) {
	for _, sourceSlice := range slices {
		if sourceSlice.Path != pathValue ||
			line < sourceSlice.StartLine ||
			line > sourceSlice.EndLine {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(sourceSlice.Text, "\n"), "\n")
		lineIndex := line - sourceSlice.StartLine
		if lineIndex < 0 || lineIndex >= len(lines) {
			return "", false
		}
		lineText := lines[lineIndex]
		startIndex := startColumn - 1
		endIndex := endColumn - 1
		if startIndex < 0 || endIndex <= startIndex || endIndex > len(lineText) {
			return "", false
		}
		return lineText[startIndex:endIndex], true
	}
	return "", false
}

func validateUnresolvedCalls(
	t *testing.T,
	calls []unresolvedCall,
	symbols map[string]structureSymbol,
	slices []sourceSlice,
) {
	t.Helper()
	if len(calls) > maxUnresolvedCalls {
		t.Fatalf("unresolved calls count = %d, limit %d", len(calls), maxUnresolvedCalls)
	}
	for i, call := range calls {
		prefix := fmt.Sprintf("unresolved_calls[%d]", i)
		validateID(t, prefix+".fact_id", call.FactID)
		caller, ok := symbols[call.CallerSymbolID]
		if !ok {
			t.Fatalf("%s.caller_symbol_id = %q, want known symbol", prefix, call.CallerSymbolID)
		}
		validateSource(t, prefix, source{
			Path:      call.Path,
			StartLine: call.StartLine,
			EndLine:   call.EndLine,
		})
		if caller.Path != call.Path || !rangeContains(caller, call.StartLine, call.EndLine) {
			t.Fatalf("%s is outside its caller symbol", prefix)
		}
		if !containedBySourceSlice(source{
			Path:      call.Path,
			StartLine: call.StartLine,
			EndLine:   call.EndLine,
		}, slices) {
			t.Fatalf("%s is outside retained source", prefix)
		}
		validateText(t, prefix+".expression", call.Expression, maxStructureScalarBytes)
		if call.Reason != "dynamic_target" {
			t.Fatalf("%s.reason = %q", prefix, call.Reason)
		}
	}
}

func validateUnresolvedFactReferences(
	t *testing.T,
	name string,
	calls []unresolvedCall,
) {
	t.Helper()
	if len(calls) == 0 {
		return
	}
	factBytes := readBoundedFile(t, name+".adapter-facts.json", maxAdapterFactsBytes)
	facts := decodeStrict[[]recordedAdapterFact](t, factBytes)
	known := make(map[string]recordedAdapterFact, len(facts))
	for _, fact := range facts {
		known[fact.ID] = fact
	}
	for i, call := range calls {
		fact, ok := known[call.FactID]
		if !ok {
			t.Fatalf("unresolved_calls[%d].fact_id = %q, want recorded adapter fact", i, call.FactID)
		}
		if fact.Path != call.Path ||
			fact.StartLine != call.StartLine ||
			fact.EndLine != call.EndLine ||
			fact.Kind != "call" ||
			fact.Subject != call.Expression {
			t.Fatalf("unresolved_calls[%d] does not preserve its recorded adapter fact", i)
		}
	}
}

func validateRequiredStructure(t *testing.T, name string, packet structurePacket) {
	t.Helper()
	requireEdge := func(caller, callee string) {
		t.Helper()
		for _, call := range packet.StaticCalls {
			if call.CallerSymbolID == caller && call.CalleeSymbolID == callee {
				return
			}
		}
		t.Errorf("missing exact selected-symbol edge %q -> %q", caller, callee)
	}

	switch name {
	case "caddy":
		if packet.StructureProvenance.AdapterIdentity !=
			"internal/analyzer/golang/gopls@084c07588657556b9c23d8632ef7ba3a9e14ae9f;collector=2" {
			t.Fatalf("Caddy adapter identity = %q", packet.StructureProvenance.AdapterIdentity)
		}
		if len(packet.StaticCalls) != 4 {
			t.Fatalf("Caddy selected-symbol calls = %d, want 4", len(packet.StaticCalls))
		}
		requireEdge(
			"function:caddy.go:158:6:changeConfig",
			"function:caddy.go:337:6:unsyncedDecodeAndRun",
		)
		requireEdge(
			"function:caddy.go:337:6:unsyncedDecodeAndRun",
			"function:caddy.go:419:6:run",
		)
		requireEdge(
			"function:caddy.go:419:6:run",
			"function:caddy.go:484:6:provisionContext",
		)
		requireEdge(
			"function:caddy.go:337:6:unsyncedDecodeAndRun",
			"function:caddy.go:724:6:unsyncedStop",
		)
	case "beets":
		if packet.StructureProvenance.AdapterIdentity !=
			"internal/analyzer/python/pyright@084c07588657556b9c23d8632ef7ba3a9e14ae9f" {
			t.Fatalf("Beets adapter identity = %q", packet.StructureProvenance.AdapterIdentity)
		}
		if len(packet.StaticCalls) != 11 {
			t.Fatalf("Beets selected-symbol calls = %d, want 11", len(packet.StaticCalls))
		}
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:802:1:_raw_main",
			"pyright:symbol:beets/ui/__init__.py:749:1:_setup",
		)
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:802:1:_raw_main",
			"pyright:symbol:beets/ui/__init__.py:643:5:add_subcommand",
		)
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:802:1:_raw_main",
			"pyright:symbol:beets/ui/__init__.py:725:5:parse_subcommand",
		)
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:749:1:_setup",
			"pyright:symbol:beets/plugins.py:449:1:load_plugins",
		)
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:749:1:_setup",
			"pyright:symbol:beets/plugins.py:471:1:commands",
		)
		requireEdge(
			"pyright:symbol:beets/plugins.py:449:1:load_plugins",
			"pyright:symbol:beets/plugins.py:365:1:get_plugin_names",
		)
		requireEdge(
			"pyright:symbol:beets/plugins.py:471:1:commands",
			"pyright:symbol:beets/plugins.py:464:1:find_plugins",
		)
		requireEdge(
			"pyright:symbol:beets/ui/__init__.py:725:5:parse_subcommand",
			"pyright:symbol:beets/ui/__init__.py:702:5:_subcommand_for_name",
		)
		for _, symbol := range packet.Symbols {
			if symbol.Name == "main" {
				t.Fatal("Beets fixture must not synthesize main -> _setup")
			}
		}
		if len(packet.UnresolvedCalls) != 1 ||
			packet.UnresolvedCalls[0].Expression != "subcommand.func" {
			t.Fatal("Beets dynamic dispatch must remain explicitly unresolved")
		}
	default:
		t.Fatalf("unknown structure case %q", name)
	}
}

func rangeContains(symbol structureSymbol, startLine, endLine int) bool {
	return symbol.StartLine <= startLine && symbol.EndLine >= endLine
}

func projectStructure(packet structurePacket) structureProjection {
	return structureProjection{
		CaseID:              packet.CaseID,
		Repository:          packet.Repository,
		StructureProvenance: packet.StructureProvenance,
		Symbols:             packet.Symbols,
		StaticCalls:         packet.StaticCalls,
		UnresolvedCalls:     packet.UnresolvedCalls,
	}
}

func encodeStructure(t *testing.T, projection structureProjection) []byte {
	t.Helper()
	data, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func TestStructureProjectionDoesNotClaimNegativeCallEvidence(t *testing.T) {
	for _, name := range []string{"caddy", "beets"} {
		packetBytes := readBoundedFile(t, name+".source-slices.json", maxSourcePacketBytes)
		packet := decodeStrict[structurePacket](t, packetBytes)
		if packet.StructureProvenance.CallCoverage != "selected_symbol_targets_only" {
			t.Fatalf("%s call coverage = %q", name, packet.StructureProvenance.CallCoverage)
		}
		sidecar := string(encodeStructure(t, projectStructure(packet)))
		for _, forbidden := range []string{
			`"call_coverage": "complete"`,
			`"call_coverage": "exhaustive"`,
			`"name": "main"`,
			`"expression": "subcommand.func",\n      "callee_symbol_id"`,
		} {
			if strings.Contains(sidecar, forbidden) {
				t.Fatalf("%s structure contains forbidden negative-evidence claim %q", name, forbidden)
			}
		}
	}
}
