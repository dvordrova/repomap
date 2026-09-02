// Package pythonprogramindex projects a selected Python target into the
// language-neutral program index without importing or executing repository
// modules.
package pythonprogramindex

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

const (
	adapterVersion       = 8
	maxParserStderrBytes = 16 << 10
)

//go:embed parser.py
var parserHelper string

type parserRequest struct {
	Sources []parserSource `json:"sources"`
	Views   []parserView   `json:"views"`
}

type parserSource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type parserView struct {
	Files    []parserFile    `json:"files"`
	Packages []parserPackage `json:"packages"`
}

type parserFile struct {
	SourceRef string   `json:"source_ref"`
	Path      string   `json:"path"`
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases,omitempty"`
	Package   bool     `json:"package"`
}

type parserPackage struct {
	SourceRef string `json:"source_ref"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Namespace bool   `json:"namespace,omitempty"`
}

type parserResponse struct {
	Fatal         string             `json:"fatal,omitempty"`
	PythonVersion string             `json:"python_version"`
	Views         []parserViewResult `json:"views"`
}

type parserViewResult struct {
	Objects   []parsedObject   `json:"objects"`
	Relations []parsedRelation `json:"relations"`
}

type parsedObject struct {
	SourceRef            string                       `json:"source_ref"`
	Kind                 string                       `json:"kind"`
	Name                 string                       `json:"name"`
	Visibility           string                       `json:"visibility"`
	Signature            string                       `json:"signature,omitempty"`
	OwnerRef             string                       `json:"owner_ref,omitempty"`
	ContainerRef         string                       `json:"container_ref,omitempty"`
	Location             *programindex.Location       `json:"location,omitempty"`
	SymbolLinkIdentities []parsedSymbolLinkIdentity   `json:"symbol_link_identities,omitempty"`
	External             *programindex.ExternalSymbol `json:"external,omitempty"`
}

type parsedSymbolLinkIdentity struct {
	Domain  string   `json:"domain"`
	Parts   []string `json:"parts"`
	Display string   `json:"display,omitempty"`
}

type parsedRelation struct {
	SourceRef         string                    `json:"source_ref"`
	Kind              string                    `json:"kind"`
	FromRef           string                    `json:"from_ref"`
	ToRefs            []string                  `json:"to_refs"`
	Resolution        string                    `json:"resolution"`
	Invocation        string                    `json:"invocation,omitempty"`
	Location          *programindex.Location    `json:"location,omitempty"`
	TargetsObserved   int                       `json:"targets_observed"`
	Witnesses         []programindex.Witness    `json:"witnesses"`
	WitnessesObserved int                       `json:"witnesses_observed"`
	Patterns          []parsedRelationPattern   `json:"patterns"`
	PatternsObserved  int                       `json:"patterns_observed"`
	SourceArgument    *parsedPatternArgumentRef `json:"source_argument,omitempty"`
}

type parsedPatternArgumentRef struct {
	RelationSourceRef string `json:"relation_source_ref"`
	PatternSourceRef  string `json:"pattern_source_ref"`
	Position          int    `json:"position,omitempty"`
	Keyword           string `json:"keyword,omitempty"`
}

type parsedRelationPattern struct {
	SourceRef                string                   `json:"source_ref"`
	Form                     programindex.PatternForm `json:"form"`
	Selector                 string                   `json:"selector"`
	Location                 *programindex.Location   `json:"location,omitempty"`
	ResultRef                string                   `json:"result_ref,omitempty"`
	ReceiverRef              string                   `json:"receiver_ref,omitempty"`
	ReceiverOriginRefs       []string                 `json:"receiver_origin_refs,omitempty"`
	ReceiverOriginResolution programindex.Resolution  `json:"receiver_origin_resolution,omitempty"`
	ReceiverOriginsObserved  int                      `json:"receiver_origins_observed"`
	Arguments                []parsedPatternArgument  `json:"arguments"`
	ArgumentsObserved        int                      `json:"arguments_observed"`
}

type parsedPatternArgument struct {
	Position                int                           `json:"position,omitempty"`
	Keyword                 string                        `json:"keyword,omitempty"`
	Kind                    programindex.PatternValueKind `json:"kind"`
	Value                   string                        `json:"value,omitempty"`
	Parts                   []parsedPatternPart           `json:"parts,omitempty"`
	ObjectRefs              []string                      `json:"object_refs,omitempty"`
	Resolution              programindex.Resolution       `json:"resolution,omitempty"`
	ObjectsObserved         int                           `json:"objects_observed"`
	ValueCandidates         []parsedPatternValueCandidate `json:"value_candidates,omitempty"`
	ValueCandidatesObserved int                           `json:"value_candidates_observed"`
}

type parsedPatternValueCandidate struct {
	Kind                  programindex.PatternValueKind       `json:"kind"`
	Value                 string                              `json:"value,omitempty"`
	Parts                 []parsedPatternPart                 `json:"parts,omitempty"`
	Resolution            programindex.PatternValueResolution `json:"resolution"`
	SourceKind            programindex.PatternValueSourceKind `json:"source_kind"`
	SourceObjectRefs      []string                            `json:"source_object_refs"`
	SourceObjectsObserved int                                 `json:"source_objects_observed"`
}

type parsedPatternPart struct {
	Kind programindex.PatternPartKind `json:"kind"`
	Text string                       `json:"text,omitempty"`
}

type sourceDigest struct {
	FileID     corpus.FileID `json:"file_id"`
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Aliases    []string      `json:"aliases,omitempty"`
	Importable bool          `json:"importable,omitempty"`
	Package    bool          `json:"package"`
	SHA256     string        `json:"sha256"`
}

type parserRunner func(context.Context, parserRequest) (parserResponse, error)

type targetGroup struct {
	positions     []int
	targets       []pythontarget.Target
	modules       []pythontarget.Module
	packages      []pythontarget.Package
	aliasesByPath map[string]map[string]struct{}
}

type sourceGroup struct {
	modules []pythontarget.Module
	views   []*targetGroup
}

type moduleAliasIdentity struct {
	Path    string   `json:"path"`
	Aliases []string `json:"aliases"`
}

type targetGroupIdentity struct {
	Modules  []pythontarget.Module  `json:"modules"`
	Packages []pythontarget.Package `json:"packages,omitempty"`
	Aliases  []moduleAliasIdentity  `json:"aliases,omitempty"`
}

type parsedGroup struct {
	objects        []programindex.ObjectInput
	relations      []programindex.RelationInput
	objectRefs     map[string]parsedObject
	scenarioSHA256 string
	sourceSHA256   string
}

// BuildMany preserves target order while sharing one complete corpus read and AST
// parse across targets with an identical module inventory. Module aliases and
// package authority remain target-local: views that share source files but
// assign them different meanings are projected independently over that one
// parsed syntax batch.
func BuildMany(
	ctx context.Context,
	repository *corpus.Corpus,
	targets []pythontarget.Target,
) ([]programindex.Index, error) {
	return buildMany(ctx, repository, targets, runParser)
}

// BuildInput runs the complete isolated Python parser for one exact target and
// returns the adapter-owned facts before common ProgramIndex identity sealing.
// The ordinary repository dispatcher passes this value through the same
// programindex.New boundary as every other atomic language adapter.
func BuildInput(
	ctx context.Context,
	repository *corpus.Corpus,
	target pythontarget.Target,
) (programindex.Input, error) {
	inputs, err := buildInputs(ctx, repository, []pythontarget.Target{target}, runParser)
	if err != nil {
		return programindex.Input{}, err
	}
	if len(inputs) != 1 {
		return programindex.Input{}, fmt.Errorf("python program index: parser returned no exact target input")
	}
	return inputs[0], nil
}

func buildMany(
	ctx context.Context,
	repository *corpus.Corpus,
	targets []pythontarget.Target,
	runner parserRunner,
) ([]programindex.Index, error) {
	inputs, err := buildInputs(ctx, repository, targets, runner)
	if err != nil {
		return nil, err
	}
	indexes := make([]programindex.Index, len(inputs))
	for position, input := range inputs {
		index, sealErr := programindex.New(input)
		if sealErr != nil {
			return nil, fmt.Errorf(
				"python program index: target %q: seal: %w",
				targets[position].Selector,
				sealErr,
			)
		}
		indexes[position] = index
	}
	return indexes, nil
}

func buildInputs(
	ctx context.Context,
	repository *corpus.Corpus,
	targets []pythontarget.Target,
	runner parserRunner,
) ([]programindex.Input, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if repository == nil {
		return nil, fmt.Errorf("python program index: repository corpus is required")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return nil, fmt.Errorf("python program index: corpus: %w", err)
	}
	if runner == nil {
		return nil, fmt.Errorf("python program index: parser runner is required")
	}
	if len(targets) == 0 {
		return []programindex.Input{}, nil
	}

	groups := make([]*targetGroup, 0)
	bySemantics := make(map[string]*targetGroup)
	for position, target := range targets {
		if err := target.Validate(); err != nil {
			return nil, fmt.Errorf("python program index: target %d: %w", position, err)
		}
		candidate := targetGroupForTarget(target)
		key, err := targetGroupKey(candidate)
		if err != nil {
			return nil, fmt.Errorf("python program index: target %d parser semantics: %w", position, err)
		}
		group := bySemantics[key]
		if group == nil {
			group = candidate
			groups = append(groups, group)
			bySemantics[key] = group
		}
		group.positions = append(group.positions, position)
		group.targets = append(group.targets, target)
	}

	sourceGroups := make([]*sourceGroup, 0, len(groups))
	bySources := make(map[string]*sourceGroup, len(groups))
	for _, group := range groups {
		key, err := sourceGroupKey(group.modules)
		if err != nil {
			return nil, fmt.Errorf("python program index: source inventory: %w", err)
		}
		batch := bySources[key]
		if batch == nil {
			batch = &sourceGroup{modules: append([]pythontarget.Module(nil), group.modules...)}
			bySources[key] = batch
			sourceGroups = append(sourceGroups, batch)
		}
		batch.views = append(batch.views, group)
	}

	inputs := make([]programindex.Input, len(targets))
	for _, batch := range sourceGroups {
		parsedViews, err := parseSourceGroup(ctx, repository, batch, runner)
		if err != nil {
			return nil, err
		}
		for viewPosition, group := range batch.views {
			parsed := parsedViews[viewPosition]
			for offset, target := range group.targets {
				input, err := inputForTarget(repository, target, parsed)
				if err != nil {
					return nil, fmt.Errorf("python program index: target %q: %w", target.Selector, err)
				}
				inputs[group.positions[offset]] = input
			}
		}
	}
	return inputs, nil
}

func targetGroupForTarget(target pythontarget.Target) *targetGroup {
	group := &targetGroup{
		modules:       append([]pythontarget.Module(nil), target.Modules...),
		packages:      append([]pythontarget.Package(nil), target.Packages...),
		aliasesByPath: make(map[string]map[string]struct{}),
	}
	addModuleAliases(group, target)
	return group
}

func targetGroupKey(group *targetGroup) (string, error) {
	aliases := make([]moduleAliasIdentity, 0, len(group.aliasesByPath))
	for filePath, values := range group.aliasesByPath {
		row := moduleAliasIdentity{Path: filePath, Aliases: make([]string, 0, len(values))}
		for value := range values {
			row.Aliases = append(row.Aliases, value)
		}
		sort.Strings(row.Aliases)
		aliases = append(aliases, row)
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].Path < aliases[j].Path })
	wire, err := json.Marshal(targetGroupIdentity{
		Modules: group.modules, Packages: group.packages, Aliases: aliases,
	})
	if err != nil {
		return "", err
	}
	return string(wire), nil
}

func sourceGroupKey(modules []pythontarget.Module) (string, error) {
	wire, err := json.Marshal(modules)
	if err != nil {
		return "", err
	}
	return string(wire), nil
}

// addModuleAliases preserves additional exact import spellings established
// by the target's project-relative source layout. A src-layout module named
// "config" can also be imported as "src.config" when src is itself a Python
// package. Both spellings resolve to the same source object; no duplicate
// object or guessed runtime module is created.
func addModuleAliases(group *targetGroup, target pythontarget.Target) {
	for _, module := range target.Modules {
		alias := projectRelativeModuleAlias(target.ProjectDir, module)
		if alias == "" || alias == module.Name {
			continue
		}
		aliases := group.aliasesByPath[module.Path]
		if aliases == nil {
			aliases = make(map[string]struct{})
			group.aliasesByPath[module.Path] = aliases
		}
		aliases[alias] = struct{}{}
	}
}

func projectRelativeModuleAlias(projectDir string, module pythontarget.Module) string {
	relative := module.Path
	if projectDir != "." {
		prefix := strings.TrimSuffix(projectDir, "/") + "/"
		if !strings.HasPrefix(relative, prefix) {
			return ""
		}
		relative = strings.TrimPrefix(relative, prefix)
	}
	if path.Ext(relative) != ".py" {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(relative, ".py"), "/")
	if len(parts) > 0 && parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return ""
	}
	for _, part := range parts {
		if !pythonImportIdentifier(part) {
			return ""
		}
	}
	return strings.Join(parts, ".")
}

func pythonImportIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for position, character := range value {
		if character == '_' || character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' || position > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func parseSourceGroup(
	ctx context.Context,
	repository *corpus.Corpus,
	group *sourceGroup,
	runner parserRunner,
) ([]parsedGroup, error) {
	if group == nil || len(group.modules) == 0 || len(group.views) == 0 {
		return nil, fmt.Errorf("python program index: source group is empty")
	}
	request := parserRequest{
		Sources: make([]parserSource, 0, len(group.modules)),
		Views:   make([]parserView, 0, len(group.views)),
	}
	allowedPaths := make(map[string]struct{}, len(group.modules))
	baseDigests := make([]sourceDigest, 0, len(group.modules))
	for _, module := range group.modules {
		info, ok := repository.Info(module.FileID)
		if !ok || info.Entry.Path != module.Path {
			return nil, fmt.Errorf(
				"python program index: module %q is not bound to repository corpus", module.Path,
			)
		}
		content, err := repository.ReadFileAll(module.FileID)
		if err != nil {
			return nil, fmt.Errorf("python program index: read %q: %w", module.Path, err)
		}
		allowedPaths[module.Path] = struct{}{}
		digest := sha256.Sum256(content.Bytes)
		baseDigests = append(baseDigests, sourceDigest{
			FileID: module.FileID, Path: module.Path, Name: module.Name,
			Importable: module.Importable, Package: module.Package,
			SHA256: hex.EncodeToString(digest[:]),
		})
		request.Sources = append(request.Sources, parserSource{
			Path: module.Path, Content: base64.StdEncoding.EncodeToString(content.Bytes),
		})
	}

	viewDigests := make([][]sourceDigest, 0, len(group.views))
	for _, semantic := range group.views {
		if key, err := sourceGroupKey(semantic.modules); err != nil {
			return nil, err
		} else if expected, expectedErr := sourceGroupKey(group.modules); expectedErr != nil || key != expected {
			return nil, fmt.Errorf("python program index: semantic view source inventory mismatch")
		}
		view := parserView{
			Files:    make([]parserFile, 0, len(semantic.modules)),
			Packages: make([]parserPackage, 0, len(semantic.packages)),
		}
		packageRefs := make(map[string]string, len(semantic.packages))
		for _, pkg := range semantic.packages {
			ref := packageSourceRef(pkg)
			packageRefs[pkg.Name] = ref
			view.Packages = append(view.Packages, parserPackage{
				SourceRef: ref, Name: pkg.Name, Path: pkg.Path, Namespace: pkg.Namespace,
			})
		}
		digests := make([]sourceDigest, len(baseDigests))
		copy(digests, baseDigests)
		for position, module := range semantic.modules {
			ref := stableSourceRef("module", module.Name, module.Path)
			if module.Package {
				if packageRef, exists := packageRefs[module.Name]; exists {
					ref = packageRef
				}
			}
			aliases := make([]string, 0, len(semantic.aliasesByPath[module.Path]))
			for alias := range semantic.aliasesByPath[module.Path] {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			digests[position].Aliases = aliases
			view.Files = append(view.Files, parserFile{
				SourceRef: ref, Path: module.Path, Name: module.Name, Aliases: aliases, Package: module.Package,
			})
		}
		request.Views = append(request.Views, view)
		viewDigests = append(viewDigests, digests)
	}

	response, err := runner(ctx, request)
	if err != nil {
		return nil, err
	}
	if len(response.Views) != len(request.Views) {
		return nil, fmt.Errorf(
			"python program index: parser returned %d views for %d requests",
			len(response.Views), len(request.Views),
		)
	}
	scenarioSHA256, err := hashCanonical(struct {
		Version       int    `json:"version"`
		Language      string `json:"language"`
		Parser        string `json:"parser"`
		HelperSHA256  string `json:"helper_sha256"`
		PythonVersion string `json:"python_version"`
	}{
		Version: adapterVersion, Language: "python", Parser: "stdlib_ast",
		HelperSHA256: stableDigest([]byte(parserHelper)), PythonVersion: response.PythonVersion,
	})
	if err != nil {
		return nil, err
	}

	result := make([]parsedGroup, len(response.Views))
	for position, responseView := range response.Views {
		parsed, err := compileParserView(responseView, allowedPaths)
		if err != nil {
			return nil, fmt.Errorf("python program index: parser view %d: %w", position, err)
		}
		sourceSHA256, err := hashCanonical(struct {
			Version  int             `json:"version"`
			Packages []parserPackage `json:"packages"`
			Sources  []sourceDigest  `json:"sources"`
		}{Version: adapterVersion, Packages: request.Views[position].Packages, Sources: viewDigests[position]})
		if err != nil {
			return nil, err
		}
		parsed.scenarioSHA256 = scenarioSHA256
		parsed.sourceSHA256 = sourceSHA256
		result[position] = parsed
	}
	return result, nil
}

func compileParserView(response parserViewResult, allowedPaths map[string]struct{}) (parsedGroup, error) {
	objects := make([]programindex.ObjectInput, 0, len(response.Objects))
	objectRefs := make(map[string]parsedObject, len(response.Objects))
	for _, value := range response.Objects {
		if err := validateHelperLocation(value.Location, allowedPaths); err != nil {
			return parsedGroup{}, fmt.Errorf("object %q: %w", value.SourceRef, err)
		}
		kind := programindex.ObjectKind(value.Kind)
		if kind == programindex.ObjectExternalSymbol {
			if value.External == nil || !value.External.AuthorityKind.Valid() {
				return parsedGroup{}, fmt.Errorf("object %q: missing or invalid external authority kind", value.SourceRef)
			}
		}
		if _, duplicate := objectRefs[value.SourceRef]; duplicate {
			return parsedGroup{}, fmt.Errorf("duplicate helper object %q", value.SourceRef)
		}
		objectRefs[value.SourceRef] = value
		linkIdentities := make([]programindex.SymbolLinkIdentityInput, len(value.SymbolLinkIdentities))
		for position, identity := range value.SymbolLinkIdentities {
			linkIdentities[position] = programindex.SymbolLinkIdentityInput{
				Domain: identity.Domain, Parts: append([]string(nil), identity.Parts...), Display: identity.Display,
			}
		}
		objects = append(objects, programindex.ObjectInput{
			SourceRef: value.SourceRef, Kind: kind, Name: value.Name,
			Visibility: programindex.Visibility(value.Visibility), Signature: value.Signature,
			OwnerRef: value.OwnerRef, ContainerRef: value.ContainerRef, Location: cloneLocation(value.Location),
			SymbolLinkIdentities: linkIdentities, External: cloneParsedExternalSymbol(value.External),
		})
	}
	relations := make([]programindex.RelationInput, 0, len(response.Relations))
	for _, value := range response.Relations {
		if err := validateHelperLocation(value.Location, allowedPaths); err != nil {
			return parsedGroup{}, fmt.Errorf("relation %q: %w", value.SourceRef, err)
		}
		witnesses := make([]programindex.Witness, len(value.Witnesses))
		for position, witness := range value.Witnesses {
			if err := validateHelperLocation(witness.Location, allowedPaths); err != nil {
				return parsedGroup{}, fmt.Errorf("relation %q witness: %w", value.SourceRef, err)
			}
			witnesses[position] = programindex.Witness{
				Kind: witness.Kind, Detail: witness.Detail, SourceExpression: witness.SourceExpression,
				Location: cloneLocation(witness.Location),
			}
		}
		patterns := make([]programindex.RelationPatternInput, len(value.Patterns))
		for patternPosition, pattern := range value.Patterns {
			if err := validateHelperLocation(pattern.Location, allowedPaths); err != nil {
				return parsedGroup{}, fmt.Errorf("relation %q pattern: %w", value.SourceRef, err)
			}
			arguments := make([]programindex.PatternArgumentInput, len(pattern.Arguments))
			for argumentPosition, argument := range pattern.Arguments {
				parts := make([]programindex.PatternPartInput, len(argument.Parts))
				for partPosition, part := range argument.Parts {
					parts[partPosition] = programindex.PatternPartInput{Kind: part.Kind, Text: part.Text}
				}
				valueCandidates := make([]programindex.PatternValueCandidateInput, len(argument.ValueCandidates))
				for candidatePosition, candidate := range argument.ValueCandidates {
					candidateParts := make([]programindex.PatternPartInput, len(candidate.Parts))
					for partPosition, part := range candidate.Parts {
						candidateParts[partPosition] = programindex.PatternPartInput{Kind: part.Kind, Text: part.Text}
					}
					valueCandidates[candidatePosition] = programindex.PatternValueCandidateInput{
						Kind: candidate.Kind, Value: candidate.Value, Parts: candidateParts,
						Resolution: candidate.Resolution, SourceKind: candidate.SourceKind,
						SourceObjectRefs:      append([]string(nil), candidate.SourceObjectRefs...),
						SourceObjectsObserved: candidate.SourceObjectsObserved,
					}
				}
				arguments[argumentPosition] = programindex.PatternArgumentInput{
					Position: argument.Position, Keyword: argument.Keyword, Kind: argument.Kind,
					Value: argument.Value, Parts: parts,
					ObjectRefs: append([]string(nil), argument.ObjectRefs...),
					Resolution: argument.Resolution, ObjectsObserved: argument.ObjectsObserved,
					ValueCandidates: valueCandidates, ValueCandidatesObserved: argument.ValueCandidatesObserved,
				}
			}
			patterns[patternPosition] = programindex.RelationPatternInput{
				SourceRef: pattern.SourceRef, Form: pattern.Form, Selector: pattern.Selector,
				Location:                 cloneLocation(pattern.Location),
				ResultRef:                pattern.ResultRef,
				ReceiverRef:              pattern.ReceiverRef,
				ReceiverOriginRefs:       append([]string(nil), pattern.ReceiverOriginRefs...),
				ReceiverOriginResolution: pattern.ReceiverOriginResolution,
				ReceiverOriginsObserved:  pattern.ReceiverOriginsObserved,
				Arguments:                arguments, ArgumentsObserved: pattern.ArgumentsObserved,
			}
		}
		relations = append(relations, programindex.RelationInput{
			SourceRef: value.SourceRef, Kind: programindex.RelationKind(value.Kind), FromRef: value.FromRef,
			ToRefs: append([]string(nil), value.ToRefs...), Resolution: programindex.Resolution(value.Resolution),
			Invocation: value.Invocation, Location: cloneLocation(value.Location),
			TargetsObserved: value.TargetsObserved, Witnesses: witnesses,
			WitnessesObserved: value.WitnessesObserved,
			Patterns:          patterns, PatternsObserved: value.PatternsObserved,
			SourceArgument: clonePatternArgumentRefInput(value.SourceArgument),
		})
	}
	return parsedGroup{objects: objects, relations: relations, objectRefs: objectRefs}, nil
}

func clonePatternArgumentRefInput(value *parsedPatternArgumentRef) *programindex.PatternArgumentRefInput {
	if value == nil {
		return nil
	}
	return &programindex.PatternArgumentRefInput{
		RelationSourceRef: value.RelationSourceRef,
		PatternSourceRef:  value.PatternSourceRef,
		Position:          value.Position,
		Keyword:           value.Keyword,
	}
}

func cloneParsedExternalSymbol(value *programindex.ExternalSymbol) *programindex.ExternalSymbol {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func inputForTarget(repository *corpus.Corpus, target pythontarget.Target, parsed parsedGroup) (programindex.Input, error) {
	programTarget, err := projectTarget(repository, target, parsed.objectRefs)
	if err != nil {
		return programindex.Input{}, err
	}
	return programindex.Input{
		ScenarioSHA256: parsed.scenarioSHA256,
		SourceSHA256:   parsed.sourceSHA256,
		Target:         programTarget,
		Objects:        parsed.objects,
		Relations:      parsed.relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(parsed.objects), RelationsObserved: len(parsed.relations),
		},
	}, nil
}

func packageSourceRef(pkg pythontarget.Package) string {
	return stableSourceRef("package", pkg.Name, pkg.Dir, pkg.Path, strconv.FormatBool(pkg.Namespace))
}

func runParser(ctx context.Context, request parserRequest) (parserResponse, error) {
	wire, err := json.Marshal(request)
	if err != nil {
		return parserResponse{}, fmt.Errorf("python program index: encode parser request: %w", err)
	}
	command := exec.CommandContext(ctx, "python3", "-I", "-S", "-c", parserHelper)
	command.Stdin = bytes.NewReader(wire)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	var stderr limitedBuffer
	stderr.limit = maxParserStderrBytes
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return parserResponse{}, fmt.Errorf("python program index: start isolated parser: %w", err)
	}
	waitErr := command.Wait()
	if waitErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return parserResponse{}, ctxErr
		}
		return parserResponse{}, fmt.Errorf(
			"python program index: isolated parser failed: %s", strings.TrimSpace(stderr.String()),
		)
	}
	return decodeParserResponse(stdout.Bytes())
}

func decodeParserResponse(encoded []byte) (parserResponse, error) {
	var response parserResponse
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return parserResponse{}, fmt.Errorf("python program index: decode parser output: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return parserResponse{}, fmt.Errorf("python program index: parser output contains trailing JSON")
	}
	if response.Fatal != "" {
		return parserResponse{}, fmt.Errorf("python program index: %s", response.Fatal)
	}
	if strings.TrimSpace(response.PythonVersion) == "" || response.Views == nil || len(response.Views) == 0 {
		return parserResponse{}, fmt.Errorf("python program index: incomplete parser output")
	}
	for position, view := range response.Views {
		if view.Objects == nil || view.Relations == nil {
			return parserResponse{}, fmt.Errorf("python program index: incomplete parser view %d", position)
		}
	}
	return response, nil
}

func projectTarget(
	repository *corpus.Corpus,
	target pythontarget.Target,
	objects map[string]parsedObject,
) (programindex.TargetInput, error) {
	sources, err := pythonTargetSourceEvidence(repository, target)
	if err != nil {
		return programindex.TargetInput{}, err
	}
	seeds := make([]programindex.TargetSeedInput, 0, 2*len(target.Roots))
	kind := "executable"
	if target.Kind == pythontarget.KindLibrary {
		kind = "library"
	} else {
		for _, root := range target.Roots {
			ref, err := rootObjectRef(root, objects)
			if err != nil {
				return programindex.TargetInput{}, err
			}
			if ref == "" {
				return programindex.TargetInput{}, fmt.Errorf(
					"python program index: target root %s:%d has no exact seed object",
					root.Path, root.Line,
				)
			}
			seedKind, err := pythonSeedKind(root.Kind)
			if err != nil {
				return programindex.TargetInput{}, err
			}
			seeds = append(seeds, programindex.TargetSeedInput{
				ObjectRef: ref,
				Kind:      seedKind,
				Location:  &programindex.Location{Path: root.Path, Line: root.Line, Column: 1},
			})
			if root.Kind == pythontarget.RootBoundObject {
				moduleRef, err := moduleObjectRef(root, objects)
				if err != nil {
					return programindex.TargetInput{}, err
				}
				module := objects[moduleRef]
				if module.Location == nil {
					return programindex.TargetInput{}, fmt.Errorf(
						"python program index: bound target root module %q has no exact location", root.Path,
					)
				}
				seeds = append(seeds, programindex.TargetSeedInput{
					ObjectRef: moduleRef,
					Kind:      programindex.SeedModule,
					Location:  cloneLocation(module.Location),
				})
			}
		}
	}
	if len(sources) == 0 {
		return programindex.TargetInput{}, fmt.Errorf("python program index: target has no exact source refs")
	}
	return programindex.TargetInput{
		Language: "python", Kind: kind, Name: target.DisplayName, Selector: target.Selector,
		Sources: sources, AnchorFileRef: string(target.AnchorFileRef), Seeds: seeds,
	}, nil
}

func pythonTargetSourceEvidence(
	repository *corpus.Corpus,
	target pythontarget.Target,
) ([]programindex.TargetSource, error) {
	if repository == nil {
		return nil, fmt.Errorf("python program index: repository corpus is required")
	}
	pathsByRef := make(map[string]string, len(target.Modules)+len(target.Basis))
	for _, module := range target.Modules {
		if err := bindTargetEvidence(repository, pathsByRef, module.FileID, module.Path, "module"); err != nil {
			return nil, err
		}
	}
	for _, basis := range target.Basis {
		if err := bindTargetEvidence(repository, pathsByRef, basis.FileID, basis.Path, "basis"); err != nil {
			return nil, err
		}
	}
	selected := make(map[string]struct{}, len(target.SourceRefs)+len(target.Basis))
	for _, ref := range target.SourceRefs {
		selected[string(ref)] = struct{}{}
	}
	for _, basis := range target.Basis {
		selected[string(basis.FileID)] = struct{}{}
	}
	sources := make([]programindex.TargetSource, 0, len(selected))
	for ref := range selected {
		sourcePath := pathsByRef[ref]
		if sourcePath == "" {
			return nil, fmt.Errorf(
				"python program index: target evidence ref %q has no repository path", ref,
			)
		}
		if err := verifyCorpusBinding(repository, corpus.FileID(ref), sourcePath, "source"); err != nil {
			return nil, err
		}
		sources = append(sources, programindex.TargetSource{FileRef: ref, Path: sourcePath})
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].FileRef != sources[j].FileRef {
			return sources[i].FileRef < sources[j].FileRef
		}
		return sources[i].Path < sources[j].Path
	})
	if len(sources) == 0 {
		return nil, fmt.Errorf("python program index: target evidence inventory is incomplete")
	}
	anchorPath := pathsByRef[string(target.AnchorFileRef)]
	if anchorPath == "" {
		return nil, fmt.Errorf(
			"python program index: target anchor ref %q has no repository path", target.AnchorFileRef,
		)
	}
	if err := verifyCorpusBinding(repository, target.AnchorFileRef, anchorPath, "anchor"); err != nil {
		return nil, err
	}
	return sources, nil
}

func bindTargetEvidence(
	repository *corpus.Corpus,
	pathsByRef map[string]string,
	fileID corpus.FileID,
	filePath string,
	kind string,
) error {
	ref := string(fileID)
	if previous, exists := pathsByRef[ref]; exists && previous != filePath {
		return fmt.Errorf(
			"python program index: target evidence ref %q has conflicting paths %q and %q",
			ref, previous, filePath,
		)
	}
	if err := verifyCorpusBinding(repository, fileID, filePath, kind); err != nil {
		return err
	}
	pathsByRef[ref] = filePath
	return nil
}

func verifyCorpusBinding(
	repository *corpus.Corpus,
	fileID corpus.FileID,
	filePath string,
	kind string,
) error {
	info, ok := repository.Info(fileID)
	if !ok {
		return fmt.Errorf("python program index: target %s ref %q is outside repository corpus", kind, fileID)
	}
	if info.Entry.Path != filePath {
		return fmt.Errorf(
			"python program index: target %s ref %q resolves to %q, not %q",
			kind, fileID, info.Entry.Path, filePath,
		)
	}
	resolved, ok := repository.ID(filePath)
	if !ok || resolved != fileID {
		return fmt.Errorf(
			"python program index: target %s path %q is not bound to ref %q",
			kind, filePath, fileID,
		)
	}
	return nil
}

func pythonSeedKind(kind pythontarget.RootKind) (programindex.SeedKind, error) {
	switch kind {
	case pythontarget.RootCallable:
		return programindex.SeedCallable, nil
	case pythontarget.RootModule:
		return programindex.SeedModule, nil
	case pythontarget.RootModuleExecution:
		return programindex.SeedModule, nil
	case pythontarget.RootMainGuard:
		return programindex.SeedMainGuard, nil
	case pythontarget.RootScriptFile:
		return programindex.SeedScript, nil
	case pythontarget.RootBoundObject:
		return programindex.SeedBoundObject, nil
	default:
		return "", fmt.Errorf("python program index: unsupported target root kind %q", kind)
	}
}

func rootObjectRef(root pythontarget.Root, objects map[string]parsedObject) (string, error) {
	switch root.Kind {
	case pythontarget.RootModule, pythontarget.RootModuleExecution,
		pythontarget.RootMainGuard, pythontarget.RootScriptFile:
		return moduleObjectRef(root, objects)
	}
	wantName := root.Qualname
	if index := strings.LastIndexByte(wantName, '.'); index >= 0 {
		wantName = wantName[index+1:]
	}
	if wantName == "" {
		return "", fmt.Errorf(
			"python program index: target root %s:%d has no declaration name",
			root.Path, root.Line,
		)
	}
	exact := make([]string, 0, 1)
	for ref, object := range objects {
		if object.Location == nil || object.Location.Path != root.Path || object.Location.Line != root.Line {
			continue
		}
		if object.Name == wantName {
			exact = append(exact, ref)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		sort.Strings(exact)
		return "", fmt.Errorf(
			"python program index: target root %s:%d is ambiguous across %d objects",
			root.Path, root.Line, len(exact),
		)
	}

	return "", fmt.Errorf(
		"python program index: target root %s:%d does not resolve to a declaration object",
		root.Path, root.Line,
	)
}

func moduleObjectRef(root pythontarget.Root, objects map[string]parsedObject) (string, error) {
	modules := make([]string, 0, 1)
	for ref, object := range objects {
		if object.Location != nil && object.Location.Path == root.Path &&
			(object.Kind == string(programindex.ObjectModule) || object.Kind == string(programindex.ObjectPackage)) {
			modules = append(modules, ref)
		}
	}
	if len(modules) == 1 {
		return modules[0], nil
	}
	if len(modules) > 1 {
		sort.Strings(modules)
		return "", fmt.Errorf(
			"python program index: target root file %q has %d module objects",
			root.Path, len(modules),
		)
	}
	return "", fmt.Errorf("python program index: target root file %q has no module object", root.Path)
}

func validateHelperLocation(location *programindex.Location, allowed map[string]struct{}) error {
	if location == nil {
		return nil
	}
	if _, ok := allowed[location.Path]; !ok {
		return fmt.Errorf("location path %q is outside target modules", location.Path)
	}
	if location.Line < 1 || location.Column < 1 {
		return fmt.Errorf("invalid source location %#v", location)
	}
	return nil
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func stableSourceRef(kind string, values ...string) string {
	parts := append([]string{kind}, values...)
	wire, _ := json.Marshal(parts)
	digest := sha256.Sum256(wire)
	return "python-source-" + hex.EncodeToString(digest[:16])
}

func stableDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func hashCanonical(value any) (string, error) {
	wire, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("python program index: encode identity: %w", err)
	}
	return stableDigest(wire), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := buffer.limit - buffer.Len()
	if original > remaining {
		buffer.exceeded = true
	}
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = buffer.Buffer.Write(value)
	}
	return original, nil
}

func (buffer *limitedBuffer) Exceeded() bool {
	return buffer != nil && buffer.exceeded
}
