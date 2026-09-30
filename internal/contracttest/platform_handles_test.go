package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// A module logger is its file's own handle on the platform: its value is
// what logging.getLogger returned and only the functions of its file read
// it, so its part draws no tile for it (levels.py's logger), nor for one
// nothing reads (storage.py's, as freqtrade's commands modules declare).
// READ_LIMIT, a literal another file reads, is no handle, nor is
// runtime_registrations' unstarted Thread, which hands over the program's
// bounded_retry. Go, Clojure and C record no call
// for a package variable, and JS/TS records a module constant's call
// without its outside symbol, so none of theirs is one (PYTHON).
func TestAModuleLoggerIsItsFilesHandleOnThePlatform(t *testing.T) {
	index := pythonLibraryIndex(t)
	handles := groupindex.PlatformHandles(index)
	var found []string
	for _, object := range index.Objects {
		if handles[object.ID] {
			found = append(found, object.Location.Path+":"+object.Name)
		}
	}
	slices.Sort(found)
	if !slices.Equal(found, []string{"src/fixture_app/levels.py:logger", "src/fixture_app/storage.py:logger"}) {
		t.Fatalf("platform handles %v, want levels.py's and storage.py's loggers", found)
	}
}
