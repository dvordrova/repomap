package orient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

const (
	orientationReferenceCatalogVersion = "orientation-reference-catalog-v1"
	orientationResponseContractVersion = "orientation-reference-response-v1"
	orientationCacheContractVersion    = "orientation-stage-cache-v1"
)

type orientationReferenceCatalog struct {
	digest           string
	filesByRef       map[string]orientationFileReference
	fileRefByPath    map[string]string
	evidenceByRef    map[string]orientationEvidenceReference
	evidenceRefByKey map[string]string
	canonicalJSON    []byte
}

type orientationFileReference struct {
	Ref         string `json:"ref"`
	Path        string `json:"path"`
	CandidateID string `json:"candidate_id,omitempty"`
}

type orientationEvidenceReference struct {
	Ref       string `json:"ref"`
	Kind      string `json:"kind"`
	Statement string `json:"statement"`
}

// orientationWireBundle replaces duplicate repository-bearing identities in
// the existing bounded bundle. Paths occur only in file_index;
// every other fact points back with a request-local file or evidence ref.
type orientationWireBundle struct {
	RepoName               string                         `json:"repo_name"`
	ReadmeExcerpt          string                         `json:"readme_excerpt"`
	TopLevelDirectoryStats any                            `json:"top_level_directory_stats"`
	LanguageHints          any                            `json:"language_hints"`
	Go                     orientationWireGo              `json:"go"`
	KnownDocRefs           []string                       `json:"known_doc_file_refs"`
	FileIndex              []orientationWireFile          `json:"file_index"`
	CandidateFileIndex     []orientationWireCandidateFile `json:"candidate_file_index"`
	SourceSignals          []orientationWireSourceSignal  `json:"source_signals,omitempty"`
	Warnings               []string                       `json:"warnings,omitempty"`
	PolicyVersion          string                         `json:"research_policy_version,omitempty"`
	LocalAuthorizedFiles   int                            `json:"local_authorized_file_count,omitempty"`
}

type orientationWireGo struct {
	ModulesCount          int                                   `json:"modules_count"`
	PackagesCount         int                                   `json:"packages_count"`
	ModuleSummaries       any                                   `json:"module_summaries"`
	Entrypoints           []orientationWireEntrypoint           `json:"entrypoints"`
	CommandTraces         []orientationWireCommandTrace         `json:"command_traces,omitempty"`
	OrientationCandidates []orientationWireOrientationCandidate `json:"orientation_candidates"`
	ImportantEdges        []orientationWireEdge                 `json:"important_edges"`
}

type orientationWireCandidateFile struct {
	FileRef     string   `json:"file_ref"`
	Kind        string   `json:"kind"`
	Signals     []string `json:"signals"`
	Score       int      `json:"score"`
	Reasons     []string `json:"reasons"`
	EvidenceRef string   `json:"evidence_ref"`
}

type orientationWireFile struct {
	FileRef string `json:"file_ref"`
	Path    string `json:"path"`
}

