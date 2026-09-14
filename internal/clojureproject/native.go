package clojureproject

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/dvordrova/repomap/internal/corpus"
)

type site struct {
	Filename string `json:"filename"`
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	EndRow   int    `json:"end-row"`
	EndCol   int    `json:"end-col"`
	NameRow  int    `json:"name-row"`
	NameCol  int    `json:"name-col"`
	Lang     string `json:"lang"`
}
type namespace struct {
	site
	Name, From, To string
}
type definition struct {
	site
	NS        string   `json:"ns"`
	Name      string   `json:"name"`
	DefinedBy string   `json:"defined-by"`
	LintAs    string   `json:"defined-by->lint-as"`
	Arglists  []string `json:"arglist-strs"`
	Doc       string   `json:"doc"`
	Private   bool     `json:"private"`
	Macro     bool     `json:"macro"`
}
type usage struct {
	site
	Name, From, To string
	FromVar        string `json:"from-var"`
	Arity          *int   `json:"arity"`
	Macro          bool   `json:"macro"`
}
type local struct {
	site
	ID          int
	Name        string
	ScopeEndRow int `json:"scope-end-row"`
	ScopeEndCol int `json:"scope-end-col"`
}
type javaUsage struct {
	site
	Class  string
	Method string `json:"method-name"`
	Call   bool
}
type analysis struct {
	Namespaces  []namespace  `json:"namespace-definitions"`
	Imports     []namespace  `json:"namespace-usages"`
	Definitions []definition `json:"var-definitions"`
	Usages      []usage      `json:"var-usages"`
	Locals      []local      `json:"locals"`
	LocalUsages []local      `json:"local-usages"`
	Java        []javaUsage  `json:"java-class-usages"`
	Instances   []javaUsage  `json:"instance-invocations"`
}

func analyze(ctx context.Context, root string, files []corpus.Entry) (analysis, error) {
	args := []string{"--parallel", "--config", `{:output {:format :edn} :analysis {:arglists true :locals true :java-class-usages true :instance-invocations true}}`, "--lint"}
	for _, file := range files {
		args = append(args, file.Path)
	}
	cmd := exec.CommandContext(ctx, "clj-kondo", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	// Split only when the OS cannot represent the exact argument list. Merge
	// native rows before resolving namespace/var identities across the project.
	if errors.Is(err, syscall.E2BIG) && len(files) > 1 {
		left, e := analyze(ctx, root, files[:len(files)/2])
		if e != nil {
			return analysis{}, e
		}
		right, e := analyze(ctx, root, files[len(files)/2:])
		if e != nil {
			return analysis{}, e
		}
		left.Namespaces = append(left.Namespaces, right.Namespaces...)
		left.Imports = append(left.Imports, right.Imports...)
		left.Definitions = append(left.Definitions, right.Definitions...)
		left.Usages = append(left.Usages, right.Usages...)
		left.Locals = append(left.Locals, right.Locals...)
		left.LocalUsages = append(left.LocalUsages, right.LocalUsages...)
		left.Java = append(left.Java, right.Java...)
		left.Instances = append(left.Instances, right.Instances...)
		return left, nil
	}
	if ctx.Err() != nil {
		return analysis{}, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		// Lint findings do not invalidate native analysis; malformed syntax does.
		if !errors.As(err, &exit) || (exit.ExitCode() != 2 && exit.ExitCode() != 3) {
			return analysis{}, fmt.Errorf("Clojure native analysis (install clj-kondo): %w: %s", err, stderr.String())
		}
	}
	var output struct {
		Analysis *analysis `json:"analysis"`
		Findings []struct {
			Type, Message, Filename string
			Row                     int
		} `json:"findings"`
	}
	if err := decodeEDN(raw, &output); err != nil {
		return analysis{}, fmt.Errorf("decode clj-kondo: %w", err)
	}
	if output.Analysis == nil {
		return analysis{}, fmt.Errorf("clj-kondo returned no analysis")
	}
	for _, finding := range output.Findings {
		if finding.Type == "syntax" || finding.Type == "file" {
			return analysis{}, fmt.Errorf("clj-kondo %s:%d: %s", finding.Filename, finding.Row, finding.Message)
		}
	}
	return *output.Analysis, nil
}
