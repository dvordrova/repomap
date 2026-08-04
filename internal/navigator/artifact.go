package navigator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

const (
	RequestArtifactFilename = "navigator_request.v1.json"
	RecordArtifactFilename  = "navigator_result.v1.json"
	StatusArtifactFilename  = "navigator_status.v1.json"

	MaxStatusArtifactBytes = 64 << 10
)

var productSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProductState string

const (
	ProductStateEmpty       ProductState = "empty"
	ProductStatePrepared    ProductState = "prepared"
	ProductStateSelected    ProductState = "selected"
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

// RequestRecord is the local persisted identity of the exact provider-visible
// wire plus the backend-only action catalog used to restore a selection.
type RequestRecord struct {
	Version       int                    `json:"version"`
	AtlasSHA256   string                 `json:"atlas_sha256"`
	WireSHA256    string                 `json:"wire_sha256"`
	CatalogSHA256 string                 `json:"catalog_sha256"`
	CatalogRef    string                 `json:"catalog_ref"`
	Question      string                 `json:"question"`
	Actions       []RecommendationAction `json:"actions"`
	WireJSON      string                 `json:"wire_json"`
}

type RecommendationRecord struct {
	Version       int                    `json:"version"`
	State         ProductState           `json:"state"`
	AtlasSHA256   string                 `json:"atlas_sha256"`
	WireSHA256    string                 `json:"wire_sha256,omitempty"`
	CatalogSHA256 string                 `json:"catalog_sha256,omitempty"`
	CatalogRef    string                 `json:"catalog_ref,omitempty"`
	Question      string                 `json:"question"`
	Actions       []RecommendationAction `json:"actions"`
	Selected      *RecommendationAction  `json:"selected,omitempty"`
}

// Status is intentionally closed and contains no provider text, endpoint,
// source locator, or error string. The semantic exchange journal remains the
// sole Q/A recorder.
type Status struct {
	Version           int             `json:"version"`
	State             ProductState    `json:"state"`
	AtlasSHA256       string          `json:"atlas_sha256"`
	WireSHA256        string          `json:"wire_sha256,omitempty"`
	CatalogSHA256     string          `json:"catalog_sha256,omitempty"`
	CatalogRef        string          `json:"catalog_ref,omitempty"`
	ActionCount       int             `json:"action_count"`
	SelectedActionKey string          `json:"selected_action_key,omitempty"`
	UnavailableCode   UnavailableCode `json:"unavailable_code,omitempty"`
	FailureCode       FailureCode     `json:"failure_code,omitempty"`
}

func (product Product) RequestRecord() (RequestRecord, error) {
	compiled, ok := product.CompiledRequest()
	if !ok {
		return RequestRecord{}, fmt.Errorf("navigator product: empty local result has no provider request")
	}
	return RequestRecord{
		Version: ProductVersion, AtlasSHA256: product.atlasSHA256,
		WireSHA256: compiled.WireSHA256(), CatalogSHA256: compiled.CatalogSHA256(),
		CatalogRef: compiled.CatalogRef(), Question: ProductQuestion,
		Actions: product.Actions(), WireJSON: string(compiled.WireJSON()),
	}, nil
}

// ValidateRequestRecord proves a decoded request is the exact wire/private
// catalog produced by this Product, not only a well-shaped persisted value.
func (product Product) ValidateRequestRecord(record RequestRecord) error {
	if err := validateRequestRecord(record); err != nil {
		return err
	}
	compiled, ok := product.CompiledRequest()
	if !ok {
		return fmt.Errorf("navigator product: empty local result has no provider request")
	}
	if record.AtlasSHA256 != product.atlasSHA256 ||
		record.WireSHA256 != compiled.WireSHA256() ||
		record.CatalogSHA256 != compiled.CatalogSHA256() ||
		record.CatalogRef != compiled.CatalogRef() ||
		record.WireJSON != string(compiled.WireJSON()) ||
		!recommendationActionSlicesEqual(record.Actions, product.actions) {
		return fmt.Errorf("navigator product: request record does not match compiled product")
	}
	return nil
}

// ValidateRecommendationRecord proves a decoded result was selected from this
// exact Product request and backend-owned action catalog.
func (product Product) ValidateRecommendationRecord(record RecommendationRecord) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	wantState := ProductStateSelected
	if product.empty {
		wantState = ProductStateEmpty
	}
	if record.State != wantState || record.AtlasSHA256 != product.atlasSHA256 ||
		!recommendationActionSlicesEqual(record.Actions, product.actions) {
		return fmt.Errorf("navigator product: result record does not match compiled product")
	}
	if product.empty {
		return nil
	}
	if record.WireSHA256 != product.compiled.WireSHA256() ||
		record.CatalogSHA256 != product.compiled.CatalogSHA256() ||
		record.CatalogRef != product.compiled.CatalogRef() {
		return fmt.Errorf("navigator product: result record does not match compiled product")
	}
	selected, ok := product.byKey[record.Selected.Key]
	if !ok || !recommendationActionsEqual(selected, *record.Selected) {
		return fmt.Errorf("navigator product: selected result is outside compiled product")
	}
	return nil
}

