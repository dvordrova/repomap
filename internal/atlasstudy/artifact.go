package atlasstudy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
)

var artifactSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProductState string

const (
	ProductStatePrepared    ProductState = "prepared"
	ProductStateAccepted    ProductState = "accepted"
	ProductStateUnavailable ProductState = "unavailable"
	ProductStateFailed      ProductState = "failed"
)

type UnavailableCode string

const UnavailableOffline UnavailableCode = "offline"

type FailureCode string

const (
	FailureProvider  FailureCode = "provider_failed"
	FailureDecode    FailureCode = "response_decode_failed"
	FailureReference FailureCode = "response_reference_failed"
	FailureResource  FailureCode = "resource_exhausted"
	FailureCanceled  FailureCode = "canceled"
)

type RequestRecord struct {
	Version            int             `json:"version"`
	PromptVersion      string          `json:"prompt_version"`
	AtlasSHA256        string          `json:"atlas_sha256"`
	ArchitectureSHA256 string          `json:"architecture_sha256"`
	WireSHA256         string          `json:"wire_sha256"`
	CatalogSHA256      string          `json:"catalog_sha256"`
	CatalogRef         string          `json:"catalog_ref"`
	Language           Language        `json:"language"`
	Catalog            []CatalogObject `json:"catalog"`
	WireJSON           string          `json:"wire_json"`
}

type ResultRecord struct {
	Version            int             `json:"version"`
	State              ProductState    `json:"state"`
	PromptVersion      string          `json:"prompt_version"`
	AtlasSHA256        string          `json:"atlas_sha256"`
	ArchitectureSHA256 string          `json:"architecture_sha256"`
	WireSHA256         string          `json:"wire_sha256"`
	CatalogSHA256      string          `json:"catalog_sha256"`
	CatalogRef         string          `json:"catalog_ref"`
	Language           Language        `json:"language"`
	Catalog            []CatalogObject `json:"catalog"`
	RepositoryType     RepositoryType  `json:"repository_type"`
	Brief              Brief           `json:"brief"`
	Directions         []Direction     `json:"directions"`
	ShapeComponentRefs []CanonicalRef  `json:"shape_component_refs"`
	Diagnostics        Diagnostics     `json:"diagnostics"`
}

// Status contains no provider prose, source locator, endpoint, model name, or
// raw error. The semantic exchange journal remains the Q/A recorder.
type Status struct {
	Version            int             `json:"version"`
	State              ProductState    `json:"state"`
	PromptVersion      string          `json:"prompt_version"`
	AtlasSHA256        string          `json:"atlas_sha256"`
	ArchitectureSHA256 string          `json:"architecture_sha256"`
	WireSHA256         string          `json:"wire_sha256"`
	CatalogSHA256      string          `json:"catalog_sha256"`
	CatalogRef         string          `json:"catalog_ref"`
	Language           Language        `json:"language"`
	DirectionCount     int             `json:"direction_count"`
	UnavailableCode    UnavailableCode `json:"unavailable_code,omitempty"`
	FailureCode        FailureCode     `json:"failure_code,omitempty"`
}

func (product Product) RequestRecord() (RequestRecord, error) {
	record := RequestRecord{
		Version: Version, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		Catalog: product.Catalog(), WireJSON: string(product.wire),
	}
	return record, product.ValidateRequestRecord(record)
}

func (product Product) result(
	repositoryType RepositoryType,
	brief Brief,
	directions []Direction,
	shape []CanonicalRef,
	diagnostics Diagnostics,
) ResultRecord {
	return ResultRecord{
		Version: Version, State: ProductStateAccepted, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		Catalog: product.Catalog(), RepositoryType: repositoryType,
		Brief: cloneBrief(brief), Directions: cloneDirections(directions),
		ShapeComponentRefs: append([]CanonicalRef(nil), shape...),
		Diagnostics:        cloneDiagnostics(diagnostics),
	}
}

