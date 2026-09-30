package places

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// packageMember is a module, or a function, type or variable a module
// holds, of one target: its file and the target.
type packageMember struct {
	file, target string
}

// packageUse is a call or an import from a target's code into an outside
// package symbol, by the symbol's whole dotted name.
type packageUse struct {
	target, from, name, kind string
	witness                  atlas.Witness
}

// collectPackageMembers records what each target declares by the name
// another target's import writes (a module, and a module's function, type
// or variable: freqtrade_client.ft_client.main), and each call or import of
// an outside package symbol by that name. A target indexes only its own
// project, so another project's package it imports is an outside symbol
// there: freqtrade's scripts/rest_client.py imports and calls
// freqtrade_client.ft_client.main, which freqtrade-client's own index
// declares in ft_client/freqtrade_client/ft_client.py.
func (b *builder) collectPackageMembers(target TargetInput) {
	index := target.Index
	targetID := index.Target.ID
	if b.packageMembers == nil {
		b.packageMembers = make(map[string]map[packageMember]bool)
	}
	add := func(name, file string) {
		if b.packageMembers[name] == nil {
			b.packageMembers[name] = make(map[packageMember]bool)
		}
		b.packageMembers[name][packageMember{file: file, target: targetID}] = true
	}
	for _, object := range index.Objects {
		file, located := b.fileOf[object.ID]
		if !located || object.Name == "" {
			continue
		}
		switch object.Kind {
		case programindex.ObjectModule, programindex.ObjectPackage:
			add(object.Name, file)
		case programindex.ObjectFunction, programindex.ObjectType, programindex.ObjectVariable:
			if module := b.byID[object.ContainerID]; module.Name != "" && (module.Kind == programindex.ObjectModule || module.Kind == programindex.ObjectPackage) {
				add(module.Name+"."+object.Name, file)
			}
		}
	}
	for _, relation := range index.Relations {
		kind := edgeKind(relation.Kind)
		if relation.Kind == programindex.RelationInvokesExternal {
			kind = "calls"
		}
		if kind != "calls" && kind != "imports" {
			continue
		}
		from, ok := b.fileOf[relation.FromID]
		if !ok {
			continue
		}
		for _, toID := range relation.ToIDs {
			symbol := b.byID[toID]
			if symbol.Kind != programindex.ObjectExternalSymbol || symbol.External == nil || symbol.External.AuthorityKind != programindex.ExternalAuthorityPackage {
				continue
			}
			b.packageUses = append(b.packageUses, packageUse{target: targetID, from: from, name: symbol.Name, kind: kind, witness: atlas.Witness{
				Caller: displayName(b.byID[relation.FromID], b.byID), Callee: symbol.Name, Path: from, LineNo: relationLine(relation),
			}})
		}
	}
}

// joinPackageUses makes a call or an import into another target's package
// the edge between the two files: the import names the module by its
// dotted path, and the package it begins with is that target's, so Python
// resolves it there when both are installed. The name must be declared by
// other targets alone, in one file; a name the importing target declares
// itself, or two files declare, joins nothing. An import a package
// re-exports under a shorter name (freqtrade_client.FtRestClient) names no
// module member and stays outside. The edge's files are then each claimed
// by their own target's root (claimByRoot), so the call is the seam
// between the two targets, a joint like any other call between them.
func (b *builder) joinPackageUses() {
	for _, use := range b.packageUses {
		files := make(map[string]bool)
		own := false
		for member := range b.packageMembers[use.name] {
			own = own || member.target == use.target
			files[member.file] = true
		}
		if own || len(files) != 1 {
			continue
		}
		for to := range files {
			if to == use.from {
				continue
			}
			b.addEdge(atlas.FileID(use.from), atlas.FileID(to), use.kind, use.witness)
			b.file(use.from).callees[to] = struct{}{}
			b.file(to).callers[use.from] = struct{}{}
		}
	}
	b.packageMembers, b.packageUses = nil, nil
}
