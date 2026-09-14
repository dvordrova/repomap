package clojureproject

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// clj-kondo's EDN preserves native records even when internal reader metadata
// cannot be written as JSON. Decode the public fields consumed by this adapter;
// skip other fields as complete forms, just as json.Unmarshal ignores them.
// This is a transport decoder, not another Clojure analyzer.
type ednDecoder struct {
	text []rune
	at   int
}

var nativeFields = map[string]bool{
	"filename": true, "row": true, "col": true, "end-row": true, "end-col": true, "name-row": true, "name-col": true, "lang": true,
	"name": true, "from": true, "to": true, "ns": true, "defined-by": true, "defined-by->lint-as": true, "arglist-strs": true,
	"doc": true, "private": true, "macro": true, "from-var": true, "arity": true, "id": true, "scope-end-row": true, "scope-end-col": true,
	"class": true, "method-name": true, "call": true, "type": true, "message": true,
}
var nativeCollections = map[string]bool{
	"namespace-definitions": true, "namespace-usages": true, "var-definitions": true, "var-usages": true,
	"locals": true, "local-usages": true, "java-class-usages": true, "instance-invocations": true,
}

func decodeEDN(raw []byte, destination any) error {
	d := ednDecoder{text: []rune(string(raw))}
	value, err := d.value(0)
	if err != nil {
		return err
	}
	d.at = space(d.text, d.at)
	if d.at != len(d.text) {
		return fmt.Errorf("trailing native EDN data")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, destination)
}
func (d *ednDecoder) value(level int) (any, error) {
	d.at = space(d.text, d.at)
	if d.at >= len(d.text) {
		return nil, fmt.Errorf("unexpected end of native EDN")
	}
	switch d.text[d.at] {
	case '{':
		d.at++
		out := map[string]any{}
		for {
			d.at = space(d.text, d.at)
			if d.at >= len(d.text) {
				return nil, fmt.Errorf("unterminated native EDN map")
			}
			if d.text[d.at] == '}' {
				d.at++
				return out, nil
			}
			keyValue, err := d.scalar()
			if err != nil {
				return nil, err
			}
			key, ok := keyValue.(string)
			if !ok {
				return nil, fmt.Errorf("invalid native EDN key")
			}
			keep := nativeFields[key]
			next := 2
			if level == 0 {
				keep = key == "analysis" || key == "findings"
				if key == "analysis" {
					next = 1
				}
			}
			if level == 1 {
				keep = nativeCollections[key]
			}
			if !keep {
				d.at = space(d.text, d.at)
				_, end := readForm(d.text, d.at)
				if end <= d.at {
					return nil, fmt.Errorf("missing native EDN value")
				}
				d.at = end
				continue
			}
			value, err := d.value(next)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
	case '[':
		d.at++
		var out []any
		for {
			d.at = space(d.text, d.at)
			if d.at >= len(d.text) {
				return nil, fmt.Errorf("unterminated native EDN vector")
			}
			if d.text[d.at] == ']' {
				d.at++
				return out, nil
			}
			value, err := d.value(level)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
	default:
		return d.scalar()
	}
}
func (d *ednDecoder) scalar() (any, error) {
	d.at = space(d.text, d.at)
	if d.at >= len(d.text) {
		return nil, fmt.Errorf("missing native EDN scalar")
	}
	start := d.at
	if d.text[d.at] == '"' {
		_, d.at = readForm(d.text, d.at)
		return clojureString(string(d.text[start:d.at]))
	}
	for d.at < len(d.text) && !unicode.IsSpace(d.text[d.at]) && !strings.ContainsRune("{}[](),", d.text[d.at]) {
		d.at++
	}
	if d.at == start {
		return nil, fmt.Errorf("invalid native EDN scalar at %d", start)
	}
	value := string(d.text[start:d.at])
	switch value {
	case "nil":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(value, ":") {
		return value[1:], nil
	}
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		return integer, nil
	}
	return value, nil
}
