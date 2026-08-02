package atlasstudy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type ReferenceError struct {
	Field    string
	Position int
	Code     string
}

func (err *ReferenceError) Error() string {
	if err == nil {
		return "atlas study response: invalid reference"
	}
	return fmt.Sprintf("atlas study response: %s[%d]: %s", err.Field, err.Position, err.Code)
}

type responseEnvelope struct {
	RepositoryType RepositoryType    `json:"repository_type"`
	Brief          providerBrief     `json:"brief"`
	Directions     []json.RawMessage `json:"directions"`
}

type providerBrief struct {
	WhatItIs              providerStatement    `json:"what_it_is"`
	Problem               providerStatement    `json:"problem"`
	MainInput             providerStatement    `json:"main_input"`
	CentralResponsibility providerStatement    `json:"central_responsibility"`
	ObservableResult      providerStatement    `json:"observable_result"`
	DomainTerms           []providerDomainTerm `json:"domain_terms,omitempty"`
}

type providerStatement struct {
	Text        string   `json:"text"`
	SupportRefs []string `json:"support_refs"`
}

type providerDomainTerm struct {
	Term        string   `json:"term"`
	Meaning     string   `json:"meaning"`
	SupportRefs []string `json:"support_refs"`
}

type providerDirection struct {
	Question        string            `json:"question"`
	WhyItMatters    string            `json:"why_it_matters"`
	LearningOutcome string            `json:"learning_outcome"`
	TargetJob       TargetJob         `json:"target_job"`
	LearningStage   LearningStage     `json:"learning_stage"`
	PrincipalRefs   []string          `json:"principal_refs"`
	Reading         []providerReading `json:"reading"`
}

type providerReading struct {
	TargetRef     string       `json:"target_ref"`
	Label         ReadingLabel `json:"label"`
	WhatToLookFor string       `json:"what_to_look_for"`
}

func (product Product) ResolveResponseJSON(data []byte) (ResultRecord, Diagnostics, error) {
	if product.catalogRef == "" || len(product.byRef) == 0 {
		return ResultRecord{}, Diagnostics{}, fmt.Errorf("atlas study response: compiled catalog is unavailable")
	}
	if len(data) == 0 {
		return ResultRecord{}, Diagnostics{}, fmt.Errorf("atlas study response: empty payload")
	}
	if len(data) > product.input.Limits.MaxResponseBytes {
		return ResultRecord{}, Diagnostics{}, &ResourceLimitError{
			Section: "response_bytes", Limit: product.input.Limits.MaxResponseBytes, Actual: len(data),
		}
	}
	var envelope responseEnvelope
	if err := decodeStrict(data, &envelope); err != nil {
		return ResultRecord{}, Diagnostics{}, fmt.Errorf("atlas study response: decode: %w", err)
	}
	if !envelope.RepositoryType.Valid() {
		return ResultRecord{}, Diagnostics{}, fmt.Errorf("atlas study response: invalid repository type")
	}
	brief, err := product.resolveBrief(envelope.Brief)
	if err != nil {
		return ResultRecord{}, Diagnostics{}, err
	}
	directions, diagnostics := product.resolveDirections(envelope.Directions)
	if len(directions) == 0 {
		return ResultRecord{}, diagnostics, fmt.Errorf("atlas study response: no valid Study directions")
	}
	shape := shapeFromDirections(directions)
	result := product.result(envelope.RepositoryType, brief, directions, shape, diagnostics)
	if err := product.ValidateResultRecord(result); err != nil {
		return ResultRecord{}, diagnostics, err
	}
	return result, diagnostics, nil
}

