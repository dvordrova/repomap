package groupindex

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// ProjectAtlas turns the atlas into one GroupsIndex per target, the shape
// the page, the orientation and the publication already read: a box is a
// group with explicitly selected declarations and native lexical children,
// a zone is a container, a native relation supplies a connection's endpoints,
// a joint is a connection into another target. Lanes follow the box's side.
// Programs maps a target ID to its program index; every atlas target needs
// one. The result is a validated set.
func ProjectAtlas(programs map[string]programindex.Index, value atlas.Atlas) ([]Index, error) {
	ids := make([]string, 0, len(programs))
	for id := range programs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return programindex.TargetIDLess(ids[i], ids[j]) })
	for _, id := range ids {
		if err := programs[id].Validate(); err != nil {
			return nil, fmt.Errorf("group index: project atlas: target %s: %w", programs[id].Target.Name, err)
		}
	}
	return projectAtlasFrom(ids, value, nil, func(id string) (programindex.Index, error) {
		program, ok := programs[id]
		if !ok {
			return programindex.Index{}, fmt.Errorf("group index: project atlas: target %s has no program index", id)
		}
		return program, nil
	})
}

// ProjectAtlasFrom reads saved programs one at a time; read returns a
// validated index. Only declaration keys used by the atlas survive between
// the lookup and projection passes.
func ProjectAtlasFrom(value atlas.Atlas, read func(string) (programindex.Index, error)) ([]Index, error) {
	return ProjectAtlasWithKeys(value, nil, read)
}

// ProjectAtlasWithKeys is ProjectAtlasFrom with the lookup pass already
// read: keys read for the atlas targets in their order replace it, so each
// program is read once more, for its projection. Keys read for other targets
// or another order are read again here.
func ProjectAtlasWithKeys(value atlas.Atlas, keys *DeclarationKeys, read func(string) (programindex.Index, error)) ([]Index, error) {
	ids := make([]string, 0, len(value.Targets))
	for _, target := range value.Targets {
		ids = append(ids, target.ID)
	}
	return projectAtlasFrom(ids, value, keys, read)
}

// DeclarationKeys are the declaration keys of every object of some programs,
// by target-qualified and by bare object ID. A bare ID two programs share
// keeps the later program's key, as the lookup pass always has.
type DeclarationKeys struct {
	targets []string
	byRef   map[string]string
}

// ReadDeclarationKeys reads the programs one at a time, in order, and keeps
// only their declaration keys.
func ReadDeclarationKeys(ids []string, read func(string) (programindex.Index, error)) (DeclarationKeys, error) {
	keys := DeclarationKeys{targets: slices.Clone(ids), byRef: make(map[string]string)}
	for _, id := range ids {
		program, err := read(id)
		if err != nil {
			return DeclarationKeys{}, err
		}
		for _, object := range program.Objects {
			key := DeclarationKey(object)
			keys.byRef[program.Target.ID+"."+object.ID] = key
			keys.byRef[object.ID] = key
		}
	}
	return keys, nil
}

