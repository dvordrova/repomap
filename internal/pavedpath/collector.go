package pavedpath

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	collectorMaxFiles     = 80
	collectorMaxBytes     = 512 << 10
	collectorMaxFileBytes = 32 << 10
	collectorMaxExcerpt   = 20
	collectorMaxPerFile   = 16
	collectorMaxLinkDepth = 16
)

type operationalFileKind uint8

const (
	operationalMarkdown operationalFileKind = iota + 1
	operationalMakefile
	operationalTaskfile
	operationalJustfile
	operationalPackageJSON
	operationalScript
	operationalCompose
	operationalEnvironment
	operationalDockerfile
	operationalConfiguration
)

type operationalCandidate struct {
	path string
	kind operationalFileKind
	rank int
}

type collectedLine struct {
	number    int
	text      string
	redacted  bool
	omitted   bool
	truncated bool
}

var (
	markdownHeadingRE  = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
	makeTargetRE       = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*(?:\s+[A-Za-z0-9][A-Za-z0-9_.-]*)*)\s*:(?:[^=]|$)`)
	taskTargetRE       = regexp.MustCompile(`^([ \t]+)([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:\s*(?:#.*)?$`)
	justTargetRE       = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*)(?:\s+[^:=]+)?\s*:\s*(?:#.*)?$`)
	environmentRE      = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)
	assignmentRE       = regexp.MustCompile(`^([ \t]*(?:#[ \t]*)?(?:export[ \t]+)?["']?([A-Za-z_][A-Za-z0-9_.-]*)["']?[ \t]*[:=][ \t]*)(.*)$`)
	inlineAssignmentRE = regexp.MustCompile("[A-Za-z_][A-Za-z0-9_-]*=[^\\s\"'`;,]+")
	secretFlagRE       = regexp.MustCompile(`(?i)(--(?:password|passphrase|[a-z0-9_-]*token|secret|api[-_]?key|access[-_]?key|credential)(?:=|\s+))("[^"]*"|'[^']*'|\S+)`)
	basicAuthFlagRE    = regexp.MustCompile(`(?i)((?:-u|--user)(?:=|\s+))("[^"]*"|'[^']*'|\S+)`)
	urlUserInfoRE      = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s:]+:[^/@\s]+@`)
	knownSecretRE      = regexp.MustCompile(`(?:AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,})`)
	bearerSecretRE     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~-]{8,}`)
	localEndpointRE    = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|\[::1\])(?::[0-9]{1,5})?(?:/[^\s\])}>"']*)?`)
	logPathRE          = regexp.MustCompile(`(?:/|\.\.?/)[A-Za-z0-9_.~/-]+\.log(?:\b|$)`)
	placeholderRE      = regexp.MustCompile(`<[^>]+>|\$\{?[A-Za-z_][A-Za-z0-9_]*\}?`)
)

// Collect gathers bounded repository-owned operational evidence without
// executing any repository command. Every read is confined to repoRoot and to
// the caller-supplied allowlist.
func Collect(repoRoot, repoName string, allowedPaths []string) (Bundle, error) {
	if !validText(repoName, 256, true) {
		return Bundle{}, fmt.Errorf("paved paths: invalid repository name")
	}
	candidates, err := operationalCandidates(allowedPaths)
	if err != nil {
		return Bundle{}, err
	}
	authorizedPaths := make(map[string]struct{}, len(allowedPaths))
	for _, filePath := range allowedPaths {
		authorizedPaths[filePath] = struct{}{}
	}
	root, err := os.OpenRoot(repoRoot)
	if err != nil {
		return Bundle{}, fmt.Errorf("paved paths: open repository root: %w", err)
	}
	defer root.Close()

	bundle := Bundle{
		Version:      BundleVersion,
		RepoName:     strings.TrimSpace(repoName),
		Evidence:     []Evidence{},
		AllowedPaths: []string{},
		Stats: Stats{
			ConsideredFiles: len(candidates),
		},
	}
	collected := make([]Evidence, 0, MaxEvidence*2)
	readTargets := make(map[string]struct{}, len(candidates))
	if len(candidates) > collectorMaxFiles {
		bundle.Stats.Truncated = true
	}

	allowed := make(map[string]struct{})
	for _, candidate := range candidates {
		if bundle.Stats.ReadFiles == collectorMaxFiles || bundle.Stats.ReadBytes == collectorMaxBytes {
			bundle.Stats.Truncated = true
			break
		}
		remaining := collectorMaxBytes - bundle.Stats.ReadBytes
		limit := min(collectorMaxFileBytes, remaining)
		data, mode, truncated, resolvedPath, err := readOperationalFile(
			root,
			candidate.path,
			limit,
			authorizedPaths,
		)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Bundle{}, fmt.Errorf("paved paths: read %q: %w", candidate.path, err)
		}
		if resolvedPath == "" || !mode.IsRegular() {
			continue
		}
		if _, duplicate := readTargets[resolvedPath]; duplicate {
			continue
		}
		readTargets[resolvedPath] = struct{}{}
		bundle.Stats.ReadFiles++
		bundle.Stats.ReadBytes += len(data)
		bundle.Stats.Truncated = bundle.Stats.Truncated || truncated
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}

		lines, redactions := prepareLines(data, candidate.kind == operationalEnvironment)
		bundle.Stats.Redactions += redactions
		items := parseOperationalFile(candidate, lines, mode, truncated)
		if len(items) > collectorMaxPerFile {
			items = items[:collectorMaxPerFile]
			bundle.Stats.Truncated = true
		}
		collected = append(collected, items...)
	}
	bundle.Evidence = selectOperationalEvidence(collected)
	if len(collected) > len(bundle.Evidence) {
		bundle.Stats.Truncated = true
	}
	for _, item := range bundle.Evidence {
		allowed[item.Path] = struct{}{}
	}

	for filePath := range allowed {
		bundle.AllowedPaths = append(bundle.AllowedPaths, filePath)
	}
	sort.Strings(bundle.AllowedPaths)
	if err := bundle.Validate(); err != nil {
		return Bundle{}, fmt.Errorf("paved paths: collected invalid evidence: %w", err)
	}
	return bundle, nil
}

