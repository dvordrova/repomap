package coremap

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// CompileGoProgram feeds the Go adapter's complete language-neutral
// ProgramIndex into the same CoreMap map/reduce path as Python while retaining
// the exact SSA and core-object authorities required by the existing Go
// presentation joins. ProgramIndex object source refs are adapter-owned; an
// exact core-object/direct-node binding is restored locally and never guessed
// from model output.
func CompileGoProgram(
	repoName string,
	repository *corpus.Corpus,
	program programindex.Index,
	target analysistarget.Target,
	direct surfacediscovery.DirectCallIndex,
	coreObjects gocoreobject.Index,
	readmeRoles readmetargetscout.Result,
) (Compilation, error) {
	if program.Target.Language != "go" {
		return Compilation{}, fmt.Errorf("coremap: Go ProgramIndex has language %q", program.Target.Language)
	}
	if err := goadapter.ValidateTargetBinding(target, program.Target); err != nil {
		return Compilation{}, fmt.Errorf("coremap: Go ProgramIndex target: %w", err)
	}
	if err := target.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("coremap: Go target: %w", err)
	}
	if err := direct.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("coremap: Go direct-call index: %w", err)
	}
	if direct.State != surfacediscovery.DirectCallIndexReady || direct.Scope.TargetRef != target.Ref {
		return Compilation{}, fmt.Errorf("coremap: Go direct-call authority does not match the target")
	}
	if err := validateCoreObjectScope(target, coreObjects); err != nil {
		return Compilation{}, err
	}
	compilation, err := compileProgram(repoName, repository, program, readmeRoles)
	if err != nil {
		return Compilation{}, err
	}
	objects := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	directNodeByObjectSource := make(map[string]string, len(coreObjects.Callables))
	for _, callable := range coreObjects.Callables {
		if callable.DirectCallNodeID == "" {
			continue
		}
		if previous, duplicate := directNodeByObjectSource[callable.ID]; duplicate && previous != callable.DirectCallNodeID {
			return Compilation{}, fmt.Errorf("coremap: Go callable has conflicting direct-node authority")
		}
		directNodeByObjectSource[callable.ID] = callable.DirectCallNodeID
	}
	for ref, authority := range compilation.symbols {
		object, ok := objects[authority.fact.NodeID]
		if !ok {
			return Compilation{}, fmt.Errorf("coremap: Go symbol has no ProgramIndex object authority")
		}
		directNodeID := directNodeByObjectSource[object.SourceRef]
		if directNodeID == "" {
			if _, exists := direct.Node(object.SourceRef); exists {
				directNodeID = object.SourceRef
			}
		}
		if directNodeID != "" {
			node, exists := direct.Node(directNodeID)
			if !exists || object.Location == nil || !goCallableNameMatchesDirectNode(object.Name, node.Symbol.Name) ||
				node.Declaration.Path != object.Location.Path ||
				node.Declaration.Line != object.Location.Line ||
				node.Declaration.Column != object.Location.Column {
				return Compilation{}, fmt.Errorf(
					"coremap: Go ProgramIndex callable %q differs from direct-node authority %q",
					object.SourceRef, directNodeID,
				)
			}
			authority.fact.NodeID = directNodeID
			compilation.symbols[ref] = authority
		}
	}
	compilation.target = target.Snapshot()
	compilation.directCallSHA256 = direct.SHA256
	compilation.coreObjectSHA256 = coreObjects.SHA256
	compilation.directCallState = direct.State
	compilation.directCoverage = direct.Coverage
	compilation.seal = sealCompilation(compilation)
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

// go/types retains the source spelling "init", while go/ssa assigns each
// package initializer a unique init#N name. The exact source position and the
// adapter-owned DirectCallNodeID remain the authority; no other spelling
// difference is accepted here.
func goCallableNameMatchesDirectNode(declared, direct string) bool {
	if declared == direct {
		return true
	}
	if declared != "init" || !strings.HasPrefix(direct, "init#") {
		return false
	}
	ordinal := strings.TrimPrefix(direct, "init#")
	value, err := strconv.Atoi(ordinal)
	return err == nil && value > 0 && strconv.Itoa(value) == ordinal
}

