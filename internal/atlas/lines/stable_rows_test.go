package lines

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// stableRows are rows no former cut touched: a leaf directory (nil Dirs), a
// file of two declarations with two callers of two calling declarations,
// and a symbol whose signature is under 160 runes. Their request bytes were
// taken from main before the cuts were removed (cc415346).
func stableRows() []table.Row {
	directory := atlas.Place{ID: atlas.DirectoryID("pkg/leaf"), Kind: atlas.PlaceDirectory, Path: "pkg/leaf",
		Directory: &atlas.DirectoryFacts{Files: []string{"a.go", "b.go"}, FileCount: 2}}
	caller := atlas.Place{ID: atlas.FileID("pkg/x.go"), Kind: atlas.PlaceFile, Path: "pkg/x.go", File: &atlas.FileFacts{}}
	other := atlas.Place{ID: atlas.FileID("pkg/z.go"), Kind: atlas.PlaceFile, Path: "pkg/z.go", File: &atlas.FileFacts{}}
	file := atlas.Place{ID: atlas.FileID("pkg/leaf/a.go"), Kind: atlas.PlaceFile, Path: "pkg/leaf/a.go", Parent: directory.ID,
		File: &atlas.FileFacts{Callers: []string{caller.ID, other.ID}, Decls: []atlas.Decl{
			{Name: "Open", Kind: "function", Exported: true, Signature: "func Open(path string) (*Store, error)", LineNo: 3},
			{Name: "close", Kind: "function", Signature: "func close(s *Store)", LineNo: 9}}}}
	places := map[string]atlas.Place{caller.ID: caller, other.ID: other, directory.ID: directory}
	calling := FileCallers{caller.ID: {"main", "run"}, other.ID: {"serve"}}
	symbol := atlas.Place{ID: "s1", Kind: atlas.PlaceSymbol, Path: file.Path, Parent: file.ID,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Open", Kind: "function", Signature: "func Open(path string) (*Store, error)", FanIn: 2}}}
	return []table.Row{DirectoryRow(directory), FileRow(file, directory, noLines{}, places, calling), SymbolRow(symbol, "")}
}
