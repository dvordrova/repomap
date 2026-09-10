package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/llm"
)

const adjunctVersion = "repomap.terminology.deferred.v1"
const catalogDelimiter = "\n\nREPOMAP_PROSE_SOURCES_V1\n"

// Collector retains prose only after the original cube validates its result;
// optional definitions are generated separately from that accepted prose.
// Prepared requests carry their catalogue, so cache and memo reuse need no
// process-local preparation registry and remain safe under parallel execution.
type Collector struct {
	paths   map[string]bool
	mu      sync.Mutex
	values  map[string]Candidate
	pending map[string]proseSource
}

func NewCollector(paths []string) *Collector {
	c := &Collector{paths: make(map[string]bool), values: make(map[string]Candidate), pending: make(map[string]proseSource)}
	for _, name := range paths {
		if canonicalPath(name) {
			c.paths[name] = true
		}
	}
	return c
}

func canonicalPath(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, "../") && !path.IsAbs(name) && path.Clean(name) == name && !strings.ContainsAny(name, "\\\x00")
}

type provider struct {
	base      llm.Provider
	collector *Collector
}

func (c *Collector) Wrap(base llm.Provider) llm.Provider {
	if base == nil {
		return nil
	}
	return &provider{base: base, collector: c}
}

func (p *provider) State() []byte {
	// The source-catalogue contract is already in the exact prepared request. Keep the
	// transport identity unchanged so replay through the base provider refreshes
	// the same cache entry that ordinary decorated calls subsequently read.
	return append([]byte(nil), p.base.State()...)
}

