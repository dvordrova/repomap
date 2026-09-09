package pythontarget

import (
	"path"
	"sort"
	"strings"
)

// PackageDeclaration records written distribution membership, separately from
// the complete importable module inventory. Dynamic setup calls supply no row.
type PackageDeclaration struct {
	Path       string   `json:"path"`
	Line       int      `json:"line"`
	Kind       string   `json:"kind"`
	Where      []string `json:"where,omitempty"`
	Packages   []string `json:"packages,omitempty"`
	Include    []string `json:"include,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	Namespaces bool     `json:"namespaces,omitempty"`
}

func cloneDeclarations(rows []PackageDeclaration) []PackageDeclaration {
	if rows == nil {
		return nil
	}
	out := make([]PackageDeclaration, len(rows))
	for i, row := range rows {
		out[i] = row
		out[i].Where = cloneStrings(row.Where)
		out[i].Packages = cloneStrings(row.Packages)
		out[i].Include = cloneStrings(row.Include)
		out[i].Exclude = cloneStrings(row.Exclude)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// DeclaresModule proves distribution membership from a static declaration.
// It does not infer an architectural role or discard any observed source.
func (target Target) DeclaresModule(module Module) bool {
	if !module.Importable {
		return false
	}
	packageName := module.Name
	if !module.Package {
		last := strings.LastIndexByte(packageName, '.')
		if last < 0 {
			packageName = ""
		} else {
			packageName = packageName[:last]
		}
	}
	for _, row := range target.DeclaredPackages {
		switch row.Kind {
		case "modules":
			if !module.Package && containsString(row.Packages, module.Name) {
				return true
			}
		case "packages":
			if containsString(row.Packages, packageName) {
				return true
			}
		case "project_name":
			for _, name := range row.Packages {
				if packageName != name && !strings.HasPrefix(packageName, name+".") {
					continue
				}
				for _, candidate := range target.Modules {
					if candidate.Package && candidate.Name == name {
						return true
					}
				}
			}
		case "find":
			if packageName == "" {
				continue
			}
			underRoot := false
			for _, root := range row.Where {
				root = path.Join(target.ProjectDir, root)
				if root == "." || strings.HasPrefix(module.Path, root+"/") {
					underRoot = true
				}
			}
			if !underRoot || !packagePatternMatch(row.Include, packageName) || packagePatternMatch(row.Exclude, packageName) {
				continue
			}
			if !row.Namespaces {
				regular := true
				parts := strings.Split(packageName, ".")
				for n := 1; n <= len(parts); n++ {
					found := false
					for _, candidate := range target.Modules {
						if candidate.Package && candidate.Name == strings.Join(parts[:n], ".") {
							found = true
							break
						}
					}
					regular = regular && found
				}
				if !regular {
					continue
				}
			}
			return true
		}
	}
	return false
}

func packagePatternMatch(patterns []string, name string) bool {
	for _, pattern := range patterns {
		// Python fnmatch uses ! for a negated class; Go's matcher uses ^.
		matched, err := path.Match(strings.ReplaceAll(pattern, "[!", "[^"), name)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// CanSeed preserves the owner's complete module view and library API. Only
// guards inside its declared distribution are structurally eligible; whether
// they are demonstrations or independent services is a model decision.
func CanSeed(owner, seed Target) bool {
	if owner.Kind != KindLibrary || seed.Kind != KindExecutable || owner.ProjectDir != seed.ProjectDir || len(seed.Roots) == 0 {
		return false
	}
	for _, basis := range seed.Basis {
		if basis.Kind != BasisNameMainGuard {
			return false
		}
	}
	for _, root := range seed.Roots {
		if root.Kind != RootMainGuard {
			return false
		}
		found := false
		for _, module := range owner.Modules {
			if module.Path == root.Path && module.Name == root.Module && owner.DeclaresModule(module) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
