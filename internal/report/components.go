package report

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/evidenceref"
)

const (
	anchorGroundingPath         = "verified_path"
	anchorGroundingLine         = "verified_line"
	anchorGroundingDirection    = "verified_direction_path"
	maxDirectionFallbackAnchors = 4
)

func buildComponents(data *ReportData) {
	data.Components = make([]Component, 0, len(data.HighLevelMap))
	for index, subsystem := range data.HighLevelMap {
		role, _ := componentmap.Normalize(string(subsystem.Role))
		roleBasis := evidence.CertaintyUnknown
		if role != componentmap.RoleUnknown {
			roleBasis = evidence.CertaintyHypothesis
		}
		component := Component{
			ID:           stableReportID("component", subsystem.Name, strconv.Itoa(index)),
			Name:         subsystem.Name,
			Role:         role,
			RoleBasis:    roleBasis,
			ModelPurpose: subsystem.WhyItMatters,
		}
		component.AnchorGroups = buildAnchorGroups(data, component.ID, subsystem.Evidence)
		component.RelatedFlowIDs = relatedFlowIDs(data, component.AnchorGroups)
		if !hasSymbolAnchor(component.AnchorGroups) {
			if direction := preferredComponentDirection(data, component, component.RelatedFlowIDs); direction != nil {
				component.AnchorGroups = append(
					component.AnchorGroups,
					directionFallbackAnchors(data, component.ID, component, *direction)...,
				)
				if direction.ID != "" && !containsString(component.RelatedFlowIDs, direction.ID) {
					component.RelatedFlowIDs = append(component.RelatedFlowIDs, direction.ID)
				}
			}
		}
		for _, flowID := range relatedFlowIDs(data, component.AnchorGroups) {
			if !containsString(component.RelatedFlowIDs, flowID) {
				component.RelatedFlowIDs = append(component.RelatedFlowIDs, flowID)
			}
		}
		for _, anchor := range component.AnchorGroups {
			pkg := repositoryPackageForFile(data.RepositoryGraph, anchor.Path)
			if pkg == "" || containsString(component.Packages, pkg) {
				continue
			}
			component.Packages = append(component.Packages, pkg)
			if component.PrimaryPackage == "" {
				component.PrimaryPackage = pkg
			}
		}
		data.Components = append(data.Components, component)
	}
	data.ComponentRelations = buildComponentRelations(data)
}

func hasSymbolAnchor(anchors []AnchorGroup) bool {
	for _, anchor := range anchors {
		if anchor.CanListSymbols {
			return true
		}
	}
	return false
}

func preferredComponentDirection(data *ReportData, component Component, relatedIDs []string) *CandidateDirection {
	for _, relatedID := range relatedIDs {
		for index := range data.CandidateDirections {
			if data.CandidateDirections[index].ID == relatedID {
				return &data.CandidateDirections[index]
			}
		}
	}
	componentTokens := semanticTokens(component.Name)
	bestIndex := -1
	bestScore := 0
	for index := range data.CandidateDirections {
		score := sharedTokenScore(componentTokens, semanticTokens(data.CandidateDirections[index].Name))
		if score > bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	if bestIndex < 0 {
		return nil
	}
	return &data.CandidateDirections[bestIndex]
}

func directionFallbackAnchors(
	data *ReportData,
	componentID string,
	component Component,
	direction CandidateDirection,
) []AnchorGroup {
	type candidate struct {
		path     string
		score    int
		tier     int
		seed     bool
		position int
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		openable[filePath] = struct{}{}
	}
	componentTerms := semanticTokens(component.Name)
	directionTerms := semanticTokens(direction.Name)
	seen := make(map[string]struct{})
	var candidates []candidate
	position := 0
	add := func(filePath string, seed bool) {
		if _, duplicate := seen[filePath]; duplicate {
			return
		}
		seen[filePath] = struct{}{}
		if _, ok := openable[filePath]; !ok || !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
			return
		}
		if path.Base(filePath) == "main.go" {
			return
		}
		semantic := 2*pathSemanticScore(filePath, componentTerms) + pathSemanticScore(filePath, directionTerms)
		if semantic == 0 {
			return
		}
		score := semantic*10 + repositoryPathPreference(filePath)
		if seed {
			score += 3
		}
		candidates = append(candidates, candidate{
			path: filePath, score: score, tier: repositoryPathTier(filePath), seed: seed, position: position,
		})
		position++
	}
	add(direction.LikelyEntrypoint, true)
	for _, filePath := range direction.LikelyFiles {
		add(filePath, true)
	}
	for _, statement := range direction.Evidence {
		for _, location := range evidenceref.Extract(statement, data.OpenablePaths) {
			add(location.Path, true)
		}
	}
	for _, flow := range data.Flows {
		if flow.ID != direction.ID {
			continue
		}
		for _, file := range flow.BundleFiles {
			add(file.Path, false)
		}
		break
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].tier != candidates[j].tier {
			return candidates[i].tier < candidates[j].tier
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].seed != candidates[j].seed {
			return candidates[i].seed
		}
		return candidates[i].position < candidates[j].position
	})
	if len(candidates) > maxDirectionFallbackAnchors {
		candidates = candidates[:maxDirectionFallbackAnchors]
	}
	anchors := make([]AnchorGroup, 0, len(candidates))
	for _, candidate := range candidates {
		anchor := AnchorGroup{
			ID:             stableReportID("anchor", componentID, candidate.path),
			Path:           candidate.path,
			Grounding:      anchorGroundingDirection,
			ModelNotes:     []string{"Selected from related direction: " + direction.Name},
			CanListSymbols: strings.HasSuffix(candidate.path, ".go") && !strings.HasSuffix(candidate.path, "_test.go"),
		}
		anchor.LocalContext = sourceContextForAnchor(data.sourceSignals, anchor)
		anchors = append(anchors, anchor)
	}
	return anchors
}

