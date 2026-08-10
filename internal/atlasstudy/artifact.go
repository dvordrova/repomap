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
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

var artifactSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProductState string

const (
	ProductStatePrepared        ProductState = "prepared"
	ProductStateAccepted        ProductState = "accepted"
	ProductStateAcceptedPartial ProductState = "accepted_partial"
	ProductStateUnavailable     ProductState = "unavailable"
	ProductStateFailed          ProductState = "failed"
)

type UnavailableCode string

const UnavailableOffline UnavailableCode = "offline"

type FailureCode string

const (
	FailureProvider   FailureCode = "provider_failed"
	FailureDecode     FailureCode = "response_decode_failed"
	FailureReference  FailureCode = "response_reference_failed"
	FailureValidation FailureCode = "response_validation_failed"
	FailureResource   FailureCode = "resource_exhausted"
	FailureCanceled   FailureCode = "canceled"
)

type RequestRecord struct {
	Version            int                      `json:"version"`
	PromptVersion      string                   `json:"prompt_version"`
	AtlasSHA256        string                   `json:"atlas_sha256"`
	ArchitectureSHA256 string                   `json:"architecture_sha256"`
	WireSHA256         string                   `json:"wire_sha256"`
	CatalogSHA256      string                   `json:"catalog_sha256"`
	CatalogRef         string                   `json:"catalog_ref"`
	Language           Language                 `json:"language"`
	CandidateCoverage  CandidateCoverage        `json:"candidate_coverage"`
	AnalysisTargetRoot *AnalysisTargetRootScope `json:"analysis_target_root,omitempty"`
	Catalog            []CatalogObject          `json:"catalog"`
	WireJSON           string                   `json:"wire_json"`
}

type ResultRecord struct {
	Version            int                      `json:"version"`
	State              ProductState             `json:"state"`
	PromptVersion      string                   `json:"prompt_version"`
	AtlasSHA256        string                   `json:"atlas_sha256"`
	ArchitectureSHA256 string                   `json:"architecture_sha256"`
	WireSHA256         string                   `json:"wire_sha256"`
	CatalogSHA256      string                   `json:"catalog_sha256"`
	CatalogRef         string                   `json:"catalog_ref"`
	Language           Language                 `json:"language"`
	CandidateCoverage  CandidateCoverage        `json:"candidate_coverage"`
	AnalysisTargetRoot *AnalysisTargetRootScope `json:"analysis_target_root,omitempty"`
	Catalog            []CatalogObject          `json:"catalog"`
	RepositoryType     RepositoryType           `json:"repository_type"`
	Brief              Brief                    `json:"brief"`
	Directions         []Direction              `json:"directions"`
	ShapeComponentRefs []CanonicalRef           `json:"shape_component_refs"`
	SpanCoverage       SpanCoverage             `json:"span_coverage"`
	// ModelSelectedSpanRefs are the distinct spans referenced by the returned
	// directions, including locally rejected siblings, in canonical order.
	// They are the model-selected stage of the four-stage span pipeline and
	// must be persisted so exact coverage can be re-derived from an artifact.
	ModelSelectedSpanRefs []CanonicalRef `json:"model_selected_span_refs,omitempty"`
	Diagnostics           Diagnostics    `json:"diagnostics"`
}

// Status contains no provider prose, source locator, endpoint, model name, or
// raw error. The semantic exchange journal remains the Q/A recorder. It
// carries the four distinct span stage counts and the four independent
// coverage flags instead of one overloaded coverage_complete.
type Status struct {
	Version            int               `json:"version"`
	State              ProductState      `json:"state"`
	PromptVersion      string            `json:"prompt_version"`
	AtlasSHA256        string            `json:"atlas_sha256"`
	ArchitectureSHA256 string            `json:"architecture_sha256"`
	WireSHA256         string            `json:"wire_sha256"`
	CatalogSHA256      string            `json:"catalog_sha256"`
	CatalogRef         string            `json:"catalog_ref"`
	Language           Language          `json:"language"`
	CandidateCoverage  CandidateCoverage `json:"candidate_coverage"`
	DirectionCount     int               `json:"direction_count"`
	// Four-stage span counts: considered (complete set), advertised (request
	// frontier), model-selected (returned directions, rejected siblings
	// included) and locally accepted (valid directions only).
	ConsideredSpanCount    int `json:"considered_span_count"`
	AdvertisedSpanCount    int `json:"advertised_span_count"`
	ModelSelectedSpanCount int `json:"model_selected_span_count"`
	AcceptedSpanCount      int `json:"accepted_span_count"`
	// Four independent coverage flags.
	FrontierComplete        bool            `json:"frontier_complete"`
	SelectedItemsComplete   bool            `json:"selected_items_complete"`
	SupportCoverageComplete bool            `json:"support_coverage_complete"`
	PortfolioTargetMet      bool            `json:"portfolio_target_met"`
	UnavailableCode         UnavailableCode `json:"unavailable_code,omitempty"`
	FailureCode             FailureCode     `json:"failure_code,omitempty"`
}

func (product Product) RequestRecord() (RequestRecord, error) {
	record := RequestRecord{
		Version: Version, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		CandidateCoverage:  product.Coverage(),
		AnalysisTargetRoot: cloneAnalysisTargetRootScope(product.input.AnalysisTargetRoot),
		Catalog:            product.Catalog(), WireJSON: string(product.wire),
	}
	return record, product.ValidateRequestRecord(record)
}