func (product Product) ValidateRequestRecord(record RequestRecord) error {
	if err := validateRequestRecord(record); err != nil {
		return err
	}
	if record.AtlasSHA256 != product.atlasSHA256 ||
		record.ArchitectureSHA256 != product.architectureSHA256 ||
		record.WireSHA256 != product.wireSHA256 ||
		record.CatalogSHA256 != product.catalogSHA256 ||
		record.CatalogRef != product.catalogRef || record.Language != product.input.Language ||
		record.WireJSON != string(product.wire) || !reflect.DeepEqual(record.Catalog, product.catalog) {
		return fmt.Errorf("atlas study request artifact: does not match the exact compiled product")
	}
	return nil
}

func (product Product) ValidateResultRecord(record ResultRecord) error {
	if err := validateResultIdentity(record); err != nil {
		return err
	}
	if record.AtlasSHA256 != product.atlasSHA256 ||
		record.ArchitectureSHA256 != product.architectureSHA256 ||
		record.WireSHA256 != product.wireSHA256 ||
		record.CatalogSHA256 != product.catalogSHA256 ||
		record.CatalogRef != product.catalogRef || record.Language != product.input.Language ||
		!reflect.DeepEqual(record.Catalog, product.catalog) {
		return fmt.Errorf("atlas study result artifact: does not match the exact compiled product")
	}
	if !record.RepositoryType.Valid() {
		return fmt.Errorf("atlas study result artifact: invalid repository type")
	}
	if err := product.validateResolvedBrief(record.Brief); err != nil {
		return err
	}
	if len(record.Directions) == 0 || len(record.Directions) > MaxDirections {
		return fmt.Errorf("atlas study result artifact: invalid direction count")
	}
	seen := make(map[string]struct{}, len(record.Directions))
	for index, direction := range record.Directions {
		if err := product.validateResolvedDirection(direction); err != nil {
			return fmt.Errorf("atlas study result artifact: direction %d: %w", index, err)
		}
		if _, duplicate := seen[direction.ID]; duplicate {
			return fmt.Errorf("atlas study result artifact: duplicate direction")
		}
		seen[direction.ID] = struct{}{}
	}
	wantShape := shapeFromDirections(record.Directions)
	if !slices.Equal(record.ShapeComponentRefs, wantShape) {
		return fmt.Errorf("atlas study result artifact: Shape does not match cited components")
	}
	if err := validateDiagnostics(record.Diagnostics, len(record.Directions)); err != nil {
		return err
	}
	return nil
}

func (product Product) PreparedStatus() Status {
	return product.status(ProductStatePrepared, 0, "", "")
}

func (product Product) AcceptedStatus(record ResultRecord) (Status, error) {
	if err := product.ValidateResultRecord(record); err != nil {
		return Status{}, err
	}
	return product.status(ProductStateAccepted, len(record.Directions), "", ""), nil
}

func (product Product) UnavailableStatus(code UnavailableCode) (Status, error) {
	if code != UnavailableOffline {
		return Status{}, fmt.Errorf("atlas study status: unsupported unavailable code %q", code)
	}
	return product.status(ProductStateUnavailable, 0, code, ""), nil
}

func (product Product) FailureStatus(code FailureCode) (Status, error) {
	if !code.Valid() {
		return Status{}, fmt.Errorf("atlas study status: unsupported failure code %q", code)
	}
	return product.status(ProductStateFailed, 0, "", code), nil
}

func (code FailureCode) Valid() bool {
	switch code {
	case FailureProvider, FailureDecode, FailureReference, FailureResource, FailureCanceled:
		return true
	default:
		return false
	}
}

func (product Product) status(
	state ProductState,
	directionCount int,
	unavailable UnavailableCode,
	failure FailureCode,
) Status {
	return Status{
		Version: Version, State: state, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		DirectionCount: directionCount, UnavailableCode: unavailable, FailureCode: failure,
	}
}

