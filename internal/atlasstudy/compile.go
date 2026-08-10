package atlasstudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

type ResourceLimitError struct {
	Section string
	Limit   int
	Actual  int
}

func (err *ResourceLimitError) Error() string {
	if err == nil {
		return "atlas study: resource limit exceeded"
	}
	return fmt.Sprintf(
		"atlas study: %s requires %d item(s)/byte(s); explicit limit is %d",
		err.Section, err.Actual, err.Limit,
	)
}

// Product is one immutable, request-bound Atlas Study question. The catalog
// never appears on the model wire.
type Product struct {
	input              Input
	wire               []byte
	wireSHA256         string
	catalogSHA256      string
	catalogRef         string
	atlasSHA256        string
	architectureSHA256 string
	catalog            []CatalogObject
	byRef              map[string]CatalogObject
	byCanonical        map[CanonicalRef]CatalogObject
	privateIdentities  map[string]struct{}
	alwaysPrivate      map[string]struct{}
	coverage           CandidateCoverage
	selectedSpanIDs    []string
}

func (product Product) WireJSON() []byte { return append([]byte(nil), product.wire...) }

func (product Product) WireSHA256() string          { return product.wireSHA256 }
func (product Product) CatalogSHA256() string       { return product.catalogSHA256 }
func (product Product) CatalogRef() string          { return product.catalogRef }
func (product Product) AtlasSHA256() string         { return product.atlasSHA256 }
func (product Product) ArchitectureSHA256() string  { return product.architectureSHA256 }
func (product Product) Language() Language          { return product.input.Language }
func (product Product) Coverage() CandidateCoverage { return cloneCandidateCoverage(product.coverage) }

func (product Product) Catalog() []CatalogObject {
	return cloneCatalog(product.catalog)
}

type wireProjection struct {
	Version             int                      `json:"version"`
	Language            Language                 `json:"language"`
	AllowedPaths        []string                 `json:"allowed_paths"`
	Architecture        wireArchitecture         `json:"architecture"`
	BriefSupportChoices []wireBriefSupportChoice `json:"brief_support_choices"`
	Units               []wireUnit               `json:"units"`
	Subsystems          []wireSubsystem          `json:"subsystems"`
	Components          []wireComponent          `json:"components"`
	Surfaces            []wireSurface            `json:"surfaces,omitempty"`
	Targets             []wireTarget             `json:"reading_targets"`
	Evidence            []wireEvidence           `json:"evidence,omitempty"`
	Documents           []wireDocument           `json:"documented_claims,omitempty"`
	RouteSupports       []wireRouteSupport       `json:"route_supports"`
	RouteSpans          []wireRouteSpan          `json:"route_spans"`
}

// wireBriefSupportChoice is the complete model-visible allowlist for Brief
// support_refs. Units remain useful route-principal context, but never appear
// here and therefore cannot be selected as Brief evidence.
type wireBriefSupportChoice struct {
	Ref  string  `json:"ref"`
	Kind RefKind `json:"kind"`
}

type wireArchitecture struct {
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
}

type wireUnit struct {
	Ref       string                    `json:"ref"`
	Kind      repositoryatlas.UnitKind  `json:"kind"`
	Label     string                    `json:"label"`
	Authority repositoryatlas.Authority `json:"authority"`
}

type wireSubsystem struct {
	Ref           string                    `json:"ref"`
	Label         string                    `json:"label"`
	Description   string                    `json:"description,omitempty"`
	Authority     repositoryatlas.Authority `json:"authority"`
	ComponentRefs []string                  `json:"component_refs"`
}

type wireComponent struct {
	Ref               string                    `json:"ref"`
	SubsystemRef      string                    `json:"subsystem_ref"`
	Label             string                    `json:"label"`
	Description       string                    `json:"description,omitempty"`
	Authority         repositoryatlas.Authority `json:"authority"`
	ReadingTargetRefs []string                  `json:"reading_target_refs"`
}

type wireSurface struct {
	Ref               string                    `json:"ref"`
	UnitRef           string                    `json:"unit_ref"`
	Label             string                    `json:"label"`
	Kind              string                    `json:"kind"`
	Authority         repositoryatlas.Authority `json:"authority"`
	ReadingTargetRefs []string                  `json:"reading_target_refs"`
}

type wireTarget struct {
	Ref                  string                    `json:"ref"`
	OwnerRef             string                    `json:"owner_ref,omitempty"`
	RelatedComponentRefs []string                  `json:"related_component_refs,omitempty"`
	PrincipalRefs        []string                  `json:"principal_refs"`
	Kind                 ReadingTargetKind         `json:"kind"`
	Label                string                    `json:"label"`
	Fact                 string                    `json:"fact"`
	Path                 string                    `json:"path"`
	Line                 int                       `json:"line"`
	Symbol               string                    `json:"symbol,omitempty"`
	Authority            repositoryatlas.Authority `json:"authority"`
}

type wireEvidence struct {
	Ref         string                    `json:"ref"`
	SubjectRefs []string                  `json:"subject_refs"`
	Fact        string                    `json:"fact"`
	Authority   repositoryatlas.Authority `json:"authority"`
}

type wireDocument struct {
	Ref       string                    `json:"ref"`
	Label     string                    `json:"label"`
	Claim     string                    `json:"claim"`
	Authority repositoryatlas.Authority `json:"authority"`
}

type wireRouteSupport struct {
	Ref       string                    `json:"ref"`
	Role      SupportRole               `json:"role"`
	TargetRef string                    `json:"target_ref"`
	Authority repositoryatlas.Authority `json:"authority"`
}

type wireRouteSpan struct {
	Ref                 string        `json:"span_ref"`
	Kind                RouteSpanKind `json:"kind"`
	Question            string        `json:"question"`
	TargetJob           TargetJob     `json:"target_job"`
	LearningStage       LearningStage `json:"learning_stage"`
	RequiredSupportRefs []string      `json:"required_support_refs"`
	AllowedTargetRefs   []string      `json:"allowed_target_refs"`
}

type catalogMaterial struct {
	Version            int                      `json:"version"`
	AtlasSHA256        string                   `json:"atlas_sha256"`
	ArchitectureSHA256 string                   `json:"architecture_sha256"`
	Language           Language                 `json:"language"`
	Limits             Limits                   `json:"limits"`
	ProjectionSHA256   string                   `json:"projection_sha256"`
	Coverage           CandidateCoverage        `json:"coverage"`
	AnalysisTargetRoot *AnalysisTargetRootScope `json:"analysis_target_root,omitempty"`
	Objects            []CatalogObject          `json:"objects"`
}

