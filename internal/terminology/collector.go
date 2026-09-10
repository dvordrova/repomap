package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
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

const adjunctVersion = "repomap.terminology.adjunct.v3"
const catalogDelimiter = "\n\nREPOMAP_TERMINOLOGY_CATALOG_V3\n"

//go:embed prompts/adjunct.md
var adjunctPrompt string

//go:embed prompts/response-contract.md
var responseContract string

// Collector accepts metadata only after the original cube validates its result.
// Prepared requests carry their catalogue, so cache and memo reuse need no
// process-local preparation registry and remain safe under parallel execution.
type Collector struct {
	paths  map[string]bool
	mu     sync.Mutex
	values map[string]Candidate
}

func NewCollector(paths []string) *Collector {
	c := &Collector{paths: make(map[string]bool), values: make(map[string]Candidate)}
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
	// The adjunct contract is already in the exact prepared request. Keep the
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
	Version          string          `json:"version"`
	Sources          []catalogSource `json:"sources"`
	ResponseContract string          `json:"response_contract"`
	ResponseExample  json.RawMessage `json:"response_example"`
}

func (p *provider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	return p.base.Prepare(prompt, limits)
}

func (p *provider) AdaptPrompt(prompt llm.Prompt) (llm.Prompt, error) {
	input, _ := jsonValue([]byte(prompt.User))
	catalog := sourceCatalog{Version: adjunctVersion, Sources: sourcesIn(input, p.collector.paths)}
	if len(catalog.Sources) == 0 {
		// No source-backed terms can exist. Preserve the owner's exact request,
		// including an array response and its original provider controls.
		return prompt, nil
	}
	if prompt.ResponseExample == "" || !json.Valid([]byte(prompt.ResponseExample)) {
		return llm.Prompt{}, fmt.Errorf("terminology: owning task must supply a valid JSON response example")
	}
	example, err := json.Marshal(struct {
		Result json.RawMessage `json:"result"`
		Terms  []termWire      `json:"terms"`
	}{json.RawMessage(prompt.ResponseExample), []termWire{}})
	if err != nil {
		return llm.Prompt{}, err
	}
	catalog.ResponseContract = strings.TrimSpace(responseContract)
	catalog.ResponseExample = example
	prompt.System += "\n\n" + strings.TrimSpace(adjunctPrompt)
	prompt.ResponseFormatJSON = true
	// The catalogue carries the sole final shape, including the owner's exact
	// result container. Do not also emit a bare-domain response example.
	prompt.ResponseExample = ""
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
	sources             map[string]catalogSource
	noTerminology       bool
	termsOnlyInEnvelope bool
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
	if len(catalog.Sources) == 0 || catalog.ResponseContract != strings.TrimSpace(responseContract) || !validResponseExample(catalog.ResponseExample) {
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
	ctx := requestContext{sources: make(map[string]catalogSource)}
	// The owner supplied this exact result shape before the adjunct existed.
	// A legitimate result.terms field keeps its original domain authority.
	example, _ := jsonValue(catalog.ResponseExample)
	if object, ok := example.(map[string]any)["result"].(map[string]any); ok {
		_, ownsTerms := object["terms"]
		ctx.termsOnlyInEnvelope = !ownsTerms
	}
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

func validResponseExample(raw []byte) bool {
	var example struct {
		Result json.RawMessage `json:"result"`
		Terms  []termWire      `json:"terms"`
	}
	return strictJSON(raw, &example) == nil && len(example.Result) > 0 && example.Terms != nil && len(example.Terms) == 0
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
	c := p.collector
	ctx, err := c.requestContext(request)
	if err != nil {
		return llm.AdaptedResponse{}, err
	}
	if ctx.noTerminology {
		return llm.AdaptedResponse{Domain: response}, nil
	}
	raw, err := llm.NormalizeJSON(response)
	if err != nil {
		return llm.AdaptedResponse{}, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return llm.AdaptedResponse{}, fmt.Errorf("terminology: invalid response envelope")
	}
	domain := envelope["result"]
	if len(domain) == 0 || bytes.Equal(bytes.TrimSpace(domain), []byte("null")) {
		return llm.AdaptedResponse{}, fmt.Errorf("terminology: computed result is required")
	}
	result, err := jsonValue(domain)
	if err != nil {
		return llm.AdaptedResponse{}, fmt.Errorf("terminology: invalid original result")
	}
	adapted := llm.AdaptedResponse{Domain: domain}
	// Metadata never supplies domain authority. Keep one exact rejection count
	// per reason, with short positions pointing into the saved raw response.
	rejectionByReason := make(map[string]int)
	reject := func(reason, position string) {
		index, found := rejectionByReason[reason]
		if !found {
			index = len(adapted.Rejections)
			rejectionByReason[reason] = index
			adapted.Rejections = append(adapted.Rejections, llm.ResponseRejection{Kind: "terminology_metadata_rejected", Reason: reason})
		}
		rejection := &adapted.Rejections[index]
		rejection.Count++
		if len(rejection.Samples) < 5 {
			rejection.Samples = append(rejection.Samples, position)
		}
	}
	if object, ok := result.(map[string]any); ok && ctx.termsOnlyInEnvelope {
		if _, misplaced := object["terms"]; misplaced {
			// Discard only adjunct metadata misplaced beside the owner's fields.
			// Never move these definitions into the glossary, or relax validation
			// of another field. The exchange's raw response remains unchanged.
			delete(object, "terms")
			adapted.Domain, err = json.Marshal(object)
			if err != nil {
				return llm.AdaptedResponse{}, err
			}
			reject("misplaced optional terms field", "result.terms")
		}
	}
	var extra []string
	for field := range envelope {
		if field != "result" && field != "terms" {
			extra = append(extra, field)
		}
	}
	sort.Strings(extra)
	for range extra {
		reject("unknown optional envelope field", "envelope")
	}
	var wire []json.RawMessage
	if len(envelope["terms"]) == 0 {
		reject("missing optional terms array", "terms")
	} else if err := json.Unmarshal(envelope["terms"], &wire); err != nil || wire == nil {
		reject("invalid optional terms array", "terms")
		wire = nil
	}
	terms := make([]validatedTerm, 0, len(wire))
	sourceRows := make(map[string]bool)
	for _, source := range ctx.sources {
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
			source, ok := ctx.sources[*ref]
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
	digest := sha256.Sum256(request)
	requestID := hex.EncodeToString(digest[:])
	byRow := make(map[string][]int)
	for index, term := range terms {
		for _, row := range term.rows {
			byRow[row] = append(byRow[row], index)
		}
	}
	adapted.Accept = func(rows []string) {
		if rows == nil {
			c.accept(requestID, terms, nil)
			return
		}
		seen := make(map[int]bool)
		for _, row := range rows {
			for _, index := range byRow[row] {
				seen[index] = true
			}
		}
		indexes := make([]int, 0, len(seen))
		for index := range seen {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		selected := make([]validatedTerm, 0, len(indexes))
		for _, index := range indexes {
			selected = append(selected, terms[index])
		}
		c.accept(requestID, selected, rows)
	}
	return adapted, nil
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
			for _, child := range value {
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

func (c *Collector) accept(requestID string, terms []validatedTerm, rows []string) {
	allowed := make(map[string]bool)
	for _, row := range rows {
		allowed[row] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, term := range terms {
		var origins []Origin
		for _, row := range term.rows {
			if rows == nil || allowed[row] {
				origins = append(origins, Origin{RequestSHA256: requestID, Row: row})
			}
		}
		if len(origins) == 0 {
			continue
		}
		candidate := term.candidate
		// Keep the original term identity while separate row memos restore its
		// accepted sources. The same answer collected as one batch must produce
		// the same variant as that answer collected one row at a time.
		original := term.candidate
		for _, source := range term.sources {
			original.Sources = append(original.Sources, Source{Path: source.Path, Line: source.Line})
		}
		original.Sources = normalizeSources(original.Sources)
		raw, _ := json.Marshal(original)
		key := string(raw)
		seen := make(map[Source]bool)
		for _, source := range term.sources {
			if rows != nil && source.Row != "" && !allowed[source.Row] {
				continue
			}
			anchor := Source{Path: source.Path, Line: source.Line}
			if !seen[anchor] {
				candidate.Sources = append(candidate.Sources, anchor)
				seen[anchor] = true
			}
		}
		if len(candidate.Sources) == 0 {
			continue
		}
		stored, ok := c.values[key]
		if !ok {
			stored = candidate
		} else {
			stored.Sources = append(stored.Sources, candidate.Sources...)
		}
		stored.Sources = normalizeSources(stored.Sources)
		stored.Origins = normalizeOrigins(append(origins, stored.Origins...))
		c.values[key] = stored
	}
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
