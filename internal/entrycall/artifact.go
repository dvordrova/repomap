package entrycall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	ResultArtifactFilename         = "entry_call_result.v3.json"
	StatusArtifactFilename         = "entry_call_status.v3.json"
	LegacyV2ResultArtifactFilename = "entry_call_result.v2.json"
	LegacyV2StatusArtifactFilename = "entry_call_status.v2.json"
	legacyV1ResultArtifactFilename = "entry_call_result.v1.json"
	legacyV1StatusArtifactFilename = "entry_call_status.v1.json"
	legacyV2PromptVersion          = "entry-call-compression-prompt-c53df8bbacc9"
	legacyV2ResultVersion          = 2
	legacyV2StatusVersion          = 2
)

var ArtifactFilenames = []string{
	ResultArtifactFilename, StatusArtifactFilename,
	LegacyV2ResultArtifactFilename, LegacyV2StatusArtifactFilename,
	legacyV1ResultArtifactFilename, legacyV1StatusArtifactFilename,
}

type StatusState string

const (
	StatusAccepted        StatusState = "accepted"
	StatusAcceptedPartial StatusState = "accepted_partial"
	StatusRejected        StatusState = "rejected"
	StatusUnavailable     StatusState = "unavailable"
	StatusSkipped         StatusState = "skipped"
)

type StatusReason string

const (
	ReasonNone                 StatusReason = ""
	ReasonNoFamilies           StatusReason = "no_families"
	ReasonSubstrateUnavailable StatusReason = "substrate_unavailable"
	ReasonProviderFailed       StatusReason = "provider_failed"
	ReasonConfigurationFailed  StatusReason = "configuration_failed"
	ReasonOutputLimit          StatusReason = "output_limit"
	ReasonResponseRejected     StatusReason = "response_rejected"
	ReasonCanceled             StatusReason = "canceled"
	ReasonResponsePartial      StatusReason = "response_partial"
	ReasonNoCandidates         StatusReason = "no_candidates"
)

type Status struct {
	Version                     int                      `json:"version"`
	State                       StatusState              `json:"state"`
	Reason                      StatusReason             `json:"reason,omitempty"`
	PromptVersion               string                   `json:"prompt_version"`
	RequestRef                  string                   `json:"request_ref,omitempty"`
	RequestSHA256               string                   `json:"request_sha256,omitempty"`
	SubstrateSHA256             string                   `json:"substrate_sha256,omitempty"`
	RepositoryStateSHA256       string                   `json:"repository_state_sha256,omitempty"`
	ResultSHA256                string                   `json:"result_sha256,omitempty"`
	AdvertisedFamilies          int                      `json:"advertised_families"`
	SelectedFamilies            int                      `json:"selected_families"`
	RejectedFamilies            int                      `json:"rejected_families"`
	AdvertisedSurfaceCandidates int                      `json:"advertised_surface_candidates"`
	SelectedSurfaces            int                      `json:"selected_surfaces"`
	RejectedSurfaces            int                      `json:"rejected_surfaces"`
	SurfaceCandidateCoverage    SurfaceCandidateCoverage `json:"surface_candidate_coverage"`
}

func EncodeResult(result Result) ([]byte, error) {
	copyResult := cloneResult(result)
	sortResult(&copyResult)
	if err := validateResult(copyResult); err != nil {
		return nil, err
	}
	return encodeArtifact("result", copyResult)
}

func DecodeResult(data []byte) (Result, error) {
	version, err := artifactVersion("result", data)
	if err != nil {
		return Result{}, err
	}
	if version == legacyV2ResultVersion {
		return decodeLegacyV2Result(data)
	}
	var result Result
	if err := decodeArtifact("result", data, &result); err != nil {
		return Result{}, err
	}
	if err := validateResult(result); err != nil {
		return Result{}, err
	}
	canonical := cloneResult(result)
	sortResult(&canonical)
	left, _ := json.Marshal(result)
	right, _ := json.Marshal(canonical)
	if !bytes.Equal(left, right) {
		return Result{}, fmt.Errorf("entry call: result is not in canonical order")
	}
	return result, nil
}

func EncodeStatus(status Status) ([]byte, error) {
	if err := validateStatus(status); err != nil {
		return nil, err
	}
	return encodeArtifact("status", status)
}

