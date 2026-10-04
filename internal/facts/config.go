package facts

import (
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// configReads are the outside functions that read one setting of the
// process environment under a literal key: Python's os.getenv and
// os.environ.get, Go's os and syscall Getenv and LookupEnv, C's getenv and the
// JVM's System/getenv, and pydantic's Field(env=...) on a field of a
// BaseSettings class, which reads the variable when the settings are built
// (on any other model env is metadata). A JavaScript program reads
// process.env.KEY as a property, which is no call: it has none.
var configReads = []configRead{
	{function: outsideFunction{pkg: "os", name: "getenv"}},
	{function: outsideFunction{pkg: "os", receiver: "environ", name: "get"}},
	{function: outsideFunction{pkg: "pydantic", name: "Field", anyReceiver: true}, keywords: []string{"env"},
		classes: []outsideFunction{{pkg: "pydantic", name: "BaseSettings", anyReceiver: true}}},
	{function: outsideFunction{pkg: "os", name: "Getenv"}},
	{function: outsideFunction{pkg: "os", name: "LookupEnv"}},
	{function: outsideFunction{pkg: "syscall", name: "Getenv"}},
	{function: outsideFunction{pkg: "syscall", name: "LookupEnv"}},
	{function: outsideFunction{pkg: "stdlib.h", name: "getenv"}},
	{function: outsideFunction{pkg: "java.lang.System", name: "getenv"}},
}

// configRead is one reading function and where its call writes the key: the
// first positional argument, or, for a declaration that only names the
// variable it reads (Field(default=..., env="APP_PORT")), one of keywords.
// classes, when set, are the outside classes the class whose body makes the
// call must derive from.
type configRead struct {
	function outsideFunction
	keywords []string
	classes  []outsideFunction
}

func (b *builder) addConfigReads(target *targetContext) {
	functions := make([]outsideFunction, 0, len(configReads))
	for _, read := range configReads {
		functions = append(functions, read.function)
	}
	for _, relation := range target.input.Index.Relations {
		if target.unreachable(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			function, _, resolution, ok := target.calledFunction(relation, pattern, functions)
			if !ok {
				continue
			}
			read := configReadOf(function)
			if len(read.classes) > 0 && !target.derivesFrom(relation.FromID, read.classes) {
				continue
			}
			anchor := target.patternAnchor(relation, pattern)
			if anchor == nil {
				continue
			}
			symbol, _ := target.enclosingSymbol(relation.FromID)
			if key, value, ok := configKey(read, pattern); ok {
				b.addConfigRead(target, *anchor, key, value, symbol, resolution, nil)
				continue
			}
			// A key the call is handed rather than written there is the
			// one its callers name: casdoor's GetConfigString(key) reads
			// os.LookupEnv(key) for each key a caller gives it. Each such
			// key is read at this call, the walk's literals its evidence;
			// a key no walk reaches is no fact.
			argument, found := configKeyArgument(read, pattern)
			if !found {
				continue
			}
			for _, literal := range target.values().argument(argument) {
				if literal.text == "" {
					continue
				}
				walked := resolution
				if literal.possible {
					walked = ResolutionPossible
				}
				b.addConfigRead(target, *anchor, literal.text, defaultLiteral(pattern), symbol, walked, literal.evidence)
			}
		}
	}
}

func configReadOf(function outsideFunction) configRead {
	for _, read := range configReads {
		if read.function == function {
			return read
		}
	}
	return configRead{function: function}
}

func (b *builder) addConfigRead(target *targetContext, anchor Anchor, key, value, symbol string, resolution Resolution, evidence []Anchor) {
	if !b.once(strings.Join([]string{string(KindConfigRead), anchor.Path, itoa(anchor.Line), key}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:       KindConfigRead,
		TargetID:   target.target.ID,
		Anchor:     &anchor,
		Key:        key,
		Value:      value,
		Symbol:     symbol,
		Resolution: resolution,
		Evidence:   evidence,
	}, key)
}

// configKey reads the literal key a reading call writes, with the literal
// default it gives beside it.
func configKey(read configRead, pattern programindex.RelationPattern) (key, value string, ok bool) {
	argument, found := configKeyArgument(read, pattern)
	if !found {
		return "", "", false
	}
	key, _, ok = literalValue(argument)
	if !ok || key == "" {
		return "", "", false
	}
	return key, defaultLiteral(pattern), true
}

func defaultLiteral(pattern programindex.RelationPattern) string {
	if argument, ok := keywordArgument(pattern, "default"); ok {
		if value, _, literal := literalValue(argument); literal {
			return value
		}
	}
	if argument, ok := positionalArgument(pattern, 2); ok {
		if value, _, literal := literalValue(argument); literal {
			return value
		}
	}
	return ""
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

// configKeyArgument is the argument a reading call writes its key in.
func configKeyArgument(read configRead, pattern programindex.RelationPattern) (programindex.PatternArgument, bool) {
	if len(read.keywords) > 0 {
		return keywordArgument(pattern, read.keywords...)
	}
	return positionalArgument(pattern, 1)
}