// selectOperationalEvidence reserves a small slice for every operational
// role before filling the remaining budget in deterministic file-rank order.
// Long READMEs therefore cannot displace Make targets, scripts, config, or
// readiness signals that were found within the same file/byte budget.
func selectOperationalEvidence(items []Evidence) []Evidence {
	items = uniqueEvidence(items)
	result := make([]Evidence, 0, min(MaxEvidence, len(items)))
	selected := make(map[string]struct{}, len(items))
	roles := []EvidenceRole{
		RoleBuildTarget, RoleRepositoryScript, RolePackageScript, RoleComposeService,
		RoleEnvironment, RoleConfiguration, RoleEndpoint, RoleLogLocation,
		RoleVerification, RoleDocumentedProcedure,
	}
	for _, role := range roles {
		retained := 0
		for _, item := range items {
			if item.Role != role || retained == 4 {
				continue
			}
			result = append(result, item)
			selected[item.ID] = struct{}{}
			retained++
		}
	}
	for _, item := range items {
		if len(result) == MaxEvidence {
			break
		}
		if _, exists := selected[item.ID]; exists {
			continue
		}
		result = append(result, item)
		selected[item.ID] = struct{}{}
	}
	return result
}

func operationalCandidates(allowedPaths []string) ([]operationalCandidate, error) {
	unique := make(map[string]struct{}, len(allowedPaths))
	result := make([]operationalCandidate, 0, len(allowedPaths))
	for _, filePath := range allowedPaths {
		if !validPath(filePath) {
			return nil, fmt.Errorf("paved paths: allowed path %q is not repository-local", filePath)
		}
		if _, duplicate := unique[filePath]; duplicate {
			continue
		}
		unique[filePath] = struct{}{}
		kind, rank, ok := classifyOperationalPath(filePath)
		if !ok {
			continue
		}
		result = append(result, operationalCandidate{path: filePath, kind: kind, rank: rank})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].rank != result[j].rank {
			return result[i].rank < result[j].rank
		}
		return result[i].path < result[j].path
	})
	return result, nil
}

func classifyOperationalPath(filePath string) (operationalFileKind, int, bool) {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	depth := strings.Count(filePath, "/")
	if historicalOperationalPath(lower) {
		return 0, 0, false
	}
	if depth == 0 {
		switch base {
		case "readme", "readme.md", "readme.rst", "readme.adoc", "contributing", "contributing.md",
			"contributing.rst", "agents.md", "claude.md":
			return operationalMarkdown, 0, true
		}
	}
	switch base {
	case "makefile", "gnumakefile":
		return operationalMakefile, 10 + depth, true
	case "taskfile.yml", "taskfile.yaml":
		return operationalTaskfile, 10 + depth, true
	case "justfile":
		return operationalJustfile, 10 + depth, true
	case "package.json":
		return operationalPackageJSON, 10 + depth, true
	}
	if isEnvironmentExample(base) {
		return operationalEnvironment, 20 + depth, true
	}
	if isComposeFile(base) {
		return operationalCompose, 21 + depth, true
	}
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return operationalDockerfile, 22 + depth, true
	}
	if inExecutableDirectory(lower) {
		return operationalScript, 30 + depth, true
	}
	if isOperationalDocument(lower, base) {
		return operationalMarkdown, 40 + depth, true
	}
	if strings.HasPrefix(base, "readme") && isDocumentExtension(base) {
		return operationalMarkdown, 50 + depth, true
	}
	if isOperationalConfiguration(lower, base) {
		return operationalConfiguration, 60 + depth, true
	}
	return 0, 0, false
}

func historicalOperationalPath(filePath string) bool {
	for _, segment := range strings.Split(filePath, "/") {
		switch segment {
		case "archive", "archives", "historical", "history", "decisions", "decision", "adr", "adrs", "agent-room":
			return true
		}
	}
	base := path.Base(filePath)
	return strings.HasPrefix(base, "changelog") || strings.HasPrefix(base, "release-notes")
}

func isDocumentExtension(base string) bool {
	return strings.HasSuffix(base, ".md") || strings.HasSuffix(base, ".rst") ||
		strings.HasSuffix(base, ".adoc") || strings.HasSuffix(base, ".txt") || base == "readme"
}

func isOperationalDocument(filePath, base string) bool {
	if !isDocumentExtension(base) {
		return false
	}
	if !strings.HasPrefix(filePath, "docs/") && !strings.HasPrefix(filePath, "doc/") &&
		!strings.HasPrefix(filePath, "documentation/") {
		return false
	}
	for _, marker := range []string{
		"setup", "install", "quickstart", "getting-started", "getting_started", "build", "develop",
		"run", "usage", "test", "verify", "deploy", "operation", "configuration", "config", "docker",
		"troubleshoot", "restore", "backup", "monitor", "health", "contribut",
	} {
		if strings.Contains(base, marker) {
			return true
		}
	}
	return false
}

func inExecutableDirectory(filePath string) bool {
	for _, prefix := range []string{"scripts/", "hack/", "dev/", ".agents/"} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	return false
}

func isEnvironmentExample(base string) bool {
	return base == ".env.example" || base == ".env.sample" || base == ".env.template" ||
		base == "env.example" || base == "env.sample" || base == "env.template"
}

