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

// ResponseDecodeError identifies malformed provider JSON. A syntactically
// valid response that violates the closed Atlas Study contract is a response
// validation failure instead, even when no direction can be accepted.
type ResponseDecodeError struct{ Err error }

func (err *ResponseDecodeError) Error() string {
	if err == nil || err.Err == nil {
		return "atlas study response: decode"
	}
	return "atlas study response: decode: " + err.Err.Error()
}

func (err *ResponseDecodeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
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
	WhatItIs              providerStatement `json:"what_it_is"`
	Problem               providerStatement `json:"problem"`
	MainInput             providerStatement `json:"main_input"`
	CentralResponsibility providerStatement `json:"central_responsibility"`
	ObservableResult      providerStatement `json:"observable_result"`
	DomainTerms           []json.RawMessage `json:"domain_terms,omitempty"`
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
		return ResultRecord{}, Diagnostics{}, &ResponseDecodeError{Err: err}
	}
	if !envelope.RepositoryType.Valid() {
		return ResultRecord{}, Diagnostics{}, fmt.Errorf("atlas study response: invalid repository type")
	}
	brief, diagnostics, err := product.resolveBrief(envelope.Brief)
	if err != nil {
		return ResultRecord{}, Diagnostics{}, err
	}
	directions, directionDiagnostics := product.resolveDirections(envelope.Directions)
	diagnostics.DirectionsReceived = directionDiagnostics.DirectionsReceived
	diagnostics.DirectionsAccepted = directionDiagnostics.DirectionsAccepted
	diagnostics.DirectionsRejected = directionDiagnostics.DirectionsRejected
	diagnostics.Issues = directionDiagnostics.Issues
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

func (product Product) resolveBrief(provider providerBrief) (Brief, Diagnostics, error) {
	diagnostics := Diagnostics{DomainTermsReceived: len(provider.DomainTerms)}
	resolve := func(field string, statement providerStatement) (SupportedStatement, error) {
		refs, err := product.resolveSupportRefs(field+".support_refs", statement.SupportRefs)
		if err != nil {
			return SupportedStatement{}, err
		}
		if len(refs) == 0 {
			return SupportedStatement{}, fmt.Errorf("atlas study response: %s requires support", field)
		}
		if err := product.validateModelTextWithTargetLocators(
			statement.Text, 1024, true, true, product.supportReadingTargets(refs),
		); err != nil {
			return SupportedStatement{}, fmt.Errorf("atlas study response: %s: %w", field, err)
		}
		return SupportedStatement{Text: statement.Text, SupportRefs: refs}, nil
	}
	what, err := resolve("brief.what_it_is", provider.WhatItIs)
	if err != nil {
		return Brief{}, Diagnostics{}, err
	}
	problem, err := resolve("brief.problem", provider.Problem)
	if err != nil {
		return Brief{}, Diagnostics{}, err
	}
	input, err := resolve("brief.main_input", provider.MainInput)
	if err != nil {
		return Brief{}, Diagnostics{}, err
	}
	central, err := resolve("brief.central_responsibility", provider.CentralResponsibility)
	if err != nil {
		return Brief{}, Diagnostics{}, err
	}
	result, err := resolve("brief.observable_result", provider.ObservableResult)
	if err != nil {
		return Brief{}, Diagnostics{}, err
	}
	limit := len(provider.DomainTerms)
	if limit > MaxDomainTerms {
		limit = MaxDomainTerms
	}
	terms := make([]DomainTerm, 0, limit)
	for index := 0; index < limit; index++ {
		term, code := product.resolveDomainTerm(index, provider.DomainTerms[index])
		if code != "" {
			diagnostics.addDomainTermIssue(index, code)
			continue
		}
		terms = append(terms, term)
	}
	for index := MaxDomainTerms; index < len(provider.DomainTerms); index++ {
		diagnostics.addDomainTermIssue(index, DomainTermIssueUnrequestedOutput)
	}
	diagnostics.DomainTermsAccepted = len(terms)
	diagnostics.DomainTermsRejected = diagnostics.DomainTermsReceived - diagnostics.DomainTermsAccepted
	return Brief{
		WhatItIs: what, Problem: problem, MainInput: input,
		CentralResponsibility: central, ObservableResult: result, DomainTerms: terms,
	}, diagnostics, nil
}

