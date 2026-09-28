package report

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strconv"
	"strings"
)

// pageData is what the page's script reads beside the HTML, written once
// in one <script type="application/json" id="rm-page-data"> (the
// page-size study's M1-M4, same output and behaviour):
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
//     the script restores it.
//
// Values are registered by the page's templates as they are written
// (pageTemplateFuncs "pagedata"), in that order.
type pageData struct {
	base   string
	values []json.RawMessage
	value  map[string]int
	decls  []json.RawMessage
	decl   map[string]int
}

// pageDataLink stands for the base of the source links inside the page
// data; pageDataAttrLink does in an attribute holding a declaration's key.
const (
	pageDataLink     = "\u0001"
	pageDataAttrLink = "@"
)

// pageDataDecls are the readings whose top-level "decls" list declarations
// the page names once.
var pageDataDecls = map[string]bool{"reading": true, "inputPath": true, "catalogue": true, "launch": true}

func newPageData(base string) *pageData {
	return &pageData{base: base, value: map[string]int{}, decl: map[string]int{}}
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
	return strconv.Itoa(index), nil
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

// JSON is the page data as the page embeds it.
func (data *pageData) JSON() (template.JS, error) {
	encoded, err := json.Marshal(struct {
		Base   string            `json:"base,omitempty"`
		Decls  []json.RawMessage `json:"decls"`
		Values []json.RawMessage `json:"values"`
	}{data.base, orEmpty(data.decls), orEmpty(data.values)})
	if err != nil {
		return "", err
	}
	return template.JS(encoded), nil
}

func orEmpty(list []json.RawMessage) []json.RawMessage {
	if list == nil {
		return []json.RawMessage{}
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
