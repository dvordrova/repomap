package pythondeclareddependencies

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/dvordrova/repomap/internal/dependencydeclaration"
)

func (state *builder) parsePyproject(source *sourceDraft) error {
	var document map[string]any
	if err := toml.Unmarshal(source.content, &document); err != nil {
		return fmt.Errorf("python declared dependencies: parse %q: %w", source.entry.Path, err)
	}
	ordinal := 0
	nextOrdinal := func() int {
		ordinal++
		return ordinal
	}

	if project, exists := mapValue(document, "project"); exists {
		state.parsePEP621Array(source, project, "dependencies", "project.dependencies",
			dependencydeclaration.RoleRuntime, "", nextOrdinal)
		if optional, exists := mapValue(project, "optional-dependencies"); exists {
			groups := sortedMapKeys(optional)
			for _, group := range groups {
				section := "project.optional-dependencies." + group
				state.parsePEP621Value(source, optional[group], section,
					dependencydeclaration.RoleOptional, group, nextOrdinal)
			}
		} else if raw, present := project["optional-dependencies"]; present {
			state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
				dependencydeclaration.FrontierUnsupportedShape, "project.optional-dependencies",
				nextOrdinal(), raw)
		}
		if dynamic, present := project["dynamic"]; present {
			values, ok := stringSlice(dynamic)
			if !ok {
				state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
					dependencydeclaration.FrontierUnsupportedShape, "project.dynamic", nextOrdinal(), dynamic)
			} else {
				for _, name := range values {
					if name != "dependencies" && name != "optional-dependencies" {
						continue
					}
					state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
						dependencydeclaration.FrontierDynamicDeclaration, "project.dynamic",
						nextOrdinal(), name)
				}
			}
		}
	} else if raw, present := document["project"]; present {
		state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
			dependencydeclaration.FrontierUnsupportedShape, "project", nextOrdinal(), raw)
	}

	if groups, exists := mapValue(document, "dependency-groups"); exists {
		for _, group := range sortedMapKeys(groups) {
			section := "dependency-groups." + group
			values, ok := groups[group].([]any)
			if !ok {
				state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
					dependencydeclaration.FrontierUnsupportedShape, section, nextOrdinal(), groups[group])
				continue
			}
			for _, value := range values {
				expression, ok := value.(string)
				if !ok {
					// PEP 735 include-group and future composite item forms are
					// explicit boundaries until an exact local expansion exists.
					state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
						dependencydeclaration.FrontierUnsupportedShape, section, nextOrdinal(), value)
					continue
				}
				state.parsePEP621Value(source, []any{expression}, section,
					dependencydeclaration.RoleUnspecified, group, nextOrdinal)
			}
		}
	} else if raw, present := document["dependency-groups"]; present {
		state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
			dependencydeclaration.FrontierUnsupportedShape, "dependency-groups", nextOrdinal(), raw)
	}

	if buildSystem, exists := mapValue(document, "build-system"); exists {
		state.parsePEP621Array(source, buildSystem, "requires", "build-system.requires",
			dependencydeclaration.RoleBuild, "", nextOrdinal)
	} else if raw, present := document["build-system"]; present {
		state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
			dependencydeclaration.FrontierUnsupportedShape, "build-system", nextOrdinal(), raw)
	}

	tool, _ := mapValue(document, "tool")
	poetry, poetryExists := mapValue(tool, "poetry")
	if poetryExists {
		state.parsePoetryTable(source, poetry, "dependencies", "tool.poetry.dependencies",
			dependencydeclaration.RoleRuntime, "", nextOrdinal)
		state.parsePoetryTable(source, poetry, "dev-dependencies", "tool.poetry.dev-dependencies",
			dependencydeclaration.RoleDevelopment, "dev", nextOrdinal)
		if groups, exists := mapValue(poetry, "group"); exists {
			for _, group := range sortedMapKeys(groups) {
				groupValue, ok := groups[group].(map[string]any)
				if !ok {
					state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
						dependencydeclaration.FrontierUnsupportedShape, "tool.poetry.group."+group,
						nextOrdinal(), groups[group])
					continue
				}
				state.parsePoetryTable(source, groupValue, "dependencies",
					"tool.poetry.group."+group+".dependencies",
					dependencydeclaration.RoleUnspecified, group, nextOrdinal)
			}
		} else if raw, present := poetry["group"]; present {
			state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
				dependencydeclaration.FrontierUnsupportedShape, "tool.poetry.group", nextOrdinal(), raw)
		}
	}
	if len(state.statements) > dependencydeclaration.MaxStatements || len(state.frontiers) > dependencydeclaration.MaxFrontiers {
		return fmt.Errorf("python declared dependencies: pyproject artifact bound exceeded")
	}
	return nil
}

