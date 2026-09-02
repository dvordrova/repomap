package facts

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
)

type manifestRow struct {
	key   string
	value string
	line  int
}

var pinnedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`)

func isManifestName(name string) bool {
	switch name {
	case "package.json", "Pipfile", "pyproject.toml", "go.mod":
		return true
	}
	return strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt")
}

// addManifests quotes run-relevant keys from every manifest in the corpus:
// the targets' own manifests first, then any other manifest file tracked in
// the repository (attributed to a target when it lies under its root).
func (b *builder) addManifests() {
	set := make(map[string]struct{})
	for _, target := range b.targets {
		if target.target.Manifest != "" && b.source.has(target.target.Manifest) {
			set[target.target.Manifest] = struct{}{}
		}
	}
	for _, filePath := range b.source.paths() {
		if isManifestName(path.Base(filePath)) {
			set[filePath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(set))
	for filePath := range set {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		b.addManifestRows(filePath)
	}
	b.addEnvFiles()
}

func (b *builder) addManifestRows(filePath string) {
	file, ok := b.source.file(filePath)
	if !ok || file.binary {
		return
	}
	rows, err := parseManifest(path.Base(filePath), file.lines)
	if err != nil {
		b.diagnose("manifest_unreadable", filePath+": "+err.Error())
		return
	}
	targetID := b.targetForPath(filePath)
	root := b.rootForTarget(targetID)
	for _, row := range rows {
		if row.key == "" {
			continue
		}
		anchor := Anchor{Path: filePath, Line: row.line}
		if !b.once(strings.Join([]string{string(KindManifest), filePath, row.key}, "\x00")) {
			continue
		}
		b.add(root, Fact{
			Kind:     KindManifest,
			TargetID: targetID,
			Anchor:   &anchor,
			Key:      row.key,
			Value:    clipText(row.value),
		}, row.key, row.value)
	}
}

func (b *builder) rootForTarget(targetID string) string {
	if target, ok := b.targetByID(targetID); ok {
		return target.Root
	}
	return "."
}

// addEnvFiles records committed .env files by path only. Their contents are
// never read: key names would be a new disclosure the product has not
// approved.
func (b *builder) addEnvFiles() {
	paths := append([]string(nil), b.input.TrackedPaths...)
	sort.Strings(paths)
	for _, filePath := range paths {
		if filePath == "" || !strings.HasPrefix(path.Base(filePath), ".env") || path.Clean(filePath) != filePath {
			continue
		}
		if !b.once(strings.Join([]string{string(KindManifest), filePath, "env_file"}, "\x00")) {
			continue
		}
		targetID := b.targetForPath(filePath)
		b.add(b.rootForTarget(targetID), Fact{
			Kind:     KindManifest,
			TargetID: targetID,
			Anchor:   &Anchor{Path: filePath, Line: 1},
			Key:      "env_file",
			Value:    filePath,
		}, "env_file", filePath)
	}
}

func parseManifest(name string, lines []string) ([]manifestRow, error) {
	switch {
	case name == "package.json":
		return parsePackageJSON(lines)
	case name == "Pipfile":
		return parsePipfile(lines), nil
	case name == "pyproject.toml":
		return parsePyproject(lines), nil
	case name == "go.mod":
		return parseGoMod(lines), nil
	default:
		return parseRequirements(lines), nil
	}
}

func parsePackageJSON(lines []string) ([]manifestRow, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &document); err != nil {
		return nil, err
	}
	var rows []manifestRow
	rows = append(rows, jsonStringRows(lines, document, "scripts", "scripts.", nil)...)
	rows = append(rows, jsonStringRows(lines, document, "engines", "engines.", nil)...)
	rows = append(rows, jsonStringRows(lines, document, "dependencies", "dependency.", pinnedOnly)...)
	rows = append(rows, jsonStringRows(lines, document, "devDependencies", "dev_dependency.", pinnedOnly)...)
	if raw, ok := document["proxy"]; ok {
		if value, isString := jsonString(raw); isString {
			rows = append(rows, manifestRow{key: "proxy", value: value, line: jsonKeyLine(lines, "proxy", 0)})
		}
	}
	if raw, ok := document["bin"]; ok {
		if value, isString := jsonString(raw); isString {
			rows = append(rows, manifestRow{key: "bin", value: value, line: jsonKeyLine(lines, "bin", 0)})
		} else {
			rows = append(rows, jsonStringRows(lines, document, "bin", "bin.", nil)...)
		}
	}
	return rows, nil
}

func pinnedOnly(value string) bool {
	return pinnedVersion.MatchString(value)
}

// jsonStringRows quotes the string members of one object section, locating
// each key on the first line after the section header that declares it.
func jsonStringRows(lines []string, document map[string]json.RawMessage, section, prefix string, accept func(string) bool) []manifestRow {
	raw, ok := document[section]
	if !ok {
		return nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil
	}
	sectionLine := jsonKeyLine(lines, section, 0)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	var rows []manifestRow
	for _, name := range names {
		value, isString := jsonString(members[name])
		if !isString || accept != nil && !accept(value) {
			continue
		}
		rows = append(rows, manifestRow{key: prefix + name, value: value, line: jsonKeyLine(lines, name, sectionLine)})
	}
	return rows
}

func jsonString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// jsonKeyLine finds the 1-based line that declares "key": at or after from.
// A key that opens its own line wins; an inline object ("engines": {"node":
// ...}) is found by a second pass over whole lines from the section line.
// It falls back to line 1 so a row is never left without an anchor.
func jsonKeyLine(lines []string, key string, from int) int {
	quoted := "\"" + key + "\""
	declares := func(text string) bool {
		return strings.HasPrefix(text, quoted) && strings.HasPrefix(strings.TrimSpace(text[len(quoted):]), ":")
	}
	for number := from; number < len(lines); number++ {
		if declares(strings.TrimSpace(lines[number])) {
			return number + 1
		}
	}
	for number := max(from-1, 0); number < len(lines); number++ {
		if offset := strings.Index(lines[number], quoted); offset >= 0 && declares(lines[number][offset:]) {
			return number + 1
		}
	}
	return 1
}

var tomlAssignment = regexp.MustCompile(`^\s*("?)([A-Za-z0-9_.\-]+)("?)\s*=\s*(.*)$`)
var tomlInlineVersion = regexp.MustCompile(`version\s*=\s*"([^"]*)"`)

