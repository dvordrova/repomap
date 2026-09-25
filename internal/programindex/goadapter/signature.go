package goadapter

import (
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/programindex"
)

// shortSignature drops module paths from a Go signature: a reader says
// corpus.Entry, not github.com/owner/repo/internal/corpus.Entry.
func shortSignature(signature string) string {
	if !strings.Contains(signature, "/") {
		return signature
	}
	var out strings.Builder
	token := strings.Builder{}
	flush := func() {
		text := token.String()
		if slash := strings.LastIndex(text, "/"); slash >= 0 && strings.Contains(text[slash:], ".") {
			text = text[slash+1:]
		}
		out.WriteString(text)
		token.Reset()
	}
	for _, r := range signature {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == '.' || r == '_' || r == '-' {
			token.WriteRune(r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

// typeSignature keeps what a named type is, not its members: fields and
// interface methods are objects of their own. "type pkg.User struct{...}"
// becomes "struct"; "type pkg.Status string" becomes "string". A generic
// type keeps its whole type-parameter list, whose constraints may hold
// spaces and brackets of their own: "type pkg.Page[K comparable, V
// map[string]int] struct{...}" becomes "[K comparable, V map[string]int] struct".
func typeSignature(signature string) string {
	rest := strings.TrimPrefix(signature, "type ")
	end := strings.IndexAny(rest, "[ ")
	if end < 0 {
		return shortSignature(signature)
	}
	parameters := ""
	if rest[end] == '[' {
		depth := 0
		for i := end; i < len(rest); i++ {
			if rest[i] == '"' {
				// A struct tag inside a constraint may hold brackets.
				if quoted, err := strconv.QuotedPrefix(rest[i:]); err == nil {
					i += len(quoted) - 1
				}
				continue
			}
			if rest[i] == '[' {
				depth++
			} else if rest[i] == ']' {
				if depth--; depth == 0 {
					parameters = shortSignature(rest[end:i+1]) + " "
					end = i + 1
					break
				}
			}
		}
	}
	rest = strings.TrimPrefix(rest[end:], " ")
	switch {
	case strings.HasPrefix(rest, "struct{"):
		return parameters + "struct"
	case strings.HasPrefix(rest, "interface{"):
		return parameters + "interface"
	}
	return parameters + shortSignature(rest)
}

// tagAliases reads a struct tag's format names: `json:"count,omitempty"
// db:"count"` names the field count in json and db. Options after the comma
// and the "-" exclusion are not names.
func tagAliases(tag string) []programindex.Alias {
	var result []programindex.Alias
	for tag != "" {
		tag = strings.TrimLeft(tag, " ")
		colon := strings.Index(tag, `:"`)
		if colon <= 0 || strings.ContainsAny(tag[:colon], " \"") {
			break
		}
		format := tag[:colon]
		value, err := strconv.QuotedPrefix(tag[colon+1:])
		if err != nil {
			break
		}
		tag = tag[colon+1+len(value):]
		name, _ := reflect.StructTag(format + ":" + value).Lookup(format)
		name, _, _ = strings.Cut(name, ",")
		if name != "" && name != "-" {
			result = append(result, programindex.Alias{Format: format, Name: name})
		}
	}
	return result
}