func Compile(input Input) (Product, error) {
	canonical, atlasSHA, architectureSHA, err := canonicalInput(input)
	if err != nil {
		return Product{}, err
	}
	if err := validateLimits(canonical.Limits); err != nil {
		return Product{}, err
	}
	canonical, coverage, err := selectStudyCandidates(canonical)
	if err != nil {
		return Product{}, err
	}
	counts := []struct {
		section string
		limit   int
		actual  int
	}{
		{"units", canonical.Limits.MaxUnits, len(canonical.Atlas.Units)},
		{"subsystems", canonical.Limits.MaxSubsystems, len(canonical.Architecture.Subsystems)},
		{"components", canonical.Limits.MaxComponents, len(canonical.Architecture.Components)},
		{"surfaces", canonical.Limits.MaxSurfaces, len(canonical.Surfaces)},
		{"reading_targets", canonical.Limits.MaxReadingTargets, len(canonical.ReadingTargets)},
		{"evidence", canonical.Limits.MaxEvidence, len(canonical.Evidence)},
		{"documents", canonical.Limits.MaxDocuments, len(canonical.Documents)},
	}
	for _, count := range counts {
		if err := enforceLimit(count.section, count.limit, count.actual); err != nil {
			return Product{}, err
		}
	}

	objects, refs, identities, err := compileCatalog(canonical)
	if err != nil {
		return Product{}, err
	}
	projection, err := buildWire(canonical, refs, allPrivateIdentities(canonical, false))
	if err != nil {
		return Product{}, err
	}
	projectionJSON, err := json.Marshal(projection)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study: encode projection: %w", err)
	}
	material := catalogMaterial{
		Version: Version, AtlasSHA256: atlasSHA, ArchitectureSHA256: architectureSHA,
		Language: canonical.Language, Limits: canonical.Limits,
		ProjectionSHA256: digest(projectionJSON), Coverage: coverage,
		AnalysisTargetRoot: cloneAnalysisTargetRootScope(canonical.AnalysisTargetRoot),
		Objects:            objects,
	}
	materialJSON, err := json.Marshal(material)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study: encode private catalog: %w", err)
	}
	catalogSHA := digest(materialJSON)
	catalogRef := fmt.Sprintf("atlas-study-v%d-%s", Version, catalogSHA)
	wireJSON := projectionJSON
	if err := enforceLimit("wire_bytes", canonical.Limits.MaxWireBytes, len(wireJSON)); err != nil {
		return Product{}, err
	}
	byRef := make(map[string]CatalogObject, len(objects))
	byCanonical := make(map[CanonicalRef]CatalogObject, len(objects))
	alwaysPrivate := allPrivateIdentities(canonical, false)
	for _, object := range objects {
		byRef[object.Ref] = object
		byCanonical[CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}] = object
		// Request-local refs are valid only in typed identity fields. They are
		// never product prose and must not leak into Brief or Study copy.
		identities[object.Ref] = struct{}{}
		alwaysPrivate[object.Ref] = struct{}{}
	}
	return Product{
		input: canonical, wire: wireJSON, wireSHA256: digest(wireJSON),
		catalogSHA256: catalogSHA, catalogRef: catalogRef,
		atlasSHA256: atlasSHA, architectureSHA256: architectureSHA,
		catalog: objects, byRef: byRef, byCanonical: byCanonical,
		privateIdentities: identities,
		alwaysPrivate:     alwaysPrivate, coverage: coverage,
		selectedSpanIDs: selectedSpanIDs(canonical.RouteSpans),
	}, nil
}

func canonicalInput(input Input) (Input, string, string, error) {
	atlas, err := repositoryatlas.Canonical(input.Atlas)
	if err != nil {
		return Input{}, "", "", fmt.Errorf("atlas study: Atlas: %w", err)
	}
	atlasJSON, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		return Input{}, "", "", fmt.Errorf("atlas study: encode Atlas identity: %w", err)
	}
	input.Atlas = atlas
	input.Architecture.Subsystems = cloneSubsystems(input.Architecture.Subsystems)
	input.Architecture.Components = cloneComponents(input.Architecture.Components)
	input.Surfaces = cloneSurfaces(input.Surfaces)
	input.ReadingTargets = append([]ReadingTarget(nil), input.ReadingTargets...)
	input.ReadingSupports = append([]ReadingSupport(nil), input.ReadingSupports...)
	input.ProducerRelations = append([]RouteProducerRelation(nil), input.ProducerRelations...)
	input.RouteSpans = cloneRouteSpans(input.RouteSpans)
	for index := range input.ReadingTargets {
		input.ReadingTargets[index].RelatedComponentIDs = append(
			[]string(nil), input.ReadingTargets[index].RelatedComponentIDs...,
		)
		input.ReadingTargets[index].PrincipalRefs = append(
			[]CanonicalRef(nil), input.ReadingTargets[index].PrincipalRefs...,
		)
	}
	input.Evidence = cloneEvidenceFacts(input.Evidence)
	input.Documents = append([]DocumentClaim(nil), input.Documents...)
	input.AnalysisTargetRoot = cloneAnalysisTargetRootScope(input.AnalysisTargetRoot)
	sort.Slice(input.Architecture.Subsystems, func(i, j int) bool {
		return input.Architecture.Subsystems[i].ID < input.Architecture.Subsystems[j].ID
	})
	sort.Slice(input.Architecture.Components, func(i, j int) bool {
		return input.Architecture.Components[i].ID < input.Architecture.Components[j].ID
	})
	sort.Slice(input.Surfaces, func(i, j int) bool { return input.Surfaces[i].ID < input.Surfaces[j].ID })
	sort.Slice(input.ReadingTargets, func(i, j int) bool {
		return input.ReadingTargets[i].ID < input.ReadingTargets[j].ID
	})
	sort.Slice(input.ReadingSupports, func(i, j int) bool { return input.ReadingSupports[i].ID < input.ReadingSupports[j].ID })
	sort.Slice(input.ProducerRelations, func(i, j int) bool { return input.ProducerRelations[i].ID < input.ProducerRelations[j].ID })
	sort.Slice(input.RouteSpans, func(i, j int) bool { return input.RouteSpans[i].ID < input.RouteSpans[j].ID })
	sort.Slice(input.Evidence, func(i, j int) bool { return input.Evidence[i].ID < input.Evidence[j].ID })
	sort.Slice(input.Documents, func(i, j int) bool { return input.Documents[i].ID < input.Documents[j].ID })
	for index := range input.Architecture.Subsystems {
		sort.Strings(input.Architecture.Subsystems[index].ComponentIDs)
	}
	for index := range input.Architecture.Components {
		sort.Strings(input.Architecture.Components[index].ReadingTargetIDs)
	}
	for index := range input.Surfaces {
		sort.Strings(input.Surfaces[index].ReadingTargetIDs)
	}
	for index := range input.ReadingTargets {
		sort.Strings(input.ReadingTargets[index].RelatedComponentIDs)
		sort.Slice(input.ReadingTargets[index].PrincipalRefs, func(i, j int) bool {
			return canonicalRefLess(
				input.ReadingTargets[index].PrincipalRefs[i],
				input.ReadingTargets[index].PrincipalRefs[j],
			)
		})
	}
	for index := range input.Evidence {
		sort.Slice(input.Evidence[index].SubjectRefs, func(i, j int) bool {
			return canonicalRefLess(input.Evidence[index].SubjectRefs[i], input.Evidence[index].SubjectRefs[j])
		})
	}
	for index := range input.RouteSpans {
		sort.Strings(input.RouteSpans[index].RequiredSupportIDs)
		sort.Strings(input.RouteSpans[index].AllowedTargetIDs)
		sort.Slice(input.RouteSpans[index].Joins, func(i, j int) bool {
			return input.RouteSpans[index].Joins[i].RelationID < input.RouteSpans[index].Joins[j].RelationID
		})
	}
	architectureJSON, err := json.Marshal(input.Architecture)
	if err != nil {
		return Input{}, "", "", fmt.Errorf("atlas study: encode Architecture identity: %w", err)
	}
	return input, digest(atlasJSON), digest(architectureJSON), nil
}