func (product Product) ValidateStatus(status Status) error {
	if err := validateStatus(status); err != nil {
		return err
	}
	if status.AtlasSHA256 != product.atlasSHA256 ||
		status.ArchitectureSHA256 != product.architectureSHA256 ||
		status.WireSHA256 != product.wireSHA256 ||
		status.CatalogSHA256 != product.catalogSHA256 ||
		status.CatalogRef != product.catalogRef || status.Language != product.input.Language {
		return fmt.Errorf("atlas study status artifact: does not match the exact compiled product")
	}
	return nil
}

func ValidateRequestRecordAgainstInput(record RequestRecord, input Input) error {
	product, err := Compile(input)
	if err != nil {
		return err
	}
	return product.ValidateRequestRecord(record)
}

func ValidateResultRecordAgainstInput(record ResultRecord, input Input) error {
	product, err := Compile(input)
	if err != nil {
		return err
	}
	return product.ValidateResultRecord(record)
}

func ValidateStatusAgainstInput(status Status, input Input) error {
	product, err := Compile(input)
	if err != nil {
		return err
	}
	return product.ValidateStatus(status)
}

func EncodeRequestRecord(record RequestRecord) ([]byte, error) {
	if err := validateRequestRecord(record); err != nil {
		return nil, err
	}
	return encodeBoundedArtifact("request", MaxRequestArtifactBytes, record)
}

func DecodeRequestRecord(data []byte) (RequestRecord, error) {
	var record RequestRecord
	if err := decodeArtifact("request", data, MaxRequestArtifactBytes, &record); err != nil {
		return RequestRecord{}, err
	}
	if err := validateRequestRecord(record); err != nil {
		return RequestRecord{}, err
	}
	return record, requireCanonicalArtifact("request", data, record)
}

func EncodeResultRecord(record ResultRecord) ([]byte, error) {
	if err := validateStandaloneResult(record); err != nil {
		return nil, err
	}
	return encodeBoundedArtifact("result", MaxResultArtifactBytes, record)
}

func DecodeResultRecord(data []byte) (ResultRecord, error) {
	var record ResultRecord
	if err := decodeArtifact("result", data, MaxResultArtifactBytes, &record); err != nil {
		return ResultRecord{}, err
	}
	if err := validateStandaloneResult(record); err != nil {
		return ResultRecord{}, err
	}
	return record, requireCanonicalArtifact("result", data, record)
}

func EncodeStatus(status Status) ([]byte, error) {
	if err := validateStatus(status); err != nil {
		return nil, err
	}
	return encodeBoundedArtifact("status", MaxStatusArtifactBytes, status)
}

func DecodeStatus(data []byte) (Status, error) {
	var status Status
	if err := decodeArtifact("status", data, MaxStatusArtifactBytes, &status); err != nil {
		return Status{}, err
	}
	if err := validateStatus(status); err != nil {
		return Status{}, err
	}
	return status, requireCanonicalArtifact("status", data, status)
}

func validateRequestRecord(record RequestRecord) error {
	if record.Version != Version || record.PromptVersion != PromptVersion ||
		!validArtifactSHA(record.AtlasSHA256) || !validArtifactSHA(record.ArchitectureSHA256) ||
		!validArtifactSHA(record.WireSHA256) || !validArtifactSHA(record.CatalogSHA256) ||
		record.CatalogRef != "atlas-study-v2-"+record.CatalogSHA256 || !record.Language.Valid() ||
		len(record.Catalog) == 0 || record.WireJSON == "" || !json.Valid([]byte(record.WireJSON)) ||
		digest([]byte(record.WireJSON)) != record.WireSHA256 {
		return fmt.Errorf("atlas study request artifact: invalid identity or wire")
	}
	return validateCatalog(record.Catalog)
}

