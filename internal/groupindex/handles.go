package groupindex

import "github.com/dvordrova/repomap/internal/programindex"

// PlatformHandles are the module variables a file keeps only as its own
// handle on the platform: the value is what a standard-library call
// returned (a pattern's ResultID on a call into platform symbols alone),
// and only the functions and methods of its own file read it. freqtrade's module logger,
// logger = logging.getLogger(__name__), one in each of 196 files: three of
// them had stood as three "logger" tiles in Trading bot core. The file's
// functions call through it as they call the platform itself, and those
// calls are no rows either. A variable another file or the module's own
// body reads (a script's parser, its parsed args) is no handle. Only the
// Python adapter records the platform call a module variable's value comes
// from: Go, Clojure and C record no call for a package or namespace
// variable, and JS/TS records a module constant's call without its outside
// symbol (the fixture's new Command()), so none of theirs is one.
func PlatformHandles(program programindex.Index) map[string]bool {
	byID := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		byID[object.ID] = object
	}
	platform := func(ids []string) bool {
		for _, id := range ids {
			if object, ok := byID[id]; !ok || object.External == nil || object.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
				return false
			}
		}
		return len(ids) > 0
	}
	// A module's or a package's own variable: a local is its function's.
	moduleLevel := func(object programindex.Object) bool {
		for _, id := range []string{object.OwnerID, object.ContainerID} {
			if holder, ok := byID[id]; ok && (holder.Kind == programindex.ObjectModule || holder.Kind == programindex.ObjectPackage) {
				return true
			}
		}
		return false
	}
	made := make(map[string]bool)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || !platform(relation.ToIDs) {
			continue
		}
		for _, pattern := range relation.Patterns {
			if object, ok := byID[pattern.ResultID]; ok && object.Kind == programindex.ObjectVariable && object.Location != nil && moduleLevel(object) {
				made[object.ID] = true
			}
		}
	}
	// A handle is read: a variable nothing reads (a thread made and never
	// started) is no handle anything calls through.
	handles := make(map[string]bool)
	outside := make(map[string]bool)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationReads {
			continue
		}
		reader, known := byID[relation.FromID]
		for _, id := range relation.ToIDs {
			if !made[id] {
				continue
			}
			if !known || !reader.Kind.Callable() || reader.Location == nil || reader.Location.Path != byID[id].Location.Path {
				outside[id] = true
			}
			handles[id] = true
		}
	}
	for id := range outside {
		delete(handles, id)
	}
	return handles
}
