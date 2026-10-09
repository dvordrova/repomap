package cproject

import (
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// makefileNames are the names make reads in a directory, in its order.
var makefileNames = []string{"GNUmakefile", "makefile", "Makefile"}

// readNested reads the makefiles below the root that no dry run entered, as
// a developer runs `make` in that directory (C.md "Nested makefiles"): Lua's
// testes/libs/makefile builds the test libraries with -I../../ and its root
// makefile never enters it. A directory counts when it holds a makefile and
// .c units whose nearest makefile is its own that no line compiled yet,
// shallow directories first. Its default goal is dry-run as the root's is,
// and its compile and link lines join description; the units still
// uncompiled, when it compiled none of them, are then built by its `all`
// rule when it has one, as the default goal's are (Lua 5.1.5's src/Makefile:
// liblua.a, lua, luac), and otherwise compiled as that makefile compiles
// their objects (`make -n -B -k -w -o <makefile> x.o …`), taking their
// compile lines only: Lua 5.1.5's etc/Makefile prints "Please choose a
// target" by default and compiles min.c with -I../src. It returns each dry
// run's record.
func readNested(ctx context.Context, env parseEnv, description *buildDescription, skip func(string) bool) ([]Build, []Observation, map[string]string) {
	makefileOf := map[string]string{}
	for _, name := range makefileNames {
		for file := range env.corpus {
			if path.Base(file) != name || corpus.ToolingPath(file) {
				continue
			}
			if dir := path.Dir(file); dir != "." && makefileOf[dir] == "" {
				makefileOf[dir] = file
			}
		}
	}
	if len(makefileOf) == 0 {
		return nil, nil, nil
	}
	if description.archives == nil {
		description.archives = map[string][]string{}
	}
	nearest := func(file string) string {
		for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
			if makefileOf[dir] != "" {
				return dir
			}
		}
		return "."
	}
	compiled := map[string]bool{}
	for _, record := range description.compiles {
		compiled[record.spec.Path] = true
	}
	entered := map[string]bool{}
	enter := func(dirs []string) {
		for _, dir := range dirs {
			entered[relativeTo(env.roots, dir)] = true
		}
	}
	enter(description.entered)
	var files []string
	for file := range env.corpus {
		if path.Ext(file) == ".c" && !skip(file) {
			files = append(files, file)
		}
	}
	slices.Sort(files)
	var dirs []string
	for dir := range makefileOf {
		dirs = append(dirs, dir)
	}
	slices.SortFunc(dirs, func(a, b string) int {
		if depth := strings.Count(a, "/") - strings.Count(b, "/"); depth != 0 {
			return depth
		}
		return strings.Compare(a, b)
	})
	var builds []Build
	var observations []Observation
	// failed are, by unit no line compiled, why its makefile's runs failed:
	// its program's build error.
	failed := map[string]string{}
	for _, dir := range dirs {
		if entered[dir] {
			continue
		}
		pending := func() []string {
			var units []string
			for _, file := range files {
				if !compiled[file] && nearest(file) == dir {
					units = append(units, file)
				}
			}
			return units
		}
		if len(pending()) == 0 {
			continue
		}
		makefile := path.Base(makefileOf[dir])
		absDir := filepath.Join(env.root, filepath.FromSlash(dir))
		join := func(output string, links bool) int {
			read := parseDryRun(env, output, absDir)
			for _, record := range read.compiles {
				record.spec.Makefile, record.spec.ObjectRule = makefileOf[dir], !links
				description.compiles = append(description.compiles, record)
				compiled[record.spec.Path] = true
			}
			enter(read.entered)
			if !links {
				return len(read.compiles)
			}
			description.links = append(description.links, read.links...)
			for archive, members := range read.archives {
				description.archives[archive] = append(description.archives[archive], members...)
			}
			for _, rule := range read.archiveRules {
				if !slices.ContainsFunc(description.archiveRules, func(known archiveRecord) bool { return known.output == rule.output }) {
					description.archiveRules = append(description.archiveRules, rule)
				}
			}
			return len(read.compiles)
		}
		// Its default goal, as the root's: what `make` there builds.
		command := []string{"make", "-n", "-B", "-w", "-o", makefile}
		units := pending()
		output, err := dryRun(ctx, absDir, command)
		if ctx.Err() != nil {
			return builds, observations, failed
		}
		build := Build{Kind: BuildMake, Path: makefileOf[dir], Command: command}
		reason := ""
		if err != nil {
			build.Err, reason = err.Error(), err.Error()
			observations = append(observations, Observation{Kind: "c_build_error", Path: makefileOf[dir], Fields: map[string]string{"error": build.Err}})
		} else {
			join(output, true)
		}
		builds = append(builds, build)
		// A default goal compiling some of its units leaves the others out
		// as this platform's build does (owner decision D3).
		if left := pending(); len(left) < len(units) {
			continue
		}
		// When it compiles none of them and the makefile has an `all` rule,
		// what `all` builds, as the default goal's lines are taken: Lua
		// 5.1.5's src/Makefile chooses a platform by default and builds
		// liblua.a, lua and luac on `all`. No other named goal is run.
		if hasRule(env, makefileOf[dir], "all") {
			all := []string{"make", "-n", "-B", "-w", "-o", makefile, "all"}
			output, err := dryRun(ctx, absDir, all)
			if ctx.Err() != nil {
				return builds, observations, failed
			}
			build := Build{Kind: BuildMake, Path: makefileOf[dir], Command: all}
			if err != nil {
				build.Err, reason = err.Error(), err.Error()
				observations = append(observations, Observation{Kind: "c_build_error", Path: makefileOf[dir], Fields: map[string]string{"error": build.Err}})
			} else {
				join(output, true)
			}
			builds = append(builds, build)
			if left := pending(); len(left) < len(units) {
				continue
			}
		}
		// Its units, when neither compiles any of them, as it compiles their
		// objects: their flags only, never a link line.
		objects := []string{"make", "-n", "-B", "-k", "-w", "-o", makefile}
		for _, unit := range units {
			objects = append(objects, strings.TrimSuffix(strings.TrimPrefix(unit, dir+"/"), ".c")+".o")
		}
		output, err = dryRun(ctx, absDir, objects)
		if ctx.Err() != nil {
			return builds, observations, failed
		}
		build = Build{Kind: BuildMake, Path: makefileOf[dir], Command: objects}
		// With -k, a goal no rule makes fails the run and the others still
		// print: what stdout lacks says which units kept clang's defaults.
		if read := join(output, false); err != nil {
			build.Err = err.Error()
			if read == 0 {
				observations = append(observations, Observation{Kind: "c_build_error", Path: makefileOf[dir], Fields: map[string]string{"error": build.Err}})
				reason = build.Err
			}
		}
		builds = append(builds, build)
		for _, unit := range pending() {
			observations = append(observations, Observation{Kind: "c_unit_unbuilt", Path: unit, Fields: map[string]string{"makefile": makefileOf[dir]}})
			if reason != "" {
				failed[unit] = reason
			}
		}
	}
	return builds, observations, failed
}
