package corpus

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/gitfiles"
)

// inventory collects source and project documentation from disk, including
// generated sources ignored by Git. It does not admit arbitrary local data or
// credential files. The existing ForbiddenPath policy still applies first.
func inventory(ctx context.Context, root string, exclusions []string) (gitfiles.Listing, error) {
	if err := ctx.Err(); err != nil {
		return gitfiles.Listing{}, err
	}
	if strings.TrimSpace(root) == "" {
		return gitfiles.Listing{}, fmt.Errorf("directory is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return gitfiles.Listing{}, err
	}
	if !info.IsDir() {
		return gitfiles.Listing{}, fmt.Errorf("%q is not a directory", root)
	}
	excluded, err := analysisExclusions(root, exclusions)
	if err != nil {
		return gitfiles.Listing{}, err
	}
	listing := gitfiles.Listing{}
	err = filepath.WalkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if fullPath == root {
			return nil
		}
		relative, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if excludedAnalysisPath(relative, excluded) || ForbiddenPath(relative) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".hg", ".svn", "node_modules", "__pycache__", ".cache", ".mypy_cache", ".pytest_cache", ".ruff_cache":
				return fs.SkipDir
			}
			if marker, err := os.Stat(filepath.Join(fullPath, "pyvenv.cfg")); err == nil && marker.Mode().IsRegular() {
				return fs.SkipDir
			}
			return nil
		}
		// Keep path-only inventory even when this file's content is outside
		// the analysis input types. Absence checks must not confuse unread
		// configuration with a file missing from the working directory.
		listing.Paths = append(listing.Paths, relative)
		if !projectInput(relative) {
			if !entry.Type().IsRegular() || filepath.Ext(relative) != "" {
				return nil
			}
			file, err := os.Open(fullPath)
			if err != nil {
				return err
			}
			var magic [2]byte
			_, readErr := io.ReadFull(file, magic[:])
			closeErr := file.Close()
			if closeErr != nil {
				return closeErr
			}
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				return readErr
			}
			if magic != [2]byte{'#', '!'} {
				return nil
			}
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		listing.RegularPaths = append(listing.RegularPaths, relative)
		if info.Mode().Perm()&0o111 != 0 {
			listing.ExecutablePaths = append(listing.ExecutablePaths, relative)
		}
		return nil
	})
	return listing, err
}

// The working-directory inventory covers supported language sources, build
// manifests and documentation. Other local data is outside this input contract.
func projectInput(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if base == "sqlc.yaml" || base == "sqlc.yml" || base == "sqlc.json" || base == ".repomap.json" {
		return true
	}
	switch base {
	case "go.mod", "go.sum", "go.work", "go.work.sum", "package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "pnpm-workspace.yaml", "bun.lock", "bun.lockb", "pyproject.toml", "poetry.lock", "uv.lock", "pipfile", "pipfile.lock", "setup.cfg", "requirements.txt", "makefile", "dockerfile", "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml", ".gitignore", ".repomapignore", "license":
		return true
	}
	if base == "readme" || strings.HasPrefix(base, "readme.") || base == "agents.md" || strings.HasPrefix(base, "tsconfig") && strings.HasSuffix(base, ".json") || strings.HasPrefix(base, "jsconfig") && strings.HasSuffix(base, ".json") {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go", ".py", ".pyi", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".c", ".cc", ".cpp", ".h", ".hpp", ".s", ".proto", ".sh", ".bash", ".sql", ".graphql", ".gql", ".vue", ".svelte", ".html", ".css", ".scss", ".md", ".rst", ".adoc":
		return true
	}
	return false
}

// One literal root-relative file/directory per line, optional trailing slash,
// blank lines and # comments. This is deliberately independent of .gitignore.
func analysisExclusions(root string, extra []string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".repomapignore"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read .repomapignore: %w", err)
	}
	lines := append(strings.Split(string(data), "\n"), extra...)
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		if err := validatePath(line); err != nil {
			return nil, fmt.Errorf("analysis exclusion %q: %w", line, err)
		}
		result = append(result, line)
	}
	return result, nil
}

func excludedAnalysisPath(path string, excluded []string) bool {
	for _, prefix := range excluded {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