func DecodeStatus(data []byte) (Status, error) {
	version, err := artifactVersion("status", data)
	if err != nil {
		return Status{}, err
	}
	if version == legacyV2StatusVersion {
		return decodeLegacyV2Status(data)
	}
	var status Status
	if err := decodeArtifact("status", data, &status); err != nil {
		return Status{}, err
	}
	if err := validateStatus(status); err != nil {
		return Status{}, err
	}
	return status, nil
}

type legacyV2Result struct {
	Version               int           `json:"version"`
	PromptVersion         string        `json:"prompt_version"`
	RequestRef            string        `json:"request_ref"`
	RequestSHA256         string        `json:"request_sha256"`
	SubstrateSHA256       string        `json:"substrate_sha256"`
	RepositoryStateSHA256 string        `json:"repository_state_sha256,omitempty"`
	Entries               []ResultEntry `json:"entries"`
}

type legacyV2Status struct {
	Version               int          `json:"version"`
	State                 StatusState  `json:"state"`
	Reason                StatusReason `json:"reason,omitempty"`
	PromptVersion         string       `json:"prompt_version"`
	RequestRef            string       `json:"request_ref,omitempty"`
	RequestSHA256         string       `json:"request_sha256,omitempty"`
	SubstrateSHA256       string       `json:"substrate_sha256,omitempty"`
	RepositoryStateSHA256 string       `json:"repository_state_sha256,omitempty"`
	ResultSHA256          string       `json:"result_sha256,omitempty"`
	AdvertisedFamilies    int          `json:"advertised_families"`
	SelectedFamilies      int          `json:"selected_families"`
	RejectedFamilies      int          `json:"rejected_families"`
}

func decodeLegacyV2Result(data []byte) (Result, error) {
	var legacy legacyV2Result
	if err := decodeArtifact("result", data, &legacy); err != nil {
		return Result{}, err
	}
	if legacy.Version != legacyV2ResultVersion || legacy.PromptVersion != legacyV2PromptVersion {
		return Result{}, fmt.Errorf("entry call: invalid legacy result identity")
	}
	current := Result{
		Version: ResultVersion, PromptVersion: PromptVersion,
		RequestRef: legacy.RequestRef, RequestSHA256: legacy.RequestSHA256,
		SubstrateSHA256: legacy.SubstrateSHA256, RepositoryStateSHA256: legacy.RepositoryStateSHA256,
		Entries: legacy.Entries, SurfaceProposals: []ResultSurfaceProposal{},
		RejectedSurfaceProposals: []RejectedSurfaceProposal{},
	}
	if err := validateResult(current); err != nil {
		return Result{}, err
	}
	canonical := cloneResult(current)
	sortResult(&canonical)
	left, _ := json.Marshal(current.Entries)
	right, _ := json.Marshal(canonical.Entries)
	if !bytes.Equal(left, right) {
		return Result{}, fmt.Errorf("entry call: legacy result is not in canonical order")
	}
	return Result{
		Version: legacy.Version, PromptVersion: legacy.PromptVersion,
		RequestRef: legacy.RequestRef, RequestSHA256: legacy.RequestSHA256,
		SubstrateSHA256: legacy.SubstrateSHA256, RepositoryStateSHA256: legacy.RepositoryStateSHA256,
		Entries: legacy.Entries,
	}, nil
}

