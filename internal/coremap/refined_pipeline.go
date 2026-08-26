package coremap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

const maxRefinedPayloadBytes = 768 << 10

type relationEndpointRequest struct {
	SymbolRef  string                  `json:"symbol_ref,omitempty"`
	Kind       programindex.ObjectKind `json:"kind"`
	Name       string                  `json:"name"`
	Package    string                  `json:"package,omitempty"`
	Visibility programindex.Visibility `json:"visibility"`
	Signature  string                  `json:"signature,omitempty"`
	Location   *programindex.Location  `json:"location,omitempty"`
}

type dynamicRelationRequest struct {
	Ref             string                    `json:"ref"`
	JointRef        string                    `json:"joint_ref"`
	Perspective     string                    `json:"perspective"`
	Kind            programindex.RelationKind `json:"kind"`
	Resolution      programindex.Resolution   `json:"resolution"`
	From            relationEndpointRequest   `json:"from"`
	To              *relationEndpointRequest  `json:"to,omitempty"`
	Invocation      string                    `json:"invocation,omitempty"`
	Location        *programindex.Location    `json:"location,omitempty"`
	TargetsObserved int                       `json:"targets_observed"`
	TargetsRetained int                       `json:"targets_retained"`
	TargetsOmitted  int                       `json:"targets_omitted"`
	TargetOrdinal   int                       `json:"target_ordinal"`
}

type baselineBlockRequest struct {
	Ancestors []string   `json:"ancestors,omitempty"`
	Name      string     `json:"name"`
	Purpose   string     `json:"purpose"`
	Files     []FileFact `json:"files"`
}

type semanticFactRequest struct {
	Ref             string                  `json:"ref"`
	BaselineBlock   *baselineBlockRequest   `json:"baseline_block,omitempty"`
	Symbol          *symbolRequest          `json:"symbol,omitempty"`
	TargetSeed      *targetSeedRequest      `json:"target_seed,omitempty"`
	IntegrationUse  *integrationUseRequest  `json:"integration_use,omitempty"`
	DynamicRelation *dynamicRelationRequest `json:"dynamic_relation,omitempty"`
}

type shardRequest struct {
	Ordinal int `json:"ordinal"`
	Count   int `json:"count"`
}

type refinedMapRequest struct {
	Repository      string                `json:"repository"`
	Target          targetRequest         `json:"target"`
	ProgramCoverage programindex.Coverage `json:"program_coverage"`
	Shard           shardRequest          `json:"shard"`
	Facts           []semanticFactRequest `json:"facts"`
}

type candidateSymbolRequest struct {
	Ref      string                  `json:"ref"`
	Kind     programindex.ObjectKind `json:"kind,omitempty"`
	Package  string                  `json:"package"`
	Name     string                  `json:"name"`
	Receiver string                  `json:"receiver,omitempty"`
	Path     string                  `json:"path"`
	Line     int                     `json:"line"`
	Exported bool                    `json:"exported"`
}

type candidateRequest struct {
	Ref     string                   `json:"ref"`
	Name    string                   `json:"name"`
	Purpose string                   `json:"purpose"`
	Files   []FileFact               `json:"files"`
	Symbols []candidateSymbolRequest `json:"symbols"`
}

type refinedReduceRequest struct {
	Repository      string                `json:"repository"`
	Target          targetRequest         `json:"target"`
	ProgramCoverage programindex.Coverage `json:"program_coverage"`
	Level           int                   `json:"level"`
	Batch           shardRequest          `json:"batch"`
	Candidates      []candidateRequest    `json:"candidates"`
}

type refinedPipelineAccounting struct {
	semanticFacts        int
	dynamicRelationFacts int
	mapCalls             int
	reduceCalls          int
	requests             StageRequestSize
}

type refinedAuthority struct {
	files   map[corpus.FileID]FileFact
	symbols map[string]symbolAuthority
}

type semanticFactWithKey struct {
	key  string
	fact semanticFactRequest
}