type orientationWireSourceSignal struct {
	EvidenceRef string `json:"evidence_ref"`
	FileRef     string `json:"file_ref"`
	Line        int    `json:"line"`
	Category    string `json:"category"`
	Match       string `json:"match"`
	Snippet     string `json:"snippet"`
	Weight      int    `json:"weight,omitempty"`
	Penalty     int    `json:"penalty,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type orientationWireEntrypoint struct {
	Kind         string                  `json:"kind"`
	ImportPath   string                  `json:"import_path"`
	PackageDir   string                  `json:"package_dir"`
	Anchors      []orientationWireAnchor `json:"anchors,omitempty"`
	OpenFileRefs []string                `json:"open_file_refs"`
}

type orientationWireAnchor struct {
	EvidenceRef string `json:"evidence_ref"`
	Version     int    `json:"version"`
	Kind        string `json:"kind"`
	FileRef     string `json:"file_ref"`
	Line        int    `json:"line"`
}

type orientationWireCommandTrace struct {
	Version           int                          `json:"version"`
	Framework         string                       `json:"framework"`
	EntrypointPackage string                       `json:"entrypoint_package"`
	Command           string                       `json:"command"`
	Steps             []orientationWireCommandStep `json:"steps"`
	HandlerCalls      []orientationWireCommandCall `json:"handler_calls,omitempty"`
	Concurrency       string                       `json:"concurrency"`
	Complete          bool                         `json:"complete"`
	Missing           []string                     `json:"missing,omitempty"`
}

type orientationWireCommandStep struct {
	TargetEvidenceRef   string                   `json:"target_evidence_ref"`
	CallsiteEvidenceRef string                   `json:"callsite_evidence_ref,omitempty"`
	Symbol              string                   `json:"symbol"`
	Relation            string                   `json:"relation"`
	CallsiteLocation    *orientationWireLocation `json:"callsite_location,omitempty"`
	TargetLocation      orientationWireLocation  `json:"target_location"`
}

type orientationWireCommandCall struct {
	EvidenceRef   string `json:"evidence_ref"`
	Symbol        string `json:"symbol"`
	FileRef       string `json:"file_ref"`
	Line          int    `json:"line"`
	Relation      string `json:"relation"`
	Condition     any    `json:"condition,omitempty"`
	Resolved      bool   `json:"resolved"`
	TargetFileRef string `json:"target_file_ref,omitempty"`
	TargetLine    int    `json:"target_line,omitempty"`
}

type orientationWireLocation struct {
	FileRef string `json:"file_ref"`
	Line    int    `json:"line"`
}

type orientationWireOrientationCandidate struct {
	EvidenceRef       string   `json:"evidence_ref"`
	Name              string   `json:"name"`
	Kind              string   `json:"kind"`
	EntrypointPackage string   `json:"entrypoint_package"`
	OpenFileRefs      []string `json:"open_file_refs"`
	Why               string   `json:"why"`
	Priority          int      `json:"priority"`
}

type orientationWireEdge struct {
	EvidenceRef string `json:"evidence_ref"`
	From        string `json:"from"`
	To          string `json:"to"`
}

type orientationProviderResponse struct {
	ProjectGuess         string                                `json:"project_guess"`
	Confidence           float64                               `json:"confidence"`
	HighLevelMap         []orientationProviderMapItem          `json:"high_level_map"`
	FirstFilesToOpen     []orientationProviderFileToOpen       `json:"first_files_to_open"`
	CandidateFlows       []orientationProviderCandidateFlow    `json:"candidate_flows"`
	ImportantDomainWords []orientationProviderDomainWord       `json:"important_domain_words"`
	QuestionsForHuman    []string                              `json:"questions_for_human"`
	ResearchQuestions    []orientationProviderResearchQuestion `json:"research_questions,omitempty"`
	Warnings             []string                              `json:"warnings"`
}

type orientationProviderMapItem struct {
	Name         string            `json:"name"`
	Role         componentmap.Role `json:"role"`
	EvidenceRefs []string          `json:"evidence_refs"`
	WhyItMatters string            `json:"why_it_matters"`
}

type orientationProviderFileToOpen struct {
	FileRef string `json:"file_ref"`
	Reason  string `json:"reason"`
}

type orientationProviderCandidateFlow struct {
	Name                string   `json:"name"`
	FlowType            string   `json:"flow_type"`
	Trigger             string   `json:"trigger"`
	LikelyEntrypointRef string   `json:"likely_entrypoint_ref"`
	LikelyFileRefs      []string `json:"likely_file_refs"`
	WhyInteresting      string   `json:"why_interesting"`
	EvidenceRefs        []string `json:"evidence_refs"`
	Confidence          float64  `json:"confidence"`
}

type orientationProviderDomainWord struct {
	Word         string   `json:"word"`
	Guess        string   `json:"guess"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type orientationProviderResearchQuestion struct {
	ID                 string   `json:"id"`
	Purpose            string   `json:"purpose"`
	Question           string   `json:"question"`
	CandidateFileRefs  []string `json:"candidate_file_refs,omitempty"`
	EvidenceCategories []string `json:"evidence_categories,omitempty"`
}

type orientationEvidenceSeed struct {
	Kind      string
	Statement string
}

func buildOrientationReferenceCatalog(bundle llmbundle.Bundle) (orientationReferenceCatalog, error) {
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return orientationReferenceCatalog{}, fmt.Errorf("orientation references: encode bounded bundle: %w", err)
	}

	candidateIDs := make(map[string]string, len(bundle.CandidateFileIndex))
	candidatePaths := make(map[string]string, len(bundle.CandidateFileIndex))
	for _, candidate := range bundle.CandidateFileIndex {
		if candidate.Path == "" || candidate.ID == "" {
			return orientationReferenceCatalog{}, fmt.Errorf("orientation references: candidate file is missing path or id")
		}
		if existing, duplicate := candidateIDs[candidate.Path]; duplicate && existing != candidate.ID {
			return orientationReferenceCatalog{}, fmt.Errorf("orientation references: path %q has multiple candidate ids", candidate.Path)
		}
		if existing, duplicate := candidatePaths[candidate.ID]; duplicate && existing != candidate.Path {
			return orientationReferenceCatalog{}, fmt.Errorf("orientation references: candidate id maps to multiple paths")
		}
		candidateIDs[candidate.Path] = candidate.ID
		candidatePaths[candidate.ID] = candidate.Path
	}
	paths, err := orientationVisibleFilePaths(bundle)
	if err != nil {
		return orientationReferenceCatalog{}, err
	}
	files := make([]orientationFileReference, 0, len(paths))
	for index, path := range paths {
		files = append(files, orientationFileReference{
			Ref: orientationReferenceHandle('f', index+1), Path: path, CandidateID: candidateIDs[path],
		})
	}

	evidenceSeeds := make(map[string]orientationEvidenceSeed)
	addEvidence := func(kind, statement string) {
		statement = strings.TrimSpace(statement)
		if kind == "" || statement == "" {
			return
		}
		evidenceSeeds[orientationEvidenceKey(kind, statement)] = orientationEvidenceSeed{Kind: kind, Statement: statement}
	}
	for _, candidate := range bundle.CandidateFileIndex {
		addEvidence("candidate_file", orientationCandidateFileEvidence(candidate.Path, candidate.Kind, candidate.Signals))
	}
	for _, signal := range bundle.SourceSignals {
		addEvidence("source_signal", orientationSourceSignalEvidence(signal))
	}
	for _, entrypoint := range bundle.Go.Entrypoints {
		for _, anchor := range entrypoint.Anchors {
			addEvidence("entrypoint", orientationEntrypointEvidence(anchor.Path, anchor.Line, string(anchor.Kind)))
		}
	}
	for _, trace := range bundle.Go.CommandTraces {
		for _, step := range trace.Steps {
			addEvidence("command_trace", orientationCommandTraceEvidence(step.TargetLocation.Path, step.TargetLocation.Line, step.Relation, step.Symbol, false))
			if step.CallsiteLocation != nil {
				addEvidence("command_trace", orientationCommandTraceEvidence(step.CallsiteLocation.Path, step.CallsiteLocation.Line, step.Relation, step.Symbol, true))
			}
		}
		for _, call := range trace.HandlerCalls {
			addEvidence("command_trace", orientationCommandTraceEvidence(call.Path, call.Line, call.Relation, call.Symbol, false))
		}
	}
	for _, edge := range bundle.Go.ImportantEdges {
		addEvidence("import_edge", orientationImportEdgeEvidence(edge.From, edge.To))
	}
	for _, candidate := range bundle.Go.OrientationCandidates {
		addEvidence("orientation_candidate", orientationCandidateEvidence(candidate.Name, candidate.Why))
	}

	evidenceKeys := make([]string, 0, len(evidenceSeeds))
	for key := range evidenceSeeds {
		evidenceKeys = append(evidenceKeys, key)
	}
	sort.Strings(evidenceKeys)
	evidenceRefs := make([]orientationEvidenceReference, 0, len(evidenceKeys))
	for index, key := range evidenceKeys {
		seed := evidenceSeeds[key]
		evidenceRefs = append(evidenceRefs, orientationEvidenceReference{
			Ref: orientationReferenceHandle('e', index+1), Kind: seed.Kind, Statement: seed.Statement,
		})
	}

	identity := struct {
		Version          string                         `json:"version"`
		ResponseContract string                         `json:"response_contract"`
		Bundle           json.RawMessage                `json:"bundle"`
		Files            []orientationFileReference     `json:"files"`
		Evidence         []orientationEvidenceReference `json:"evidence"`
	}{
		Version:          orientationReferenceCatalogVersion,
		ResponseContract: orientationResponseContractVersion,
		Bundle:           append(json.RawMessage(nil), bundleJSON...),
		Files:            files,
		Evidence:         evidenceRefs,
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return orientationReferenceCatalog{}, fmt.Errorf("orientation references: encode identity: %w", err)
	}
	digest := sha256.Sum256(identityJSON)
	catalog := orientationReferenceCatalog{
		digest:           hex.EncodeToString(digest[:]),
		filesByRef:       make(map[string]orientationFileReference, len(files)),
		fileRefByPath:    make(map[string]string, len(files)),
		evidenceByRef:    make(map[string]orientationEvidenceReference, len(evidenceRefs)),
		evidenceRefByKey: make(map[string]string, len(evidenceRefs)),
		canonicalJSON:    append([]byte(nil), identityJSON...),
	}
	for _, file := range files {
		catalog.filesByRef[file.Ref] = file
		catalog.fileRefByPath[file.Path] = file.Ref
	}
	for _, item := range evidenceRefs {
		catalog.evidenceByRef[item.Ref] = item
		catalog.evidenceRefByKey[orientationEvidenceKey(item.Kind, item.Statement)] = item.Ref
	}
	return catalog, nil
}

