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
// by whitespace, one Markdown JSON fence, or non-structural leading prose. It
// rejects multiple values, trailing prose or delimiters, and truncated roots.
// It does not repair fields, refs, schemas, values, or malformed JSON.
func NormalizeJSON(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("llm: JSON response is empty")
	}
	if validJSONRoot(trimmed) {
		return cloneBytes(trimmed), nil
	}

	if bytes.Contains(trimmed, []byte("```")) {
		return normalizeFencedJSON(trimmed)
	}

	start := bytes.IndexAny(trimmed, "{[")
	if start < 0 {
		return nil, errors.New("llm: response contains no JSON object or array")
	}
	// Once the first structural opener appears, its complete value is the
	// only candidate. Hunting inside a malformed outer value would turn
	// truncation into implicit semantic repair.
	decoder := json.NewDecoder(bytes.NewReader(trimmed[start:]))
	var candidate json.RawMessage
	if err := decoder.Decode(&candidate); err != nil {
		return nil, errors.New("llm: response contains an incomplete or invalid JSON value")
	}
	candidate = bytes.TrimSpace(candidate)
	if !validJSONRoot(candidate) {
		return nil, errors.New("llm: response JSON root must be an object or array")
	}
	consumed := int(decoder.InputOffset())
	if consumed < 0 || consumed > len(trimmed[start:]) {
		return nil, errors.New("llm: response JSON boundary is invalid")
	}
	if len(bytes.TrimSpace(trimmed[start+consumed:])) != 0 {
		return nil, errors.New("llm: response contains trailing or ambiguous data")
	}
	return cloneBytes(candidate), nil
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
	if closeOffset < 0 {
		// Some providers omit only the closing Markdown delimiter while still
		// returning one complete JSON root. Accept that harmless presentation
		// defect, but never infer a missing JSON byte or discard trailing prose.
		content := bytes.TrimSpace(contentAndClose)
		if bytes.Contains(content, []byte("```")) || !validJSONRoot(content) {
			return nil, errors.New("llm: JSON fence is incomplete")
		}
		return cloneBytes(content), nil
	}
	content := bytes.TrimSpace(contentAndClose[:closeOffset])
	suffix := contentAndClose[closeOffset+3:]
	if len(bytes.TrimSpace(suffix)) != 0 {
		return nil, errors.New("llm: fenced response contains trailing or ambiguous data")
	}
	if bytes.Contains(content, []byte("```")) || !validJSONRoot(content) {
		return nil, errors.New("llm: fenced response does not contain one complete JSON object or array")
	}
	return cloneBytes(content), nil
}

func validJSONRoot(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') || !json.Valid(trimmed) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
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
