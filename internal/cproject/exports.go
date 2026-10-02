package cproject

import p "github.com/dvordrova/repomap/internal/programindex"

// exports are a library's API (C.md "A library's exports"): of the functions
// with external linkage its units define, the ones a header declares that a
// program the build links with the library includes from its own units
// (Lua's lua.c includes lua.h, lauxlib.h and lualib.h, so liblua.a exports
// lua_*, luaL_* and luaopen_*, while luaD_* and the other functions its
// internal headers declare stay its own). When no program the build links
// takes the library (a shared object a program loads, a directory's units
// no link line links, an archive nothing links) every function with
// external linkage is an export: any program may call it by name.
func (b *builder) exports() ([]p.TargetExportInput, p.ExportBasis) {
	api := make(map[string]bool, len(b.parsed.APIHeaders))
	for _, header := range b.parsed.APIHeaders {
		api[header] = true
	}
	declared := map[string]bool{}
	for _, unit := range b.parsed.Units {
		for _, node := range unit.Decls {
			if node.Kind == "FunctionDecl" && api[node.Loc.Site().File] {
				declared[node.Name] = true
			}
		}
	}
	basis := p.ExportsLinkage
	if len(api) > 0 {
		basis = p.ExportsConsumerHeaders
	}
	var exports []p.TargetExportInput
	for _, fn := range b.functions {
		if fn.scope.internal[fn.node.Name] || fn.location == nil || basis == p.ExportsConsumerHeaders && !declared[fn.node.Name] {
			continue
		}
		exports = append(exports, p.TargetExportInput{ObjectRef: fn.ref, Location: fn.location})
	}
	if len(exports) == 0 {
		return nil, ""
	}
	return exports, basis
}
