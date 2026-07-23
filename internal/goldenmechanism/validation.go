package goldenmechanism

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

var artifactIDPattern = regexp.MustCompile(`^gm-(?:file|fn|src|ev|obs)-[0-9a-f]{24}$`)

// Validate checks the local artifact shape and every opaque reference. It does
// not upgrade syntax evidence into a runtime claim.
func (result Result) Validate() error {
	if result.Version != Version {
		return fmt.Errorf("golden mechanism: unsupported result version %d", result.Version)
	}
	if !mechanismIDPattern.MatchString(result.MechanismID) {
		return fmt.Errorf("golden mechanism: result has invalid mechanism id %q", result.MechanismID)
	}
	if len(result.Seeds) < minSeeds || len(result.Seeds) > maxSeeds {
		return fmt.Errorf("golden mechanism: result seed count is outside %d..%d", minSeeds, maxSeeds)
	}
	if result.Partial && !validStopReason(result.StopReason) {
		return fmt.Errorf("golden mechanism: partial result requires a valid stop reason")
	}
	if !result.Partial && result.StopReason != "" {
		return fmt.Errorf("golden mechanism: complete result has stop reason %q", result.StopReason)
	}

	files := make(map[string]File, len(result.Files))
	fileIDs := make(map[string]struct{}, len(result.Files))
	parsedBytes := 0
	for index, file := range result.Files {
		if !artifactIDPattern.MatchString(file.ID) || !validGoPath(file.Path) || file.SHA256 == "" || file.Bytes <= 0 || file.Package == "" {
			return fmt.Errorf("golden mechanism: files[%d] is incomplete", index)
		}
		if _, exists := files[file.Path]; exists {
			return fmt.Errorf("golden mechanism: duplicate file path %q", file.Path)
		}
		if _, exists := fileIDs[file.ID]; exists {
			return fmt.Errorf("golden mechanism: duplicate file id %q", file.ID)
		}
		files[file.Path] = file
		fileIDs[file.ID] = struct{}{}
		parsedBytes += file.Bytes
	}

	functions := make(map[string]Function, len(result.Functions))
	includedBytes := 0
	maxDepth := 0
	for index, function := range result.Functions {
		if !artifactIDPattern.MatchString(function.ID) || function.Symbol == "" || !validGoPath(function.Path) {
			return fmt.Errorf("golden mechanism: functions[%d] identity is incomplete", index)
		}
		if _, exists := files[function.Path]; !exists {
			return fmt.Errorf("golden mechanism: function %q references unparsed path %q", function.ID, function.Path)
		}
		if _, exists := functions[function.ID]; exists {
			return fmt.Errorf("golden mechanism: duplicate function id %q", function.ID)
		}
		if function.Location.Path != function.Path || function.Location.Line <= 0 || function.Location.Column <= 0 ||
			function.Location.EndLine < function.Location.Line || function.Location.EndColumn <= 0 {
			return fmt.Errorf("golden mechanism: function %q has invalid location", function.ID)
		}
		if function.Depth < 0 || function.Depth > hardMaxDepth || len(function.Source) == 0 || function.SourceStopReason == "" {
			return fmt.Errorf("golden mechanism: function %q has invalid depth or source window", function.ID)
		}
		if function.Seed && (len(function.OriginFactIDs) == 0 || len(function.OriginEvidenceIDs) == 0) {
			return fmt.Errorf("golden mechanism: seed function %q lost its origin ids", function.ID)
		}
		if !sort.StringsAreSorted(function.OriginFactIDs) || !sort.StringsAreSorted(function.OriginEvidenceIDs) ||
			!sort.StringsAreSorted(function.ReachedFromIDs) ||
			!sort.StringsAreSorted(function.PlannedFromEvidenceIDs) {
			return fmt.Errorf("golden mechanism: function %q ids are not sorted", function.ID)
		}
		previousLine := 0
		for sourceIndex, line := range function.Source {
			if !artifactIDPattern.MatchString(line.ID) || line.Location.Path != function.Path || line.Location.Line <= previousLine ||
				line.Location.Column != 1 || line.Location.EndLine != line.Location.Line || line.Location.EndColumn <= 0 {
				return fmt.Errorf("golden mechanism: function %q source[%d] is invalid", function.ID, sourceIndex)
			}
			includedBytes += len(line.Text)
			if sourceIndex > 0 {
				includedBytes++
			}
			previousLine = line.Location.Line
		}
		functions[function.ID] = function
		if function.Depth > maxDepth {
			maxDepth = function.Depth
		}
	}

	resolvedSeeds := 0
	for index, resolution := range result.Seeds {
		if !validGoPath(resolution.Seed.Path) || resolution.Seed.Symbol == "" ||
			resolution.Seed.OriginFactID == "" || resolution.Seed.OriginEvidenceID == "" || !validSeedStatus(resolution.Status) {
			return fmt.Errorf("golden mechanism: seeds[%d] is invalid", index)
		}
		if resolution.Status == SeedResolved {
			resolvedSeeds++
			function, exists := functions[resolution.FunctionID]
			if !exists || !function.Seed || function.Path != resolution.Seed.Path || function.Symbol != resolution.Seed.Symbol {
				return fmt.Errorf("golden mechanism: resolved seed %q has no matching included function", resolution.Seed.Symbol)
			}
		} else if resolution.FunctionID != "" {
			return fmt.Errorf("golden mechanism: skipped seed %q has a function id", resolution.Seed.Symbol)
		}
	}

	observations := make(map[string]Observation, len(result.Observations))
	for index, observation := range result.Observations {
		if !artifactIDPattern.MatchString(observation.ID) || !validCapability(observation.Capability) ||
			observation.Operation == "" || observation.Statement == "" || !validSyntaxBasis(observation.Basis) || len(observation.Evidence) == 0 {
			return fmt.Errorf("golden mechanism: observations[%d] is incomplete", index)
		}
		if _, exists := functions[observation.FunctionID]; !exists {
			return fmt.Errorf("golden mechanism: observation %q references unknown function %q", observation.ID, observation.FunctionID)
		}
		if _, exists := observations[observation.ID]; exists {
			return fmt.Errorf("golden mechanism: duplicate observation id %q", observation.ID)
		}
		if observation.TargetFunctionID != "" && (!artifactIDPattern.MatchString(observation.TargetFunctionID) || observation.TargetSymbol == "") {
			return fmt.Errorf("golden mechanism: observation %q has invalid direct-call target", observation.ID)
		}
		for evidenceIndex, ref := range observation.Evidence {
			if !artifactIDPattern.MatchString(ref.ID) || !validGoPath(ref.Location.Path) || ref.Location.Line <= 0 || ref.Location.Column <= 0 ||
				ref.Location.EndLine < ref.Location.Line || ref.Location.EndColumn <= 0 {
				return fmt.Errorf("golden mechanism: observation %q evidence[%d] is invalid", observation.ID, evidenceIndex)
			}
			if _, exists := files[ref.Location.Path]; !exists {
				return fmt.Errorf("golden mechanism: observation %q evidence references unparsed path", observation.ID)
			}
		}
		observations[observation.ID] = observation
	}
	for _, observation := range result.Observations {
		for _, relatedID := range observation.RelatedObservationIDs {
			if _, exists := observations[relatedID]; !exists {
				return fmt.Errorf("golden mechanism: observation %q references unknown observation %q", observation.ID, relatedID)
			}
		}
	}
	for _, function := range result.Functions {
		for _, reachedFromID := range function.ReachedFromIDs {
			if _, exists := observations[reachedFromID]; !exists {
				return fmt.Errorf("golden mechanism: function %q references unknown frontier observation %q", function.ID, reachedFromID)
			}
		}
	}

	if result.Budget.SeedCount != len(result.Seeds) || result.Budget.ResolvedSeedCount != resolvedSeeds ||
		result.Budget.FilesParsed != len(result.Files) || result.Budget.ParsedSourceBytes != parsedBytes ||
		result.Budget.FunctionsIncluded != len(result.Functions) || result.Budget.IncludedSourceBytes != includedBytes ||
		result.Budget.Observations != len(result.Observations) || result.Budget.MaxDepthReached != maxDepth || result.Budget.ElapsedMillis < 0 {
		return fmt.Errorf("golden mechanism: budget stats do not match result contents")
	}
	return nil
}