func isComposeFile(base string) bool {
	return base == "compose.yml" || base == "compose.yaml" || base == "docker-compose.yml" ||
		base == "docker-compose.yaml" ||
		(strings.HasPrefix(base, "docker-compose.") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")))
}

func isOperationalConfiguration(filePath, base string) bool {
	extension := path.Ext(base)
	if extension != ".yml" && extension != ".yaml" && extension != ".toml" &&
		extension != ".json" && extension != ".ini" && extension != ".conf" {
		return false
	}
	return strings.HasPrefix(filePath, "config/") || strings.HasPrefix(filePath, "configs/") ||
		strings.HasPrefix(filePath, "etc/") || strings.Contains(base, ".example.") ||
		strings.Contains(base, ".sample.")
}

func readOperationalFile(
	root *os.Root,
	filePath string,
	limit int,
	authorizedPaths map[string]struct{},
) ([]byte, os.FileMode, bool, string, error) {
	resolvedPath, eligible, err := resolveOperationalPath(root, filePath, authorizedPaths)
	if err != nil || !eligible {
		return nil, 0, false, "", err
	}
	info, err := root.Stat(resolvedPath)
	if err != nil {
		return nil, 0, false, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, info.Mode(), false, resolvedPath, nil
	}
	file, err := root.Open(resolvedPath)
	if err != nil {
		return nil, 0, false, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, 0, false, "", err
	}
	truncated := len(data) > limit || info.Size() > int64(limit)
	if len(data) > limit {
		data = data[:limit]
	}
	return data, info.Mode(), truncated, resolvedPath, nil
}

// resolveOperationalPath permits a tracked relative file alias without
// widening the caller's inventory authority. os.Root confines every lookup to
// the repository; the explicit inventory check also prevents a tracked link
// from exposing an untracked or otherwise unauthorized in-repository target.
// Ineligible links are skipped so one optional alias cannot erase unrelated
// operational evidence.
func resolveOperationalPath(
	root *os.Root,
	filePath string,
	authorizedPaths map[string]struct{},
) (string, bool, error) {
	current := filePath
	seen := make(map[string]struct{}, collectorMaxLinkDepth)
	for range collectorMaxLinkDepth {
		if _, duplicate := seen[current]; duplicate {
			return "", false, nil
		}
		seen[current] = struct{}{}

		info, err := root.Lstat(current)
		if err != nil {
			return "", false, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if _, authorized := authorizedPaths[current]; !authorized {
				return "", false, nil
			}
			return current, true, nil
		}

		target, err := root.Readlink(current)
		if err != nil {
			return "", false, err
		}
		if filepath.IsAbs(target) {
			return "", false, nil
		}
		target = filepath.ToSlash(target)
		if path.IsAbs(target) {
			return "", false, nil
		}
		current = path.Clean(path.Join(path.Dir(current), target))
		if !validPath(current) {
			return "", false, nil
		}
		if _, authorized := authorizedPaths[current]; !authorized {
			return "", false, nil
		}
	}
	return "", false, nil
}

func prepareLines(data []byte, redactEveryAssignment bool) ([]collectedLine, int) {
	rawLines := strings.Split(strings.ToValidUTF8(string(data), "�"), "\n")
	result := make([]collectedLine, len(rawLines))
	redactions := 0
	inPrivateKey := false
	for index, raw := range rawLines {
		raw = strings.TrimSuffix(raw, "\r")
		line := collectedLine{number: index + 1, text: raw}
		upper := strings.ToUpper(raw)
		if strings.Contains(upper, "-----BEGIN ") && strings.Contains(upper, "PRIVATE KEY-----") {
			inPrivateKey = true
			line.omitted = true
			line.redacted = true
			redactions++
			result[index] = line
			continue
		}
		if inPrivateKey {
			line.omitted = true
			line.redacted = true
			redactions++
			if strings.Contains(upper, "-----END ") && strings.Contains(upper, "PRIVATE KEY-----") {
				inPrivateKey = false
			}
			result[index] = line
			continue
		}
		if strings.Contains(strings.ToLower(raw), "authorization:") || bearerSecretRE.MatchString(raw) {
			line.omitted = true
			line.redacted = true
			redactions++
			result[index] = line
			continue
		}
		redacted, changed := redactSensitiveLine(raw, redactEveryAssignment)
		line.text = limitLine(redacted)
		line.truncated = line.text != redacted
		line.redacted = changed
		if changed {
			redactions++
		}
		result[index] = line
	}
	return result, redactions
}

func redactSensitiveLine(value string, redactEveryAssignment bool) (string, bool) {
	changed := false
	if match := assignmentRE.FindStringSubmatch(value); len(match) == 4 {
		key := strings.ToLower(match[2])
		if redactEveryAssignment || secretShapedName(key) {
			replacement := "<redacted>"
			if strings.Contains(match[1], "\"") && strings.Contains(match[1], ":") {
				replacement = "\"<redacted>\""
			}
			value = match[1] + replacement
			changed = true
		}
	}
	updated := secretFlagRE.ReplaceAllString(value, `${1}<redacted>`)
	if updated != value {
		value = updated
		changed = true
	}
	updated = basicAuthFlagRE.ReplaceAllString(value, `${1}<redacted>`)
	if updated != value {
		value = updated
		changed = true
	}
	updated = urlUserInfoRE.ReplaceAllString(value, `${1}<redacted>@`)
	if updated != value {
		value = updated
		changed = true
	}
	updated = knownSecretRE.ReplaceAllString(value, `<redacted>`)
	if updated != value {
		value = updated
		changed = true
	}
	updated = inlineAssignmentRE.ReplaceAllStringFunc(value, func(assignment string) string {
		key, _, ok := strings.Cut(assignment, "=")
		if !ok || !secretShapedName(key) {
			return assignment
		}
		return key + "=<redacted>"
	})
	if updated != value {
		value = updated
		changed = true
	}
	return value, changed
}

func secretShapedName(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	for _, marker := range []string{
		"password", "passwd", "passphrase", "secret", "token", "api_key", "apikey", "access_key",
		"private_key", "credential", "authorization",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func limitLine(value string) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= 2048 {
		return value
	}
	value = value[:2045]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "..."
}

func parseOperationalFile(
	candidate operationalCandidate,
	lines []collectedLine,
	mode os.FileMode,
	truncated bool,
) []Evidence {
	var items []Evidence
	switch candidate.kind {
	case operationalMarkdown:
		items = parseDocumentation(candidate.path, lines)
	case operationalMakefile:
		items = parseMakefile(candidate.path, lines, truncated)
	case operationalTaskfile:
		items = parseTaskfile(candidate.path, lines, truncated)
	case operationalJustfile:
		items = parseJustfile(candidate.path, lines, truncated)
	case operationalPackageJSON:
		items = parsePackageJSON(candidate.path, lines)
	case operationalScript:
		if mode.Perm()&0o111 != 0 {
			items = parseExecutableScript(candidate.path, lines, truncated)
		}
	case operationalCompose:
		items = parseCompose(candidate.path, lines)
	case operationalEnvironment:
		items = parseEnvironment(candidate.path, lines)
	case operationalDockerfile:
		items = parseDockerfile(candidate.path, lines)
	case operationalConfiguration:
		items = parseConfiguration(candidate.path, lines)
	}
	if candidate.kind == operationalMarkdown || candidate.kind == operationalCompose ||
		candidate.kind == operationalConfiguration {
		items = append(items, parseLocalEndpoints(candidate.path, lines)...)
		items = append(items, parseLogLocations(candidate.path, lines)...)
	}
	return uniqueEvidence(items)
}

func parseDocumentation(filePath string, lines []collectedLine) []Evidence {
	items := parseDocumentSections(filePath, lines)
	items = append(items, parseFencedCommands(filePath, lines)...)
	items = append(items, parseRSTCommands(filePath, lines)...)
	return items
}

func parseDocumentSections(filePath string, lines []collectedLine) []Evidence {
	type heading struct {
		line  int
		level int
		text  string
	}
	headings := make([]heading, 0)
	for index, line := range lines {
		if line.omitted {
			continue
		}
		if match := markdownHeadingRE.FindStringSubmatch(strings.TrimSpace(line.text)); len(match) == 3 {
			headings = append(headings, heading{line: index, level: len(match[1]), text: strings.TrimSpace(match[2])})
			continue
		}
		if index+1 < len(lines) && !lines[index+1].omitted && isRSTUnderline(strings.TrimSpace(lines[index+1].text)) &&
			strings.TrimSpace(line.text) != "" {
			headings = append(headings, heading{line: index, level: 1, text: strings.TrimSpace(line.text)})
		}
	}
	items := make([]Evidence, 0)
	for index, current := range headings {
		if !operationalHeading(current.text) {
			continue
		}
		end := len(lines) - 1
		for next := index + 1; next < len(headings); next++ {
			if headings[next].level <= current.level {
				end = headings[next].line - 1
				break
			}
		}
		excerpt, startLine, endLine, redacted := excerptFromRange(lines, current.line, end, 12)
		if len(excerpt) < 2 {
			continue
		}
		items = append(items, makeEvidence(
			RoleDocumentedProcedure,
			filePath,
			startLine,
			endLine,
			current.text,
			excerpt,
			nil,
			"",
			"",
			redacted,
			false,
		))
	}
	return items
}

func isRSTUnderline(value string) bool {
	if len(value) < 3 {
		return false
	}
	for _, char := range value {
		if char != '=' && char != '-' && char != '~' && char != '^' {
			return false
		}
	}
	return true
}

func operationalHeading(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"quick start", "quickstart", "getting started", "setup", "install", "build", "develop", "run",
		"usage", "example", "test", "verify", "check", "deploy", "configuration", "configure", "docker",
		"troubleshoot", "restore", "backup", "monitor", "observability", "health", "contribut",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func parseFencedCommands(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	for index := 0; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index].text)
		marker, shell := shellFence(trimmed)
		if marker == "" || !shell {
			continue
		}
		start := index + 1
		end := start
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end].text), marker) {
			end++
		}
		if end > start {
			if item, ok := commandEvidence(filePath, lines, start, end-1, documentLabel(lines, index)); ok {
				items = append(items, item)
			}
		}
		index = end
	}
	return items
}