func validateResultIdentity(record ResultRecord) error {
	if record.Version != Version || record.State != ProductStateAccepted ||
		record.PromptVersion != PromptVersion || !validArtifactSHA(record.AtlasSHA256) ||
		!validArtifactSHA(record.ArchitectureSHA256) || !validArtifactSHA(record.WireSHA256) ||
		!validArtifactSHA(record.CatalogSHA256) ||
		record.CatalogRef != "atlas-study-v2-"+record.CatalogSHA256 || !record.Language.Valid() ||
		len(record.Catalog) == 0 {
		return fmt.Errorf("atlas study result artifact: invalid identity")
	}
	return validateCatalog(record.Catalog)
}

func validateStatus(status Status) error {
	if status.Version != Version || status.PromptVersion != PromptVersion ||
		!validArtifactSHA(status.AtlasSHA256) || !validArtifactSHA(status.ArchitectureSHA256) ||
		!validArtifactSHA(status.WireSHA256) || !validArtifactSHA(status.CatalogSHA256) ||
		status.CatalogRef != "atlas-study-v2-"+status.CatalogSHA256 || !status.Language.Valid() ||
		status.DirectionCount < 0 || status.DirectionCount > MaxDirections {
		return fmt.Errorf("atlas study status artifact: invalid identity")
	}
	switch status.State {
	case ProductStatePrepared:
		if status.DirectionCount != 0 || status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid prepared status")
		}
	case ProductStateAccepted:
		if status.DirectionCount == 0 || status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid accepted status")
		}
	case ProductStateUnavailable:
		if status.DirectionCount != 0 || status.UnavailableCode != UnavailableOffline || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid unavailable status")
		}
	case ProductStateFailed:
		if status.DirectionCount != 0 || status.UnavailableCode != "" || !status.FailureCode.Valid() {
			return fmt.Errorf("atlas study status artifact: invalid failed status")
		}
	default:
		return fmt.Errorf("atlas study status artifact: unsupported state %q", status.State)
	}
	return nil
}