func (product Product) PreparedStatus() Status {
	if product.empty {
		return product.status(ProductStateEmpty, "", "", "")
	}
	return product.status(ProductStatePrepared, "", "", "")
}

func (product Product) SelectedStatus(record RecommendationRecord) (Status, error) {
	if record.State != ProductStateSelected || record.Selected == nil {
		return Status{}, fmt.Errorf("navigator product: selected status requires one selected record")
	}
	if err := product.ValidateRecommendationRecord(record); err != nil {
		return Status{}, err
	}
	return product.status(ProductStateSelected, record.Selected.Key, "", ""), nil
}

func (product Product) UnavailableStatus(code UnavailableCode) (Status, error) {
	if product.empty {
		return Status{}, fmt.Errorf("navigator product: empty local result cannot be unavailable")
	}
	if code != UnavailableOffline {
		return Status{}, fmt.Errorf("navigator product: unsupported unavailable code %q", code)
	}
	return product.status(ProductStateUnavailable, "", code, ""), nil
}

func (product Product) FailureStatus(code FailureCode) (Status, error) {
	if product.empty {
		return Status{}, fmt.Errorf("navigator product: empty local result cannot have a provider failure")
	}
	if !validFailureCode(code) {
		return Status{}, fmt.Errorf("navigator product: unsupported failure code %q", code)
	}
	return product.status(ProductStateFailed, "", "", code), nil
}

func (product Product) status(
	state ProductState,
	selected string,
	unavailable UnavailableCode,
	failure FailureCode,
) Status {
	status := Status{
		Version: ProductVersion, State: state, AtlasSHA256: product.atlasSHA256,
		ActionCount: len(product.actions), SelectedActionKey: selected,
		UnavailableCode: unavailable, FailureCode: failure,
	}
	if !product.empty {
		status.WireSHA256 = product.compiled.WireSHA256()
		status.CatalogSHA256 = product.compiled.CatalogSHA256()
		status.CatalogRef = product.compiled.CatalogRef()
	}
	return status
}

func EncodeRequestRecord(record RequestRecord) ([]byte, error) {
	if err := validateRequestRecord(record); err != nil {
		return nil, err
	}
	return encodeProductArtifact(record)
}

func DecodeRequestRecord(data []byte) (RequestRecord, error) {
	var record RequestRecord
	if err := decodeProductArtifact("request", data, repositoryatlas.MaxArtifactBytes, &record); err != nil {
		return RequestRecord{}, err
	}
	if err := validateRequestRecord(record); err != nil {
		return RequestRecord{}, err
	}
	return record, requireCanonicalProductArtifact("request", data, record)
}

// ValidateRequestRecordAgainstAtlas proves that an independently decoded
// request advertises exactly the eligible application startup relations in
// its bound Atlas. Consumers do not duplicate private action-key derivation.
func ValidateRequestRecordAgainstAtlas(
	record RequestRecord,
	atlas repositoryatlas.Atlas,
) error {
	if err := validateRequestRecord(record); err != nil {
		return err
	}
	canonical, err := repositoryatlas.Canonical(atlas)
	if err != nil {
		return fmt.Errorf("navigator request artifact: Atlas: %w", err)
	}
	encoded, err := repositoryatlas.CanonicalJSON(canonical)
	if err != nil {
		return fmt.Errorf("navigator request artifact: encode Atlas identity: %w", err)
	}
	if productDigest(encoded) != record.AtlasSHA256 {
		return fmt.Errorf("navigator request artifact: Atlas sha256 mismatch")
	}
	expected := startupActionsFromAtlas(canonical)
	if len(expected) == 0 || !recommendationActionSlicesEqual(expected, record.Actions) {
		return fmt.Errorf("navigator request artifact: action catalog does not match the exact startup vertical")
	}
	return nil
}