func orientationVisibleFilePaths(bundle llmbundle.Bundle) ([]string, error) {
	seen := make(map[string]struct{})
	add := func(path string) error {
		if path == "" {
			return fmt.Errorf("orientation references: model-visible file path is empty")
		}
		seen[path] = struct{}{}
		return nil
	}
	for _, path := range bundle.KnownDocs {
		if err := add(path); err != nil {
			return nil, err
		}
	}
	for _, candidate := range bundle.CandidateFileIndex {
		if err := add(candidate.Path); err != nil {
			return nil, err
		}
	}
	for _, signal := range bundle.SourceSignals {
		if err := add(signal.Path); err != nil {
			return nil, err
		}
	}
	for _, entrypoint := range bundle.Go.Entrypoints {
		for _, anchor := range entrypoint.Anchors {
			if err := add(anchor.Path); err != nil {
				return nil, err
			}
		}
		for _, path := range entrypoint.OpenFiles {
			if err := add(path); err != nil {
				return nil, err
			}
		}
	}
	for _, trace := range bundle.Go.CommandTraces {
		for _, step := range trace.Steps {
			if err := add(step.TargetLocation.Path); err != nil {
				return nil, err
			}
			if step.CallsiteLocation != nil {
				if err := add(step.CallsiteLocation.Path); err != nil {
					return nil, err
				}
			}
		}
		for _, call := range trace.HandlerCalls {
			if err := add(call.Path); err != nil {
				return nil, err
			}
			if call.TargetPath != "" {
				if err := add(call.TargetPath); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, candidate := range bundle.Go.OrientationCandidates {
		for _, path := range candidate.OpenFiles {
			if err := add(path); err != nil {
				return nil, err
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func buildOrientationWireBundle(bundle llmbundle.Bundle, catalog orientationReferenceCatalog) ([]byte, error) {
	wire := orientationWireBundle{
		RepoName:               bundle.RepoName,
		ReadmeExcerpt:          bundle.ReadmeExcerpt,
		TopLevelDirectoryStats: bundle.TopLevelDirectoryStats,
		LanguageHints:          bundle.LanguageHints,
		Go: orientationWireGo{
			ModulesCount:    bundle.Go.ModulesCount,
			PackagesCount:   bundle.Go.PackagesCount,
			ModuleSummaries: bundle.Go.ModuleSummaries,
		},
		Warnings:             append([]string(nil), bundle.Warnings...),
		PolicyVersion:        bundle.PolicyVersion,
		LocalAuthorizedFiles: bundle.LocalAuthorizedFiles,
	}
	for _, file := range catalog.filesByRef {
		wire.FileIndex = append(wire.FileIndex, orientationWireFile{FileRef: file.Ref, Path: file.Path})
	}
	sort.Slice(wire.FileIndex, func(i, j int) bool { return wire.FileIndex[i].FileRef < wire.FileIndex[j].FileRef })
	for _, path := range bundle.KnownDocs {
		ref, err := catalog.fileRef(path)
		if err != nil {
			return nil, err
		}
		wire.KnownDocRefs = append(wire.KnownDocRefs, ref)
	}
	for _, candidate := range bundle.CandidateFileIndex {
		fileRef, err := catalog.fileRef(candidate.Path)
		if err != nil {
			return nil, err
		}
		evidenceRef, err := catalog.evidenceRef("candidate_file", orientationCandidateFileEvidence(candidate.Path, candidate.Kind, candidate.Signals))
		if err != nil {
			return nil, err
		}
		wire.CandidateFileIndex = append(wire.CandidateFileIndex, orientationWireCandidateFile{
			FileRef: fileRef, Kind: candidate.Kind,
			Signals: append([]string(nil), candidate.Signals...), Score: candidate.Score,
			Reasons: append([]string(nil), candidate.Reasons...), EvidenceRef: evidenceRef,
		})
	}
	for _, signal := range bundle.SourceSignals {
		fileRef, err := catalog.fileRef(signal.Path)
		if err != nil {
			return nil, err
		}
		evidenceRef, err := catalog.evidenceRef("source_signal", orientationSourceSignalEvidence(signal))
		if err != nil {
			return nil, err
		}
		wire.SourceSignals = append(wire.SourceSignals, orientationWireSourceSignal{
			EvidenceRef: evidenceRef, FileRef: fileRef, Line: signal.Line, Category: signal.Category,
			Match: signal.Match, Snippet: signal.Snippet, Weight: signal.Weight, Penalty: signal.Penalty, Reason: signal.Reason,
		})
	}
	for _, entrypoint := range bundle.Go.Entrypoints {
		item := orientationWireEntrypoint{Kind: entrypoint.Kind, ImportPath: entrypoint.ImportPath, PackageDir: entrypoint.PackageDir}
		for _, anchor := range entrypoint.Anchors {
			fileRef, err := catalog.fileRef(anchor.Path)
			if err != nil {
				return nil, err
			}
			evidenceRef, err := catalog.evidenceRef("entrypoint", orientationEntrypointEvidence(anchor.Path, anchor.Line, string(anchor.Kind)))
			if err != nil {
				return nil, err
			}
			item.Anchors = append(item.Anchors, orientationWireAnchor{EvidenceRef: evidenceRef, Version: anchor.Version, Kind: string(anchor.Kind), FileRef: fileRef, Line: anchor.Line})
		}
		for _, path := range entrypoint.OpenFiles {
			ref, err := catalog.fileRef(path)
			if err != nil {
				return nil, err
			}
			item.OpenFileRefs = append(item.OpenFileRefs, ref)
		}
		wire.Go.Entrypoints = append(wire.Go.Entrypoints, item)
	}
	for _, trace := range bundle.Go.CommandTraces {
		item := orientationWireCommandTrace{
			Version: trace.Version, Framework: trace.Framework, EntrypointPackage: trace.EntrypointPackage,
			Command: trace.Command, Concurrency: string(trace.Concurrency), Complete: trace.Complete,
			Missing: append([]string(nil), trace.Missing...),
		}
		for _, step := range trace.Steps {
			target, err := catalog.wireLocation(step.TargetLocation.Path, step.TargetLocation.Line)
			if err != nil {
				return nil, err
			}
			targetEvidence, err := catalog.evidenceRef("command_trace", orientationCommandTraceEvidence(step.TargetLocation.Path, step.TargetLocation.Line, step.Relation, step.Symbol, false))
			if err != nil {
				return nil, err
			}
			wireStep := orientationWireCommandStep{TargetEvidenceRef: targetEvidence, Symbol: step.Symbol, Relation: step.Relation, TargetLocation: target}
			if step.CallsiteLocation != nil {
				callsite, err := catalog.wireLocation(step.CallsiteLocation.Path, step.CallsiteLocation.Line)
				if err != nil {
					return nil, err
				}
				callsiteEvidence, err := catalog.evidenceRef("command_trace", orientationCommandTraceEvidence(step.CallsiteLocation.Path, step.CallsiteLocation.Line, step.Relation, step.Symbol, true))
				if err != nil {
					return nil, err
				}
				wireStep.CallsiteLocation = &callsite
				wireStep.CallsiteEvidenceRef = callsiteEvidence
			}
			item.Steps = append(item.Steps, wireStep)
		}
		for _, call := range trace.HandlerCalls {
			fileRef, err := catalog.fileRef(call.Path)
			if err != nil {
				return nil, err
			}
			evidenceRef, err := catalog.evidenceRef("command_trace", orientationCommandTraceEvidence(call.Path, call.Line, call.Relation, call.Symbol, false))
			if err != nil {
				return nil, err
			}
			wireCall := orientationWireCommandCall{
				EvidenceRef: evidenceRef, Symbol: call.Symbol, FileRef: fileRef, Line: call.Line,
				Relation: call.Relation, Condition: call.Condition, Resolved: call.Resolved, TargetLine: call.TargetLine,
			}
			if call.TargetPath != "" {
				wireCall.TargetFileRef, err = catalog.fileRef(call.TargetPath)
				if err != nil {
					return nil, err
				}
			}
			item.HandlerCalls = append(item.HandlerCalls, wireCall)
		}
		wire.Go.CommandTraces = append(wire.Go.CommandTraces, item)
	}
	for _, candidate := range bundle.Go.OrientationCandidates {
		evidenceRef, err := catalog.evidenceRef("orientation_candidate", orientationCandidateEvidence(candidate.Name, candidate.Why))
		if err != nil {
			return nil, err
		}
		item := orientationWireOrientationCandidate{
			EvidenceRef: evidenceRef, Name: candidate.Name, Kind: candidate.Kind,
			EntrypointPackage: candidate.EntrypointPackage, Why: candidate.Why, Priority: candidate.Priority,
		}
		for _, path := range candidate.OpenFiles {
			ref, err := catalog.fileRef(path)
			if err != nil {
				return nil, err
			}
			item.OpenFileRefs = append(item.OpenFileRefs, ref)
		}
		wire.Go.OrientationCandidates = append(wire.Go.OrientationCandidates, item)
	}
	for _, edge := range bundle.Go.ImportantEdges {
		evidenceRef, err := catalog.evidenceRef("import_edge", orientationImportEdgeEvidence(edge.From, edge.To))
		if err != nil {
			return nil, err
		}
		wire.Go.ImportantEdges = append(wire.Go.ImportantEdges, orientationWireEdge{EvidenceRef: evidenceRef, From: edge.From, To: edge.To})
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("orientation references: encode wire bundle: %w", err)
	}
	return encoded, nil
}

func orientationReferenceHandle(namespace byte, ordinal int) string {
	return fmt.Sprintf("%c%04d", namespace, ordinal)
}

func orientationEvidenceKey(kind, statement string) string { return kind + "\x00" + statement }

func orientationCandidateFileEvidence(path, kind string, signals []string) string {
	detail := strings.Join(signals, ", ")
	if detail == "" {
		detail = kind
	}
	return fmt.Sprintf("%s candidate_file %s", path, detail)
}

func orientationEntrypointEvidence(path string, line int, kind string) string {
	return fmt.Sprintf("%s:%d entrypoint %s", path, line, kind)
}

func orientationCommandTraceEvidence(path string, line int, relation, symbol string, callsite bool) string {
	if callsite {
		return fmt.Sprintf("%s:%d command_trace callsite %s %s", path, line, relation, symbol)
	}
	return fmt.Sprintf("%s:%d command_trace %s %s", path, line, relation, symbol)
}

func orientationImportEdgeEvidence(from, to string) string {
	return fmt.Sprintf("internal import edge %s -> %s", from, to)
}

func orientationCandidateEvidence(name, why string) string {
	return fmt.Sprintf("orientation candidate %s: %s", name, why)
}

func (catalog orientationReferenceCatalog) fileRef(path string) (string, error) {
	ref, ok := catalog.fileRefByPath[path]
	if !ok {
		return "", fmt.Errorf("orientation references: no file ref for %q", path)
	}
	return ref, nil
}

func (catalog orientationReferenceCatalog) evidenceRef(kind, statement string) (string, error) {
	ref, ok := catalog.evidenceRefByKey[orientationEvidenceKey(kind, statement)]
	if !ok {
		return "", fmt.Errorf("orientation references: no evidence ref for %s", kind)
	}
	return ref, nil
}

func (catalog orientationReferenceCatalog) wireLocation(path string, line int) (orientationWireLocation, error) {
	ref, err := catalog.fileRef(path)
	if err != nil {
		return orientationWireLocation{}, err
	}
	return orientationWireLocation{FileRef: ref, Line: line}, nil
}

func parseAndResolveOrientationResponse(raw []byte, catalog orientationReferenceCatalog) (orientationPart, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response orientationProviderResponse
	if err := decoder.Decode(&response); err != nil {
		return orientationPart{}, fmt.Errorf("orientation refs: decode exact response: %w", err)
	}
	if err := ensureOrientationJSONEOF(decoder); err != nil {
		return orientationPart{}, err
	}
	report := orientationPart{
		ProjectGuess: response.ProjectGuess, Confidence: response.Confidence,
		QuestionsForHuman: append([]string(nil), response.QuestionsForHuman...),
		Warnings:          append([]string(nil), response.Warnings...), UnverifiedPaths: unverifiedPathList{},
	}
	for index, item := range response.HighLevelMap {
		if !isExactOrientationRole(item.Role) {
			return orientationPart{}, fmt.Errorf("orientation refs: high_level_map[%d] has unsupported role", index)
		}
		evidence, err := catalog.resolveEvidenceRefs(fmt.Sprintf("high_level_map[%d].evidence_refs", index), item.EvidenceRefs)
		if err != nil {
			return orientationPart{}, err
		}
		report.HighLevelMap = append(report.HighLevelMap, orientationMapItem{Name: item.Name, Role: item.Role, Evidence: evidence, WhyItMatters: item.WhyItMatters})
	}
	seenFirstFiles := make(map[string]struct{}, len(response.FirstFilesToOpen))
	for index, item := range response.FirstFilesToOpen {
		file, err := catalog.resolveFileRef(fmt.Sprintf("first_files_to_open[%d].file_ref", index), item.FileRef)
		if err != nil {
			return orientationPart{}, err
		}
		if _, duplicate := seenFirstFiles[item.FileRef]; duplicate {
			return orientationPart{}, fmt.Errorf("orientation refs: duplicate first_files_to_open file ref %q", item.FileRef)
		}
		seenFirstFiles[item.FileRef] = struct{}{}
		report.FirstFilesToOpen = append(report.FirstFilesToOpen, fileToOpen{Path: file.Path, Reason: item.Reason})
	}
	for index, flow := range response.CandidateFlows {
		if flow.FlowType != flowexplain.FlowTypeRequest && flow.FlowType != flowexplain.FlowTypeOperational {
			return orientationPart{}, fmt.Errorf("orientation refs: candidate_flows[%d] has unsupported flow_type", index)
		}
		entrypoint, err := catalog.resolveFileRef(fmt.Sprintf("candidate_flows[%d].likely_entrypoint_ref", index), flow.LikelyEntrypointRef)
		if err != nil {
			return orientationPart{}, err
		}
		files, err := catalog.resolveFileRefs(fmt.Sprintf("candidate_flows[%d].likely_file_refs", index), flow.LikelyFileRefs)
		if err != nil {
			return orientationPart{}, err
		}
		evidence, err := catalog.resolveEvidenceRefs(fmt.Sprintf("candidate_flows[%d].evidence_refs", index), flow.EvidenceRefs)
		if err != nil {
			return orientationPart{}, err
		}
		report.CandidateFlows = append(report.CandidateFlows, flowexplain.CandidateFlow{
			Name: flow.Name, FlowType: flow.FlowType, Trigger: flow.Trigger,
			LikelyEntrypoint: entrypoint.Path, LikelyFiles: files, WhyInteresting: flow.WhyInteresting,
			Evidence: evidence, Confidence: flow.Confidence, CandidateBasis: flowexplain.CandidateBasisModelOrientation,
		})
	}
	for index, item := range response.ImportantDomainWords {
		evidence, err := catalog.resolveEvidenceRefs(fmt.Sprintf("important_domain_words[%d].evidence_refs", index), item.EvidenceRefs)
		if err != nil {
			return orientationPart{}, err
		}
		report.ImportantDomainWords = append(report.ImportantDomainWords, orientationDomainWord{Word: item.Word, Guess: item.Guess, Evidence: evidence})
	}
	for index, question := range response.ResearchQuestions {
		candidateIDs, err := catalog.resolveCandidateRefs(fmt.Sprintf("research_questions[%d].candidate_file_refs", index), question.CandidateFileRefs)
		if err != nil {
			return orientationPart{}, err
		}
		report.ResearchQuestions = append(report.ResearchQuestions, modelresearch.ProposedQuestion{
			ID: question.ID, Purpose: question.Purpose, Question: question.Question,
			CandidateIDs: candidateIDs, EvidenceCategories: append([]string(nil), question.EvidenceCategories...),
		})
	}
	return report, nil
}

func isExactOrientationRole(role componentmap.Role) bool {
	switch role {
	case componentmap.RoleUnknown, componentmap.RoleEntry, componentmap.RoleBoundary,
		componentmap.RoleCoordination, componentmap.RoleDomain, componentmap.RoleState,
		componentmap.RoleSupport:
		return true
	default:
		return false
	}
}

func ensureOrientationJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("orientation refs: decode trailing response: %w", err)
	}
	return fmt.Errorf("orientation refs: multiple JSON values")
}

func (catalog orientationReferenceCatalog) resolveFileRef(field, ref string) (orientationFileReference, error) {
	if file, ok := catalog.filesByRef[ref]; ok {
		return file, nil
	}
	if _, wrongKind := catalog.evidenceByRef[ref]; wrongKind {
		return orientationFileReference{}, fmt.Errorf("orientation refs: %s uses evidence ref %q", field, ref)
	}
	return orientationFileReference{}, fmt.Errorf("orientation refs: %s has unknown file ref %q", field, ref)
}

func (catalog orientationReferenceCatalog) resolveFileRefs(field string, refs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(refs))
	paths := make([]string, 0, len(refs))
	for index, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, fmt.Errorf("orientation refs: %s has duplicate ref %q", field, ref)
		}
		seen[ref] = struct{}{}
		file, err := catalog.resolveFileRef(fmt.Sprintf("%s[%d]", field, index), ref)
		if err != nil {
			return nil, err
		}
		paths = append(paths, file.Path)
	}
	return paths, nil
}

func (catalog orientationReferenceCatalog) resolveCandidateRefs(field string, refs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(refs))
	ids := make([]string, 0, len(refs))
	for index, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, fmt.Errorf("orientation refs: %s has duplicate ref %q", field, ref)
		}
		seen[ref] = struct{}{}
		file, err := catalog.resolveFileRef(fmt.Sprintf("%s[%d]", field, index), ref)
		if err != nil {
			return nil, err
		}
		if file.CandidateID == "" {
			return nil, fmt.Errorf("orientation refs: %s has no candidate mapping", field)
		}
		ids = append(ids, file.CandidateID)
	}
	return ids, nil
}

func (catalog orientationReferenceCatalog) resolveEvidenceRefs(field string, refs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(refs))
	statements := make([]string, 0, len(refs))
	for index, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, fmt.Errorf("orientation refs: %s has duplicate ref %q", field, ref)
		}
		seen[ref] = struct{}{}
		item, ok := catalog.evidenceByRef[ref]
		if !ok {
			if _, wrongKind := catalog.filesByRef[ref]; wrongKind {
				return nil, fmt.Errorf("orientation refs: %s[%d] uses file ref %q", field, index, ref)
			}
			return nil, fmt.Errorf("orientation refs: %s[%d] has unknown evidence ref %q", field, index, ref)
		}
		statements = append(statements, item.Statement)
	}
	return statements, nil
}