func projectAtlasFrom(ids []string, value atlas.Atlas, keys *DeclarationKeys, read func(string) (programindex.Index, error)) ([]Index, error) {
	if err := atlas.Validate(value); err != nil {
		return nil, fmt.Errorf("group index: project atlas: %w", err)
	}
	projected := make([]projectedTarget, 0, len(value.Targets))
	groupIDs := make(map[string]map[string]string, len(value.Targets)) // target -> box -> group
	boxOfBoundary := make(map[string]map[string]string, len(value.Targets))
	boundaries := make(map[string]map[string]atlas.Boundary)
	// Native IDs and source refs are scoped by target. A declaration's source
	// anchor and kind identify it across a library and its executable.
	sourceRefs := make(map[string]string)
	for _, target := range value.Targets {
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				for _, symbol := range file.Symbols {
					sourceRefs[symbol.ObjectID] = ""
				}
			}
		}
		for _, entry := range target.OffMap {
			for _, symbol := range entry.File.Symbols {
				sourceRefs[symbol.ObjectID] = ""
			}
		}
		for _, boundary := range target.Boundaries {
			sourceRefs[boundary.ObjectID] = ""
		}
		for _, call := range target.Unsure {
			sourceRefs[call.ObjectID] = ""
		}
		for _, idiom := range target.Idioms {
			for _, id := range idiom.ObjectIDs {
				sourceRefs[id] = ""
			}
		}
	}
	if keys == nil || !slices.Equal(keys.targets, ids) {
		read, err := ReadDeclarationKeys(ids, read)
		if err != nil {
			return nil, err
		}
		keys = &read
	}
	for ref := range sourceRefs {
		sourceRefs[ref] = keys.byRef[ref]
	}
	for _, target := range value.Targets {
		program, err := read(target.ID)
		if err != nil {
			return nil, err
		}
		one, err := projectTarget(program, target, sourceRefs)
		if err != nil {
			return nil, err
		}
		projected = append(projected, one)
		groupIDs[target.ID] = one.groupOfBox
		boxes := make(map[string]string, len(target.Boundaries))
		boundaries[target.ID] = make(map[string]atlas.Boundary)
		localRefs := make(map[string]string)
		for _, boundary := range target.Boundaries {
			localRefs[sourceRefs[boundary.ObjectID]] = ""
		}
		for _, object := range program.Objects {
			key := DeclarationKey(object)
			if _, needed := localRefs[key]; needed {
				localRefs[key] = object.ID
			}
		}
		for _, boundary := range target.Boundaries {
			boundary.ObjectID = localRefs[sourceRefs[boundary.ObjectID]]
			boxes[boundary.ID] = boundary.BoxID
			boundaries[target.ID][boundary.ID] = boundary
		}
		boxOfBoundary[target.ID] = boxes
	}
	// Joints are stored by their source target, as matching stored them.
	// A catalogue joint names the peer input a table row sends: it is kept
	// on the row's input and draws nothing.
	at := make(map[string]int, len(projected))
	for position := range projected {
		at[projected[position].index.Target.ID] = position
	}
	for _, joint := range value.Joints {
		if !joint.Same || joint.SourceKind != "catalogue" {
			continue
		}
		// atlas.Validate holds a joint's targets to the atlas's, and every
		// target is projected. A boundary refused before it became an
		// input (a name that cannot stand) has no operation to keep a peer.
		from, to := at[joint.From.TargetID], at[joint.To.TargetID]
		row, peer := projected[from].operationOf[joint.From.BoundaryID], projected[to].operationOf[joint.To.BoundaryID]
		if row == "" || peer == "" {
			continue
		}
		for position := range projected[from].index.Operations {
			operation := &projected[from].index.Operations[position]
			if operation.ID == row {
				operation.Sends = append(operation.Sends, PeerInput{TargetID: joint.To.TargetID, OperationID: peer, Label: strings.TrimSpace(joint.Label)})
			}
		}
	}
	for _, joint := range value.Joints {
		if !joint.Same || joint.SourceKind == "catalogue" {
			continue
		}
		endpointBox := func(endpoint atlas.Endpoint) string {
			if endpoint.BoxID != "" {
				return endpoint.BoxID
			}
			return boxOfBoundary[endpoint.TargetID][endpoint.BoundaryID]
		}
		fromGroup := groupIDs[joint.From.TargetID][endpointBox(joint.From)]
		toGroup := groupIDs[joint.To.TargetID][endpointBox(joint.To)]
		if fromGroup == "" || toGroup == "" {
			continue
		}
		for position := range projected {
			if projected[position].index.Target.ID != joint.From.TargetID {
				continue
			}
			label := strings.TrimSpace(joint.Label)
			if label == "" {
				label = "integrates with"
			}
			resolution := programindex.PatternValueExact
			if joint.Possible {
				resolution = programindex.PatternValuePossible
			}
			connection := Connection{
				From:              Endpoint{TargetID: joint.From.TargetID, GroupID: fromGroup},
				To:                Endpoint{TargetID: joint.To.TargetID, GroupID: toGroup},
				SemanticKind:      snakeCase(label),
				Label:             label,
				Summary:           strings.TrimSpace(joint.Value),
				SupportResolution: resolution,
				Evidence:          []SubjectEndpoint{},
				SourceKind:        joint.SourceKind,
				SourceID:          joint.ID,
			}
			if from, ok := boundaries[joint.From.TargetID][joint.From.BoundaryID]; ok {
				connection.FromSubjectID = from.ObjectID
				connection.FromLocation = &programindex.Location{Path: from.Path, Line: from.LineNo, Column: max(1, from.Column)}
			}
			if to, ok := boundaries[joint.To.TargetID][joint.To.BoundaryID]; ok {
				connection.ToSubjectID = to.ObjectID
				connection.ToLocation = &programindex.Location{Path: to.Path, Line: to.LineNo, Column: max(1, to.Column)}
			}
			if connection.SourceKind == "" && joint.From.BoundaryID != "" {
				connection.SourceKind = "integration"
			}
			if connection.Summary == "" {
				connection.Summary = label
			}
			projected[position].index.Connections = append(projected[position].index.Connections, connection)
		}
	}
	result := make([]Index, 0, len(projected))
	for _, one := range projected {
		index := one.index
		sort.Slice(index.Connections, func(i, j int) bool {
			return connectionKey(index.Connections[i]) < connectionKey(index.Connections[j])
		})
		index.Connections = dedupeConnections(index.Connections)
		assignConnectionIDs(index.Connections, 0)
		// Derived after the joints are in, as Hydrate derives them, so the
		// ordinary run and a saved rendering phase the same connections.
		Derive(&index)
		seal, err := indexDigest(index)
		if err != nil {
			return nil, err
		}
		index.SHA256 = seal
		if err := index.Validate(); err != nil {
			return nil, fmt.Errorf("group index: project atlas target %s: %w", index.Target.Name, err)
		}
		result = append(result, index)
	}
	if err := ValidateSet(result); err != nil {
		return nil, fmt.Errorf("group index: project atlas: %w", err)
	}
	return result, nil
}

// drawnEnds are the declarations a relation's arrow may reach: its targets,
// or, for a call left unresolved, what the witnesses of its stores name. The
// relation keeps its resolution, so such an arrow is possible like one of
// several alternatives; nothing here makes a witness a target.
func drawnEnds(relation programindex.Relation) []string {
	if relation.Resolution != programindex.ResolutionUnresolved {
		return relation.ToIDs
	}
	var ends []string
	for _, witness := range relation.Witnesses {
		if witness.ObjectID != "" && !slices.Contains(ends, witness.ObjectID) {
			ends = append(ends, witness.ObjectID)
		}
	}
	return ends
}

// storedName is one name the stores of a pair's open calls wrote: how often,
// the first call that reads it, its first store and where its function is
// declared.
type storedName struct {
	count               int
	call, store, callee *programindex.Location
}

func (name *storedName) observe(call, store, callee *programindex.Location) {
	name.count++
	if name.count == 1 || locationBefore(call, name.call) {
		name.call = call
	}
	if name.count == 1 || locationBefore(store, name.store) {
		name.store = store
	}
	if name.count == 1 || locationBefore(callee, name.callee) {
		name.callee = callee
	}
}

// locationBefore orders source positions by file, line and column; an
// unknown one comes after every known one.
func locationBefore(a, b *programindex.Location) bool {
	switch {
	case a == nil || b == nil:
		return a != nil
	case a.Path != b.Path:
		return a.Path < b.Path
	case a.Line != b.Line:
		return a.Line < b.Line
	default:
		return a.Column < b.Column
	}
}

// storedSentence is the fallback sentence of a pair's calls left open: the
// names their stores wrote, most often named first, as the reading writes it
// for an arrow the model has not spoken for. A tie goes in source order, as
// the reading's does: the call, then the store, then the declaration.
func storedSentence(from, to string, names map[string]*storedName) string {
	witnesses := make([]atlas.Witness, 0, len(names))
	for name := range names {
		witnesses = append(witnesses, atlas.Witness{Callee: name})
	}
	sort.Slice(witnesses, func(i, j int) bool {
		a, b := names[witnesses[i].Callee], names[witnesses[j].Callee]
		switch {
		case a.count != b.count:
			return a.count > b.count
		case locationBefore(a.call, b.call) || locationBefore(b.call, a.call):
			return locationBefore(a.call, b.call)
		case locationBefore(a.store, b.store) || locationBefore(b.store, a.store):
			return locationBefore(a.store, b.store)
		case locationBefore(a.callee, b.callee) || locationBefore(b.callee, a.callee):
			return locationBefore(a.callee, b.callee)
		default:
			return witnesses[i].Callee < witnesses[j].Callee
		}
	})
	return lines.FallbackSentence(lines.BoxSummary{Title: from}, lines.BoxSummary{Title: to}, witnesses)
}