func shellFence(value string) (string, bool) {
	marker := ""
	switch {
	case strings.HasPrefix(value, "```"):
		marker = "```"
	case strings.HasPrefix(value, "~~~"):
		marker = "~~~"
	default:
		return "", false
	}
	language := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, marker)))
	if language == "" {
		return marker, true
	}
	for _, accepted := range []string{"sh", "shell", "bash", "zsh", "console", "terminal"} {
		if language == accepted {
			return marker, true
		}
	}
	return marker, false
}

func parseRSTCommands(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	for index, line := range lines {
		trimmed := strings.ToLower(strings.TrimSpace(line.text))
		if !strings.HasPrefix(trimmed, ".. code-block::") && !strings.HasPrefix(trimmed, ".. code::") {
			continue
		}
		language := strings.TrimSpace(strings.SplitN(trimmed, "::", 2)[1])
		if !slices.Contains([]string{"sh", "shell", "bash", "zsh", "console", "terminal"}, language) {
			continue
		}
		start := index + 1
		for start < len(lines) && strings.TrimSpace(lines[start].text) == "" {
			start++
		}
		end := start
		for end < len(lines) {
			text := lines[end].text
			if strings.TrimSpace(text) != "" && len(text)-len(strings.TrimLeftFunc(text, unicode.IsSpace)) == 0 {
				break
			}
			end++
		}
		if end > start {
			if item, ok := commandEvidence(filePath, lines, start, end-1, documentLabel(lines, index)); ok {
				items = append(items, item)
			}
		}
	}
	return items
}

func documentLabel(lines []collectedLine, before int) string {
	for index := before - 1; index >= 0; index-- {
		if lines[index].omitted {
			continue
		}
		trimmed := strings.TrimSpace(lines[index].text)
		if match := markdownHeadingRE.FindStringSubmatch(trimmed); len(match) == 3 {
			return strings.TrimSpace(match[2])
		}
		if index+1 < before && isRSTUnderline(strings.TrimSpace(lines[index+1].text)) && trimmed != "" {
			return trimmed
		}
	}
	return "Documented shell commands"
}

func commandEvidence(filePath string, lines []collectedLine, start, end int, label string) (Evidence, bool) {
	commands, excerpt, startLine, endLine, redacted := commandsFromRange(lines, start, end)
	if len(commands) == 0 || len(excerpt) == 0 {
		return Evidence{}, false
	}
	if redacted {
		for index := range commands {
			commands[index].SafeToCopy = false
		}
	}
	role := RoleDocumentedProcedure
	allVerification := true
	for _, command := range commands {
		if !verificationCommand(command.Value) {
			allVerification = false
			break
		}
	}
	if allVerification {
		role = RoleVerification
	}
	return makeEvidence(
		role,
		filePath,
		startLine,
		endLine,
		label,
		excerpt,
		commands,
		"",
		"",
		redacted,
		false,
	), true
}

