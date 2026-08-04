package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

var caddyDirectoryMechanismIdentity = semanticdiscovery.MechanismIdentity{
	RepositoryNamespace: "github.com/caddyserver/caddy/v2",
	IntentKey:           "caddy-directory-listing",
	Scope: semanticdiscovery.MechanismScope{
		Kind:  semanticdiscovery.MechanismScopeGoPackage,
		Value: "github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver",
	},
}

func runMechanismV1CLI(args []string, stdout io.Writer) error {
	runDir, replayOnly, err := parseMechanismV1Args(args)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	mechanismPath := filepath.Join(absDir, semanticdiscovery.MechanismFile)
	if !replayOnly {
		mechanism, artifact, extractErr := report.ExtractMechanismV1(
			absDir,
			goldenDirectoryListingCandidateID,
			caddyDirectoryMechanismIdentity,
		)
		if extractErr != nil {
			return extractErr
		}
		if artifact.ID != goldenDirectoryArtifactID {
			return fmt.Errorf("mechanism v1: accepted Caddy artifact id changed")
		}
		artifactSHA, hashErr := goldenMechanismArtifactSHA256(artifact)
		if hashErr != nil {
			return hashErr
		}
		if artifactSHA != goldenDirectoryArtifactSHA256 {
			return fmt.Errorf("mechanism v1: accepted Caddy artifact content changed")
		}
		raw, encodeErr := semanticdiscovery.EncodeMechanism(mechanism)
		if encodeErr != nil {
			return encodeErr
		}
		if writeErr := writeAtomicFile(mechanismPath, raw, 0o600); writeErr != nil {
			return writeErr
		}
	}

	if err := report.Generate(absDir); err != nil {
		return err
	}
	data, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	published, err := requirePublishedUserMechanism(
		data,
		goldenDirectoryArtifactID,
		false,
	)
	if err != nil {
		return fmt.Errorf("mechanism v1: %w", err)
	}
	if !replayOnly {
		publishedSHA, hashErr := goldenMechanismArtifactSHA256(published)
		if hashErr != nil {
			return hashErr
		}
		if publishedSHA != goldenDirectoryArtifactSHA256 {
			return fmt.Errorf("mechanism v1: report projection changed the accepted artifact")
		}
	}
	mechanismRaw, err := readBoundedRegularFile(mechanismPath, maxGoldenSavedFileBytes)
	if err != nil {
		return err
	}
	mechanism, err := semanticdiscovery.DecodeMechanism(mechanismRaw)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		stdout,
		"Mechanism: %s\nContent: %s\nStart Here: %s\nFacts: %d\nReport: %s\n",
		mechanism.ID,
		mechanism.ContentSHA256,
		data.StartHereArtifactID,
		len(mechanism.Input.Facts),
		filepath.Join(absDir, "report.html"),
	)
	return nil
}

func requirePublishedUserMechanism(
	data *report.ReportData,
	artifactID string,
	requireStartHere bool,
) (semanticdiscovery.Artifact, error) {
	if data == nil {
		return semanticdiscovery.Artifact{}, fmt.Errorf("report is unavailable")
	}
	if requireStartHere && data.StartHereArtifactID != artifactID {
		return semanticdiscovery.Artifact{}, fmt.Errorf("accepted artifact is not Start Here")
	}

	var published semanticdiscovery.Artifact
	for _, artifact := range data.SemanticArtifacts {
		if artifact.ID == artifactID {
			published = artifact
			break
		}
	}
	if published.ID == "" || !semanticSearchContainsArtifact(data, artifactID) {
		return semanticdiscovery.Artifact{}, fmt.Errorf("report or Search projection is incomplete")
	}
	for _, mechanism := range data.UserMechanisms {
		if mechanism.ArtifactID == artifactID && userMechanismHasSupportedSource(mechanism) {
			return published, nil
		}
	}
	return semanticdiscovery.Artifact{}, fmt.Errorf(
		"accepted artifact is not a source-backed user mechanism",
	)
}

func userMechanismHasSupportedSource(mechanism report.UserMechanism) bool {
	if len(mechanism.Steps) == 0 {
		return false
	}
	for _, step := range mechanism.Steps {
		if len(step.Locations) == 0 {
			return false
		}
		sourceBacked := false
		for _, source := range step.Sources {
			if source.Path != "" && source.Content != "" && len(source.Lines) > 0 &&
				len(source.RelatedEvidenceIDs) > 0 {
				sourceBacked = true
				break
			}
		}
		if !sourceBacked {
			return false
		}
	}
	return true
}

func parseMechanismV1Args(args []string) (string, bool, error) {
	runDir := ""
	replay := false
	for _, arg := range args {
		switch arg {
		case "--replay":
			if replay {
				return "", false, mechanismV1Usage()
			}
			replay = true
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", false, mechanismV1Usage()
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", false, mechanismV1Usage()
	}
	return runDir, replay, nil
}

func mechanismV1Usage() error {
	return fmt.Errorf("Usage: repomap dev mechanism-v1 <run-dir> [--replay]")
}
