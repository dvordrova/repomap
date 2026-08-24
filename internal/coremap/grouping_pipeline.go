package coremap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

type groupingRepresentativeRequest struct {
	Kind            programindex.ObjectKind `json:"kind"`
	Package         string                  `json:"package"`
	Name            string                  `json:"name"`
	Exported        bool                    `json:"exported"`
	TargetSeedKinds []programindex.SeedKind `json:"target_seed_kinds,omitempty"`
}

type groupingEffectRequest struct {
	DependencyKind    dependencies.Kind `json:"dependency_kind"`
	DependencyName    string            `json:"dependency_name"`
	DependencyModule  string            `json:"dependency_module,omitempty"`
	DependencyPackage string            `json:"dependency_package"`
	Label             string            `json:"label"`
	Mechanism         string            `json:"mechanism"`
	Authority         string            `json:"authority"`
	Count             int               `json:"count"`
}

type groupingBlockRequest struct {
	Ref             string                          `json:"ref"`
	Name            string                          `json:"name"`
	Purpose         string                          `json:"purpose"`
	Representatives []groupingRepresentativeRequest `json:"representatives"`
	Effects         []groupingEffectRequest         `json:"effects"`
}

type groupingRelationRequest struct {
	LeftRef                 string `json:"left_ref"`
	RightRef                string `json:"right_ref"`
	SharedRepresentatives   int    `json:"shared_representatives"`
	LeftReachesRightMinHops *int   `json:"left_reaches_right_min_hops,omitempty"`
	RightReachesLeftMinHops *int   `json:"right_reaches_left_min_hops,omitempty"`
}

type groupingTargetSeedRequest struct {
	Kind       programindex.SeedKind   `json:"kind"`
	ObjectKind programindex.ObjectKind `json:"object_kind"`
	Name       string                  `json:"name"`
	BlockRefs  []string                `json:"block_refs"`
}

type groupingRequest struct {
	Repository      string                      `json:"repository"`
	Target          targetRequest               `json:"target"`
	ProgramCoverage programindex.Coverage       `json:"program_coverage"`
	Blocks          []groupingBlockRequest      `json:"blocks"`
	Relations       []groupingRelationRequest   `json:"relations"`
	TargetSeeds     []groupingTargetSeedRequest `json:"target_seeds"`
}

type groupProposal struct {
	Name      string   `json:"name"`
	Purpose   string   `json:"purpose"`
	BlockRefs []string `json:"block_refs"`
}

type groupingResponse struct {
	Groups []groupProposal `json:"groups"`
}

type groupingEffectKey struct {
	dependencyKind    dependencies.Kind
	dependencyName    string
	dependencyModule  string
	dependencyPackage string
	label             string
	mechanism         string
	authority         string
}

func runRefinedGrouping(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	blocks []Block,
) ([]Group, StageRequestSize, int, error) {
	if len(blocks) < 2 {
		return []Group{}, StageRequestSize{}, 0, nil
	}
	request, allowed, err := buildGroupingRequest(compilation, blocks)
	if err != nil {
		return nil, StageRequestSize{}, 0, err
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return nil, StageRequestSize{}, 0, fmt.Errorf("encode request: %w", err)
	}
	if len(wire)+len(groupPrompt) > maxRefinedPayloadBytes {
		return nil, StageRequestSize{}, 0, fmt.Errorf(
			"request is %d bytes plus prompt, limit is %d; grouping evidence was not truncated",
			len(wire), maxRefinedPayloadBytes,
		)
	}
	call, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageRefined), provider, llm.Call[groupingResponse]{
		State:  refinedCallState(compilation, "group", 0, 1, 1, wire, groupPrompt),
		Prompt: llm.Prompt{System: groupPrompt, User: string(wire), ResponseFormatJSON: true},
		Limits: llm.Limits{
			MaxRequestBytes: MaxRequestBytes, MaxResponseBytes: MaxResponseBytes,
			MaxOutputTokens: MaxOutputTokens,
		},
		DecodeValidate: func(raw []byte) (groupingResponse, error) {
			response, decodeErr := decodeGroupingResponse(raw)
			if decodeErr == nil {
				decodeErr = validateGroupingResponse(response, allowed)
			}
			return response, decodeErr
		},
	})
	if err != nil {
		return nil, StageRequestSize{}, 0, fmt.Errorf("model cube: %w", err)
	}
	groups, err := restoreRefinedGroups(call.Value, allowed, compilationTargetKey(compilation))
	if err != nil {
		return nil, StageRequestSize{}, 0, err
	}
	if err := validateRefinedGroups(groups, blocks, compilationTargetKey(compilation)); err != nil {
		return nil, StageRequestSize{}, 0, err
	}
	return groups, StageRequestSize{
		Calls: 1, PayloadBytes: len(wire), ProviderBytes: call.RequestBytes,
		MaxPayloadBytes: len(wire), MaxProviderBytes: call.RequestBytes,
	}, 1, nil
}