// CompileProgram prepares the baseline and bounded refined map/reduce CoreMap
// pipeline from the sealed, language-neutral ProgramIndex. It does not read
// source or invoke a language tool. Language adapters must express uncertainty
// in ProgramIndex coverage and relation resolution before this boundary.
func CompileProgram(
	repoName string,
	repository *corpus.Corpus,
	index programindex.Index,
	readmeRoles readmetargetscout.Result,
) (Compilation, error) {
	compilation, err := compileProgram(repoName, repository, index, readmeRoles)
	if err != nil {
		return Compilation{}, err
	}
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

// CompileProgramWithIntegrationUsage binds the complete selected integration
// operation artifact into the ProgramIndex-backed CoreMap. This is the
// ordinary Python path: the lossless refined shards collectively see every
// accepted operation as additional semantic context, while output authority
// remains exact request-advertised file and symbol refs.
func CompileProgramWithIntegrationUsage(
	repoName string,
	repository *corpus.Corpus,
	index programindex.Index,
	readmeRoles readmetargetscout.Result,
	selected integrationdependency.Result,
	uses integrationusage.Result,
) (Compilation, error) {
	compilation, err := compileProgram(repoName, repository, index, readmeRoles)
	if err != nil {
		return Compilation{}, err
	}
	return BindIntegrationUsage(compilation, index, selected, uses)
}

// BindIntegrationUsage attaches the validated language-neutral operation
// selection to an already sealed CoreMap compilation. This lets a language
// adapter retain exact companion authority while the shared semantic pipeline
// still owns the IntegrationUsage result and the final CoreMap execution.
func BindIntegrationUsage(
	compilation Compilation,
	index programindex.Index,
	selected integrationdependency.Result,
	uses integrationusage.Result,
) (Compilation, error) {
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	if compilation.programTarget == nil || compilation.programIndexSHA256 != index.SHA256 ||
		compilation.programTarget.ID != index.Target.ID {
		return Compilation{}, fmt.Errorf("coremap: prepared compilation does not bind the ProgramIndex")
	}
	if err := uses.ValidateAgainst(index, selected); err != nil {
		return Compilation{}, fmt.Errorf("coremap: integration usage handoff: %w", err)
	}
	usageSHA256, err := uses.ArtifactSHA256()
	if err != nil {
		return Compilation{}, fmt.Errorf("coremap: integration usage identity: %w", err)
	}
	request, err := compileIntegrationUsageRequest(compilation.symbols, selected, uses)
	if err != nil {
		return Compilation{}, err
	}
	compilation.integrationUsageSHA256 = usageSHA256
	compilation.integrationUsage = &request
	compilation.seal = sealCompilation(compilation)
	if err := validateCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

func compileProgram(
	repoName string,
	repository *corpus.Corpus,
	index programindex.Index,
	readmeRoles readmetargetscout.Result,
) (Compilation, error) {
	if !validText(repoName, 256) {
		return Compilation{}, fmt.Errorf("coremap: invalid repository name")
	}
	if repository == nil {
		return Compilation{}, fmt.Errorf("coremap: repository corpus is required")
	}
	if err := index.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("coremap: program index: %w", err)
	}
	snapshot := repository.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("coremap: corpus: %w", err)
	}
	if err := validateProgramTargetCorpus(repository, index.Target); err != nil {
		return Compilation{}, err
	}
	files := make(map[corpus.FileID]FileFact, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		files[entry.ID] = FileFact{FileRef: entry.ID, Path: entry.Path}
	}
	acceptedRoles, err := readmeRoles.SnapshotAgainstCorpus(repository)
	if err != nil {
		return Compilation{}, fmt.Errorf("coremap: README role handoff: %w", err)
	}
	roleRows := make([]roleFileRequest, len(acceptedRoles))
	baselineFiles := make(map[corpus.FileID]FileFact, len(acceptedRoles))
	for position, role := range acceptedRoles {
		file := files[role.FileRef]
		roleRows[position] = roleFileRequest{
			FileRef: role.FileRef, Path: file.Path,
			Classifications: cloneClassifications(role.Classifications),
		}
		baselineFiles[role.FileRef] = file
	}
	targetWire, err := visibleProgramTarget(repository, index.Target)
	if err != nil {
		return Compilation{}, err
	}
	request := baselineRequest{
		Target: targetWire, ProgramCoverage: index.Coverage, RoleFiles: roleRows,
	}
	wire, err := encodeBaselineRequest(request)
	if err != nil {
		return Compilation{}, err
	}
	symbols, rows, seedRows, err := compileProgramSymbols(repository, index)
	if err != nil {
		return Compilation{}, err
	}
	if len(rows) == 0 {
		return Compilation{}, fmt.Errorf(
			"coremap: ProgramIndex has no exact core declarations; README role evidence cannot substitute for program authority",
		)
	}
	dynamicRows, err := compileProgramDynamicRelations(repository, index, symbols)
	if err != nil {
		return Compilation{}, err
	}
	groupingEdges := compileGroupingEdges(index)
	target := index.Target.Snapshot()
	compilation := Compilation{
		repository: repoName, corpusRef: snapshot.Ref,
		programTarget: &target, programIndexSHA256: index.SHA256, programCoverage: index.Coverage,
		baselineRequest: request, baselineWire: wire, files: files,
		baselineFiles: baselineFiles, symbols: symbols, symbolRows: rows,
		targetSeedRows: seedRows, dynamicRelationRows: dynamicRows, groupingEdges: groupingEdges,
	}
	compilation.seal = sealCompilation(compilation)
	return compilation, nil
}