func (product Product) resolveBrief(provider providerBrief) (Brief, error) {
	resolve := func(field string, statement providerStatement) (SupportedStatement, error) {
		if err := product.validateModelText(statement.Text, 1024, true, true); err != nil {
			return SupportedStatement{}, fmt.Errorf("atlas study response: %s: %w", field, err)
		}
		refs, err := product.resolveSupportRefs(field+".support_refs", statement.SupportRefs)
		if err != nil {
			return SupportedStatement{}, err
		}
		if len(refs) == 0 {
			return SupportedStatement{}, fmt.Errorf("atlas study response: %s requires support", field)
		}
		return SupportedStatement{Text: statement.Text, SupportRefs: refs}, nil
	}
	what, err := resolve("brief.what_it_is", provider.WhatItIs)
	if err != nil {
		return Brief{}, err
	}
	problem, err := resolve("brief.problem", provider.Problem)
	if err != nil {
		return Brief{}, err
	}
	input, err := resolve("brief.main_input", provider.MainInput)
	if err != nil {
		return Brief{}, err
	}
	central, err := resolve("brief.central_responsibility", provider.CentralResponsibility)
	if err != nil {
		return Brief{}, err
	}
	result, err := resolve("brief.observable_result", provider.ObservableResult)
	if err != nil {
		return Brief{}, err
	}
	if len(provider.DomainTerms) > 8 {
		return Brief{}, fmt.Errorf("atlas study response: too many domain terms")
	}
	terms := make([]DomainTerm, 0, len(provider.DomainTerms))
	for index, term := range provider.DomainTerms {
		if err := product.validateModelText(term.Term, 128, true, false); err != nil {
			return Brief{}, fmt.Errorf("atlas study response: domain_terms[%d].term: %w", index, err)
		}
		if err := product.validateModelText(term.Meaning, 512, true, true); err != nil {
			return Brief{}, fmt.Errorf("atlas study response: domain_terms[%d].meaning: %w", index, err)
		}
		refs, err := product.resolveSupportRefs(
			fmt.Sprintf("brief.domain_terms[%d].support_refs", index), term.SupportRefs,
		)
		if err != nil {
			return Brief{}, err
		}
		if len(refs) == 0 {
			return Brief{}, fmt.Errorf("atlas study response: domain term requires support")
		}
		terms = append(terms, DomainTerm{Term: term.Term, Meaning: term.Meaning, SupportRefs: refs})
	}
	return Brief{
		WhatItIs: what, Problem: problem, MainInput: input,
		CentralResponsibility: central, ObservableResult: result, DomainTerms: terms,
	}, nil
}

func (product Product) resolveSupportRefs(field string, refs []string) ([]CanonicalRef, error) {
	if len(refs) == 0 || len(refs) > 8 {
		return nil, fmt.Errorf("atlas study response: %s count is outside 1..8", field)
	}
	result := make([]CanonicalRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for position, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, &ReferenceError{Field: field, Position: position, Code: "duplicate_ref"}
		}
		seen[ref] = struct{}{}
		object, err := product.resolveRef(field, position, ref)
		if err != nil {
			return nil, err
		}
		switch object.Kind {
		case RefSubsystem, RefComponent, RefSurface, RefReadingTarget, RefEvidence, RefDocument:
		default:
			return nil, &ReferenceError{Field: field, Position: position, Code: "wrong_kind_ref"}
		}
		result = append(result, CanonicalRef{Kind: object.Kind, ID: object.CanonicalID})
	}
	sort.Slice(result, func(i, j int) bool { return canonicalRefLess(result[i], result[j]) })
	return result, nil
}