func compileCatalog(input Input) (
	[]CatalogObject,
	map[CanonicalRef]string,
	map[string]struct{},
	error,
) {
	if !input.Language.Valid() {
		return nil, nil, nil, fmt.Errorf("atlas study: unsupported language %q", input.Language)
	}
	// Decision 235 (v11) 1D sqlc: model Architecture is enrichment, not a
	// prerequisite. A local-source empty Architecture block (unavailable
	// synthesis — no canonical candidates) is a valid input; the typed
	// reading supports/spans remain mandatory.
	architectureBlocked := input.Architecture.Source == "" ||
		len(input.Architecture.Subsystems) > 0 && len(input.Architecture.Components) == 0 ||
		len(input.Architecture.Components) > 0 && len(input.Architecture.Subsystems) == 0
	if architectureBlocked ||
		len(input.ReadingTargets) == 0 || len(input.ReadingSupports) == 0 || len(input.RouteSpans) == 0 {
		return nil, nil, nil, fmt.Errorf("atlas study: canonical Architecture and typed reading supports/spans are required")
	}
	if err := validateVisibleText(
		input.Architecture.Source, input.Limits.MaxTextBytes, true, nil,
	); err != nil {
		return nil, nil, nil, fmt.Errorf("atlas study: Architecture source: %w", err)
	}
	identities := allPrivateIdentities(input, true)
	// Question text may legitimately reference the exact advertised symbol of
	// a reading target through its derived package.Symbol segment. Those
	// model-visible symbols are therefore not private for backend-owned
	// question prose (a derived reference can collide with another target's
	// shorter bare symbol on real repositories). Response prose still
	// validates against the full private set, where qualified symbols stay
	// private unless the target is explicitly in scope.
	questionIdentities := make(map[string]struct{}, len(identities))
	for identity := range identities {
		questionIdentities[identity] = struct{}{}
	}
	for _, target := range input.ReadingTargets {
		if symbol := modelVisibleTargetSymbol(target.Symbol); symbol != "" {
			delete(questionIdentities, symbol)
		}
	}
	objects := make([]CatalogObject, 0,
		len(input.Atlas.Units)+len(input.Architecture.Subsystems)+
			len(input.Architecture.Components)+len(input.Surfaces)+
			len(input.ReadingTargets)+len(input.Evidence)+len(input.Documents)+
			len(input.ReadingSupports)+len(input.ProducerRelations)+len(input.RouteSpans))
	refs := make(map[CanonicalRef]string, cap(objects))
	seen := make(map[CanonicalRef]struct{}, cap(objects))
	seenShortRefs := make(map[string]struct{}, cap(objects))
	nextRefOrdinal := make(map[RefKind]int)
	add := func(kind RefKind, id, label, fact string, authority repositoryatlas.Authority,
		owner *CanonicalRef, relatedComponents, principals []CanonicalRef,
		location *evidence.Location, symbol string,
	) error {
		key := CanonicalRef{Kind: kind, ID: id}
		if id == "" || !kind.Valid() || !authority.Valid() {
			return fmt.Errorf("atlas study: invalid canonical %s object", kind)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("atlas study: duplicate canonical %s object", kind)
		}
		seen[key] = struct{}{}
		var ref string
		for {
			nextRefOrdinal[kind]++
			ref = refPrefix(kind) + fmt.Sprint(nextRefOrdinal[kind])
			_, privateCollision := identities[ref]
			_, shortCollision := seenShortRefs[ref]
			if !privateCollision && !shortCollision {
				break
			}
		}
		seenShortRefs[ref] = struct{}{}
		object := CatalogObject{
			Ref: ref, Kind: kind, CanonicalID: id, Label: label, Fact: fact,
			Authority: authority, Symbol: symbol,
		}
		if owner != nil {
			copyOwner := *owner
			object.Owner = &copyOwner
		}
		object.RelatedComponentRefs = append([]CanonicalRef(nil), relatedComponents...)
		object.PrincipalRefs = append([]CanonicalRef(nil), principals...)
		if location != nil {
			copyLocation := *location
			object.Location = &copyLocation
		}
		objects = append(objects, object)
		refs[key] = ref
		return nil
	}

	units := make(map[string]repositoryatlas.Unit, len(input.Atlas.Units))
	for _, unit := range input.Atlas.Units {
		units[unit.ID] = unit
		if err := add(RefUnit, unit.ID, unit.Name, "", repositoryatlas.AuthorityObserved, nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
		objects[len(objects)-1].UnitKind = unit.Kind
		objects[len(objects)-1].UnitParentID = unit.ParentID
	}
	subsystems := make(map[string]Subsystem, len(input.Architecture.Subsystems))
	for _, subsystem := range input.Architecture.Subsystems {
		if _, duplicate := subsystems[subsystem.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate subsystem")
		}
		subsystems[subsystem.ID] = subsystem
		if err := add(RefSubsystem, subsystem.ID, subsystem.Name, subsystem.Description,
			subsystem.Authority, nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
	}
	components := make(map[string]Component, len(input.Architecture.Components))
	for _, component := range input.Architecture.Components {
		if _, ok := subsystems[component.SubsystemID]; !ok {
			return nil, nil, nil, fmt.Errorf("atlas study: component references unknown subsystem")
		}
		if _, duplicate := components[component.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate component")
		}
		components[component.ID] = component
		if err := add(RefComponent, component.ID, component.Name, component.Description,
			component.Authority, nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
	}
	componentMembership := make(map[string]int, len(components))
	for _, subsystem := range input.Architecture.Subsystems {
		if len(subsystem.ComponentIDs) == 0 || !uniqueSorted(subsystem.ComponentIDs) {
			return nil, nil, nil, fmt.Errorf("atlas study: subsystem requires unique ordered components")
		}
		for _, componentID := range subsystem.ComponentIDs {
			component, ok := components[componentID]
			if !ok || component.SubsystemID != subsystem.ID {
				return nil, nil, nil, fmt.Errorf("atlas study: subsystem component membership is inconsistent")
			}
			componentMembership[componentID]++
		}
	}
	for componentID := range components {
		if componentMembership[componentID] != 1 {
			return nil, nil, nil, fmt.Errorf("atlas study: every component requires one exact subsystem membership")
		}
	}

	atlasEntities := make(map[string]repositoryatlas.Entity, len(input.Atlas.Entities))
	for _, entity := range input.Atlas.Entities {
		atlasEntities[entity.ID] = entity
	}
	surfaces := make(map[string]Surface, len(input.Surfaces))
	for _, surface := range input.Surfaces {
		entity, ok := atlasEntities[surface.ID]
		if !ok || entity.Kind != repositoryatlas.EntitySurface || entity.UnitID != surface.UnitID {
			return nil, nil, nil, fmt.Errorf("atlas study: Surface does not match the exact Atlas")
		}
		if _, ok := units[surface.UnitID]; !ok {
			return nil, nil, nil, fmt.Errorf("atlas study: Surface references unknown Unit")
		}
		if _, duplicate := surfaces[surface.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate Surface")
		}
		surfaces[surface.ID] = surface
		if err := add(RefSurface, surface.ID, surface.Name, surface.Kind,
			surface.Authority, nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
	}

	targets := make(map[string]ReadingTarget, len(input.ReadingTargets))
	locators := make(map[readingLocatorIdentity]string, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		if err := validateReadingTargetSymbol(target.Symbol, input.Limits.MaxTextBytes); err != nil {
			return nil, nil, nil, err
		}
		if !target.Kind.Valid() || !repositoryLocation(target.Location) ||
			len(target.PrincipalRefs) == 0 || !uniqueCanonicalRefs(target.PrincipalRefs) ||
			!uniqueSorted(target.RelatedComponentIDs) {
			return nil, nil, nil, fmt.Errorf("atlas study: invalid exact reading target")
		}
		principalSet := make(map[CanonicalRef]struct{}, len(target.PrincipalRefs))
		for _, principal := range target.PrincipalRefs {
			switch principal.Kind {
			case RefUnit:
				unit, ok := units[principal.ID]
				if !ok || unit.Kind != repositoryatlas.UnitPackage {
					return nil, nil, nil, fmt.Errorf("atlas study: reading target has unknown package Unit principal")
				}
			case RefComponent:
				if _, ok := components[principal.ID]; !ok {
					return nil, nil, nil, fmt.Errorf("atlas study: reading target has unknown component principal")
				}
			case RefSurface:
				if _, ok := surfaces[principal.ID]; !ok {
					return nil, nil, nil, fmt.Errorf("atlas study: reading target has unknown Surface principal")
				}
			default:
				return nil, nil, nil, fmt.Errorf("atlas study: reading target has wrong-kind principal")
			}
			principalSet[principal] = struct{}{}
		}
		related := make([]CanonicalRef, 0, len(target.RelatedComponentIDs))
		for _, componentID := range target.RelatedComponentIDs {
			ref := CanonicalRef{Kind: RefComponent, ID: componentID}
			if _, ok := components[componentID]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: reading target has unknown related component")
			}
			if _, ok := principalSet[ref]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: related component is not a target principal")
			}
			related = append(related, ref)
		}
		var owner *CanonicalRef
		if target.Owner != (CanonicalRef{}) {
			if target.Owner.Kind != RefComponent {
				return nil, nil, nil, fmt.Errorf("atlas study: reading target owner must be an exact component proof")
			}
			if _, ok := principalSet[target.Owner]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: reading target owner is not a target principal")
			}
			if index, found := slices.BinarySearch(target.RelatedComponentIDs, target.Owner.ID); !found || index < 0 {
				return nil, nil, nil, fmt.Errorf("atlas study: reading target owner is not a related component")
			}
			owner = &target.Owner
		}
		if _, duplicate := targets[target.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate reading target")
		}
		locator := readingLocatorKey(target)
		if existing, duplicate := locators[locator]; duplicate {
			return nil, nil, nil, fmt.Errorf(
				"atlas study: reading targets %q and %q duplicate one exact locator",
				existing, target.ID,
			)
		}
		locators[locator] = target.ID
		targets[target.ID] = target
		if err := add(RefReadingTarget, target.ID, target.Label, target.Fact,
			target.Authority, owner, related, target.PrincipalRefs,
			&target.Location, target.Symbol); err != nil {
			return nil, nil, nil, err
		}
		objects[len(objects)-1].ReadingTargetKind = target.Kind
	}
	if err := validateTargetAssociations(input.Architecture.Components, input.Surfaces, targets); err != nil {
		return nil, nil, nil, err
	}

	supports := make(map[string]ReadingSupport, len(input.ReadingSupports))
	for _, support := range input.ReadingSupports {
		if support.ID == "" || support.TargetID == "" || support.PackageBucket == "" ||
			!support.Role.Valid() || !validSupportAuthority(support.Role, support.Authority) {
			return nil, nil, nil, fmt.Errorf("atlas study: invalid route support")
		}
		if _, ok := targets[support.TargetID]; !ok {
			return nil, nil, nil, fmt.Errorf("atlas study: route support references unknown target")
		}
		if _, duplicate := supports[support.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate route support")
		}
		supports[support.ID] = support
		targetRef := CanonicalRef{Kind: RefReadingTarget, ID: support.TargetID}
		if err := add(RefRouteSupport, support.ID, "", "", support.Authority,
			nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
		objects[len(objects)-1].SupportRole = support.Role
		objects[len(objects)-1].SupportTarget = &targetRef
		objects[len(objects)-1].PackageBucket = support.PackageBucket
	}
	if err := validateAnalysisTargetRootContract(targets, supports, units, input.AnalysisTargetRoot); err != nil {
		return nil, nil, nil, err
	}
	relations := make(map[string]RouteProducerRelation, len(input.ProducerRelations))
	producerRelationIDs := make(map[string]struct{}, len(input.ProducerRelations))
	for _, relation := range input.ProducerRelations {
		if err := validateRouteProducerRelation(relation, supports, targets); err != nil {
			return nil, nil, nil, err
		}
		if _, duplicate := relations[relation.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate route producer relation")
		}
		if _, duplicate := producerRelationIDs[relation.ProducerID]; duplicate {
			return nil, nil, nil, fmt.Errorf("atlas study: duplicate route producer identity")
		}
		producerRelationIDs[relation.ProducerID] = struct{}{}
		relations[relation.ID] = relation
		if err := add(RefRouteRelation, relation.ID, "", "", repositoryatlas.AuthorityResolved,
			nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
		object := &objects[len(objects)-1]
		object.RelationKind, object.ProducerID = relation.Kind, relation.ProducerID
		fromSupport := CanonicalRef{Kind: RefRouteSupport, ID: relation.FromSupportID}
		toSupport := CanonicalRef{Kind: RefRouteSupport, ID: relation.ToSupportID}
		fromTarget := CanonicalRef{Kind: RefReadingTarget, ID: relation.FromTargetID}
		toTarget := CanonicalRef{Kind: RefReadingTarget, ID: relation.ToTargetID}
		object.FromSupport, object.ToSupport = &fromSupport, &toSupport
		object.FromTarget, object.ToTarget = &fromTarget, &toTarget
		object.SavedFlowID, object.FromStepID, object.ToStepID = relation.SavedFlowID, relation.FromStepID, relation.ToStepID
		object.FromStepOrdinal, object.ToStepOrdinal = relation.FromStepOrdinal, relation.ToStepOrdinal
	}
	for _, span := range input.RouteSpans {
		if span.ID == "" || !span.Kind.Valid() || !span.TargetJob.Valid() ||
			!span.LearningStage.Valid() || !uniqueSorted(span.RequiredSupportIDs) ||
			!uniqueSorted(span.AllowedTargetIDs) || len(span.RequiredSupportIDs) == 0 ||
			len(span.AllowedTargetIDs) == 0 {
			return nil, nil, nil, fmt.Errorf("atlas study: invalid route span")
		}
		if err := validateLocalizedSpanQuestions(span, input.Limits.MaxTextBytes, questionIdentities); err != nil {
			return nil, nil, nil, err
		}
		required := make([]CanonicalRef, 0, len(span.RequiredSupportIDs))
		allowed := make([]CanonicalRef, 0, len(span.AllowedTargetIDs))
		allowedSet := make(map[string]struct{}, len(span.AllowedTargetIDs))
		for _, targetID := range span.AllowedTargetIDs {
			if _, ok := targets[targetID]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: route span allows unknown target")
			}
			allowedSet[targetID] = struct{}{}
			allowed = append(allowed, CanonicalRef{Kind: RefReadingTarget, ID: targetID})
		}
		coveredTargets := make(map[string]struct{})
		for _, supportID := range span.RequiredSupportIDs {
			support, ok := supports[supportID]
			if !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: route span requires unknown support")
			}
			if _, ok := allowedSet[support.TargetID]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: route span support target is not allowed")
			}
			coveredTargets[support.TargetID] = struct{}{}
			required = append(required, CanonicalRef{Kind: RefRouteSupport, ID: supportID})
			if support.Role == SupportAnalysisTargetRoot && span.Kind != RouteSpanFocused {
				return nil, nil, nil, fmt.Errorf("atlas study: public API root support requires a focused span")
			}
		}
		if span.Kind == RouteSpanSystemPath && len(coveredTargets) < 2 {
			return nil, nil, nil, fmt.Errorf("atlas study: system-path span requires two distinct exact locators")
		}
		canonicalJoins, err := validateRouteSpanJoins(span, supports, relations, allowedSet)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := add(RefRouteSpan, span.ID, "", "", repositoryatlas.AuthorityResolved,
			nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
		object := &objects[len(objects)-1]
		object.SpanKind = span.Kind
		object.Question = span.question(input.Language)
		object.TargetJob = span.TargetJob
		object.LearningStage = span.LearningStage
		object.RequiredSupportRefs = required
		object.AllowedTargetRefs = allowed
		object.SpanJoins = canonicalJoins
	}

	atlasEvidence := make(map[string]repositoryatlas.Evidence, len(input.Atlas.Evidence))
	for _, item := range input.Atlas.Evidence {
		atlasEvidence[item.ID] = item
	}
	for _, item := range input.Evidence {
		exact, ok := atlasEvidence[item.ID]
		if !ok {
			return nil, nil, nil, fmt.Errorf("atlas study: evidence does not match the exact Atlas")
		}
		if len(item.SubjectRefs) == 0 || !uniqueCanonicalRefs(item.SubjectRefs) {
			return nil, nil, nil, fmt.Errorf("atlas study: evidence requires unique ordered subjects")
		}
		for _, subject := range item.SubjectRefs {
			if _, ok := refs[subject]; !ok {
				return nil, nil, nil, fmt.Errorf("atlas study: evidence references unknown subject")
			}
		}
		location := exact.Location
		if err := add(RefEvidence, item.ID, "", item.Fact, item.Authority,
			nil, nil, nil, &location, exact.Symbol); err != nil {
			return nil, nil, nil, err
		}
	}
	for _, document := range input.Documents {
		if err := add(RefDocument, document.ID, document.Label, document.Claim,
			document.Authority, nil, nil, nil, nil, ""); err != nil {
			return nil, nil, nil, err
		}
	}

	for _, object := range objects {
		labelRequired := object.Kind != RefEvidence && object.Kind != RefRouteSupport &&
			object.Kind != RefRouteRelation && object.Kind != RefRouteSpan
		factRequired := object.Kind == RefSurface || object.Kind == RefReadingTarget ||
			object.Kind == RefEvidence || object.Kind == RefDocument
		labelIdentities := identities
		if object.Kind == RefReadingTarget && modelVisibleTargetSymbol(object.Symbol) != "" {
			labelIdentities = make(map[string]struct{}, len(identities))
			for identity := range identities {
				if identity != object.Symbol {
					labelIdentities[identity] = struct{}{}
				}
			}
		}
		if err := validateVisibleText(object.Label, input.Limits.MaxTextBytes, labelRequired, labelIdentities); err != nil {
			return nil, nil, nil, fmt.Errorf("atlas study: %s label: %w", object.Kind, err)
		}
		if err := validateVisibleText(object.Fact, input.Limits.MaxTextBytes, factRequired, identities); err != nil {
			return nil, nil, nil, fmt.Errorf("atlas study: %s fact: %w", object.Kind, err)
		}
	}
	if err := validateVisibleText(input.Architecture.Title, input.Limits.MaxTextBytes, false, identities); err != nil {
		return nil, nil, nil, fmt.Errorf("atlas study: Architecture title: %w", err)
	}
	if err := validateVisibleText(input.Architecture.Subtitle, input.Limits.MaxTextBytes, false, identities); err != nil {
		return nil, nil, nil, fmt.Errorf("atlas study: Architecture subtitle: %w", err)
	}
	return objects, refs, identities, nil
}

func buildWire(
	input Input,
	refs map[CanonicalRef]string,
	identities map[string]struct{},
) (wireProjection, error) {
	wire := wireProjection{
		Version: Version, Language: input.Language,
		Architecture: wireArchitecture{Title: input.Architecture.Title, Subtitle: input.Architecture.Subtitle},
	}
	allowedPaths := make(map[string]struct{}, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		allowedPaths[target.Location.Path] = struct{}{}
	}
	for sourcePath := range allowedPaths {
		wire.AllowedPaths = append(wire.AllowedPaths, sourcePath)
	}
	sort.Strings(wire.AllowedPaths)
	addBriefSupport := func(kind RefKind, ref string) {
		if briefSupportKind(kind) {
			wire.BriefSupportChoices = append(wire.BriefSupportChoices, wireBriefSupportChoice{
				Ref: ref, Kind: kind,
			})
		}
	}
	for _, unit := range input.Atlas.Units {
		wire.Units = append(wire.Units, wireUnit{
			Ref: refs[CanonicalRef{Kind: RefUnit, ID: unit.ID}], Kind: unit.Kind,
			Label: unit.Name, Authority: repositoryatlas.AuthorityObserved,
		})
	}
	for _, subsystem := range input.Architecture.Subsystems {
		ref := refs[CanonicalRef{Kind: RefSubsystem, ID: subsystem.ID}]
		item := wireSubsystem{
			Ref:   ref,
			Label: subsystem.Name, Description: subsystem.Description, Authority: subsystem.Authority,
		}
		for _, componentID := range subsystem.ComponentIDs {
			item.ComponentRefs = append(item.ComponentRefs, refs[CanonicalRef{Kind: RefComponent, ID: componentID}])
		}
		wire.Subsystems = append(wire.Subsystems, item)
		addBriefSupport(RefSubsystem, ref)
	}
	for _, component := range input.Architecture.Components {
		ref := refs[CanonicalRef{Kind: RefComponent, ID: component.ID}]
		item := wireComponent{
			Ref:          ref,
			SubsystemRef: refs[CanonicalRef{Kind: RefSubsystem, ID: component.SubsystemID}],
			Label:        component.Name, Description: component.Description, Authority: component.Authority,
		}
		for _, targetID := range component.ReadingTargetIDs {
			item.ReadingTargetRefs = append(item.ReadingTargetRefs,
				refs[CanonicalRef{Kind: RefReadingTarget, ID: targetID}])
		}
		wire.Components = append(wire.Components, item)
		addBriefSupport(RefComponent, ref)
	}
	for _, surface := range input.Surfaces {
		ref := refs[CanonicalRef{Kind: RefSurface, ID: surface.ID}]
		item := wireSurface{
			Ref:     ref,
			UnitRef: refs[CanonicalRef{Kind: RefUnit, ID: surface.UnitID}],
			Label:   surface.Name, Kind: surface.Kind, Authority: surface.Authority,
		}
		for _, targetID := range surface.ReadingTargetIDs {
			item.ReadingTargetRefs = append(item.ReadingTargetRefs,
				refs[CanonicalRef{Kind: RefReadingTarget, ID: targetID}])
		}
		wire.Surfaces = append(wire.Surfaces, item)
		addBriefSupport(RefSurface, ref)
	}
	for _, target := range input.ReadingTargets {
		ref := refs[CanonicalRef{Kind: RefReadingTarget, ID: target.ID}]
		item := wireTarget{
			Ref: ref, Kind: target.Kind, Label: target.Label,
			Fact: target.Fact, Path: target.Location.Path, Line: target.Location.Line,
			Symbol: modelVisibleTargetSymbol(target.Symbol), Authority: target.Authority,
		}
		if target.Owner != (CanonicalRef{}) {
			item.OwnerRef = refs[target.Owner]
		}
		for _, componentID := range target.RelatedComponentIDs {
			item.RelatedComponentRefs = append(item.RelatedComponentRefs,
				refs[CanonicalRef{Kind: RefComponent, ID: componentID}],
			)
		}
		for _, principal := range target.PrincipalRefs {
			item.PrincipalRefs = append(item.PrincipalRefs, refs[principal])
		}
		wire.Targets = append(wire.Targets, item)
		addBriefSupport(RefReadingTarget, ref)
	}
	for _, fact := range input.Evidence {
		ref := refs[CanonicalRef{Kind: RefEvidence, ID: fact.ID}]
		item := wireEvidence{
			Ref:  ref,
			Fact: fact.Fact, Authority: fact.Authority,
		}
		for _, subject := range fact.SubjectRefs {
			item.SubjectRefs = append(item.SubjectRefs, refs[subject])
		}
		wire.Evidence = append(wire.Evidence, item)
		addBriefSupport(RefEvidence, ref)
	}
	for _, document := range input.Documents {
		ref := refs[CanonicalRef{Kind: RefDocument, ID: document.ID}]
		wire.Documents = append(wire.Documents, wireDocument{
			Ref:   ref,
			Label: document.Label, Claim: document.Claim, Authority: document.Authority,
		})
		addBriefSupport(RefDocument, ref)
	}
	for _, support := range input.ReadingSupports {
		wire.RouteSupports = append(wire.RouteSupports, wireRouteSupport{
			Ref:       refs[CanonicalRef{Kind: RefRouteSupport, ID: support.ID}],
			Role:      support.Role,
			TargetRef: refs[CanonicalRef{Kind: RefReadingTarget, ID: support.TargetID}],
			Authority: support.Authority,
		})
	}
	for _, span := range input.RouteSpans {
		item := wireRouteSpan{
			Ref: refs[CanonicalRef{Kind: RefRouteSpan, ID: span.ID}], Kind: span.Kind,
			Question: span.question(input.Language), TargetJob: span.TargetJob,
			LearningStage: span.LearningStage,
		}
		for _, supportID := range span.RequiredSupportIDs {
			item.RequiredSupportRefs = append(item.RequiredSupportRefs,
				refs[CanonicalRef{Kind: RefRouteSupport, ID: supportID}])
		}
		for _, targetID := range span.AllowedTargetIDs {
			item.AllowedTargetRefs = append(item.AllowedTargetRefs,
				refs[CanonicalRef{Kind: RefReadingTarget, ID: targetID}])
		}
		wire.RouteSpans = append(wire.RouteSpans, item)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return wireProjection{}, err
	}
	for identity := range identities {
		if identity != "" && bytesContainIdentity(encoded, identity) {
			return wireProjection{}, fmt.Errorf("atlas study: provider wire exposes a canonical identity or source locator")
		}
	}
	return wire, nil
}

func allPrivateIdentities(input Input, includeTargetLocators bool) map[string]struct{} {
	result := make(map[string]struct{})
	// Exact bounded reading-target locators are intentionally model-visible
	// task context when includeTargetLocators is false. The full set remains
	// private to response prose validation, so the model may return only the
	// corresponding short request-local ref.
	allowedPaths := make(map[string]struct{}, len(input.ReadingTargets))
	allowedSymbols := make(map[string]struct{}, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		allowedPaths[target.Location.Path] = struct{}{}
		if symbol := modelVisibleTargetSymbol(target.Symbol); symbol != "" {
			allowedSymbols[symbol] = struct{}{}
		}
	}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	if input.AnalysisTargetRoot != nil {
		add(input.AnalysisTargetRoot.AnalysisTarget.Ref)
		add(input.AnalysisTargetRoot.UnitID)
	}
	for _, unit := range input.Atlas.Units {
		add(unit.ID)
	}
	for _, entity := range input.Atlas.Entities {
		add(entity.ID)
	}
	for _, item := range input.Atlas.Evidence {
		add(item.ID)
		if _, visible := allowedPaths[item.Location.Path]; includeTargetLocators || !visible {
			add(item.Location.Path)
		}
	}
	for _, relation := range input.Atlas.Relations {
		add(relation.ID)
	}
	for _, subsystem := range input.Architecture.Subsystems {
		add(subsystem.ID)
	}
	for _, component := range input.Architecture.Components {
		add(component.ID)
	}
	for _, surface := range input.Surfaces {
		add(surface.ID)
	}
	for _, target := range input.ReadingTargets {
		add(target.ID)
		if includeTargetLocators {
			add(target.Location.Path)
		}
		// Qualified symbols outside the exact bounded reading catalog remain
		// private. Exact target symbols are deliberately exposed beside their
		// short ref and cannot be returned as identities.
		if _, visible := allowedSymbols[target.Symbol]; (includeTargetLocators || !visible) &&
			strings.ContainsAny(target.Symbol, "./()") {
			add(target.Symbol)
		}
	}
	for _, item := range input.Evidence {
		add(item.ID)
	}
	for _, document := range input.Documents {
		add(document.ID)
	}
	for _, support := range input.ReadingSupports {
		add(support.ID)
		add(support.PackageBucket)
	}
	for _, relation := range input.ProducerRelations {
		add(relation.ID)
		add(relation.ProducerID)
		add(relation.SavedFlowID)
		add(relation.FromStepID)
		add(relation.ToStepID)
	}
	for _, span := range input.RouteSpans {
		add(span.ID)
	}
	return result
}

func modelVisibleTargetSymbol(symbol string) string {
	// Qualified symbols are unambiguous exact locator context. Bare identifiers
	// such as "Run" are ordinary prose and cannot be safely forbidden in model
	// text without false positives, so the wire omits them.
	if strings.ContainsAny(symbol, "./()") {
		return symbol
	}
	return ""
}

func validateReadingTargetSymbol(symbol string, limit int) error {
	if strings.TrimSpace(symbol) != symbol || !utf8.ValidString(symbol) || len(symbol) > limit {
		return fmt.Errorf("atlas study: reading target symbol must be bounded exact UTF-8 text")
	}
	for _, value := range symbol {
		if unicode.IsControl(value) {
			return fmt.Errorf("atlas study: reading target symbol contains a control character")
		}
	}
	return nil
}

func validateLocalizedSpanQuestions(span RouteSpan, limit int, identities map[string]struct{}) error {
	for language, question := range map[Language]string{
		LanguageEnglish: span.QuestionEnglish,
		LanguageRussian: span.QuestionRussian,
	} {
		if err := validateVisibleText(question, limit, true, identities); err != nil || !naturalQuestion(question) {
			return fmt.Errorf("atlas study: route span has invalid %s question", language)
		}
	}
	return nil
}

func validateRouteProducerRelation(
	relation RouteProducerRelation,
	supports map[string]ReadingSupport,
	targets map[string]ReadingTarget,
) error {
	if relation.ID == "" || relation.ProducerID == "" || !relation.Kind.Valid() ||
		relation.FromSupportID == relation.ToSupportID || relation.FromTargetID == relation.ToTargetID {
		return fmt.Errorf("atlas study: invalid route producer relation")
	}
	from, fromOK := supports[relation.FromSupportID]
	to, toOK := supports[relation.ToSupportID]
	if !fromOK || !toOK || from.TargetID != relation.FromTargetID || to.TargetID != relation.ToTargetID {
		return fmt.Errorf("atlas study: route producer relation endpoints do not resolve exactly")
	}
	if _, ok := targets[relation.FromTargetID]; !ok {
		return fmt.Errorf("atlas study: route producer relation has unknown source target")
	}
	if _, ok := targets[relation.ToTargetID]; !ok {
		return fmt.Errorf("atlas study: route producer relation has unknown target")
	}
	switch relation.Kind {
	case RouteRelationEntryHandoff:
		if from.Role != SupportProcessEntry || to.Role != SupportEntryHandoff || relation.SavedFlowID != "" ||
			relation.FromStepID != "" || relation.ToStepID != "" || relation.FromStepOrdinal != 0 || relation.ToStepOrdinal != 0 {
			return fmt.Errorf("atlas study: entry-handoff relation has invalid exact endpoints")
		}
	case RouteRelationSavedFlowEdge:
		if from.Role != SupportSavedFlow || to.Role != SupportSavedFlow || relation.SavedFlowID == "" ||
			relation.FromStepID == "" || relation.ToStepID == "" || relation.FromStepID == relation.ToStepID ||
			relation.FromStepOrdinal < 0 || relation.ToStepOrdinal != relation.FromStepOrdinal+1 {
			return fmt.Errorf("atlas study: saved-flow relation is not one exact adjacent accepted edge")
		}
	}
	return nil
}

func validateRouteSpanJoins(
	span RouteSpan,
	supports map[string]ReadingSupport,
	relations map[string]RouteProducerRelation,
	allowedTargets map[string]struct{},
) ([]CanonicalSpanJoin, error) {
	if span.Kind == RouteSpanFocused {
		if len(span.Joins) != 0 {
			return nil, fmt.Errorf("atlas study: focused span cannot claim a system join")
		}
		targets := make(map[string]struct{})
		for _, supportID := range span.RequiredSupportIDs {
			targets[supports[supportID].TargetID] = struct{}{}
		}
		if len(targets) != 1 {
			return nil, fmt.Errorf("atlas study: focused span requires one exact locator")
		}
		return nil, nil
	}
	if len(span.Joins) != 1 {
		return nil, fmt.Errorf("atlas study: system-path span requires exactly one producer join")
	}
	if len(span.RequiredSupportIDs) != 2 || len(allowedTargets) != 2 {
		return nil, fmt.Errorf("atlas study: system-path span must equal one directed producer relation")
	}
	required := make(map[string]struct{}, 2)
	for _, id := range span.RequiredSupportIDs {
		required[id] = struct{}{}
	}
	join := span.Joins[0]
	if join.RelationID == "" {
		return nil, fmt.Errorf("atlas study: invalid route span join")
	}
	relation, ok := relations[join.RelationID]
	if !ok {
		return nil, fmt.Errorf("atlas study: route span joins unknown producer relation")
	}
	_, fromRequired := required[relation.FromSupportID]
	_, toRequired := required[relation.ToSupportID]
	_, fromAllowed := allowedTargets[relation.FromTargetID]
	_, toAllowed := allowedTargets[relation.ToTargetID]
	if !fromRequired || !toRequired || !fromAllowed || !toAllowed {
		return nil, fmt.Errorf("atlas study: system-path span does not equal its exact producer relation")
	}
	return []CanonicalSpanJoin{{
		Relation: CanonicalRef{Kind: RefRouteRelation, ID: relation.ID},
	}}, nil
}

func validateTargetAssociations(
	components []Component,
	surfaces []Surface,
	targets map[string]ReadingTarget,
) error {
	claimed := make(map[string]map[CanonicalRef]struct{}, len(targets))
	claim := func(principal CanonicalRef, values []string) error {
		if !uniqueSorted(values) {
			return fmt.Errorf("atlas study: reading target refs must be unique and ordered")
		}
		for _, id := range values {
			target, ok := targets[id]
			if !ok || !containsCanonicalRef(target.PrincipalRefs, principal) {
				return fmt.Errorf("atlas study: reading target principal association is inconsistent")
			}
			if claimed[id] == nil {
				claimed[id] = make(map[CanonicalRef]struct{})
			}
			claimed[id][principal] = struct{}{}
		}
		return nil
	}
	for _, component := range components {
		if err := claim(CanonicalRef{Kind: RefComponent, ID: component.ID}, component.ReadingTargetIDs); err != nil {
			return err
		}
	}
	for _, surface := range surfaces {
		if err := claim(CanonicalRef{Kind: RefSurface, ID: surface.ID}, surface.ReadingTargetIDs); err != nil {
			return err
		}
	}
	for id, target := range targets {
		expected := 0
		for _, principal := range target.PrincipalRefs {
			if principal.Kind != RefUnit {
				expected++
			}
		}
		if len(claimed[id]) != expected {
			return fmt.Errorf("atlas study: every reading target principal requires one exact association")
		}
		for _, principal := range target.PrincipalRefs {
			if principal.Kind == RefUnit {
				continue
			}
			if _, ok := claimed[id][principal]; !ok {
				return fmt.Errorf("atlas study: reading target principal association is incomplete")
			}
		}
	}
	return nil
}

// validateAnalysisTargetRootContract is the only admission path for a Unit
// principal. A public API root is one resolved function or method owned by one
// exact package Unit; it has no conceptual owner or related components. The
// support's private package bucket is the same canonical Unit identity, which
// prevents an arbitrary repository Unit from being substituted after local
// target selection. All pre-D277 Component/Surface principal paths are
// unchanged.
func validateAnalysisTargetRootContract(
	targets map[string]ReadingTarget,
	supports map[string]ReadingSupport,
	units map[string]repositoryatlas.Unit,
	scope *AnalysisTargetRootScope,
) error {
	if err := validateAnalysisTargetRootScope(scope); err != nil {
		return err
	}
	rootSupports := make(map[string]int)
	allSupports := make(map[string][]ReadingSupport)
	for _, support := range supports {
		allSupports[support.TargetID] = append(allSupports[support.TargetID], support)
		if support.Role != SupportAnalysisTargetRoot {
			continue
		}
		target, ok := targets[support.TargetID]
		if !ok || support.Authority != repositoryatlas.AuthorityResolved ||
			target.Authority != repositoryatlas.AuthorityResolved ||
			(target.Kind != ReadingTargetFunction && target.Kind != ReadingTargetMethod) ||
			target.Owner != (CanonicalRef{}) || len(target.RelatedComponentIDs) != 0 ||
			len(target.PrincipalRefs) != 1 || target.PrincipalRefs[0].Kind != RefUnit {
			return fmt.Errorf("atlas study: invalid public API root support")
		}
		unit := units[target.PrincipalRefs[0].ID]
		if scope == nil || unit.Kind != repositoryatlas.UnitPackage ||
			unit.ID != scope.UnitID || unit.Name != scope.AnalysisTarget.PackagePath ||
			unit.ParentID != scope.AnalysisTarget.ModuleID || support.PackageBucket != scope.UnitID {
			return fmt.Errorf("atlas study: public API root does not match its exact package Unit")
		}
		rootSupports[target.ID]++
	}
	for _, target := range targets {
		hasUnit := false
		for _, principal := range target.PrincipalRefs {
			hasUnit = hasUnit || principal.Kind == RefUnit
		}
		if !hasUnit {
			if rootSupports[target.ID] != 0 {
				return fmt.Errorf("atlas study: public API root support lacks a package Unit principal")
			}
			continue
		}
		if rootSupports[target.ID] != 1 || len(allSupports[target.ID]) != 1 {
			return fmt.Errorf("atlas study: package Unit principal is not one exact public API root")
		}
	}
	if scope != nil && len(rootSupports) == 0 {
		return fmt.Errorf("atlas study: selected AnalysisTarget root scope has no public API roots")
	}
	return nil
}

func validateAnalysisTargetRootScope(scope *AnalysisTargetRootScope) error {
	if scope == nil {
		return nil
	}
	if scope.AnalysisTarget.Validate() != nil ||
		scope.AnalysisTarget.Kind != analysistarget.KindLibraryPackage ||
		scope.AnalysisTarget.RootBoundary != analysistarget.RootBoundaryExactPublicAPI ||
		scope.UnitID == "" || strings.TrimSpace(scope.UnitID) != scope.UnitID ||
		len(scope.UnitID) > 4096 {
		return fmt.Errorf("atlas study: invalid selected AnalysisTarget root scope")
	}
	return nil
}

func cloneAnalysisTargetRootScope(scope *AnalysisTargetRootScope) *AnalysisTargetRootScope {
	if scope == nil {
		return nil
	}
	cloned := *scope
	return &cloned
}

func containsCanonicalRef(values []CanonicalRef, want CanonicalRef) bool {
	index, found := slices.BinarySearchFunc(values, want, func(left, right CanonicalRef) int {
		if canonicalRefLess(left, right) {
			return -1
		}
		if canonicalRefLess(right, left) {
			return 1
		}
		return 0
	})
	return found && index >= 0
}

type readingLocatorIdentity struct {
	Kind      ReadingTargetKind
	Path      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Symbol    string
}

func readingLocatorKey(target ReadingTarget) readingLocatorIdentity {
	return readingLocatorIdentity{
		Kind: target.Kind, Path: target.Location.Path,
		Line: target.Location.Line, Column: target.Location.Column,
		EndLine: target.Location.EndLine, EndColumn: target.Location.EndColumn,
		Symbol: target.Symbol,
	}
}

func validateLimits(limits Limits) error {
	values := []struct {
		name  string
		value int
	}{
		{"max_wire_bytes", limits.MaxWireBytes},
		{"max_response_bytes", limits.MaxResponseBytes},
		{"max_text_bytes", limits.MaxTextBytes},
		{"max_units", limits.MaxUnits},
		{"max_subsystems", limits.MaxSubsystems},
		{"max_components", limits.MaxComponents},
		{"max_surfaces", limits.MaxSurfaces},
		{"max_reading_targets", limits.MaxReadingTargets},
		{"max_advertised_spans", limits.MaxAdvertisedSpans},
		{"max_evidence", limits.MaxEvidence},
		{"max_documents", limits.MaxDocuments},
	}
	for _, item := range values {
		if item.value <= 0 {
			return fmt.Errorf("atlas study: %s must be explicitly positive", item.name)
		}
	}
	return nil
}

func enforceLimit(section string, limit, actual int) error {
	if actual <= limit {
		return nil
	}
	return &ResourceLimitError{Section: section, Limit: limit, Actual: actual}
}

func validateVisibleText(
	value string,
	limit int,
	required bool,
	identities map[string]struct{},
) error {
	if strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		(required && value == "") || len(value) > limit || strings.ContainsAny(value, "\x00\r") {
		return fmt.Errorf("must be bounded exact UTF-8 text")
	}
	for identity := range identities {
		if identity == "" {
			continue
		}
		if containsExactIdentity(value, identity) {
			return fmt.Errorf("contains a canonical identity or source locator")
		}
	}
	return nil
}

func containsExactIdentity(value, identity string) bool {
	valueRunes := []rune(value)
	identityRunes := []rune(identity)
	if len(identityRunes) == 0 || len(identityRunes) > len(valueRunes) {
		return false
	}
	identifierRune := func(value rune) bool {
		return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
	}
	for start := 0; start+len(identityRunes) <= len(valueRunes); start++ {
		if !slices.Equal(valueRunes[start:start+len(identityRunes)], identityRunes) {
			continue
		}
		beforeBoundary := start == 0 || !identifierRune(valueRunes[start-1])
		end := start + len(identityRunes)
		afterBoundary := end == len(valueRunes) || !identifierRune(valueRunes[end])
		if beforeBoundary && afterBoundary {
			return true
		}
	}
	return false
}

func bytesContainIdentity(encoded []byte, identity string) bool {
	quoted, err := json.Marshal(identity)
	return err == nil && strings.Contains(string(encoded), string(quoted))
}

func repositoryLocation(location evidence.Location) bool {
	if location.Path == "" || path.Clean(location.Path) != location.Path ||
		path.IsAbs(location.Path) || location.Path == "." ||
		strings.HasPrefix(location.Path, "../") || strings.Contains(location.Path, "\\") {
		return false
	}
	if location.Line <= 0 || location.Column < 0 || location.EndLine < 0 || location.EndColumn < 0 {
		return false
	}
	if location.EndLine == 0 {
		return location.EndColumn == 0
	}
	if location.EndLine < location.Line {
		return false
	}
	return location.EndLine != location.Line || location.Column == 0 ||
		location.EndColumn == 0 || location.EndColumn >= location.Column
}

func refPrefix(kind RefKind) string {
	switch kind {
	case RefUnit:
		return "u"
	case RefSubsystem:
		return "ss"
	case RefComponent:
		return "c"
	case RefSurface:
		return "sf"
	case RefReadingTarget:
		return "a"
	case RefEvidence:
		return "e"
	case RefDocument:
		return "d"
	case RefRouteSupport:
		return "rs"
	case RefRouteRelation:
		return "rr"
	case RefRouteSpan:
		return "sp"
	default:
		return "x"
	}
}

func refKindRank(kind RefKind) int {
	switch kind {
	case RefUnit:
		return 0
	case RefSubsystem:
		return 1
	case RefComponent:
		return 2
	case RefSurface:
		return 3
	case RefReadingTarget:
		return 4
	case RefEvidence:
		return 8
	case RefDocument:
		return 9
	case RefRouteSupport:
		return 5
	case RefRouteRelation:
		return 6
	case RefRouteSpan:
		return 7
	default:
		return 99
	}
}

func digest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func canonicalRefLess(left, right CanonicalRef) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.ID < right.ID
}

