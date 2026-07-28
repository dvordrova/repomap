package semanticmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	adapterMaxPacketBytes  = 32 << 10
	adapterMaxSourceBytes  = 24 << 10
	adapterMaxSourceSlices = 12
	adapterMaxSliceLines   = 201
	adapterMaxPathBytes    = 240
	adapterMaxSyntaxBytes  = 240
	adapterMaxFacts        = 48
)

// AdapterFact is one bounded syntax record extracted from a retained Go
// source slice.
type AdapterFact struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Object    string `json:"object"`
}

type adapterPacket struct {
	SourceSlices []adapterSourceSlice `json:"source_slices"`
}

type adapterSourceSlice struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

type adapterFactKey struct {
	path      string
	startLine int
	endLine   int
	kind      string
	subject   string
	object    string
	depth     int
	region    int
}

type adapterSliceFacts struct {
	path      string
	startLine int
	region    int
	facts     []adapterFactKey
}

type adapterParseCandidate struct {
	prefix      string
	suffix      string
	prefixLines int
}

// ExtractGoAdapterFacts extracts deterministic, bounded syntax records from
// the .go source slices in packetJSON. Non-Go slices are ignored.
func ExtractGoAdapterFacts(packetJSON []byte) ([]AdapterFact, error) {
	packet, err := decodeAdapterPacket(packetJSON)
	if err != nil {
		return nil, err
	}

	groups := make([]adapterSliceFacts, 0, len(packet.SourceSlices))
	totalSourceBytes := 0
	for i, sourceSlice := range packet.SourceSlices {
		if err := validateAdapterSourceSlice(sourceSlice); err != nil {
			return nil, fmt.Errorf("source_slices[%d]: %w", i, err)
		}
		totalSourceBytes += len(sourceSlice.Text)
		if totalSourceBytes > adapterMaxSourceBytes {
			return nil, fmt.Errorf("source slice text exceeds %d bytes", adapterMaxSourceBytes)
		}
		if path.Ext(sourceSlice.Path) != ".go" {
			continue
		}

		facts := extractAdapterSliceFacts(sourceSlice)
		sortAdapterFactKeys(facts)
		facts = deduplicateAdapterFactKeys(facts)
		byRegion := make(map[int][]adapterFactKey)
		for _, fact := range facts {
			byRegion[fact.region] = append(byRegion[fact.region], fact)
		}
		regions := make([]int, 0, len(byRegion))
		for region := range byRegion {
			regions = append(regions, region)
		}
		sort.Ints(regions)
		for _, region := range regions {
			groups = append(groups, adapterSliceFacts{
				path:      sourceSlice.Path,
				startLine: sourceSlice.StartLine,
				region:    region,
				facts:     byRegion[region],
			})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].path != groups[j].path {
			return groups[i].path < groups[j].path
		}
		if groups[i].startLine != groups[j].startLine {
			return groups[i].startLine < groups[j].startLine
		}
		return groups[i].region < groups[j].region
	})

	selected := selectAdapterFacts(groups)
	sortAdapterFactKeys(selected)

	result := make([]AdapterFact, 0, min(len(selected), adapterMaxFacts))
	for i, fact := range selected {
		result = append(result, AdapterFact{
			ID:        fmt.Sprintf("r%d", i+1),
			Path:      fact.path,
			StartLine: fact.startLine,
			EndLine:   fact.endLine,
			Kind:      fact.kind,
			Subject:   fact.subject,
			Object:    fact.object,
		})
	}
	return result, nil
}

func decodeAdapterPacket(packetJSON []byte) (adapterPacket, error) {
	if len(packetJSON) == 0 || len(packetJSON) > adapterMaxPacketBytes {
		return adapterPacket{}, fmt.Errorf(
			"packet size is %d bytes; limit is %d",
			len(packetJSON),
			adapterMaxPacketBytes,
		)
	}

	var packet adapterPacket
	decoder := json.NewDecoder(bytes.NewReader(packetJSON))
	if err := decoder.Decode(&packet); err != nil {
		return adapterPacket{}, fmt.Errorf("decode packet: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return adapterPacket{}, fmt.Errorf("decode packet: trailing JSON value")
		}
		return adapterPacket{}, fmt.Errorf("decode packet trailing data: %w", err)
	}
	if len(packet.SourceSlices) == 0 || len(packet.SourceSlices) > adapterMaxSourceSlices {
		return adapterPacket{}, fmt.Errorf(
			"source_slices count is %d; limit is 1..%d",
			len(packet.SourceSlices),
			adapterMaxSourceSlices,
		)
	}
	return packet, nil
}