func (product Product) resolveDomainTerm(index int, raw json.RawMessage) (DomainTerm, DomainTermIssueCode) {
	var provider providerDomainTerm
	if err := decodeStrict(raw, &provider); err != nil {
		return DomainTerm{}, DomainTermIssueDecodeCandidate
	}
	refs, err := product.resolveSupportRefs(
		fmt.Sprintf("brief.domain_terms[%d].support_refs", index), provider.SupportRefs,
	)
	if err != nil || len(refs) == 0 {
		return DomainTerm{}, DomainTermIssueInvalidSupport
	}
	if err := product.validateModelText(provider.Term, 128, true, false); err != nil {
		return DomainTerm{}, DomainTermIssueInvalidTerm
	}
	if err := product.validateModelTextWithTargetLocators(
		provider.Meaning, 512, true, true, product.supportReadingTargets(refs),
	); err != nil {
		return DomainTerm{}, DomainTermIssueInvalidMeaning
	}
	return DomainTerm{Term: provider.Term, Meaning: provider.Meaning, SupportRefs: refs}, ""
}

func (product Product) supportReadingTargets(refs []CanonicalRef) []CatalogObject {
	targets := make([]CatalogObject, 0, len(refs))
	for _, ref := range refs {
		object, ok := product.byCanonical[ref]
		if ok && object.Kind == RefReadingTarget {
			targets = append(targets, object)
		}
	}
	return targets
}

func (product Product) resolveSupportRefs(field string, refs []string) ([]CanonicalRef, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("atlas study response: %s requires support", field)
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
		if !briefSupportKind(object.Kind) {
			return nil, &ReferenceError{Field: field, Position: position, Code: "wrong_kind_ref"}
		}
		result = append(result, CanonicalRef{Kind: object.Kind, ID: object.CanonicalID})
	}
	sort.Slice(result, func(i, j int) bool { return canonicalRefLess(result[i], result[j]) })
	return result, nil
}