func validStopReason(reason StopReason) bool {
	switch reason {
	case StopFileLimit, StopSourceByteLimit, StopFunctionLimit, StopDepthLimit,
		StopFunctionLineLimit, StopFunctionByteLimit, StopTimeout:
		return true
	default:
		return false
	}
}

func validSeedStatus(status SeedStatus) bool {
	switch status {
	case SeedResolved, SeedSkippedFileLimit, SeedSkippedByteLimit, SeedSkippedFunctionLimit, SeedSkippedTimeout:
		return true
	default:
		return false
	}
}

func validSyntaxBasis(basis SyntaxBasis) bool {
	switch basis {
	case BasisDeclaration, BasisDirectCall, BasisBranch, BasisRead, BasisAssignment,
		BasisTransform, BasisOutput, BasisReturn, BasisErrorHandoff, BasisLexicalOrder:
		return true
	default:
		return false
	}
}

func validCapability(capability semanticdiscovery.Capability) bool {
	switch capability {
	case semanticdiscovery.CapabilityStatic,
		semanticdiscovery.CapabilityEntry,
		semanticdiscovery.CapabilityDirectCall,
		semanticdiscovery.CapabilitySequence,
		semanticdiscovery.CapabilityBranch,
		semanticdiscovery.CapabilityDataRead,
		semanticdiscovery.CapabilityDataWrite,
		semanticdiscovery.CapabilityDataTransformation,
		semanticdiscovery.CapabilityOutputEffect,
		semanticdiscovery.CapabilityErrorPath:
		return true
	default:
		return false
	}
}
