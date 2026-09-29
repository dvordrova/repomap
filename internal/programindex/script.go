package programindex

import "sort"

// ScriptFile is the file a script program is: an executable of one source
// file whose build gives its executable no name (Executables), such as a
// Python `__main__` guard or shebang script run as
// `python build_helpers/create_command_partials.py`. The program is that
// file and what its code imports: it is titled by the path, not by a dotted
// module name or its directory, and holds its file, never its directory's
// other files. A program its build names (a console script, a Go main
// package's directory, a Makefile target, a package.json bin command), or
// one of several source files, is no script. Empty for any other program.
func (target Target) ScriptFile() string {
	if len(target.Executables) > 0 || target.Kind != "executable" || len(target.Sources) != 1 {
		return ""
	}
	return target.Sources[0].Path
}

// ImportedFiles are the files the code of from imports, followed through
// each imported file's own imports: the repository files an `imports`
// relation written in a reached file names (a Python import inside a
// function included), from itself. skip leaves a file out and stops the
// walk there (tests). A walk reads relations only; an index whose adapter
// records no imports (Go names packages, not files) gives from alone.
// Sorted, each once.
func (index Index) ImportedFiles(from string, skip map[string]bool) []string {
	located := make(map[string]string, len(index.Objects))
	for _, object := range index.Objects {
		if object.Location != nil {
			located[object.ID] = object.Location.Path
		}
	}
	imports := make(map[string][]Relation)
	for _, relation := range index.Relations {
		if relation.Kind != RelationImports {
			continue
		}
		written := located[relation.FromID]
		if relation.Location != nil {
			written = relation.Location.Path
		}
		if written != "" {
			imports[written] = append(imports[written], relation)
		}
	}
	files := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		for _, relation := range imports[file] {
			for _, to := range relation.ToIDs {
				if path := located[to]; path != "" && !files[path] && !skip[path] {
					files[path] = true
					queue = append(queue, path)
				}
			}
		}
	}
	result := make([]string, 0, len(files))
	for file := range files {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}