func (state *builder) parsePEP621Array(
	source *sourceDraft,
	table map[string]any,
	key, section string,
	role dependencydeclaration.Role,
	group string,
	nextOrdinal func() int,
) {
	raw, exists := table[key]
	if !exists {
		return
	}
	state.parsePEP621Value(source, raw, section, role, group, nextOrdinal)
}

func (state *builder) parsePEP621Value(
	source *sourceDraft,
	raw any,
	section string,
	role dependencydeclaration.Role,
	group string,
	nextOrdinal func() int,
) {
	values, ok := stringSlice(raw)
	if !ok {
		state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
			dependencydeclaration.FrontierUnsupportedShape, section, nextOrdinal(), raw)
		return
	}
	for _, expression := range values {
		ordinal := nextOrdinal()
		parsed, err := parseRequirementExpression(expression, source.entry.Path, state.projectDir)
		if err != nil {
			reason := dependencydeclaration.FrontierUnsupportedRequirement
			if strings.Contains(err.Error(), "package identity") {
				reason = dependencydeclaration.FrontierPackageIdentityUnavailable
			}
			state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
				reason, section, ordinal, expression)
			continue
		}
		state.statements = append(state.statements, dependencydeclaration.StatementInput{
			SourceKey: source.key, Kind: dependencydeclaration.StatementRequirement, Role: role,
			Group: group, Name: parsed.name, NormalizedName: parsed.normalized, Extras: parsed.extras,
			Specifier: parsed.specifier, Conditional: parsed.conditional, Locator: parsed.locator,
			Section: section, Ordinal: ordinal, ExpressionSHA256: textSHA256(expression),
		})
	}
}

func (state *builder) parsePoetryTable(
	source *sourceDraft,
	table map[string]any,
	key, section string,
	role dependencydeclaration.Role,
	group string,
	nextOrdinal func() int,
) {
	raw, exists := table[key]
	if !exists {
		return
	}
	values, ok := raw.(map[string]any)
	if !ok {
		state.addManifestFrontier(source, dependencydeclaration.FrontierDirective,
			dependencydeclaration.FrontierUnsupportedShape, section, nextOrdinal(), raw)
		return
	}
	for _, name := range sortedMapKeys(values) {
		if section == "tool.poetry.dependencies" && strings.EqualFold(name, "python") {
			continue
		}
		state.parsePoetryValue(source, name, values[name], section, role, group, nextOrdinal)
	}
}

