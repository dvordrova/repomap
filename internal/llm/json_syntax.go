package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// NormalizeJSON accepts one complete object or array, optionally surrounded
// by whitespace, one Markdown JSON fence, or non-structural leading prose.
// One complete leading <think>...</think> block is separate from the answer.
// It may remove unmatched closing brackets and append missing closing brackets
// at EOF, outside strings. Values, fields, refs and schemas remain unchanged;
// crossed nesting, multiple values and trailing prose are still refused.
func NormalizeJSON(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.HasPrefix(trimmed, []byte("<think>")) {
		end := bytes.Index(trimmed, []byte("</think>"))
		if end < 0 {
			return nil, errors.New("llm: response thinking block is incomplete")
		}
		if bytes.Contains(trimmed[len("<think>"):end], []byte("<think>")) {
			return nil, errors.New("llm: response thinking blocks are nested")
		}
		trimmed = bytes.TrimSpace(trimmed[end+len("</think>"):])
		if bytes.HasPrefix(trimmed, []byte("<think>")) {
			return nil, errors.New("llm: response contains multiple thinking blocks")
		}
	}
	if len(trimmed) == 0 {
		return nil, errors.New("llm: JSON response is empty")
	}
	if validJSONRoot(trimmed) {
		return cloneBytes(trimmed), nil
	}

	start := bytes.IndexAny(trimmed, "{[")
	if fence := bytes.Index(trimmed, []byte("```")); fence >= 0 && (start < 0 || fence < start) {
		return normalizeFencedJSON(trimmed)
	}

	if start < 0 {
		return nil, errors.New("llm: response contains no JSON object or array")
	}
	// Keep the entire first candidate, including its tail. Never hunt for a
	// valid nested value inside a malformed outer answer.
	if candidate, ok := balanceJSONRoot(trimmed[start:]); ok {
		return candidate, nil
	}
	return nil, errors.New("llm: response contains an incomplete or invalid JSON value")
}

func normalizeFencedJSON(raw []byte) ([]byte, error) {
	open := bytes.Index(raw, []byte("```"))
	if open < 0 {
		return nil, errors.New("llm: response contains no JSON fence")
	}
	// The prefix may be a short provider preamble, but it must not contain a
	// competing structural candidate.
	if bytes.IndexAny(raw[:open], "{[") >= 0 {
		return nil, errors.New("llm: fenced response contains ambiguous JSON")
	}
	afterOpen := raw[open+3:]
	lineEnd := bytes.IndexByte(afterOpen, '\n')
	if lineEnd < 0 {
		return nil, errors.New("llm: JSON fence is incomplete")
	}
	language := strings.TrimSpace(string(afterOpen[:lineEnd]))
	if language != "" && !strings.EqualFold(language, "json") {
		return nil, errors.New("llm: response fence is not JSON")
	}
	contentAndClose := afterOpen[lineEnd+1:]
	closeOffset := bytes.Index(contentAndClose, []byte("```"))
	content := contentAndClose
	if closeOffset >= 0 {
		content = contentAndClose[:closeOffset]
		if len(bytes.TrimSpace(contentAndClose[closeOffset+3:])) != 0 {
			return nil, errors.New("llm: fenced response contains trailing or ambiguous data")
		}
	}
	if normalized, ok := balanceJSONRoot(bytes.TrimSpace(content)); ok {
		return normalized, nil
	}
	return nil, errors.New("llm: fenced response does not contain one complete JSON object or array")
}

// Balance only delimiters whose role is unambiguous. A mismatched closer with
// an opener deeper in the stack is crossed nesting, not an extra delimiter.
func balanceJSONRoot(raw []byte) ([]byte, bool) {
	if validJSONRoot(raw) {
		return cloneBytes(raw), true
	}
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return nil, false
	}
	out := make([]byte, 0, len(raw))
	var stack []byte
	objects, arrays := 0, 0
	quoted, escaped := false, false
	for _, ch := range raw {
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
		} else {
			switch ch {
			case '"':
				quoted = true
			case '{', '[':
				stack = append(stack, ch)
				if ch == '{' {
					objects++
				} else {
					arrays++
				}
			case '}', ']':
				opener := byte('{')
				open := &objects
				if ch == ']' {
					opener = '['
					open = &arrays
				}
				if len(stack) > 0 && stack[len(stack)-1] == opener {
					stack = stack[:len(stack)-1]
					*open--
				} else if *open > 0 {
					return nil, false
				} else {
					// Keep tokens separated: [1}2] must not become [12].
					out = append(out, ' ')
					continue
				}
			}
		}
		out = append(out, ch)
	}
	if quoted {
		return nil, false
	}
	for i := len(stack) - 1; i >= 0; i-- {
		closer := byte('}')
		if stack[i] == '[' {
			closer = ']'
		}
		out = append(out, closer)
	}
	return out, validJSONRoot(out)
}

func validJSONRoot(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed)
}

func decodeJSONValue[T any](raw []byte, validate func(T) error) (T, error) {
	var value T
	normalized, err := NormalizeJSON(raw)
	if err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return value, errors.New("decode JSON: multiple values")
		}
		return value, fmt.Errorf("decode JSON tail: %w", err)
	}
	if validate != nil {
		if err := validate(value); err != nil {
			return value, fmt.Errorf("validate JSON: %w", err)
		}
	}
	return value, nil
}
