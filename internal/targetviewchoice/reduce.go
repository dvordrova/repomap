package targetviewchoice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dvordrova/repomap/internal/secretscan"
)

// ResolveResponse accepts exactly one supplied request-local ref and restores
// the complete chosen view from local authority. It never guesses, repairs,
// fuzzy-matches, or ranks an invalid response.
func (cube Cube) ResolveResponse(raw []byte) (Selection, error) {
	if err := validateCube(cube); err != nil {
		return Selection{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Selection{}, fmt.Errorf("target view choice: response exceeds bounded envelope")
	}
	if _, found := secretscan.Detect(string(raw)); found {
		return Selection{}, fmt.Errorf("target view choice: response contains credential-shaped content")
	}
	response, err := decodeResponse(raw)
	if err != nil {
		return Selection{}, err
	}
	view, known := cube.authority[response.DefaultViewRef]
	if !known {
		return Selection{}, fmt.Errorf("target view choice: response cites unknown default_view_ref")
	}
	return Selection{
		DefaultViewRef: response.DefaultViewRef,
		DefaultView:    cloneView(view),
	}, nil
}

func decodeResponse(raw []byte) (Response, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return Response{}, fmt.Errorf("target view choice: invalid JSON response")
	}
	var response Response
	seen := false
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return Response{}, fmt.Errorf("target view choice: invalid JSON response")
		}
		name, ok := nameToken.(string)
		if !ok || name != "default_view_ref" {
			return Response{}, fmt.Errorf("target view choice: response contains an unknown field")
		}
		if seen {
			return Response{}, fmt.Errorf("target view choice: response contains duplicate default_view_ref")
		}
		seen = true
		if err := decoder.Decode(&response.DefaultViewRef); err != nil {
			return Response{}, fmt.Errorf("target view choice: default_view_ref must be a string")
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !seen || response.DefaultViewRef == "" {
		return Response{}, fmt.Errorf("target view choice: response must contain exactly default_view_ref")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Response{}, fmt.Errorf("target view choice: response contains trailing data")
	}
	return response, nil
}