func compileProgramDynamicRelations(
	repository *corpus.Corpus,
	index programindex.Index,
	symbols map[string]symbolAuthority,
) ([]dynamicRelationRequest, error) {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	symbolRefByObjectID := make(map[string]string, len(symbols))
	for ref, symbol := range symbols {
		symbolRefByObjectID[symbol.fact.NodeID] = ref
		symbolRefByObjectID[symbol.programObjectID] = ref
	}
	rows := make([]dynamicRelationRequest, 0)
	relationOrdinal := 0
	jointOrdinal := 0
	for _, relation := range index.Relations {
		if !coreMapDynamicRelation(relation) {
			continue
		}
		fromObject, ok := objects[relation.FromID]
		if !ok {
			return nil, fmt.Errorf("coremap: dynamic relation has unknown source object")
		}
		from, err := compileRelationEndpoint(repository, fromObject, objects, symbolRefByObjectID)
		if err != nil {
			return nil, err
		}
		jointOrdinal++
		jointRef := fmt.Sprintf("j%d", jointOrdinal)
		if len(relation.ToIDs) == 0 {
			relationOrdinal++
			row := dynamicRelationRequest{
				Ref: fmt.Sprintf("r%d", relationOrdinal), JointRef: jointRef, Kind: relation.Kind,
				Resolution: relation.Resolution, From: from, Invocation: relation.Invocation,
				Location:        cloneProgramLocation(relation.Location),
				TargetsObserved: relation.TargetsObserved, TargetsRetained: 0,
				TargetsOmitted: relation.TargetsOmitted, TargetOrdinal: 0,
			}
			row.Perspective = dynamicRelationPerspective(row)
			rows = append(rows, row)
			continue
		}
		for targetPosition, targetID := range relation.ToIDs {
			toObject, ok := objects[targetID]
			if !ok {
				return nil, fmt.Errorf("coremap: dynamic relation has unknown target object")
			}
			to, err := compileRelationEndpoint(repository, toObject, objects, symbolRefByObjectID)
			if err != nil {
				return nil, err
			}
			relationOrdinal++
			row := dynamicRelationRequest{
				Ref: fmt.Sprintf("r%d", relationOrdinal), JointRef: jointRef, Kind: relation.Kind,
				Resolution: relation.Resolution, From: from, To: &to,
				Invocation: relation.Invocation, Location: cloneProgramLocation(relation.Location),
				TargetsObserved: relation.TargetsObserved, TargetsRetained: len(relation.ToIDs),
				TargetsOmitted: relation.TargetsOmitted, TargetOrdinal: targetPosition + 1,
			}
			if from.SymbolRef != "" && to.SymbolRef != "" && from.SymbolRef != to.SymbolRef {
				fromRow := row
				fromRow.Perspective = "from"
				toRow := row
				toRow.Perspective = "to"
				rows = append(rows, fromRow, toRow)
			} else {
				row.Perspective = dynamicRelationPerspective(row)
				rows = append(rows, row)
			}
		}
	}
	if rows == nil {
		rows = []dynamicRelationRequest{}
	}
	return rows, nil
}