func commandsFromRange(
	lines []collectedLine,
	start int,
	end int,
) ([]Command, []string, int, int, bool) {
	commands := make([]Command, 0)
	var excerpt []string
	startLine := 0
	endLine := 0
	redacted := false
	firstCommand := -1
	lastCommand := -1
	for index := start; index <= end && index < len(lines); index++ {
		line := lines[index]
		if line.omitted {
			redacted = redacted || line.redacted
			continue
		}
		value, prompted := stripShellPrompt(strings.TrimSpace(line.text))
		if value == "" || strings.HasPrefix(value, "#") || (!prompted && !looksLikeCommand(value)) {
			continue
		}
		commandStart := index
		commandEnd := index
		parts := []string{value}
		for strings.HasSuffix(strings.TrimSpace(parts[len(parts)-1]), "\\") && commandEnd < end {
			commandEnd++
			next := lines[commandEnd]
			if next.omitted {
				redacted = true
				continue
			}
			continuation, _ := stripShellPrompt(strings.TrimSpace(next.text))
			parts = append(parts, continuation)
			redacted = redacted || next.redacted
		}
		if firstCommand >= 0 && commandEnd-firstCommand+1 > collectorMaxExcerpt {
			break
		}
		value = strings.Join(parts, "\n")
		commandRedacted := line.redacted
		for offset := commandStart; offset <= commandEnd; offset++ {
			commandRedacted = commandRedacted || lines[offset].redacted
		}
		if commandRedacted {
			redacted = true
			index = commandEnd
			continue
		}
		commands = append(commands, Command{
			Value:      value,
			Basis:      CommandExact,
			SafeToCopy: safeToCopy(value, ""),
		})
		if firstCommand < 0 {
			firstCommand = commandStart
		}
		lastCommand = commandEnd
		redacted = redacted || commandRedacted
		index = commandEnd
	}
	if firstCommand >= 0 {
		excerpt, startLine, endLine, redacted = exactExcerptFromRange(
			lines,
			firstCommand,
			lastCommand,
			collectorMaxExcerpt,
		)
	}
	return commands, excerpt, startLine, endLine, redacted
}

func stripShellPrompt(value string) (string, bool) {
	for _, prefix := range []string{"$ ", "> ", "% "} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix)), true
		}
	}
	return value, false
}

func looksLikeCommand(value string) bool {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return false
	}
	if strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "=") {
		return true
	}
	first := fields[0]
	if first == "env" && len(fields) > 1 {
		first = fields[1]
	}
	if strings.HasPrefix(first, "./") || strings.HasPrefix(first, "../") {
		return true
	}
	first = path.Base(first)
	if slices.Contains([]string{
		"bash", "bun", "bundle", "cargo", "cd", "curl", "docker", "dotnet", "export", "git", "go",
		"gradle", "java", "just", "make", "mvn", "node", "npm", "npx", "pnpm", "python", "python3",
		"ruby", "sh", "task", "wget", "yarn",
	}, first) {
		return true
	}
	if len(fields) < 2 {
		return false
	}
	second := strings.TrimLeft(fields[1], "-")
	return slices.Contains([]string{
		"adapt", "backup", "build", "check", "doctor", "help", "info", "init", "list", "orient",
		"register", "reload", "replicate", "restore", "run", "serve", "start", "status", "stop", "sync",
		"test", "validate", "version",
	}, second)
}

func parseMakefile(filePath string, lines []collectedLine, fileTruncated bool) []Evidence {
	items := make([]Evidence, 0)
	for index, line := range lines {
		if line.omitted || strings.HasPrefix(line.text, "\t") {
			continue
		}
		match := makeTargetRE.FindStringSubmatch(line.text)
		if len(match) != 2 {
			continue
		}
		for _, target := range strings.Fields(match[1]) {
			if !collectableTarget(target) {
				continue
			}
			end := blockEnd(lines, index, func(candidate collectedLine) bool {
				return !strings.HasPrefix(candidate.text, "\t") && makeTargetRE.MatchString(candidate.text)
			})
			excerpt, startLine, endLine, redacted := excerptFromRange(lines, index, end, collectorMaxExcerpt)
			blockSource := strings.Join(lineTexts(lines[index:end+1], true), "\n")
			blockRedacted := collectedRangeRedacted(lines, index, end)
			commandValue := "make " + target
			items = append(items, makeEvidence(
				RoleBuildTarget,
				filePath,
				startLine,
				endLine,
				"Make target "+target,
				excerpt,
				[]Command{{
					Value:      commandValue,
					Basis:      CommandStructural,
					SafeToCopy: !fileTruncated && !redacted && !blockRedacted && safeToCopy(commandValue, blockSource),
				}},
				"",
				target,
				redacted,
				false,
			))
		}
	}
	return items
}

func parseTaskfile(filePath string, lines []collectedLine, fileTruncated bool) []Evidence {
	items := make([]Evidence, 0)
	tasksIndent := -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line.text)
		indent := leadingSpaceCount(line.text)
		if trimmed == "tasks:" {
			tasksIndent = indent
			continue
		}
		if tasksIndent < 0 || line.omitted || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if indent <= tasksIndent {
			tasksIndent = -1
			continue
		}
		match := taskTargetRE.FindStringSubmatch(line.text)
		if len(match) != 3 || len(match[1]) != tasksIndent+2 || !collectableTarget(match[2]) {
			continue
		}
		target := match[2]
		end := yamlBlockEnd(lines, index, indent)
		excerpt, startLine, endLine, redacted := excerptFromRange(lines, index, end, collectorMaxExcerpt)
		blockSource := strings.Join(lineTexts(lines[index:end+1], true), "\n")
		blockRedacted := collectedRangeRedacted(lines, index, end)
		commandValue := "task " + target
		items = append(items, makeEvidence(
			RoleBuildTarget,
			filePath,
			startLine,
			endLine,
			"Task target "+target,
			excerpt,
			[]Command{{
				Value:      commandValue,
				Basis:      CommandStructural,
				SafeToCopy: !fileTruncated && !redacted && !blockRedacted && safeToCopy(commandValue, blockSource),
			}},
			"",
			target,
			redacted,
			false,
		))
	}
	return items
}