func runRefinedPipeline(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	baseline modelResponse,
) (modelResponse, refinedPipelineAccounting, error) {
	facts, dynamicFacts, err := buildSemanticFacts(compilation, baseline)
	if err != nil {
		return modelResponse{}, refinedPipelineAccounting{}, err
	}
	mapRequests, err := packMapRequests(compilation, facts)
	if err != nil {
		return modelResponse{}, refinedPipelineAccounting{}, err
	}
	accounting := refinedPipelineAccounting{
		semanticFacts: len(facts), dynamicRelationFacts: dynamicFacts,
	}
	candidates := make([]proposal, 0)
	for position, request := range mapRequests {
		wire, err := json.Marshal(request)
		if err != nil {
			return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf("coremap: encode refined map request: %w", err)
		}
		authority, err := authorityForFacts(compilation, request.Facts)
		if err != nil {
			return modelResponse{}, refinedPipelineAccounting{}, err
		}
		response, requestSize, err := executeRefinedCall(
			ctx, executor, provider, compilation, "map", 0, position+1, len(mapRequests),
			refinedPrompt, wire, authority,
		)
		if err != nil {
			return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
				"coremap: refined map batch %d/%d: %w", position+1, len(mapRequests), err,
			)
		}
		accounting.mapCalls++
		accounting.requests.add(requestSize)
		candidates = append(candidates, response.Blocks...)
	}
	candidates = deduplicateProposals(candidates)
	if len(candidates) == 0 {
		return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
			"coremap: all %d refined map batches returned legitimate empty results; no target-core block was established",
			len(mapRequests),
		)
	}
	if len(mapRequests) == 1 {
		return modelResponse{Blocks: cloneProposals(candidates)}, accounting, nil
	}

	for level := 1; level <= maxReduceLevels; level++ {
		batches, err := packReduceRequests(compilation, level, candidates)
		if err != nil {
			return modelResponse{}, refinedPipelineAccounting{}, err
		}
		next := make([]proposal, 0)
		for position, request := range batches {
			wire, err := marshalReduceRequest(request)
			if err != nil {
				return modelResponse{}, refinedPipelineAccounting{}, err
			}
			authority, err := authorityForCandidates(compilation, request.Candidates)
			if err != nil {
				return modelResponse{}, refinedPipelineAccounting{}, err
			}
			response, requestSize, err := executeRefinedCall(
				ctx, executor, provider, compilation, "reduce", level, position+1, len(batches),
				reducePrompt, wire, authority,
			)
			if err != nil {
				return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
					"coremap: refined reduce level %d batch %d/%d: %w", level, position+1, len(batches), err,
				)
			}
			accounting.reduceCalls++
			accounting.requests.add(requestSize)
			next = append(next, response.Blocks...)
		}
		next = deduplicateProposals(next)
		if len(next) == 0 {
			return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
				"coremap: refined reduce level %d returned only legitimate empty results", level,
			)
		}
		if len(batches) == 1 {
			return modelResponse{Blocks: cloneProposals(next)}, accounting, nil
		}
		if len(next) > len(candidates) {
			return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
				"coremap: refined reduce level %d expanded %d input candidates into %d output candidates",
				level, len(candidates), len(next),
			)
		}
		if len(next) == len(candidates) {
			beforeBytes, sizeErr := proposalFootprint(candidates)
			if sizeErr != nil {
				return modelResponse{}, refinedPipelineAccounting{}, sizeErr
			}
			afterBytes, sizeErr := proposalFootprint(next)
			if sizeErr != nil {
				return modelResponse{}, refinedPipelineAccounting{}, sizeErr
			}
			if afterBytes >= beforeBytes {
				return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
					"coremap: refined reduce level %d made no count or byte progress (%d candidates, %d bytes); no candidates were sliced",
					level, len(candidates), beforeBytes,
				)
			}
		}
		candidates = next
	}
	return modelResponse{}, refinedPipelineAccounting{}, fmt.Errorf(
		"coremap: refined reduction exceeded %d levels without slicing candidates", maxReduceLevels,
	)
}

func proposalFootprint(values []proposal) (int, error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return 0, fmt.Errorf("coremap: encode refined candidate progress: %w", err)
	}
	return len(encoded), nil
}