func (product Product) result(
	repositoryType RepositoryType,
	brief Brief,
	directions []Direction,
	modelSelected []CanonicalRef,
	shape []CanonicalRef,
	spanCoverage SpanCoverage,
	diagnostics Diagnostics,
) ResultRecord {
	// The ordered directions array IS the portfolio. Status is accepted when
	// every returned selected item is locally valid (zero rejected siblings)
	// and at least one direction exists; accepted_partial when at least one
	// returned sibling is locally rejected; failure (zero valid directions) is
	// handled by the caller before this point.
	state := ProductStateAccepted
	if diagnostics.DirectionsRejected > 0 {
		state = ProductStateAcceptedPartial
	}
	return ResultRecord{
		Version: ResultVersion, State: state, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		CandidateCoverage:  product.Coverage(),
		AnalysisTargetRoot: cloneAnalysisTargetRootScope(product.input.AnalysisTargetRoot),
		Catalog:            product.Catalog(), RepositoryType: repositoryType,
		Brief: cloneBrief(brief), Directions: cloneDirections(directions),
		ShapeComponentRefs:    append([]CanonicalRef(nil), shape...),
		SpanCoverage:          spanCoverage,
		ModelSelectedSpanRefs: append([]CanonicalRef(nil), modelSelected...),
		Diagnostics:           cloneDiagnostics(diagnostics),
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
		!reflect.DeepEqual(record.AnalysisTargetRoot, product.input.AnalysisTargetRoot) ||
		!reflect.DeepEqual(record.CandidateCoverage, product.coverage) ||
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
		!reflect.DeepEqual(record.AnalysisTargetRoot, product.input.AnalysisTargetRoot) ||
		!reflect.DeepEqual(record.CandidateCoverage, product.coverage) ||
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
	seenSpans := make(map[CanonicalRef]struct{}, len(record.Directions))
	for index, direction := range record.Directions {
		if err := product.validateResolvedDirection(direction); err != nil {
			return fmt.Errorf("atlas study result artifact: direction %d: %w", index, err)
		}
		if _, duplicate := seen[direction.ID]; duplicate {
			return fmt.Errorf("atlas study result artifact: duplicate direction")
		}
		seen[direction.ID] = struct{}{}
		if _, duplicate := seenSpans[direction.Span]; duplicate {
			return fmt.Errorf("atlas study result artifact: duplicate span")
		}
		seenSpans[direction.Span] = struct{}{}
	}
	wantShape := shapeFromDirections(record.Directions)
	if !slices.Equal(record.ShapeComponentRefs, wantShape) {
		return fmt.Errorf("atlas study result artifact: Shape does not match cited components")
	}
	if err := validateDiagnostics(record.Diagnostics, len(record.Directions), len(record.Brief.DomainTerms)); err != nil {
		return err
	}
	if err := product.validateSpanCoverage(
		record.SpanCoverage, record.Directions, record.ModelSelectedSpanRefs, record.Diagnostics,
	); err != nil {
		return err
	}
	if err := product.validateModelSelectedSpanRefs(record.ModelSelectedSpanRefs, record.Directions); err != nil {
		return err
	}
	wantState := ProductStateAccepted
	if record.Diagnostics.DirectionsRejected > 0 {
		wantState = ProductStateAcceptedPartial
	}
	if record.State != wantState {
		return fmt.Errorf("atlas study result artifact: state does not match exact returned directions")
	}
	return nil
}

// validateModelSelectedSpanRefs checks the persisted model-selected stage: the
// distinct spans referenced by the returned directions (including locally
// rejected siblings), in canonical order, each an advertised route span, and a
// superset of the locally accepted directions' spans.
func (product Product) validateModelSelectedSpanRefs(modelSelected []CanonicalRef, directions []Direction) error {
	if len(modelSelected) == 0 || !uniqueCanonicalRefs(modelSelected) {
		return fmt.Errorf("atlas study result artifact: invalid model-selected span refs")
	}
	seen := make(map[CanonicalRef]struct{}, len(modelSelected))
	for _, ref := range modelSelected {
		if ref.Kind != RefRouteSpan {
			return fmt.Errorf("atlas study result artifact: wrong-kind model-selected ref")
		}
		object, ok := product.byCanonical[ref]
		if !ok || object.Kind != RefRouteSpan {
			return fmt.Errorf("atlas study result artifact: model-selected span is outside the advertised catalog")
		}
		seen[ref] = struct{}{}
	}
	for _, direction := range directions {
		if _, ok := seen[direction.Span]; !ok {
			return fmt.Errorf("atlas study result artifact: accepted direction span is missing from model-selected refs")
		}
	}
	return nil
}

func (product Product) PreparedStatus() Status {
	return product.status(ProductStatePrepared, 0, SpanCoverage{}, "", "")
}

func (product Product) AcceptedStatus(record ResultRecord) (Status, error) {
	if err := product.ValidateResultRecord(record); err != nil {
		return Status{}, err
	}
	return product.status(record.State, len(record.Directions), record.SpanCoverage, "", ""), nil
}

func (product Product) UnavailableStatus(code UnavailableCode) (Status, error) {
	if code != UnavailableOffline {
		return Status{}, fmt.Errorf("atlas study status: unsupported unavailable code %q", code)
	}
	return product.status(ProductStateUnavailable, 0, SpanCoverage{}, code, ""), nil
}

func (product Product) FailureStatus(code FailureCode) (Status, error) {
	if !code.Valid() {
		return Status{}, fmt.Errorf("atlas study status: unsupported failure code %q", code)
	}
	return product.status(ProductStateFailed, 0, SpanCoverage{}, "", code), nil
}

func (code FailureCode) Valid() bool {
	switch code {
	case FailureProvider, FailureDecode, FailureReference, FailureValidation,
		FailureResource, FailureCanceled:
		return true
	default:
		return false
	}
}

func (product Product) status(
	state ProductState,
	directionCount int,
	spanCoverage SpanCoverage,
	unavailable UnavailableCode,
	failure FailureCode,
) Status {
	return Status{
		Version: ResultVersion, State: state, PromptVersion: PromptVersion,
		AtlasSHA256: product.atlasSHA256, ArchitectureSHA256: product.architectureSHA256,
		WireSHA256: product.wireSHA256, CatalogSHA256: product.catalogSHA256,
		CatalogRef: product.catalogRef, Language: product.input.Language,
		CandidateCoverage:       product.Coverage(),
		DirectionCount:          directionCount,
		ConsideredSpanCount:     spanCoverage.ConsideredSpanCount,
		AdvertisedSpanCount:     spanCoverage.AdvertisedSpanCount,
		ModelSelectedSpanCount:  spanCoverage.ModelSelectedSpanCount,
		AcceptedSpanCount:       spanCoverage.AcceptedSpanCount,
		FrontierComplete:        spanCoverage.FrontierComplete,
		SelectedItemsComplete:   spanCoverage.SelectedItemsComplete,
		SupportCoverageComplete: spanCoverage.SupportCoverageComplete,
		PortfolioTargetMet:      spanCoverage.PortfolioTargetMet,
		UnavailableCode:         unavailable,
		FailureCode:             failure,
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
		status.CatalogRef != product.catalogRef || status.Language != product.input.Language ||
		!reflect.DeepEqual(status.CandidateCoverage, product.coverage) {
		return fmt.Errorf("atlas study status artifact: does not match the exact compiled product")
	}
	if status.State == ProductStateAccepted || status.State == ProductStateAcceptedPartial {
		if status.ConsideredSpanCount != product.coverage.SpansConsidered ||
			status.AdvertisedSpanCount != len(product.selectedSpanIDs) {
			return fmt.Errorf("atlas study status artifact: span counts do not match exact compiled product")
		}
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
		record.CatalogRef != fmt.Sprintf("atlas-study-v%d-%s", Version, record.CatalogSHA256) || !record.Language.Valid() ||
		len(record.Catalog) == 0 || record.WireJSON == "" || !json.Valid([]byte(record.WireJSON)) ||
		digest([]byte(record.WireJSON)) != record.WireSHA256 {
		return fmt.Errorf("atlas study request artifact: invalid identity or wire")
	}
	if err := validateCandidateCoverage(record.CandidateCoverage); err != nil {
		return err
	}
	if err := validateCatalog(record.Catalog, record.AnalysisTargetRoot); err != nil {
		return err
	}
	return validateCatalogDigest(
		record.AtlasSHA256, record.ArchitectureSHA256, record.WireSHA256,
		record.CatalogSHA256, record.Language, record.CandidateCoverage,
		record.AnalysisTargetRoot, record.Catalog,
	)
}

func validateResultIdentity(record ResultRecord) error {
	if record.Version != ResultVersion ||
		(record.State != ProductStateAccepted && record.State != ProductStateAcceptedPartial) ||
		record.PromptVersion != PromptVersion || !validArtifactSHA(record.AtlasSHA256) ||
		!validArtifactSHA(record.ArchitectureSHA256) || !validArtifactSHA(record.WireSHA256) ||
		!validArtifactSHA(record.CatalogSHA256) ||
		record.CatalogRef != fmt.Sprintf("atlas-study-v%d-%s", Version, record.CatalogSHA256) || !record.Language.Valid() ||
		len(record.Catalog) == 0 {
		return fmt.Errorf("atlas study result artifact: invalid identity")
	}
	if err := validateCandidateCoverage(record.CandidateCoverage); err != nil {
		return err
	}
	if err := validateCatalog(record.Catalog, record.AnalysisTargetRoot); err != nil {
		return err
	}
	return validateCatalogDigest(
		record.AtlasSHA256, record.ArchitectureSHA256, record.WireSHA256,
		record.CatalogSHA256, record.Language, record.CandidateCoverage,
		record.AnalysisTargetRoot, record.Catalog,
	)
}

func validateStatus(status Status) error {
	if status.Version != ResultVersion || status.PromptVersion != PromptVersion ||
		!validArtifactSHA(status.AtlasSHA256) || !validArtifactSHA(status.ArchitectureSHA256) ||
		!validArtifactSHA(status.WireSHA256) || !validArtifactSHA(status.CatalogSHA256) ||
		status.CatalogRef != fmt.Sprintf("atlas-study-v%d-%s", Version, status.CatalogSHA256) || !status.Language.Valid() ||
		status.DirectionCount < 0 || status.DirectionCount > MaxDirections {
		return fmt.Errorf("atlas study status artifact: invalid identity")
	}
	if err := validateCandidateCoverage(status.CandidateCoverage); err != nil {
		return err
	}
	zeroStageCounts := status.ConsideredSpanCount == 0 && status.AdvertisedSpanCount == 0 &&
		status.ModelSelectedSpanCount == 0 && status.AcceptedSpanCount == 0
	zeroFlags := !status.FrontierComplete && !status.SelectedItemsComplete &&
		!status.SupportCoverageComplete && !status.PortfolioTargetMet
	switch status.State {
	case ProductStatePrepared:
		if status.DirectionCount != 0 || !zeroStageCounts || !zeroFlags ||
			status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid prepared status")
		}
	case ProductStateAccepted, ProductStateAcceptedPartial:
		if status.DirectionCount == 0 || status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid accepted status")
		}
		// Four-stage counts: considered >= advertised >= model-selected, and
		// the locally accepted span count equals the accepted direction count
		// because duplicate-span directions are rejected. Selected-items
		// completeness is exactly the accepted state (no rejected sibling and
		// at least one returned direction); support coverage is recorded true
		// whenever at least one direction is accepted; portfolio target is the
		// desired 6..MaxDirections band, independent of status. Frontier
		// completeness is advertised == considered, and advertised-but-returned
		// never turns accepted into accepted_partial.
		if status.ConsideredSpanCount <= 0 || status.AdvertisedSpanCount <= 0 ||
			status.AdvertisedSpanCount > status.ConsideredSpanCount ||
			status.ModelSelectedSpanCount < status.DirectionCount ||
			status.ModelSelectedSpanCount > status.AdvertisedSpanCount ||
			status.AcceptedSpanCount != status.DirectionCount ||
			status.FrontierComplete != (status.AdvertisedSpanCount == status.ConsideredSpanCount) ||
			status.SelectedItemsComplete != (status.State == ProductStateAccepted) ||
			!status.SupportCoverageComplete ||
			status.PortfolioTargetMet != (status.DirectionCount >= MinPortfolioDirections && status.DirectionCount <= MaxDirections) {
			return fmt.Errorf("atlas study status artifact: invalid accepted span coverage")
		}
	case ProductStateUnavailable:
		if status.DirectionCount != 0 || !zeroStageCounts || !zeroFlags ||
			status.UnavailableCode != UnavailableOffline || status.FailureCode != "" {
			return fmt.Errorf("atlas study status artifact: invalid unavailable status")
		}
	case ProductStateFailed:
		if status.DirectionCount != 0 || !zeroStageCounts || !zeroFlags ||
			status.UnavailableCode != "" || !status.FailureCode.Valid() {
			return fmt.Errorf("atlas study status artifact: invalid failed status")
		}
	default:
		return fmt.Errorf("atlas study status artifact: unsupported state %q", status.State)
	}
	return nil
}

func validateCandidateCoverage(coverage CandidateCoverage) error {
	if !validArtifactSHA(coverage.CandidateSHA256) ||
		coverage.TargetsConsidered <= 0 || coverage.TargetsSelected <= 0 ||
		coverage.TargetsSelected > coverage.TargetsConsidered ||
		coverage.SpansConsidered <= 0 || coverage.SpansSelected <= 0 ||
		coverage.SpansSelected > coverage.SpansConsidered ||
		// Complete is full-selection equality: the advertised frontier equals
		// the complete considered span set (zero omissions) and every
		// considered target is selected.
		coverage.Complete != (coverage.TargetsSelected == coverage.TargetsConsidered &&
			coverage.SpansSelected == coverage.SpansConsidered) {
		return fmt.Errorf("atlas study artifact: invalid candidate coverage")
	}
	validateCounts := func(name string, values []CandidateCoverageCount) error {
		if len(values) == 0 {
			return fmt.Errorf("atlas study artifact: candidate coverage %s is empty", name)
		}
		previous := ""
		for _, value := range values {
			if value.Key == "" || value.Key <= previous || value.Considered <= 0 ||
				value.Selected < 0 || value.Selected > value.Considered {
				return fmt.Errorf("atlas study artifact: invalid candidate coverage %s", name)
			}
			previous = value.Key
		}
		return nil
	}
	if err := validateCounts("per_role", coverage.PerRole); err != nil {
		return err
	}
	if err := validateCounts("per_package", coverage.PerPackage); err != nil {
		return err
	}
	return validateCandidateOmissions(coverage)
}

// validateCandidateOmissions checks the bounded omission aggregates: sorted
// unique closed reasons, positive counts that exactly partition the spans
// omitted from the advertised frontier, and bounded unique representative
// route-span refs. A complete frontier (zero omissions) records no omissions.
func validateCandidateOmissions(coverage CandidateCoverage) error {
	omitted := coverage.SpansConsidered - coverage.SpansSelected
	if omitted == 0 {
		if len(coverage.Omissions) != 0 {
			return fmt.Errorf("atlas study artifact: complete candidate records omissions")
		}
		return nil
	}
	if len(coverage.Omissions) == 0 {
		return fmt.Errorf("atlas study artifact: omitted candidate spans lack omission aggregates")
	}
	total := 0
	previousReason := CoverageOmissionReason("")
	for _, omission := range coverage.Omissions {
		if !omission.Reason.Valid() || omission.Count <= 0 ||
			omission.Count > coverage.SpansConsidered ||
			len(omission.Representatives) > MaxOmissionRepresentatives {
			return fmt.Errorf("atlas study artifact: invalid candidate omission")
		}
		if previousReason != "" && omission.Reason <= previousReason {
			return fmt.Errorf("atlas study artifact: candidate omissions are not canonical")
		}
		previousReason = omission.Reason
		total += omission.Count
		if len(omission.Representatives) == 0 {
			continue
		}
		if !uniqueCanonicalRefs(omission.Representatives) {
			return fmt.Errorf("atlas study artifact: omission representatives are not canonical")
		}
		for _, ref := range omission.Representatives {
			if ref.Kind != RefRouteSpan || ref.ID == "" {
				return fmt.Errorf("atlas study artifact: wrong-kind omission representative")
			}
		}
	}
	if total != omitted {
		return fmt.Errorf("atlas study artifact: omission aggregates do not cover the omitted span count")
	}
	return nil
}

func validateCatalogDigest(
	atlasSHA, architectureSHA, wireSHA, catalogSHA string,
	language Language,
	coverage CandidateCoverage,
	analysisTargetRoot *AnalysisTargetRootScope,
	catalog []CatalogObject,
) error {
	material := catalogMaterial{
		Version: Version, AtlasSHA256: atlasSHA, ArchitectureSHA256: architectureSHA,
		Language: language, Limits: DefaultLimits(), ProjectionSHA256: wireSHA,
		Coverage:           cloneCandidateCoverage(coverage),
		AnalysisTargetRoot: cloneAnalysisTargetRootScope(analysisTargetRoot),
		Objects:            cloneCatalog(catalog),
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return fmt.Errorf("atlas study artifact: encode catalog identity: %w", err)
	}
	if digest(encoded) != catalogSHA {
		return fmt.Errorf("atlas study artifact: catalog identity mismatch")
	}
	return nil
}

func validateCatalog(values []CatalogObject, analysisTargetRoot *AnalysisTargetRootScope) error {
	previousRank := -1
	previousID := ""
	seenRefs := make(map[string]struct{}, len(values))
	seenCanonical := make(map[CanonicalRef]struct{}, len(values))
	seenProducerIDs := make(map[string]struct{})
	ordinals := make(map[RefKind]int)
	identities := make(map[string]struct{}, 3*len(values))
	for _, object := range values {
		identities[object.CanonicalID] = struct{}{}
		if object.PackageBucket != "" {
			identities[object.PackageBucket] = struct{}{}
		}
		for _, private := range []string{
			object.ProducerID, object.SavedFlowID, object.FromStepID, object.ToStepID,
		} {
			if private != "" {
				identities[private] = struct{}{}
			}
		}
		if object.Location != nil {
			identities[object.Location.Path] = struct{}{}
		}
		if modelVisibleTargetSymbol(object.Symbol) != "" {
			identities[object.Symbol] = struct{}{}
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
		prefix := refPrefix(object.Kind)
		if !strings.HasPrefix(object.Ref, prefix) {
			return fmt.Errorf("atlas study artifact: noncanonical private ref")
		}
		ordinal, err := strconv.Atoi(strings.TrimPrefix(object.Ref, prefix))
		if err != nil || ordinal <= ordinals[object.Kind] || ordinal <= 0 {
			return fmt.Errorf("atlas study artifact: noncanonical private ref")
		}
		for skipped := ordinals[object.Kind] + 1; skipped < ordinal; skipped++ {
			if _, collision := identities[prefix+strconv.Itoa(skipped)]; !collision {
				return fmt.Errorf("atlas study artifact: noncanonical private ref gap")
			}
		}
		if _, collision := identities[object.Ref]; collision {
			return fmt.Errorf("atlas study artifact: private ref collides with private identity")
		}
		ordinals[object.Kind] = ordinal
		seenRefs[object.Ref] = struct{}{}
		previousRank, previousID = rank, object.CanonicalID
		if object.Kind == RefUnit {
			if !object.UnitKind.Valid() {
				return fmt.Errorf("atlas study artifact: invalid private Unit kind")
			}
		} else if object.UnitKind != "" || object.UnitParentID != "" {
			return fmt.Errorf("atlas study artifact: non-Unit object carries Unit kind")
		}
		if object.Kind == RefReadingTarget {
			if object.Location == nil || !repositoryLocation(*object.Location) ||
				!object.ReadingTargetKind.Valid() ||
				len(object.PrincipalRefs) == 0 || !uniqueCanonicalRefs(object.PrincipalRefs) ||
				!uniqueCanonicalRefs(object.RelatedComponentRefs) {
				return fmt.Errorf("atlas study artifact: reading target lacks exact private locator")
			}
		} else if object.Owner != nil || len(object.RelatedComponentRefs) != 0 ||
			len(object.PrincipalRefs) != 0 || object.ReadingTargetKind != "" {
			return fmt.Errorf("atlas study artifact: non-target object has private target associations")
		}
		if object.Location != nil && object.Kind != RefReadingTarget && object.Kind != RefEvidence {
			return fmt.Errorf("atlas study artifact: unexpected private locator")
		}
		if object.Kind == RefEvidence && (object.Location == nil || !repositoryLocation(*object.Location)) {
			return fmt.Errorf("atlas study artifact: evidence lacks exact private locator")
		}
		switch object.Kind {
		case RefRouteSupport:
			if !validSupportAuthority(object.SupportRole, object.Authority) || object.SupportTarget == nil ||
				object.SupportTarget.Kind != RefReadingTarget || object.PackageBucket == "" ||
				object.SpanKind != "" || object.Question != "" || object.TargetJob != "" ||
				object.LearningStage != "" || len(object.RequiredSupportRefs) != 0 ||
				len(object.AllowedTargetRefs) != 0 || len(object.SpanJoins) != 0 ||
				object.Location != nil || object.Symbol != "" {
				return fmt.Errorf("atlas study artifact: invalid route support")
			}
			if err := validateVisibleText(object.PackageBucket, DefaultLimits().MaxTextBytes, true, nil); err != nil {
				return fmt.Errorf("atlas study artifact: invalid route support package bucket")
			}
			if hasRouteRelationMetadata(object) {
				return fmt.Errorf("atlas study artifact: route support has producer relation metadata")
			}
		case RefRouteRelation:
			if object.Authority != repositoryatlas.AuthorityResolved || !object.RelationKind.Valid() ||
				object.ProducerID == "" || object.FromSupport == nil || object.ToSupport == nil ||
				object.FromTarget == nil || object.ToTarget == nil ||
				object.FromSupport.Kind != RefRouteSupport || object.ToSupport.Kind != RefRouteSupport ||
				object.FromTarget.Kind != RefReadingTarget || object.ToTarget.Kind != RefReadingTarget ||
				*object.FromSupport == *object.ToSupport || *object.FromTarget == *object.ToTarget ||
				object.SupportRole != "" || object.SupportTarget != nil || object.PackageBucket != "" ||
				object.SpanKind != "" || object.Question != "" || object.TargetJob != "" ||
				object.LearningStage != "" || len(object.RequiredSupportRefs) != 0 ||
				len(object.AllowedTargetRefs) != 0 || len(object.SpanJoins) != 0 ||
				object.Location != nil || object.Symbol != "" {
				return fmt.Errorf("atlas study artifact: invalid route producer relation")
			}
			if err := validateVisibleText(object.ProducerID, DefaultLimits().MaxTextBytes, true, nil); err != nil {
				return fmt.Errorf("atlas study artifact: invalid route producer identity")
			}
			if _, duplicate := seenProducerIDs[object.ProducerID]; duplicate {
				return fmt.Errorf("atlas study artifact: duplicate route producer identity")
			}
			seenProducerIDs[object.ProducerID] = struct{}{}
			switch object.RelationKind {
			case RouteRelationEntryHandoff:
				if object.SavedFlowID != "" || object.FromStepID != "" || object.ToStepID != "" ||
					object.FromStepOrdinal != 0 || object.ToStepOrdinal != 0 {
					return fmt.Errorf("atlas study artifact: entry-handoff relation has flow metadata")
				}
			case RouteRelationSavedFlowEdge:
				if object.SavedFlowID == "" || object.FromStepID == "" || object.ToStepID == "" ||
					object.FromStepID == object.ToStepID || object.FromStepOrdinal < 0 ||
					object.ToStepOrdinal != object.FromStepOrdinal+1 {
					return fmt.Errorf("atlas study artifact: invalid saved-flow relation")
				}
			}
		case RefRouteSpan:
			if !object.SpanKind.Valid() || !object.TargetJob.Valid() || !object.LearningStage.Valid() ||
				!naturalQuestion(object.Question) || len(object.RequiredSupportRefs) == 0 ||
				len(object.AllowedTargetRefs) == 0 ||
				!uniqueCanonicalRefs(object.RequiredSupportRefs) ||
				!uniqueCanonicalRefs(object.AllowedTargetRefs) || object.SupportRole != "" ||
				object.SupportTarget != nil || object.PackageBucket != "" || hasRouteRelationMetadata(object) || object.Location != nil ||
				object.Symbol != "" {
				return fmt.Errorf("atlas study artifact: invalid route span")
			}
			if err := validateVisibleText(object.Question, DefaultLimits().MaxTextBytes, true, identities); err != nil {
				return fmt.Errorf("atlas study artifact: invalid route span question")
			}
		default:
			if object.SupportRole != "" || object.SupportTarget != nil || object.PackageBucket != "" || hasRouteRelationMetadata(object) ||
				object.SpanKind != "" || object.Question != "" || object.TargetJob != "" ||
				object.LearningStage != "" || len(object.RequiredSupportRefs) != 0 ||
				len(object.AllowedTargetRefs) != 0 || len(object.SpanJoins) != 0 {
				return fmt.Errorf("atlas study artifact: unexpected route metadata")
			}
		}
		labelRequired := object.Kind != RefEvidence && object.Kind != RefRouteSupport &&
			object.Kind != RefRouteRelation && object.Kind != RefRouteSpan
		factRequired := object.Kind == RefSurface || object.Kind == RefReadingTarget ||
			object.Kind == RefEvidence || object.Kind == RefDocument
		labelIdentities := identities
		if object.Kind == RefReadingTarget {
			labelIdentities = make(map[string]struct{}, len(identities))
			for identity := range identities {
				labelIdentities[identity] = struct{}{}
			}
			delete(labelIdentities, modelVisibleTargetSymbol(object.Symbol))
		}
		if err := validateVisibleText(object.Label, DefaultLimits().MaxTextBytes, labelRequired, labelIdentities); err != nil {
			return fmt.Errorf("atlas study artifact: invalid private label")
		}
		if err := validateVisibleText(object.Fact, DefaultLimits().MaxTextBytes, factRequired, identities); err != nil {
			return fmt.Errorf("atlas study artifact: invalid private fact")
		}
	}
	for _, object := range values {
		if object.Kind == RefRouteSupport {
			if _, ok := seenCanonical[*object.SupportTarget]; !ok {
				return fmt.Errorf("atlas study artifact: route support target is outside private catalog")
			}
			continue
		}
		if object.Kind == RefRouteRelation {
			fromSupport, fromOK := catalogObjectByCanonical(values, *object.FromSupport)
			toSupport, toOK := catalogObjectByCanonical(values, *object.ToSupport)
			if !fromOK || !toOK || fromSupport.SupportTarget == nil || toSupport.SupportTarget == nil ||
				*fromSupport.SupportTarget != *object.FromTarget || *toSupport.SupportTarget != *object.ToTarget {
				return fmt.Errorf("atlas study artifact: route producer relation endpoints do not resolve exactly")
			}
			if _, ok := seenCanonical[*object.FromTarget]; !ok {
				return fmt.Errorf("atlas study artifact: route producer relation source is outside private catalog")
			}
			if _, ok := seenCanonical[*object.ToTarget]; !ok {
				return fmt.Errorf("atlas study artifact: route producer relation target is outside private catalog")
			}
			switch object.RelationKind {
			case RouteRelationEntryHandoff:
				if fromSupport.SupportRole != SupportProcessEntry || toSupport.SupportRole != SupportEntryHandoff {
					return fmt.Errorf("atlas study artifact: entry-handoff relation has wrong support roles")
				}
			case RouteRelationSavedFlowEdge:
				if fromSupport.SupportRole != SupportSavedFlow || toSupport.SupportRole != SupportSavedFlow {
					return fmt.Errorf("atlas study artifact: saved-flow relation has wrong support roles")
				}
			}
			continue
		}
		if object.Kind == RefRouteSpan {
			allowedTargets := make(map[CanonicalRef]struct{}, len(object.AllowedTargetRefs))
			for _, target := range object.AllowedTargetRefs {
				if target.Kind != RefReadingTarget {
					return fmt.Errorf("atlas study artifact: route span has wrong-kind allowed target")
				}
				if _, ok := seenCanonical[target]; !ok {
					return fmt.Errorf("atlas study artifact: route span target is outside private catalog")
				}
				allowedTargets[target] = struct{}{}
			}
			coveredTargets := make(map[CanonicalRef]struct{})
			requiredSupports := make(map[CanonicalRef]CatalogObject, len(object.RequiredSupportRefs))
			for _, supportRef := range object.RequiredSupportRefs {
				if supportRef.Kind != RefRouteSupport {
					return fmt.Errorf("atlas study artifact: route span has wrong-kind support")
				}
				support, ok := catalogObjectByCanonical(values, supportRef)
				if !ok || support.SupportTarget == nil {
					return fmt.Errorf("atlas study artifact: route span support is outside private catalog")
				}
				if support.SupportRole == SupportAnalysisTargetRoot && object.SpanKind != RouteSpanFocused {
					return fmt.Errorf("atlas study artifact: public API root support requires a focused span")
				}
				if _, allowed := allowedTargets[*support.SupportTarget]; !allowed {
					return fmt.Errorf("atlas study artifact: route span support target is not allowed")
				}
				coveredTargets[*support.SupportTarget] = struct{}{}
				requiredSupports[supportRef] = support
			}
			if object.SpanKind == RouteSpanFocused {
				if len(coveredTargets) != 1 || len(object.SpanJoins) != 0 {
					return fmt.Errorf("atlas study artifact: focused span has invalid exact shape")
				}
				continue
			}
			if len(coveredTargets) != 2 || len(requiredSupports) != 2 || len(allowedTargets) != 2 || len(object.SpanJoins) != 1 {
				return fmt.Errorf("atlas study artifact: system-path span must equal one directed producer relation")
			}
			join := object.SpanJoins[0]
			if join.Relation.Kind != RefRouteRelation {
				return fmt.Errorf("atlas study artifact: invalid route span join")
			}
			relation, ok := catalogObjectByCanonical(values, join.Relation)
			if !ok || relation.FromSupport == nil || relation.ToSupport == nil ||
				relation.FromTarget == nil || relation.ToTarget == nil {
				return fmt.Errorf("atlas study artifact: route span joins unknown producer relation")
			}
			if _, fromOK := requiredSupports[*relation.FromSupport]; !fromOK {
				return fmt.Errorf("atlas study artifact: route span join source is outside exact clauses")
			}
			if _, toOK := requiredSupports[*relation.ToSupport]; !toOK {
				return fmt.Errorf("atlas study artifact: route span join target is outside exact clauses")
			}
			if _, fromAllowed := allowedTargets[*relation.FromTarget]; !fromAllowed {
				return fmt.Errorf("atlas study artifact: route span join source target is not allowed")
			}
			if _, toAllowed := allowedTargets[*relation.ToTarget]; !toAllowed {
				return fmt.Errorf("atlas study artifact: route span join target is not allowed")
			}
			continue
		}
		principalSet := make(map[CanonicalRef]struct{}, len(object.PrincipalRefs))
		for _, principal := range object.PrincipalRefs {
			if principal.Kind != RefComponent && principal.Kind != RefSurface && principal.Kind != RefUnit {
				return fmt.Errorf("atlas study artifact: target has wrong-kind principal")
			}
			principalObject, ok := catalogObjectByCanonical(values, principal)
			if !ok || principal.Kind == RefUnit && principalObject.UnitKind != repositoryatlas.UnitPackage {
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
	if err := validateArtifactAnalysisTargetRootContract(values, analysisTargetRoot); err != nil {
		return err
	}
	return nil
}

func validateArtifactAnalysisTargetRootContract(
	values []CatalogObject,
	scope *AnalysisTargetRootScope,
) error {
	if err := validateAnalysisTargetRootScope(scope); err != nil {
		return fmt.Errorf("atlas study artifact: invalid selected AnalysisTarget root scope")
	}
	targets := make(map[CanonicalRef]CatalogObject)
	units := make(map[CanonicalRef]CatalogObject)
	allSupports := make(map[CanonicalRef][]CatalogObject)
	rootCounts := make(map[CanonicalRef]int)
	for _, object := range values {
		ref := CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
		switch object.Kind {
		case RefReadingTarget:
			targets[ref] = object
		case RefUnit:
			units[ref] = object
		case RefRouteSupport:
			if object.SupportTarget == nil {
				continue
			}
			allSupports[*object.SupportTarget] = append(allSupports[*object.SupportTarget], object)
			if object.SupportRole == SupportAnalysisTargetRoot {
				rootCounts[*object.SupportTarget]++
			}
		}
	}
	for _, object := range values {
		if object.Kind != RefRouteSupport || object.SupportRole != SupportAnalysisTargetRoot ||
			object.SupportTarget == nil {
			continue
		}
		target, ok := targets[*object.SupportTarget]
		if !ok || object.Authority != repositoryatlas.AuthorityResolved ||
			target.Authority != repositoryatlas.AuthorityResolved || target.Owner != nil ||
			(target.ReadingTargetKind != ReadingTargetFunction && target.ReadingTargetKind != ReadingTargetMethod) ||
			len(target.RelatedComponentRefs) != 0 || len(target.PrincipalRefs) != 1 ||
			target.PrincipalRefs[0].Kind != RefUnit {
			return fmt.Errorf("atlas study artifact: invalid public API root support")
		}
		unit, ok := units[target.PrincipalRefs[0]]
		if scope == nil || !ok || unit.UnitKind != repositoryatlas.UnitPackage ||
			unit.CanonicalID != scope.UnitID ||
			unit.Label != scope.AnalysisTarget.PackagePath ||
			unit.UnitParentID != scope.AnalysisTarget.ModuleID ||
			object.PackageBucket != scope.UnitID {
			return fmt.Errorf("atlas study artifact: public API root does not match its exact package Unit")
		}
	}
	for ref, target := range targets {
		hasUnit := false
		for _, principal := range target.PrincipalRefs {
			hasUnit = hasUnit || principal.Kind == RefUnit
		}
		if hasUnit {
			if rootCounts[ref] != 1 || len(allSupports[ref]) != 1 {
				return fmt.Errorf("atlas study artifact: package Unit principal is not one exact public API root")
			}
		} else if rootCounts[ref] != 0 {
			return fmt.Errorf("atlas study artifact: public API root support lacks a package Unit principal")
		}
	}
	if scope != nil && len(rootCounts) == 0 {
		return fmt.Errorf("atlas study artifact: selected AnalysisTarget root scope has no public API roots")
	}
	return nil
}

func catalogObjectByCanonical(values []CatalogObject, ref CanonicalRef) (CatalogObject, bool) {
	for _, object := range values {
		if object.Kind == ref.Kind && object.CanonicalID == ref.ID {
			return object, true
		}
	}
	return CatalogObject{}, false
}

func hasRouteRelationMetadata(object CatalogObject) bool {
	return object.RelationKind != "" || object.ProducerID != "" || object.FromSupport != nil ||
		object.ToSupport != nil || object.FromTarget != nil || object.ToTarget != nil ||
		object.SavedFlowID != "" || object.FromStepID != "" || object.ToStepID != "" ||
		object.FromStepOrdinal != 0 || object.ToStepOrdinal != 0
}

func validateStandaloneResult(record ResultRecord) error {
	if err := validateResultIdentity(record); err != nil {
		return err
	}
	product := productFromArtifact(
		record.AtlasSHA256, record.ArchitectureSHA256, record.WireSHA256,
		record.CatalogSHA256, record.CatalogRef, record.Language,
		record.CandidateCoverage, record.AnalysisTargetRoot, record.Catalog,
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
	coverage CandidateCoverage,
	analysisTargetRoot *AnalysisTargetRootScope,
	catalog []CatalogObject,
) Product {
	byRef := make(map[string]CatalogObject, len(catalog))
	byCanonical := make(map[CanonicalRef]CatalogObject, len(catalog))
	identities := make(map[string]struct{})
	alwaysPrivate := make(map[string]struct{})
	visiblePaths := make(map[string]struct{})
	visibleSymbols := make(map[string]struct{})
	for _, object := range catalog {
		if object.Kind != RefReadingTarget {
			continue
		}
		if object.Location != nil {
			visiblePaths[object.Location.Path] = struct{}{}
		}
		if symbol := modelVisibleTargetSymbol(object.Symbol); symbol != "" {
			visibleSymbols[symbol] = struct{}{}
		}
	}
	addAlwaysPrivate := func(value string) {
		if value == "" {
			return
		}
		identities[value] = struct{}{}
		alwaysPrivate[value] = struct{}{}
	}
	if analysisTargetRoot != nil {
		addAlwaysPrivate(analysisTargetRoot.AnalysisTarget.Ref)
		addAlwaysPrivate(analysisTargetRoot.UnitID)
	}
	for _, object := range catalog {
		byRef[object.Ref] = object
		byCanonical[CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}] = object
		addAlwaysPrivate(object.Ref)
		addAlwaysPrivate(object.CanonicalID)
		addAlwaysPrivate(object.PackageBucket)
		addAlwaysPrivate(object.ProducerID)
		addAlwaysPrivate(object.SavedFlowID)
		addAlwaysPrivate(object.FromStepID)
		addAlwaysPrivate(object.ToStepID)
		if object.Location != nil {
			identities[object.Location.Path] = struct{}{}
			if _, visible := visiblePaths[object.Location.Path]; !visible {
				alwaysPrivate[object.Location.Path] = struct{}{}
			}
		}
		if symbol := modelVisibleTargetSymbol(object.Symbol); symbol != "" {
			identities[symbol] = struct{}{}
			if _, visible := visibleSymbols[symbol]; !visible {
				alwaysPrivate[symbol] = struct{}{}
			}
		}
	}
	selectedSpanIDs := make([]string, 0)
	for _, object := range catalog {
		if object.Kind == RefRouteSpan {
			selectedSpanIDs = append(selectedSpanIDs, object.CanonicalID)
		}
	}
	return Product{
		input: Input{Language: language, Limits: DefaultLimits(),
			AnalysisTargetRoot: cloneAnalysisTargetRootScope(analysisTargetRoot)},
		wireSHA256: wireSHA, catalogSHA256: catalogSHA, catalogRef: catalogRef,
		atlasSHA256: atlasSHA, architectureSHA256: architectureSHA,
		catalog: cloneCatalog(catalog), byRef: byRef, byCanonical: byCanonical,
		privateIdentities: identities, alwaysPrivate: alwaysPrivate,
		coverage: cloneCandidateCoverage(coverage), selectedSpanIDs: selectedSpanIDs,
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
		if err := product.validateResolvedSupport(item.value.SupportRefs); err != nil {
			return err
		}
		if err := product.validateModelTextWithTargetLocators(
			item.value.Text, 1024, true, true,
			product.supportReadingTargets(item.value.SupportRefs),
		); err != nil {
			return fmt.Errorf("atlas study result artifact: brief.%s: %w", item.name, err)
		}
	}
	if len(brief.DomainTerms) > MaxDomainTerms {
		return fmt.Errorf("atlas study result artifact: invalid domain term count")
	}
	for _, term := range brief.DomainTerms {
		if err := product.validateResolvedSupport(term.SupportRefs); err != nil {
			return err
		}
		if err := product.validateModelText(term.Term, 128, true, false); err != nil {
			return err
		}
		if err := product.validateModelTextWithTargetLocators(
			term.Meaning, 512, true, true,
			product.supportReadingTargets(term.SupportRefs),
		); err != nil {
			return err
		}
	}
	return nil
}

func (product Product) validateResolvedSupport(refs []CanonicalRef) error {
	if len(refs) == 0 || !uniqueCanonicalRefs(refs) {
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
		!direction.TargetJob.Valid() || !direction.LearningStage.Valid() ||
		len(direction.PrincipalRefs) == 0 || len(direction.PrincipalRefs) > 5 ||
		len(direction.Reading) < MinDirectionReadingCount ||
		len(direction.Reading) > MaxDirectionReadingCount {
		return fmt.Errorf("invalid canonical direction")
	}
	span, ok := product.byCanonical[direction.Span]
	if !ok || span.Kind != RefRouteSpan || !span.SpanKind.Valid() ||
		direction.Question != span.Question || direction.TargetJob != span.TargetJob ||
		direction.LearningStage != span.LearningStage {
		return fmt.Errorf("unknown or mismatched route span")
	}
	allowedTargets := make(map[CanonicalRef]struct{}, len(span.AllowedTargetRefs))
	for _, target := range span.AllowedTargetRefs {
		allowedTargets[target] = struct{}{}
	}
	requiredSupports := make(map[CanonicalRef]struct{}, len(span.RequiredSupportRefs))
	for _, support := range span.RequiredSupportRefs {
		requiredSupports[support] = struct{}{}
	}
	coveredSupports := make(map[CanonicalRef]struct{}, len(requiredSupports))
	principalSet := make(map[CanonicalRef]struct{}, len(direction.PrincipalRefs))
	hasComponent, hasAnalysisTargetRoot := false, false
	if !uniqueCanonicalRefs(direction.PrincipalRefs) {
		return fmt.Errorf("principals are not canonical")
	}
	for _, ref := range direction.PrincipalRefs {
		if _, duplicate := principalSet[ref]; duplicate {
			return fmt.Errorf("duplicate principal")
		}
		principalSet[ref] = struct{}{}
		object, ok := product.byCanonical[ref]
		if !ok || (object.Kind != RefComponent && object.Kind != RefSurface &&
			(object.Kind != RefUnit || !product.isAnalysisTargetRootPrincipal(ref))) {
			return fmt.Errorf("unknown or wrong-kind principal")
		}
		if !product.advertisesPrincipal(ref) {
			return fmt.Errorf("principal is not advertised by a reading target")
		}
		hasComponent = hasComponent || object.Kind == RefComponent
		hasAnalysisTargetRoot = hasAnalysisTargetRoot || object.Kind == RefUnit
	}
	if !hasComponent && !hasAnalysisTargetRoot {
		return fmt.Errorf("component principal missing")
	}
	seenReading := make(map[CanonicalRef]struct{}, len(direction.Reading))
	coveredPrincipals := make(map[CanonicalRef]struct{}, len(principalSet))
	readingObjects := make([]CatalogObject, 0, len(direction.Reading))
	for _, reading := range direction.Reading {
		if _, duplicate := seenReading[reading.Target]; duplicate {
			return fmt.Errorf("duplicate reading target")
		}
		seenReading[reading.Target] = struct{}{}
		object, ok := product.byCanonical[reading.Target]
		if !ok || object.Kind != RefReadingTarget || len(object.PrincipalRefs) == 0 {
			return fmt.Errorf("unknown or wrong-kind reading target")
		}
		if _, allowed := allowedTargets[reading.Target]; !allowed {
			return fmt.Errorf("reading target is outside exact route span")
		}
		coversRequiredSupport := false
		for _, support := range product.catalog {
			if support.Kind != RefRouteSupport || support.SupportTarget == nil ||
				*support.SupportTarget != reading.Target {
				continue
			}
			ref := CanonicalRef{Kind: RefRouteSupport, ID: support.CanonicalID}
			if _, required := requiredSupports[ref]; required {
				coveredSupports[ref] = struct{}{}
				coversRequiredSupport = true
			}
		}
		if !coversRequiredSupport {
			return fmt.Errorf("reading target does not cover a required route clause")
		}
		if !intersectsPrincipalSet(object.PrincipalRefs, principalSet) {
			return fmt.Errorf("reading target has no selected principal")
		}
		for _, principal := range object.PrincipalRefs {
			if _, selected := principalSet[principal]; selected {
				coveredPrincipals[principal] = struct{}{}
			}
		}
		if !reading.Label.Valid() {
			return fmt.Errorf("invalid reading guidance")
		}
		if product.validateModelTextWithTargetLocators(
			reading.WhatToLookFor, 768, true, true, []CatalogObject{object},
		) != nil {
			return fmt.Errorf("invalid reading guidance")
		}
		readingObjects = append(readingObjects, object)
	}
	if len(coveredPrincipals) != len(principalSet) {
		return fmt.Errorf("principal is not advertised by the selected reading targets")
	}
	if len(coveredSupports) != len(requiredSupports) {
		return fmt.Errorf("route span support is incomplete")
	}
	if span.SpanKind == RouteSpanSystemPath && len(readingObjects) < 2 {
		return fmt.Errorf("system-path route is too short")
	}
	if product.validateModelTextWithTargetLocators(
		direction.Question, 512, true, false, readingObjects,
	) != nil || product.validateModelTextWithTargetLocators(
		direction.WhyItMatters, 1024, true, true, readingObjects,
	) != nil || product.validateModelTextWithTargetLocators(
		direction.LearningOutcome, 1024, true, true, readingObjects,
	) != nil {
		return fmt.Errorf("invalid canonical direction")
	}
	return nil
}

func validateDiagnostics(diagnostics Diagnostics, acceptedDirections, acceptedTerms int) error {
	wantDomainTermIssues := diagnostics.DomainTermsRejected
	if wantDomainTermIssues > MaxDomainTermDiagnostics {
		wantDomainTermIssues = MaxDomainTermDiagnostics
	}
	if diagnostics.DirectionsReceived < acceptedDirections || diagnostics.DirectionsReceived < 1 ||
		diagnostics.DirectionsAccepted != acceptedDirections ||
		diagnostics.DirectionsRejected != diagnostics.DirectionsReceived-acceptedDirections ||
		len(diagnostics.Issues) > MaxDirectionDiagnostics ||
		diagnostics.DomainTermsReceived < acceptedTerms ||
		diagnostics.DomainTermsAccepted != acceptedTerms ||
		diagnostics.DomainTermsRejected != diagnostics.DomainTermsReceived-acceptedTerms ||
		len(diagnostics.DomainTermIssues) != wantDomainTermIssues {
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
	previous = -1
	for _, issue := range diagnostics.DomainTermIssues {
		if issue.Position < 0 || issue.Position >= diagnostics.DomainTermsReceived ||
			issue.Position <= previous || !issue.Code.Valid() {
			return fmt.Errorf("atlas study result artifact: invalid domain term diagnostic issue")
		}
		if (issue.Position >= MaxDomainTerms) != (issue.Code == DomainTermIssueUnrequestedOutput) {
			return fmt.Errorf("atlas study result artifact: inconsistent domain term diagnostic issue")
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
	value.DomainTermIssues = append([]DomainTermIssue(nil), value.DomainTermIssues...)
	return value
}