func validateAdapterSourceSlice(sourceSlice adapterSourceSlice) error {
	if sourceSlice.Path == "" ||
		len(sourceSlice.Path) > adapterMaxPathBytes ||
		!utf8.ValidString(sourceSlice.Path) ||
		strings.ContainsRune(sourceSlice.Path, 0) ||
		strings.Contains(sourceSlice.Path, `\`) ||
		strings.Contains(sourceSlice.Path, ":") ||
		path.IsAbs(sourceSlice.Path) ||
		path.Clean(sourceSlice.Path) != sourceSlice.Path ||
		sourceSlice.Path == "." ||
		sourceSlice.Path == ".." ||
		strings.HasPrefix(sourceSlice.Path, "../") {
		return fmt.Errorf("path is not bounded canonical repository-relative syntax")
	}
	if sourceSlice.StartLine <= 0 ||
		sourceSlice.EndLine < sourceSlice.StartLine ||
		sourceSlice.EndLine-sourceSlice.StartLine+1 > adapterMaxSliceLines {
		return fmt.Errorf(
			"line range %d..%d is invalid or exceeds %d lines",
			sourceSlice.StartLine,
			sourceSlice.EndLine,
			adapterMaxSliceLines,
		)
	}
	if sourceSlice.Text == "" ||
		len(sourceSlice.Text) > adapterMaxSourceBytes ||
		!utf8.ValidString(sourceSlice.Text) ||
		strings.ContainsRune(sourceSlice.Text, 0) {
		return fmt.Errorf("text is empty, invalid, or exceeds %d bytes", adapterMaxSourceBytes)
	}
	lineCount := len(strings.Split(strings.TrimSuffix(sourceSlice.Text, "\n"), "\n"))
	if lineCount != sourceSlice.EndLine-sourceSlice.StartLine+1 {
		return fmt.Errorf(
			"text has %d lines; range has %d",
			lineCount,
			sourceSlice.EndLine-sourceSlice.StartLine+1,
		)
	}
	return nil
}

func extractAdapterSliceFacts(sourceSlice adapterSourceSlice) []adapterFactKey {
	candidates := []adapterParseCandidate{
		{
			prefix:      "package repomapslice\n",
			prefixLines: 1,
		},
		{
			prefix:      "package repomapslice\nfunc __repomap_slice__() {\n",
			suffix:      "\n}\n",
			prefixLines: 2,
		},
	}

	var best []adapterFactKey
	bestClean := false
	for _, candidate := range candidates {
		fset := token.NewFileSet()
		source := candidate.prefix + sourceSlice.Text + candidate.suffix
		file, parseErr := parser.ParseFile(
			fset,
			sourceSlice.Path,
			source,
			parser.AllErrors|parser.SkipObjectResolution,
		)
		if file == nil {
			continue
		}

		facts := make([]adapterFactKey, 0)
		nextRegion := 0
		ast.Walk(&adapterFactVisitor{
			fset:        fset,
			sourceSlice: sourceSlice,
			prefixLines: candidate.prefixLines,
			nextRegion:  &nextRegion,
			facts:       &facts,
		}, file)
		clean := parseErr == nil
		if (clean && !bestClean && len(facts) > 0) ||
			(clean == bestClean && len(facts) > len(best)) {
			best = facts
			bestClean = clean
		}
	}
	return best
}

type adapterFactVisitor struct {
	fset        *token.FileSet
	sourceSlice adapterSourceSlice
	prefixLines int
	depth       int
	region      int
	nextRegion  *int
	facts       *[]adapterFactKey
}

func (v *adapterFactVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch value := node.(type) {
	case *ast.ExprStmt:
		if call, ok := value.X.(*ast.CallExpr); ok {
			v.emitCall("call", call)
		}
	case *ast.AssignStmt:
		subject, subjectOK := adapterRenderExprs(v.fset, value.Lhs, "")
		object, objectOK := adapterRenderExprs(v.fset, value.Rhs, "")
		if subjectOK && objectOK {
			v.emit("assign", subject, object, value.Pos(), value.End())
		}
	case *ast.ValueSpec:
		if len(value.Values) > 0 {
			subject := strings.Join(adapterIdentifierNames(value.Names), ", ")
			object, objectOK := adapterRenderExprs(v.fset, value.Values, "")
			if subject != "" && objectOK {
				v.emit("assign", subject, object, value.Pos(), value.End())
			}
		}
	case *ast.IncDecStmt:
		subject, subjectOK := adapterRenderNode(v.fset, value.X)
		if subjectOK {
			v.emit("assign", subject, value.Tok.String(), value.Pos(), value.End())
		}
	case *ast.IfStmt:
		object, ok := adapterRenderNode(v.fset, value.Cond)
		if ok {
			v.emit("branch", "if", object, value.If, value.Cond.End())
		}
	case *ast.SwitchStmt:
		object := "{}"
		end := value.Body.Lbrace
		if value.Tag != nil {
			var ok bool
			object, ok = adapterRenderNode(v.fset, value.Tag)
			if !ok {
				object = ""
			}
			end = value.Tag.End()
		}
		if object != "" {
			v.emit("branch", "switch", object, value.Switch, end)
		}
	case *ast.TypeSwitchStmt:
		object, ok := adapterRenderNode(v.fset, value.Assign)
		if ok {
			v.emit("branch", "type switch", object, value.Switch, value.Assign.End())
		}
	case *ast.SelectStmt:
		v.emit("branch", "select", "{}", value.Select, value.Body.Lbrace)
	case *ast.CaseClause:
		subject := "default"
		object := ":"
		if len(value.List) > 0 {
			subject = "case"
			var ok bool
			object, ok = adapterRenderExprs(v.fset, value.List, "")
			if !ok {
				object = ""
			}
		}
		if object != "" {
			v.emit("branch", subject, object, value.Case, value.Colon)
		}
	case *ast.ForStmt:
		object := "{}"
		if value.Cond != nil {
			var ok bool
			object, ok = adapterRenderNode(v.fset, value.Cond)
			if !ok {
				object = ""
			}
		}
		if object != "" {
			v.emit("loop", "for", object, value.For, value.Body.Lbrace)
		}
	case *ast.RangeStmt:
		subject := "range"
		if value.Key != nil {
			expressions := []ast.Expr{value.Key}
			if value.Value != nil {
				expressions = append(expressions, value.Value)
			}
			var ok bool
			subject, ok = adapterRenderExprs(v.fset, expressions, "")
			if !ok {
				subject = ""
			}
		}
		object, objectOK := adapterRenderNode(v.fset, value.X)
		if subject != "" && objectOK {
			v.emit("loop", subject, object, value.For, value.X.End())
		}
	case *ast.ReturnStmt:
		object, ok := adapterRenderExprs(v.fset, value.Results, "()")
		if ok {
			v.emit("return", "return", object, value.Return, value.End())
		}
	case *ast.DeferStmt:
		v.emitCall("defer", value.Call)
	}

	switch node.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		child := *v
		(*v.nextRegion)++
		child.region = *v.nextRegion
		child.depth = 0
		return &child
	case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt,
		*ast.SelectStmt, *ast.CaseClause, *ast.ForStmt, *ast.RangeStmt:
		child := *v
		child.depth++
		return &child
	}
	return v
}

func (v *adapterFactVisitor) emitCall(kind string, call *ast.CallExpr) {
	subject, subjectOK := adapterRenderNode(v.fset, call.Fun)
	object, objectOK := adapterRenderExprs(v.fset, call.Args, "()")
	if subjectOK && objectOK {
		v.emit(kind, subject, object, call.Pos(), call.End())
	}
}

func (v *adapterFactVisitor) emit(
	kind, subject, object string,
	start, end token.Pos,
) {
	startLine, startOK := v.originalLine(start)
	endLine, endOK := v.originalLine(end)
	if !startOK || !endOK || endLine < startLine {
		return
	}
	if !adapterSyntaxScalar(subject) || !adapterSyntaxScalar(object) {
		return
	}
	*v.facts = append(*v.facts, adapterFactKey{
		path:      v.sourceSlice.Path,
		startLine: startLine,
		endLine:   endLine,
		kind:      kind,
		subject:   subject,
		object:    object,
		depth:     v.depth,
		region:    v.region,
	})
}

func (v *adapterFactVisitor) originalLine(position token.Pos) (int, bool) {
	if !position.IsValid() {
		return 0, false
	}
	parsedLine := v.fset.PositionFor(position, false).Line
	originalLine := v.sourceSlice.StartLine + parsedLine - v.prefixLines - 1
	if originalLine < v.sourceSlice.StartLine || originalLine > v.sourceSlice.EndLine {
		return 0, false
	}
	return originalLine, true
}

func adapterRenderExprs(
	fset *token.FileSet,
	expressions []ast.Expr,
	empty string,
) (string, bool) {
	if len(expressions) == 0 {
		return empty, adapterSyntaxScalar(empty)
	}
	parts := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		part, ok := adapterRenderNode(fset, expression)
		if !ok {
			return "", false
		}
		parts = append(parts, part)
	}
	value := strings.Join(parts, ", ")
	return value, adapterSyntaxScalar(value)
}

func adapterRenderNode(fset *token.FileSet, node any) (string, bool) {
	var buffer bytes.Buffer
	config := printer.Config{Mode: printer.RawFormat, Tabwidth: 8}
	if err := config.Fprint(&buffer, fset, node); err != nil {
		return "", false
	}
	value, ok := compactAdapterSyntax(buffer.String())
	if !ok || !adapterSyntaxScalar(value) || !adapterCompactSyntaxParses(node, value) {
		return "", false
	}
	return value, true
}

func adapterCompactSyntaxParses(node any, value string) bool {
	switch node.(type) {
	case ast.Expr:
		_, err := parser.ParseExpr(value)
		return err == nil
	case ast.Stmt:
		_, err := parser.ParseFile(
			token.NewFileSet(),
			"syntax.go",
			"package syntax\nfunc _() {\n"+value+"\n}\n",
			parser.SkipObjectResolution,
		)
		return err == nil
	default:
		return true
	}
}

func compactAdapterSyntax(value string) (string, bool) {
	var builder strings.Builder
	builder.Grow(min(len(value), adapterMaxSyntaxBytes))

	var quote rune
	escaped := false
	pendingSpace := false
	for _, current := range value {
		if quote != 0 {
			if current == '\r' || current == '\n' {
				return "", false
			}
			builder.WriteRune(current)
			if quote != '`' && escaped {
				escaped = false
				continue
			}
			if quote != '`' && current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}

		if unicode.IsSpace(current) {
			pendingSpace = builder.Len() > 0
			continue
		}
		if pendingSpace {
			builder.WriteByte(' ')
			pendingSpace = false
		}
		builder.WriteRune(current)
		if current == '"' || current == '\'' || current == '`' {
			quote = current
		}
		if builder.Len() > adapterMaxSyntaxBytes {
			return "", false
		}
	}
	if quote != 0 {
		return "", false
	}

	result := strings.TrimSpace(builder.String())
	return result, adapterSyntaxScalar(result)
}