func buildGroupingRequest(
	compilation Compilation,
	blocks []Block,
) (groupingRequest, map[string]Block, error) {
	allowed := make(map[string]Block, len(blocks))
	blockRefsBySymbol := make(map[string][]string)
	programObjectsByBlock := make(map[string]map[string]struct{}, len(blocks))
	requestBlocks := make([]groupingBlockRequest, len(blocks))

	symbolRefByNodeID := make(map[string]string, len(compilation.symbols))
	for ref, authority := range compilation.symbols {
		symbolRefByNodeID[authority.fact.NodeID] = ref
	}
	for position, block := range blocks {
		ref := fmt.Sprintf("b%d", position+1)
		allowed[ref] = block
		programObjectsByBlock[ref] = make(map[string]struct{}, len(block.Symbols))
		representatives := make([]groupingRepresentativeRequest, len(block.Symbols))
		for symbolPosition, symbol := range block.Symbols {
			symbolRef := symbolRefByNodeID[symbol.NodeID]
			authority, ok := compilation.symbols[symbolRef]
			if !ok || authority.programObjectID == "" {
				return groupingRequest{}, nil, fmt.Errorf("block %q has no grouping symbol authority", block.ID)
			}
			blockRefsBySymbol[symbolRef] = append(blockRefsBySymbol[symbolRef], ref)
			programObjectsByBlock[ref][authority.programObjectID] = struct{}{}
			representatives[symbolPosition] = groupingRepresentativeRequest{
				Kind: symbol.Kind, Package: symbol.Package, Name: symbol.Symbol.Name,
				Exported:        symbol.Exported,
				TargetSeedKinds: append([]programindex.SeedKind(nil), symbol.TargetSeedKinds...),
			}
		}
		requestBlocks[position] = groupingBlockRequest{
			Ref: ref, Name: block.Name, Purpose: block.Purpose,
			Representatives: representatives, Effects: []groupingEffectRequest{},
		}
	}

	effectsByBlock := make(map[string]map[groupingEffectKey]int, len(blocks))
	if compilation.integrationUsage != nil {
		for _, use := range compilation.integrationUsage.Uses {
			for _, blockRef := range blockRefsBySymbol[use.CallerSymbolRef] {
				if effectsByBlock[blockRef] == nil {
					effectsByBlock[blockRef] = make(map[groupingEffectKey]int)
				}
				key := groupingEffectKey{
					dependencyKind: use.DependencyKind, dependencyName: use.DependencyName,
					dependencyModule: use.DependencyModule, dependencyPackage: use.DependencyPackage,
					label: use.Label, mechanism: use.Mechanism, authority: use.Authority,
				}
				effectsByBlock[blockRef][key]++
			}
		}
	}
	for position := range requestBlocks {
		counts := effectsByBlock[requestBlocks[position].Ref]
		for key, count := range counts {
			requestBlocks[position].Effects = append(requestBlocks[position].Effects, groupingEffectRequest{
				DependencyKind: key.dependencyKind, DependencyName: key.dependencyName,
				DependencyModule: key.dependencyModule, DependencyPackage: key.dependencyPackage,
				Label: key.label, Mechanism: key.mechanism, Authority: key.authority, Count: count,
			})
		}
		sort.Slice(requestBlocks[position].Effects, func(i, j int) bool {
			left := requestBlocks[position].Effects[i]
			right := requestBlocks[position].Effects[j]
			return groupingEffectSortKey(left) < groupingEffectSortKey(right)
		})
	}

	adjacency := groupingAdjacency(compilation.groupingEdges)
	relations := make([]groupingRelationRequest, 0, len(blocks)*(len(blocks)-1)/2)
	for left := 0; left < len(requestBlocks); left++ {
		for right := left + 1; right < len(requestBlocks); right++ {
			leftRef := requestBlocks[left].Ref
			rightRef := requestBlocks[right].Ref
			relations = append(relations, groupingRelationRequest{
				LeftRef: leftRef, RightRef: rightRef,
				SharedRepresentatives:   sharedGroupingObjects(programObjectsByBlock[leftRef], programObjectsByBlock[rightRef]),
				LeftReachesRightMinHops: minGroupingHops(adjacency, programObjectsByBlock[leftRef], programObjectsByBlock[rightRef]),
				RightReachesLeftMinHops: minGroupingHops(adjacency, programObjectsByBlock[rightRef], programObjectsByBlock[leftRef]),
			})
		}
	}

	seeds := make([]groupingTargetSeedRequest, len(compilation.targetSeedRows))
	for position, seed := range compilation.targetSeedRows {
		blockRefs := append([]string(nil), blockRefsBySymbol[seed.SymbolRef]...)
		if blockRefs == nil {
			blockRefs = []string{}
		}
		seeds[position] = groupingTargetSeedRequest{
			Kind: seed.Kind, ObjectKind: seed.ObjectKind, Name: seed.Name, BlockRefs: blockRefs,
		}
	}

	return groupingRequest{
		Repository: compilation.repository, Target: compilation.baselineRequest.Target,
		ProgramCoverage: compilation.programCoverage,
		Blocks:          requestBlocks, Relations: relations, TargetSeeds: seeds,
	}, allowed, nil
}

