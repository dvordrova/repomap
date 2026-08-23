package pythondeclareddependencies

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/dependencydeclaration"
)

const maxLogicalRequirementBytes = 64 << 10

var (
	distributionNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?`)
	distributionNormalize   = regexp.MustCompile(`[-_.]+`)
	requirementHashOption   = regexp.MustCompile(`(?i)[ \t]+--hash(?:=|[ \t]+)[^ \t]+`)
)

type logicalRequirement struct {
	text       string
	raw        string
	line       int
	incomplete bool
}

type parsedRequirement struct {
	name        string
	normalized  string
	extras      []string
	specifier   string
	conditional bool
	locator     dependencydeclaration.Locator
}

func (state *builder) parseRequirements(
	source *sourceDraft,
	kind dependencydeclaration.StatementKind,
) error {
	lines, err := logicalRequirements(source.content)
	if err != nil {
		return fmt.Errorf("python declared dependencies: requirements %q: %w", source.entry.Path, err)
	}
	for _, line := range lines {
		if err := state.ctx.Err(); err != nil {
			return err
		}
		digest := textSHA256(line.raw)
		location := &dependencydeclaration.Location{Line: line.line, Column: 1}
		if line.incomplete {
			state.addFrontier(source, dependencydeclaration.FrontierDirective,
				dependencydeclaration.FrontierUnsupportedRequirement, "requirements", line.line, location, digest)
			continue
		}
		text := strings.TrimSpace(stripRequirementComment(line.text))
		if text == "" {
			continue
		}
		if includeKind, argument, ok := parseIncludeDirective(text); ok {
			if err := state.addInclude(source, includeKind, argument, location, digest); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(text, "-") {
			state.addFrontier(source, dependencydeclaration.FrontierDirective,
				dependencydeclaration.FrontierUnsupportedOption, "requirements", line.line, location, digest)
			continue
		}
		parsed, err := parseRequirementExpression(text, source.entry.Path, state.projectDir)
		if err != nil {
			reason := dependencydeclaration.FrontierUnsupportedRequirement
			if strings.Contains(err.Error(), "package identity") {
				reason = dependencydeclaration.FrontierPackageIdentityUnavailable
			}
			state.addFrontier(source, dependencydeclaration.FrontierStatement,
				reason, "requirements", line.line, location, digest)
			continue
		}
		state.statements = append(state.statements, dependencydeclaration.StatementInput{
			SourceKey: source.key, Kind: kind, Role: dependencydeclaration.RoleUnspecified,
			Name: parsed.name, NormalizedName: parsed.normalized, Extras: parsed.extras,
			Specifier: parsed.specifier, Conditional: parsed.conditional, Locator: parsed.locator,
			Section: "requirements", Ordinal: line.line, Location: location,
			ExpressionSHA256: digest,
		})
		if len(state.statements) > dependencydeclaration.MaxStatements {
			return fmt.Errorf("python declared dependencies: statement bound %d exceeded", dependencydeclaration.MaxStatements)
		}
	}
	return nil
}

func logicalRequirements(content []byte) ([]logicalRequirement, error) {
	if !utf8.Valid(content) || strings.ContainsRune(string(content), 0) {
		return nil, fmt.Errorf("content is not valid UTF-8 text")
	}
	physical := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	result := make([]logicalRequirement, 0, len(physical))
	var parts []string
	start := 0
	for index, raw := range physical {
		lineNumber := index + 1
		if len(raw) > maxLogicalRequirementBytes {
			return nil, fmt.Errorf("line %d exceeds logical line bound", lineNumber)
		}
		trimmedRight := strings.TrimRight(raw, " \t\r")
		continued := strings.HasSuffix(trimmedRight, "\\")
		if continued {
			trimmedRight = strings.TrimSuffix(trimmedRight, "\\")
		}
		if len(parts) == 0 {
			start = lineNumber
		}
		parts = append(parts, trimmedRight)
		combined := strings.Join(parts, " ")
		if len(combined) > maxLogicalRequirementBytes {
			return nil, fmt.Errorf("logical line at %d exceeds %d bytes", start, maxLogicalRequirementBytes)
		}
		if continued {
			continue
		}
		result = append(result, logicalRequirement{
			text: combined, raw: strings.Join(parts, "\n"), line: start,
		})
		parts = nil
	}
	if len(parts) > 0 {
		result = append(result, logicalRequirement{
			text: strings.Join(parts, " "), raw: strings.Join(parts, "\n"), line: start, incomplete: true,
		})
	}
	return result, nil
}

func stripRequirementComment(value string) string {
	for index, character := range value {
		if character != '#' {
			continue
		}
		if index == 0 || value[index-1] == ' ' || value[index-1] == '\t' {
			return value[:index]
		}
	}
	return value
}

func parseIncludeDirective(value string) (dependencydeclaration.IncludeKind, string, bool) {
	type prefix struct {
		value string
		kind  dependencydeclaration.IncludeKind
	}
	prefixes := []prefix{
		{"--requirement=", dependencydeclaration.IncludeRequirement},
		{"--constraint=", dependencydeclaration.IncludeConstraint},
		{"--requirement ", dependencydeclaration.IncludeRequirement},
		{"--constraint ", dependencydeclaration.IncludeConstraint},
		{"-r ", dependencydeclaration.IncludeRequirement},
		{"-c ", dependencydeclaration.IncludeConstraint},
	}
	for _, candidate := range prefixes {
		if strings.HasPrefix(value, candidate.value) {
			return candidate.kind, strings.TrimSpace(strings.TrimPrefix(value, candidate.value)), true
		}
	}
	if strings.HasPrefix(value, "-r") && len(value) > 2 {
		return dependencydeclaration.IncludeRequirement, strings.TrimSpace(value[2:]), true
	}
	if strings.HasPrefix(value, "-c") && len(value) > 2 {
		return dependencydeclaration.IncludeConstraint, strings.TrimSpace(value[2:]), true
	}
	return "", "", false
}

func (state *builder) addInclude(
	source *sourceDraft,
	kind dependencydeclaration.IncludeKind,
	argument string,
	location *dependencydeclaration.Location,
	digest string,
) error {
	seenKey := source.key + fmt.Sprintf(":%d:", location.Line) + string(kind) + ":" + digest
	if _, duplicate := state.includeSeen[seenKey]; duplicate {
		return nil
	}
	state.includeSeen[seenKey] = struct{}{}
	resolution := dependencydeclaration.IncludeOutsideScope
	targetKey := ""
	cleanArgument, safe := cleanIncludeArgument(argument)
	if safe {
		candidate := path.Clean(path.Join(path.Dir(source.entry.Path), cleanArgument))
		if state.inScope(candidate) {
			if fileRef, ok := state.repository.ID(candidate); ok {
				info, exists := state.repository.Info(fileRef)
				if !exists || info.Entry.Path != candidate {
					return fmt.Errorf("python declared dependencies: include corpus binding mismatch")
				}
				target, err := state.addSource(info.Entry, formatRequirements, dependencydeclaration.SourceParsed)
				if err != nil {
					return err
				}
				targetKey = target.key
				resolution = dependencydeclaration.IncludeResolved
				statementKind := dependencydeclaration.StatementRequirement
				if kind == dependencydeclaration.IncludeConstraint {
					statementKind = dependencydeclaration.StatementConstraint
				}
				state.queue = append(state.queue, requirementWork{sourceKey: target.key, kind: statementKind})
			} else {
				resolution = dependencydeclaration.IncludeMissing
			}
		}
	}
	state.includes = append(state.includes, dependencydeclaration.IncludeInput{
		SourceKey: source.key, TargetSourceKey: targetKey, Kind: kind, Resolution: resolution,
		Location: location, ExpressionSHA256: digest,
	})
	if len(state.includes) > dependencydeclaration.MaxIncludes {
		return fmt.Errorf("python declared dependencies: include bound %d exceeded", dependencydeclaration.MaxIncludes)
	}
	return nil
}

func cleanIncludeArgument(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"')) {
		value = value[1 : len(value)-1]
	}
	if value == "" || path.IsAbs(value) || strings.HasPrefix(value, "~") ||
		strings.Contains(value, "\\") || strings.ContainsAny(value, "\r\n") {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return value, true
}

func parseRequirementExpression(value, sourcePath, projectDir string) (parsedRequirement, error) {
	value = strings.TrimSpace(requirementHashOption.ReplaceAllString(value, ""))
	if value == "" || strings.Contains(value, " --") {
		return parsedRequirement{}, fmt.Errorf("unsupported requirement")
	}
	requirement, marker := splitRequirementMarker(value)
	match := distributionNamePattern.FindString(requirement)
	if match == "" {
		return parsedRequirement{}, fmt.Errorf("package identity unavailable")
	}
	rest := strings.TrimSpace(requirement[len(match):])
	if strings.HasPrefix(rest, "://") {
		return parsedRequirement{}, fmt.Errorf("package identity unavailable")
	}
	parsed := parsedRequirement{
		name: match, normalized: normalizeDistributionName(match), conditional: strings.TrimSpace(marker) != "",
		locator: dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorRegistry},
	}
	if strings.HasPrefix(rest, "[") {
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return parsedRequirement{}, fmt.Errorf("unsupported requirement extras")
		}
		for _, extra := range strings.Split(rest[1:end], ",") {
			extra = strings.TrimSpace(extra)
			if extra == "" || !validDistributionName(extra) {
				return parsedRequirement{}, fmt.Errorf("unsupported requirement extras")
			}
			parsed.extras = append(parsed.extras, extra)
		}
		rest = strings.TrimSpace(rest[end+1:])
	}
	parsed.extras = canonicalStrings(parsed.extras)
	if strings.HasPrefix(rest, "@") {
		reference := strings.TrimSpace(strings.TrimPrefix(rest, "@"))
		if reference == "" {
			return parsedRequirement{}, fmt.Errorf("unsupported direct requirement")
		}
		parsed.locator = safeLocator(reference, path.Dir(sourcePath), projectDir)
		return parsed, nil
	}
	if rest != "" {
		if strings.ContainsAny(rest, "\r\n") || !strings.ContainsAny(rest[:1], "<>=!~(") ||
			!safeVersionExpression(rest) {
			return parsedRequirement{}, fmt.Errorf("unsupported requirement specifier")
		}
		parsed.specifier = rest
	}
	return parsed, nil
}

func splitRequirementMarker(value string) (string, string) {
	depth := 0
	quote := rune(0)
	for index, character := range value {
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		switch character {
		case '[', '(':
			depth++
		case ']', ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				return strings.TrimSpace(value[:index]), strings.TrimSpace(value[index+1:])
			}
		}
	}
	return strings.TrimSpace(value), ""
}

func safeLocator(reference, sourceDir, projectDir string) dependencydeclaration.Locator {
	parsed, err := url.Parse(reference)
	if err == nil && parsed.Scheme != "" {
		host := strings.ToLower(parsed.Hostname())
		if strings.HasPrefix(strings.ToLower(parsed.Scheme), "git+") || parsed.Scheme == "git" || parsed.Scheme == "ssh" {
			return dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorVCS, Host: host}
		}
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorURL, Host: host}
		}
		return dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorExternalPath}
	}
	repositoryReference := reference
	if err == nil {
		repositoryReference = parsed.Path
	}
	if !path.IsAbs(repositoryReference) && !strings.Contains(repositoryReference, "\\") &&
		!strings.ContainsAny(repositoryReference, "%?#@:") {
		candidate := path.Clean(path.Join(sourceDir, repositoryReference))
		if candidate != "." && candidate != ".." && !strings.HasPrefix(candidate, "../") &&
			(projectDir == "" || candidate == projectDir || strings.HasPrefix(candidate, projectDir+"/")) {
			return dependencydeclaration.Locator{
				Kind: dependencydeclaration.LocatorRepositoryPath, RepositoryPath: candidate,
			}
		}
	}
	return dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorExternalPath}
}

func normalizeDistributionName(value string) string {
	return strings.ToLower(distributionNormalize.ReplaceAllString(strings.TrimSpace(value), "-"))
}

func safeVersionExpression(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) ||
			strings.ContainsRune("._-*+!<>=~^,|() ", character) {
			continue
		}
		return false
	}
	return true
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	return result[:write]
}

func textSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (state *builder) addFrontier(
	source *sourceDraft,
	kind dependencydeclaration.FrontierKind,
	reason dependencydeclaration.FrontierReason,
	section string,
	ordinal int,
	location *dependencydeclaration.Location,
	digest string,
) {
	key := source.key + ":" + string(kind) + ":" + string(reason) + ":" +
		fmt.Sprintf("%d:", ordinal) + digest
	if _, duplicate := state.frontierSeen[key]; duplicate {
		return
	}
	state.frontierSeen[key] = struct{}{}
	state.frontiers = append(state.frontiers, dependencydeclaration.FrontierInput{
		SourceKey: source.key, Kind: kind, Reason: reason, Section: section,
		Ordinal: ordinal, Location: location, ExpressionSHA256: digest,
	})
}
