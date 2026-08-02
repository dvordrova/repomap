package navigator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

const (
	ProductVersion = 1

	ProductQuestion        = "Which locally resolved application startup should a newcomer inspect first?"
	StartupActionOperation = "inspect exact startup evidence"
)

// ProductInput is the complete local input for the first Atlas-first product
// question. Limits remain explicit; the product compiler never trims eligible
// startup relations or substitutes a smaller request.
type ProductInput struct {
	Atlas  repositoryatlas.Atlas
	Limits Limits
}

// RecommendationAction is the local, backend-owned meaning of one advertised
// request action. Canonical identities stay out of the provider wire and are
// restored only after the request-local action ref validates.
type RecommendationAction struct {
	Key         string                    `json:"key"`
	Operation   string                    `json:"operation"`
	Surface     repositoryatlas.EntityRef `json:"surface"`
	Application repositoryatlas.EntityRef `json:"application_operation"`
	RelationID  string                    `json:"relation_id"`
	EvidenceIDs []string                  `json:"evidence_ids"`
}

// Product is an immutable compiled first-question request. An empty Product is
// a successful local outcome and must not be sent to a provider.
type Product struct {
	atlas       repositoryatlas.Atlas
	atlasSHA256 string
	compiled    Compiled
	actions     []RecommendationAction
	byKey       map[string]RecommendationAction
	empty       bool
}

// CompileProduct derives only exact exposes/startup/resolved
// Surface-to-Operation relations. All other Atlas relations remain local and
// are removed from the task-shaped projection before Compile derives trails.
func CompileProduct(input ProductInput) (Product, error) {
	atlas, err := repositoryatlas.Canonical(input.Atlas)
	if err != nil {
		return Product{}, fmt.Errorf("navigator product: Atlas: %w", err)
	}
	encodedAtlas, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		return Product{}, fmt.Errorf("navigator product: encode Atlas identity: %w", err)
	}
	product := Product{
		atlas: atlas, atlasSHA256: productDigest(encodedAtlas),
		byKey: make(map[string]RecommendationAction),
	}

	repositoryUnitID, err := exactRepositoryUnitID(atlas.Units)
	if err != nil {
		return Product{}, err
	}
	eligible := startupRelationsFromAtlas(atlas)
	if len(eligible) == 0 {
		product.empty = true
		return product, nil
	}

	seedByID := make(map[string]repositoryatlas.EntityRef)
	inputActions := make([]Action, 0, len(eligible))
	for _, relation := range eligible {
		key := startupActionKey(relation.ID)
		action := RecommendationAction{
			Key: key, Operation: StartupActionOperation,
			Surface: relation.Source, Application: relation.Target,
			RelationID: relation.ID, EvidenceIDs: append([]string(nil), relation.EvidenceRefs...),
		}
		sort.Strings(action.EvidenceIDs)
		product.actions = append(product.actions, action)
		product.byKey[key] = action
		seedByID[relation.Source.ID] = relation.Source
		inputActions = append(inputActions, Action{
			Key: key, Operation: StartupActionOperation, Target: relation.Source,
		})
	}
	sort.Slice(product.actions, func(i, j int) bool { return product.actions[i].Key < product.actions[j].Key })
	seedIDs := make([]string, 0, len(seedByID))
	for id := range seedByID {
		seedIDs = append(seedIDs, id)
	}
	sort.Strings(seedIDs)
	seeds := make([]repositoryatlas.EntityRef, 0, len(seedIDs))
	for _, id := range seedIDs {
		seeds = append(seeds, seedByID[id])
	}

	projectionAtlas := atlas
	projectionAtlas.Relations = eligible
	compiled, err := Compile(Input{
		Atlas: projectionAtlas, Question: ProductQuestion,
		ScopeUnitID: repositoryUnitID, Seeds: seeds, Actions: inputActions,
		Limits: input.Limits,
	})
	if err != nil {
		return Product{}, fmt.Errorf("navigator product: compile first question: %w", err)
	}
	product.compiled = compiled
	return product, nil
}

func (product Product) Empty() bool { return product.empty }

// CompiledRequest returns the exact request-local projection only when a
// provider call is required.
func (product Product) CompiledRequest() (Compiled, bool) {
	if product.empty || product.compiled.CatalogRef() == "" {
		return Compiled{}, false
	}
	return product.compiled, true
}

func (product Product) AtlasSHA256() string { return product.atlasSHA256 }

func (product Product) Actions() []RecommendationAction {
	return cloneRecommendationActions(product.actions)
}

