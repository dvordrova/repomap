package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode"
)

// NormalizeJSON accepts one object or array, optionally surrounded by
// whitespace, one Markdown fence or non-structural prose. Complete leading
// <think>...</think> blocks, repeated or nested, are separate from the answer.
// It may remove unmatched closing brackets, delete a crossed closer when both
// of its readings give the same value, append missing closing brackets at EOF
// outside strings, and close an unfinished string at EOF without completing
// escapes. After the root closes, a prose tail is discarded and an identical
// repeat counts once. Values, fields, refs and schemas remain unchanged; a
// different second value, a structural tail and an ambiguous closer are
// refused.
func NormalizeJSON(raw []byte) ([]byte, error) {
	trimmed, err := stripThinking(raw)
	if err != nil {
		return nil, err
	}
	if len(trimmed) == 0 {
		return nil, errors.New("llm: JSON response is empty")
	}
	if validJSONRoot(trimmed) {
		return cloneBytes(bytes.TrimSpace(trimmed)), nil
	}

	start := bytes.IndexAny(trimmed, "{[")
	fence := bytes.Index(trimmed, []byte("```"))
	if fence >= 0 && (start < 0 || fence < start) {
		return normalizeFencedJSON(trimmed, fence)
	}
	if start < 0 {
		return nil, errors.New("llm: response contains no JSON object or array")
	}
	// Keep the entire first candidate, including its tail. Never hunt for a
	// valid nested value inside a malformed outer answer.
	root, err := readJSONRoot(trimmed[start:])
	if err == nil {
		return root.value, nil
	}
	// A bracket in a sentence before the fence is prose, not a competing answer.
	if fence > start && !containsJSONValue(trimmed[:fence]) {
		return normalizeFencedJSON(trimmed, fence)
	}
	return nil, err
}

// stripThinking removes every complete leading <think> block. A block closes
// at the </think> that balances its nested openers; without one the answer
// boundary is unknown and the response is refused.
func stripThinking(raw []byte) ([]byte, error) {
	open, close := []byte("<think>"), []byte("</think>")
	trimmed := bytes.TrimLeftFunc(raw, unicode.IsSpace)
	for bytes.HasPrefix(trimmed, open) {
		depth, at := 0, 0
		for {
			nextClose := bytes.Index(trimmed[at:], close)
			if nextClose < 0 {
				return nil, errors.New("llm: response thinking block is incomplete")
			}
			if nextOpen := bytes.Index(trimmed[at:], open); nextOpen >= 0 && nextOpen < nextClose {
				depth++
				at += nextOpen + len(open)
				continue
			}
			depth--
			at += nextClose + len(close)
			if depth == 0 {
				break
			}
		}
		trimmed = bytes.TrimLeftFunc(trimmed[at:], unicode.IsSpace)
	}
	return trimmed, nil
}

// normalizeFencedJSON reads the fence at raw[open:]. The fence tag and an
// inline layout carry no content. Prose may precede the fence and follow its
// close; a second fence must repeat the same value.
func normalizeFencedJSON(raw []byte, open int) ([]byte, error) {
	if containsJSONValue(raw[:open]) {
		return nil, errors.New("llm: fenced response contains ambiguous JSON")
	}
	root, rest, err := readJSONFence(raw[open:])
	if err != nil {
		return nil, err
	}
	for {
		rest = bytes.TrimLeftFunc(rest, unicode.IsSpace)
		if len(rest) == 0 || proseTail(rest) {
			return root, nil
		}
		next := bytes.Index(rest, []byte("```"))
		if next < 0 || !proseTail(rest[:next]) {
			return nil, errors.New("llm: fenced response contains trailing or ambiguous data")
		}
		again, after, err := readJSONFence(rest[next:])
		if err != nil || !sameJSONValue(root, again) {
			return nil, errors.New("llm: fenced response contains trailing or ambiguous data")
		}
		rest = after
	}
}

// readJSONFence reads one fence that starts at raw[0]. It returns the root and
// the bytes after the closing fence, which are empty for an unclosed fence.
func readJSONFence(raw []byte) ([]byte, []byte, error) {
	body := raw[len("```"):]
	info := body
	lineEnd := bytes.IndexByte(body, '\n')
	if lineEnd >= 0 {
		info = body[:lineEnd]
	}
	var content []byte
	if inline := bytes.IndexAny(info, "{["); inline >= 0 {
		content = body[inline:]
	} else if lineEnd >= 0 {
		content = body[lineEnd+1:]
	} else {
		return nil, nil, errors.New("llm: JSON fence is incomplete")
	}
	var rest []byte
	closeOffset := bytes.Index(content, []byte("```"))
	if closeOffset >= 0 {
		rest = content[closeOffset+len("```"):]
		content = content[:closeOffset]
	}
	root, err := readJSONRoot(bytes.TrimLeftFunc(content, unicode.IsSpace))
	if err != nil {
		return nil, nil, errors.New("llm: fenced response does not contain one complete JSON object or array")
	}
	// A string still open at the closing fence may have contained that fence.
	if root.closedString && len(bytes.TrimSpace(rest)) != 0 {
		return nil, nil, errors.New("llm: fenced response contains trailing or ambiguous data")
	}
	return root.value, rest, nil
}

type jsonRoot struct {
	value []byte
	// closedString reports that an unfinished string received its quote at EOF.
	closedString bool
}

// maxCrossedReadings bounds the readings compared for crossed closers. Each
// crossed closer doubles the readings; past this bound the answer is refused
// as ambiguous rather than searched further.
const maxCrossedReadings = 64