func briefSupportKind(kind RefKind) bool {
	switch kind {
	case RefSubsystem, RefComponent, RefSurface, RefReadingTarget, RefEvidence, RefDocument:
		return true
	default:
		return false
	}
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
	if !naturalQuestion(provider.Question) {
		return Direction{}, "invalid_question"
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
		case RefComponent, RefSurface:
		default:
			return Direction{}, "wrong_kind_principal_ref"
		}
		canonical := CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
		if !product.advertisesPrincipal(canonical) {
			return Direction{}, "principal_not_advertised"
		}
		principals = append(principals, canonical)
		principalSet[canonical] = struct{}{}
		hasComponent = hasComponent || object.Kind == RefComponent
	}
	if !hasComponent {
		return Direction{}, "component_principal_missing"
	}
	sort.Slice(principals, func(i, j int) bool { return canonicalRefLess(principals[i], principals[j]) })
	if len(provider.Reading) < MinDirectionReadingCount ||
		len(provider.Reading) > MaxDirectionReadingCount {
		return Direction{}, "invalid_reading_count"
	}
	reading := make([]ResolvedReading, 0, len(provider.Reading))
	readingObjects := make([]CatalogObject, 0, len(provider.Reading))
	seenTargets := make(map[string]struct{}, len(provider.Reading))
	coveredPrincipals := make(map[CanonicalRef]struct{}, len(principalSet))
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
		if object.Kind != RefReadingTarget || len(object.PrincipalRefs) == 0 {
			return Direction{}, "wrong_kind_reading_ref"
		}
		if !intersectsPrincipalSet(object.PrincipalRefs, principalSet) {
			return Direction{}, "reading_principal_not_selected"
		}
		for _, principal := range object.PrincipalRefs {
			if _, selected := principalSet[principal]; selected {
				coveredPrincipals[principal] = struct{}{}
			}
		}
		if !item.Label.Valid() {
			return Direction{}, "invalid_reading_label"
		}
		reading = append(reading, ResolvedReading{
			Target: CanonicalRef{Kind: object.Kind, ID: object.CanonicalID},
			Label:  item.Label, WhatToLookFor: item.WhatToLookFor,
		})
		readingObjects = append(readingObjects, object)
	}
	if len(coveredPrincipals) != len(principalSet) {
		return Direction{}, "principal_not_advertised"
	}
	if err := product.validateModelTextWithTargetLocators(
		provider.Question, 512, true, false, readingObjects,
	); err != nil {
		return Direction{}, "invalid_question"
	}
	if err := product.validateModelTextWithTargetLocators(
		provider.WhyItMatters, 1024, true, true, readingObjects,
	); err != nil {
		return Direction{}, "invalid_why"
	}
	if err := product.validateModelTextWithTargetLocators(
		provider.LearningOutcome, 1024, true, true, readingObjects,
	); err != nil {
		return Direction{}, "invalid_outcome"
	}
	for index, item := range provider.Reading {
		if err := product.validateModelTextWithTargetLocators(
			item.WhatToLookFor, 768, true, true, readingObjects[index:index+1],
		); err != nil {
			return Direction{}, "invalid_reading_copy"
		}
	}
	direction := Direction{
		Question: provider.Question, WhyItMatters: provider.WhyItMatters,
		LearningOutcome: provider.LearningOutcome, TargetJob: provider.TargetJob,
		LearningStage: provider.LearningStage, PrincipalRefs: principals, Reading: reading,
	}
	direction.ID = stableDirectionID(direction)
	return direction, ""
}

func (product Product) advertisesPrincipal(principal CanonicalRef) bool {
	for _, object := range product.catalog {
		if object.Kind != RefReadingTarget {
			continue
		}
		for _, advertised := range object.PrincipalRefs {
			if advertised == principal {
				return true
			}
		}
	}
	return false
}

func intersectsPrincipalSet(
	values []CanonicalRef,
	selected map[CanonicalRef]struct{},
) bool {
	for _, value := range values {
		if _, ok := selected[value]; ok {
			return true
		}
	}
	return false
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
	return product.validateModelTextWithTargetLocators(value, limit, required, rejectOrder, nil)
}

func (product Product) validateModelTextWithTargetLocators(
	value string,
	limit int,
	required bool,
	rejectOrder bool,
	targets []CatalogObject,
) error {
	if limit > product.input.Limits.MaxTextBytes {
		limit = product.input.Limits.MaxTextBytes
	}
	identities := product.privateIdentities
	if len(targets) != 0 {
		identities = make(map[string]struct{}, len(product.privateIdentities))
		for identity := range product.privateIdentities {
			identities[identity] = struct{}{}
		}
		for _, target := range targets {
			if target.Kind != RefReadingTarget {
				continue
			}
			if target.Location != nil {
				if _, private := product.alwaysPrivate[target.Location.Path]; !private {
					delete(identities, target.Location.Path)
				}
			}
			if symbol := modelVisibleTargetSymbol(target.Symbol); symbol != "" {
				if _, private := product.alwaysPrivate[symbol]; !private {
					delete(identities, symbol)
				}
			}
		}
	}
	if err := validateVisibleText(value, limit, required, identities); err != nil {
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

func (diagnostics *Diagnostics) addDomainTermIssue(position int, code DomainTermIssueCode) {
	if len(diagnostics.DomainTermIssues) < MaxDomainTermDiagnostics {
		diagnostics.DomainTermIssues = append(
			diagnostics.DomainTermIssues,
			DomainTermIssue{Position: position, Code: code},
		)
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
	return strings.HasSuffix(value, "?")
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