func validateCatalog(values []CatalogObject) error {
	previousRank := -1
	previousID := ""
	seenRefs := make(map[string]struct{}, len(values))
	seenCanonical := make(map[CanonicalRef]struct{}, len(values))
	counts := make(map[RefKind]int)
	identities := make(map[string]struct{}, 3*len(values))
	for _, object := range values {
		identities[object.CanonicalID] = struct{}{}
		if object.Location != nil {
			identities[object.Location.Path] = struct{}{}
		}
	}
	for _, object := range values {
		if !object.Kind.Valid() || object.CanonicalID == "" || object.Ref == "" ||
			!object.Authority.Valid() {
			return fmt.Errorf("atlas study artifact: invalid private catalog object")
		}
		rank := refKindRank(object.Kind)
		if rank < previousRank || (rank == previousRank && previousID >= object.CanonicalID) {
			return fmt.Errorf("atlas study artifact: private catalog is not canonical")
		}
		if _, duplicate := seenRefs[object.Ref]; duplicate {
			return fmt.Errorf("atlas study artifact: duplicate private ref")
		}
		canonical := CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
		if _, duplicate := seenCanonical[canonical]; duplicate {
			return fmt.Errorf("atlas study artifact: duplicate canonical object")
		}
		seenCanonical[canonical] = struct{}{}
		counts[object.Kind]++
		if object.Ref != refPrefix(object.Kind)+fmt.Sprint(counts[object.Kind]) {
			return fmt.Errorf("atlas study artifact: noncanonical private ref")
		}
		seenRefs[object.Ref] = struct{}{}
		previousRank, previousID = rank, object.CanonicalID
		if object.Kind == RefReadingTarget {
			if object.Location == nil || !repositoryLocation(*object.Location) ||
				len(object.PrincipalRefs) == 0 || !uniqueCanonicalRefs(object.PrincipalRefs) ||
				!uniqueCanonicalRefs(object.RelatedComponentRefs) {
				return fmt.Errorf("atlas study artifact: reading target lacks exact private locator")
			}
		} else if object.Owner != nil || len(object.RelatedComponentRefs) != 0 ||
			len(object.PrincipalRefs) != 0 {
			return fmt.Errorf("atlas study artifact: non-target object has private target associations")
		}
		if object.Location != nil && object.Kind != RefReadingTarget && object.Kind != RefEvidence {
			return fmt.Errorf("atlas study artifact: unexpected private locator")
		}
		if object.Kind == RefEvidence && (object.Location == nil || !repositoryLocation(*object.Location)) {
			return fmt.Errorf("atlas study artifact: evidence lacks exact private locator")
		}
		labelRequired := object.Kind != RefEvidence
		factRequired := object.Kind == RefSurface || object.Kind == RefReadingTarget ||
			object.Kind == RefEvidence || object.Kind == RefDocument
		if err := validateVisibleText(object.Label, DefaultLimits().MaxTextBytes, labelRequired, identities); err != nil {
			return fmt.Errorf("atlas study artifact: invalid private label")
		}
		if err := validateVisibleText(object.Fact, DefaultLimits().MaxTextBytes, factRequired, identities); err != nil {
			return fmt.Errorf("atlas study artifact: invalid private fact")
		}
	}
	for _, object := range values {
		principalSet := make(map[CanonicalRef]struct{}, len(object.PrincipalRefs))
		for _, principal := range object.PrincipalRefs {
			if principal.Kind != RefComponent && principal.Kind != RefSurface {
				return fmt.Errorf("atlas study artifact: target has wrong-kind principal")
			}
			if _, ok := seenCanonical[principal]; !ok {
				return fmt.Errorf("atlas study artifact: target principal is outside private catalog")
			}
			principalSet[principal] = struct{}{}
		}
		for _, related := range object.RelatedComponentRefs {
			if related.Kind != RefComponent {
				return fmt.Errorf("atlas study artifact: target has wrong-kind related component")
			}
			if _, ok := principalSet[related]; !ok {
				return fmt.Errorf("atlas study artifact: related component is not a target principal")
			}
		}
		if object.Owner == nil {
			continue
		}
		if object.Owner.Kind != RefComponent {
			return fmt.Errorf("atlas study artifact: target has wrong-kind owner")
		}
		if _, ok := principalSet[*object.Owner]; !ok {
			return fmt.Errorf("atlas study artifact: target owner is not a principal")
		}
		if !containsCanonicalRef(object.RelatedComponentRefs, *object.Owner) {
			return fmt.Errorf("atlas study artifact: target owner is not a related component")
		}
	}
	return nil
}

func validateStandaloneResult(record ResultRecord) error {
	if err := validateResultIdentity(record); err != nil {
		return err
	}
	product := productFromArtifact(
		record.AtlasSHA256, record.ArchitectureSHA256, record.WireSHA256,
		record.CatalogSHA256, record.CatalogRef, record.Language, record.Catalog,
	)
	return product.ValidateResultRecord(record)
}

func productFromArtifact(
	atlasSHA string,
	architectureSHA string,
	wireSHA string,
	catalogSHA string,
	catalogRef string,
	language Language,
	catalog []CatalogObject,
) Product {
	byRef := make(map[string]CatalogObject, len(catalog))
	byCanonical := make(map[CanonicalRef]CatalogObject, len(catalog))
	identities := make(map[string]struct{})
	for _, object := range catalog {
		byRef[object.Ref] = object
		byCanonical[CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}] = object
		identities[object.CanonicalID] = struct{}{}
		if object.Location != nil {
			identities[object.Location.Path] = struct{}{}
		}
	}
	return Product{
		input:      Input{Language: language, Limits: DefaultLimits()},
		wireSHA256: wireSHA, catalogSHA256: catalogSHA, catalogRef: catalogRef,
		atlasSHA256: atlasSHA, architectureSHA256: architectureSHA,
		catalog: cloneCatalog(catalog), byRef: byRef, byCanonical: byCanonical,
		privateIdentities: identities,
	}
}