func readJSONRoot(raw []byte) (jsonRoot, error) {
	remaining := maxCrossedReadings
	return readJSONRootWithin(raw, &remaining)
}

// readJSONRootWithin balances only delimiters whose role is unambiguous. A
// closer with no opener of its kind in the stack becomes whitespace. A closer
// whose opener lies deeper in the stack is crossed: deleting it and closing
// the inner brackets before it are both readings, and it is deleted only when
// the second reading is invalid or gives the same value.
func readJSONRootWithin(raw []byte, remaining *int) (jsonRoot, error) {
	invalid := errors.New("llm: response contains an incomplete or invalid JSON value")
	if *remaining--; *remaining < 0 {
		return jsonRoot{}, errors.New("llm: response contains too many crossed JSON brackets")
	}
	if validJSONRoot(raw) {
		return jsonRoot{value: cloneBytes(bytes.TrimSpace(raw))}, nil
	}
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return jsonRoot{}, invalid
	}
	out := make([]byte, 0, len(raw)+8)
	var stack []byte
	quoted, escaped := false, false
	for i, ch := range raw {
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			out = append(out, ch)
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			opener := byte('{')
			if ch == ']' {
				opener = '['
			}
			if len(stack) > 0 && stack[len(stack)-1] == opener {
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					break
				}
				out = append(out, ch)
				if !validJSONRoot(out) || !acceptableTail(out, raw[i+1:]) {
					return jsonRoot{}, invalid
				}
				return jsonRoot{value: out}, nil
			}
			match := bytes.LastIndexByte(stack, opener)
			if match < 0 {
				// Keep tokens separated: [1}2] must not become [12].
				out = append(out, ' ')
				continue
			}
			return readCrossedCloser(raw, i, stack[match+1:], remaining)
		}
		out = append(out, ch)
	}
	root := jsonRoot{}
	if quoted {
		if escaped {
			return jsonRoot{}, invalid
		}
		// Complete only the EOF delimiter. Never choose a quote position
		// inside the text or reinterpret bracket-looking string contents.
		out = append(out, '"')
		root.closedString = true
	}
	out = appendClosers(out, stack)
	if !validJSONRoot(out) {
		return jsonRoot{}, invalid
	}
	root.value = out
	return root, nil
}

// readCrossedCloser compares the two readings of the closer at raw[at]:
// deleted, or preceded by closers for the inner brackets. Only the deletion
// is ever returned; the second reading is only compared.
func readCrossedCloser(raw []byte, at int, inner []byte, remaining *int) (jsonRoot, error) {
	deleted := append(append(cloneBytes(raw[:at]), ' '), raw[at+1:]...)
	root, err := readJSONRootWithin(deleted, remaining)
	if err != nil {
		return jsonRoot{}, err
	}
	closed := append(appendClosers(cloneBytes(raw[:at]), inner), raw[at:]...)
	other, err := readJSONRootWithin(closed, remaining)
	if err == nil && !sameJSONValue(root.value, other.value) {
		return jsonRoot{}, errors.New("llm: response contains ambiguous crossed JSON brackets")
	}
	if *remaining < 0 {
		return jsonRoot{}, errors.New("llm: response contains too many crossed JSON brackets")
	}
	return root, nil
}

func appendClosers(out, stack []byte) []byte {
	for i := len(stack) - 1; i >= 0; i-- {
		closer := byte('}')
		if stack[i] == '[' {
			closer = ']'
		}
		out = append(out, closer)
	}
	return out
}

// acceptableTail reads what follows a closed root: stray closers, prose with
// no structural character, or further roots that repeat the first exactly.
func acceptableTail(root, tail []byte) bool {
	for {
		tail = bytes.TrimLeftFunc(tail, unicode.IsSpace)
		if len(bytes.Trim(tail, "}] \t\r\n")) == 0 || proseTail(tail) {
			return true
		}
		if tail[0] != '{' && tail[0] != '[' {
			return false
		}
		decoder := json.NewDecoder(bytes.NewReader(tail))
		var next json.RawMessage
		if decoder.Decode(&next) != nil || !sameJSONValue(root, next) {
			return false
		}
		tail = tail[decoder.InputOffset():]
	}
}

// proseTail reports text that cannot hide a JSON value or a fence. A tail that
// starts like a continued member or a second value (`, "b": 2`) is not prose.
func proseTail(raw []byte) bool {
	trimmed := bytes.TrimLeftFunc(raw, unicode.IsSpace)
	if len(trimmed) > 0 && bytes.IndexByte([]byte(`,:"`), trimmed[0]) >= 0 {
		return false
	}
	return bytes.IndexAny(raw, "{}[]") < 0 && !bytes.Contains(raw, []byte("```"))
}

// containsJSONValue reports a complete object or array anywhere in raw.
func containsJSONValue(raw []byte) bool {
	for at := 0; at < len(raw); at++ {
		if raw[at] != '{' && raw[at] != '[' {
			continue
		}
		var value json.RawMessage
		if json.NewDecoder(bytes.NewReader(raw[at:])).Decode(&value) == nil {
			return true
		}
	}
	return false
}

func sameJSONValue(left, right []byte) bool {
	decode := func(raw []byte) (any, bool) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		return value, decoder.Decode(&value) == nil
	}
	first, ok := decode(left)
	if !ok {
		return false
	}
	second, ok := decode(right)
	return ok && reflect.DeepEqual(first, second)
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
	if validate != nil {
		if err := validate(value); err != nil {
			return value, fmt.Errorf("validate JSON: %w", err)
		}
	}
	return value, nil
}