func semanticTokens(value string) []string {
	stop := map[string]struct{}{
		"and": {}, "api": {}, "core": {}, "engine": {}, "flow": {}, "for": {},
		"framework": {}, "layer": {}, "management": {}, "module": {}, "operations": {},
		"request": {}, "service": {}, "system": {}, "the": {}, "tool": {},
	}
	seen := make(map[string]struct{})
	var tokens []string
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len(token) < 3 {
			continue
		}
		if _, ignored := stop[token]; ignored {
			continue
		}
		if _, duplicate := seen[token]; duplicate {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
		if token == "grpc" {
			if _, duplicate := seen["rpc"]; !duplicate {
				seen["rpc"] = struct{}{}
				tokens = append(tokens, "rpc")
			}
		}
	}
	return tokens
}

func sharedTokenScore(left, right []string) int {
	rightSet := make(map[string]struct{}, len(right))
	for _, token := range right {
		rightSet[token] = struct{}{}
	}
	score := 0
	for _, token := range left {
		if _, ok := rightSet[token]; ok {
			score++
		}
	}
	return score
}

func pathSemanticScore(filePath string, terms []string) int {
	segments := strings.FieldsFunc(strings.ToLower(filePath), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	base := strings.TrimSuffix(strings.ToLower(path.Base(filePath)), strings.ToLower(path.Ext(filePath)))
	score := 0
	for _, term := range terms {
		matched := false
		for _, segment := range segments {
			if segment == term || (len(term) >= 3 && strings.Contains(segment, term)) {
				matched = true
				break
			}
		}
		if matched {
			score++
			if base == term {
				score++
			}
		}
	}
	return score
}

func repositoryPathPreference(filePath string) int {
	segments := strings.Split(strings.ToLower(filePath), "/")
	score := 1
	for _, segment := range segments[:max(0, len(segments)-1)] {
		switch segment {
		case "server":
			score += 2
		case "api":
			score += 2
		}
	}
	if path.Base(filePath) == "main.go" {
		score -= 4
	}
	return score
}

func repositoryPathTier(filePath string) int {
	tier := 0
	segments := strings.Split(strings.ToLower(filePath), "/")
	for _, segment := range segments[:max(0, len(segments)-1)] {
		switch segment {
		case "bench", "benchmark", "contrib", "example", "examples", "test", "testdata", "tests", "tool", "tools", "vendor":
			return 2
		case "client", "proxy":
			tier = 1
		}
	}
	return tier
}

func buildAnchorGroups(data *ReportData, componentID string, statements []string) []AnchorGroup {
	type anchorBuilder struct {
		group     AnchorGroup
		lineSet   map[int]struct{}
		noteSet   map[string]struct{}
		firstSeen int
	}
	builders := make(map[string]*anchorBuilder)
	order := 0
	for _, statement := range statements {
		for _, location := range evidenceref.Extract(statement, data.OpenablePaths) {
			builder := builders[location.Path]
			if builder == nil {
				builder = &anchorBuilder{
					group: AnchorGroup{
						ID:             stableReportID("anchor", componentID, location.Path),
						Path:           location.Path,
						Grounding:      anchorGroundingPath,
						CanListSymbols: strings.HasSuffix(location.Path, ".go") && !strings.HasSuffix(location.Path, "_test.go"),
					},
					lineSet:   make(map[int]struct{}),
					noteSet:   make(map[string]struct{}),
					firstSeen: order,
				}
				order++
				builders[location.Path] = builder
			}
			if statement != "" {
				if _, ok := builder.noteSet[statement]; !ok {
					builder.noteSet[statement] = struct{}{}
					builder.group.ModelNotes = append(builder.group.ModelNotes, statement)
				}
			}
			if location.Line > 0 {
				if _, ok := builder.lineSet[location.Line]; !ok {
					builder.lineSet[location.Line] = struct{}{}
					builder.group.Locations = append(builder.group.Locations, location)
					builder.group.Grounding = anchorGroundingLine
				}
			}
		}
	}

	ordered := make([]*anchorBuilder, 0, len(builders))
	for _, builder := range builders {
		sort.Slice(builder.group.Locations, func(i, j int) bool {
			return builder.group.Locations[i].Line < builder.group.Locations[j].Line
		})
		builder.group.LocalContext = sourceContextForAnchor(data.sourceSignals, builder.group)
		ordered = append(ordered, builder)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].firstSeen < ordered[j].firstSeen })
	groups := make([]AnchorGroup, 0, len(ordered))
	for _, builder := range ordered {
		groups = append(groups, builder.group)
	}
	return groups
}