func decodeLegacyV2Status(data []byte) (Status, error) {
	var legacy legacyV2Status
	if err := decodeArtifact("status", data, &legacy); err != nil {
		return Status{}, err
	}
	if legacy.Version != legacyV2StatusVersion || legacy.PromptVersion != legacyV2PromptVersion {
		return Status{}, fmt.Errorf("entry call: invalid legacy status identity")
	}
	reason := legacy.Reason
	if legacy.State == StatusSkipped && reason == ReasonNoFamilies {
		reason = ReasonNoCandidates
	}
	current := Status{
		Version: StatusVersion, State: legacy.State, Reason: reason, PromptVersion: PromptVersion,
		RequestRef: legacy.RequestRef, RequestSHA256: legacy.RequestSHA256,
		SubstrateSHA256: legacy.SubstrateSHA256, RepositoryStateSHA256: legacy.RepositoryStateSHA256,
		ResultSHA256: legacy.ResultSHA256, AdvertisedFamilies: legacy.AdvertisedFamilies,
		SelectedFamilies: legacy.SelectedFamilies, RejectedFamilies: legacy.RejectedFamilies,
	}
	if err := validateStatus(current); err != nil {
		return Status{}, err
	}
	return Status{
		Version: legacy.Version, State: legacy.State, Reason: legacy.Reason,
		PromptVersion: legacy.PromptVersion, RequestRef: legacy.RequestRef,
		RequestSHA256: legacy.RequestSHA256, SubstrateSHA256: legacy.SubstrateSHA256,
		RepositoryStateSHA256: legacy.RepositoryStateSHA256, ResultSHA256: legacy.ResultSHA256,
		AdvertisedFamilies: legacy.AdvertisedFamilies, SelectedFamilies: legacy.SelectedFamilies,
		RejectedFamilies: legacy.RejectedFamilies,
	}, nil
}

func artifactVersion(kind string, data []byte) (int, error) {
	if len(data) == 0 || len(data) > MaxArtifactBytes {
		return 0, fmt.Errorf("entry call: %s exceeds bounded artifact", kind)
	}
	if _, found := secretscan.DetectAlways(string(data)); found {
		return 0, fmt.Errorf("entry call: %s contains credential-shaped content", kind)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, fmt.Errorf("entry call: decode %s version: %w", kind, err)
	}
	return header.Version, nil
}

