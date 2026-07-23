package sourcesignals

import (
	"bufio"
	"bytes"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/reporead"
)

type Signal struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Category string `json:"category"`
	Match    string `json:"match"`
	Snippet  string `json:"snippet"`
	Weight   int    `json:"weight,omitempty"`
	Penalty  int    `json:"penalty,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type categoryPattern struct {
	category string
	pattern  *regexp.Regexp
	weight   int
	reason   string
}

var categoryPatterns = buildPatterns()

func buildPatterns() []categoryPattern {
	var p []categoryPattern

	add := func(cat, pat string, weight int, reason string) {
		// Pattern must contain at least one capture group for the match
		p = append(p, categoryPattern{
			category: cat,
			pattern:  regexp.MustCompile(pat),
			weight:   weight,
			reason:   reason,
		})
	}

	// ── request_handler ──
	add("request_handler", `func\s*\(\w+\s*\*?\w*\)\s*(Put|Range|DeleteRange|Txn|Compact|Watch|LeaseGrant|LeaseRevoke|LeaseKeepAlive)\s*\(`, 40, "gRPC handler method signature")
	add("request_handler", `func\s+\w*(Put|Range|Watch|Lease|Txn|Compact)\w*\s*\(`, 35, "function name suggests handler role")
	add("request_handler", `\.(Register\w+Server|HandleFunc|Handle)\s*\(`, 40, "server registration call")
	add("request_handler", `ServeHTTP\s*\(`, 35, "HTTP handler method")
	add("request_handler", `(func\s+\(\w+\s+\*?\w+\)\s+(\w+)\s*\(.*\)\s*error)`, 15, "method returning error, possible handler")

	// ── background_loop ──
	add("background_loop", `time\.NewTicker\s*\(`, 40, "periodic ticker created")
	add("background_loop", `time\.NewTimer\s*\(`, 30, "timer created")
	add("background_loop", `\bticker\b`, 10, "ticker variable reference")
	add("background_loop", `<-ctx\.Done\(\)`, 30, "context cancellation check")
	add("background_loop", `go\s+func\s*\(`, 20, "goroutine launch (possible background)")

	// ── admin_maintenance ──
	add("admin_maintenance", `\b(compaction|Compaction)\b`, 35, "compaction reference")
	add("admin_maintenance", `\bdefrag\w*\b`, 35, "defragmentation reference")
	add("admin_maintenance", `\b(snapshot|Snapshot)\b`, 30, "snapshot reference")
	add("admin_maintenance", `\balarm\b`, 25, "alarm reference")
	add("admin_maintenance", `\bquota\b`, 25, "quota reference")
	add("admin_maintenance", `\b(restore|Restore)\b`, 25, "restore operation")
	add("admin_maintenance", `\b(endpoint|Endpoint)\s+(status|Status)\b`, 25, "endpoint status check")
	add("admin_maintenance", `\bmember\s+\w+\b`, 15, "member management")

	// ── threshold_limit ──
	add("threshold_limit", `\b(Quota|quota)\b`, 35, "quota enforcement")
	add("threshold_limit", `\bexceed(ed|s)?\b`, 30, "exceeded threshold")
	add("threshold_limit", `\btoo\s+(large|big|many)\b`, 25, "too large/many reference")
	add("threshold_limit", `\bNOSPACE\b`, 40, "no space error constant")
	add("threshold_limit", `\bResourceExhausted\b`, 35, "resource exhausted")
	add("threshold_limit", `\bErrNoSpace\b`, 35, "no space error")

	// ── consensus_state ──
	add("consensus_state", `\b(Propose|propose)\w*\s*\(`, 35, "Raft proposal")
	add("consensus_state", `\bReady\b`, 25, "Raft Ready state")
	add("consensus_state", `\b(apply|Apply)\w*\s*\(`, 20, "apply call")
	add("consensus_state", `\b(committed|Committed)\b`, 25, "committed state")
	add("consensus_state", `\b(leader|Leader)\b`, 20, "leader role/predicate")
	add("consensus_state", `\b(follower|Follower)\b`, 20, "follower role")
	add("consensus_state", `\b(election|Election)\b`, 25, "election process")
	add("consensus_state", `\b(heartbeat|Heartbeat)\b`, 25, "heartbeat mechanism")
	add("consensus_state", `\b(Campaign|Step)\s*\(`, 25, "Raft campaign/step")

	// ── storage_durability ──
	add("storage_durability", `\bwal\b`, 30, "WAL reference")
	add("storage_durability", `\b(snapshot|Snapshot)\b`, 25, "snapshot persistence")
	add("storage_durability", `\b(fsync|fdatasync|Sync)\s*\(`, 35, "fsync/sync call for durability")
	add("storage_durability", `\b(backend|Backend)\b`, 20, "backend storage reference")
	add("storage_durability", `\b(bolt|bbolt|Bolt)\b`, 30, "bolt/bbolt storage engine")
	add("storage_durability", `\b(commit|Commit)\s*\(`, 20, "storage commit call")
	add("storage_durability", `\bpersist\w*\b`, 15, "persistence reference")

	// ── observability ──
	add("observability", `\b(metrics|Metrics)\b`, 20, "metrics reference")
	add("observability", `\b(prometheus|Prometheus)\b`, 25, "prometheus reference")
	add("observability", `\b(log|Log)ger\b`, 15, "logger reference")
	add("observability", `\b(log|Log)\.(Info|Error|Warn|Debug)\w*\s*\(`, 10, "log call")
	add("observability", `\b(tracing|Tracing|trace|Trace)\b`, 15, "tracing reference")

	// ── scope_marker ──
	add("scope_marker", `\b(client/v3|ClientV3)\b`, 20, "client-side v3 scope")
	add("scope_marker", `\bv3rpc\b`, 25, "v3 RPC scope")
	add("scope_marker", `\bv2store\b`, 15, "v2 store scope (legacy)")
	add("scope_marker", `\b(rafthttp|RaftHTTP)\b`, 15, "raft HTTP transport scope")
	add("scope_marker", `\betcdserverpb\b`, 15, "etcd server protobuf scope")
	add("scope_marker", `\bmvccpb\b`, 15, "mvcc protobuf scope")

	return p
}

var skipFileSuffixes = []string{
	".pb.go", ".pb.gw.go",
}

var skipDirPrefixes = []string{
	"vendor/", "node_modules/", ".git/",
}

const maxFileBytes = 100 * 1024

func shouldSkipFile(path string) bool {
	lower := strings.ToLower(path)
	for _, s := range skipFileSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	for _, p := range skipDirPrefixes {
		if strings.HasPrefix(lower, p) || strings.Contains(lower, "/"+p) {
			return true
		}
	}
	if !strings.HasSuffix(lower, ".go") {
		return true
	}
	return false
}

type ScanOptions struct {
	MaxPerFile int
	MaxTotal   int
}

func defaults(opts ScanOptions) ScanOptions {
	if opts.MaxPerFile <= 0 {
		opts.MaxPerFile = 5
	}
	if opts.MaxTotal <= 0 {
		opts.MaxTotal = 200
	}
	return opts
}

func ScanFiles(filePaths []string, repoPath string, opts ScanOptions) []Signal {
	opts = defaults(opts)

	var signals []Signal
	reader, err := reporead.New(repoPath)
	if err != nil {
		return signals
	}
	defer reader.Close()

	for _, f := range artifactrole.SortPaths(filePaths) {
		if shouldSkipFile(f) {
			continue
		}
		if len(signals) >= opts.MaxTotal {
			break
		}

		remaining := opts.MaxTotal - len(signals)
		perFile := opts.MaxPerFile
		if perFile > remaining {
			perFile = remaining
		}

		fileSignals := scanFile(reader, f, perFile)
		signals = append(signals, fileSignals...)
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Path != signals[j].Path {
			return signals[i].Path < signals[j].Path
		}
		return signals[i].Line < signals[j].Line
	})

	return signals
}

func scanFile(reader *reporead.Reader, repoRel string, maxSignals int) []Signal {
	content, err := reader.ReadFile(repoRel, maxFileBytes)
	if err != nil || content.Truncated {
		return nil
	}

	return scanContent(content.Bytes, repoRel, maxSignals)
}

type lineMatch struct {
	line    int
	content string
}

func scanContent(data []byte, repoRel string, maxSignals int) []Signal {
	var signals []Signal

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 128*1024), 128*1024)

	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		lines = append(lines, scanner.Text())
	}
	_ = scanner.Err()

	type match struct {
		line     int
		rawLine  string
		category string
		match    string
		weight   int
		reason   string
	}

	var matches []match
	for i, l := range lines {
		lineNo := i + 1
		for _, cp := range categoryPatterns {
			found := cp.pattern.FindStringSubmatch(l)
			if found == nil {
				continue
			}
			matchText := trimMatch(found[0])
			if matchText == "" {
				continue
			}
			matches = append(matches, match{
				line:     lineNo,
				rawLine:  strings.TrimSpace(l),
				category: cp.category,
				match:    matchText,
				weight:   cp.weight,
				reason:   cp.reason,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].weight != matches[j].weight {
			return matches[i].weight > matches[j].weight
		}
		return matches[i].line < matches[j].line
	})

	seen := make(map[int]bool)
	for _, m := range matches {
		if len(signals) >= maxSignals {
			break
		}
		if seen[m.line] {
			continue
		}
		seen[m.line] = true

		snippet := m.rawLine
		if len(snippet) > 120 {
			snippet = snippet[:117] + "..."
		}

		signals = append(signals, Signal{
			Path:     repoRel,
			Line:     m.line,
			Category: m.category,
			Match:    m.match,
			Snippet:  snippet,
			Weight:   m.weight,
			Reason:   m.reason,
		})
	}

	return signals
}

func trimMatch(s string) string {
	s = strings.TrimSpace(s)
	// Remove excessive whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func ScanSelectedFiles(filePaths []string, repoPath string, maxTotal int) []Signal {
	if maxTotal <= 0 {
		maxTotal = 30
	}
	opts := ScanOptions{
		MaxPerFile: 3,
		MaxTotal:   maxTotal,
	}
	return ScanFiles(filePaths, repoPath, opts)
}

func ContainsCategory(signals []Signal, category string) bool {
	for _, s := range signals {
		if s.Category == category {
			return true
		}
	}
	return false
}

func CategoriesForFile(signals []Signal, path string) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, s := range signals {
		if s.Path == path {
			if !seen[s.Category] {
				seen[s.Category] = true
				cats = append(cats, s.Category)
			}
		}
	}
	return cats
}

func SignalReasonsForFile(signals []Signal, path string) []string {
	var reasons []string
	for _, s := range signals {
		if s.Path == path && s.Reason != "" {
			reasons = append(reasons, s.Reason)
		}
	}
	return reasons
}

func FileHasSignalCategory(signals []Signal, path, category string) bool {
	for _, s := range signals {
		if s.Path == path && s.Category == category {
			return true
		}
	}
	return false
}

func BuildFileSignalMap(signals []Signal) map[string][]Signal {
	m := make(map[string][]Signal)
	for _, s := range signals {
		m[s.Path] = append(m[s.Path], s)
	}
	return m
}

func Summary(signals []Signal) map[string]int {
	s := make(map[string]int)
	for _, sig := range signals {
		s[sig.Category]++
	}
	return s
}

// NopWriter implements a writer that does nothing — for use in tests.
type NopWriter struct{}

func (NopWriter) Write(p []byte) (n int, err error) { return len(p), nil }