func parseJustfile(filePath string, lines []collectedLine, fileTruncated bool) []Evidence {
	items := make([]Evidence, 0)
	for index, line := range lines {
		if line.omitted || leadingSpaceCount(line.text) != 0 {
			continue
		}
		match := justTargetRE.FindStringSubmatch(line.text)
		if len(match) != 2 || !collectableTarget(match[1]) {
			continue
		}
		target := match[1]
		end := blockEnd(lines, index, func(candidate collectedLine) bool {
			return leadingSpaceCount(candidate.text) == 0 && justTargetRE.MatchString(candidate.text)
		})
		excerpt, startLine, endLine, redacted := excerptFromRange(lines, index, end, collectorMaxExcerpt)
		blockSource := strings.Join(lineTexts(lines[index:end+1], true), "\n")
		blockRedacted := collectedRangeRedacted(lines, index, end)
		commandValue := "just " + target
		commands := []Command{{
			Value:      commandValue,
			Basis:      CommandStructural,
			SafeToCopy: !fileTruncated && !redacted && !blockRedacted && safeToCopy(commandValue, blockSource),
		}}
		signature := strings.TrimSpace(strings.SplitN(line.text, ":", 2)[0])
		if len(strings.Fields(signature)) > 1 {
			// Required recipe arguments have no repository-owned concrete values.
			commands = nil
		}
		items = append(items, makeEvidence(
			RoleBuildTarget,
			filePath,
			startLine,
			endLine,
			"Just recipe "+target,
			excerpt,
			commands,
			"",
			target,
			redacted,
			false,
		))
	}
	return items
}

func collectableTarget(target string) bool {
	if target == "" || strings.ContainsAny(target, "%/$(){}") {
		return false
	}
	for _, char := range target {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func blockEnd(lines []collectedLine, start int, next func(collectedLine) bool) int {
	end := start
	for index := start + 1; index < len(lines); index++ {
		if next(lines[index]) {
			break
		}
		end = index
	}
	return end
}

func yamlBlockEnd(lines []collectedLine, start, indent int) int {
	end := start
	for index := start + 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index].text) != "" && leadingSpaceCount(lines[index].text) <= indent {
			break
		}
		end = index
	}
	return end
}

func parsePackageJSON(filePath string, lines []collectedLine) []Evidence {
	var decoded struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
	}
	raw := strings.Join(lineTexts(lines, false), "\n")
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	manager := strings.SplitN(decoded.PackageManager, "@", 2)[0]
	if !slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, manager) {
		manager = "npm"
	}
	names := make([]string, 0, len(decoded.Scripts))
	for name := range decoded.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]Evidence, 0, len(names))
	for _, name := range names {
		if !collectableTarget(name) {
			continue
		}
		script, changed := redactSensitiveLine(decoded.Scripts[name], false)
		if strings.Contains(strings.ToLower(script), "authorization:") || strings.TrimSpace(script) == "" {
			continue
		}
		excerpt, startLine, endLine, lineRedacted, ok := packageScriptSource(
			lines,
			raw,
			name,
			decoded.Scripts[name],
		)
		if !ok {
			continue
		}
		redacted := changed || lineRedacted
		commandValue := manager + " run " + name
		items = append(items, makeEvidence(
			RolePackageScript,
			filePath,
			startLine,
			endLine,
			"Package script "+name,
			excerpt,
			[]Command{{
				Value:      commandValue,
				Basis:      CommandStructural,
				SafeToCopy: !redacted && safeToCopy(commandValue, script),
			}},
			"",
			name,
			redacted,
			false,
		))
	}
	return items
}

func packageScriptSource(
	lines []collectedLine,
	raw string,
	name string,
	expected string,
) ([]string, int, int, bool, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, 0, 0, false, false
	}
	scriptsObjects := 0
	matches := 0
	var matchedExcerpt []string
	var matchedStart, matchedEnd int
	var matchedRedacted bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, 0, 0, false, false
		}
		if key != "scripts" {
			if err := skipJSONValue(decoder); err != nil {
				return nil, 0, 0, false, false
			}
			continue
		}
		scriptsObjects++
		if scriptsObjects != 1 {
			return nil, 0, 0, false, false
		}
		opening, err := decoder.Token()
		if err != nil || opening != json.Delim('{') {
			return nil, 0, 0, false, false
		}
		for decoder.More() {
			scriptName, err := decoder.Token()
			if err != nil {
				return nil, 0, 0, false, false
			}
			start := strings.Count(raw[:min(int(decoder.InputOffset()), len(raw))], "\n")
			value, err := decoder.Token()
			if err != nil {
				return nil, 0, 0, false, false
			}
			end := strings.Count(raw[:min(int(decoder.InputOffset()), len(raw))], "\n")
			if scriptName != name {
				continue
			}
			matches++
			stringValue, stringOK := value.(string)
			if matches != 1 || !stringOK || stringValue != expected ||
				start < 0 || end < start ||
				end >= len(lines) || end-start+1 > collectorMaxExcerpt {
				return nil, 0, 0, false, false
			}
			for index := start; index <= end; index++ {
				if lines[index].omitted || lines[index].truncated {
					return nil, 0, 0, false, false
				}
			}
			matchedExcerpt, matchedStart, matchedEnd, matchedRedacted = exactExcerptFromRange(
				lines,
				start,
				end,
				collectorMaxExcerpt,
			)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, 0, 0, false, false
		}
	}
	if _, err := decoder.Token(); err != nil || scriptsObjects != 1 || matches != 1 || len(matchedExcerpt) == 0 {
		return nil, 0, 0, false, false
	}
	return matchedExcerpt, matchedStart, matchedEnd, matchedRedacted, true
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, nested := token.(json.Delim)
	if !nested || delimiter != '{' && delimiter != '[' {
		return nil
	}
	for decoder.More() {
		if delimiter == '{' {
			if _, err := decoder.Token(); err != nil {
				return err
			}
		}
		if err := skipJSONValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func parseExecutableScript(filePath string, lines []collectedLine, truncated bool) []Evidence {
	excerpt, startLine, endLine, redacted := excerptFromRange(lines, 0, len(lines)-1, collectorMaxExcerpt)
	if len(excerpt) == 0 {
		return nil
	}
	commandValue := "./" + filePath
	body := strings.Join(lineTexts(lines, true), "\n")
	safe := !truncated && !redacted && safeToCopy(commandValue, body)
	return []Evidence{makeEvidence(
		RoleRepositoryScript,
		filePath,
		startLine,
		endLine,
		"Executable repository script "+path.Base(filePath),
		excerpt,
		[]Command{{Value: commandValue, Basis: CommandStructural, SafeToCopy: safe}},
		"",
		filePath,
		redacted,
		true,
	)}
}

func parseCompose(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	servicesIndent := -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line.text)
		indent := leadingSpaceCount(line.text)
		if trimmed == "services:" {
			servicesIndent = indent
			continue
		}
		if servicesIndent < 0 || line.omitted || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if indent <= servicesIndent {
			servicesIndent = -1
			continue
		}
		if indent != servicesIndent+2 || !strings.HasSuffix(trimmed, ":") {
			continue
		}
		service := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
		if service == "" || strings.ContainsAny(service, " &*{}[]") {
			continue
		}
		end := yamlBlockEnd(lines, index, indent)
		excerpt, startLine, endLine, redacted := excerptFromRange(lines, index, end, collectorMaxExcerpt)
		commandValue := composeCommand(filePath, service)
		items = append(items, makeEvidence(
			RoleComposeService,
			filePath,
			startLine,
			endLine,
			"Compose service "+service,
			excerpt,
			[]Command{{Value: commandValue, Basis: CommandStructural, SafeToCopy: false}},
			"",
			service,
			redacted,
			false,
		))
	}
	return items
}