func (product Product) validateResolvedBrief(brief Brief) error {
	statements := []struct {
		name  string
		value SupportedStatement
	}{
		{"what_it_is", brief.WhatItIs}, {"problem", brief.Problem},
		{"main_input", brief.MainInput}, {"central_responsibility", brief.CentralResponsibility},
		{"observable_result", brief.ObservableResult},
	}
	for _, item := range statements {
		if err := product.validateModelText(item.value.Text, 1024, true, true); err != nil {
			return fmt.Errorf("atlas study result artifact: brief.%s: %w", item.name, err)
		}
		if err := product.validateResolvedSupport(item.value.SupportRefs); err != nil {
			return err
		}
	}
	if len(brief.DomainTerms) > 8 {
		return fmt.Errorf("atlas study result artifact: too many domain terms")
	}
	for _, term := range brief.DomainTerms {
		if err := product.validateModelText(term.Term, 128, true, false); err != nil {
			return err
		}
		if err := product.validateModelText(term.Meaning, 512, true, true); err != nil {
			return err
		}
		if err := product.validateResolvedSupport(term.SupportRefs); err != nil {
			return err
		}
	}
	return nil
}

func (product Product) validateResolvedSupport(refs []CanonicalRef) error {
	if len(refs) == 0 || len(refs) > 8 || !uniqueCanonicalRefs(refs) {
		return fmt.Errorf("atlas study result artifact: invalid support count")
	}
	seen := make(map[CanonicalRef]struct{}, len(refs))
	for _, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("atlas study result artifact: duplicate support")
		}
		seen[ref] = struct{}{}
		object, ok := product.byCanonical[ref]
		if !ok || object.Kind == RefUnit {
			return fmt.Errorf("atlas study result artifact: unknown or wrong-kind support")
		}
	}
	return nil
}

func (product Product) validateResolvedDirection(direction Direction) error {
	if direction.ID != stableDirectionID(direction) || !naturalQuestion(direction.Question) ||
		product.validateModelText(direction.Question, 512, true, false) != nil ||
		product.validateModelText(direction.WhyItMatters, 1024, true, true) != nil ||
		product.validateModelText(direction.LearningOutcome, 1024, true, true) != nil ||
		!direction.TargetJob.Valid() || !direction.LearningStage.Valid() ||
		len(direction.PrincipalRefs) == 0 || len(direction.PrincipalRefs) > 5 ||
		len(direction.Reading) < 3 || len(direction.Reading) > 5 {
		return fmt.Errorf("invalid canonical direction")
	}
	principalSet := make(map[CanonicalRef]struct{}, len(direction.PrincipalRefs))
	hasComponent := false
	if !uniqueCanonicalRefs(direction.PrincipalRefs) {
		return fmt.Errorf("principals are not canonical")
	}
	for _, ref := range direction.PrincipalRefs {
		if _, duplicate := principalSet[ref]; duplicate {
			return fmt.Errorf("duplicate principal")
		}
		principalSet[ref] = struct{}{}
		object, ok := product.byCanonical[ref]
		if !ok || (object.Kind != RefUnit && object.Kind != RefSubsystem &&
			object.Kind != RefComponent && object.Kind != RefSurface) {
			return fmt.Errorf("unknown or wrong-kind principal")
		}
		hasComponent = hasComponent || object.Kind == RefComponent
	}
	if !hasComponent {
		return fmt.Errorf("component principal missing")
	}
	seenReading := make(map[CanonicalRef]struct{}, len(direction.Reading))
	for _, reading := range direction.Reading {
		if _, duplicate := seenReading[reading.Target]; duplicate {
			return fmt.Errorf("duplicate reading target")
		}
		seenReading[reading.Target] = struct{}{}
		object, ok := product.byCanonical[reading.Target]
		if !ok || object.Kind != RefReadingTarget || len(object.PrincipalRefs) == 0 {
			return fmt.Errorf("unknown or wrong-kind reading target")
		}
		if !intersectsPrincipalSet(object.PrincipalRefs, principalSet) {
			return fmt.Errorf("reading target has no selected principal")
		}
		if !reading.Label.Valid() ||
			product.validateModelText(reading.WhatToLookFor, 768, true, true) != nil {
			return fmt.Errorf("invalid reading guidance")
		}
	}
	return nil
}

