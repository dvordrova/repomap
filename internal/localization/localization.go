// Package localization defines provider-neutral contracts for projecting
// inventoried human-readable presentation prose without changing semantic
// identity.
package localization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CanonicalVersion  = 1
	InputVersion      = 1
	ProjectionVersion = 1

	LocaleEnglish = "en"
	LocaleRussian = "ru"
)

type OwnerKind string

const (
	OwnerRepository      OwnerKind = "repository"
	OwnerSubsystem       OwnerKind = "subsystem"
	OwnerComponent       OwnerKind = "component"
	OwnerGuidedTour      OwnerKind = "guided_tour"
	OwnerGuidedStep      OwnerKind = "guided_step"
	OwnerGuidedGap       OwnerKind = "guided_gap"
	OwnerStudyBrief      OwnerKind = "study_brief"
	OwnerBriefDomainTerm OwnerKind = "brief_domain_term"
	OwnerStudyDirection  OwnerKind = "study_direction"
	OwnerStudyReading    OwnerKind = "study_reading"
	OwnerConceptualArea  OwnerKind = "conceptual_area"
	OwnerMechanism       OwnerKind = "mechanism"
	OwnerMechanismStep   OwnerKind = "mechanism_step"
	OwnerMechanismPhase  OwnerKind = "mechanism_phase"
	// OwnerPresentationText addresses terminal presentation prose by its stable
	// render path rather than by the semantic object that happened to supply
	// it. Report-wide localization uses this owner exclusively.
	OwnerPresentationText OwnerKind = "presentation_text"
)

type FieldName string

const (
	FieldProjectGuess          FieldName = "project_guess"
	FieldDocumentedPurpose     FieldName = "documented_purpose"
	FieldNameText              FieldName = "name"
	FieldTitle                 FieldName = "title"
	FieldSummary               FieldName = "summary"
	FieldDescription           FieldName = "description"
	FieldResponsibility        FieldName = "responsibility"
	FieldExplanation           FieldName = "explanation"
	FieldWhyItMatters          FieldName = "why_it_matters"
	FieldGapExplanation        FieldName = "gap_explanation"
	FieldHeadline              FieldName = "headline"
	FieldArchitecture          FieldName = "architecture"
	FieldOperatingModel        FieldName = "operating_model"
	FieldStudyAdvice           FieldName = "study_advice"
	FieldWhatItIs              FieldName = "what_it_is"
	FieldProblem               FieldName = "problem"
	FieldMainInput             FieldName = "main_input"
	FieldCentralResponsibility FieldName = "central_responsibility"
	FieldObservableResult      FieldName = "observable_result"
	FieldDomainTermMeaning     FieldName = "domain_term_meaning"
	FieldQuestion              FieldName = "question"
	FieldWhy                   FieldName = "why"
	FieldOutcome               FieldName = "outcome"
	FieldWhatToLookFor         FieldName = "what_to_look_for"
	FieldSearchQuery           FieldName = "search_query"
	FieldAnswer                FieldName = "answer"
	// FieldText is the single payload field for a typed presentation address.
	// The address itself identifies the report object and schema field.
	FieldText FieldName = "text"
)

type ProtectedKind string

const (
	ProtectedPath       ProtectedKind = "path"
	ProtectedSymbol     ProtectedKind = "symbol"
	ProtectedPackage    ProtectedKind = "package"
	ProtectedModule     ProtectedKind = "module"
	ProtectedProduct    ProtectedKind = "product"
	ProtectedLibrary    ProtectedKind = "library"
	ProtectedAPI        ProtectedKind = "api"
	ProtectedProtocol   ProtectedKind = "protocol"
	ProtectedIdentifier ProtectedKind = "identifier"
	ProtectedURL        ProtectedKind = "url"
)

var allowedProtectedKinds = map[ProtectedKind]struct{}{
	ProtectedPath:       {},
	ProtectedSymbol:     {},
	ProtectedPackage:    {},
	ProtectedModule:     {},
	ProtectedProduct:    {},
	ProtectedLibrary:    {},
	ProtectedAPI:        {},
	ProtectedProtocol:   {},
	ProtectedIdentifier: {},
	ProtectedURL:        {},
}