func (product Product) resolveDirections(items []json.RawMessage) ([]Direction, Diagnostics) {
	diagnostics := Diagnostics{DirectionsReceived: len(items)}
	limit := len(items)
	if limit > MaxDirections {
		limit = MaxDirections
	}
	result := make([]Direction, 0, limit)
	seenIDs := make(map[string]struct{}, limit)
	for position := 0; position < limit; position++ {
		var provider providerDirection
		if err := decodeStrict(items[position], &provider); err != nil {
			diagnostics.addIssue(position, "decode_candidate")
			continue
		}
		direction, code := product.resolveDirection(position, provider)
		if code != "" {
			diagnostics.addIssue(position, code)
			continue
		}
		if _, duplicate := seenIDs[direction.ID]; duplicate {
			diagnostics.addIssue(position, "duplicate_direction")
			continue
		}
		seenIDs[direction.ID] = struct{}{}
		result = append(result, direction)
	}
	for position := MaxDirections; position < len(items) && len(diagnostics.Issues) < MaxDirectionDiagnostics; position++ {
		diagnostics.Issues = append(diagnostics.Issues, DirectionIssue{
			Position: position, Code: "unrequested_output",
		})
	}
	diagnostics.DirectionsAccepted = len(result)
	diagnostics.DirectionsRejected = diagnostics.DirectionsReceived - diagnostics.DirectionsAccepted
	return result, diagnostics
}

func (product Product) resolveDirection(position int, provider providerDirection) (Direction, DirectionIssueCode) {
	if err := product.validateModelText(provider.Question, 512, true, false); err != nil ||
		!naturalQuestion(provider.Question) {
		return Direction{}, "invalid_question"
	}
	if err := product.validateModelText(provider.WhyItMatters, 1024, true, true); err != nil {
		return Direction{}, "invalid_why"
	}
	if err := product.validateModelText(provider.LearningOutcome, 1024, true, true); err != nil {
		return Direction{}, "invalid_outcome"
	}
	if !provider.TargetJob.Valid() {
		return Direction{}, "invalid_target_job"
	}
	if !provider.LearningStage.Valid() {
		return Direction{}, "invalid_learning_stage"
	}
	if len(provider.PrincipalRefs) == 0 || len(provider.PrincipalRefs) > 5 {
		return Direction{}, "invalid_principal_count"
	}
	principals := make([]CanonicalRef, 0, len(provider.PrincipalRefs))
	principalSet := make(map[CanonicalRef]struct{}, len(provider.PrincipalRefs))
	seenRefs := make(map[string]struct{}, len(provider.PrincipalRefs))
	hasComponent := false
	for index, ref := range provider.PrincipalRefs {
		if _, duplicate := seenRefs[ref]; duplicate {
			return Direction{}, "duplicate_principal_ref"
		}
		seenRefs[ref] = struct{}{}
		object, err := product.resolveRef(
			fmt.Sprintf("directions[%d].principal_refs", position), index, ref,
		)
		if err != nil {
			return Direction{}, referenceCode(err)
		}
		switch object.Kind {
		case RefUnit, RefSubsystem, RefComponent, RefSurface:
		default:
			return Direction{}, "wrong_kind_principal_ref"
		}
		canonical := CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
		principals = append(principals, canonical)
		principalSet[canonical] = struct{}{}
		hasComponent = hasComponent || object.Kind == RefComponent
	}
	if !hasComponent {
		return Direction{}, "component_principal_missing"
	}
	sort.Slice(principals, func(i, j int) bool { return canonicalRefLess(principals[i], principals[j]) })
	if len(provider.Reading) < 3 || len(provider.Reading) > 5 {
		return Direction{}, "invalid_reading_count"
	}
	reading := make([]ResolvedReading, 0, len(provider.Reading))
	seenTargets := make(map[string]struct{}, len(provider.Reading))
	for index, item := range provider.Reading {
		if _, duplicate := seenTargets[item.TargetRef]; duplicate {
			return Direction{}, "duplicate_reading_target"
		}
		seenTargets[item.TargetRef] = struct{}{}
		object, err := product.resolveRef(
			fmt.Sprintf("directions[%d].reading", position), index, item.TargetRef,
		)
		if err != nil {
			return Direction{}, referenceCode(err)
		}
		if object.Kind != RefReadingTarget || object.Owner == nil {
			return Direction{}, "wrong_kind_reading_ref"
		}
		if _, ok := principalSet[*object.Owner]; !ok {
			return Direction{}, "reading_owner_not_principal"
		}
		if !item.Label.Valid() {
			return Direction{}, "invalid_reading_label"
		}
		if err := product.validateModelText(item.WhatToLookFor, 768, true, true); err != nil {
			return Direction{}, "invalid_reading_copy"
		}
		reading = append(reading, ResolvedReading{
			Target: CanonicalRef{Kind: object.Kind, ID: object.CanonicalID},
			Label:  item.Label, WhatToLookFor: item.WhatToLookFor,
		})
	}
	direction := Direction{
		Question: provider.Question, WhyItMatters: provider.WhyItMatters,
		LearningOutcome: provider.LearningOutcome, TargetJob: provider.TargetJob,
		LearningStage: provider.LearningStage, PrincipalRefs: principals, Reading: reading,
	}
	direction.ID = stableDirectionID(direction)
	return direction, ""
}