func validateResult(result Result) error {
	if result.Version != ResultVersion || result.PromptVersion != PromptVersion ||
		!validRequestRef(result.RequestRef) || !validDigest(result.RequestSHA256, false) ||
		!validDigest(result.SubstrateSHA256, false) || !validDigest(result.RepositoryStateSHA256, false) ||
		result.Entries == nil || len(result.Entries) == 0 || len(result.Entries) > MaxRoots ||
		result.SurfaceProposals == nil || result.RejectedSurfaceProposals == nil ||
		!validSurfaceCoverage(result.SurfaceCandidateCoverage) {
		return fmt.Errorf("entry call: invalid result identity")
	}
	seenRoots := make(map[string]struct{}, len(result.Entries))
	seenFamilies := make(map[string]struct{})
	for _, entry := range result.Entries {
		if !validRef(entry.RootRef, "r") || sanitizeLabel(entry.Label) != entry.Label ||
			!validLocation(entry.Declaration) || entry.Families == nil || entry.RejectedFamilies == nil || entry.Frontier == nil ||
			len(entry.Families) > MaxSelectedFamiliesPerRoot ||
			len(entry.Families)+len(entry.RejectedFamilies) > MaxFamiliesPerRoot ||
			entry.Omitted.Nodes < 0 || entry.Omitted.Families < 0 || entry.Omitted.Witnesses < 0 {
			return fmt.Errorf("entry call: invalid result entry")
		}
		if _, duplicate := seenRoots[entry.RootRef]; duplicate {
			return fmt.Errorf("entry call: duplicate result root")
		}
		seenRoots[entry.RootRef] = struct{}{}
		for _, frontier := range entry.Frontier {
			if !validRef(frontier.NodeRef, "n") || strings.TrimSpace(frontier.Reason) == "" ||
				frontier.FamilyCount <= 0 || frontier.WitnessCount <= 0 {
				return fmt.Errorf("entry call: invalid result frontier")
			}
		}
		for _, family := range entry.Families {
			if !validRef(family.Ref, "f") || sanitizeLabel(family.CallerLabel) != family.CallerLabel ||
				sanitizeLabel(family.CalleeLabel) != family.CalleeLabel || !family.Invocation.Valid() ||
				family.WitnessCount <= 0 || len(family.Callsites) == 0 ||
				len(family.Callsites) > MaxRepresentativeCallsites {
				return fmt.Errorf("entry call: invalid result family")
			}
			if _, duplicate := seenFamilies[family.Ref]; duplicate {
				return fmt.Errorf("entry call: duplicate result family")
			}
			seenFamilies[family.Ref] = struct{}{}
			for _, callsite := range family.Callsites {
				if !validLocation(callsite) {
					return fmt.Errorf("entry call: invalid result callsite")
				}
			}
		}
		overLimitSelection := len(entry.RejectedFamilies) > MaxSelectedFamiliesPerRoot
		if overLimitSelection && len(entry.Families) != 0 {
			return fmt.Errorf("entry call: invalid over-limit result entry")
		}
		if !overLimitSelection && len(entry.Families)+len(entry.RejectedFamilies) > MaxSelectedFamiliesPerRoot {
			return fmt.Errorf("entry call: invalid bounded result entry")
		}
		for _, rejected := range entry.RejectedFamilies {
			expectedReason := RejectedFamilyUnreachable
			if overLimitSelection {
				expectedReason = RejectedFamilySelectionLimit
			}
			if !validRef(rejected.Ref, "f") || rejected.Reason != expectedReason {
				return fmt.Errorf("entry call: invalid rejected result family")
			}
			if _, duplicate := seenFamilies[rejected.Ref]; duplicate {
				return fmt.Errorf("entry call: duplicate result family")
			}
			seenFamilies[rejected.Ref] = struct{}{}
		}
	}
	seenSurfaceCandidates := make(map[string]struct{})
	seenSurfaceIDs := make(map[string]struct{})
	for _, proposal := range result.SurfaceProposals {
		if !validRef(proposal.CandidateRef, "c") || !validRef(proposal.RootRef, "r") ||
			!validSurfaceProposalID(proposal.ID) || !validLocation(proposal.Site) || !proposal.Form.Valid() {
			return fmt.Errorf("entry call: invalid result surface proposal")
		}
		if _, knownRoot := seenRoots[proposal.RootRef]; !knownRoot {
			return fmt.Errorf("entry call: result surface proposal cites unknown root")
		}
		if _, duplicate := seenSurfaceCandidates[proposal.CandidateRef]; duplicate {
			return fmt.Errorf("entry call: duplicate result surface candidate")
		}
		seenSurfaceCandidates[proposal.CandidateRef] = struct{}{}
		if _, duplicate := seenSurfaceIDs[proposal.ID]; duplicate {
			return fmt.Errorf("entry call: duplicate result surface id")
		}
		seenSurfaceIDs[proposal.ID] = struct{}{}
		if err := validateResultSurfaceProposal(proposal); err != nil {
			return err
		}
	}
	for _, rejected := range result.RejectedSurfaceProposals {
		if !validRef(rejected.CandidateRef, "c") || !rejected.Reason.Valid() {
			return fmt.Errorf("entry call: invalid rejected surface proposal")
		}
		if _, duplicate := seenSurfaceCandidates[rejected.CandidateRef]; duplicate {
			return fmt.Errorf("entry call: duplicate result surface candidate")
		}
		seenSurfaceCandidates[rejected.CandidateRef] = struct{}{}
	}
	if len(seenSurfaceCandidates) > result.SurfaceCandidateCoverage.AdvertisedCandidates ||
		len(result.SurfaceProposals) > MaxSelectedSurfaceProposals {
		return fmt.Errorf("entry call: invalid result surface accounting")
	}
	return nil
}

func (reason RejectedSurfaceReason) Valid() bool {
	switch reason {
	case RejectedSurfaceIncompatibleForm, RejectedSurfaceMissingBinding,
		RejectedSurfaceDuplicateBinding, RejectedSurfaceDuplicateProposal,
		RejectedSurfaceIncompatibleBinding:
		return true
	default:
		return false
	}
}

func validateResultSurfaceProposal(proposal ResultSurfaceProposal) error {
	switch proposal.Kind {
	case SurfaceKindCLICommand:
		if proposal.Role != SurfaceRoleDescriptor || proposal.Form != SurfaceCandidateKeyedComposite ||
			!validResultSurfaceValue(proposal.Identity, SurfaceFactString) ||
			proposal.Method != nil || proposal.Path != nil ||
			(proposal.Handler != nil && !validResultSurfaceValue(proposal.Handler, SurfaceFactCallable)) {
			return fmt.Errorf("entry call: invalid CLI surface proposal")
		}
	case SurfaceKindHTTPRoute:
		if proposal.Role != SurfaceRoleEntrySurface || proposal.Form != SurfaceCandidateDirectCall ||
			proposal.Identity != nil || !validResultSurfaceValueEither(proposal.Method, SurfaceFactString, SurfaceFactToken) ||
			!validResultSurfaceValue(proposal.Path, SurfaceFactString) ||
			!validResultSurfaceValue(proposal.Handler, SurfaceFactCallable) {
			return fmt.Errorf("entry call: invalid HTTP surface proposal")
		}
	default:
		return fmt.Errorf("entry call: invalid result surface kind")
	}
	return nil
}

