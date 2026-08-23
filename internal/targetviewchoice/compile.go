package targetviewchoice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/secretscan"
)

// Compile validates and canonicalizes the complete ambiguous view set, then
// assigns compact request-local refs. Exactly one view is not ambiguous and
// is intentionally rejected rather than silently bypassed here.
func Compile(views []View, selectedFileHypotheses []string) (Cube, error) {
	canonical, err := canonicalViews(views)
	if err != nil {
		return Cube{}, err
	}
	hypotheses, err := canonicalFileHypotheses(selectedFileHypotheses)
	if err != nil {
		return Cube{}, err
	}
	request := Request{
		SelectedFileHypotheses: append([]string(nil), hypotheses...),
		Views:                  make([]VisibleView, len(canonical)),
	}
	authority := make(map[string]View, len(canonical))
	for index, view := range canonical {
		ref := fmt.Sprintf("v%d", index+1)
		request.Views[index] = visibleView(ref, view)
		authority[ref] = cloneView(view)
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Cube{}, fmt.Errorf("target view choice: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Cube{}, fmt.Errorf(
			"target view choice: complete view request is %d bytes, limit is %d",
			len(wire), MaxRequestBytes,
		)
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return Cube{}, fmt.Errorf("target view choice: provider request contains credential-shaped content")
	}
	state, err := compileState(wire)
	if err != nil {
		return Cube{}, err
	}
	cube := Cube{
		views: canonical, fileHypotheses: hypotheses, request: request, wire: cloneBytes(wire),
		state: cloneBytes(state), authority: authority,
	}
	cube.seal = cubeSeal(state)
	if err := validateCube(cube); err != nil {
		return Cube{}, err
	}
	return cube, nil
}

// State returns an independently owned cache identity for the exact compiled
// choice. Prompt, preparation, response schema, and request bytes are bound.
func (cube Cube) State() ([]byte, error) {
	if err := validateCube(cube); err != nil {
		return nil, err
	}
	return cloneBytes(cube.state), nil
}

// ProviderVisibleJSON returns the exact complete request bytes and no private
// target authority.
func (cube Cube) ProviderVisibleJSON() ([]byte, error) {
	if err := validateCube(cube); err != nil {
		return nil, err
	}
	return cloneBytes(cube.wire), nil
}

func validateCube(cube Cube) error {
	canonical, err := canonicalViews(cube.views)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonical, cube.views) {
		return fmt.Errorf("target view choice: private view authority is not canonical")
	}
	hypotheses, err := canonicalFileHypotheses(cube.fileHypotheses)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(hypotheses, cube.fileHypotheses) {
		return fmt.Errorf("target view choice: private file hypotheses are not canonical")
	}
	wantRequest := Request{
		SelectedFileHypotheses: append([]string(nil), hypotheses...),
		Views:                  make([]VisibleView, len(canonical)),
	}
	wantAuthority := make(map[string]View, len(canonical))
	for index, view := range canonical {
		ref := fmt.Sprintf("v%d", index+1)
		wantRequest.Views[index] = visibleView(ref, view)
		wantAuthority[ref] = cloneView(view)
	}
	if !reflect.DeepEqual(wantRequest, cube.request) || !reflect.DeepEqual(wantAuthority, cube.authority) {
		return fmt.Errorf("target view choice: request-local view authority mismatch")
	}
	wire, err := json.Marshal(wantRequest)
	if err != nil {
		return fmt.Errorf("target view choice: encode request: %w", err)
	}
	if len(wire) > MaxRequestBytes || !reflect.DeepEqual(wire, cube.wire) {
		return fmt.Errorf("target view choice: request wire binding mismatch")
	}
	if _, found := secretscan.Detect(string(wire)); found {
		return fmt.Errorf("target view choice: provider request contains credential-shaped content")
	}
	state, err := compileState(wire)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(state, cube.state) || cube.seal != cubeSeal(state) {
		return fmt.Errorf("target view choice: cube state binding mismatch")
	}
	return nil
}

func canonicalFileHypotheses(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("target view choice: selected file hypotheses are empty")
	}
	result, err := canonicalSummaries("selected file hypothesis", values, MaxFileHypotheses)
	if err != nil {
		return nil, fmt.Errorf("target view choice: %w", err)
	}
	return result, nil
}

