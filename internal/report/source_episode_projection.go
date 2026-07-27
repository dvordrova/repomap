package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

const (
	maxSourceEpisodeBytes         = 64 << 10
	maxSourceEpisodeAnchors       = 32
	maxSourceEpisodeClaims        = 16
	maxSourceEpisodeUncertainties = 8
	maxSourceEpisodeReferences    = 8
	maxSourceEpisodeIDBytes       = 192
	maxSourceEpisodeQuestionBytes = 1024
	maxSourceEpisodeTitleBytes    = 320
	maxSourceEpisodeTextBytes     = 4096
	maxSourceEpisodePathBytes     = 512
)

type sourceEpisodeApproval struct {
	episodeID  string
	repository string
	revision   string
}

var approvedSourceEpisodes = map[string]sourceEpisodeApproval{
	"1f41085eea5fc0c59ddbb7ae66b7e3a67c82b8b588babd97edfe71ec873aa21a": {
		episodeID:  "etcd-put-recoverability",
		repository: "etcd-io/etcd",
		revision:   "58f45a9ff1c083130830eb02b0cc7d9783609095",
	},
	"9599553a777e8d8fd582bb1874dd4ab534c1f24d9d87e82cfce09cc775281665": {
		episodeID:  "django-nested-atomic",
		repository: "django/django",
		revision:   "3e389b7ddaf08109900da5415ddaac5a355a170f",
	},
}

type sourceEpisodeInput struct {
	ArtifactKind    string                     `json:"artifact_kind"`
	ArtifactVersion string                     `json:"artifact_version"`
	EpisodeID       string                     `json:"episode_id"`
	Repository      sourceEpisodeRepository    `json:"repository"`
	Question        string                     `json:"question"`
	Anchors         []sourceEpisodeAnchor      `json:"anchors"`
	Claims          []sourceEpisodeClaim       `json:"claims"`
	Uncertainties   []sourceEpisodeUncertainty `json:"uncertainties"`
}

