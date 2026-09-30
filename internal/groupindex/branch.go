package groupindex

// outsideBranch reports an edge from an input's handler written outside
// the lines the input's case selects (Operation.Branch): an input a case
// of a comparison declares is handled by the code written for it, so its
// reach starts from the calls, reads and hand-overs in those lines alone.
// litestream's case "replicate" in Main.Run reaches NewReplicateCommand and
// the command's Run, never case "databases"' DatabasesCommand.Run.
func (graph *reachGraph) outsideBranch(operation Operation, from, root, edge int) bool {
	if operation.Branch == nil || from != root {
		return false
	}
	at := graph.index.StructuralEdges[edge].Location
	return at == nil || at.Line < operation.Branch.Line || at.Line > operation.Branch.EndLine
}