func groupingAdjacency(edges []groupingEdgeAuthority) map[string][]string {
	result := make(map[string][]string)
	for _, edge := range edges {
		result[edge.FromObjectID] = append(result[edge.FromObjectID], edge.ToObjectID)
	}
	return result
}

func minGroupingHops(
	adjacency map[string][]string,
	sources map[string]struct{},
	targets map[string]struct{},
) *int {
	if len(sources) == 0 || len(targets) == 0 {
		return nil
	}
	visited := make(map[string]struct{}, len(sources))
	frontier := make([]string, 0, len(sources))
	for source := range sources {
		visited[source] = struct{}{}
		frontier = append(frontier, source)
	}
	for depth := 1; len(frontier) > 0; depth++ {
		next := make([]string, 0)
		for _, node := range frontier {
			for _, candidate := range adjacency[node] {
				if _, seen := visited[candidate]; seen {
					continue
				}
				if _, reached := targets[candidate]; reached {
					value := depth
					return &value
				}
				visited[candidate] = struct{}{}
				next = append(next, candidate)
			}
		}
		frontier = next
	}
	return nil
}

func sharedGroupingObjects(left, right map[string]struct{}) int {
	if len(left) > len(right) {
		left, right = right, left
	}
	total := 0
	for objectID := range left {
		if _, ok := right[objectID]; ok {
			total++
		}
	}
	return total
}

func groupingEffectSortKey(value groupingEffectRequest) string {
	return strings.Join([]string{
		string(value.DependencyKind), value.DependencyModule, value.DependencyPackage,
		value.DependencyName, value.Mechanism, value.Label, value.Authority,
	}, "\x00")
}

func decodeGroupingResponse(raw []byte) (groupingResponse, error) {
	var response groupingResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return groupingResponse{}, fmt.Errorf("decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return groupingResponse{}, fmt.Errorf("response contains trailing JSON")
	}
	return response, nil
}