func adapterSyntaxScalar(value string) bool {
	return value != "" &&
		len(value) <= adapterMaxSyntaxBytes &&
		utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) &&
		!strings.ContainsAny(value, "\r\n") &&
		strings.TrimSpace(value) == value
}

func adapterIdentifierNames(identifiers []*ast.Ident) []string {
	names := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if identifier != nil && identifier.Name != "" {
			names = append(names, identifier.Name)
		}
	}
	return names
}

func selectAdapterFacts(groups []adapterSliceFacts) []adapterFactKey {
	lengths := make([]int, len(groups))
	for i, group := range groups {
		lengths[i] = len(group.facts)
	}
	quotas := coverageAdapterQuotas(lengths, adapterMaxFacts)

	selected := make([]adapterFactKey, 0, adapterMaxFacts)
	seen := make(map[adapterFactKey]struct{}, adapterMaxFacts)
	for i, group := range groups {
		for _, fact := range selectAdapterGroupFacts(group.facts, quotas[i]) {
			comparable := fact
			comparable.depth = 0
			comparable.region = 0
			if _, duplicate := seen[comparable]; duplicate {
				continue
			}
			seen[comparable] = struct{}{}
			selected = append(selected, fact)
		}
	}
	return selected
}

func coverageAdapterQuotas(lengths []int, limit int) []int {
	quotas := make([]int, len(lengths))
	selected := 0
	for selected < limit {
		best := -1
		for i, length := range lengths {
			if quotas[i] >= length {
				continue
			}
			if best < 0 ||
				quotas[i]*lengths[best] < quotas[best]*length {
				best = i
			}
		}
		if best < 0 {
			break
		}
		quotas[best]++
		selected++
	}
	return quotas
}

