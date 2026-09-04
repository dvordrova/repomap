package groupindex

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// ProjectAtlas turns the atlas into one GroupsIndex per target, the shape
// the page, the orientation and the publication already read: a box is a
// group whose members are the objects declared in its files, a zone is a
// container, an arrow is a connection labelled with the model's sentence,
// a joint is a connection into another target. Lanes follow the box's side.
// Programs maps a target ID to its program index; every atlas target needs
// one. The result is a validated set.
func ProjectAtlas(programs map[string]programindex.Index, value atlas.Atlas) ([]Index, error) {
	if err := atlas.Validate(value); err != nil {
		return nil, fmt.Errorf("group index: project atlas: %w", err)
	}
	projected := make([]projectedTarget, 0, len(value.Targets))
	groupIDs := make(map[string]map[string]string, len(value.Targets)) // target -> box -> group
	boxOfBoundary := make(map[string]map[string]string, len(value.Targets))
	for _, target := range value.Targets {
		program, ok := programs[target.ID]
		if !ok {
			return nil, fmt.Errorf("group index: project atlas: target %s has no program index", target.Name)
		}
		if err := program.Validate(); err != nil {
			return nil, fmt.Errorf("group index: project atlas: target %s: %w", target.Name, err)
		}
		one, err := projectTarget(program, target)
		if err != nil {
			return nil, err
		}
		projected = append(projected, one)
		groupIDs[target.ID] = one.groupOfBox
		boxes := make(map[string]string, len(target.Boundaries))
		for _, boundary := range target.Boundaries {
			boxes[boundary.ID] = boundary.BoxID
		}
		boxOfBoundary[target.ID] = boxes
	}
	// Joints are stored by their source target, as matching stored them.
	for _, joint := range value.Joints {
		if !joint.Same {
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
			}
			if connection.Summary == "" {
				connection.Summary = label
			}
			connection.ID = connectionIdentity(connection)
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

type projectedTarget struct {
	index      Index
	groupOfBox map[string]string
}

func laneOfSide(side string) Lane {
	switch side {
	case atlas.SideIn:
		return LaneTriggers
	case atlas.SideOut:
		return LaneDependencies
	default:
		return LaneCore
	}
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

func projectTarget(program programindex.Index, target atlas.Target) (projectedTarget, error) {
	boxOfFile := make(map[string]*atlas.Box)
	for position := range target.Boxes {
		box := &target.Boxes[position]
		for _, file := range box.Files {
			boxOfFile[file.Path] = box
		}
	}
	// Every object is a subject, so owner and container references resolve;
	// the objects of a box's files carry its lane's category.
	membersOfBox := make(map[string][]string, len(target.Boxes))
	subjects := make([]Subject, 0, len(program.Objects))
	for _, object := range program.Objects {
		categories := []programindex.Category{}
		if object.Location != nil {
			if box, ok := boxOfFile[atlasPath(object.Location.Path)]; ok {
				categories = []programindex.Category{categoryOfLane(laneOfSide(box.Side))}
				membersOfBox[box.ID] = append(membersOfBox[box.ID], object.ID)
			}
		}
		subjects = append(subjects, Subject{
			ID: object.ID, Kind: SubjectObject, Categories: categories,
			Object: &ObjectFacts{
				Name: object.Name, Kind: object.Kind, Visibility: object.Visibility,
				Signature: object.Signature, OwnerID: object.OwnerID, ContainerID: object.ContainerID,
				SymbolLinkIdentities: cloneSymbolLinkIdentities(object.SymbolLinkIdentities),
				External:             cloneExternal(object.External),
				Location:             cloneLocation(object.Location),
			},
		})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].ID < subjects[j].ID })

	groupOfBox := make(map[string]string, len(target.Boxes))
	groups := make([]Group, 0, len(target.Boxes))
	for _, box := range target.Boxes {
		members := membersOfBox[box.ID]
		if len(members) == 0 {
			continue
		}
		sort.Strings(members)
		members = compactSorted(members)
		summary := strings.TrimSpace(box.Line)
		if summary == "" {
			summary = strings.TrimSpace(box.Title)
		}
		group := Group{
			Title: strings.TrimSpace(box.Title), Summary: summary, Lane: laneOfSide(box.Side),
			MemberSubjectIDs: members, EvidenceSubjectIDs: []string{},
		}
		group.ID = groupIdentity(program.Target.ID, group)
		groupOfBox[box.ID] = group.ID
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })

	containers := make([]Container, 0, len(target.Zones))
	for _, zone := range target.Zones {
		var ids []string
		lanes := make(map[Lane]int)
		for _, boxID := range zone.BoxIDs {
			groupID, ok := groupOfBox[boxID]
			if !ok {
				continue
			}
			ids = append(ids, groupID)
			for _, box := range target.Boxes {
				if box.ID == boxID {
					lanes[laneOfSide(box.Side)]++
				}
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		lane := LaneCore
		best := 0
		for _, candidate := range []Lane{LaneCore, LaneTriggers, LaneDependencies} {
			if lanes[candidate] > best {
				lane, best = candidate, lanes[candidate]
			}
		}
		summary := strings.TrimSpace(zone.Line)
		if summary == "" {
			summary = strings.TrimSpace(zone.Title)
		}
		container := Container{Title: strings.TrimSpace(zone.Title), Summary: summary, Lane: lane, GroupIDs: ids}
		container.ID = containerIdentity(program.Target.ID, container)
		containers = append(containers, container)
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
		connection.ID = connectionIdentity(connection)
		connections = append(connections, connection)
	}

	return projectedTarget{
		index: Index{
			Version:            Version,
			Target:             program.Target.Snapshot(),
			ProgramIndexSHA256: program.SHA256,
			Subjects:           subjects,
			Groups:             groups,
			Containers:         containers,
			StructuralEdges:    []StructuralEdge{},
			Connections:        connections,
		},
		groupOfBox: groupOfBox,
	}, nil
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
