package snapshot

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	neutralRepositoryName  = "local-repository"
	maxRepositoryNameBytes = 512
	maxManifestBytes       = 128 * 1024
)

type manifestNameReader struct {
	path  string
	parse func([]byte) string
}

var repositoryManifestReaders = []manifestNameReader{
	{path: "package.json", parse: parseJSONManifestName},
	{path: "pyproject.toml", parse: parsePythonManifestName},
	{path: "Cargo.toml", parse: parseCargoManifestName},
	{path: "composer.json", parse: parseJSONManifestName},
	{path: "setup.cfg", parse: parseSetupConfigName},
}

func repositoryIdentity(repoPath string, trackedFiles []string, goHints GoHints) string {
	if identity := cleanRepositoryIdentity(goHints.ModuleName); identity != "" {
		return identity
	}
	if identity := repositoryRemoteIdentity(repoPath); identity != "" {
		return identity
	}
	if identity := repositoryManifestIdentity(repoPath, trackedFiles); identity != "" {
		return identity
	}
	return neutralRepositoryName
}

func repositoryDisplayName(repoPath string) string {
	name := filepath.Base(filepath.Clean(repoPath))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return neutralRepositoryName
	}
	return name
}

// RepositoryOriginIdentity returns the credential-free host/path identity of
// the repository-local origin remote. It never guesses among other remotes.
func RepositoryOriginIdentity(repoPath string) string {
	origin, _ := localGitConfigValue(repoPath, "remote.origin.url")
	return normalizeRemoteIdentity(origin)
}

func repositoryRemoteIdentity(repoPath string) string {
	origin, _ := localGitConfigValue(repoPath, "remote.origin.url")
	if identity := normalizeRemoteIdentity(origin); identity != "" {
		return identity
	}

	output, err := runLocalGitConfig(repoPath, "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		return ""
	}
	type candidate struct {
		key      string
		identity string
	}
	var candidates []candidate
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		identity := normalizeRemoteIdentity(strings.TrimSpace(value))
		if identity != "" {
			candidates = append(candidates, candidate{key: key, identity: identity})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].key == candidates[j].key {
			return candidates[i].identity < candidates[j].identity
		}
		return candidates[i].key < candidates[j].key
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].identity
}

func localGitConfigValue(repoPath, key string) (string, error) {
	return runLocalGitConfig(repoPath, "--get", key)
}

func runLocalGitConfig(repoPath string, args ...string) (string, error) {
	commandArgs := []string{"-C", repoPath, "config", "--local"}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Env = repositoryGitEnvironment(os.Environ())
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func repositoryGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM":
			continue
		}
		if strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, item)
	}
	return result
}

func normalizeRemoteIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4096 || !utf8.ValidString(raw) {
		return ""
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || !supportedRemoteScheme(parsed.Scheme) || parsed.Hostname() == "" {
			return ""
		}
		remotePath, err := url.PathUnescape(parsed.EscapedPath())
		if err != nil {
			return ""
		}
		return joinRemoteIdentity(parsed.Hostname(), remotePath)
	}

	separator := strings.IndexByte(raw, ':')
	if separator <= 0 || separator == len(raw)-1 {
		return ""
	}
	host := raw[:separator]
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	if !strings.Contains(host, ".") && !strings.Contains(raw[:separator], "@") {
		return ""
	}
	return joinRemoteIdentity(host, raw[separator+1:])
}

func supportedRemoteScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "git", "git+ssh", "http", "https", "ssh":
		return true
	default:
		return false
	}
}

func joinRemoteIdentity(host, remotePath string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	remotePath = strings.Trim(strings.TrimSpace(remotePath), "/")
	if len(remotePath) >= 4 && strings.EqualFold(remotePath[len(remotePath)-4:], ".git") {
		remotePath = remotePath[:len(remotePath)-4]
	}
	if host == "" || remotePath == "" || strings.ContainsAny(host+remotePath, "\\?#") {
		return ""
	}
	cleanPath := path.Clean(remotePath)
	if cleanPath != remotePath || cleanPath == "." || strings.HasPrefix(cleanPath, "../") {
		return ""
	}
	return cleanRepositoryIdentity(host + "/" + cleanPath)
}

func repositoryManifestIdentity(repoPath string, trackedFiles []string) string {
	tracked := make(map[string]struct{}, len(trackedFiles))
	for _, filePath := range trackedFiles {
		tracked[filePath] = struct{}{}
	}
	reader, err := reporead.New(repoPath)
	if err != nil {
		return ""
	}
	defer reader.Close()

	for _, manifest := range repositoryManifestReaders {
		if _, ok := tracked[manifest.path]; !ok {
			continue
		}
		content, err := reader.ReadFile(manifest.path, maxManifestBytes)
		if err != nil || content.Truncated {
			continue
		}
		if identity := cleanRepositoryIdentity(manifest.parse(content.Bytes)); identity != "" {
			return identity
		}
	}
	return ""
}

func parseJSONManifestName(data []byte) string {
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	return manifest.Name
}

func parsePythonManifestName(data []byte) string {
	if name := parseSectionValue(data, "project", "name"); name != "" {
		return name
	}
	return parseSectionValue(data, "tool.poetry", "name")
}

func parseCargoManifestName(data []byte) string {
	return parseSectionValue(data, "package", "name")
}

func parseSetupConfigName(data []byte) string {
	return parseSectionValue(data, "metadata", "name")
}

func parseSectionValue(data []byte, wantedSection, wantedKey string) string {
	section := ""
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section != wantedSection || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != wantedKey {
			continue
		}
		return parseManifestValue(value)
	}
	return ""
}

func parseManifestValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if value[0] == '\'' {
		end := strings.IndexByte(value[1:], '\'')
		if end < 0 {
			return ""
		}
		return value[1 : end+1]
	}
	if value[0] != '"' {
		if comment := strings.IndexAny(value, "#;"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		return value
	}
	for end := 1; end < len(value); end++ {
		if value[end] != '"' || value[end-1] == '\\' {
			continue
		}
		unquoted, err := strconv.Unquote(value[:end+1])
		if err != nil {
			return ""
		}
		return unquoted
	}
	return ""
}

func cleanRepositoryIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || len(value) > maxRepositoryNameBytes ||
		!utf8.ValidString(value) || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ""
		}
	}
	return value
}