func validateDiagnostics(diagnostics Diagnostics, accepted int) error {
	if diagnostics.DirectionsReceived < accepted || diagnostics.DirectionsReceived < 1 ||
		diagnostics.DirectionsAccepted != accepted ||
		diagnostics.DirectionsRejected != diagnostics.DirectionsReceived-accepted ||
		len(diagnostics.Issues) > MaxDirectionDiagnostics {
		return fmt.Errorf("atlas study result artifact: invalid diagnostics")
	}
	previous := -1
	for _, issue := range diagnostics.Issues {
		if issue.Position < 0 || issue.Position >= diagnostics.DirectionsReceived ||
			issue.Position <= previous || !issue.Code.Valid() {
			return fmt.Errorf("atlas study result artifact: invalid diagnostic issue")
		}
		previous = issue.Position
	}
	return nil
}

func encodeArtifact(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("atlas study artifact: encode: %w", err)
	}
	return append(encoded, '\n'), nil
}

func encodeBoundedArtifact(name string, limit int, value any) ([]byte, error) {
	encoded, err := encodeArtifact(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > limit {
		return nil, &ResourceLimitError{
			Section: name + "_artifact_bytes", Limit: limit, Actual: len(encoded),
		}
	}
	return encoded, nil
}

func decodeArtifact(name string, data []byte, limit int, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("atlas study %s artifact: empty", name)
	}
	if len(data) > limit {
		return &ResourceLimitError{Section: name + "_artifact_bytes", Limit: limit, Actual: len(data)}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("atlas study %s artifact: decode: %w", name, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("atlas study %s artifact: trailing data", name)
	}
	return nil
}

func requireCanonicalArtifact(name string, data []byte, value any) error {
	encoded, err := encodeArtifact(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, encoded) {
		return fmt.Errorf("atlas study %s artifact: not canonical", name)
	}
	return nil
}

func validArtifactSHA(value string) bool { return artifactSHA256Pattern.MatchString(value) }

func cloneBrief(value Brief) Brief {
	cloneStatement := func(statement SupportedStatement) SupportedStatement {
		statement.SupportRefs = append([]CanonicalRef(nil), statement.SupportRefs...)
		return statement
	}
	value.WhatItIs = cloneStatement(value.WhatItIs)
	value.Problem = cloneStatement(value.Problem)
	value.MainInput = cloneStatement(value.MainInput)
	value.CentralResponsibility = cloneStatement(value.CentralResponsibility)
	value.ObservableResult = cloneStatement(value.ObservableResult)
	value.DomainTerms = append([]DomainTerm(nil), value.DomainTerms...)
	for index := range value.DomainTerms {
		value.DomainTerms[index].SupportRefs = append([]CanonicalRef(nil), value.DomainTerms[index].SupportRefs...)
	}
	return value
}

func cloneDirections(values []Direction) []Direction {
	result := append([]Direction(nil), values...)
	for index := range result {
		result[index].PrincipalRefs = append([]CanonicalRef(nil), result[index].PrincipalRefs...)
		result[index].Reading = append([]ResolvedReading(nil), result[index].Reading...)
	}
	return result
}

func cloneDiagnostics(value Diagnostics) Diagnostics {
	value.Issues = append([]DirectionIssue(nil), value.Issues...)
	return value
}