func (product Product) resolveRef(field string, position int, ref string) (CatalogObject, error) {
	object, ok := product.byRef[ref]
	if ok {
		return object, nil
	}
	for canonical := range product.byCanonical {
		if ref == canonical.ID {
			return CatalogObject{}, &ReferenceError{Field: field, Position: position, Code: "raw_canonical_ref"}
		}
	}
	return CatalogObject{}, &ReferenceError{Field: field, Position: position, Code: "unknown_ref"}
}

func (product Product) validateModelText(value string, limit int, required, rejectOrder bool) error {
	if limit > product.input.Limits.MaxTextBytes {
		limit = product.input.Limits.MaxTextBytes
	}
	if err := validateVisibleText(value, limit, required, product.privateIdentities); err != nil {
		return err
	}
	if rejectOrder && impliesRuntimeOrder(value) {
		return fmt.Errorf("contains an unsupported runtime-order claim")
	}
	return nil
}

func (diagnostics *Diagnostics) addIssue(position int, code DirectionIssueCode) {
	if len(diagnostics.Issues) < MaxDirectionDiagnostics {
		diagnostics.Issues = append(diagnostics.Issues, DirectionIssue{Position: position, Code: code})
	}
}

func shapeFromDirections(directions []Direction) []CanonicalRef {
	seen := make(map[string]struct{})
	var result []CanonicalRef
	for _, direction := range directions {
		for _, ref := range direction.PrincipalRefs {
			if ref.Kind != RefComponent {
				continue
			}
			if _, duplicate := seen[ref.ID]; duplicate {
				continue
			}
			seen[ref.ID] = struct{}{}
			result = append(result, ref)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func stableDirectionID(direction Direction) string {
	identity := struct {
		Question   string         `json:"question"`
		Principals []CanonicalRef `json:"principals"`
		Targets    []CanonicalRef `json:"targets"`
	}{Question: direction.Question, Principals: append([]CanonicalRef(nil), direction.PrincipalRefs...)}
	for _, reading := range direction.Reading {
		identity.Targets = append(identity.Targets, reading.Target)
	}
	encoded, _ := json.Marshal(identity)
	return "study-" + digest(encoded)[:24]
}

func naturalQuestion(value string) bool {
	return strings.HasSuffix(value, "?") && len(strings.Fields(value)) >= 4
}

func impliesRuntimeOrder(value string) bool {
	lower := strings.ToLower(value)
	for _, phrase := range []string{
		"runtime step", "runtime order", "execution order", "proven sequence",
		"executes after", "executes before", "subsequently executes",
		"затем система", "далее система", "сначала система",
		"затем выполняется", "далее выполняется", "после этого выполняется",
		"шаг выполнения", "порядок выполнения", "порядок исполнения",
		"доказанная последовательность", "выполняется после", "выполняется до",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func referenceCode(err error) DirectionIssueCode {
	var reference *ReferenceError
	if errors.As(err, &reference) {
		code := DirectionIssueCode(reference.Code)
		if code.Valid() {
			return code
		}
	}
	return IssueInvalidRef
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}