func validResultSurfaceValue(value *ResultSurfaceValue, kind SurfaceFactKind) bool {
	return value != nil && value.Kind == kind && strings.TrimSpace(value.Text) != "" &&
		sanitizeSurfaceValue(value.Text) == value.Text && value.Location != nil && validLocation(*value.Location)
}

func validResultSurfaceValueEither(value *ResultSurfaceValue, left, right SurfaceFactKind) bool {
	return validResultSurfaceValue(value, left) || validResultSurfaceValue(value, right)
}

func validateStatus(status Status) error {
	if status.Version != StatusVersion || status.PromptVersion != PromptVersion ||
		status.AdvertisedFamilies < 0 || status.AdvertisedFamilies > MaxFamilies ||
		status.SelectedFamilies < 0 || status.SelectedFamilies > MaxRoots*MaxSelectedFamiliesPerRoot ||
		status.SelectedFamilies+status.RejectedFamilies > status.AdvertisedFamilies ||
		status.RejectedFamilies < 0 || status.AdvertisedSurfaceCandidates < 0 ||
		status.AdvertisedSurfaceCandidates > MaxSurfaceCandidates || status.SelectedSurfaces < 0 ||
		status.RejectedSurfaces < 0 || status.SelectedSurfaces+status.RejectedSurfaces > status.AdvertisedSurfaceCandidates ||
		!validSurfaceCoverage(status.SurfaceCandidateCoverage) ||
		status.AdvertisedSurfaceCandidates != status.SurfaceCandidateCoverage.AdvertisedCandidates ||
		!validDigest(status.RepositoryStateSHA256, false) {
		return fmt.Errorf("entry call: invalid status")
	}
	switch status.State {
	case StatusAccepted:
		if status.Reason != ReasonNone || status.RejectedFamilies != 0 || status.RejectedSurfaces != 0 ||
			!validRequestBinding(status) || !validDigest(status.ResultSHA256, false) {
			return fmt.Errorf("entry call: invalid accepted status")
		}
	case StatusAcceptedPartial:
		if status.Reason != ReasonResponsePartial || status.RejectedFamilies+status.RejectedSurfaces == 0 ||
			!validRequestBinding(status) || !validDigest(status.ResultSHA256, false) {
			return fmt.Errorf("entry call: invalid accepted-partial status")
		}
	case StatusRejected:
		if status.Reason != ReasonProviderFailed && status.Reason != ReasonConfigurationFailed &&
			status.Reason != ReasonOutputLimit && status.Reason != ReasonResponseRejected &&
			status.Reason != ReasonCanceled {
			return fmt.Errorf("entry call: invalid rejected status reason")
		}
		if !validRequestBinding(status) || status.ResultSHA256 != "" ||
			status.SelectedFamilies != 0 || status.RejectedFamilies != 0 ||
			status.SelectedSurfaces != 0 || status.RejectedSurfaces != 0 {
			return fmt.Errorf("entry call: invalid rejected status binding")
		}
	case StatusSkipped:
		if status.Reason != ReasonNoCandidates || !validRequestBinding(status) || status.AdvertisedFamilies != 0 ||
			status.AdvertisedSurfaceCandidates != 0 || status.SelectedFamilies != 0 || status.RejectedFamilies != 0 ||
			status.SelectedSurfaces != 0 || status.RejectedSurfaces != 0 || status.ResultSHA256 != "" {
			return fmt.Errorf("entry call: invalid skipped status")
		}
	case StatusUnavailable:
		if status.Reason != ReasonSubstrateUnavailable || status.RequestRef != "" || status.RequestSHA256 != "" ||
			status.ResultSHA256 != "" || status.AdvertisedFamilies != 0 || status.SelectedFamilies != 0 ||
			status.RejectedFamilies != 0 || status.AdvertisedSurfaceCandidates != 0 ||
			status.SelectedSurfaces != 0 || status.RejectedSurfaces != 0 ||
			(status.SubstrateSHA256 != "" && !validDigest(status.SubstrateSHA256, false)) {
			return fmt.Errorf("entry call: invalid unavailable status")
		}
	default:
		return fmt.Errorf("entry call: invalid status state")
	}
	return nil
}