// ResolveRecommendation accepts exactly one coherent advertised action. The
// provider must select its matching trail, both endpoints, and all exact
// evidence refs; unrelated request-local refs cannot be used as an explanation.
func (product Product) ResolveRecommendation(data []byte) (RecommendationRecord, error) {
	if product.empty {
		return RecommendationRecord{}, fmt.Errorf("navigator product: empty local result requires no provider response")
	}
	resolved, err := product.compiled.ValidateResponseJSON(data)
	if err != nil {
		return RecommendationRecord{}, err
	}
	if len(resolved.Actions) != 1 {
		return RecommendationRecord{}, fmt.Errorf("navigator product: response must select exactly one advertised action")
	}
	selected, ok := product.byKey[resolved.Actions[0].Key]
	if !ok || resolved.Actions[0].Operation != selected.Operation ||
		resolved.Actions[0].Target != selected.Surface {
		return RecommendationRecord{}, fmt.Errorf("navigator product: selected action does not match the backend catalog")
	}
	if !slices.Equal(resolved.RelationIDs, []string{selected.RelationID}) {
		return RecommendationRecord{}, fmt.Errorf("navigator product: response must cite the selected startup relation")
	}
	wantEntities := []repositoryatlas.EntityRef{selected.Surface, selected.Application}
	sort.Slice(wantEntities, func(i, j int) bool {
		if wantEntities[i].Kind != wantEntities[j].Kind {
			return wantEntities[i].Kind < wantEntities[j].Kind
		}
		return wantEntities[i].ID < wantEntities[j].ID
	})
	gotEntities := append([]repositoryatlas.EntityRef(nil), resolved.Entities...)
	sort.Slice(gotEntities, func(i, j int) bool {
		if gotEntities[i].Kind != gotEntities[j].Kind {
			return gotEntities[i].Kind < gotEntities[j].Kind
		}
		return gotEntities[i].ID < gotEntities[j].ID
	})
	if !slices.Equal(gotEntities, wantEntities) {
		return RecommendationRecord{}, fmt.Errorf("navigator product: response must cite both selected startup endpoints")
	}
	gotEvidence := append([]string(nil), resolved.EvidenceIDs...)
	sort.Strings(gotEvidence)
	if !slices.Equal(gotEvidence, selected.EvidenceIDs) {
		return RecommendationRecord{}, fmt.Errorf("navigator product: response must cite the selected startup evidence")
	}
	if len(resolved.IntersectionEntityIDs) != 0 || len(resolved.GapKeys) != 0 {
		return RecommendationRecord{}, fmt.Errorf("navigator product: response contains refs outside the first startup question")
	}
	return product.record(ProductStateSelected, &selected), nil
}

// EmptyRecord returns the explicit no-provider local result.
func (product Product) EmptyRecord() (RecommendationRecord, error) {
	if !product.empty {
		return RecommendationRecord{}, fmt.Errorf("navigator product: eligible startup actions require a provider response")
	}
	return product.record(ProductStateEmpty, nil), nil
}

func (product Product) record(state ProductState, selected *RecommendationAction) RecommendationRecord {
	record := RecommendationRecord{
		Version: ProductVersion, State: state, AtlasSHA256: product.atlasSHA256,
		Question: ProductQuestion, Actions: cloneRecommendationActions(product.actions),
	}
	if !product.empty {
		record.WireSHA256 = product.compiled.WireSHA256()
		record.CatalogSHA256 = product.compiled.CatalogSHA256()
		record.CatalogRef = product.compiled.CatalogRef()
	}
	if selected != nil {
		cloned := cloneRecommendationAction(*selected)
		record.Selected = &cloned
	}
	return record
}

func exactRepositoryUnitID(units []repositoryatlas.Unit) (string, error) {
	result := ""
	for _, unit := range units {
		if unit.Kind != repositoryatlas.UnitRepository {
			continue
		}
		if result != "" {
			return "", fmt.Errorf("navigator product: Atlas requires exactly one repository Unit")
		}
		result = unit.ID
	}
	if result == "" {
		return "", fmt.Errorf("navigator product: Atlas requires exactly one repository Unit")
	}
	return result, nil
}

func startupActionKey(relationID string) string {
	digest := sha256.Sum256([]byte("startup-action\x00" + relationID))
	return "startup-action-" + hex.EncodeToString(digest[:12])
}

func productDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func cloneRecommendationActions(values []RecommendationAction) []RecommendationAction {
	result := make([]RecommendationAction, len(values))
	for index := range values {
		result[index] = cloneRecommendationAction(values[index])
	}
	return result
}

func cloneRecommendationAction(value RecommendationAction) RecommendationAction {
	value.EvidenceIDs = append([]string(nil), value.EvidenceIDs...)
	return value
}
