package navigator

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func Compile(input Input) (Compiled, error) {
	atlas, err := repositoryatlas.Canonical(input.Atlas)
	if err != nil {
		return Compiled{}, fmt.Errorf("navigator: Atlas: %w", err)
	}
	question, err := exactMeaning("question", input.Question)
	if err != nil {
		return Compiled{}, err
	}
	if err := validateLimits(input.Limits); err != nil {
		return Compiled{}, err
	}

	units := make(map[string]repositoryatlas.Unit, len(atlas.Units))
	for _, unit := range atlas.Units {
		units[unit.ID] = unit
	}
	if _, ok := units[input.ScopeUnitID]; !ok {
		return Compiled{}, fmt.Errorf("navigator: unknown scope unit")
	}
	entities := make(map[string]repositoryatlas.Entity, len(atlas.Entities))
	for _, entity := range atlas.Entities {
		entities[entity.ID] = entity
	}
	evidence := make(map[string]repositoryatlas.Evidence, len(atlas.Evidence))
	for _, item := range atlas.Evidence {
		evidence[item.ID] = item
	}

	if err := rejectIdentityInMeaning(question, atlas); err != nil {
		return Compiled{}, err
	}
	seedIDs, seedSet, err := validateSeeds(input.Seeds, input.ScopeUnitID, entities, units)
	if err != nil {
		return Compiled{}, err
	}
	if err := enforceLimit("seeds", input.Limits.MaxSeeds, len(seedIDs)); err != nil {
		return Compiled{}, err
	}

	gaps, err := validateGaps(input.Gaps, seedSet, input.ScopeUnitID, entities, evidence, units, atlas)
	if err != nil {
		return Compiled{}, err
	}
	if err := enforceLimit("gaps", input.Limits.MaxGaps, len(gaps)); err != nil {
		return Compiled{}, err
	}
	actions, err := validateActions(input.Actions, seedSet, input.ScopeUnitID, entities, units, atlas)
	if err != nil {
		return Compiled{}, err
	}
	if err := enforceLimit("actions", input.Limits.MaxActions, len(actions)); err != nil {
		return Compiled{}, err
	}

	trails := directTrails(atlas.Relations, seedSet, input.ScopeUnitID, entities, units)
	if err := enforceLimit("direct_trails", input.Limits.MaxDirectTrails, len(trails)); err != nil {
		return Compiled{}, err
	}
	visibleEntityIDs := append([]string(nil), seedIDs...)
	visibleSet := make(map[string]struct{}, len(seedIDs)+2*len(trails))
	for _, id := range seedIDs {
		visibleSet[id] = struct{}{}
	}
	for _, relation := range trails {
		visibleSet[relation.Source.ID] = struct{}{}
		visibleSet[relation.Target.ID] = struct{}{}
	}
	visibleEntityIDs = visibleEntityIDs[:0]
	for id := range visibleSet {
		visibleEntityIDs = append(visibleEntityIDs, id)
	}
	sort.Strings(visibleEntityIDs)

	evidenceIDs := representativeEvidence(
		atlas.Observations, trails, gaps, visibleSet, input.ScopeUnitID, evidence, units,
	)
	if err := enforceLimit("evidence", input.Limits.MaxEvidence, len(evidenceIDs)); err != nil {
		return Compiled{}, err
	}
	intersections := deriveIntersections(trails, seedSet)
	if err := enforceLimit("intersections", input.Limits.MaxIntersections, len(intersections)); err != nil {
		return Compiled{}, err
	}

	visibleUnitIDs := scopedUnitSlice(
		input.ScopeUnitID, visibleEntityIDs, evidenceIDs, entities, evidence, units,
	)
	if err := validateUnitLabels(visibleUnitIDs, units, input.Limits.MaxUnitLabelBytes, atlas); err != nil {
		return Compiled{}, err
	}
	entityRoles := deriveEntityRoles(visibleEntityIDs, trails, entities)
	refs, entries := assignRefs(
		visibleUnitIDs, visibleEntityIDs, trails, intersections, evidenceIDs, gaps, actions, entities, entityRoles,
	)
	projection := buildWire(
		question, "", input.ScopeUnitID, seedIDs, visibleUnitIDs, visibleEntityIDs,
		trails, intersections, evidenceIDs, gaps, actions, refs, atlas.Observations, units, entities, entityRoles,
	)
	projectionJSON, err := marshalCanonical(projection)
	if err != nil {
		return Compiled{}, fmt.Errorf("navigator: encode projection: %w", err)
	}
	material := catalogMaterial{
		Version: Version, ProjectionSHA256: digestBytes(projectionJSON), ScopeUnitID: input.ScopeUnitID,
		Limits: input.Limits, Entries: entries,
	}
	materialJSON, err := marshalCanonical(material)
	if err != nil {
		return Compiled{}, fmt.Errorf("navigator: encode private catalog: %w", err)
	}
	catalogSHA := digestBytes(materialJSON)
	catalogRef := "navigator-v1-" + catalogSHA

	wire := buildWire(
		question, catalogRef, input.ScopeUnitID, seedIDs, visibleUnitIDs, visibleEntityIDs,
		trails, intersections, evidenceIDs, gaps, actions, refs, atlas.Observations, units, entities, entityRoles,
	)
	wireJSON, err := marshalCanonical(wire)
	if err != nil {
		return Compiled{}, fmt.Errorf("navigator: encode wire: %w", err)
	}
	if err := enforceLimit("wire_bytes", input.Limits.MaxWireBytes, len(wireJSON)); err != nil {
		return Compiled{}, err
	}
	private := privateCatalog{
		entries: make(map[string]catalogEntry, len(entries)), byCanonical: make(map[string][]catalogEntry, len(entries)),
		outsideCanonical: make(map[string]struct{}),
	}
	for _, entry := range entries {
		private.entries[entry.Ref] = entry
		private.byCanonical[entry.CanonicalID] = append(private.byCanonical[entry.CanonicalID], entry)
	}
	for _, id := range allCanonicalIDs(atlas) {
		if len(private.byCanonical[id]) == 0 {
			private.outsideCanonical[id] = struct{}{}
		}
	}
	return Compiled{
		wire: wireJSON, wireSHA256: digestBytes(wireJSON),
		catalogSHA256: catalogSHA, catalogRef: catalogRef,
		maxResponseBytes: input.Limits.MaxResponseBytes, catalog: private,
	}, nil
}