func EncodeRecommendationRecord(record RecommendationRecord) ([]byte, error) {
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	return encodeProductArtifact(record)
}

func DecodeRecommendationRecord(data []byte) (RecommendationRecord, error) {
	var record RecommendationRecord
	if err := decodeProductArtifact("result", data, repositoryatlas.MaxArtifactBytes, &record); err != nil {
		return RecommendationRecord{}, err
	}
	if err := validateRecord(record); err != nil {
		return RecommendationRecord{}, err
	}
	return record, requireCanonicalProductArtifact("result", data, record)
}

// ValidateRecommendationRecordAgainstAtlas proves that the persisted action
// catalog and selected canonical identities are exactly the eligible startup
// vertical from the bound Atlas, not merely well-shaped local values.
func ValidateRecommendationRecordAgainstAtlas(
	record RecommendationRecord,
	atlas repositoryatlas.Atlas,
) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	canonical, err := repositoryatlas.Canonical(atlas)
	if err != nil {
		return fmt.Errorf("navigator result artifact: Atlas: %w", err)
	}
	encoded, err := repositoryatlas.CanonicalJSON(canonical)
	if err != nil {
		return fmt.Errorf("navigator result artifact: encode Atlas identity: %w", err)
	}
	if productDigest(encoded) != record.AtlasSHA256 {
		return fmt.Errorf("navigator result artifact: Atlas sha256 mismatch")
	}
	expected := startupActionsFromAtlas(canonical)
	if len(expected) != len(record.Actions) {
		return fmt.Errorf("navigator result artifact: action catalog does not cover the exact startup vertical")
	}
	for index := range expected {
		if !recommendationActionsEqual(expected[index], record.Actions[index]) {
			return fmt.Errorf("navigator result artifact: action catalog does not match the exact startup vertical")
		}
	}
	if record.State == ProductStateEmpty && len(expected) != 0 {
		return fmt.Errorf("navigator result artifact: nonempty Atlas cannot use an empty result")
	}
	return nil
}

func EncodeStatus(status Status) ([]byte, error) {
	if err := validateStatus(status); err != nil {
		return nil, err
	}
	return encodeProductArtifact(status)
}

func DecodeStatus(data []byte) (Status, error) {
	var status Status
	if err := decodeProductArtifact("status", data, MaxStatusArtifactBytes, &status); err != nil {
		return Status{}, err
	}
	if err := validateStatus(status); err != nil {
		return Status{}, err
	}
	return status, requireCanonicalProductArtifact("status", data, status)
}

// ValidateStatusAgainstAtlas proves that a persisted status is bound to this
// exact Atlas and describes its complete eligible application startup set.
// Consumers do not need to duplicate Navigator's private action-key derivation.
func ValidateStatusAgainstAtlas(status Status, atlas repositoryatlas.Atlas) error {
	if err := validateStatus(status); err != nil {
		return err
	}
	canonical, err := repositoryatlas.Canonical(atlas)
	if err != nil {
		return fmt.Errorf("navigator status artifact: Atlas: %w", err)
	}
	encoded, err := repositoryatlas.CanonicalJSON(canonical)
	if err != nil {
		return fmt.Errorf("navigator status artifact: encode Atlas identity: %w", err)
	}
	if productDigest(encoded) != status.AtlasSHA256 {
		return fmt.Errorf("navigator status artifact: Atlas sha256 mismatch")
	}
	expected := startupActionsFromAtlas(canonical)
	if status.ActionCount != len(expected) {
		return fmt.Errorf("navigator status artifact: action count does not match the exact startup vertical")
	}
	if status.State == ProductStateEmpty {
		if len(expected) != 0 {
			return fmt.Errorf("navigator status artifact: nonempty Atlas cannot use an empty status")
		}
		return nil
	}
	if len(expected) == 0 {
		return fmt.Errorf("navigator status artifact: empty Atlas cannot use a nonempty status")
	}
	if status.State == ProductStateSelected {
		for _, action := range expected {
			if action.Key == status.SelectedActionKey {
				return nil
			}
		}
		return fmt.Errorf("navigator status artifact: selected action is outside the exact startup vertical")
	}
	return nil
}

