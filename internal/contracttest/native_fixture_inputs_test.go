package contracttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

var nativeInputMemo = struct {
	sync.Mutex
	c                                              map[string]*cproject.Parsed
	cProjects                                      map[string]*cproject.Project
	py                                             map[string]programindex.Input
	clj                                            map[string]*clojureproject.Result
	cRuns, cHits, pyRuns, pyHits, cljRuns, cljHits int
}{c: map[string]*cproject.Parsed{}, cProjects: map[string]*cproject.Project{}, py: map[string]programindex.Input{}, clj: map[string]*clojureproject.Result{}}

func fixtureDecisionKey(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// A graph snapshot preserves AST pointer aliases and fields omitted from JSON
// (Unit.Decls and Main.Node). None of these typed native values has private
// runtime state; encountering it is an error rather than a partial snapshot.
func ownNativeFixture[T any](t *testing.T, value T) T {
	t.Helper()
	type pointerKey struct {
		typ     reflect.Type
		address uintptr
	}
	seen := map[pointerKey]reflect.Value{}
	var clone func(reflect.Value) reflect.Value
	clone = func(v reflect.Value) reflect.Value {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			key := pointerKey{v.Type(), v.Pointer()}
			if prior, ok := seen[key]; ok {
				return prior
			}
			out := reflect.New(v.Type().Elem())
			seen[key] = out
			out.Elem().Set(clone(v.Elem()))
			return out
		case reflect.Struct:
			out := reflect.New(v.Type()).Elem()
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).PkgPath != "" {
					t.Fatalf("native fixture snapshot has private state: %s.%s", v.Type(), v.Type().Field(i).Name)
				}
				out.Field(i).Set(clone(v.Field(i)))
			}
			return out
		case reflect.Slice:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			for i := 0; i < v.Len(); i++ {
				out.Index(i).Set(clone(v.Index(i)))
			}
			return out
		case reflect.Map:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			out := reflect.MakeMapWithSize(v.Type(), v.Len())
			it := v.MapRange()
			for it.Next() {
				out.SetMapIndex(clone(it.Key()), clone(it.Value()))
			}
			return out
		case reflect.Interface:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			out := reflect.New(v.Type()).Elem()
			out.Set(clone(v.Elem()))
			return out
		case reflect.Array:
			out := reflect.New(v.Type()).Elem()
			for i := 0; i < v.Len(); i++ {
				out.Index(i).Set(clone(v.Index(i)))
			}
			return out
		default:
			return v
		}
	}
	return clone(reflect.ValueOf(value)).Interface().(T)
}

func sharedCFixture(t *testing.T) cFixture {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "c")
	key := nativeFixtureKey(t, repository, root, "c complete programs", false)
	nativeInputMemo.Lock()
	defer nativeInputMemo.Unlock()
	project, known := nativeInputMemo.cProjects[key]
	var err error
	if !known {
		project, err = cproject.Discover(t.Context(), root, repository)
		if err != nil {
			t.Fatal(err)
		}
		if project != nil && project.Toolchain.Err == "" {
			nativeInputMemo.cProjects[key] = ownNativeFixture(t, project)
		}
	}
	if project == nil || project.Toolchain.Err != "" {
		t.Fatalf("C fixture needs clang: %+v", project)
	}
	project = ownNativeFixture(t, project)
	// Root is an unsealed physical locator. The complete immutable native
	// programs, commands and observations stay original; only the current
	// consumer's checkout locator is rebound. No memo command is executed.
	project.Root = root
	fixture := cFixture{root: root, repository: repository, project: project, parsed: map[string]*cproject.Parsed{}}
	store := cproject.NewStore()
	// Clone the whole map at once to preserve units shared by programs.
	originals := map[string]*cproject.Parsed{}
	for _, program := range project.Programs {
		programKey := key + fixtureDecisionKey(t, program)
		parsed, ok := nativeInputMemo.c[programKey]
		if !ok {
			parsed, err = cproject.Parse(t.Context(), root, repository, program, store)
			if err != nil {
				t.Fatal(err)
			}
			nativeInputMemo.cRuns++
			nativeInputMemo.c[programKey] = parsed
		} else {
			nativeInputMemo.cHits++
		}
		originals[program.Selector] = parsed
	}
	fixture.parsed = ownNativeFixture(t, originals)
	return fixture
}

func sharedPythonFixtureInput(t *testing.T, repository *corpus.Corpus, target pythontarget.Target, seeds ...pythontarget.Target) (programindex.Input, error) {
	t.Helper()
	key := nativeFixtureKey(t, repository, "", "python", false) + fixtureDecisionKey(t, []any{target, seeds})
	nativeInputMemo.Lock()
	defer nativeInputMemo.Unlock()
	if value, ok := nativeInputMemo.py[key]; ok {
		nativeInputMemo.pyHits++
		return ownNativeFixture(t, value), nil
	}
	value, err := pythonprogramindex.BuildInput(t.Context(), repository, target, seeds...)
	if err != nil {
		return programindex.Input{}, err
	}
	nativeInputMemo.pyRuns++
	nativeInputMemo.py[key] = ownNativeFixture(t, value)
	return value, nil
}

