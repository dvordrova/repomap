package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

const (
	runArtifactManifestVersion  = 1
	runArtifactManifestName     = "run_artifact_manifest.json"
	maxRunArtifactManifestBytes = 1 << 20
)

type runArtifacts struct {
	graphJSON           []byte
	rawSource           []byte
	evaluationJSON      []byte
	parseWarnings       []byte
	deepseekRequest     []byte
	sourceProvider      string
	sourceModel         string
	sourcePromptVersion string
}

type runArtifactManifest struct {
	Version   int                         `json:"version"`
	Artifacts map[string]runArtifactEntry `json:"artifacts"`
}

type runArtifactEntry struct {
	LineageSHA256    string `json:"lineage_sha256"`
	ContentSHA256    string `json:"content_sha256"`
	CallGroupSHA256  string `json:"call_group_sha256,omitempty"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptVersion    string `json:"prompt_version,omitempty"`
	ParserVersion    int    `json:"parser_version,omitempty"`
	EvaluatorVersion int    `json:"evaluator_version,omitempty"`
}

func writeRun(dir string, checkpoint memory.Input, artifacts runArtifacts, preserveRunArtifacts bool) error {
	session := checkpoint.Session
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	var priorManifest runArtifactManifest
	if preserveRunArtifacts {
		var err error
		priorManifest, err = readRunArtifactManifest(dir)
		if err != nil {
			return err
		}
	}
	rawArtifacts := map[string][]byte{
		"evidence_graph.json":                   artifacts.graphJSON,
		"deepseek_source_request.redacted.json": artifacts.deepseekRequest,
		"deepseek_source_response.raw.txt":      artifacts.rawSource,
		"source_evaluation.json":                artifacts.evaluationJSON,
		"source_parse_warnings.json":            artifacts.parseWarnings,
	}
	sourceCallGroup, err := sourceArtifactCallGroup(rawArtifacts, artifacts)
	if err != nil {
		return err
	}
	values := map[string]any{}
	if session.Symbol != nil {
		values["symbol_bundle.json"] = session.Symbol
	}
	if session.Source != nil {
		values["source_card.json"] = session.Source
	}
	if session.Assessment != nil {
		values["source_assessment_bundle.json"] = session.Assessment
	}
	if session.SourceReport != nil {
		values["source_report.json"] = session.SourceReport
	}
	if session.Tests != nil {
		values["test_evidence.json"] = session.Tests
	}
	for name, value := range values {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		if err := writeArtifact(dir, name, data); err != nil {
			return err
		}
	}
	nextManifest := runArtifactManifest{
		Version:   runArtifactManifestVersion,
		Artifacts: make(map[string]runArtifactEntry),
	}
	for name, data := range rawArtifacts {
		if len(data) > 0 {
			lineage, err := runArtifactLineage(session, name)
			if err != nil {
				return err
			}
			payload := artifactPayload(data)
			if err := writeArtifactPayload(dir, name, payload); err != nil {
				return err
			}
			entry := runArtifactEntry{
				LineageSHA256: lineage,
				ContentSHA256: sha256Hex(payload),
			}
			if isSourceRunArtifactName(name) {
				entry.CallGroupSHA256 = sourceCallGroup
				entry.Provider = artifacts.sourceProvider
				entry.Model = artifacts.sourceModel
				entry.PromptVersion = artifacts.sourcePromptVersion
				entry.ParserVersion = sourceexplain.ParserVersion
				entry.EvaluatorVersion = sourceexplain.EvaluationVersion
			}
			nextManifest.Artifacts[name] = entry
			continue
		}
		if preserveRunArtifacts {
			entry, reusable, err := reusableRunArtifact(dir, name, session, priorManifest)
			if err != nil {
				return err
			}
			if reusable {
				nextManifest.Artifacts[name] = entry
				continue
			}
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", name, err)
		}
	}
	for _, name := range sessionArtifactNames {
		_, hasValue := values[name]
		if hasValue {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", name, err)
		}
	}
	if len(nextManifest.Artifacts) == 0 {
		if err := os.Remove(filepath.Join(dir, runArtifactManifestName)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", runArtifactManifestName, err)
		}
	} else {
		manifestJSON, err := json.MarshalIndent(nextManifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", runArtifactManifestName, err)
		}
		if err := writeArtifact(dir, runArtifactManifestName, manifestJSON); err != nil {
			return err
		}
	}
	_, err = memory.Save(dir, checkpoint)
	return err
}

func readRunArtifactManifest(dir string) (runArtifactManifest, error) {
	path := filepath.Join(dir, runArtifactManifestName)
	data, err := readLimitedFile(path, maxRunArtifactManifestBytes)
	if os.IsNotExist(err) {
		return runArtifactManifest{}, nil
	}
	if err != nil {
		return runArtifactManifest{}, fmt.Errorf("read %s: %w", runArtifactManifestName, err)
	}
	var manifest runArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return runArtifactManifest{}, fmt.Errorf("decode %s: %w", runArtifactManifestName, err)
	}
	if manifest.Version != runArtifactManifestVersion || manifest.Artifacts == nil {
		return runArtifactManifest{}, fmt.Errorf("invalid %s", runArtifactManifestName)
	}
	var sourceIdentity *runArtifactEntry
	for name, entry := range manifest.Artifacts {
		if !isRunArtifactName(name) || len(entry.LineageSHA256) != 64 || len(entry.ContentSHA256) != 64 {
			return runArtifactManifest{}, fmt.Errorf("invalid %s entry %q", runArtifactManifestName, name)
		}
		if isSourceRunArtifactName(name) &&
			(len(entry.CallGroupSHA256) != 64 || entry.Provider == "" || entry.Model == "" || entry.PromptVersion == "" ||
				entry.ParserVersion <= 0 || entry.EvaluatorVersion <= 0) {
			return runArtifactManifest{}, fmt.Errorf("invalid %s model entry %q", runArtifactManifestName, name)
		}
		if isSourceRunArtifactName(name) {
			if sourceIdentity == nil {
				stored := entry
				sourceIdentity = &stored
				continue
			}
			if entry.CallGroupSHA256 != sourceIdentity.CallGroupSHA256 || entry.Provider != sourceIdentity.Provider ||
				entry.Model != sourceIdentity.Model || entry.PromptVersion != sourceIdentity.PromptVersion ||
				entry.ParserVersion != sourceIdentity.ParserVersion || entry.EvaluatorVersion != sourceIdentity.EvaluatorVersion {
				return runArtifactManifest{}, fmt.Errorf("inconsistent %s model artifact group", runArtifactManifestName)
			}
		}
	}
	return manifest, nil
}

func reusableRunArtifact(
	dir string,
	name string,
	session investigation.Session,
	manifest runArtifactManifest,
) (runArtifactEntry, bool, error) {
	entry, ok := manifest.Artifacts[name]
	if !ok {
		return runArtifactEntry{}, false, nil
	}
	lineage, err := runArtifactLineage(session, name)
	if err != nil || lineage != entry.LineageSHA256 {
		return runArtifactEntry{}, false, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return runArtifactEntry{}, false, nil
	}
	if err != nil {
		return runArtifactEntry{}, false, fmt.Errorf("read prior %s: %w", name, err)
	}
	if sha256Hex(data) != entry.ContentSHA256 {
		return runArtifactEntry{}, false, nil
	}
	return entry, true, nil
}

func runArtifactLineage(session investigation.Session, name string) (string, error) {
	base := struct {
		Version    int                      `json:"version"`
		Goal       investigation.Goal       `json:"goal"`
		Repository investigation.Repository `json:"repository"`
		FocusKind  investigation.FocusKind  `json:"focus_kind"`
		FocusName  string                   `json:"focus_name"`
		Origin     *investigation.Origin    `json:"origin,omitempty"`
	}{
		Version:    session.Version,
		Goal:       session.Goal,
		Repository: session.Repository,
		FocusKind:  session.Focus.Kind,
		FocusName:  session.Focus.Symbol,
		Origin:     session.Origin,
	}
	var value any
	switch name {
	case "evidence_graph.json":
		if session.Symbol == nil {
			return "", fmt.Errorf("cannot bind %s without symbol evidence", name)
		}
		value = struct {
			Base   any            `json:"base"`
			Symbol *symbol.Bundle `json:"symbol"`
		}{Base: base, Symbol: session.Symbol}
	case "deepseek_source_request.redacted.json", "deepseek_source_response.raw.txt", "source_evaluation.json", "source_parse_warnings.json":
		if session.Assessment == nil {
			return "", fmt.Errorf("cannot bind %s without source assessment", name)
		}
		value = struct {
			Base       any                   `json:"base"`
			Assessment *sourceexplain.Bundle `json:"assessment"`
			Outcome    any                   `json:"outcome"`
		}{Base: base, Assessment: session.Assessment, Outcome: sourceArtifactOutcome(session)}
	default:
		return "", fmt.Errorf("unknown run artifact %q", name)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal %s lineage: %w", name, err)
	}
	return sha256Hex(data), nil
}

func sourceArtifactOutcome(session investigation.Session) any {
	if session.SourceReport != nil {
		return struct {
			Status string                `json:"status"`
			Report *sourceexplain.Report `json:"report"`
		}{Status: "accepted", Report: session.SourceReport}
	}
	if session.State == investigation.StateAssessingSource {
		return struct {
			Status string `json:"status"`
		}{Status: "pending"}
	}
	return struct {
		Status string              `json:"status"`
		State  investigation.State `json:"state"`
		Stop   *investigation.Stop `json:"stop,omitempty"`
	}{Status: "not_accepted", State: session.State, Stop: session.Stop}
}

func sourceArtifactCallGroup(rawArtifacts map[string][]byte, artifacts runArtifacts) (string, error) {
	type item struct {
		Name          string `json:"name"`
		ContentSHA256 string `json:"content_sha256"`
	}
	group := struct {
		Provider         string `json:"provider"`
		Model            string `json:"model"`
		PromptVersion    string `json:"prompt_version"`
		ParserVersion    int    `json:"parser_version"`
		EvaluatorVersion int    `json:"evaluator_version"`
		Items            []item `json:"items"`
	}{
		Provider:         artifacts.sourceProvider,
		Model:            artifacts.sourceModel,
		PromptVersion:    artifacts.sourcePromptVersion,
		ParserVersion:    sourceexplain.ParserVersion,
		EvaluatorVersion: sourceexplain.EvaluationVersion,
	}
	for _, name := range sourceRunArtifactNames {
		data := rawArtifacts[name]
		if len(data) == 0 {
			continue
		}
		group.Items = append(group.Items, item{Name: name, ContentSHA256: sha256Hex(artifactPayload(data))})
	}
	if len(group.Items) == 0 {
		return "", nil
	}
	if group.Provider == "" || group.Model == "" || group.PromptVersion == "" {
		return "", fmt.Errorf("source run artifacts require provider, model, and prompt provenance")
	}
	data, err := json.Marshal(group)
	if err != nil {
		return "", fmt.Errorf("marshal source artifact call group: %w", err)
	}
	return sha256Hex(data), nil
}

func isRunArtifactName(name string) bool {
	for _, candidate := range runArtifactNames {
		if name == candidate {
			return true
		}
	}
	return false
}

func isSourceRunArtifactName(name string) bool {
	for _, candidate := range sourceRunArtifactNames {
		if name == candidate {
			return true
		}
	}
	return false
}

func sha256Hex(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

var sessionArtifactNames = []string{
	"symbol_bundle.json",
	"source_card.json",
	"source_assessment_bundle.json",
	"source_report.json",
	"test_evidence.json",
}

var runArtifactNames = []string{
	"evidence_graph.json",
	"deepseek_source_request.redacted.json",
	"deepseek_source_response.raw.txt",
	"source_evaluation.json",
	"source_parse_warnings.json",
}

var sourceRunArtifactNames = []string{
	"deepseek_source_request.redacted.json",
	"deepseek_source_response.raw.txt",
	"source_evaluation.json",
	"source_parse_warnings.json",
}

func writeArtifact(dir, name string, data []byte) error {
	return writeArtifactPayload(dir, name, artifactPayload(data))
}

func artifactPayload(data []byte) []byte {
	return append(append([]byte{}, data...), '\n')
}

func writeArtifactPayload(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
}