func coreMapDynamicRelation(relation programindex.Relation) bool {
	if relation.Resolution == programindex.ResolutionAlternatives {
		return true
	}
	switch relation.Kind {
	case programindex.RelationImplements, programindex.RelationDecorates, programindex.RelationPassesCallback:
		return true
	default:
		return false
	}
}

func compileRelationEndpoint(
	repository *corpus.Corpus,
	object programindex.Object,
	objects map[string]programindex.Object,
	symbolRefByObjectID map[string]string,
) (relationEndpointRequest, error) {
	packageName, err := relationObjectPackage(object, objects)
	if err != nil {
		return relationEndpointRequest{}, err
	}
	if object.Location != nil {
		if _, ok := repository.ID(object.Location.Path); !ok {
			return relationEndpointRequest{}, fmt.Errorf(
				"coremap: dynamic relation endpoint %q is outside repository corpus", object.Name,
			)
		}
	}
	return relationEndpointRequest{
		SymbolRef: symbolRefByObjectID[object.ID], Kind: object.Kind, Name: object.Name,
		Package: packageName, Visibility: object.Visibility, Signature: object.Signature,
		Location: cloneProgramLocation(object.Location),
	}, nil
}

func relationObjectPackage(
	object programindex.Object,
	objects map[string]programindex.Object,
) (string, error) {
	seen := make(map[string]struct{})
	current := object
	for {
		if current.Kind == programindex.ObjectModule || current.Kind == programindex.ObjectPackage {
			return current.Name, nil
		}
		if _, duplicate := seen[current.ID]; duplicate {
			return "", fmt.Errorf("coremap: dynamic relation containment contains a cycle")
		}
		seen[current.ID] = struct{}{}
		nextID := current.ContainerID
		if nextID == "" {
			nextID = current.OwnerID
		}
		if nextID == "" {
			return "", nil
		}
		next, ok := objects[nextID]
		if !ok {
			return "", fmt.Errorf("coremap: dynamic relation endpoint has unknown container")
		}
		current = next
	}
}

func dynamicRelationPerspective(row dynamicRelationRequest) string {
	if row.From.SymbolRef != "" {
		return "from"
	}
	if row.To != nil && row.To.SymbolRef != "" {
		return "to"
	}
	return "relation"
}

func cloneProgramLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func encodeBaselineRequest(request baselineRequest) ([]byte, error) {
	if len(request.RoleFiles) == 0 {
		return nil, nil
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("coremap: encode baseline request: %w", err)
	}
	if err := enforceRequestLimit(StageBaseline, wire, baselinePrompt); err != nil {
		return nil, err
	}
	return wire, nil
}

func validateProgramTargetCorpus(repository *corpus.Corpus, target programindex.Target) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("coremap: program target: %w", err)
	}
	for _, source := range target.Sources {
		info, ok := repository.Info(corpus.FileID(source.FileRef))
		if !ok || info.Entry.Path != source.Path {
			return fmt.Errorf("coremap: program target source %q does not match repository corpus", source.FileRef)
		}
	}
	return nil
}

func visibleProgramTarget(repository *corpus.Corpus, target programindex.Target) (targetRequest, error) {
	info, ok := repository.Info(corpus.FileID(target.AnchorFileRef))
	if !ok {
		return targetRequest{}, fmt.Errorf("coremap: program target anchor is outside repository corpus")
	}
	return targetRequest{
		Language: target.Language, Kind: target.Kind, Name: target.Name,
		Selector: target.Selector, AnchorPath: info.Entry.Path,
	}, nil
}