func uniqueSorted(values []string) bool {
	return slices.IsSorted(values) && !hasAdjacentDuplicate(values)
}

func uniqueCanonicalRefs(values []CanonicalRef) bool {
	if !slices.IsSortedFunc(values, func(left, right CanonicalRef) int {
		if canonicalRefLess(left, right) {
			return -1
		}
		if canonicalRefLess(right, left) {
			return 1
		}
		return 0
	}) {
		return false
	}
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return false
		}
	}
	return true
}

func hasAdjacentDuplicate(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] == values[index-1] {
			return true
		}
	}
	return false
}

func cloneSubsystems(values []Subsystem) []Subsystem {
	result := append([]Subsystem(nil), values...)
	for index := range result {
		result[index].ComponentIDs = append([]string(nil), result[index].ComponentIDs...)
	}
	return result
}

func cloneComponents(values []Component) []Component {
	result := append([]Component(nil), values...)
	for index := range result {
		result[index].ReadingTargetIDs = append([]string(nil), result[index].ReadingTargetIDs...)
	}
	return result
}

func cloneSurfaces(values []Surface) []Surface {
	result := append([]Surface(nil), values...)
	for index := range result {
		result[index].ReadingTargetIDs = append([]string(nil), result[index].ReadingTargetIDs...)
	}
	return result
}