func sourceContextForAnchor(signals []SourceSignal, anchor AnchorGroup) []SourceSignal {
	lineSet := make(map[int]struct{}, len(anchor.Locations))
	for _, location := range anchor.Locations {
		lineSet[location.Line] = struct{}{}
	}
	var exact []SourceSignal
	var nearby []SourceSignal
	for _, signal := range signals {
		if signal.Path != anchor.Path {
			continue
		}
		if _, ok := lineSet[signal.Line]; ok {
			exact = append(exact, signal)
		} else {
			nearby = append(nearby, signal)
		}
	}
	result := append([]SourceSignal(nil), exact...)
	if len(result) == 0 {
		result = append(result, nearby...)
	}
	if len(result) > 3 {
		result = result[:3]
	}
	return result
}

func relatedFlowIDs(data *ReportData, anchors []AnchorGroup) []string {
	anchorPaths := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		anchorPaths[anchor.Path] = struct{}{}
	}
	var ids []string
	for _, direction := range data.CandidateDirections {
		paths := append([]string{direction.LikelyEntrypoint}, direction.LikelyFiles...)
		for _, statement := range direction.Evidence {
			for _, location := range evidenceref.Extract(statement, data.OpenablePaths) {
				paths = append(paths, location.Path)
			}
		}
		matched := false
		for _, candidate := range paths {
			if _, ok := anchorPaths[candidate]; ok {
				matched = true
				break
			}
		}
		if matched && direction.ID != "" && !containsString(ids, direction.ID) {
			ids = append(ids, direction.ID)
		}
	}
	return ids
}

func buildComponentRelations(data *ReportData) []ComponentRelation {
	if data.RepositoryGraph == nil {
		return nil
	}
	packageOwners := make(map[string][]int)
	for componentIndex, component := range data.Components {
		seen := make(map[string]struct{}, len(component.Packages))
		for _, packagePath := range component.Packages {
			if packagePath == "" {
				continue
			}
			if _, duplicate := seen[packagePath]; duplicate {
				continue
			}
			seen[packagePath] = struct{}{}
			packageOwners[packagePath] = append(packageOwners[packagePath], componentIndex)
		}
	}
	edges := append([]EdgeInfo(nil), data.RepositoryGraph.PackageEdges...)
	for _, flow := range data.Flows {
		edges = append(edges, flow.BundleEdges...)
	}
	edgeSet := make(map[string]EdgeInfo)
	for _, edge := range edges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			continue
		}
		edgeSet[edge.From+"\x00"+edge.To] = edge
	}
	var edgeKeys []string
	for key := range edgeSet {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Strings(edgeKeys)

	relationByKey := make(map[string]*ComponentRelation)
	var relationOrder []string
	for _, edgeKey := range edgeKeys {
		edge := edgeSet[edgeKey]
		fromOwners := packageOwners[edge.From]
		toOwners := packageOwners[edge.To]
		if len(fromOwners) != 1 || len(toOwners) != 1 {
			continue
		}
		from := data.Components[fromOwners[0]]
		to := data.Components[toOwners[0]]
		if from.ID == to.ID {
			continue
		}
		key := from.ID + "\x00" + to.ID
		relation := relationByKey[key]
		if relation == nil {
			relation = &ComponentRelation{
				From:      from.ID,
				To:        to.ID,
				Kind:      "package_import",
				Certainty: evidence.CertaintyStatic,
			}
			relationByKey[key] = relation
			relationOrder = append(relationOrder, key)
		}
		relation.Evidence = append(relation.Evidence, edge)
	}
	relations := make([]ComponentRelation, 0, len(relationOrder))
	for _, key := range relationOrder {
		relations = append(relations, *relationByKey[key])
	}
	return relations
}

func repositoryPackageForFile(graph *RepositoryGraph, filePath string) string {
	if graph == nil || filePath == "" {
		return ""
	}
	dir := path.Dir(filePath)
	if dir == "." {
		dir = ""
	}
	if len(graph.Packages) > 0 {
		for _, pkg := range graph.Packages {
			if pkg.Dir == dir {
				return pkg.CanonicalPath
			}
		}
		return ""
	}
	var best *ModuleInfo
	for index := range graph.Modules {
		module := &graph.Modules[index]
		if module.Path == "" || (module.Dir != "" && dir != module.Dir && !strings.HasPrefix(dir, module.Dir+"/")) {
			continue
		}
		if best == nil || len(module.Dir) > len(best.Dir) {
			best = module
		}
	}
	if best == nil {
		return ""
	}
	relative := dir
	if best.Dir != "" {
		relative = strings.TrimPrefix(strings.TrimPrefix(dir, best.Dir), "/")
	}
	if relative == "" {
		return best.Path
	}
	return strings.TrimSuffix(best.Path, "/") + "/" + relative
}

func stableReportID(prefix string, parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "-" + hex.EncodeToString(hash[:6])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
