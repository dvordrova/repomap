package gofacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/reporead"
)

func TestCobraCommandReaderBuildsDispatchAndHandlerEvidence(t *testing.T) {
	t.Parallel()

	trace := buildCommandTraceFixture(t, map[string]string{
		"main.go": `package main

func main() { newRootCommand().ExecuteContext() }

func newRootCommand() *Command {
	cmd := &Command{}
	cmd.AddCommand(newBackupCommand())
	return cmd
}
`,
		"backup.go": `package main

func newBackupCommand() *Command {
	return &Command{
		Use: "backup [paths]",
		RunE: func() error { return runBackup() },
	}
}

func runBackup() error {
	repo := openRepository()
	repo.LoadIndex()
	scanner := archiver.NewScanner()
	go scanner.Scan()
	arch := archiver.New()
	return arch.Snapshot()
}
`,
	})

	if trace.Version != CommandTraceVersion || !trace.Complete || trace.Framework != "cobra" || trace.Command != "backup" {
		t.Fatalf("trace header = %#v", trace)
	}
	wantSymbols := []string{"main", "newRootCommand", "newBackupCommand", "runBackup"}
	wantRelations := []string{"entrypoint", "calls", "registers_command", "callback"}
	if len(trace.Steps) != len(wantSymbols) {
		t.Fatalf("steps = %#v", trace.Steps)
	}
	for index := range wantSymbols {
		if trace.Steps[index].Symbol != wantSymbols[index] || trace.Steps[index].Relation != wantRelations[index] {
			t.Fatalf("steps[%d] = %#v, want %s via %s", index, trace.Steps[index], wantSymbols[index], wantRelations[index])
		}
	}
	for _, symbol := range []string{"repo.LoadIndex", "archiver.NewScanner", "scanner.Scan", "archiver.New", "arch.Snapshot"} {
		if !hasCommandTraceCall(trace.HandlerCalls, symbol) {
			t.Fatalf("handler calls = %#v, want %s", trace.HandlerCalls, symbol)
		}
	}
}

func TestCommandTraceNestedCallbackIsNotSynchronousHandlerCall(t *testing.T) {
	t.Parallel()

	trace := buildCommandTraceFixture(t, map[string]string{
		"main.go": `package main

func main() { newRootCommand().ExecuteContext() }

func newRootCommand() *Command {
	cmd := &Command{}
	cmd.AddCommand(newBackupCommand())
	return cmd
}
`,
		"backup.go": `package main

func newBackupCommand() *Command {
	return &Command{
		Use: "backup",
		RunE: func() error { return runBackup(false) },
	}
}

func runBackup(noScan bool) error {
	scanner := archiver.NewScanner()
	if !noScan {
		wg.Go(func() error {
			return scanner.Scan()
		})
	}
	return nil
}
`,
	})

	if countCommandTraceCalls(trace.HandlerCalls, "wg.Go") != 1 {
		t.Fatalf("handler calls = %#v, want one wg.Go task registration", trace.HandlerCalls)
	}
	if countCommandTraceCalls(trace.HandlerCalls, "scanner.Scan") != 0 {
		t.Fatalf("handler calls = %#v, nested scanner.Scan must not be an immediate handler call", trace.HandlerCalls)
	}
	if trace.Concurrency != ConcurrencyPresentInHandler {
		t.Fatalf("concurrency scope = %q, want present handler lifecycle", trace.Concurrency)
	}
	var registration CommandTraceCall
	for _, call := range trace.HandlerCalls {
		if call.Symbol == "wg.Go" {
			registration = call
			break
		}
	}
	if registration.Condition == nil || registration.Condition.Expression != "!noScan" ||
		registration.Condition.Location.Path != "cmd/tool/backup.go" || registration.Condition.Location.Line <= 0 {
		t.Fatalf("wg.Go branch condition = %#v, want exact !noScan source condition", registration.Condition)
	}

	redacted := buildCommandTraceFixture(t, map[string]string{
		"main.go": `package main

func main() { newRootCommand().ExecuteContext() }

func newRootCommand() *Command {
	cmd := &Command{}
	cmd.AddCommand(newBackupCommand())
	return cmd
}
`,
		"backup.go": `package main

func newBackupCommand() *Command {
	return &Command{Use: "backup", RunE: func() error { return runBackup("") }}
}

func runBackup(token string) error {
	if token == "do-not-send" {
		wg.Go(func() error { return scanner.Scan() })
	}
	return nil
}
`,
	})
	redactedRegistrationFound := false
	for _, call := range redacted.HandlerCalls {
		if call.Symbol != "wg.Go" {
			continue
		}
		redactedRegistrationFound = true
		if call.Condition == nil || call.Condition.Expression != "" {
			t.Fatalf("literal-bearing condition leaked into compact facts: %#v", call.Condition)
		}
	}
	if !redactedRegistrationFound {
		t.Fatalf("redaction fixture has no wg.Go registration: %#v", redacted.HandlerCalls)
	}
}

func TestInitTraceIncludesCreateRepository(t *testing.T) {
	t.Parallel()

	trace := buildCommandTraceFixture(t, map[string]string{
		"main.go": `package main

func main() { newRootCommand().ExecuteContext() }

func newRootCommand() *Command {
	cmd := &Command{}
	cmd.AddCommand(newInitCommand())
	return cmd
}
`,
		"init.go": `package main

func newInitCommand() *Command {
	return &Command{
		Use: "init",
		RunE: func() error { return runInit() },
	}
}

func runInit() error {
	return global.CreateRepository(ctx)
}
`,
	})

	if countCommandTraceCalls(trace.HandlerCalls, "global.CreateRepository") != 1 {
		t.Fatalf("handler calls = %#v, want one global.CreateRepository callsite", trace.HandlerCalls)
	}
	if trace.Concurrency != ConcurrencyAbsentFromHandlerScope {
		t.Fatalf("concurrency scope = %q, want explicit synchronous handler scope", trace.Concurrency)
	}
}