type sourceEpisodeRepository struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type sourceEpisodeAnchor struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type sourceEpisodeClaim struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Strength  string   `json:"strength"`
	Title     string   `json:"title"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
	Limits    []string `json:"limits"`
}

type sourceEpisodeUncertainty struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Statement string   `json:"statement"`
	AnchorIDs []string `json:"anchor_ids"`
}

type sourceEpisodeProjection struct {
	EpisodeID     string                        `json:"episode_id"`
	Repository    string                        `json:"repository"`
	Revision      string                        `json:"revision"`
	Question      string                        `json:"question"`
	Claims        []sourceEpisodeProjectedClaim `json:"claims"`
	Uncertainties []sourceEpisodeProjectedGap   `json:"uncertainties,omitempty"`
}

type sourceEpisodeProjectedClaim struct {
	State     string                `json:"state"`
	Strength  string                `json:"strength,omitempty"`
	Title     string                `json:"title"`
	Statement string                `json:"statement"`
	Limits    []string              `json:"limits,omitempty"`
	Sources   []sourceEpisodeSource `json:"sources,omitempty"`
}

type sourceEpisodeProjectedGap struct {
	State     string                `json:"state"`
	Statement string                `json:"statement"`
	Sources   []sourceEpisodeSource `json:"sources,omitempty"`
}

type sourceEpisodeSource struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func projectApprovedSourceEpisode(data *ReportData, raw []byte) (*sourceEpisodeProjection, error) {
	if data == nil {
		return nil, fmt.Errorf("report: source episode requires report data")
	}
	if len(raw) == 0 || len(raw) > maxSourceEpisodeBytes {
		return nil, fmt.Errorf("report: source episode input is outside the byte budget")
	}
	digest := sha256.Sum256(raw)
	approval, ok := approvedSourceEpisodes[hex.EncodeToString(digest[:])]
	if !ok {
		return nil, fmt.Errorf("report: source episode is not an approved pinned fixture")
	}
	if strings.TrimSpace(data.CapturedRevision) != approval.revision {
		return nil, fmt.Errorf("report: source episode revision does not match the rendered report")
	}

	var episode sourceEpisodeInput
	if err := json.Unmarshal(raw, &episode); err != nil {
		return nil, fmt.Errorf("report: decode source episode: %w", err)
	}
	if err := validateSourceEpisode(episode, approval); err != nil {
		return nil, err
	}

	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		if filePath = strings.TrimSpace(filePath); filePath != "" {
			openable[filePath] = struct{}{}
		}
	}
	anchors := make(map[string]sourceEpisodeAnchor, len(episode.Anchors))
	for _, anchor := range episode.Anchors {
		anchors[anchor.ID] = anchor
	}

	claims := make([]sourceEpisodeProjectedClaim, 0, len(episode.Claims))
	for _, claim := range episode.Claims {
		claims = append(claims, sourceEpisodeProjectedClaim{
			State:     claim.State,
			Strength:  claim.Strength,
			Title:     claim.Title,
			Statement: claim.Statement,
			Limits:    append([]string(nil), claim.Limits...),
			Sources:   authorizedSourceEpisodeSources(claim.AnchorIDs, anchors, openable, data.SourceIDs),
		})
	}
	uncertainties := make([]sourceEpisodeProjectedGap, 0, len(episode.Uncertainties))
	for _, uncertainty := range episode.Uncertainties {
		uncertainties = append(uncertainties, sourceEpisodeProjectedGap{
			State:     uncertainty.State,
			Statement: uncertainty.Statement,
			Sources:   authorizedSourceEpisodeSources(uncertainty.AnchorIDs, anchors, openable, data.SourceIDs),
		})
	}

	return &sourceEpisodeProjection{
		EpisodeID:     episode.EpisodeID,
		Repository:    episode.Repository.Name,
		Revision:      episode.Repository.Revision,
		Question:      episode.Question,
		Claims:        claims,
		Uncertainties: uncertainties,
	}, nil
}

func validateSourceEpisode(episode sourceEpisodeInput, approval sourceEpisodeApproval) error {
	if episode.ArtifactKind != "source-episode-microexperiment" || episode.ArtifactVersion != "1" {
		return fmt.Errorf("report: source episode has an unsupported artifact contract")
	}
	if episode.EpisodeID != approval.episodeID ||
		episode.Repository.Name != approval.repository ||
		episode.Repository.Revision != approval.revision {
		return fmt.Errorf("report: source episode identity does not match its approval")
	}
	if !boundedSourceEpisodeText(episode.EpisodeID, maxSourceEpisodeIDBytes) ||
		!boundedSourceEpisodeText(episode.Repository.Name, maxSourceEpisodeIDBytes) ||
		!boundedSourceEpisodeText(episode.Question, maxSourceEpisodeQuestionBytes) ||
		len(episode.Repository.Revision) != 40 {
		return fmt.Errorf("report: source episode contains invalid identity text")
	}
	if _, err := hex.DecodeString(episode.Repository.Revision); err != nil {
		return fmt.Errorf("report: source episode revision is invalid")
	}
	if len(episode.Anchors) == 0 || len(episode.Anchors) > maxSourceEpisodeAnchors ||
		len(episode.Claims) == 0 || len(episode.Claims) > maxSourceEpisodeClaims ||
		len(episode.Uncertainties) > maxSourceEpisodeUncertainties {
		return fmt.Errorf("report: source episode collection is outside its budget")
	}

	anchorIDs := make(map[string]struct{}, len(episode.Anchors))
	for _, anchor := range episode.Anchors {
		if !boundedSourceEpisodeText(anchor.ID, maxSourceEpisodeIDBytes) ||
			!safeSourceEpisodePath(anchor.Path) ||
			anchor.StartLine <= 0 || anchor.EndLine < anchor.StartLine {
			return fmt.Errorf("report: source episode contains an invalid anchor")
		}
		if _, exists := anchorIDs[anchor.ID]; exists {
			return fmt.Errorf("report: source episode contains duplicate anchor IDs")
		}
		anchorIDs[anchor.ID] = struct{}{}
	}

	claimIDs := make(map[string]struct{}, len(episode.Claims))
	for _, claim := range episode.Claims {
		if !boundedSourceEpisodeText(claim.ID, maxSourceEpisodeIDBytes) ||
			!validSourceEpisodeState(claim.State) ||
			!boundedSourceEpisodeText(claim.Strength, maxSourceEpisodeIDBytes) ||
			!boundedSourceEpisodeText(claim.Title, maxSourceEpisodeTitleBytes) ||
			!boundedSourceEpisodeText(claim.Statement, maxSourceEpisodeTextBytes) ||
			!validSourceEpisodeReferenceCount(claim.State, len(claim.AnchorIDs)) ||
			len(claim.Limits) > maxSourceEpisodeReferences {
			return fmt.Errorf("report: source episode contains an invalid claim")
		}
		if _, exists := claimIDs[claim.ID]; exists {
			return fmt.Errorf("report: source episode contains duplicate claim IDs")
		}
		claimIDs[claim.ID] = struct{}{}
		if !validSourceEpisodeReferences(claim.AnchorIDs, anchorIDs) ||
			!boundedSourceEpisodeTexts(claim.Limits, maxSourceEpisodeTextBytes) {
			return fmt.Errorf("report: source episode claim references are invalid")
		}
	}

	uncertaintyIDs := make(map[string]struct{}, len(episode.Uncertainties))
	for _, uncertainty := range episode.Uncertainties {
		if !boundedSourceEpisodeText(uncertainty.ID, maxSourceEpisodeIDBytes) ||
			!validSourceEpisodeState(uncertainty.State) ||
			!boundedSourceEpisodeText(uncertainty.Statement, maxSourceEpisodeTextBytes) ||
			!validSourceEpisodeReferenceCount(uncertainty.State, len(uncertainty.AnchorIDs)) ||
			!validSourceEpisodeReferences(uncertainty.AnchorIDs, anchorIDs) {
			return fmt.Errorf("report: source episode contains an invalid uncertainty")
		}
		if _, exists := uncertaintyIDs[uncertainty.ID]; exists {
			return fmt.Errorf("report: source episode contains duplicate uncertainty IDs")
		}
		uncertaintyIDs[uncertainty.ID] = struct{}{}
	}
	return nil
}

func authorizedSourceEpisodeSources(
	ids []string,
	anchors map[string]sourceEpisodeAnchor,
	openable map[string]struct{},
	sourceIDs map[string]string,
) []sourceEpisodeSource {
	sources := make([]sourceEpisodeSource, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		anchor := anchors[id]
		if _, ok := openable[anchor.Path]; !ok || strings.TrimSpace(sourceIDs[anchor.Path]) == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", anchor.Path, anchor.StartLine, anchor.EndLine)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, sourceEpisodeSource{
			Path:      anchor.Path,
			StartLine: anchor.StartLine,
			EndLine:   anchor.EndLine,
		})
	}
	return sources
}

func validSourceEpisodeReferences(ids []string, known map[string]struct{}) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

// Weak signals stay publishable without a code anchor. Their state is the
// disclosure; an anchor adds inspectable context but is not proof. Stronger
// extracted and corroborated claims retain at least one contextual anchor.
func validSourceEpisodeReferenceCount(state string, count int) bool {
	if count < 0 || count > maxSourceEpisodeReferences {
		return false
	}
	return count > 0 || state == "inferred" || state == "unknown"
}

func boundedSourceEpisodeTexts(values []string, limit int) bool {
	for _, value := range values {
		if !boundedSourceEpisodeText(value, limit) {
			return false
		}
	}
	return true
}

func boundedSourceEpisodeText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit
}

func validSourceEpisodeState(state string) bool {
	switch state {
	case "extracted", "corroborated", "inferred", "unknown":
		return true
	default:
		return false
	}
}

func safeSourceEpisodePath(value string) bool {
	if value == "" || len(value) > maxSourceEpisodePathBytes || strings.Contains(value, "\\") ||
		path.IsAbs(value) || path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../") {
		return false
	}
	return true
}