func validateLimits(limits Limits) error {
	values := []struct {
		name  string
		value int
	}{
		{"max_wire_bytes", limits.MaxWireBytes},
		{"max_response_bytes", limits.MaxResponseBytes},
		{"max_unit_label_bytes", limits.MaxUnitLabelBytes},
		{"max_seeds", limits.MaxSeeds},
		{"max_direct_trails", limits.MaxDirectTrails},
		{"max_intersections", limits.MaxIntersections},
		{"max_evidence", limits.MaxEvidence},
		{"max_gaps", limits.MaxGaps},
		{"max_actions", limits.MaxActions},
	}
	for _, item := range values {
		if item.value < 0 || ((item.name == "max_wire_bytes" || item.name == "max_response_bytes" || item.name == "max_unit_label_bytes") && item.value == 0) {
			return fmt.Errorf("navigator: %s must be explicitly non-negative and byte limits must be positive", item.name)
		}
	}
	return nil
}

func validateUnitLabels(
	unitIDs []string,
	units map[string]repositoryatlas.Unit,
	limit int,
	atlas repositoryatlas.Atlas,
) error {
	for _, id := range unitIDs {
		label := units[id].Name
		if !utf8.ValidString(label) {
			return fmt.Errorf("navigator: Unit label must be exact UTF-8 text")
		}
		for _, r := range label {
			if r == 0 {
				return fmt.Errorf("navigator: Unit label contains a control byte")
			}
		}
		if err := enforceLimit("unit_label_bytes", limit, len(label)); err != nil {
			return err
		}
		if err := rejectIdentityInMeaning(label, atlas); err != nil {
			return fmt.Errorf("navigator: Unit label exposes a canonical identity or source locator")
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

func exactMeaning(field, value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return "", fmt.Errorf("navigator: %s must be nonempty exact UTF-8 text", field)
	}
	for _, r := range value {
		if r == 0 {
			return "", fmt.Errorf("navigator: %s contains a control byte", field)
		}
	}
	return value, nil
}

func rejectIdentityInMeaning(value string, atlas repositoryatlas.Atlas) error {
	var identities []string
	for _, unit := range atlas.Units {
		identities = append(identities, unit.ID)
	}
	for _, entity := range atlas.Entities {
		identities = append(identities, entity.ID)
	}
	for _, item := range atlas.Evidence {
		identities = append(identities, item.ID, item.Location.Path, item.Symbol)
	}
	for _, relation := range atlas.Relations {
		identities = append(identities, relation.ID)
	}
	for _, identity := range identities {
		if identity == "" {
			continue
		}
		if value == identity || (len(identity) >= 8 && strings.Contains(value, identity)) {
			return fmt.Errorf("navigator: semantic text contains a canonical identity or source locator")
		}
	}
	return nil
}

func validateSeeds(
	values []repositoryatlas.EntityRef,
	scope string,
	entities map[string]repositoryatlas.Entity,
	units map[string]repositoryatlas.Unit,
) ([]string, map[string]struct{}, error) {
	ids := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, ref := range values {
		entity, ok := entities[ref.ID]
		if !ok {
			return nil, nil, fmt.Errorf("navigator: seed references an unknown entity")
		}
		if entity.Kind != ref.Kind {
			return nil, nil, fmt.Errorf("navigator: seed has the wrong entity kind")
		}
		if !unitInScope(entity.UnitID, scope, units) {
			return nil, nil, fmt.Errorf("navigator: seed is outside the requested Unit scope")
		}
		if _, duplicate := seen[entity.ID]; duplicate {
			return nil, nil, fmt.Errorf("navigator: duplicate seed entity")
		}
		seen[entity.ID] = struct{}{}
		ids = append(ids, entity.ID)
	}
	sort.Strings(ids)
	return ids, seen, nil
}

func validateGaps(
	values []ProvenGap,
	seeds map[string]struct{},
	scope string,
	entities map[string]repositoryatlas.Entity,
	evidence map[string]repositoryatlas.Evidence,
	units map[string]repositoryatlas.Unit,
	atlas repositoryatlas.Atlas,
) ([]ProvenGap, error) {
	result := append([]ProvenGap(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	previous := ""
	for index := range result {
		item := &result[index]
		if item.Key == "" || item.Key == previous {
			return nil, fmt.Errorf("navigator: gap keys must be nonempty and unique")
		}
		previous = item.Key
		meaning, err := exactMeaning("gap meaning", item.Meaning)
		if err != nil {
			return nil, err
		}
		if err := rejectIdentityInMeaning(meaning, atlas); err != nil {
			return nil, err
		}
		item.Meaning = meaning
		entity, ok := entities[item.Subject.ID]
		if !ok || entity.Kind != item.Subject.Kind {
			return nil, fmt.Errorf("navigator: gap subject is unknown or wrong-kind")
		}
		if _, ok := seeds[entity.ID]; !ok || !unitInScope(entity.UnitID, scope, units) {
			return nil, fmt.Errorf("navigator: gap subject must be a scoped seed")
		}
		if len(item.EvidenceIDs) == 0 {
			return nil, fmt.Errorf("navigator: proven gap requires exact evidence")
		}
		item.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
		sort.Strings(item.EvidenceIDs)
		for evidenceIndex, id := range item.EvidenceIDs {
			if evidenceIndex > 0 && id == item.EvidenceIDs[evidenceIndex-1] {
				return nil, fmt.Errorf("navigator: gap has duplicate evidence")
			}
			exact, ok := evidence[id]
			if !ok || !unitInScope(exact.UnitID, scope, units) {
				return nil, fmt.Errorf("navigator: gap evidence is unknown or outside scope")
			}
		}
	}
	return result, nil
}

func validateActions(
	values []Action,
	seeds map[string]struct{},
	scope string,
	entities map[string]repositoryatlas.Entity,
	units map[string]repositoryatlas.Unit,
	atlas repositoryatlas.Atlas,
) ([]Action, error) {
	result := append([]Action(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	previous := ""
	for index := range result {
		item := &result[index]
		if item.Key == "" || item.Key == previous {
			return nil, fmt.Errorf("navigator: action keys must be nonempty and unique")
		}
		previous = item.Key
		operation, err := exactMeaning("action operation", item.Operation)
		if err != nil {
			return nil, err
		}
		if err := rejectIdentityInMeaning(operation, atlas); err != nil {
			return nil, err
		}
		item.Operation = operation
		entity, ok := entities[item.Target.ID]
		if !ok || entity.Kind != item.Target.Kind {
			return nil, fmt.Errorf("navigator: action target is unknown or wrong-kind")
		}
		if _, ok := seeds[entity.ID]; !ok || !unitInScope(entity.UnitID, scope, units) {
			return nil, fmt.Errorf("navigator: action target must be a scoped seed")
		}
	}
	return result, nil
}

func directTrails(
	values []repositoryatlas.Relation,
	seeds map[string]struct{},
	scope string,
	entities map[string]repositoryatlas.Entity,
	units map[string]repositoryatlas.Unit,
) []repositoryatlas.Relation {
	result := make([]repositoryatlas.Relation, 0)
	for _, relation := range values {
		_, sourceSeed := seeds[relation.Source.ID]
		_, targetSeed := seeds[relation.Target.ID]
		if !sourceSeed && !targetSeed {
			continue
		}
		source := entities[relation.Source.ID]
		target := entities[relation.Target.ID]
		if !unitInScope(relation.UnitID, scope, units) ||
			!unitInScope(source.UnitID, scope, units) || !unitInScope(target.UnitID, scope, units) {
			continue
		}
		result = append(result, relation)
	}
	return result
}

func representativeEvidence(
	observations []repositoryatlas.Observation,
	trails []repositoryatlas.Relation,
	gaps []ProvenGap,
	visibleEntities map[string]struct{},
	scope string,
	evidence map[string]repositoryatlas.Evidence,
	units map[string]repositoryatlas.Unit,
) []string {
	selected := make(map[string]struct{})
	add := func(id string) {
		item, ok := evidence[id]
		if ok && unitInScope(item.UnitID, scope, units) {
			selected[id] = struct{}{}
		}
	}
	for _, observation := range observations {
		if _, ok := visibleEntities[observation.Subject.ID]; !ok {
			continue
		}
		for _, id := range observation.EvidenceRefs {
			add(id)
		}
	}
	for _, relation := range trails {
		for _, id := range relation.EvidenceRefs {
			add(id)
		}
	}
	for _, gap := range gaps {
		for _, id := range gap.EvidenceIDs {
			add(id)
		}
	}
	result := make([]string, 0, len(selected))
	for id := range selected {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

type intersection struct {
	entityID string
	seedIDs  []string
	trailIDs []string
}

func deriveIntersections(
	trails []repositoryatlas.Relation,
	seeds map[string]struct{},
) []intersection {
	type collected struct {
		seeds  map[string]struct{}
		trails map[string]struct{}
	}
	byEntity := make(map[string]*collected)
	add := func(entityID, seedID, trailID string) {
		item := byEntity[entityID]
		if item == nil {
			item = &collected{seeds: make(map[string]struct{}), trails: make(map[string]struct{})}
			byEntity[entityID] = item
		}
		item.seeds[seedID] = struct{}{}
		item.trails[trailID] = struct{}{}
	}
	for _, trail := range trails {
		if _, ok := seeds[trail.Source.ID]; ok {
			add(trail.Target.ID, trail.Source.ID, trail.ID)
		}
		if _, ok := seeds[trail.Target.ID]; ok {
			add(trail.Source.ID, trail.Target.ID, trail.ID)
		}
	}
	ids := make([]string, 0, len(byEntity))
	for id, item := range byEntity {
		if len(item.seeds) >= 2 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result := make([]intersection, 0, len(ids))
	for _, id := range ids {
		item := byEntity[id]
		value := intersection{entityID: id}
		for seed := range item.seeds {
			value.seedIDs = append(value.seedIDs, seed)
		}
		for trail := range item.trails {
			value.trailIDs = append(value.trailIDs, trail)
		}
		sort.Strings(value.seedIDs)
		sort.Strings(value.trailIDs)
		result = append(result, value)
	}
	return result
}

func deriveEntityRoles(
	entityIDs []string,
	trails []repositoryatlas.Relation,
	entities map[string]repositoryatlas.Entity,
) map[string]EntityRole {
	roles := make(map[string]EntityRole, len(entityIDs))
	for _, id := range entityIDs {
		roles[id] = genericEntityRole(entities[id].Kind)
	}
	for _, trail := range trails {
		if trail.Kind != repositoryatlas.RelationExposes ||
			trail.Phase != repositoryatlas.PhaseStartup ||
			trail.Authority != repositoryatlas.AuthorityResolved ||
			entities[trail.Source.ID].Kind != repositoryatlas.EntitySurface ||
			entities[trail.Target.ID].Kind != repositoryatlas.EntityOperation {
			continue
		}
		roles[trail.Source.ID] = EntityRoleProcessEntry
		roles[trail.Target.ID] = EntityRoleApplicationStart
	}
	return roles
}

func genericEntityRole(kind repositoryatlas.EntityKind) EntityRole {
	switch kind {
	case repositoryatlas.EntitySurface:
		return EntityRoleGenericSurface
	case repositoryatlas.EntityOperation:
		return EntityRoleGenericOperation
	case repositoryatlas.EntityBoundary:
		return EntityRoleGenericBoundary
	case repositoryatlas.EntityResource:
		return EntityRoleGenericResource
	case repositoryatlas.EntityContract:
		return EntityRoleGenericContract
	default:
		return ""
	}
}

func scopedUnitSlice(
	scope string,
	entityIDs, evidenceIDs []string,
	entities map[string]repositoryatlas.Entity,
	evidence map[string]repositoryatlas.Evidence,
	units map[string]repositoryatlas.Unit,
) []string {
	selected := map[string]struct{}{scope: {}}
	addChain := func(id string) {
		for id != "" {
			selected[id] = struct{}{}
			if id == scope {
				return
			}
			id = units[id].ParentID
		}
	}
	for _, id := range entityIDs {
		addChain(entities[id].UnitID)
	}
	for _, id := range evidenceIDs {
		addChain(evidence[id].UnitID)
	}
	result := make([]string, 0, len(selected))
	for id := range selected {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

type refIndex struct {
	units         map[string]string
	entities      map[string]string
	trails        map[string]string
	intersections map[string]string
	evidence      map[string]string
	gaps          map[string]string
	actions       map[string]string
}

func assignRefs(
	unitIDs, entityIDs []string,
	trails []repositoryatlas.Relation,
	intersections []intersection,
	evidenceIDs []string,
	gaps []ProvenGap,
	actions []Action,
	entities map[string]repositoryatlas.Entity,
	entityRoles map[string]EntityRole,
) (refIndex, []catalogEntry) {
	refs := refIndex{
		units: make(map[string]string), entities: make(map[string]string),
		trails: make(map[string]string), intersections: make(map[string]string),
		evidence: make(map[string]string), gaps: make(map[string]string), actions: make(map[string]string),
	}
	entries := make([]catalogEntry, 0, len(unitIDs)+len(entityIDs)+len(trails)+len(intersections)+len(evidenceIDs)+len(gaps)+len(actions))
	for index, id := range unitIDs {
		ref := "u" + strconv.Itoa(index+1)
		refs.units[id] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogUnit, CanonicalID: id})
	}
	entityOrdinals := make(map[repositoryatlas.EntityKind]int)
	for _, id := range entityIDs {
		entity := entities[id]
		entityOrdinals[entity.Kind]++
		ref := entityPrefix(entity.Kind) + strconv.Itoa(entityOrdinals[entity.Kind])
		refs.entities[id] = ref
		entries = append(entries, catalogEntry{
			Ref: ref, Kind: catalogEntity, CanonicalID: id,
			EntityKind: entity.Kind, EntityRole: entityRoles[id],
		})
	}
	for index, item := range trails {
		ref := "t" + strconv.Itoa(index+1)
		refs.trails[item.ID] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogTrail, CanonicalID: item.ID})
	}
	for index, item := range intersections {
		ref := "i" + strconv.Itoa(index+1)
		refs.intersections[item.entityID] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogIntersection, CanonicalID: item.entityID})
	}
	for index, id := range evidenceIDs {
		ref := "e" + strconv.Itoa(index+1)
		refs.evidence[id] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogEvidence, CanonicalID: id})
	}
	for index, item := range gaps {
		ref := "g" + strconv.Itoa(index+1)
		refs.gaps[item.Key] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogGap, CanonicalID: item.Key})
	}
	for index, item := range actions {
		ref := "a" + strconv.Itoa(index+1)
		refs.actions[item.Key] = ref
		entries = append(entries, catalogEntry{Ref: ref, Kind: catalogAction, CanonicalID: item.Key})
	}
	return refs, entries
}

func entityPrefix(kind repositoryatlas.EntityKind) string {
	switch kind {
	case repositoryatlas.EntitySurface:
		return "s"
	case repositoryatlas.EntityOperation:
		return "o"
	case repositoryatlas.EntityBoundary:
		return "b"
	case repositoryatlas.EntityResource:
		return "r"
	case repositoryatlas.EntityContract:
		return "c"
	default:
		return "x"
	}
}

func buildWire(
	question, catalogRef, scope string,
	seedIDs, unitIDs, entityIDs []string,
	trails []repositoryatlas.Relation,
	intersections []intersection,
	evidenceIDs []string,
	gaps []ProvenGap,
	actions []Action,
	refs refIndex,
	observations []repositoryatlas.Observation,
	units map[string]repositoryatlas.Unit,
	entities map[string]repositoryatlas.Entity,
	entityRoles map[string]EntityRole,
) wireProjection {
	wire := wireProjection{
		Version: Version, CatalogRef: catalogRef, Question: question, ScopeRef: refs.units[scope],
		Units: []wireUnit{}, Entities: []wireEntity{}, SeedRefs: []string{}, DirectTrails: []wireTrail{},
		Intersections: []wireIntersection{}, Evidence: []wireEvidence{}, Gaps: []wireGap{}, Actions: []wireAction{},
	}
	for _, id := range unitIDs {
		unit := units[id]
		wire.Units = append(wire.Units, wireUnit{
			Ref: refs.units[id], Kind: unit.Kind, Label: unit.Name,
			ParentRef: refs.units[unit.ParentID],
		})
	}
	for _, id := range entityIDs {
		entity := entities[id]
		wire.Entities = append(wire.Entities, wireEntity{
			Ref: refs.entities[id], Kind: entity.Kind,
			Role: entityRoles[id], UnitRef: refs.units[entity.UnitID],
		})
	}
	for _, id := range seedIDs {
		wire.SeedRefs = append(wire.SeedRefs, refs.entities[id])
	}
	for _, relation := range trails {
		item := wireTrail{
			Ref: refs.trails[relation.ID], SourceRef: refs.entities[relation.Source.ID], TargetRef: refs.entities[relation.Target.ID],
			Kind: relation.Kind, Phase: relation.Phase, Authority: relation.Authority, EvidenceRefs: []string{},
		}
		for _, id := range relation.EvidenceRefs {
			if ref := refs.evidence[id]; ref != "" {
				item.EvidenceRefs = append(item.EvidenceRefs, ref)
			}
		}
		wire.DirectTrails = append(wire.DirectTrails, item)
	}
	for _, value := range intersections {
		item := wireIntersection{Ref: refs.intersections[value.entityID], EntityRef: refs.entities[value.entityID]}
		for _, id := range value.seedIDs {
			item.SeedRefs = append(item.SeedRefs, refs.entities[id])
		}
		for _, id := range value.trailIDs {
			item.TrailRefs = append(item.TrailRefs, refs.trails[id])
		}
		wire.Intersections = append(wire.Intersections, item)
	}
	subjects := make(map[string][]string)
	for _, observation := range observations {
		entityRef := refs.entities[observation.Subject.ID]
		if entityRef == "" {
			continue
		}
		for _, id := range observation.EvidenceRefs {
			if refs.evidence[id] != "" {
				subjects[id] = append(subjects[id], entityRef)
			}
		}
	}
	for _, id := range evidenceIDs {
		values := uniqueSorted(subjects[id])
		wire.Evidence = append(wire.Evidence, wireEvidence{
			Ref: refs.evidence[id], SubjectRefs: values, ExactLocator: true,
		})
	}
	for _, gap := range gaps {
		item := wireGap{Ref: refs.gaps[gap.Key], Meaning: gap.Meaning, SubjectRef: refs.entities[gap.Subject.ID]}
		for _, id := range gap.EvidenceIDs {
			item.EvidenceRefs = append(item.EvidenceRefs, refs.evidence[id])
		}
		wire.Gaps = append(wire.Gaps, item)
	}
	for _, action := range actions {
		wire.Actions = append(wire.Actions, wireAction{
			Ref: refs.actions[action.Key], Operation: action.Operation, TargetRef: refs.entities[action.Target.ID],
		})
	}
	return wire
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func unitInScope(unitID, scopeID string, units map[string]repositoryatlas.Unit) bool {
	for unitID != "" {
		if unitID == scopeID {
			return true
		}
		unitID = units[unitID].ParentID
	}
	return false
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("%x", digest[:])
}

func allCanonicalIDs(atlas repositoryatlas.Atlas) []string {
	result := make([]string, 0, len(atlas.Units)+len(atlas.Entities)+len(atlas.Evidence)+len(atlas.Relations))
	for _, unit := range atlas.Units {
		result = append(result, unit.ID)
	}
	for _, entity := range atlas.Entities {
		result = append(result, entity.ID)
	}
	for _, item := range atlas.Evidence {
		result = append(result, item.ID)
	}
	for _, relation := range atlas.Relations {
		result = append(result, relation.ID)
	}
	return result
}