func selectAdapterGroupFacts(facts []adapterFactKey, count int) []adapterFactKey {
	if count >= len(facts) {
		return append([]adapterFactKey(nil), facts...)
	}

	selectedIndexes := make(map[int]struct{}, count)
	appendIndex := func(index int) bool {
		if len(selectedIndexes) == count {
			return false
		}
		if _, duplicate := selectedIndexes[index]; duplicate {
			return true
		}
		selectedIndexes[index] = struct{}{}
		return true
	}

	if count >= 2 &&
		facts[len(facts)-1].kind == "return" &&
		!adapterEmptyReturn(facts[len(facts)-1].object) {
		appendIndex(len(facts) - 1)
	}

	controls := make([]int, 0, len(facts))
	actions := make([]int, 0, len(facts))
	returns := make([]int, 0, len(facts))
	for i, fact := range facts {
		switch fact.kind {
		case "assign", "call", "defer":
			actions = append(actions, i)
		case "branch", "loop":
			controls = append(controls, i)
		case "return":
			returns = append(returns, i)
		}
	}
	if count >= 3 && len(controls) > 0 {
		appendIndex(controls[len(controls)/2])
	}
	if count >= 4 && len(actions) <= 2 && len(returns) > 1 {
		appendIndex(returns[0])
	}

	actionOrder := orderAdapterActions(facts, actions)
	for _, index := range actionOrder {
		if _, duplicate := selectedIndexes[index]; duplicate {
			continue
		}
		appendIndex(index)
		if len(selectedIndexes) == count {
			break
		}
	}

	for _, index := range adapterSpreadOrder(len(facts)) {
		appendIndex(index)
		if len(selectedIndexes) == count {
			break
		}
	}

	selected := make([]adapterFactKey, 0, count)
	for index := range selectedIndexes {
		selected = append(selected, facts[index])
	}
	sortAdapterFactKeys(selected)
	return selected
}

