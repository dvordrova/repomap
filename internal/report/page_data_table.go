package report

import (
	"bytes"
	"encoding/json"
	"html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// pageData is what the page's script reads beside the HTML, written once
// in one <script type="application/json" id="rm-page-data"> (the
// page-size study's M1-M4, same output and behaviour). The page needs its
// script: every answer the reading column, the canvas and the component
// pages show is in this data (owner, 2026-09-29), each fact once:
//
//   - each value an element refers to by its index (data-reading="12"),
//     once however many elements share it: a catalogue's reading, shared
//     by its 148 members, had been written 148 times, and the JSON in
//     attributes paid &#34; for every quote;
//   - the declarations a part's, an input's, a catalogue's or a launch's
//     reading names, each once and referred to by its index: Redis's 189
//     inputs carried 4,966 copies of 831 declarations;
//   - the base of the source links (host, repository and revision), once:
//     a link inside the data is written as pageDataLink and the rest, and
//     the script restores it;
//   - a link its place says (a site's "redis.c:9068" is its link to line
//     9068) is written as 1, a declaration's link to all its lines as the
//     number of its last line: 7,900 sites had carried each line twice;
//   - a call names the declarations at its ends by their index in the
//     declarations, a relation's names only when they are not its
//     declarations' names (a callable written inline, "Start (inline, 2)";
//     an outside symbol) and its kind only when it is not "calls"; a tile
//     names the declaration it draws the same way; a reading's call is a
//     call unless it says otherwise;
//   - any part of a value written again elsewhere (an input's writes, an
//     anchor, a chain every input of a dispatch site shares) is written
//     once in "shared" and referred to as {"$": index}: 98 inputs' state
//     changes had repeated 1,518 writes 10,396 times.
//
// Values are registered by the page's templates as they are written
// (pageTemplateFuncs "pagedata"), in that order; the compaction is done
// once the page is written (JSON), and the page's script (10-ui.js
// rmPage) reads each value back exactly as it was registered.
type pageData struct {
	base string
	// rangeSep writes a link to several lines: GitHub's #L10-L20, GitLab's
	// #L10-20.
	rangeSep string
	values   []json.RawMessage
	value    map[string]int
	names    map[int][]string
	decls    []json.RawMessage
	decl     map[string]int
}

// pageDataLink stands for the base of the source links inside the page
// data; pageDataAttrLink does in an attribute holding a declaration's key.
const (
	pageDataLink     = "\u0001"
	pageDataAttrLink = "@"
)

// pageDataDecls are the readings whose top-level "decls" list declarations
// the page names once.
var pageDataDecls = map[string]bool{"reading": true, "inputPath": true, "catalogue": true, "launch": true, "reached": true, "files": true}

func newPageData(base string) *pageData {
	return newPageDataRange(base, "-L")
}

func newPageDataRange(base, rangeSep string) *pageData {
	return &pageData{base: base, rangeSep: rangeSep, value: map[string]int{}, names: map[int][]string{}, decl: map[string]int{}}
}

// ref registers one value an element's attribute refers to under `name`
// (its dataset name) and returns its index, "" for no value.
func (data *pageData) ref(name, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	value, err := decodePageValue(raw)
	if err != nil {
		return "", err
	}
	if object, isObject := value.(map[string]any); isObject && pageDataDecls[name] {
		if list, listed := object["decls"].([]any); listed {
			for position, item := range list {
				if decl, isDecl := item.(map[string]any); isDecl {
					index, err := data.intern(decl, &data.decls, data.decl)
					if err != nil {
						return "", err
					}
					list[position] = index
				}
			}
		}
	}
	index, err := data.intern(value, &data.values, data.value)
	if err != nil {
		return "", err
	}
	if !contains(data.names[index], name) {
		data.names[index] = append(data.names[index], name)
	}
	return strconv.Itoa(index), nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// intern writes a value once, its links shortened, and returns its index.
func (data *pageData) intern(value any, table *[]json.RawMessage, seen map[string]int) (int, error) {
	encoded, err := json.Marshal(data.shorten(value))
	if err != nil {
		return 0, err
	}
	if index, known := seen[string(encoded)]; known {
		return index, nil
	}
	seen[string(encoded)] = len(*table)
	*table = append(*table, encoded)
	return len(*table) - 1, nil
}

func (data *pageData) shorten(value any) any {
	switch typed := value.(type) {
	case string:
		if data.base != "" && strings.HasPrefix(typed, data.base) {
			return pageDataLink + typed[len(data.base):]
		}
	case []any:
		for i := range typed {
			typed[i] = data.shorten(typed[i])
		}
	case map[string]any:
		for key, item := range typed {
			typed[key] = data.shorten(item)
		}
	}
	return value
}

// attrLink shortens a link written in an attribute the script compares with
// the page data's keys (a connection row's data-from-decl).
func (data *pageData) attrLink(link string) string {
	if data.base != "" && strings.HasPrefix(link, data.base) {
		return pageDataAttrLink + link[len(data.base):]
	}
	return link
}

// JSON is the page data as the page embeds it, compacted.
func (data *pageData) JSON() (template.JS, error) {
	decls := make([]any, len(data.decls))
	for i, raw := range data.decls {
		value, err := decodePageValue(string(raw))
		if err != nil {
			return "", err
		}
		decls[i] = value
	}
	values := make([]any, len(data.values))
	for i, raw := range data.values {
		value, err := decodePageValue(string(raw))
		if err != nil {
			return "", err
		}
		values[i] = value
	}
	compact := pageDataCompaction{data: data, decls: decls, byKey: map[string]int{}}
	for i, decl := range decls {
		if key := declKeyOf(decl); key != "" {
			if _, known := compact.byKey[key]; !known {
				compact.byKey[key] = i
			}
		}
	}
	for i := range values {
		for _, name := range data.names[i] {
			values[i] = compact.byName(name, values[i])
		}
	}
	if data.base != "" {
		for i := range decls {
			decls[i] = compact.links(decls[i])
		}
		for i := range values {
			values[i] = compact.links(values[i])
		}
	}
	shared := newPageShared(append(append([]any{}, decls...), values...))
	for i := range decls {
		decls[i] = shared.replace(decls[i], true)
	}
	for i := range values {
		values[i] = shared.replace(values[i], true)
	}
	encoded, err := json.Marshal(struct {
		Base   string `json:"base,omitempty"`
		Range  string `json:"range,omitempty"`
		Decls  []any  `json:"decls"`
		Values []any  `json:"values"`
		Shared []any  `json:"shared,omitempty"`
	}{data.base, data.rangeSep, orEmpty(decls), orEmpty(values), shared.table})
	if err != nil {
		return "", err
	}
	return template.JS(encoded), nil
}

func orEmpty(list []any) []any {
	if list == nil {
		return []any{}
	}
	return list
}

// decodePageValue reads a value keeping its numbers as written.
func decodePageValue(raw string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// declKeyOf is a declaration's key as the page's script keys it: its key,
// else its link.
func declKeyOf(decl any) string {
	object, _ := decl.(map[string]any)
	if key, _ := object["key"].(string); key != "" {
		return key
	}
	href, _ := object["href"].(string)
	return href
}

type pageDataCompaction struct {
	data  *pageData
	decls []any
	byKey map[string]int
}

func (compact pageDataCompaction) declName(index int) string {
	object, _ := compact.decls[index].(map[string]any)
	name, _ := object["name"].(string)
	return name
}

// byName writes a value of one kind the short way its reader restores.
func (compact pageDataCompaction) byName(name string, value any) any {
	switch name {
	case "calls":
		if list, isList := value.([]any); isList {
			for _, item := range list {
				if call, isCall := item.(map[string]any); isCall {
					compact.call(call)
				}
			}
		}
	case "symbols":
		if list, isList := value.([]any); isList {
			for _, item := range list {
				if symbol, isSymbol := item.(map[string]any); isSymbol {
					compact.symbol(symbol)
				}
			}
		}
	case "reading":
		compact.readingKinds(value)
	}
	return value
}

// call names the declarations at a call's ends by index and, for a relation
// between two named ends (one with neither words nor a handler's name),
// drops each name its declaration says and the kind "calls". The script
// restores the caller's and callee's keys, the names and the kind.
func (compact pageDataCompaction) call(call map[string]any) {
	if _, numbered := call["caller"].(json.Number); numbered {
		return
	}
	caller, _ := call["caller"].(string)
	callee, hasCallee := call["callee"].(string)
	to, _ := call["to"].(string)
	callerAt, callerKnown := compact.byKey[caller]
	// The callee is where the call lands unless it says otherwise.
	landing := to
	if hasCallee {
		landing = callee
	}
	calleeAt, calleeKnown := compact.byKey[landing]
	if _, said := call["label"]; !said && call["name"] == nil {
		if call["kind"] == "calls" {
			delete(call, "kind")
		}
		if callerKnown && call["caller_name"] == compact.declName(callerAt) {
			delete(call, "caller_name")
		}
		if calleeKnown && call["callee_name"] == compact.declName(calleeAt) {
			delete(call, "callee_name")
		}
	}
	if callerKnown {
		call["caller"] = json.Number(strconv.Itoa(callerAt))
	}
	if hasCallee {
		if calleeKnown && callee != "" {
			call["callee"] = json.Number(strconv.Itoa(calleeAt))
		}
	} else if calleeKnown && to != "" {
		call["to"] = json.Number(strconv.Itoa(calleeAt))
	}
}

// symbol names the declaration a tile draws by its index when the tile
// says what the declaration says (its name, link, code and file) and only
// adds to it (its signature, kind, owner): the script takes those four from
// the declaration.
func (compact pageDataCompaction) symbol(symbol map[string]any) {
	href, _ := symbol["href"].(string)
	at, known := compact.byKey[href]
	if href == "" || !known {
		return
	}
	decl, _ := compact.decls[at].(map[string]any)
	if symbol["name"] != decl["name"] || symbol["href"] != decl["href"] || symbol["code"] != decl["code"] || symbol["path"] != any(placePath(decl)) || placePath(decl) == "" {
		return
	}
	for _, field := range []string{"name", "href", "code", "path"} {
		delete(symbol, field)
	}
	symbol["d"] = json.Number(strconv.Itoa(at))
}

// placePath is the file of a declaration's place ("redis.c:1250").
func placePath(decl map[string]any) string {
	for _, field := range []string{"at", "source"} {
		if text, _ := decl[field].(string); text != "" {
			path, _ := parsePlace(text)
			return path
		}
	}
	return ""
}

// pageDataEnds are the lists of a reading whose items are a relation's
// ends: a line's and a part's group's.
var pageDataEnds = map[string]bool{"ends": true, "decls": true}

// readingKinds leaves a relation's end's kind out when it is a call.
func (compact pageDataCompaction) readingKinds(value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			compact.readingKinds(item)
		}
	case map[string]any:
		for key, item := range typed {
			if list, isList := item.([]any); isList && pageDataEnds[key] {
				for _, end := range list {
					if object, isObject := end.(map[string]any); isObject && object["kind"] == "calls" {
						delete(object, "kind")
					}
				}
			}
			compact.readingKinds(item)
		}
	}
}

// pagePlace reads a place as the page writes it: "path:line" or
// "path:line:column", else a path alone.
var pagePlace = regexp.MustCompile(`^(.*?):(\d+)(?::(\d+))?$`)

func parsePlace(text string) (string, int) {
	match := pagePlace.FindStringSubmatch(text)
	if match == nil {
		return text, 0
	}
	line, _ := strconv.Atoi(match[2])
	return match[1], line
}

// pageDataLinkPlaces are the links a place beside them says, with the
// field holding that place: a site's href by its "at", a call's by its
// "at", a call step's by its "source", an anchor's by its "Text".
var pageDataLinkPlaces = []struct{ link, place, code string }{
	{"href", "at", "code"}, {"href", "source", "code"}, {"from", "at", ""}, {"Href", "Text", "Code"},
}

// links writes a link its place says as 1 and a link to all of a
// declaration's lines as its last line.
func (compact pageDataCompaction) links(value any) any {
	switch typed := value.(type) {
	case []any:
		for i := range typed {
			typed[i] = compact.links(typed[i])
		}
	case map[string]any:
		for key, item := range typed {
			typed[key] = compact.links(item)
		}
		for _, pair := range pageDataLinkPlaces {
			place, _ := typed[pair.place].(string)
			if place == "" {
				continue
			}
			path, line := parsePlace(place)
			said := pageDataLink + path
			if line > 0 {
				said += "#L" + strconv.Itoa(line)
			}
			if link, _ := typed[pair.link].(string); link == said {
				typed[pair.link] = json.Number("1")
			}
			// An anchor with no editor action says so by leaving it out.
			if pair.place == "Text" && typed["Open"] == "" {
				delete(typed, "Open")
			}
			if pair.code == "" || line == 0 {
				continue
			}
			if code, _ := typed[pair.code].(string); strings.HasPrefix(code, said+compact.data.rangeSep) {
				if end, err := strconv.Atoi(code[len(said+compact.data.rangeSep):]); err == nil && end > line && strconv.Itoa(end) == code[len(said+compact.data.rangeSep):] {
					typed[pair.code] = json.Number(strconv.Itoa(end))
				}
			}
		}
	}
	return value
}

// pageShared writes once, in its table, any part of the page data written
// more than once, and refers to it as {"$": index}.
type pageShared struct {
	counts map[string]int
	index  map[string]int
	table  []any
}

// pageSharedLeast is the shortest part worth a reference ({"$":12345}).
const pageSharedLeast = 16

func newPageShared(roots []any) *pageShared {
	shared := &pageShared{counts: map[string]int{}, index: map[string]int{}}
	var count func(any)
	count = func(value any) {
		switch typed := value.(type) {
		case []any:
			shared.counts[pageDataKey(value)]++
			for _, item := range typed {
				count(item)
			}
		case map[string]any:
			shared.counts[pageDataKey(value)]++
			for _, item := range typed {
				count(item)
			}
		case string:
			if len(typed) >= pageSharedLeast {
				shared.counts["\x00"+typed]++
			}
		}
	}
	for _, root := range roots {
		count(root)
	}
	return shared
}

func pageDataKey(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// replace writes a value with its repeated parts as references; a root is
// never one itself.
func (shared *pageShared) replace(value any, root bool) any {
	key := ""
	switch typed := value.(type) {
	case string:
		if len(typed) < pageSharedLeast || shared.counts["\x00"+typed] < 2 {
			return value
		}
		return shared.reference("\x00"+typed, typed)
	case []any:
		key = pageDataKey(value)
		written := make([]any, len(typed))
		for i, item := range typed {
			written[i] = shared.replace(item, false)
		}
		value = written
	case map[string]any:
		key = pageDataKey(value)
		written := make(map[string]any, len(typed))
		// In the order the JSON writes the fields, so the table is the same
		// on every render.
		fields := make([]string, 0, len(typed))
		for field := range typed {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			written[field] = shared.replace(typed[field], false)
		}
		value = written
	default:
		return value
	}
	if root || shared.counts[key] < 2 || len(key) < pageSharedLeast {
		return value
	}
	return shared.reference(key, value)
}

func (shared *pageShared) reference(key string, value any) any {
	at, known := shared.index[key]
	if !known {
		at = len(shared.table)
		shared.index[key] = at
		shared.table = append(shared.table, value)
	}
	return map[string]any{"$": at}
}