func validateGroupingResponse(value groupingResponse, allowed map[string]Block) error {
	if value.Groups == nil {
		return fmt.Errorf("groups must be an exact JSON array")
	}
	if len(value.Groups) == 0 {
		return nil
	}
	if len(value.Groups) < 2 || len(value.Groups) > len(allowed) {
		return fmt.Errorf("response has invalid group count")
	}
	seenRefs := make(map[string]struct{}, len(allowed))
	seenNames := make(map[string]struct{}, len(value.Groups))
	for _, group := range value.Groups {
		if !validText(group.Name, maxNameBytes) || !validText(group.Purpose, maxPurposeBytes) || len(group.BlockRefs) == 0 {
			return fmt.Errorf("response has invalid group")
		}
		if _, duplicate := seenNames[group.Name]; duplicate {
			return fmt.Errorf("response repeats group name")
		}
		seenNames[group.Name] = struct{}{}
		inside := make(map[string]struct{}, len(group.BlockRefs))
		for _, ref := range group.BlockRefs {
			if _, ok := allowed[ref]; !ok {
				return fmt.Errorf("response cites unknown block ref %q", ref)
			}
			if _, duplicate := inside[ref]; duplicate {
				return fmt.Errorf("response repeats block ref %q inside one group", ref)
			}
			inside[ref] = struct{}{}
			if _, duplicate := seenRefs[ref]; duplicate {
				return fmt.Errorf("response assigns block ref %q to several groups", ref)
			}
			seenRefs[ref] = struct{}{}
		}
	}
	if len(seenRefs) != len(allowed) {
		return fmt.Errorf("response grouping is not a complete block partition")
	}
	return nil
}

func restoreRefinedGroups(
	value groupingResponse,
	allowed map[string]Block,
	targetKey string,
) ([]Group, error) {
	result := make([]Group, len(value.Groups))
	for position, proposal := range value.Groups {
		blockIDs := make([]string, len(proposal.BlockRefs))
		for memberPosition, ref := range proposal.BlockRefs {
			block, ok := allowed[ref]
			if !ok {
				return nil, fmt.Errorf("restore cites unknown block ref %q", ref)
			}
			blockIDs[memberPosition] = block.ID
		}
		result[position] = Group{
			Name: proposal.Name, Purpose: proposal.Purpose, BlockIDs: blockIDs,
		}
		result[position].ID = stableGroupID(targetKey, result[position].BlockIDs)
	}
	return result, nil
}

func validateRefinedGroups(groups []Group, blocks []Block, targetKey string) error {
	if groups == nil {
		return fmt.Errorf("coremap: refined groups must retain an exact array")
	}
	if len(groups) == 0 {
		return nil
	}
	if len(groups) < 2 || len(groups) > len(blocks) {
		return fmt.Errorf("coremap: invalid refined group count")
	}
	allowed := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		allowed[block.ID] = struct{}{}
	}
	seenBlocks := make(map[string]struct{}, len(blocks))
	seenGroups := make(map[string]struct{}, len(groups))
	seenNames := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if !validText(group.ID, 128) || !validText(group.Name, maxNameBytes) ||
			!validText(group.Purpose, maxPurposeBytes) || len(group.BlockIDs) == 0 ||
			group.ID != stableGroupID(targetKey, group.BlockIDs) {
			return fmt.Errorf("coremap: invalid refined group")
		}
		if _, duplicate := seenGroups[group.ID]; duplicate {
			return fmt.Errorf("coremap: duplicate refined group identity")
		}
		seenGroups[group.ID] = struct{}{}
		if _, duplicate := seenNames[group.Name]; duplicate {
			return fmt.Errorf("coremap: duplicate refined group name")
		}
		seenNames[group.Name] = struct{}{}
		inside := make(map[string]struct{}, len(group.BlockIDs))
		for _, blockID := range group.BlockIDs {
			if _, ok := allowed[blockID]; !ok {
				return fmt.Errorf("coremap: refined group cites unknown block %q", blockID)
			}
			if _, duplicate := inside[blockID]; duplicate {
				return fmt.Errorf("coremap: refined group repeats block %q", blockID)
			}
			inside[blockID] = struct{}{}
			if _, duplicate := seenBlocks[blockID]; duplicate {
				return fmt.Errorf("coremap: refined block belongs to several groups")
			}
			seenBlocks[blockID] = struct{}{}
		}
	}
	if len(seenBlocks) != len(blocks) {
		return fmt.Errorf("coremap: refined groups are not a complete block partition")
	}
	return nil
}

func stableGroupID(targetKey string, blockIDs []string) string {
	keys := append([]string(nil), blockIDs...)
	sort.Strings(keys)
	digest := sha256.Sum256([]byte("coremap-group-v1\x00" + targetKey + "\x00" + strings.Join(keys, "\x00")))
	return "core-group-" + hex.EncodeToString(digest[:8])
}