func compileProgramSymbols(
	repository *corpus.Corpus,
	index programindex.Index,
) (map[string]symbolAuthority, []symbolRequest, []targetSeedRequest, error) {
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	seedKindsByObject := make(map[string][]programindex.SeedKind)
	for _, seed := range index.Target.Seeds {
		kinds := seedKindsByObject[seed.ObjectID]
		seen := false
		for _, kind := range kinds {
			if kind == seed.Kind {
				seen = true
				break
			}
		}
		if !seen {
			seedKindsByObject[seed.ObjectID] = append(kinds, seed.Kind)
		}
	}
	incoming := make(map[string]int)
	outgoing := make(map[string]int)
	unresolved := make(map[string]int)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationExecutes {
			continue
		}
		outgoing[relation.FromID]++
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) == 0 {
			unresolved[relation.FromID]++
			continue
		}
		for _, targetID := range relation.ToIDs {
			incoming[targetID]++
		}
	}

	type programSymbol struct {
		row     symbolRequest
		fact    SymbolFact
		pathKey string
	}
	values := make([]programSymbol, 0)
	for _, object := range index.Objects {
		if !programCoreObject(object, objects) {
			continue
		}
		if object.Location == nil {
			return nil, nil, nil, fmt.Errorf("coremap: core object %q has no exact declaration", object.ID)
		}
		fileRef, ok := repository.ID(object.Location.Path)
		if !ok {
			return nil, nil, nil, fmt.Errorf("coremap: core object %q is outside repository corpus", object.ID)
		}
		packageName, receiver, err := programObjectContext(object, objects)
		if err != nil {
			return nil, nil, nil, err
		}
		location := surfacediscovery.Location{
			Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column,
		}
		row := symbolRequest{
			Kind: object.Kind, FileRef: fileRef, Path: location.Path, Line: location.Line,
			Package: packageName, Name: object.Name, Receiver: receiver,
			Signature: object.Signature, Exported: object.Visibility == programindex.VisibilityPublic,
			IncomingCalls: incoming[object.ID], OutgoingCalls: outgoing[object.ID],
			UnresolvedOutgoing: unresolved[object.ID],
			TargetSeedKinds:    append([]programindex.SeedKind(nil), seedKindsByObject[object.ID]...),
		}
		fact := SymbolFact{
			NodeID: object.ID, Kind: object.Kind,
			Symbol:  surfacediscovery.Symbol{ID: object.ID, Name: object.Name, Package: packageName, Location: location},
			Package: packageName, Exported: row.Exported, Declaration: location,
			IncomingCalls: row.IncomingCalls, OutgoingCalls: row.OutgoingCalls,
			UnresolvedOutgoing: row.UnresolvedOutgoing,
			TargetSeedKinds:    append([]programindex.SeedKind(nil), row.TargetSeedKinds...),
		}
		values = append(values, programSymbol{
			row: row, fact: fact,
			pathKey: location.Path + "\x00" + fmt.Sprintf("%09d", location.Line) + "\x00" + object.ID,
		})
	}
	sort.Slice(values, func(left, right int) bool { return values[left].pathKey < values[right].pathKey })
	authority := make(map[string]symbolAuthority, len(values))
	rows := make([]symbolRequest, len(values))
	symbolRefByObjectID := make(map[string]string, len(values))
	for position := range values {
		ref := fmt.Sprintf("s%d", position+1)
		values[position].row.Ref = ref
		rows[position] = values[position].row
		authority[ref] = symbolAuthority{
			request: rows[position], fact: values[position].fact,
			programObjectID: values[position].fact.NodeID,
		}
		symbolRefByObjectID[values[position].fact.NodeID] = ref
	}
	seedRows := make([]targetSeedRequest, len(index.Target.Seeds))
	for position, seed := range index.Target.Seeds {
		object, ok := objects[seed.ObjectID]
		if !ok || seed.Location == nil {
			return nil, nil, nil, fmt.Errorf("coremap: target seed %q has no exact object and location", seed.ObjectID)
		}
		seedRows[position] = targetSeedRequest{
			Ref: fmt.Sprintf("t%d", position+1), SymbolRef: symbolRefByObjectID[seed.ObjectID],
			Kind: seed.Kind, ObjectKind: object.Kind, Name: object.Name,
			Location: *seed.Location,
		}
	}
	return authority, rows, seedRows, nil
}