type projectedTarget struct {
	index      Index
	groupOfBox map[string]string
	// operationOf is the operation each boundary became, by boundary ID.
	operationOf map[string]string
}

func categoryOfLane(lane Lane) programindex.Category {
	switch lane {
	case LaneTriggers:
		return programindex.CategoryInbound
	case LaneDependencies:
		return programindex.CategoryDependency
	default:
		return programindex.CategoryCore
	}
}

func projectTarget(program programindex.Index, target atlas.Target, sourceRefs map[string]string) (projectedTarget, error) {
	boxOfDeclaration := make(map[string]*atlas.Box)
	objects := make(map[string]programindex.Object, len(program.Objects))
	declarations := make(map[string]bool, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
		declarations[DeclarationKey(object)] = true
	}
	interpretations := make(map[string]Interpretation)
	interpret := func(file atlas.File) {
		for _, symbol := range file.Symbols {
			interpretation := Interpretation{Line: symbol.Line, Alias: symbol.Alias, Key: symbol.Key, Activation: symbol.Activation, Operation: symbol.Operation, OperationSummary: symbol.OperationSummary, Helper: symbol.Helper}
			if interpretation != (Interpretation{}) {
				interpretations[sourceRefs[symbol.ObjectID]] = interpretation
			}
		}
	}
	for position := range target.Boxes {
		box := &target.Boxes[position]
		for _, id := range box.MemberIDs {
			key := sourceRefs[id]
			if key == "" {
				if object, ok := objects[id]; ok {
					key = DeclarationKey(object)
				}
			}
			if key == "" || !declarations[key] {
				return projectedTarget{}, fmt.Errorf("atlas projection: part %q names an unknown declaration %q", box.ID, id)
			}
			if previous := boxOfDeclaration[key]; previous != nil && previous.ID != box.ID {
				return projectedTarget{}, fmt.Errorf("atlas projection: declaration %q belongs to two parts", id)
			}
			boxOfDeclaration[key] = box
		}
		for _, file := range box.Files {
			interpret(file)
		}
	}
	// A file off the map keeps its declarations' captions and keys; its
	// subjects stay outside every group.
	for _, entry := range target.OffMap {
		interpret(entry.File)
	}
	// Every object remains a subject. Explicit declarations select semantic
	// membership; only native lexical ownership carries it to inner objects.
	// A shared source path cannot put unrelated declarations into a part.
	memberBoxes := make(map[string]*atlas.Box)
	var locate func(string, map[string]bool) *atlas.Box
	locate = func(id string, seen map[string]bool) *atlas.Box {
		if seen[id] {
			return nil
		}
		seen[id] = true
		if box := memberBoxes[id]; box != nil {
			return box
		}
		object, exists := objects[id]
		if !exists {
			return nil
		}
		box := boxOfDeclaration[DeclarationKey(object)]
		// A selected module body owns its observed top-level activity, not
		// every declaration in the file. Those declarations still require
		// their own accepted membership, including after a refused model row.
		if box == nil && object.OwnerID != "" && objects[object.OwnerID].Kind != programindex.ObjectModule {
			box = locate(object.OwnerID, seen)
		}
		if box == nil && object.ContainerID != "" && objects[object.ContainerID].Kind != programindex.ObjectModule {
			box = locate(object.ContainerID, seen)
		}
		if box != nil {
			memberBoxes[id] = box
		}
		return box
	}
	membersOfBox := make(map[string][]string, len(target.Boxes))
	retained := make(map[string]struct{})
	for _, object := range program.Objects {
		retained[object.ID] = struct{}{}
	}
	subjects := compileRetainedSubjects(program, retained)
	byID := make(map[string]*Subject, len(subjects))
	for i := range subjects {
		byID[subjects[i].ID] = &subjects[i]
	}
	// A part its program never runs is listed off the map by its
	// declarations, file by file.
	unreached := make(map[string][]programindex.Object)
	drawn := make(map[string]*atlas.Box)
	for _, object := range program.Objects {
		box := locate(object.ID, map[string]bool{})
		if box != nil && box.Unreached && object.Location != nil {
			unreached[box.ID] = append(unreached[box.ID], object)
		}
		if box != nil && !box.OffCanvas() {
			drawn[object.ID] = box
			membersOfBox[box.ID] = append(membersOfBox[box.ID], object.ID)
		}
		subject := byID[object.ID]
		subject.Categories = []programindex.Category{}
		if object.Location != nil {
			if interpretation, ok := interpretations[DeclarationKey(object)]; ok {
				subject.Interpretation = &interpretation
			}
		}
	}
	// Only the part holding a declaration execution starts from (a target
	// seed) is the program's entry. Taking requests or listening does not
	// make one: the inputs' own arrows say where the outside calls in. A
	// part that only calls out stands with the dependencies; every other
	// part is in the middle. The atlas side stays the reading's column fact.
	seeds := make(map[string]bool, len(program.Target.Seeds))
	for _, seed := range program.Target.Seeds {
		seeds[seed.ObjectID] = true
	}
	laneOfBox := make(map[string]Lane, len(target.Boxes))
	for _, box := range target.Boxes {
		lane := LaneCore
		if box.Side == atlas.SideOut {
			lane = LaneDependencies
		}
		if slices.ContainsFunc(membersOfBox[box.ID], func(id string) bool { return seeds[id] }) {
			lane = LaneTriggers
		}
		laneOfBox[box.ID] = lane
	}
	for id, box := range drawn {
		byID[id].Categories = []programindex.Category{categoryOfLane(laneOfBox[box.ID])}
	}
	sort.Slice(subjects, func(i, j int) bool { return subjectIDLess(subjects[i].ID, subjects[j].ID) })

	groupOfBox := make(map[string]string, len(target.Boxes))
	groupValueOfBox := make(map[string]string, len(target.Boxes))
	groups := make([]Group, 0, len(target.Boxes))
	for _, box := range target.Boxes {
		members := membersOfBox[box.ID]
		// A part made only of test code is not a part of the program's map;
		// its files are listed off the map as tests. Nor is a part the
		// program never runs; its declarations are listed as unreachable.
		if len(members) == 0 || box.OffCanvas() {
			continue
		}
		sort.Slice(members, func(i, j int) bool { return subjectIDLess(members[i], members[j]) })
		members = compactSorted(members)
		// An empty line is the part's explicit no-description state.
		summary := strings.TrimSpace(box.Line)
		group := Group{
			Title: strings.TrimSpace(box.Title), Summary: summary, Lane: laneOfBox[box.ID], Core: box.Core,
			MemberSubjectIDs: members, EvidenceSubjectIDs: []string{},
		}
		groupValueOfBox[box.ID] = groupKey(group)
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groupKey(groups[i]) < groupKey(groups[j]) })
	groupIDByValue := make(map[string]string, len(groups))
	for position := range groups {
		groups[position].ID = compactOrdinal("g", position)
		groupIDByValue[groupKey(groups[position])] = groups[position].ID
	}
	for boxID, value := range groupValueOfBox {
		groupOfBox[boxID] = groupIDByValue[value]
	}

	// An area is the program's entry when one of its parts is; otherwise
	// it stands where most of its parts stand. It is core when any part in
	// it is.
	groupByID := make(map[string]Group, len(groups))
	for _, group := range groups {
		groupByID[group.ID] = group
	}
	containers := make([]Container, 0, len(target.Zones))
	for _, zone := range target.Zones {
		var ids []string
		core := false
		lanes := make(map[Lane]int)
		for _, boxID := range zone.BoxIDs {
			groupID, ok := groupOfBox[boxID]
			if !ok {
				continue
			}
			ids = append(ids, groupID)
			lanes[groupByID[groupID].Lane]++
			core = core || groupByID[groupID].Core
		}
		// A zone of one group is that group; a container holds several.
		if len(ids) < 2 {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j], "g") })
		lane := LaneCore
		best := 0
		for _, candidate := range []Lane{LaneCore, LaneDependencies} {
			if lanes[candidate] > best {
				lane, best = candidate, lanes[candidate]
			}
		}
		if lanes[LaneTriggers] > 0 {
			lane = LaneTriggers
		}
		summary := strings.TrimSpace(zone.Line)
		container := Container{Title: strings.TrimSpace(zone.Title), Summary: summary, Lane: lane, Core: core, GroupIDs: ids}
		containers = append(containers, container)
	}
	// Containers keep the atlas's zone order, which is the order the areas
	// answer listed them; k1 is the first area the model named.
	for position := range containers {
		containers[position].ID = compactOrdinal("k", position)
	}

	connections := make([]Connection, 0, len(target.Arrows))
	for _, arrow := range target.Arrows {
		from, to := groupOfBox[arrow.From], groupOfBox[arrow.To]
		if from == "" || to == "" || from == to {
			continue
		}
		sentence := strings.TrimSpace(arrow.Sentence)
		if sentence == "" {
			sentence = "calls"
		}
		connection := Connection{
			From:              Endpoint{TargetID: program.Target.ID, GroupID: from},
			To:                Endpoint{TargetID: program.Target.ID, GroupID: to},
			SemanticKind:      "calls",
			Label:             sentence,
			Summary:           sentence,
			SupportResolution: programindex.PatternValueExact,
			Evidence:          []SubjectEndpoint{},
		}
		connections = append(connections, connection)
	}
	if len(boxOfDeclaration) > 0 {
		// Semantic parts do not inherit file-to-file arrows. Rebind every
		// original declaration relation, including same-file calls, reads and
		// callbacks, with its actual endpoints and source locations.
		sentences := map[[2]string]string{}
		for _, connection := range connections {
			sentences[[2]string{connection.From.GroupID, connection.To.GroupID}] = connection.Summary
		}
		connections = []Connection{}
		// A call left open reaches the map through the declarations its stores
		// name. The reading saw no call there, so the pair's sentence, when it
		// has one, says what its exact calls do; these connections take the
		// code's fallback over the names their stores wrote instead.
		stored := map[[2]string]map[string]*storedName{}
		var open []int
		// A callable written inline reads by the function holding it
		// (inlineHolders), never by its number: litestream's canvas cards
		// had read "FindSQLiteDatabases$1 calls IsSQLiteDatabase".
		inline := inlineHolders(program)
		said := func(id string) string {
			if name := inline[id]; name != "" {
				return name
			}
			return objects[id].Name
		}
		for _, relation := range program.Relations {
			fromBox := memberBoxes[relation.FromID]
			if fromBox == nil {
				continue
			}
			from := groupOfBox[fromBox.ID]
			for _, id := range drawnEnds(relation) {
				toBox := memberBoxes[id]
				if toBox == nil {
					continue
				}
				to := groupOfBox[toBox.ID]
				if from == "" || to == "" || from == to {
					continue
				}
				label := said(relation.FromID) + " " + string(relation.Kind) + " " + said(id)
				summary := label
				if sentence := sentences[[2]string{from, to}]; relation.Kind == programindex.RelationCalls && sentence != "" {
					summary = sentence
				}
				location := relation.Location
				if location == nil {
					location = objects[relation.FromID].Location
				}
				resolution := programindex.PatternValuePossible
				if relation.Resolution == programindex.ResolutionExact {
					resolution = programindex.PatternValueExact
				}
				evidence := []SubjectEndpoint{{TargetID: program.Target.ID, SubjectID: relation.FromID}, {TargetID: program.Target.ID, SubjectID: id}}
				sort.Slice(evidence, func(i, j int) bool { return evidence[i].SubjectID < evidence[j].SubjectID })
				connection := Connection{From: Endpoint{TargetID: program.Target.ID, GroupID: from}, To: Endpoint{TargetID: program.Target.ID, GroupID: to},
					SemanticKind: string(relation.Kind), Label: label, Summary: summary, SupportResolution: resolution, Evidence: evidence,
					SourceKind: "native_" + string(relation.Kind), SourceID: relation.ID, FromSubjectID: relation.FromID, ToSubjectID: id,
					FromLocation: location, ToLocation: objects[id].Location}
				if relation.Resolution == programindex.ResolutionUnresolved && relation.Kind == programindex.RelationCalls {
					pair := [2]string{from, to}
					if stored[pair] == nil {
						stored[pair] = map[string]*storedName{}
					}
					name := stored[pair][said(id)]
					if name == nil {
						name = &storedName{}
						stored[pair][said(id)] = name
					}
					var store *programindex.Location
					for _, witness := range relation.Witnesses {
						if witness.ObjectID == id && witness.Location != nil && (store == nil || locationBefore(witness.Location, store)) {
							store = witness.Location
						}
					}
					name.observe(location, store, objects[id].Location)
					open = append(open, len(connections))
				}
				connections = append(connections, connection)
			}
		}
		titles := make(map[string]string, len(groups))
		for _, group := range groups {
			titles[group.ID] = group.Title
		}
		for _, position := range open {
			connection := &connections[position]
			pair := [2]string{connection.From.GroupID, connection.To.GroupID}
			connection.Summary = storedSentence(titles[pair[0]], titles[pair[1]], stored[pair])
		}
	}

	// An operation belongs to its subject's group; one in a file off the map
	// belongs to none and is still read. Test-only parts publish none, and
	// neither does a declaration this program never runs.
	var operations []Operation
	for _, subject := range subjects {
		if subject.Interpretation == nil || subject.Interpretation.Activation == "" {
			continue
		}
		if subject.Object == nil || subject.Object.Location == nil || objects[subject.ID].Unreachable {
			continue
		}
		groupID := ""
		if box := memberBoxes[subject.ID]; box != nil {
			if box.OffCanvas() {
				continue
			}
			groupID = groupOfBox[box.ID]
		}
		v := subject.Interpretation
		operations = append(operations, Operation{ID: subject.ID, SubjectID: subject.ID, GroupID: groupID, Kind: v.Activation, Name: v.Operation, Summary: v.OperationSummary, Source: "model", Location: *subject.Object.Location})
	}
	// An undecided declaration is named by its subject, and a handler-less
	// input's declaring caller found: the program's object at the
	// declaration's source key.
	objectOfKey := make(map[string]string, len(program.Objects))
	for _, object := range program.Objects {
		if key := DeclarationKey(object); key != "" {
			if _, seen := objectOfKey[key]; !seen {
				objectOfKey[key] = object.ID
			}
		}
	}
	// subjectOf is the program's object for an atlas object reference.
	subjectOf := func(ref string) string {
		if ref == "" {
			return ""
		}
		if byID[ref] != nil {
			return ref
		}
		if key := sourceRefs[ref]; key != "" {
			return objectOfKey[key]
		}
		return ""
	}
	// enclosing is the innermost function, method or lambda whose source
	// holds a site, else the module body of its file: the code making a
	// call there.
	kindByID := make(map[string]programindex.ObjectKind, len(program.Objects))
	for _, object := range program.Objects {
		kindByID[object.ID] = object.Kind
	}
	enclosing := func(location programindex.Location) string {
		best, module := "", ""
		var bestLine int
		for _, object := range program.Objects {
			if object.Location == nil || object.Location.Path != location.Path {
				continue
			}
			if object.Kind == programindex.ObjectVariable && object.ContainerID != "" && kindByID[object.ContainerID] != programindex.ObjectModule && kindByID[object.ContainerID] != programindex.ObjectPackage {
				continue
			}
			switch object.Kind {
			case programindex.ObjectModule:
				module = object.ID
			case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectVariable:
				// A table variable's initializer holds its rows: the table
				// declares them.
				if object.Location.Line <= location.Line && location.Line <= object.EndLine && object.Location.Line >= bestLine {
					best, bestLine = object.ID, object.Location.Line
				}
			}
		}
		if best == "" {
			best = module
		}
		if byID[best] == nil {
			return ""
		}
		return best
	}
	boundRequests := make(map[string]bool)
	// What each boundary's operation is declared on, and whether its call
	// gives words of its own (J1 below), by boundary ID: folding spellings
	// moves the operations.
	onOf := make(map[string]*DeclaredOn)
	wordless := make(map[string]bool)
	declaredOn := func(boundary atlas.Boundary) *DeclaredOn {
		on := boundary.DeclaredOn
		if on == nil {
			return nil
		}
		return &DeclaredOn{Location: programindex.Location{Path: on.Path, Line: on.LineNo, Column: max(1, on.Column)}, Text: on.Text}
	}
	// An entry whose handler is not established is one input per kind,
	// words as written and declaring caller: the same option declared at
	// two sites of one function is one option, and the first site stands
	// for it.
	declared := make(map[string]int)
	// standsFor is, for a boundary whose operation was folded into another
	// (the same handler-less input at a second site, a hand-over joined to
	// its word entry, a handler registered twice under one name), the
	// boundary whose operation stands for it: a joint naming the first
	// names the second's operation.
	standsFor := make(map[string]string)
	// aliasOf is, for a handler-less input another spelling of one value,
	// the boundary of its first spelling (atlas Boundary.AliasOf).
	aliasOf := make(map[string]string)
	for _, boundary := range target.Boundaries {
		kind := OperationKind(boundary.Kind)
		if boundary.Direction != atlas.DirectionIn || kind == "" {
			continue
		}
		// A boundary in a file off the map names no box: its operation
		// belongs to no group. One in a test-only part is the tests'.
		groupID := groupOfBox[boundary.BoxID]
		if groupID == "" && boundary.BoxID != "" {
			continue
		}
		source := "model"
		if boundary.FactID != "" {
			source = "fact"
		}
		location := programindex.Location{Path: boundary.Path, Line: boundary.LineNo, Column: max(1, boundary.Column)}
		if boundary.HandlerUnknown {
			// Its handler is not established, so it has no subject and no
			// reach: the caller that declares it is not taken for its code.
			// It is named by its words, never by that caller; a name that
			// cannot stand refuses this entry alone.
			if !validText(boundary.Name) {
				continue
			}
			declaredBy := boundary.ObjectID
			if byID[declaredBy] == nil {
				declaredBy = ""
				if key := sourceRefs[boundary.ObjectID]; key != "" {
					declaredBy = objectOfKey[key]
				}
			}
			if byID[declaredBy] == nil {
				declaredBy = ""
			}
			operation := Operation{ID: boundary.ID, FactID: boundary.FactID, GroupID: groupID, Kind: kind, Name: boundary.Name, Address: boundary.Address, Summary: boundary.Line, Source: source, Location: location, HandlerUnknown: true, DeclaredBy: declaredBy, DeclaredOn: declaredOn(boundary), Written: boundary.Written, ValueOf: boundary.ValueOf}
			if boundary.AliasOf != "" {
				aliasOf[boundary.ID] = boundary.AliasOf
			}
			key := strings.Join(append([]string{kind, boundary.ObjectID}, boundary.Values...), "\x00")
			if on := operation.DeclaredOn; on != nil {
				// Two objects in one function are two declarations.
				key += "\x00" + operationLocationKey(on.Location)
			}
			if at, seen := declared[key]; seen {
				if locationBefore(&location, &operations[at].Location) {
					standsFor[operations[at].ID] = operation.ID
					operations[at] = operation
				} else {
					standsFor[operation.ID] = operations[at].ID
				}
				continue
			}
			declared[key] = len(operations)
			operations = append(operations, operation)
			continue
		}
		subjectID := boundary.ObjectID
		if byID[subjectID] == nil {
			subjectID = ""
			for _, object := range program.Objects {
				if DeclarationKey(object) == sourceRefs[boundary.ObjectID] {
					subjectID = object.ID
					break
				}
			}
		}
		// The entry is named by the words the model chose among those its
		// registration wrote, as written; without a choice, by its handler's
		// own name, never by the function making the registration (othello's
		// `:draw draw/draw-state`, no word chosen, had read
		// othello.ui.sketch/start!: the boundary's Caller is the function
		// enclosing the call when the handler is written in another file). A
		// method and a path are two such words, never a shape.
		name := boundary.Name
		if subject := byID[subjectID]; name == "" && subject != nil && subject.Object != nil {
			name = subject.Object.Name
		}
		if name == "" {
			name = boundary.Caller
		}
		// A handler written inline is named as a reader names it, never
		// by its number (inline.go): DatabasesTool$1 is DatabasesTool
		// (inline).
		if subject := byID[subjectID]; boundary.Name == "" && subject != nil && subject.Object != nil && subject.Object.Inline != "" {
			name = subject.Object.Inline
		}
		onOf[boundary.ID] = declaredOn(boundary)
		wordless[boundary.ID] = len(boundary.Values) == 0
		operations = append(operations, Operation{ID: boundary.ID, FactID: boundary.FactID, SubjectID: subjectID, GroupID: groupID, Kind: kind, Name: name, Address: boundary.Address, Summary: boundary.Line, Source: source, Location: location, DeclaredBy: enclosing(location), Written: boundary.Written})
		if subjectID != "" {
			boundRequests[subjectID] = true
		}
	}
	operations = foldSpellings(operations, aliasOf, standsFor)
	// J1: a hand-over with no words of its own, made on what a word entry
	// of the same kind produced (set_defaults(func=f) on add_parser("x")'s
	// parser, .action(run) on .command("x")), is one input with that entry:
	// named by its words, handled by the handed callable, at the entry's
	// site. Kinds that differ stay two: code prefers neither.
	entryAt := make(map[string]int)
	for position, operation := range operations {
		if operation.HandlerUnknown {
			entryAt[operationLocationKey(operation.Location)] = position
		}
	}
	joinedInto := make(map[int]bool)
	dropped := make(map[int]bool)
	for position, operation := range operations {
		on := onOf[operation.ID]
		if operation.HandlerUnknown || operation.SubjectID == "" || on == nil || !wordless[operation.ID] {
			continue
		}
		at, ok := entryAt[operationLocationKey(on.Location)]
		if !ok || joinedInto[at] || operations[at].Kind != operation.Kind {
			continue
		}
		entry := &operations[at]
		entry.SubjectID, entry.GroupID, entry.FactID, entry.Source = operation.SubjectID, operation.GroupID, operation.FactID, operation.Source
		entry.HandlerUnknown = false
		if entry.Summary == "" {
			entry.Summary = operation.Summary
		}
		joinedInto[at], dropped[position] = true, true
		standsFor[operation.ID] = entry.ID
	}
	// An input whose own call made an object only inputs of its kind with
	// their own handlers are declared on names where the one chosen is
	// kept, and is no input of its own: argparse's
	// add_subparsers(dest="command"), on which freqtrade's 34 subcommands
	// are joined to their handlers. Those inputs are its words, each its
	// own tile. An object an option without a handler is declared on too
	// is a command's own (a group taking options).
	holds := make(map[string]bool)
	for position, operation := range operations {
		on := operation.DeclaredOn
		if on == nil {
			on = onOf[operation.ID]
		}
		if dropped[position] || on == nil {
			continue
		}
		key := operation.Kind + "\x00" + operationLocationKey(on.Location)
		if seen, known := holds[key]; !operation.HandlerUnknown && (!known || seen) {
			holds[key] = true
		} else if operation.HandlerUnknown {
			holds[key] = false
		}
	}
	for position, operation := range operations {
		if !dropped[position] && operation.HandlerUnknown && holds[operation.Kind+"\x00"+operationLocationKey(operation.Location)] {
			dropped[position] = true
		}
	}
	if len(dropped) > 0 {
		kept := operations[:0]
		for position, operation := range operations {
			if !dropped[position] {
				kept = append(kept, operation)
			}
		}
		operations = kept
	}
	// A declaration with an observed route already has an operation carrying
	// that route's syntax. Keep its interpretation on the subject, without
	// presenting the declaration as a second route. Multiple observed routes
	// on the same handler remain distinct.
	uniqueOperations := operations[:0]
	sameOperation := make(map[string]string)
	for _, operation := range operations {
		if operation.ID == operation.SubjectID && boundRequests[operation.SubjectID] {
			continue
		}
		// The same handler registered twice under one name (`GET("")` and
		// `GET("/")`) is one operation; the first site stands for it.
		key := strings.Join([]string{operation.Kind, operation.Name, operation.SubjectID, operation.Address}, "\x00")
		if first, seen := sameOperation[key]; operation.SubjectID != "" && seen {
			standsFor[operation.ID] = first
			continue
		}
		sameOperation[key] = operation.ID
		uniqueOperations = append(uniqueOperations, operation)
	}
	operations = uniqueOperations
	sort.Slice(operations, func(i, j int) bool { return operationKey(operations[i]) < operationKey(operations[j]) })
	operationOf := make(map[string]string, len(operations)+len(standsFor))
	for position := range operations {
		operationOf[operations[position].ID] = compactOrdinal("o", position)
		operations[position].ID = compactOrdinal("o", position)
	}
	for folded := range standsFor {
		kept := folded
		for {
			next, ok := standsFor[kept]
			if !ok {
				break
			}
			kept = next
		}
		operationOf[folded] = operationOf[kept]
	}
	for position := range operations {
		if operations[position].ValueOf != "" {
			operations[position].ValueOf = operationOf[operations[position].ValueOf]
		}
	}
	offMap, err := projectOffMap(target, unreached, func(symbol atlas.Symbol) (string, bool) {
		key := sourceRefs[symbol.ObjectID]
		if key == "" {
			if object, ok := objects[symbol.ObjectID]; ok {
				key = DeclarationKey(object)
			}
		}
		id, ok := objectOfKey[key]
		return id, ok && key != ""
	})
	if err != nil {
		return projectedTarget{}, err
	}
	var unsure []UnsureCall
	// atlas.Validate holds every unsure call, idiom and declared-on site to
	// its shape.
	for _, call := range target.Unsure {
		location := programindex.Location{Path: call.Path, Line: call.LineNo, Column: max(1, call.Column)}
		subject := subjectOf(call.ObjectID)
		if subject == "" {
			subject = enclosing(location)
		}
		unsure = append(unsure, UnsureCall{SubjectID: subject, Location: location, Symbol: call.Symbol, Reason: call.Reason})
	}
	var idioms []Idiom
	for _, idiom := range target.Idioms {
		row := Idiom{Symbol: idiom.Symbol, Kind: OperationKind(idiom.Kind), Entries: idiom.Entries, Calls: idiom.Calls}
		for _, id := range idiom.ObjectIDs {
			if subject := subjectOf(id); subject != "" && !slices.Contains(row.SubjectIDs, subject) {
				row.SubjectIDs = append(row.SubjectIDs, subject)
			}
		}
		idioms = append(idioms, row)
	}
	// A file's calls and writes are named by the subject making them: the
	// declaration the reading named, else the function holding the site.
	data := projectData(program, target.Data, func(ref string, at facts.Anchor) string {
		if subject := subjectOf(ref); subject != "" {
			return subject
		}
		return enclosing(programindex.Location{Path: at.Path, Line: at.Line, Column: max(1, at.Column)})
	})
	outbound := projectOutbound(program, target, groupOfBox, sourceRefs)
	joinOutboundData(outbound, data)
	// Each outgoing call is reached from the callers outside its part, a
	// path starting inside it at a seed or an input's handler.
	entries := make(map[string]bool, len(seeds)+len(operations))
	for id := range seeds {
		entries[id] = true
	}
	for _, operation := range operations {
		if operation.SubjectID != "" && !operation.HandlerUnknown {
			entries[operation.SubjectID] = true
		}
	}
	reached := newReachedFrom(program, func(id string) string {
		if box := drawn[id]; box != nil {
			return groupOfBox[box.ID]
		}
		return ""
	}, entries)
	for position := range outbound {
		outbound[position].ReachedFrom = reached.of(outbound[position].SubjectID)
	}
	index := Index{
		Version:            Version,
		Role:               target.Role,
		SharedCode:         append([]string(nil), target.SharedCode...),
		Summary:            target.Line,
		Target:             program.Target.Snapshot(),
		ProgramIndexSHA256: program.SHA256,
		Data:               data,
		Subjects:           subjects,
		Groups:             groups,
		Operations:         operations,
		Outbound:           outbound,
		Containers:         containers,
		StructuralEdges:    compileStructuralEdges(program, retained),
		Connections:        connections,
		OffMap:             offMap,
		MapFailure:         strings.TrimSpace(target.MapFailure),
		Unsure:             unsure,
		Idioms:             idioms,
		Unresolved:         compileUnresolvedCalls(program, retained),
		Branches:           compileInputBranches(program, retained),
		Handed:             compileHanded(program, retained),
	}
	return projectedTarget{index: index, groupOfBox: groupOfBox, operationOf: operationOf}, nil
}

