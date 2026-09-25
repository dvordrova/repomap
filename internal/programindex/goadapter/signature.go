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