// parsePipfile reads the [packages], [dev-packages] and [requires] sections.
func parsePipfile(lines []string) []manifestRow {
	var rows []manifestRow
	section := ""
	for number, line := range lines {
		trimmed := strings.TrimSpace(line)
		if header, ok := tomlSection(trimmed); ok {
			section = header
			continue
		}
		switch section {
		case "packages", "dev-packages", "requires":
		default:
			continue
		}
		if key, value, ok := tomlKeyValue(trimmed); ok {
			rows = append(rows, manifestRow{key: section + "." + key, value: value, line: number + 1})
		}
	}
	return rows
}

// parsePyproject reads the PEP 621 [project] table and the Poetry tables.
func parsePyproject(lines []string) []manifestRow {
	var rows []manifestRow
	section := ""
	inDependencies := false
	for number, line := range lines {
		trimmed := strings.TrimSpace(line)
		if header, ok := tomlSection(trimmed); ok {
			section, inDependencies = header, false
			continue
		}
		if inDependencies {
			if strings.HasPrefix(trimmed, "]") {
				inDependencies = false
				continue
			}
			rows = append(rows, requirementRows(trimmed, "project.dependencies.", number+1)...)
			continue
		}
		key, value, ok := tomlKeyValue(trimmed)
		if !ok {
			continue
		}
		switch {
		case section == "project" && key == "dependencies":
			if strings.HasSuffix(value, "[") || value == "" {
				inDependencies = true
				continue
			}
			rows = append(rows, requirementRows(value, "project.dependencies.", number+1)...)
		case section == "project" && (key == "name" || key == "requires-python" || key == "version"):
			rows = append(rows, manifestRow{key: "project." + key, value: value, line: number + 1})
		case section == "tool.poetry" && (key == "name" || key == "version"):
			rows = append(rows, manifestRow{key: "tool.poetry." + key, value: value, line: number + 1})
		case section == "tool.poetry.dependencies" || section == "tool.poetry.dev-dependencies":
			rows = append(rows, manifestRow{key: section + "." + key, value: value, line: number + 1})
		}
	}
	return rows
}

var quotedRequirement = regexp.MustCompile(`"([^"]+)"|'([^']+)'`)
var requirementName = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)`)

func requirementRows(text, prefix string, line int) []manifestRow {
	var rows []manifestRow
	for _, match := range quotedRequirement.FindAllStringSubmatch(text, -1) {
		spec := match[1]
		if spec == "" {
			spec = match[2]
		}
		name := requirementName.FindString(spec)
		if name == "" {
			continue
		}
		rows = append(rows, manifestRow{key: prefix + name, value: spec, line: line})
	}
	return rows
}

func tomlSection(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return strings.Trim(trimmed, "[]"), true
	}
	return "", false
}

// tomlKeyValue reads one "key = value" line; quoted strings are unquoted,
// inline tables reduce to their version member, other values stay verbatim.
func tomlKeyValue(trimmed string) (string, string, bool) {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	match := tomlAssignment.FindStringSubmatch(trimmed)
	if match == nil {
		return "", "", false
	}
	key, raw := match[2], strings.TrimSpace(match[4])
	switch {
	case strings.HasPrefix(raw, "\"") || strings.HasPrefix(raw, "'"):
		quote := raw[:1]
		end := strings.Index(raw[1:], quote)
		if end < 0 {
			return key, strings.Trim(raw, quote), true
		}
		return key, raw[1 : 1+end], true
	case strings.HasPrefix(raw, "{"):
		if version := tomlInlineVersion.FindStringSubmatch(raw); version != nil {
			return key, version[1], true
		}
		return key, raw, true
	default:
		if comment := strings.Index(raw, "#"); comment >= 0 {
			raw = strings.TrimSpace(raw[:comment])
		}
		return key, raw, true
	}
}

// parseRequirements keeps only exact pins: "name==version".
func parseRequirements(lines []string) []manifestRow {
	var rows []manifestRow
	for number, line := range lines {
		trimmed := strings.TrimSpace(line)
		if comment := strings.Index(trimmed, "#"); comment >= 0 {
			trimmed = strings.TrimSpace(trimmed[:comment])
		}
		name, version, ok := strings.Cut(trimmed, "==")
		if !ok || requirementName.FindString(name) != name {
			continue
		}
		rows = append(rows, manifestRow{key: "requirements." + name, value: strings.TrimSpace(version), line: number + 1})
	}
	return rows
}

func parseGoMod(lines []string) []manifestRow {
	var rows []manifestRow
	for number, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "module", "go", "toolchain":
			rows = append(rows, manifestRow{key: fields[0], value: fields[1], line: number + 1})
		}
	}
	return rows
}