// projectOffMap lists the files the map of parts does not draw: the atlas's
// off-map record with its reasons, and the files of parts made only of test
// code under the reason tests with their part's name. A file a part holds is
// not listed for the few declarations of it that are off the map; their
// interpretations are still read. A file whose code several parts hold is
// on the map: only the declarations no box of it took are listed, by their
// subjects, under the reason undecided. A part its program never runs is
// listed by its declarations, by file and with its name, under the reason
// unreachable: its file may be another part's too.
func projectOffMap(target atlas.Target, unreached map[string][]programindex.Object, subjectOf func(atlas.Symbol) (string, bool)) ([]OffMapFile, error) {
	var files []OffMapFile
	seen := map[string]bool{}
	add := func(file OffMapFile) {
		if key := offMapKey(file); !seen[key] {
			seen[key] = true
			files = append(files, file)
		}
	}
	// A file a part holds some declarations of is on the map: what is off it
	// there is listed by its subjects, under its own reason, and so is every
	// undecided declaration. A file no part holds is listed whole.
	held := map[string]bool{}
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			held[atlasPath(file.Path)] = true
		}
	}
	for _, entry := range target.OffMap {
		path := atlasPath(entry.File.Path)
		if entry.Reason != atlas.OffMapUndecided && entry.Reason != atlas.OffMapBlocked && (!held[path] || len(entry.File.Symbols) == 0) {
			add(OffMapFile{Path: path, Reason: entry.Reason})
			continue
		}
		var ids []string
		for _, symbol := range entry.File.Symbols {
			id, ok := subjectOf(symbol)
			if !ok {
				return nil, fmt.Errorf("atlas projection: off-map declaration %q of %s is unknown", symbol.Name, entry.File.Path)
			}
			ids = appendUniqueString(ids, id)
		}
		if len(ids) > 0 {
			add(OffMapFile{Path: path, Reason: entry.Reason, SubjectIDs: ids})
		}
	}
	for _, box := range target.Boxes {
		if box.ForTests {
			for _, file := range box.Files {
				add(OffMapFile{Path: atlasPath(file.Path), Reason: OffMapTests, Part: strings.TrimSpace(box.Title)})
			}
		}
		if !box.Unreached {
			continue
		}
		byFile := map[string][]string{}
		for _, object := range unreached[box.ID] {
			path := atlasPath(object.Location.Path)
			byFile[path] = append(byFile[path], object.ID)
		}
		for path, ids := range byFile {
			sort.Slice(ids, func(i, j int) bool { return subjectIDLess(ids[i], ids[j]) })
			add(OffMapFile{Path: path, Reason: OffMapUnreachable, Part: strings.TrimSpace(box.Title), SubjectIDs: ids})
		}
	}
	sort.Slice(files, func(i, j int) bool { return offMapKey(files[i]) < offMapKey(files[j]) })
	return files, nil
}