func (state *builder) parsePoetryValue(
	source *sourceDraft,
	name string,
	raw any,
	section string,
	role dependencydeclaration.Role,
	group string,
	nextOrdinal func() int,
) {
	if values, ok := raw.([]any); ok {
		if len(values) == 0 {
			state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
				dependencydeclaration.FrontierUnsupportedShape, section, nextOrdinal(), raw)
			return
		}
		for _, value := range values {
			state.parsePoetryValue(source, name, value, section, role, group, nextOrdinal)
		}
		return
	}
	ordinal := nextOrdinal()
	if !validDistributionName(name) {
		state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
			dependencydeclaration.FrontierPackageIdentityUnavailable, section, ordinal, raw)
		return
	}
	statementRole := role
	statement := dependencydeclaration.StatementInput{
		SourceKey: source.key, Kind: dependencydeclaration.StatementRequirement,
		Role: role, Group: group, Name: name, NormalizedName: normalizeDistributionName(name),
		Locator: dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorRegistry},
		Section: section, Ordinal: ordinal, ExpressionSHA256: semanticSHA256(raw),
	}
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "git+") {
			statement.Locator = safeLocator(trimmed, path.Dir(source.entry.Path), state.projectDir)
		} else if safeVersionExpression(trimmed) {
			statement.Specifier = trimmed
		} else {
			state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
				dependencydeclaration.FrontierUnsupportedShape, section, ordinal, raw)
			return
		}
	case map[string]any:
		if version, ok := value["version"].(string); ok {
			version = strings.TrimSpace(version)
			if !safeVersionExpression(version) {
				state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
					dependencydeclaration.FrontierUnsupportedShape, section, ordinal, version)
				return
			}
			statement.Specifier = version
		} else if rawVersion, exists := value["version"]; exists {
			state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
				dependencydeclaration.FrontierUnsupportedShape, section, ordinal, rawVersion)
			return
		}
		if optional, ok := value["optional"].(bool); ok && optional {
			statementRole = dependencydeclaration.RoleOptional
		} else if rawOptional, exists := value["optional"]; exists {
			if _, ok := rawOptional.(bool); !ok {
				state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
					dependencydeclaration.FrontierUnsupportedShape, section, ordinal, rawOptional)
				return
			}
		}
		statement.Role = statementRole
		if extras, exists := value["extras"]; exists {
			parsedExtras, ok := stringSlice(extras)
			if !ok {
				state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
					dependencydeclaration.FrontierUnsupportedShape, section, ordinal, extras)
				return
			}
			statement.Extras = canonicalStrings(parsedExtras)
		}
		if markers, exists := value["markers"]; exists {
			marker, ok := markers.(string)
			if !ok {
				state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
					dependencydeclaration.FrontierUnsupportedShape, section, ordinal, markers)
				return
			}
			statement.Conditional = strings.TrimSpace(marker) != ""
		}
		locatorFields := 0
		if reference, ok := value["git"].(string); ok {
			statement.Locator = safeLocator(reference, path.Dir(source.entry.Path), state.projectDir)
			if statement.Locator.Kind != dependencydeclaration.LocatorVCS {
				statement.Locator.Kind = dependencydeclaration.LocatorVCS
			}
			locatorFields++
		} else if _, exists := value["git"]; exists {
			locatorFields += 2
		}
		if reference, ok := value["url"].(string); ok {
			statement.Locator = safeLocator(reference, path.Dir(source.entry.Path), state.projectDir)
			if statement.Locator.Kind != dependencydeclaration.LocatorURL {
				statement.Locator = dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorURL}
			}
			locatorFields++
		} else if _, exists := value["url"]; exists {
			locatorFields += 2
		}
		if reference, ok := value["path"].(string); ok {
			statement.Locator = safeLocator(reference, path.Dir(source.entry.Path), state.projectDir)
			locatorFields++
		} else if _, exists := value["path"]; exists {
			locatorFields += 2
		}
		if locatorFields > 1 {
			state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
				dependencydeclaration.FrontierUnsupportedShape, section, ordinal, raw)
			return
		}
	default:
		state.addManifestFrontier(source, dependencydeclaration.FrontierStatement,
			dependencydeclaration.FrontierUnsupportedShape, section, ordinal, raw)
		return
	}
	state.statements = append(state.statements, statement)
}

func (state *builder) addManifestFrontier(
	source *sourceDraft,
	kind dependencydeclaration.FrontierKind,
	reason dependencydeclaration.FrontierReason,
	section string,
	ordinal int,
	value any,
) {
	state.addFrontier(source, kind, reason, section, ordinal, nil, semanticSHA256(value))
}

func mapValue(parent map[string]any, key string) (map[string]any, bool) {
	if parent == nil {
		return nil, false
	}
	value, ok := parent[key].(map[string]any)
	return value, ok
}

func sortedMapKeys(values map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func stringSlice(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		result = append(result, strings.TrimSpace(text))
	}
	return result, true
}

func validDistributionName(value string) bool {
	match := distributionNamePattern.FindString(value)
	return match != "" && len(match) == len(value)
}

func semanticSHA256(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return textSHA256(fmt.Sprintf("%T", value))
	}
	return textSHA256(string(encoded))
}