func compileGroupingEdges(index programindex.Index) []groupingEdgeAuthority {
	seen := make(map[string]struct{})
	result := make([]groupingEdgeAuthority, 0)
	for _, relation := range index.Relations {
		if relation.Resolution != programindex.ResolutionExact ||
			(relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationExecutes) {
			continue
		}
		for _, targetID := range relation.ToIDs {
			if relation.FromID == "" || targetID == "" || relation.FromID == targetID {
				continue
			}
			key := relation.FromID + "\x00" + targetID
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, groupingEdgeAuthority{FromObjectID: relation.FromID, ToObjectID: targetID})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FromObjectID != result[j].FromObjectID {
			return result[i].FromObjectID < result[j].FromObjectID
		}
		return result[i].ToObjectID < result[j].ToObjectID
	})
	if result == nil {
		result = []groupingEdgeAuthority{}
	}
	return result
}

func compileIntegrationUsageRequest(
	symbols map[string]symbolAuthority,
	selected integrationdependency.Result,
	uses integrationusage.Result,
) (integrationUsageRequest, error) {
	dependenciesByID := make(map[string]integrationdependency.SelectedDependency, len(selected.Dependencies))
	for _, dependency := range selected.Dependencies {
		dependenciesByID[dependency.Dependency.ID] = dependency
	}
	symbolRefByObjectID := make(map[string]string, len(symbols))
	for ref, symbol := range symbols {
		symbolRefByObjectID[symbol.fact.NodeID] = ref
		symbolRefByObjectID[symbol.programObjectID] = ref
	}
	rows := make([]integrationUseRequest, len(uses.Uses))
	for position, use := range uses.Uses {
		dependency, ok := dependenciesByID[use.Operation.DependencyID]
		if !ok {
			return integrationUsageRequest{}, fmt.Errorf(
				"coremap: integration use %d cites an unselected dependency", position,
			)
		}
		operation := use.Operation
		rows[position] = integrationUseRequest{
			Ref:               fmt.Sprintf("u%d", position+1),
			CallerSymbolRef:   symbolRefByObjectID[operation.CallerID],
			DependencyKind:    dependency.Dependency.Kind,
			DependencyName:    dependency.Dependency.Name,
			DependencyModule:  dependency.Dependency.ModulePath,
			DependencyPackage: dependency.Dependency.PackagePath,
			CallerKind:        operation.CallerKind,
			CallerName:        operation.CallerName,
			CallerLocation:    operation.CallerLocation,
			Callsite:          operation.Callsite,
			CallExpression:    operation.CallExpression,
			CanonicalCallee:   operation.CanonicalCallee,
			Invocation:        operation.Invocation,
			Authority:         operation.Authority,
			Label:             use.Label,
			Mechanism:         use.Mechanism,
		}
	}
	if rows == nil {
		rows = []integrationUseRequest{}
	}
	return integrationUsageRequest{Uses: rows}, nil
}

func programCoreObject(
	object programindex.Object,
	objects map[string]programindex.Object,
) bool {
	switch object.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectType:
		return true
	case programindex.ObjectVariable:
		if object.Visibility != programindex.VisibilityPublic {
			return false
		}
		parentID := object.ContainerID
		if parentID == "" {
			parentID = object.OwnerID
		}
		parent, ok := objects[parentID]
		return ok && (parent.Kind == programindex.ObjectModule || parent.Kind == programindex.ObjectPackage)
	default:
		return false
	}
}

func programObjectContext(
	object programindex.Object,
	objects map[string]programindex.Object,
) (string, string, error) {
	receiver := ""
	if object.Kind == programindex.ObjectMethod && object.OwnerID != "" {
		owner, ok := objects[object.OwnerID]
		if !ok || owner.Kind != programindex.ObjectType {
			return "", "", fmt.Errorf("coremap: method %q has no exact receiver object", object.ID)
		}
		receiver = owner.Name
	}
	seen := make(map[string]struct{})
	current := object
	for {
		if current.Kind == programindex.ObjectModule || current.Kind == programindex.ObjectPackage {
			return current.Name, receiver, nil
		}
		if _, duplicate := seen[current.ID]; duplicate {
			return "", "", fmt.Errorf("coremap: object containment contains a cycle")
		}
		seen[current.ID] = struct{}{}
		nextID := current.ContainerID
		if nextID == "" {
			nextID = current.OwnerID
		}
		if nextID == "" {
			return "", "", fmt.Errorf("coremap: callable object %q has no module context", object.ID)
		}
		next, ok := objects[nextID]
		if !ok {
			return "", "", fmt.Errorf("coremap: callable object %q has unknown container", object.ID)
		}
		current = next
	}
}