func composeCommand(filePath, service string) string {
	base := strings.ToLower(path.Base(filePath))
	if slices.Contains([]string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"}, base) &&
		!strings.Contains(filePath, "/") {
		return "docker compose up " + service
	}
	return "docker compose -f " + filePath + " up " + service
}

func parseEnvironment(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	for _, line := range lines {
		if line.omitted {
			continue
		}
		match := environmentRE.FindStringSubmatch(line.text)
		if len(match) != 3 {
			continue
		}
		name := match[1]
		items = append(items, makeEvidence(
			RoleEnvironment,
			filePath,
			line.number,
			line.number,
			"Environment variable "+name,
			[]string{name + "=<redacted>"},
			nil,
			"",
			name,
			true,
			false,
		))
	}
	return items
}

func parseDockerfile(filePath string, lines []collectedLine) []Evidence {
	excerpt, startLine, endLine, redacted := excerptFromRange(lines, 0, len(lines)-1, collectorMaxExcerpt)
	if len(excerpt) == 0 {
		return nil
	}
	commandValue := ""
	if !strings.Contains(filePath, "/") && strings.EqualFold(filePath, "Dockerfile") {
		commandValue = "docker build ."
	}
	var commands []Command
	if commandValue != "" {
		commands = []Command{{Value: commandValue, Basis: CommandStructural, SafeToCopy: false}}
	}
	return []Evidence{makeEvidence(
		RoleConfiguration,
		filePath,
		startLine,
		endLine,
		"Container build configuration",
		excerpt,
		commands,
		"",
		filePath,
		redacted,
		false,
	)}
}

func parseConfiguration(filePath string, lines []collectedLine) []Evidence {
	excerpt, startLine, endLine, redacted := excerptFromRange(lines, 0, len(lines)-1, collectorMaxExcerpt)
	if len(excerpt) == 0 {
		return nil
	}
	return []Evidence{makeEvidence(
		RoleConfiguration,
		filePath,
		startLine,
		endLine,
		"Operational configuration "+path.Base(filePath),
		excerpt,
		nil,
		"",
		filePath,
		redacted,
		false,
	)}
}

func parseLocalEndpoints(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	for _, line := range lines {
		if line.omitted {
			continue
		}
		for _, endpoint := range localEndpointRE.FindAllString(line.text, -1) {
			endpoint = strings.TrimRight(endpoint, ".,;:`")
			items = append(items, makeEvidence(
				RoleEndpoint,
				filePath,
				line.number,
				line.number,
				"Local endpoint "+endpoint,
				[]string{line.text},
				nil,
				endpoint,
				"",
				line.redacted,
				false,
			))
		}
	}
	return items
}

func parseLogLocations(filePath string, lines []collectedLine) []Evidence {
	items := make([]Evidence, 0)
	for _, line := range lines {
		if line.omitted {
			continue
		}
		for _, location := range logPathRE.FindAllString(line.text, -1) {
			items = append(items, makeEvidence(
				RoleLogLocation,
				filePath,
				line.number,
				line.number,
				"Log location "+location,
				[]string{line.text},
				nil,
				"",
				location,
				line.redacted,
				false,
			))
		}
	}
	return items
}

func excerptFromRange(
	lines []collectedLine,
	start int,
	end int,
	limit int,
) ([]string, int, int, bool) {
	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for start <= end && (lines[start].omitted || strings.TrimSpace(lines[start].text) == "") {
		start++
	}
	for end >= start && (lines[end].omitted || strings.TrimSpace(lines[end].text) == "") {
		end--
	}
	return exactExcerptFromRange(lines, start, end, limit)
}

// exactExcerptFromRange preserves one source line per saved excerpt line.
// Secret-bearing lines are represented by a fixed placeholder, so redaction
// never shifts the line numbers shown in the report.
func exactExcerptFromRange(
	lines []collectedLine,
	start int,
	end int,
	limit int,
) ([]string, int, int, bool) {
	if start < 0 || start >= len(lines) || end < start || limit <= 0 {
		return nil, 0, 0, false
	}
	end = min(end, len(lines)-1, start+limit-1)
	excerpt := make([]string, 0, end-start+1)
	redacted := false
	for index := start; index <= end; index++ {
		line := lines[index]
		redacted = redacted || line.redacted
		if line.omitted {
			excerpt = append(excerpt, "<redacted line>")
			continue
		}
		excerpt = append(excerpt, line.text)
	}
	return excerpt, lines[start].number, lines[end].number, redacted
}

func makeEvidence(
	role EvidenceRole,
	filePath string,
	startLine int,
	endLine int,
	label string,
	excerpt []string,
	commands []Command,
	endpoint string,
	target string,
	redacted bool,
	executable bool,
) Evidence {
	commandValues := make([]string, 0, len(commands))
	for _, command := range commands {
		commandValues = append(commandValues, command.Value)
	}
	identity := strings.Join([]string{
		string(role), filePath, strconv.Itoa(startLine), strconv.Itoa(endLine), target, endpoint,
		strings.Join(commandValues, "\x00"),
	}, "\x00")
	return Evidence{
		ID:         stableID("operational-evidence", identity),
		Role:       role,
		Path:       filePath,
		StartLine:  startLine,
		EndLine:    endLine,
		Label:      limitLabel(label),
		Excerpt:    excerpt,
		Commands:   commands,
		Endpoint:   endpoint,
		Target:     target,
		Redacted:   redacted,
		Executable: executable,
	}
}

func limitLabel(value string) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "�"))
	if len(value) <= 256 {
		return value
	}
	value = value[:253]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "..."
}

