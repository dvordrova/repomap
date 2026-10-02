package makefile

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The rows a reader runs make with: the default goal through the
// makefile's variables (ALL names lib.a and app), the rules but an object's
// and a special target's, and the variables the goal's commands and make's
// own compiles use, each assignment under its conditional (redis-1.3.6's
// CFLAGS when SunOS and otherwise), a continued line read whole, a
// comment and a define block left out.
func TestReadGivesTheGoalItsRulesAndTheirConditions(t *testing.T) {
	text := strings.Join([]string{
		"# build",                             // 1
		"uname_S := $(shell uname -s)",        // 2
		"ifeq ($(uname_S),SunOS)",             // 3
		"  CFLAGS?= -O2 -D__EXTENSIONS__",     // 4
		"else",                                // 5
		"  CFLAGS?= -O2 # portable",           // 6
		"endif",                               // 7
		"LINK= -lm \\",                        // 8
		"  -pthread",                          // 9
		"LIB= lib.a",                          // 10
		"APP= app",                            // 11
		"ALL= $(LIB) $(APP)",                  // 12
		"UNUSED= nothing",                     // 13
		"define banner",                       // 14
		"fake: rule",                          // 15
		"endef",                               // 16
		"all: $(ALL)",                         // 17
		"$(LIB): a.o b.o",                     // 18
		"\tar rc $@ $?",                       // 19
		"$(APP): main.o $(LIB)",               // 20
		"\t$(CC) -o $@ main.o $(LIB) $(LINK)", // 21
		"main.o: main.c",                      // 22
		".PHONY: all test",                    // 23
		"test: app ; ./app --check",           // 24
	}, "\n")
	var got []string
	for _, row := range Read(strings.Split(text, "\n")) {
		got = append(got, fmt.Sprintf("%d %s = %s", row.Line, row.Key, row.Value))
	}
	want := []string{
		"4 variable.CFLAGS when ifeq ($(uname_S),SunOS) = -O2 -D__EXTENSIONS__",
		"6 variable.CFLAGS when not ifeq ($(uname_S),SunOS) = -O2",
		"8 variable.LINK = -lm -pthread",
		"10 variable.LIB = lib.a",
		"17 default_goal = all: lib.a app",
		"18 rule.lib.a = a.o b.o — runs: ar rc $@ $?",
		"20 rule.app = main.o lib.a — runs: $(CC) -o $@ main.o $(LIB) $(LINK)",
		"24 rule.test = app — runs: ./app --check",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// .DEFAULT_GOAL names the goal; a variable naming itself stays as written.
func TestReadTakesTheDefaultGoalVariable(t *testing.T) {
	rows := Read([]string{"X = $(X) more", "first: $(X)", "second:", "\techo two", ".DEFAULT_GOAL := second"})
	want := []Row{{Key: "rule.first", Value: "$(X) more", Line: 2}, {Key: "default_goal", Value: "second: — runs: echo two", Line: 3}}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows %+v", rows)
	}
}