func TestCobraDispatchKeepsCallsiteAndTargetLocationsSeparate(t *testing.T) {
	t.Parallel()

	mainSource := `package main

func main() {
	newRootCommand().ExecuteContext()
}

func newRootCommand() *Command {
	cmd := &Command{}
	cmd.AddCommand(
		newBackupCommand(),
	)
	return cmd
}
`
	backupSource := `package main

func newBackupCommand() *Command {
	return &Command{
		Use: "backup",
		RunE: func() error {
			return runBackup()
		},
	}
}

func runBackup() error { return nil }
`
	trace := buildCommandTraceFixture(t, map[string]string{
		"main.go":   mainSource,
		"backup.go": backupSource,
	})

	assertCommandTraceStepLocations(t, trace, "newRootCommand",
		"cmd/tool/main.go", sourceLine(t, mainSource, "newRootCommand().ExecuteContext()"),
		sourceCallColumn(t, mainSource, "newRootCommand().ExecuteContext()", "newRootCommand"),
		"cmd/tool/main.go", sourceLine(t, mainSource, "func newRootCommand()"),
	)
	assertCommandTraceStepLocations(t, trace, "newBackupCommand",
		"cmd/tool/main.go", sourceLine(t, mainSource, "newBackupCommand(),"),
		sourceCallColumn(t, mainSource, "newBackupCommand(),", "newBackupCommand"),
		"cmd/tool/backup.go", sourceLine(t, backupSource, "func newBackupCommand()"),
	)
	assertCommandTraceStepLocations(t, trace, "runBackup",
		"cmd/tool/backup.go", sourceLine(t, backupSource, "return runBackup()"),
		sourceCallColumn(t, backupSource, "return runBackup()", "runBackup"),
		"cmd/tool/backup.go", sourceLine(t, backupSource, "func runBackup()"),
	)
}

func buildCommandTraceFixture(t *testing.T, files map[string]string) CommandTrace {
	t.Helper()

	repo := t.TempDir()
	packageDir := filepath.Join(repo, "cmd", "tool")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	goFiles := make([]string, 0, len(files))
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(packageDir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		goFiles = append(goFiles, name)
	}

	reader, err := reporead.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	traces, warnings := buildEntrypointCommandTraces(reader, Entrypoint{
		ImportPath: "example.com/tool/cmd/tool",
		PackageDir: "cmd/tool",
		GoFiles:    goFiles,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(traces) != 1 {
		t.Fatalf("traces = %#v, want one", traces)
	}
	return traces[0]
}

func assertCommandTraceStepLocations(
	t *testing.T,
	trace CommandTrace,
	symbol string,
	wantCallsitePath string,
	wantCallsiteLine int,
	wantCallsiteColumn int,
	wantTargetPath string,
	wantTargetLine int,
) {
	t.Helper()
	for _, step := range trace.Steps {
		if step.Symbol != symbol {
			continue
		}
		if step.CallsiteLocation == nil || step.CallsiteLocation.Path != wantCallsitePath ||
			step.CallsiteLocation.Line != wantCallsiteLine || step.CallsiteLocation.Column != wantCallsiteColumn {
			t.Fatalf("%s callsite = %#v, want %s:%d:%d", symbol, step.CallsiteLocation, wantCallsitePath, wantCallsiteLine, wantCallsiteColumn)
		}
		if step.TargetLocation.Path != wantTargetPath || step.TargetLocation.Line != wantTargetLine {
			t.Fatalf("%s target = %s:%d, want %s:%d", symbol, step.TargetLocation.Path, step.TargetLocation.Line, wantTargetPath, wantTargetLine)
		}
		if step.CallsiteLocation.Path == "" || step.CallsiteLocation.Line == 0 {
			t.Fatalf("%s callsite is empty: %#v", symbol, step)
		}
		if *step.CallsiteLocation == step.TargetLocation {
			t.Fatalf("%s conflates callsite and target declaration: %#v", symbol, step)
		}
		return
	}
	t.Fatalf("step %q not found in %#v", symbol, trace.Steps)
}

func sourceLine(t *testing.T, source, needle string) int {
	t.Helper()
	for index, line := range strings.Split(source, "\n") {
		if strings.Contains(line, needle) {
			return index + 1
		}
	}
	t.Fatalf("source line containing %q not found", needle)
	return 0
}

func sourceCallColumn(t *testing.T, source, lineNeedle, callName string) int {
	t.Helper()
	for _, line := range strings.Split(source, "\n") {
		if !strings.Contains(line, lineNeedle) {
			continue
		}
		index := strings.Index(line, callName+"(")
		if index < 0 {
			t.Fatalf("call %q not found in %q", callName, line)
		}
		return index + len(callName) + 1
	}
	t.Fatalf("source line containing %q not found", lineNeedle)
	return 0
}

func hasCommandTraceCall(calls []CommandTraceCall, symbol string) bool {
	return countCommandTraceCalls(calls, symbol) > 0
}

func countCommandTraceCalls(calls []CommandTraceCall, symbol string) int {
	count := 0
	for _, call := range calls {
		if call.Symbol == symbol {
			count++
		}
	}
	return count
}