func operationKey(operation Operation) string {
	return strings.Join([]string{
		operation.GroupID, operation.Kind, operation.Name, operation.Summary,
		operation.Source, operation.FactID, operation.SubjectID,
		fmt.Sprintf("%s:%d:%d", operation.Location.Path, operation.Location.Line, operation.Location.Column), operation.ID,
	}, "\x00")
}

// DeclarationKey is a declaration's identity across the programs that index
// it: its source anchor (path, line, column), kind and name. Object IDs
// repeat across targets and a name alone is no identity; one located
// declaration compiled into two programs has this one key in both.
func DeclarationKey(object programindex.Object) string {
	if object.Location == nil {
		return ""
	}
	location := object.Location
	return fmt.Sprintf("%s:%d:%d:%s:%s", location.Path, location.Line, location.Column, object.Kind, object.Name)
}

func dedupeConnections(values []Connection) []Connection {
	result := values[:0]
	for i, value := range values {
		if i > 0 && connectionKey(values[i-1]) == connectionKey(value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func compactSorted(values []string) []string {
	result := values[:0]
	for i, value := range values {
		if i > 0 && values[i-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func atlasPath(value string) string {
	value = strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
	if value == "" {
		return "."
	}
	return value
}

// snakeCase makes a closed semantic kind out of a label: lowercase words
// joined by underscores, letters and digits only.
func snakeCase(label string) string {
	var out strings.Builder
	underscore := false
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			underscore = false
		default:
			if !underscore && out.Len() > 0 {
				out.WriteByte('_')
				underscore = true
			}
		}
	}
	kind := strings.Trim(out.String(), "_")
	if kind == "" || kind[0] < 'a' || kind[0] > 'z' {
		kind = "integrates_with"
	}
	return kind
}

// foldSpellings makes the spellings of one value one input (atlas
// Boundary.AliasOf, the reading's spellings.go): the operation of the first
// spelling stands, named by its words, and lists each other spelling's name,
// site and call as written among its aliases, in source order; a joint
// naming another spelling names that operation. An alias whose first
// spelling made no operation of its kind stays its own input.
func foldSpellings(operations []Operation, aliasOf, standsFor map[string]string) []Operation {
	if len(aliasOf) == 0 {
		return operations
	}
	at := make(map[string]int, len(operations))
	for position, operation := range operations {
		if operation.HandlerUnknown {
			at[operation.ID] = position
		}
	}
	folded := make(map[int]bool)
	for position, operation := range operations {
		first, ok := aliasOf[operation.ID]
		if !ok {
			continue
		}
		for seen := map[string]bool{}; standsFor[first] != "" && !seen[first]; first = standsFor[first] {
			seen[first] = true
		}
		stands, ok := at[first]
		if !ok || stands == position || folded[stands] || operations[stands].Kind != operation.Kind {
			continue
		}
		operations[stands].Aliases = append(operations[stands].Aliases, OperationAlias{Name: operation.Name, Location: operation.Location, Written: operation.Written})
		folded[position] = true
		standsFor[operation.ID] = operations[stands].ID
	}
	if len(folded) == 0 {
		return operations
	}
	kept := operations[:0]
	for position, operation := range operations {
		if folded[position] {
			continue
		}
		slices.SortStableFunc(operation.Aliases, func(a, b OperationAlias) int {
			if locationBefore(&a.Location, &b.Location) {
				return -1
			}
			if locationBefore(&b.Location, &a.Location) {
				return 1
			}
			return 0
		})
		kept = append(kept, operation)
	}
	return kept
}

// validOperationKind accepts the kinds the reading's activations and the
// incoming boundary kinds map onto.
func validOperationKind(kind string) bool {
	switch kind {
	case "command", "request", "consumer", "scheduled", "interaction", "extension", "entry", "continuous", "setting":
		return true
	default:
		return false
	}
}

// OperationKind is what an accepted incoming boundary makes its handler: the
// entry kinds the reading stage chooses map one to one onto operations.
func OperationKind(boundaryKind string) string {
	switch boundaryKind {
	case atlas.BoundaryRequest:
		return "request"
	case atlas.BoundaryContinuous:
		return "continuous"
	case atlas.BoundaryQueueConsumer:
		return "consumer"
	case atlas.BoundaryScheduled:
		return "scheduled"
	case atlas.BoundaryInteraction:
		return "interaction"
	case atlas.BoundaryExtension:
		return "extension"
	case atlas.BoundaryCommand:
		return "command"
	case atlas.BoundarySetting:
		return "setting"
	case atlas.BoundaryOther:
		return "entry"
	default:
		return ""
	}
}