func validRequestBinding(status Status) bool {
	return validRequestRef(status.RequestRef) && validDigest(status.RequestSHA256, false) &&
		validDigest(status.SubstrateSHA256, false)
}

func validRequestRef(value string) bool {
	if len(value) != len("q-")+16 || !strings.HasPrefix(value, "q-") {
		return false
	}
	for _, character := range value[len("q-"):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validRef(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) {
		return false
	}
	nonzero := false
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' {
			return false
		}
		if character != '0' {
			nonzero = true
		}
	}
	return nonzero
}

func encodeArtifact(kind string, value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("entry call: encode %s: %w", kind, err)
	}
	if len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("entry call: %s exceeds bounded artifact", kind)
	}
	if _, found := secretscan.DetectAlways(string(encoded)); found {
		return nil, fmt.Errorf("entry call: %s contains credential-shaped content", kind)
	}
	return encoded, nil
}

func decodeArtifact(kind string, data []byte, target any) error {
	if len(data) == 0 || len(data) > MaxArtifactBytes {
		return fmt.Errorf("entry call: %s exceeds bounded artifact", kind)
	}
	if _, found := secretscan.DetectAlways(string(data)); found {
		return fmt.Errorf("entry call: %s contains credential-shaped content", kind)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("entry call: decode %s: %w", kind, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("entry call: %s contains trailing JSON", kind)
	}
	return nil
}

func cloneResult(result Result) Result {
	copyResult := result
	copyResult.Entries = append([]ResultEntry{}, result.Entries...)
	copyResult.SurfaceProposals = append([]ResultSurfaceProposal{}, result.SurfaceProposals...)
	copyResult.RejectedSurfaceProposals = append(
		[]RejectedSurfaceProposal{}, result.RejectedSurfaceProposals...,
	)
	for index := range copyResult.SurfaceProposals {
		copyResult.SurfaceProposals[index].Identity = cloneResultSurfaceValue(result.SurfaceProposals[index].Identity)
		copyResult.SurfaceProposals[index].Method = cloneResultSurfaceValue(result.SurfaceProposals[index].Method)
		copyResult.SurfaceProposals[index].Path = cloneResultSurfaceValue(result.SurfaceProposals[index].Path)
		copyResult.SurfaceProposals[index].Handler = cloneResultSurfaceValue(result.SurfaceProposals[index].Handler)
	}
	for index := range copyResult.Entries {
		copyResult.Entries[index].Families = append([]ResultFamily{}, result.Entries[index].Families...)
		copyResult.Entries[index].RejectedFamilies = append(
			[]RejectedResultFamily{}, result.Entries[index].RejectedFamilies...,
		)
		copyResult.Entries[index].Frontier = append([]RequestFrontier{}, result.Entries[index].Frontier...)
		for familyIndex := range copyResult.Entries[index].Families {
			copyResult.Entries[index].Families[familyIndex].Callsites = append(
				[]Location(nil), result.Entries[index].Families[familyIndex].Callsites...,
			)
			sortLocations(copyResult.Entries[index].Families[familyIndex].Callsites)
		}
		sort.Slice(copyResult.Entries[index].Frontier, func(i, j int) bool {
			left, right := copyResult.Entries[index].Frontier[i], copyResult.Entries[index].Frontier[j]
			if left.NodeRef != right.NodeRef {
				return refLess(left.NodeRef, right.NodeRef)
			}
			return left.Reason < right.Reason
		})
	}
	return copyResult
}

func cloneResultSurfaceValue(value *ResultSurfaceValue) *ResultSurfaceValue {
	if value == nil {
		return nil
	}
	copyValue := *value
	if value.Location != nil {
		location := *value.Location
		copyValue.Location = &location
	}
	return &copyValue
}
