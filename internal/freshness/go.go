package freshness

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

type GoOptions struct {
	GoBinary         string
	GoplsBinary      string
	Collector        string
	CollectorVersion string
	AnalyzerOptions  string
	CollectorOptions string
}

// CaptureGoFactContext records the toolchain, analyzer, collector, and build
// inputs needed to decide whether persisted Go facts are still reusable.
func CaptureGoFactContext(ctx context.Context, repository RepositoryState, options GoOptions) (FactContext, error) {
	if err := repository.Validate(); err != nil {
		return FactContext{}, fmt.Errorf("freshness: capture Go fact context: %w", err)
	}
	if !validLabel(options.Collector) {
		return FactContext{}, fmt.Errorf("freshness: collector is required and must not contain control characters")
	}
	if !validLabel(options.CollectorVersion) {
		return FactContext{}, fmt.Errorf("freshness: collector version is required and must not contain control characters")
	}
	if !validLabel(options.AnalyzerOptions) || !validLabel(options.CollectorOptions) {
		return FactContext{}, fmt.Errorf("freshness: analyzer and collector options are required")
	}

	goBinary := options.GoBinary
	if strings.TrimSpace(goBinary) == "" {
		goBinary = "go"
	}
	goplsBinary := options.GoplsBinary
	if strings.TrimSpace(goplsBinary) == "" {
		goplsBinary = "gopls"
	}

	goOutput, err := goFactCommandOutput(
		ctx,
		repository.Identity,
		"go env",
		goBinary,
		"env", "-json", "GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "GOWORK", "CGO_ENABLED",
	)
	if err != nil {
		return FactContext{}, err
	}
	var environment struct {
		GoVersion string `json:"GOVERSION"`
		GOOS      string `json:"GOOS"`
		GOARCH    string `json:"GOARCH"`
		GOFLAGS   string `json:"GOFLAGS"`
		GOWORK    string `json:"GOWORK"`
		CGO       string `json:"CGO_ENABLED"`
	}
	if err := json.Unmarshal(goOutput, &environment); err != nil {
		return FactContext{}, fmt.Errorf("freshness: decode go env: %w", err)
	}
	buildTags, err := goBuildTags(environment.GOFLAGS)
	if err != nil {
		return FactContext{}, err
	}

	goplsOutput, err := goFactCommandOutput(ctx, repository.Identity, "gopls version", goplsBinary, "version")
	if err != nil {
		return FactContext{}, err
	}
	goplsVersion, err := lastGoplsVersionToken(goplsOutput)
	if err != nil {
		return FactContext{}, err
	}
	inputJSON, err := json.Marshal(struct {
		GOFLAGS          string `json:"go_flags"`
		GOWORK           string `json:"go_work"`
		CGO              string `json:"cgo_enabled"`
		AnalyzerOptions  string `json:"analyzer_options"`
		CollectorOptions string `json:"collector_options"`
	}{
		GOFLAGS:          environment.GOFLAGS,
		GOWORK:           environment.GOWORK,
		CGO:              environment.CGO,
		AnalyzerOptions:  options.AnalyzerOptions,
		CollectorOptions: options.CollectorOptions,
	})
	if err != nil {
		return FactContext{}, fmt.Errorf("freshness: encode analysis inputs: %w", err)
	}

	facts := FactContext{
		Version:          FactContextVersion,
		Repository:       repository,
		GoVersion:        environment.GoVersion,
		Analyzer:         "gopls",
		AnalyzerVersion:  goplsVersion,
		Collector:        options.Collector,
		CollectorVersion: options.CollectorVersion,
		InputsSHA256:     sha256Hex(inputJSON),
		Build: evidence.BuildContext{
			GOOS:      environment.GOOS,
			GOARCH:    environment.GOARCH,
			BuildTags: buildTags,
		},
	}
	if err := facts.Validate(); err != nil {
		return FactContext{}, fmt.Errorf("freshness: captured Go fact context is invalid: %w", err)
	}
	return facts, nil
}

func goFactCommandOutput(ctx context.Context, dir, stage, binary string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return nil, fmt.Errorf("freshness: %s: %w", stage, err)
	}
	return nil, fmt.Errorf("freshness: %s: %w: %s", stage, err, detail)
}

func goBuildTags(goFlags string) ([]string, error) {
	fields := strings.Fields(goFlags)
	tagSet := make(map[string]struct{})
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		var value string
		switch {
		case field == "-tags":
			index++
			if index >= len(fields) || strings.HasPrefix(fields[index], "-") {
				return nil, fmt.Errorf("freshness: GOFLAGS -tags requires a value")
			}
			value = fields[index]
		case strings.HasPrefix(field, "-tags="):
			value = strings.TrimPrefix(field, "-tags=")
		default:
			continue
		}
		if value == "" {
			return nil, fmt.Errorf("freshness: GOFLAGS -tags requires a value")
		}
		for _, tag := range strings.Split(value, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				return nil, fmt.Errorf("freshness: GOFLAGS -tags contains an empty build tag")
			}
			tagSet[tag] = struct{}{}
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func lastGoplsVersionToken(output []byte) (string, error) {
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", fmt.Errorf("freshness: gopls version output did not contain a version")
	}
	return fields[len(fields)-1], nil
}