func orderAdapterActions(facts []adapterFactKey, indexes []int) []int {
	if len(indexes) == 2 {
		left, right := facts[indexes[0]], facts[indexes[1]]
		if left.kind == "assign" && right.kind != "assign" {
			return indexes
		}
		if right.kind == "assign" && left.kind != "assign" {
			return []int{indexes[1], indexes[0]}
		}
		return []int{indexes[1], indexes[0]}
	}
	order := adapterSpreadOrder(len(indexes))
	result := make([]int, 0, len(indexes))
	for _, position := range order {
		result = append(result, indexes[position])
	}
	return result
}

func adapterSpreadOrder(length int) []int {
	if length == 0 {
		return nil
	}
	seen := make(map[int]struct{}, length)
	order := make([]int, 0, length)
	appendIndex := func(index int) {
		if index < 0 || index >= length {
			return
		}
		if _, duplicate := seen[index]; duplicate {
			return
		}
		seen[index] = struct{}{}
		order = append(order, index)
	}
	for _, index := range []int{
		length / 2,
		0,
		length - 1,
		length / 4,
		3 * (length - 1) / 4,
		length / 3,
		2 * (length - 1) / 3,
	} {
		appendIndex(index)
	}
	for index := 0; index < length; index++ {
		appendIndex(index)
	}
	return order
}

func adapterEmptyReturn(value string) bool {
	return value == "nil" || value == "()"
}

func sortAdapterFactKeys(facts []adapterFactKey) {
	sort.Slice(facts, func(i, j int) bool {
		left, right := facts[i], facts[j]
		if left.path != right.path {
			return left.path < right.path
		}
		if left.startLine != right.startLine {
			return left.startLine < right.startLine
		}
		if left.endLine != right.endLine {
			return left.endLine < right.endLine
		}
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		if left.subject != right.subject {
			return left.subject < right.subject
		}
		if left.object != right.object {
			return left.object < right.object
		}
		if left.depth != right.depth {
			return left.depth < right.depth
		}
		return left.region < right.region
	})
}

func deduplicateAdapterFactKeys(facts []adapterFactKey) []adapterFactKey {
	if len(facts) < 2 {
		return facts
	}
	write := 1
	for read := 1; read < len(facts); read++ {
		left := facts[read]
		right := facts[write-1]
		left.depth = 0
		left.region = 0
		right.depth = 0
		right.region = 0
		if left == right {
			continue
		}
		facts[write] = facts[read]
		write++
	}
	return facts[:write]
}