func executeRefinedCall(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	phase string,
	level int,
	ordinal int,
	count int,
	prompt string,
	wire []byte,
	authority refinedAuthority,
) (modelResponse, StageRequestSize, error) {
	if len(wire)+len(prompt) > maxRefinedPayloadBytes {
		return modelResponse{}, StageRequestSize{}, fmt.Errorf(
			"%s payload is %d bytes plus prompt, shard limit is %d", phase, len(wire), maxRefinedPayloadBytes,
		)
	}
	call, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageRefined), provider, llm.Call[modelResponse]{
		State:  refinedCallState(compilation, phase, level, ordinal, count, wire, prompt),
		Prompt: llm.Prompt{System: prompt, User: string(wire), ResponseFormatJSON: true},
		Limits: llm.Limits{
			MaxRequestBytes: MaxRequestBytes, MaxResponseBytes: MaxResponseBytes,
			MaxOutputTokens: MaxOutputTokens,
		},
		DecodeValidate: func(raw []byte) (modelResponse, error) {
			response, err := decodeResponse(raw)
			if err == nil {
				response.Blocks = normalizeProposalRefs(response.Blocks, authority.files, authority.symbols)
				err = validateRefinedBatchProposals(response.Blocks, authority.files, authority.symbols, true)
			}
			return response, err
		},
	})
	if err != nil {
		return modelResponse{}, StageRequestSize{}, err
	}
	return call.Value, StageRequestSize{
		Calls: 1, PayloadBytes: len(wire), ProviderBytes: call.RequestBytes,
		MaxPayloadBytes: len(wire), MaxProviderBytes: call.RequestBytes,
	}, nil
}

func (size *StageRequestSize) add(value StageRequestSize) {
	size.Calls += value.Calls
	size.PayloadBytes += value.PayloadBytes
	size.ProviderBytes += value.ProviderBytes
	if value.MaxPayloadBytes > size.MaxPayloadBytes {
		size.MaxPayloadBytes = value.MaxPayloadBytes
	}
	if value.MaxProviderBytes > size.MaxProviderBytes {
		size.MaxProviderBytes = value.MaxProviderBytes
	}
}