var allowedFields = map[OwnerKind]map[FieldName]struct{}{
	OwnerRepository: {
		FieldProjectGuess:      {},
		FieldDocumentedPurpose: {},
	},
	OwnerSubsystem: {
		FieldNameText:     {},
		FieldDescription:  {},
		FieldWhyItMatters: {},
	},
	OwnerComponent: {
		FieldNameText:       {},
		FieldSummary:        {},
		FieldDescription:    {},
		FieldResponsibility: {},
		FieldExplanation:    {},
	},
	OwnerGuidedTour: {
		FieldTitle:          {},
		FieldSummary:        {},
		FieldGapExplanation: {},
	},
	OwnerGuidedStep: {
		FieldTitle:       {},
		FieldExplanation: {},
	},
	OwnerGuidedGap: {
		FieldExplanation: {},
	},
	OwnerStudyBrief: {
		FieldHeadline:              {},
		FieldSummary:               {},
		FieldArchitecture:          {},
		FieldOperatingModel:        {},
		FieldStudyAdvice:           {},
		FieldWhatItIs:              {},
		FieldProblem:               {},
		FieldMainInput:             {},
		FieldCentralResponsibility: {},
		FieldObservableResult:      {},
	},
	OwnerBriefDomainTerm: {
		FieldDomainTermMeaning: {},
	},
	OwnerStudyDirection: {
		FieldQuestion:    {},
		FieldWhy:         {},
		FieldOutcome:     {},
		FieldSearchQuery: {},
	},
	OwnerStudyReading: {
		FieldWhatToLookFor: {},
	},
	OwnerConceptualArea: {
		FieldNameText:       {},
		FieldResponsibility: {},
	},
	OwnerMechanism: {
		FieldTitle:    {},
		FieldQuestion: {},
		FieldAnswer:   {},
	},
	OwnerMechanismStep: {
		FieldTitle:       {},
		FieldExplanation: {},
	},
	OwnerMechanismPhase: {
		FieldTitle:       {},
		FieldExplanation: {},
	},
	OwnerPresentationText: {
		FieldText: {},
	},
}

type FieldSpec struct {
	OwnerKind      OwnerKind
	OwnerID        string
	Name           FieldName
	Text           string
	ProtectedTerms []ProtectedValue
}

type ProtectedValue struct {
	Kind  ProtectedKind
	Value string
}

type ProtectedTerm struct {
	Token string        `json:"token"`
	Kind  ProtectedKind `json:"kind"`
	Value string        `json:"value"`
	Count int           `json:"count"`
}

type CanonicalField struct {
	ID             string          `json:"id"`
	OwnerKind      OwnerKind       `json:"owner_kind"`
	OwnerID        string          `json:"owner_id"`
	Name           FieldName       `json:"field"`
	Text           string          `json:"text"`
	ProtectedTerms []ProtectedTerm `json:"protected_terms,omitempty"`
}

type CanonicalArtifact struct {
	Version int              `json:"version"`
	Locale  string           `json:"locale"`
	SHA256  string           `json:"sha256"`
	Fields  []CanonicalField `json:"fields"`
}

type PlaceholderExpectation struct {
	Token string        `json:"token"`
	Kind  ProtectedKind `json:"kind"`
	Count int           `json:"count"`
}

type InputField struct {
	ID           string                   `json:"id"`
	Text         string                   `json:"text"`
	Placeholders []PlaceholderExpectation `json:"placeholders,omitempty"`
}

type Input struct {
	Version         int          `json:"version"`
	CanonicalSHA256 string       `json:"canonical_sha256"`
	SourceLocale    string       `json:"source_locale"`
	TargetLocale    string       `json:"target_locale"`
	Fields          []InputField `json:"fields"`
}