func validateRequestRecord(record RequestRecord) error {
	if record.Version != ProductVersion || record.Question != ProductQuestion ||
		!validProductSHA(record.AtlasSHA256) || !validProductSHA(record.WireSHA256) ||
		!validProductSHA(record.CatalogSHA256) || record.CatalogRef != "navigator-v1-"+record.CatalogSHA256 {
		return fmt.Errorf("navigator request artifact: invalid identity")
	}
	if len(record.Actions) == 0 || record.WireJSON == "" || !json.Valid([]byte(record.WireJSON)) ||
		productDigest([]byte(record.WireJSON)) != record.WireSHA256 {
		return fmt.Errorf("navigator request artifact: invalid wire or action catalog")
	}
	return validateRecommendationActions(record.Actions)
}

func validateRecord(record RecommendationRecord) error {
	if record.Version != ProductVersion || record.Question != ProductQuestion || !validProductSHA(record.AtlasSHA256) {
		return fmt.Errorf("navigator result artifact: invalid identity")
	}
	if err := validateRecommendationActions(record.Actions); err != nil {
		return err
	}
	switch record.State {
	case ProductStateEmpty:
		if len(record.Actions) != 0 || record.Selected != nil || record.WireSHA256 != "" ||
			record.CatalogSHA256 != "" || record.CatalogRef != "" {
			return fmt.Errorf("navigator result artifact: invalid empty result")
		}
	case ProductStateSelected:
		if len(record.Actions) == 0 || record.Selected == nil ||
			!validProductSHA(record.WireSHA256) || !validProductSHA(record.CatalogSHA256) ||
			record.CatalogRef != "navigator-v1-"+record.CatalogSHA256 {
			return fmt.Errorf("navigator result artifact: invalid selected result")
		}
		matched := false
		for _, action := range record.Actions {
			if action.Key == record.Selected.Key && recommendationActionsEqual(action, *record.Selected) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("navigator result artifact: selected action is outside the advertised catalog")
		}
	default:
		return fmt.Errorf("navigator result artifact: unsupported state %q", record.State)
	}
	return nil
}

func validateStatus(status Status) error {
	if status.Version != ProductVersion || !validProductSHA(status.AtlasSHA256) || status.ActionCount < 0 {
		return fmt.Errorf("navigator status artifact: invalid identity")
	}
	hasRequest := validProductSHA(status.WireSHA256) && validProductSHA(status.CatalogSHA256) &&
		status.CatalogRef == "navigator-v1-"+status.CatalogSHA256
	switch status.State {
	case ProductStateEmpty:
		if status.ActionCount != 0 || status.SelectedActionKey != "" ||
			status.UnavailableCode != "" || status.FailureCode != "" ||
			status.WireSHA256 != "" || status.CatalogSHA256 != "" || status.CatalogRef != "" {
			return fmt.Errorf("navigator status artifact: invalid empty status")
		}
	case ProductStatePrepared:
		if !hasRequest || status.ActionCount == 0 || status.SelectedActionKey != "" ||
			status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("navigator status artifact: invalid prepared status")
		}
	case ProductStateSelected:
		if !hasRequest || status.ActionCount == 0 || status.SelectedActionKey == "" ||
			status.UnavailableCode != "" || status.FailureCode != "" {
			return fmt.Errorf("navigator status artifact: invalid selected status")
		}
	case ProductStateUnavailable:
		if !hasRequest || status.ActionCount == 0 || status.SelectedActionKey != "" ||
			status.UnavailableCode != UnavailableOffline || status.FailureCode != "" {
			return fmt.Errorf("navigator status artifact: invalid unavailable status")
		}
	case ProductStateFailed:
		if !hasRequest || status.ActionCount == 0 || status.SelectedActionKey != "" ||
			status.UnavailableCode != "" || !validFailureCode(status.FailureCode) {
			return fmt.Errorf("navigator status artifact: invalid failed status")
		}
	default:
		return fmt.Errorf("navigator status artifact: unsupported state %q", status.State)
	}
	return nil
}