func buildSemanticFacts(compilation Compilation, baseline modelResponse) ([]semanticFactRequest, int, error) {
	values := make([]semanticFactWithKey, 0,
		countProposalTree(baseline.Blocks)+len(compilation.symbolRows)+len(compilation.targetSeedRows)+
			len(compilation.dynamicRelationRows),
	)
	var addBaseline func([]proposal, []string) error
	addBaseline = func(blocks []proposal, ancestors []string) error {
		for _, block := range blocks {
			files := make([]FileFact, len(block.FileRefs))
			key := "\xffbaseline\x00" + strings.Join(ancestors, "\x00") + "\x00" + block.Name
			for position, ref := range block.FileRefs {
				file, ok := compilation.baselineFiles[ref]
				if !ok {
					return fmt.Errorf("coremap: baseline fact cites unknown file ref %q", ref)
				}
				files[position] = file
				if position == 0 {
					key = file.Path + "\x00baseline\x00" + block.Name
				}
			}
			row := baselineBlockRequest{
				Ancestors: append([]string(nil), ancestors...), Name: block.Name,
				Purpose: block.Purpose, Files: files,
			}
			values = append(values, semanticFactWithKey{key: key, fact: semanticFactRequest{BaselineBlock: &row}})
			if err := addBaseline(block.Children, append(append([]string(nil), ancestors...), block.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := addBaseline(baseline.Blocks, nil); err != nil {
		return nil, 0, err
	}
	for position := range compilation.symbolRows {
		row := compilation.symbolRows[position]
		values = append(values, semanticFactWithKey{
			key:  row.Path + locationSortSuffix(row.Line, "symbol", row.Ref),
			fact: semanticFactRequest{Symbol: &row},
		})
	}
	for position := range compilation.targetSeedRows {
		row := compilation.targetSeedRows[position]
		values = append(values, semanticFactWithKey{
			key:  row.Location.Path + locationSortSuffix(row.Location.Line, "seed", row.Ref),
			fact: semanticFactRequest{TargetSeed: &row},
		})
	}
	if compilation.integrationUsage != nil {
		for position := range compilation.integrationUsage.Uses {
			row := compilation.integrationUsage.Uses[position]
			values = append(values, semanticFactWithKey{
				key:  row.Callsite.Path + locationSortSuffix(row.Callsite.Line, "integration", row.Ref),
				fact: semanticFactRequest{IntegrationUse: &row},
			})
		}
	}
	dynamicRefs := make(map[string]struct{})
	for position := range compilation.dynamicRelationRows {
		row := compilation.dynamicRelationRows[position]
		dynamicRefs[row.Ref] = struct{}{}
		path, line := dynamicRelationSortLocation(row)
		values = append(values, semanticFactWithKey{
			key:  path + locationSortSuffix(line, "relation", row.Ref+"\x00"+row.Perspective),
			fact: semanticFactRequest{DynamicRelation: &row},
		})
	}
	sort.SliceStable(values, func(left, right int) bool { return values[left].key < values[right].key })
	result := make([]semanticFactRequest, len(values))
	for position := range values {
		result[position] = values[position].fact
		result[position].Ref = fmt.Sprintf("q%d", position+1)
	}
	return result, len(dynamicRefs), nil
}

func locationSortSuffix(line int, kind, ref string) string {
	return fmt.Sprintf("\x00%09d\x00%s\x00%s", line, kind, ref)
}

func dynamicRelationSortLocation(row dynamicRelationRequest) (string, int) {
	if row.Perspective == "to" && row.To != nil && row.To.Location != nil {
		return row.To.Location.Path, row.To.Location.Line
	}
	if row.From.Location != nil {
		return row.From.Location.Path, row.From.Location.Line
	}
	if row.Location != nil {
		return row.Location.Path, row.Location.Line
	}
	return "\xffrelation", 0
}

func packMapRequests(compilation Compilation, facts []semanticFactRequest) ([]refinedMapRequest, error) {
	if len(facts) == 0 {
		return nil, fmt.Errorf("coremap: refined pipeline has no semantic facts")
	}
	groups := make([][]semanticFactRequest, 0)
	current := make([]semanticFactRequest, 0)
	emptyWire, err := json.Marshal(refinedMapRequest{
		Repository: compilation.repository, Target: compilation.baselineRequest.Target,
		ProgramCoverage: compilation.programCoverage,
		Shard:           shardRequest{Ordinal: 99999999, Count: 99999999},
		Facts:           []semanticFactRequest{},
	})
	if err != nil {
		return nil, fmt.Errorf("coremap: encode empty refined map shard: %w", err)
	}
	currentBytes := len(emptyWire)
	for factPosition, fact := range facts {
		factWire, err := json.Marshal(fact)
		if err != nil {
			return nil, fmt.Errorf("coremap: encode refined map fact %q: %w", fact.Ref, err)
		}
		candidateBytes := currentBytes + len(factWire)
		if len(current) > 0 {
			candidateBytes++
		}
		if candidateBytes+len(refinedPrompt) <= maxRefinedPayloadBytes {
			current = append(current, fact)
			currentBytes = candidateBytes
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf(
				"coremap: semantic fact %q (%d/%d) needs %d bytes plus prompt, shard limit is %d; the fact was not truncated",
				fact.Ref, factPosition+1, len(facts), candidateBytes, maxRefinedPayloadBytes,
			)
		}
		groups = append(groups, append([]semanticFactRequest(nil), current...))
		current = []semanticFactRequest{fact}
		currentBytes = len(emptyWire) + len(factWire)
		if currentBytes+len(refinedPrompt) > maxRefinedPayloadBytes {
			return nil, fmt.Errorf(
				"coremap: semantic fact %q (%d/%d) needs %d bytes plus prompt, shard limit is %d; the fact was not truncated",
				fact.Ref, factPosition+1, len(facts), currentBytes, maxRefinedPayloadBytes,
			)
		}
	}
	if len(current) > 0 {
		groups = append(groups, append([]semanticFactRequest(nil), current...))
	}
	requests := make([]refinedMapRequest, len(groups))
	for position, group := range groups {
		requests[position] = refinedMapRequest{
			Repository: compilation.repository, Target: compilation.baselineRequest.Target,
			ProgramCoverage: compilation.programCoverage,
			Shard:           shardRequest{Ordinal: position + 1, Count: len(groups)},
			Facts:           append([]semanticFactRequest(nil), group...),
		}
		wire, err := json.Marshal(requests[position])
		if err != nil {
			return nil, fmt.Errorf("coremap: encode refined map shard: %w", err)
		}
		if len(wire)+len(refinedPrompt) > maxRefinedPayloadBytes {
			return nil, fmt.Errorf("coremap: finalized refined map shard %d exceeds its bound", position+1)
		}
	}
	return requests, nil
}

func packReduceRequests(compilation Compilation, level int, proposals []proposal) ([]refinedReduceRequest, error) {
	if len(proposals) == 0 {
		return nil, fmt.Errorf("coremap: refined reduce level %d has no candidates", level)
	}
	values := make([]candidateRequest, len(proposals))
	for position, proposal := range proposals {
		candidate, err := candidateFromProposal(compilation, proposal)
		if err != nil {
			return nil, err
		}
		values[position] = candidate
	}
	groups := make([][]candidateRequest, 0)
	current := make([]candidateRequest, 0)
	emptyWire, err := marshalReduceRequest(refinedReduceRequest{
		Repository: compilation.repository, Target: compilation.baselineRequest.Target,
		ProgramCoverage: compilation.programCoverage,
		Level:           level, Batch: shardRequest{Ordinal: 99999999, Count: 99999999},
		Candidates: []candidateRequest{},
	})
	if err != nil {
		return nil, err
	}
	currentBytes := len(emptyWire)
	for candidatePosition, value := range values {
		value.Ref = fmt.Sprintf("c%d", len(current)+1)
		valueWire, err := marshalCandidateRequest(value)
		if err != nil {
			return nil, err
		}
		candidateBytes := currentBytes + len(valueWire)
		if len(current) > 0 {
			candidateBytes++
		}
		if candidateBytes+len(reducePrompt) <= maxRefinedPayloadBytes {
			current = append(current, value)
			currentBytes = candidateBytes
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf(
				"coremap: reduce candidate %d/%d needs %d bytes plus prompt, shard limit is %d; the candidate was not truncated",
				candidatePosition+1, len(values), candidateBytes, maxRefinedPayloadBytes,
			)
		}
		groups = append(groups, append([]candidateRequest(nil), current...))
		current = []candidateRequest{value}
		assignCandidateRefs(current)
		valueWire, err = marshalCandidateRequest(current[0])
		if err != nil {
			return nil, err
		}
		currentBytes = len(emptyWire) + len(valueWire)
		if currentBytes+len(reducePrompt) > maxRefinedPayloadBytes {
			return nil, fmt.Errorf(
				"coremap: reduce candidate %d/%d needs %d bytes plus prompt, shard limit is %d; the candidate was not truncated",
				candidatePosition+1, len(values), currentBytes, maxRefinedPayloadBytes,
			)
		}
	}
	if len(current) > 0 {
		groups = append(groups, append([]candidateRequest(nil), current...))
	}
	requests := make([]refinedReduceRequest, len(groups))
	for position, group := range groups {
		assignCandidateRefs(group)
		requests[position] = refinedReduceRequest{
			Repository: compilation.repository, Target: compilation.baselineRequest.Target,
			ProgramCoverage: compilation.programCoverage,
			Level:           level, Batch: shardRequest{Ordinal: position + 1, Count: len(groups)},
			Candidates: group,
		}
		wire, err := marshalReduceRequest(requests[position])
		if err != nil {
			return nil, err
		}
		if len(wire)+len(reducePrompt) > maxRefinedPayloadBytes {
			return nil, fmt.Errorf("coremap: finalized refined reduce batch %d exceeds its bound", position+1)
		}
	}
	return requests, nil
}

func candidateFromProposal(compilation Compilation, value proposal) (candidateRequest, error) {
	files := make([]FileFact, len(value.FileRefs))
	for position, ref := range value.FileRefs {
		file, ok := compilation.files[ref]
		if !ok {
			return candidateRequest{}, fmt.Errorf("coremap: reduce candidate cites unknown file ref %q", ref)
		}
		files[position] = file
	}
	symbols := make([]candidateSymbolRequest, len(value.SymbolRefs))
	for position, ref := range value.SymbolRefs {
		authority, ok := compilation.symbols[ref]
		if !ok {
			return candidateRequest{}, fmt.Errorf("coremap: reduce candidate cites unknown symbol ref %q", ref)
		}
		symbols[position] = candidateSymbolFromAuthority(ref, authority)
	}
	return candidateRequest{
		Name: value.Name, Purpose: value.Purpose, Files: files, Symbols: symbols,
	}, nil
}

func assignCandidateRefs(values []candidateRequest) {
	for position := range values {
		values[position].Ref = fmt.Sprintf("c%d", position+1)
	}
}

func marshalReduceRequest(request refinedReduceRequest) ([]byte, error) {
	type wireCandidate candidateRequest
	type wireRequest struct {
		Repository string          `json:"repository"`
		Target     targetRequest   `json:"target"`
		Level      int             `json:"level"`
		Batch      shardRequest    `json:"batch"`
		Candidates []wireCandidate `json:"candidates"`
	}
	candidates := make([]wireCandidate, len(request.Candidates))
	for position := range request.Candidates {
		candidates[position] = wireCandidate(request.Candidates[position])
	}
	wire, err := json.Marshal(wireRequest{
		Repository: request.Repository, Target: request.Target, Level: request.Level,
		Batch: request.Batch, Candidates: candidates,
	})
	if err != nil {
		return nil, fmt.Errorf("coremap: encode refined reduce request: %w", err)
	}
	return wire, nil
}

func marshalCandidateRequest(value candidateRequest) ([]byte, error) {
	type wireCandidate candidateRequest
	wire, err := json.Marshal(wireCandidate(value))
	if err != nil {
		return nil, fmt.Errorf("coremap: encode refined reduce candidate: %w", err)
	}
	return wire, nil
}

func authorityForFacts(compilation Compilation, facts []semanticFactRequest) (refinedAuthority, error) {
	authority := refinedAuthority{files: make(map[corpus.FileID]FileFact), symbols: make(map[string]symbolAuthority)}
	addSymbol := func(ref string, advertiseFile bool) error {
		if ref == "" {
			return nil
		}
		symbol, ok := compilation.symbols[ref]
		if !ok {
			return fmt.Errorf("coremap: shard advertises unknown symbol ref %q", ref)
		}
		authority.symbols[ref] = symbol
		if advertiseFile {
			file, ok := compilation.files[symbol.request.FileRef]
			if !ok || file.Path != symbol.request.Path {
				return fmt.Errorf("coremap: shard symbol file authority mismatch")
			}
			authority.files[symbol.request.FileRef] = file
		}
		return nil
	}
	for _, fact := range facts {
		switch {
		case fact.BaselineBlock != nil:
			for _, file := range fact.BaselineBlock.Files {
				if exact, ok := compilation.baselineFiles[file.FileRef]; !ok || exact != file {
					return refinedAuthority{}, fmt.Errorf("coremap: shard baseline file authority mismatch")
				}
				authority.files[file.FileRef] = file
			}
		case fact.Symbol != nil:
			if err := addSymbol(fact.Symbol.Ref, true); err != nil {
				return refinedAuthority{}, err
			}
		case fact.TargetSeed != nil:
			if err := addSymbol(fact.TargetSeed.SymbolRef, false); err != nil {
				return refinedAuthority{}, err
			}
		case fact.IntegrationUse != nil:
			if err := addSymbol(fact.IntegrationUse.CallerSymbolRef, false); err != nil {
				return refinedAuthority{}, err
			}
		case fact.DynamicRelation != nil:
			if err := addSymbol(fact.DynamicRelation.From.SymbolRef, false); err != nil {
				return refinedAuthority{}, err
			}
			if fact.DynamicRelation.To != nil {
				if err := addSymbol(fact.DynamicRelation.To.SymbolRef, false); err != nil {
					return refinedAuthority{}, err
				}
			}
		default:
			return refinedAuthority{}, fmt.Errorf("coremap: shard contains an empty semantic fact")
		}
	}
	return authority, nil
}

func authorityForCandidates(compilation Compilation, candidates []candidateRequest) (refinedAuthority, error) {
	authority := refinedAuthority{files: make(map[corpus.FileID]FileFact), symbols: make(map[string]symbolAuthority)}
	for _, candidate := range candidates {
		for _, file := range candidate.Files {
			exact, ok := compilation.files[file.FileRef]
			if !ok || exact != file {
				return refinedAuthority{}, fmt.Errorf("coremap: reduce candidate file authority mismatch")
			}
			authority.files[file.FileRef] = file
		}
		for _, row := range candidate.Symbols {
			exact, ok := compilation.symbols[row.Ref]
			if !ok || row != candidateSymbolFromAuthority(row.Ref, exact) {
				return refinedAuthority{}, fmt.Errorf("coremap: reduce candidate symbol authority mismatch")
			}
			authority.symbols[row.Ref] = exact
		}
	}
	return authority, nil
}

func candidateSymbolFromAuthority(ref string, authority symbolAuthority) candidateSymbolRequest {
	row := authority.request
	return candidateSymbolRequest{
		Ref: ref, Kind: row.Kind, Package: row.Package, Name: row.Name,
		Receiver: row.Receiver, Path: row.Path, Line: row.Line, Exported: row.Exported,
	}
}

func countProposalTree(blocks []proposal) int {
	total := len(blocks)
	for _, block := range blocks {
		total += countProposalTree(block.Children)
	}
	return total
}

func cloneProposal(value proposal) proposal {
	result := proposal{
		Name: value.Name, Purpose: value.Purpose,
		FileRefs:   append([]corpus.FileID(nil), value.FileRefs...),
		SymbolRefs: append([]string(nil), value.SymbolRefs...),
		Children:   make([]proposal, len(value.Children)),
	}
	for position := range value.Children {
		result.Children[position] = cloneProposal(value.Children[position])
	}
	return result
}

func cloneProposals(values []proposal) []proposal {
	result := make([]proposal, len(values))
	for position := range values {
		result[position] = cloneProposal(values[position])
	}
	return result
}