type Projection struct {
	Version         int               `json:"version"`
	CanonicalSHA256 string            `json:"canonical_sha256"`
	Locale          string            `json:"locale"`
	Translations    map[string]string `json:"translations"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	FieldID string `json:"field_id,omitempty"`
}

type ProjectedField struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Result struct {
	Locale      string           `json:"locale"`
	Fields      []ProjectedField `json:"fields"`
	Diagnostics []Diagnostic     `json:"diagnostics,omitempty"`
	Fallback    bool             `json:"fallback"`
}

type canonicalPayload struct {
	Version int              `json:"version"`
	Locale  string           `json:"locale"`
	Fields  []CanonicalField `json:"fields"`
}

var (
	placeholderPattern      = regexp.MustCompile(`\{\{[a-z][a-z0-9_]*\}\}`)
	validPlaceholderPattern = regexp.MustCompile(`^\{\{term_[0-9]+\}\}$`)
)

func NewCanonical(specs []FieldSpec) (CanonicalArtifact, error) {
	fields := make([]CanonicalField, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		field, err := canonicalField(spec)
		if err != nil {
			return CanonicalArtifact{}, err
		}
		if _, exists := seen[field.ID]; exists {
			return CanonicalArtifact{}, fmt.Errorf("localization: duplicate field id %q", field.ID)
		}
		seen[field.ID] = struct{}{}
		fields = append(fields, field)
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].ID < fields[j].ID })

	artifact := CanonicalArtifact{
		Version: CanonicalVersion,
		Locale:  LocaleEnglish,
		Fields:  fields,
	}
	artifact.SHA256 = canonicalHash(artifact)
	return artifact, nil
}

func BuildInput(canonical CanonicalArtifact, targetLocale string) (Input, error) {
	if err := validateCanonical(canonical); err != nil {
		return Input{}, err
	}
	locale, err := normalizeLocale(targetLocale)
	if err != nil {
		return Input{}, err
	}
	fields := make([]InputField, 0, len(canonical.Fields))
	for _, field := range canonical.Fields {
		text := field.Text
		expectations := make([]PlaceholderExpectation, 0, len(field.ProtectedTerms))
		for _, term := range field.ProtectedTerms {
			text, _ = replaceProtectedValue(text, term.Value, term.Token)
			expectations = append(expectations, PlaceholderExpectation{
				Token: term.Token,
				Kind:  term.Kind,
				Count: term.Count,
			})
		}
		fields = append(fields, InputField{
			ID:           field.ID,
			Text:         text,
			Placeholders: expectations,
		})
	}
	return Input{
		Version:         InputVersion,
		CanonicalSHA256: canonical.SHA256,
		SourceLocale:    LocaleEnglish,
		TargetLocale:    locale,
		Fields:          fields,
	}, nil
}

func IdentityProjection(canonical CanonicalArtifact) (Projection, error) {
	input, err := BuildInput(canonical, LocaleEnglish)
	if err != nil {
		return Projection{}, err
	}
	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		translations[field.ID] = field.Text
	}
	return Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleEnglish,
		Translations:    translations,
	}, nil
}

func Apply(canonical CanonicalArtifact, input Input, projection Projection) (Result, error) {
	if err := validateCanonical(canonical); err != nil {
		return Result{}, err
	}
	if err := validateInput(canonical, input); err != nil {
		return Result{}, err
	}
	result := canonicalResult(canonical)
	if projection.Version != ProjectionVersion {
		result.Diagnostics = []Diagnostic{{Code: "projection_version_mismatch"}}
		return result, nil
	}
	if projection.CanonicalSHA256 != canonical.SHA256 {
		result.Diagnostics = []Diagnostic{{Code: "canonical_hash_mismatch"}}
		return result, nil
	}
	if projection.Locale != input.TargetLocale {
		result.Diagnostics = []Diagnostic{{Code: "projection_locale_mismatch"}}
		return result, nil
	}
	result.Locale = input.TargetLocale

	if len(projection.Translations) > len(input.Fields) {
		result.Diagnostics = []Diagnostic{{Code: "projection_field_count_exceeded"}}
		return result, nil
	}

	canonicalByID := make(map[string]CanonicalField, len(canonical.Fields))
	inputByID := make(map[string]InputField, len(input.Fields))
	maxFieldIDBytes := 0
	for _, field := range canonical.Fields {
		canonicalByID[field.ID] = field
		if len(field.ID) > maxFieldIDBytes {
			maxFieldIDBytes = len(field.ID)
		}
	}
	for _, field := range input.Fields {
		inputByID[field.ID] = field
	}
	translationIDs := make([]string, 0, len(canonical.Fields))
	invalidFieldID := false
	for id := range projection.Translations {
		if len(id) > maxFieldIDBytes {
			invalidFieldID = true
			continue
		}
		translationIDs = append(translationIDs, id)
	}
	if invalidFieldID {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code: "invalid_field_id",
		})
	}
	sort.Strings(translationIDs)
	for _, id := range translationIDs {
		if _, exists := canonicalByID[id]; !exists {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "unknown_field_id",
				FieldID: id,
			})
		}
	}

	for index, field := range canonical.Fields {
		translated, exists := projection.Translations[field.ID]
		if !exists {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "missing_translation",
				FieldID: field.ID,
			})
			continue
		}
		inputField := inputByID[field.ID]
		if len(translated) > translationByteLimit(inputField.Text) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "translation_too_large",
				FieldID: field.ID,
			})
			continue
		}
		if !utf8.ValidString(translated) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "invalid_utf8",
				FieldID: field.ID,
			})
			continue
		}
		if !placeholdersMatch(translated, field.ProtectedTerms) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "placeholder_mismatch",
				FieldID: field.ID,
			})
			continue
		}
		if !targetLanguageQualityOK(inputField.Text, translated, input.TargetLocale) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "target_language_quality_failed",
				FieldID: field.ID,
			})
			continue
		}
		for _, term := range field.ProtectedTerms {
			translated = strings.ReplaceAll(translated, term.Token, term.Value)
		}
		result.Fields[index].Text = translated
	}
	result.Fallback = len(result.Diagnostics) > 0
	return result, nil
}

func MarshalCanonical(canonical CanonicalArtifact) ([]byte, error) {
	if err := validateCanonical(canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func FieldID(kind OwnerKind, ownerID string, name FieldName) (string, error) {
	if _, ok := allowedFields[kind][name]; !ok {
		return "", fmt.Errorf("localization: field %q is not allowed for owner %q", name, kind)
	}
	if ownerID == "" || strings.TrimSpace(ownerID) != ownerID || !utf8.ValidString(ownerID) ||
		strings.ContainsAny(ownerID, "\x00\r\n") {
		return "", fmt.Errorf("localization: invalid stable owner id")
	}
	return string(kind) + ":" + ownerID + "." + string(name), nil
}

func canonicalField(spec FieldSpec) (CanonicalField, error) {
	id, err := FieldID(spec.OwnerKind, spec.OwnerID, spec.Name)
	if err != nil {
		return CanonicalField{}, err
	}
	if spec.Text == "" || !utf8.ValidString(spec.Text) {
		return CanonicalField{}, fmt.Errorf("localization: field %q has invalid text", id)
	}
	if placeholderPattern.MatchString(spec.Text) {
		return CanonicalField{}, fmt.Errorf("localization: field %q contains a reserved placeholder", id)
	}
	terms, err := protectedTerms(spec.Text, spec.ProtectedTerms)
	if err != nil {
		return CanonicalField{}, fmt.Errorf("localization: field %q: %w", id, err)
	}
	return CanonicalField{
		ID:             id,
		OwnerKind:      spec.OwnerKind,
		OwnerID:        spec.OwnerID,
		Name:           spec.Name,
		Text:           spec.Text,
		ProtectedTerms: terms,
	}, nil
}

func protectedTerms(text string, values []ProtectedValue) ([]ProtectedTerm, error) {
	unique := make(map[string]struct{}, len(values))
	ordered := make([]ProtectedValue, 0, len(values))
	for _, value := range values {
		if _, allowed := allowedProtectedKinds[value.Kind]; !allowed {
			return nil, fmt.Errorf("invalid protected kind")
		}
		if value.Value == "" || !utf8.ValidString(value.Value) {
			return nil, fmt.Errorf("invalid protected term")
		}
		if _, exists := unique[value.Value]; exists {
			return nil, fmt.Errorf("duplicate protected term")
		}
		if !ContainsProtectedValue(text, value.Value) {
			return nil, fmt.Errorf("protected term is absent from canonical text")
		}
		unique[value.Value] = struct{}{}
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i].Value) != len(ordered[j].Value) {
			return len(ordered[i].Value) > len(ordered[j].Value)
		}
		if ordered[i].Value != ordered[j].Value {
			return ordered[i].Value < ordered[j].Value
		}
		return ordered[i].Kind < ordered[j].Kind
	})

	terms := make([]ProtectedTerm, 0, len(ordered))
	protectedText := text
	for _, value := range ordered {
		token := fmt.Sprintf("{{term_%02d}}", len(terms)+1)
		replaced, count := replaceProtectedValue(protectedText, value.Value, token)
		if count == 0 {
			continue
		}
		protectedText = replaced
		terms = append(terms, ProtectedTerm{
			Token: token,
			Kind:  value.Kind,
			Value: value.Value,
			Count: count,
		})
	}
	return terms, nil
}

// ContainsProtectedValue reports whether value occurs as the same opaque
// identity in text. Identifier-shaped edges must not be embedded in a larger
// identifier: an identity such as "main" therefore matches "main()" but not
// the prose word "domain".
func ContainsProtectedValue(text, value string) bool {
	_, count := replaceProtectedValue(text, value, value)
	return count > 0
}

func replaceProtectedValue(text, value, replacement string) (string, int) {
	if value == "" {
		return text, 0
	}
	var result strings.Builder
	scanFrom := 0
	copyFrom := 0
	count := 0
	for scanFrom <= len(text) {
		relative := strings.Index(text[scanFrom:], value)
		if relative < 0 {
			break
		}
		start := scanFrom + relative
		end := start + len(value)
		if !protectedOccurrenceBoundaryOK(text, value, start, end) {
			_, size := utf8.DecodeRuneInString(text[start:])
			scanFrom = start + size
			continue
		}
		if count == 0 {
			result.Grow(len(text))
		}
		result.WriteString(text[copyFrom:start])
		result.WriteString(replacement)
		copyFrom = end
		scanFrom = end
		count++
	}
	if count == 0 {
		return text, 0
	}
	result.WriteString(text[copyFrom:])
	return result.String(), count
}

func protectedOccurrenceBoundaryOK(text, value string, start, end int) bool {
	first, _ := utf8.DecodeRuneInString(value)
	if isProtectedIdentifierRune(first) && start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if isProtectedIdentifierRune(previous) {
			return false
		}
	}
	last, _ := utf8.DecodeLastRuneInString(value)
	if isProtectedIdentifierRune(last) && end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if isProtectedIdentifierRune(next) {
			return false
		}
	}
	return true
}

func isProtectedIdentifierRune(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func placeholdersMatch(text string, terms []ProtectedTerm) bool {
	expected := make(map[string]int, len(terms))
	for _, term := range terms {
		expected[term.Token] = term.Count
	}
	actual := make(map[string]int, len(terms))
	for _, token := range placeholderPattern.FindAllString(text, -1) {
		actual[token]++
	}
	if len(actual) != len(expected) {
		return false
	}
	for token, count := range expected {
		if actual[token] != count {
			return false
		}
	}
	return true
}

// targetLanguageQualityOK rejects a Russian projection when English prose
// remains outside opaque placeholders. Explicitly typed technical names are
// protected before this check, so any Latin run here is unprotected model
// prose rather than an allowed identifier. Fields that
// contain no English prose after placeholder removal remain valid only while
// the projection does not add new Latin prose around their opaque values.
func targetLanguageQualityOK(source, translated, targetLocale string) bool {
	if targetLocale != LocaleRussian {
		return true
	}
	source = placeholderPattern.ReplaceAllString(source, "")
	translated = placeholderPattern.ReplaceAllString(translated, "")
	if !containsASCIILetter(source) {
		return !containsASCIILetter(translated)
	}
	hasCyrillic := false
	for _, value := range translated {
		if isASCIILetter(value) {
			return false
		}
		if unicode.In(value, unicode.Cyrillic) && unicode.IsLetter(value) {
			hasCyrillic = true
		}
	}
	return hasCyrillic
}

func containsASCIILetter(value string) bool {
	for _, character := range value {
		if isASCIILetter(character) {
			return true
		}
	}
	return false
}

func isASCIILetter(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func canonicalResult(canonical CanonicalArtifact) Result {
	fields := make([]ProjectedField, 0, len(canonical.Fields))
	for _, field := range canonical.Fields {
		fields = append(fields, ProjectedField{ID: field.ID, Text: field.Text})
	}
	return Result{
		Locale:   LocaleEnglish,
		Fields:   fields,
		Fallback: true,
	}
}

func validateCanonical(canonical CanonicalArtifact) error {
	if canonical.Version != CanonicalVersion || canonical.Locale != LocaleEnglish {
		return fmt.Errorf("localization: invalid canonical envelope")
	}
	previous := ""
	for _, field := range canonical.Fields {
		if field.ID <= previous {
			return fmt.Errorf("localization: canonical fields are not uniquely sorted")
		}
		expectedID, err := FieldID(field.OwnerKind, field.OwnerID, field.Name)
		if err != nil || expectedID != field.ID || field.Text == "" || !utf8.ValidString(field.Text) {
			return fmt.Errorf("localization: invalid canonical field %q", field.ID)
		}
		if placeholderPattern.MatchString(field.Text) {
			return fmt.Errorf("localization: reserved placeholder in field %q", field.ID)
		}
		values := make([]ProtectedValue, 0, len(field.ProtectedTerms))
		for _, term := range field.ProtectedTerms {
			if !utf8.ValidString(term.Value) {
				return fmt.Errorf("localization: invalid protected term in field %q", field.ID)
			}
			values = append(values, ProtectedValue{Kind: term.Kind, Value: term.Value})
		}
		expectedTerms, err := protectedTerms(field.Text, values)
		if err != nil || !equalProtectedTerms(expectedTerms, field.ProtectedTerms) {
			return fmt.Errorf("localization: invalid protected term metadata in field %q", field.ID)
		}
		previous = field.ID
	}
	if canonical.SHA256 == "" || canonical.SHA256 != canonicalHash(canonical) {
		return fmt.Errorf("localization: invalid canonical hash")
	}
	return nil
}

func validateInput(canonical CanonicalArtifact, input Input) error {
	if input.Version != InputVersion ||
		input.CanonicalSHA256 != canonical.SHA256 ||
		input.SourceLocale != LocaleEnglish {
		return fmt.Errorf("localization: invalid input envelope")
	}
	locale, err := normalizeLocale(input.TargetLocale)
	if err != nil || locale != input.TargetLocale {
		return fmt.Errorf("localization: invalid input target locale")
	}
	expected, err := BuildInput(canonical, input.TargetLocale)
	if err != nil {
		return err
	}
	if len(input.Fields) != len(expected.Fields) {
		return fmt.Errorf("localization: input field set mismatch")
	}
	for index := range expected.Fields {
		left := input.Fields[index]
		right := expected.Fields[index]
		if left.ID != right.ID || left.Text != right.Text ||
			!equalPlaceholderExpectations(left.Placeholders, right.Placeholders) {
			return fmt.Errorf("localization: input field mismatch")
		}
	}
	return nil
}

func equalProtectedTerms(left, right []ProtectedTerm) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] ||
			!validPlaceholderPattern.MatchString(left[index].Token) {
			return false
		}
	}
	return true
}

func equalPlaceholderExpectations(left, right []PlaceholderExpectation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func translationByteLimit(canonicalText string) int {
	const (
		expansion = 8
		slack     = 4 << 10
	)
	maxInt := int(^uint(0) >> 1)
	if len(canonicalText) > (maxInt-slack)/expansion {
		return maxInt
	}
	return len(canonicalText)*expansion + slack
}

func canonicalHash(canonical CanonicalArtifact) string {
	encoded, err := json.Marshal(canonicalPayload{
		Version: canonical.Version,
		Locale:  canonical.Locale,
		Fields:  canonical.Fields,
	})
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func normalizeLocale(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case LocaleEnglish:
		return LocaleEnglish, nil
	case LocaleRussian:
		return LocaleRussian, nil
	default:
		return "", fmt.Errorf("localization: unsupported locale")
	}
}