func cloneEvidenceFacts(values []EvidenceFact) []EvidenceFact {
	result := append([]EvidenceFact(nil), values...)
	for index := range result {
		result[index].SubjectRefs = append([]CanonicalRef(nil), result[index].SubjectRefs...)
	}
	return result
}

func cloneCatalog(values []CatalogObject) []CatalogObject {
	result := append([]CatalogObject(nil), values...)
	for index := range result {
		if result[index].Owner != nil {
			owner := *result[index].Owner
			result[index].Owner = &owner
		}
		result[index].RelatedComponentRefs = append(
			[]CanonicalRef(nil), result[index].RelatedComponentRefs...,
		)
		result[index].PrincipalRefs = append(
			[]CanonicalRef(nil), result[index].PrincipalRefs...,
		)
		if result[index].Location != nil {
			location := *result[index].Location
			result[index].Location = &location
		}
		for _, field := range []**CanonicalRef{
			&result[index].SupportTarget, &result[index].FromSupport, &result[index].ToSupport,
			&result[index].FromTarget, &result[index].ToTarget,
		} {
			if *field != nil {
				copyRef := **field
				*field = &copyRef
			}
		}
		result[index].RequiredSupportRefs = append([]CanonicalRef(nil), result[index].RequiredSupportRefs...)
		result[index].AllowedTargetRefs = append([]CanonicalRef(nil), result[index].AllowedTargetRefs...)
		result[index].SpanJoins = append([]CanonicalSpanJoin(nil), result[index].SpanJoins...)
	}
	return result
}