func validateRecommendationActions(actions []RecommendationAction) error {
	previous := ""
	for _, action := range actions {
		if action.Key == "" || action.Key <= previous || action.Operation != StartupActionOperation ||
			action.Surface.Kind != repositoryatlas.EntitySurface || action.Surface.ID == "" ||
			action.Application.Kind != repositoryatlas.EntityOperation || action.Application.ID == "" ||
			action.RelationID == "" || len(action.EvidenceIDs) == 0 || !slices.IsSorted(action.EvidenceIDs) {
			return fmt.Errorf("navigator artifact: invalid backend action catalog")
		}
		for index, id := range action.EvidenceIDs {
			if id == "" || (index > 0 && id == action.EvidenceIDs[index-1]) {
				return fmt.Errorf("navigator artifact: invalid backend action evidence")
			}
		}
		previous = action.Key
	}
	return nil
}

func recommendationActionsEqual(left, right RecommendationAction) bool {
	return left.Key == right.Key && left.Operation == right.Operation &&
		left.Surface == right.Surface && left.Application == right.Application &&
		left.RelationID == right.RelationID && slices.Equal(left.EvidenceIDs, right.EvidenceIDs)
}

func recommendationActionSlicesEqual(left, right []RecommendationAction) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !recommendationActionsEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func startupActionsFromAtlas(atlas repositoryatlas.Atlas) []RecommendationAction {
	result := make([]RecommendationAction, 0)
	for _, relation := range startupRelationsFromAtlas(atlas) {
		action := RecommendationAction{
			Key: startupActionKey(relation.ID), Operation: StartupActionOperation,
			Surface: relation.Source, Application: relation.Target,
			RelationID: relation.ID, EvidenceIDs: append([]string(nil), relation.EvidenceRefs...),
		}
		sort.Strings(action.EvidenceIDs)
		result = append(result, action)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

// startupRelationsFromAtlas accepts only the exact application vertical
// already encoded by Atlas: relation, Surface and Operation must share the
// same UnitApp. Navigator never infers availability from provider/UI data.
func startupRelationsFromAtlas(atlas repositoryatlas.Atlas) []repositoryatlas.Relation {
	units := make(map[string]repositoryatlas.Unit, len(atlas.Units))
	for _, unit := range atlas.Units {
		units[unit.ID] = unit
	}
	entities := make(map[string]repositoryatlas.Entity, len(atlas.Entities))
	for _, entity := range atlas.Entities {
		entities[entity.ID] = entity
	}
	result := make([]repositoryatlas.Relation, 0)
	for _, relation := range atlas.Relations {
		surface := entities[relation.Source.ID]
		operation := entities[relation.Target.ID]
		if relation.Kind != repositoryatlas.RelationExposes ||
			relation.Phase != repositoryatlas.PhaseStartup ||
			relation.Authority != repositoryatlas.AuthorityResolved ||
			relation.Source.Kind != repositoryatlas.EntitySurface ||
			relation.Target.Kind != repositoryatlas.EntityOperation ||
			surface.Kind != repositoryatlas.EntitySurface ||
			operation.Kind != repositoryatlas.EntityOperation ||
			surface.UnitID != relation.UnitID || operation.UnitID != relation.UnitID ||
			units[relation.UnitID].Kind != repositoryatlas.UnitApp {
			continue
		}
		result = append(result, relation)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func validProductSHA(value string) bool { return productSHA256Pattern.MatchString(value) }

func validFailureCode(code FailureCode) bool {
	switch code {
	case FailureProvider, FailureDecode, FailureReference, FailureResource, FailureCanceled:
		return true
	default:
		return false
	}
}

func encodeProductArtifact(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("navigator artifact: encode: %w", err)
	}
	return append(encoded, '\n'), nil
}

func decodeProductArtifact(kind string, data []byte, limit int, target any) error {
	if len(data) == 0 || len(data) > limit {
		return fmt.Errorf("navigator %s artifact: size must be between 1 and %d bytes", kind, limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("navigator %s artifact: decode: %w", kind, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("navigator %s artifact: multiple json values", kind)
		}
		return fmt.Errorf("navigator %s artifact: trailing data: %w", kind, err)
	}
	return nil
}

func requireCanonicalProductArtifact(kind string, data []byte, value any) error {
	canonical, err := encodeProductArtifact(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("navigator %s artifact: bytes are not canonical", kind)
	}
	return nil
}