func canonicalViews(values []View) ([]View, error) {
	if len(values) < 2 {
		return nil, fmt.Errorf("target view choice: at least two exact views are required")
	}
	if len(values) > MaxViews {
		return nil, fmt.Errorf("target view choice: view count exceeds %d", MaxViews)
	}
	result := make([]View, len(values))
	for index, value := range values {
		canonical, err := canonicalView(value)
		if err != nil {
			return nil, fmt.Errorf("target view choice: view %d: %w", index, err)
		}
		result[index] = canonical
	}
	sort.Slice(result, func(left, right int) bool {
		return viewSortKey(result[left]) < viewSortKey(result[right])
	})
	seenSelectors := make(map[string]struct{}, len(result))
	for index, value := range result {
		selectorKey := value.Language + "\x00" + value.Selector
		if _, duplicate := seenSelectors[selectorKey]; duplicate {
			return nil, fmt.Errorf("target view choice: duplicate language/selector authority")
		}
		seenSelectors[selectorKey] = struct{}{}
		if index > 0 && reflect.DeepEqual(result[index-1], value) {
			return nil, fmt.Errorf("target view choice: duplicate exact view")
		}
	}
	return result, nil
}

func canonicalView(value View) (View, error) {
	for name, label := range map[string]string{
		"language": value.Language, "kind": value.Kind,
		"display_name": value.DisplayName, "selector": value.Selector,
	} {
		if err := validateText(label, MaxLabelBytes); err != nil {
			return View{}, fmt.Errorf("invalid %s", name)
		}
	}
	if err := validateAnchorPath(value.AnchorPath); err != nil {
		return View{}, err
	}
	roots, err := canonicalSummaries("root", value.RootSummaries, MaxRootSummaries)
	if err != nil {
		return View{}, err
	}
	basis, err := canonicalSummaries("basis", value.BasisSummaries, MaxBasisSummaries)
	if err != nil {
		return View{}, err
	}
	if len(basis) == 0 {
		return View{}, fmt.Errorf("basis summaries are empty")
	}
	return View{
		Language: value.Language, Kind: value.Kind, DisplayName: value.DisplayName,
		Selector: value.Selector, AnchorPath: value.AnchorPath,
		RootSummaries: roots, BasisSummaries: basis,
	}, nil
}

func canonicalSummaries(name string, values []string, limit int) ([]string, error) {
	if len(values) > limit {
		return nil, fmt.Errorf("%s summary count exceeds %d", name, limit)
	}
	result := append([]string(nil), values...)
	for _, value := range result {
		if err := validateText(value, MaxSummaryBytes); err != nil {
			return nil, fmt.Errorf("invalid %s summary", name)
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, fmt.Errorf("duplicate %s summary", name)
		}
	}
	if result == nil {
		return []string{}, nil
	}
	return result, nil
}

func validateText(value string, limit int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || len(value) > limit {
		return fmt.Errorf("invalid bounded text")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("invalid bounded text")
		}
	}
	return nil
}

func validateAnchorPath(value string) error {
	if err := validateText(value, MaxAnchorPathBytes); err != nil {
		return fmt.Errorf("invalid anchor path")
	}
	if strings.Contains(value, `\`) || path.IsAbs(value) || value == "." ||
		path.Clean(value) != value || strings.HasPrefix(value, "../") {
		return fmt.Errorf("anchor path must be a canonical repository-relative file path")
	}
	return nil
}

func viewSortKey(value View) string {
	wire, _ := json.Marshal(value)
	return string(wire)
}

func visibleView(ref string, value View) VisibleView {
	return VisibleView{
		Ref: ref, Language: value.Language, Kind: value.Kind,
		DisplayName: value.DisplayName, Selector: value.Selector,
		AnchorPath:     value.AnchorPath,
		RootSummaries:  append([]string(nil), value.RootSummaries...),
		BasisSummaries: append([]string(nil), value.BasisSummaries...),
	}
}

func compileState(requestWire []byte) ([]byte, error) {
	state, err := json.Marshal(struct {
		Contract              string `json:"contract"`
		PromptVersion         string `json:"prompt_version"`
		PreparationVersion    int    `json:"preparation_version"`
		ResponseSchemaVersion int    `json:"response_schema_version"`
		RequestSHA256         string `json:"request_sha256"`
	}{
		Contract: executionContract, PromptVersion: PromptVersion,
		PreparationVersion:    PreparationVersion,
		ResponseSchemaVersion: ResponseSchemaVersion,
		RequestSHA256:         sha256Hex(requestWire),
	})
	if err != nil {
		return nil, fmt.Errorf("target view choice: encode cube state: %w", err)
	}
	return state, nil
}

func cubeSeal(state []byte) string {
	return sha256Hex(append([]byte("target-view-choice-cube-v1\x00"), state...))
}

func cloneView(value View) View {
	value.RootSummaries = append([]string(nil), value.RootSummaries...)
	value.BasisSummaries = append([]string(nil), value.BasisSummaries...)
	return value
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