type catalogSource struct {
	Ref  string `json:"ref"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Row  string `json:"row,omitempty"`
}
type sourceCatalog struct {
	Version string          `json:"version"`
	Sources []catalogSource `json:"sources"`
}

func (p *provider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	return p.base.Prepare(prompt, limits)
}

func (p *provider) AdaptPrompt(prompt llm.Prompt) (llm.Prompt, error) {
	input, _ := jsonValue([]byte(prompt.User))
	catalog := sourceCatalog{Version: adjunctVersion, Sources: sourcesIn(input, p.collector.paths)}
	if len(catalog.Sources) == 0 {
		return prompt, nil
	}
	// Keep the owner's sole response shape unchanged. This source catalogue
	// records provenance for a later glossary pass; it asks for no metadata.
	raw, err := json.Marshal(catalog)
	if err != nil {
		return llm.Prompt{}, err
	}
	prompt.User += catalogDelimiter + string(raw)
	return prompt, nil
}

func (p *provider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	return p.base.Complete(ctx, prepared)
}

func jsonValue(raw []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("terminology: trailing JSON")
	}
	return value, nil
}

func strictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("terminology: invalid metadata shape")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("terminology: trailing metadata")
	}
	return nil
}

func sourceKey(source catalogSource) string {
	raw, _ := json.Marshal([]any{source.Path, source.Line, source.Row})
	return string(raw)
}

// Only exact path string leaves already sent by the owning cube can become
// source refs. A line belongs to the same object with one unambiguous path;
// otherwise the honest source is the whole file. No prose path extraction.
func sourcesIn(input any, paths map[string]bool) []catalogSource {
	seen := make(map[string]catalogSource)
	resultRows := make(map[string]bool)
	if object, ok := input.(map[string]any); ok {
		if rows, ok := object["rows"].([]any); ok {
			for _, row := range rows {
				if row, ok := row.(map[string]any); ok {
					if key, ok := row["key"].(string); ok && key != "" {
						resultRows[key] = true
					}
				}
			}
		}
	}
	var walk func(any, string)
	var walkObject func(map[string]any, string)
	walkObject = func(value map[string]any, row string) {
		if key, ok := value["key"].(string); ok {
			row = key
		}
		immediate := make(map[string]bool)
		for field, child := range value {
			if field == "result_rows" {
				continue
			}
			if text, ok := child.(string); ok && paths[text] {
				immediate[text] = true
			}
		}
		line := 0
		if len(immediate) == 1 {
			for _, field := range []string{"line", "line_no"} {
				if number, ok := value[field].(json.Number); ok {
					if n, err := strconv.Atoi(string(number)); err == nil && n > 0 {
						line = n
						break
					}
				}
			}
		}
		for field, child := range value {
			if field == "result_rows" {
				continue
			}
			if text, ok := child.(string); ok && paths[text] {
				source := catalogSource{Path: text, Line: line, Row: row}
				seen[sourceKey(source)] = source
			} else {
				walk(child, row)
			}
		}
	}
	walk = func(value any, row string) {
		switch value := value.(type) {
		case map[string]any:
			if memberships, scoped := value["result_rows"]; scoped {
				// A shared source may explicitly name its eligible result rows.
				// Only the owning request's top-level row keys authorize those
				// memberships; unknown or empty sets never become global scope.
				if refs, ok := memberships.([]any); ok {
					accepted := make(map[string]bool)
					for _, ref := range refs {
						if ref, ok := ref.(string); ok && resultRows[ref] && !accepted[ref] {
							accepted[ref] = true
							walkObject(value, ref)
						}
					}
				}
				return
			}
			walkObject(value, row)
		case []any:
			for _, child := range value {
				walk(child, row)
			}
		case string:
			if paths[value] {
				source := catalogSource{Path: value, Row: row}
				seen[sourceKey(source)] = source
			}
		}
	}
	walk(input, "")
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]catalogSource, 0, len(keys))
	for i, key := range keys {
		source := seen[key]
		source.Ref = fmt.Sprintf("g%d", i+1)
		result = append(result, source)
	}
	return result
}

type requestContext struct {
	sources       map[string]catalogSource
	noTerminology bool
	input         any
}

// The suffix is read from an actual string leaf in the saved provider request,
// not a remembered current Prepare call. The original input remains the sole
// source authority, and paths no longer in the current corpus are unsupported.
func (c *Collector) requestContext(request []byte) (requestContext, error) {
	var leaves []string
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		case string:
			if strings.Contains(value, catalogDelimiter) {
				leaves = append(leaves, value)
			}
		}
	}
	if value, err := jsonValue(request); err == nil {
		walk(value)
	} else if strings.Contains(string(request), catalogDelimiter) {
		leaves = append(leaves, string(request))
	}
	if len(leaves) == 0 {
		return requestContext{noTerminology: true}, nil
	}
	if len(leaves) != 1 {
		return requestContext{}, fmt.Errorf("terminology: exact request has no unique source catalogue")
	}
	index := strings.LastIndex(leaves[0], catalogDelimiter)
	var catalog sourceCatalog
	if err := strictJSON([]byte(leaves[0][index+len(catalogDelimiter):]), &catalog); err != nil {
		return requestContext{}, err
	}
	if catalog.Version != adjunctVersion || catalog.Sources == nil {
		return requestContext{}, fmt.Errorf("terminology: unsupported source catalogue")
	}
	if len(catalog.Sources) == 0 {
		return requestContext{}, fmt.Errorf("terminology: unsupported response contract")
	}
	input, _ := jsonValue([]byte(leaves[0][:index]))
	paths := make(map[string]bool)
	for _, source := range catalog.Sources {
		if !canonicalPath(source.Path) {
			return requestContext{}, fmt.Errorf("terminology: invalid source path")
		}
		paths[source.Path] = true
	}
	original := make(map[string]bool)
	for _, source := range sourcesIn(input, paths) {
		original[sourceKey(source)] = true
	}
	ctx := requestContext{sources: make(map[string]catalogSource), input: input}
	for i, source := range catalog.Sources {
		if source.Ref != fmt.Sprintf("g%d", i+1) || !original[sourceKey(source)] {
			return requestContext{}, fmt.Errorf("terminology: source catalogue is not original evidence")
		}
		if c.paths[source.Path] {
			ctx.sources[source.Ref] = source
		}
	}
	return ctx, nil
}

type termWire struct {
	Name        *string   `json:"name"`
	Explanation *string   `json:"explanation"`
	Sources     []*string `json:"sources"`
}
type validatedTerm struct {
	candidate Candidate
	sources   []catalogSource
	rows      []string
}

func (p *provider) AdaptResponse(request, response []byte) (llm.AdaptedResponse, error) {
	ctx, err := p.collector.requestContext(request)
	if err != nil {
		return llm.AdaptedResponse{}, err
	}
	adapted := llm.AdaptedResponse{Domain: response}
	if ctx.noTerminology {
		return adapted, nil
	}
	raw, err := llm.NormalizeJSON(response)
	if err != nil {
		return llm.AdaptedResponse{}, err
	}
	result, err := jsonValue(raw)
	if err != nil {
		return llm.AdaptedResponse{}, err
	}
	result = tableProse(result, ctx.input)
	sourceRows := make(map[string]bool)
	for _, source := range ctx.sources {
		if source.Row != "" {
			sourceRows[source.Row] = true
		}
	}
	texts := resultTextByRow(result, sourceRows)
	digest := sha256.Sum256(request)
	requestID := hex.EncodeToString(digest[:])
	adapted.Accept = func(rows []string) { p.collector.collectProse(requestID, texts, ctx.sources, rows) }
	return adapted, nil
}

// Table owners advertise their actual prose cells and conditional branches.
// Unused cells, extra fields and closed choices are not glossary text merely
// because the containing row's independent decision was accepted.
func tableProse(result, input any) any {
	request, ok := input.(map[string]any)
	if !ok {
		return result
	}
	fill, table := request["fill"].([]any)
	if !table {
		return result
	}
	object, ok := result.(map[string]any)
	if !ok {
		return result
	}
	rows, ok := object["rows"].([]any)
	if !ok {
		return result
	}
	var projected []any
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		prose := map[string]any{"key": row["key"]}
		for _, item := range fill {
			column, ok := item.(map[string]any)
			if !ok || (column["kind"] != "text" && column["kind"] != "prose") {
				continue
			}
			active := true
			if when, ok := column["when"].(map[string]any); ok {
				for key, expected := range when {
					active = active && row[key] == expected
				}
			}
			if name, ok := column["name"].(string); ok && active {
				if text, ok := row[name].(string); ok {
					prose[name] = text
				}
			}
		}
		projected = append(projected, prose)
	}
	return map[string]any{"rows": projected}
}

// validateTermMetadata owns only the later optional glossary response. It can
// reject a definition, never the already accepted analysis that supplied prose.
func validateTermMetadata(result any, sources map[string]catalogSource, wire []json.RawMessage) ([]validatedTerm, []llm.ResponseRejection) {
	var rejections []llm.ResponseRejection
	rejectionByReason := make(map[string]int)
	reject := func(reason, position string) {
		index, found := rejectionByReason[reason]
		if !found {
			index = len(rejections)
			rejectionByReason[reason] = index
			rejections = append(rejections, llm.ResponseRejection{Kind: "glossary_term_rejected", Reason: reason})
		}
		rejections[index].Count++
		if len(rejections[index].Samples) < 5 {
			rejections[index].Samples = append(rejections[index].Samples, position)
		}
	}
	terms := make([]validatedTerm, 0, len(wire))
	sourceRows := make(map[string]bool)
	for _, source := range sources {
		if source.Row != "" {
			sourceRows[source.Row] = true
		}
	}
	textByRow := resultTextByRow(result, sourceRows)
	matchesByName := make(map[string][]string)
	for index, rawTerm := range wire {
		position := fmt.Sprintf("terms[%d]", index)
		var term termWire
		if err := strictJSON(rawTerm, &term); err != nil || term.Name == nil || term.Explanation == nil || term.Sources == nil {
			reject("invalid optional term shape", position)
			continue
		}
		name, explanation := *term.Name, strings.TrimSpace(*term.Explanation)
		if name == "" || name != strings.TrimSpace(name) || explanation == "" || explanation == "none" {
			reject("invalid optional term fields", position)
			continue
		}
		validated := validatedTerm{candidate: Candidate{Name: name, Explanation: explanation}}
		matches, known := matchesByName[name]
		if !known {
			for row, texts := range textByRow {
				for _, text := range texts {
					if mentionsTerm(text, name) {
						matches = append(matches, row)
						break
					}
				}
			}
			sort.Strings(matches)
			matchesByName[name] = matches
		}
		seen := make(map[string]bool)
		matchedRows := make(map[string]bool)
		for _, ref := range term.Sources {
			if ref == nil {
				reject("invalid optional source ref", position)
				continue
			}
			source, ok := sources[*ref]
			if !ok {
				reject("unsupported optional source ref", position)
				continue
			}
			if seen[*ref] {
				continue
			}
			seen[*ref] = true
			supported := false
			for _, row := range matches {
				if source.Row == "" || source.Row == row {
					matchedRows[row] = true
					supported = true
				}
			}
			if supported {
				validated.sources = append(validated.sources, source)
			}
		}
		for row := range matchedRows {
			validated.rows = append(validated.rows, row)
		}
		sort.Strings(validated.rows)
		if len(validated.sources) == 0 || len(validated.rows) == 0 {
			reject("term has no source-backed occurrence in the computed result", position)
			continue
		}
		terms = append(terms, validated)
	}
	return terms, rejections
}

// Occurrence ownership comes from the computed answer. Named source and
// intent rows keep their scope throughout their prose; nested metadata cannot
// move rejected text into a neighbouring accepted row.
func resultTextByRow(result any, sourceRows map[string]bool) map[string][]string {
	rows := make(map[string][]string)
	var walk func(any, string, bool)
	walk = func(value any, row string, scoped bool) {
		switch value := value.(type) {
		case map[string]any:
			if !scoped {
				if key, ok := value["key"].(string); ok {
					row = key
				}
				// A question selection may cite an advertised evidence row.
				if ref, ok := value["row"].(string); ok && sourceRows[ref] {
					row = ref
				}
			}
			for field, child := range value {
				switch field {
				case "key", "row", "intent", "file_ref", "ref":
					// Request-local ownership is not accepted explanatory prose.
					continue
				}
				walk(child, row, scoped)
			}
		case []any:
			for _, child := range value {
				walk(child, row, scoped)
			}
		case string:
			rows[row] = append(rows[row], value)
		}
	}
	walkNamedRows := func(values []any, field string) {
		for _, value := range values {
			object, _ := value.(map[string]any)
			ref, ok := object[field].(string)
			walk(value, ref, ok && ref != "")
		}
	}
	walkSlots := func(value any, field string) {
		if values, ok := value.([]any); ok {
			for i, row := range values {
				walk(row, fmt.Sprintf("%s[%d]", field, i), true)
			}
		}
	}
	switch value := result.(type) {
	case []any:
		// Repository-guidance classifications use one file_ref per item.
		walkNamedRows(value, "file_ref")
	case map[string]any:
		_, reviews := value["reviews"].([]any)
		_, sources := value["sources"].([]any)
		orientation := false
		for _, field := range []string{"summary_refs", "roles", "run_recipe", "main_flow"} {
			if _, found := value[field]; found {
				orientation = true
			}
		}
		if _, keyed := value["key"]; keyed && !reviews && !sources && !orientation {
			walk(value, "", false)
			break
		}
		for field, child := range value {
			if orientation {
				switch field {
				case "summary":
					walk(child, "summary", true)
					continue
				case "roles", "run_recipe":
					walkSlots(child, field)
					continue
				case "main_flow":
					if flow, ok := child.(map[string]any); ok {
						walk(flow["title"], "main_flow.title", true)
						walkSlots(flow["steps"], "main_flow.steps")
					}
					continue
				}
			}
			if values, ok := child.([]any); ok {
				switch field {
				case "reviews":
					walkNamedRows(values, "intent")
					continue
				case "sources":
					walkNamedRows(values, "ref")
					continue
				}
			}
			walk(child, "", false)
		}
	default:
		walk(result, "", false)
	}
	return rows
}

func mentionsTerm(text, name string) bool {
	if name == "" {
		return false
	}
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(name)
	last, _ := utf8.DecodeLastRuneInString(name)
	for offset := 0; offset <= len(text)-len(name); {
		found := strings.Index(text[offset:], name)
		if found < 0 {
			return false
		}
		start, end := offset+found, offset+found+len(name)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if (start == 0 || !word(before) || ScriptBoundary(before, first)) && (end == len(text) || !word(after) || ScriptBoundary(last, after)) {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		offset = start + size
	}
	return false
}

// ScriptBoundary recognizes adjacent letters from different concrete Unicode
// scripts. For example, the Korean particle in Matcher를 is outside the Latin
// name, while a Latin suffix in Matchers is not. Marks, digits and Common or
// Inherited script characters cannot manufacture a boundary.
func ScriptBoundary(left, right rune) bool {
	if !unicode.IsLetter(left) || !unicode.IsLetter(right) {
		return false
	}
	script := func(r rune) *unicode.RangeTable {
		if unicode.Is(unicode.Common, r) || unicode.Is(unicode.Inherited, r) {
			return nil
		}
		for _, table := range unicode.Scripts {
			if unicode.Is(table, r) {
				return table
			}
		}
		return nil
	}
	leftScript, rightScript := script(left), script(right)
	return leftScript != nil && rightScript != nil && leftScript != rightScript
}

func (c *Collector) Snapshot() []Candidate {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.values))
	for key := range c.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Candidate, 0, len(keys))
	for _, key := range keys {
		value := c.values[key]
		value.Sources = append([]Source(nil), value.Sources...)
		value.Origins = append([]Origin(nil), value.Origins...)
		result = append(result, value)
	}
	return result
}