func sharedClojureFixture(t *testing.T, root string, repository *corpus.Corpus, target clojureproject.Target) (*clojureproject.Result, error) {
	t.Helper()
	key := nativeFixtureKey(t, repository, root, "clojure", false) + fixtureDecisionKey(t, target)
	nativeInputMemo.Lock()
	defer nativeInputMemo.Unlock()
	if value, ok := nativeInputMemo.clj[key]; ok {
		nativeInputMemo.cljHits++
		return ownNativeFixture(t, value), nil
	}
	value, err := clojureproject.Build(t.Context(), root, repository, target)
	if err != nil {
		return nil, err
	}
	nativeInputMemo.cljRuns++
	nativeInputMemo.clj[key] = ownNativeFixture(t, value)
	return value, nil
}

func TestSharedNativeInputFixturesKeepOriginalASTAndFreshSource(t *testing.T) {
	t.Run("c", func(t *testing.T) {
		first := sharedCFixture(t)
		main := first.parsed["c:kvd"].Main
		if main == nil || main.Node == nil {
			t.Fatal("real clang AST omitted main")
		}
		original := main.Node.Name
		originalProgramName := first.project.Programs[0].Name
		main.Node.Name = "consumer mutation"
		first.project.Programs[0].Name = "consumer mutation"
		oldRoot := first.root
		if err := os.RemoveAll(oldRoot); err != nil {
			t.Fatal(err)
		}
		runs := nativeInputMemo.cRuns
		second := sharedCFixture(t)
		if nativeInputMemo.cRuns != runs || second.project.Root != second.root || second.parsed["c:kvd"].Main.Node.Name != original || second.project.Programs[0].Name != originalProgramName {
			t.Fatal("C snapshot lost its native AST or fresh checkout binding")
		}
		if second.unit(t, "c:kvd", "strbuf.c") != second.unit(t, "c:kvcli", "strbuf.c") {
			t.Fatal("C snapshot erased identical shared unit identity")
		}
		index := buildCIndex(t, second, "c:kvd")
		ownerFacts(t, second.repository, index).declared(t, "kvd.c")
		path := filepath.Join(second.root, "strbuf.c")
		value, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(value, []byte("\nint fixtureMemoChangedC(void) { return 7; }\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
		// Same helper receives the changed original corpus, rather than a
		// new stock materialization whose bytes would conceal the change.
		project, err := cproject.Discover(t.Context(), second.root, second.repository)
		if err != nil {
			t.Fatal(err)
		}
		changedFixture := cFixture{project: project}
		program := changedFixture.program(t, "c:kvd")
		changedKey := nativeFixtureKey(t, second.repository, second.root, "c complete programs", false) + fixtureDecisionKey(t, program)
		if _, known := nativeInputMemo.c[changedKey]; known {
			t.Fatal("changed C source reused old native input key")
		}
		parsed, err := cproject.Parse(t.Context(), second.root, second.repository, program, cproject.NewStore())
		if err != nil {
			t.Fatal(err)
		}
		result, err := cproject.Index(second.repository, parsed)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, object := range result.Input.Objects {
			if object.Name == "fixtureMemoChangedC" {
				found = true
			}
		}
		if !found {
			t.Fatal("real C source change did not reach native input")
		}
	})
	for _, language := range []string{"python", "clojure"} {
		t.Run(language, func(t *testing.T) {
			root, repository := materializeFixtureRepository(t, language)
			var read func(string, *corpus.Corpus) (programindex.Input, error)
			if language == "python" {
				catalog, err := pythontarget.Discover(t.Context(), repository)
				if err != nil {
					t.Fatal(err)
				}
				target := pythonFixtureTarget(t, catalog)
				read = func(_ string, r *corpus.Corpus) (programindex.Input, error) {
					return sharedPythonFixtureInput(t, r, target)
				}
			} else {
				targets, err := clojureproject.Scout(repository, "clojure")
				if err != nil || len(targets) != 1 {
					t.Fatalf("native target: %v %v", targets, err)
				}
				read = func(root string, r *corpus.Corpus) (programindex.Input, error) {
					result, err := sharedClojureFixture(t, root, r, targets[0])
					if err != nil {
						return programindex.Input{}, err
					}
					return result.Input, nil
				}
			}
			first, err := read(root, repository)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Objects) == 0 {
				t.Fatal("real native input empty")
			}
			name := first.Objects[0].Name
			first.Objects[0].Name = "consumer mutation"
			oldRoot := root
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			root, repository = materializeFixtureRepository(t, language)
			runs := nativeInputMemo.pyRuns + nativeInputMemo.cljRuns
			second, err := read(root, repository)
			if err != nil {
				t.Fatal(err)
			}
			if second.Objects[0].Name != name || nativeInputMemo.pyRuns+nativeInputMemo.cljRuns != runs {
				t.Fatal("native input shares mutable state or borrowed deleted checkout")
			}
			index, err := programindex.New(second)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), oldRoot) {
				t.Fatal("native input borrowed deleted checkout")
			}
			ownerFacts(t, repository, index)
		})
	}
}