func uniqueEvidence(items []Evidence) []Evidence {
	seen := make(map[string]struct{}, len(items))
	result := make([]Evidence, 0, len(items))
	for _, item := range items {
		if item.StartLine <= 0 || item.EndLine < item.StartLine || len(item.Excerpt) == 0 {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func leadingSpaceCount(value string) int {
	return len(value) - len(strings.TrimLeft(value, " \t"))
}

func lineTexts(lines []collectedLine, omitSensitive bool) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.omitted && omitSensitive {
			continue
		}
		if line.omitted {
			result = append(result, "")
			continue
		}
		result = append(result, line.text)
	}
	return result
}

func collectedRangeRedacted(lines []collectedLine, start, end int) bool {
	start = max(0, start)
	end = min(end, len(lines)-1)
	for index := start; index <= end; index++ {
		if lines[index].redacted || lines[index].omitted {
			return true
		}
	}
	return false
}

func verificationCommand(value string) bool {
	words := commandWords(value)
	return containsAnyWord(words, "test", "check", "verify", "vet", "lint", "fmt", "format", "doctor", "smoke", "status")
}

func safeToCopy(command, source string) bool {
	if command == "" || strings.ContainsAny(command, "\n\r;&|><`*?[]{}") || strings.Contains(command, "$(") ||
		strings.HasSuffix(strings.TrimSpace(command), "\\") || placeholderRE.MatchString(command) {
		return false
	}
	if strings.ContainsAny(source, ";&|><`*?[]{}") || strings.Contains(source, "$(") || placeholderRE.MatchString(source) {
		return false
	}
	lower := strings.ToLower(command + "\n" + source)
	if containsCredential(lower) || strings.Contains(lower, "<redacted>") || strings.Contains(lower, "authorization:") {
		return false
	}
	words := commandWords(lower)
	if containsAnyWord(words,
		"sudo", "rm", "reset", "restore", "init", "migrate", "migration", "seed", "prune", "delete", "remove",
		"drop", "truncate", "destroy", "purge", "deploy", "publish", "push", "upload", "release", "apply",
		"backup", "register", "unregister", "reload", "install", "clean", "setcap",
	) {
		return false
	}
	if remoteWriteCommand(words, lower) {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 0 || len(fields) > 16 {
		return false
	}
	first := path.Base(fields[0])
	if first == "go" && len(fields) >= 2 {
		return slices.Contains([]string{"build", "test", "vet", "run"}, fields[1])
	}
	if slices.Contains([]string{"make", "task", "just"}, first) && len(fields) == 2 {
		return lowRiskName(fields[1])
	}
	if slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, first) {
		name := fields[len(fields)-1]
		return lowRiskName(name)
	}
	if strings.HasPrefix(fields[0], "./") {
		return lowRiskName(path.Base(fields[0])) && sourceLooksLowRisk(source)
	}
	if slices.Contains(fields, "--help") || (len(fields) == 2 && slices.Contains([]string{"help", "version"}, fields[1])) {
		return true
	}
	if first == "curl" && len(fields) == 2 && localEndpointRE.MatchString(fields[1]) {
		return true
	}
	return false
}

func lowRiskName(value string) bool {
	lower := strings.ToLower(strings.TrimSuffix(value, path.Ext(value)))
	for _, marker := range []string{"build", "test", "check", "verify", "vet", "lint", "fmt", "format", "help", "doctor", "smoke", "run", "serve", "dev", "bench"} {
		if lower == marker || strings.HasPrefix(lower, marker+"-") || strings.HasSuffix(lower, "_"+marker) ||
			strings.HasSuffix(lower, "-"+marker) {
			return true
		}
	}
	return false
}

func sourceLooksLowRisk(source string) bool {
	if source == "" || strings.ContainsAny(source, ";|><`") || strings.Contains(source, "$(") {
		return false
	}
	words := commandWords(strings.ToLower(source))
	return !containsAnyWord(words,
		"sudo", "rm", "reset", "restore", "init", "migrate", "seed", "prune", "delete", "remove", "drop",
		"truncate", "destroy", "purge", "deploy", "publish", "push", "upload", "release", "apply", "backup",
		"register", "unregister", "reload", "install", "clean", "setcap",
	) && !remoteWriteCommand(words, strings.ToLower(source))
}

func remoteWriteCommand(words []string, lower string) bool {
	if containsAnyWord(words, "aws", "kubectl", "terraform", "fly", "gcloud", "az", "helm") {
		return true
	}
	if strings.Contains(lower, "docker push") || strings.Contains(lower, "npm publish") ||
		strings.Contains(lower, "curl -x post") || strings.Contains(lower, "curl --request post") ||
		strings.Contains(lower, "curl -d ") || strings.Contains(lower, "curl --data") {
		return true
	}
	return false
}

func commandWords(value string) []string {
	return strings.FieldsFunc(value, func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' && char != '-'
	})
}

func containsAnyWord(words []string, values ...string) bool {
	for _, word := range words {
		for _, value := range values {
			if word == value {
				return true
			}
		}
	}
	return false
}
