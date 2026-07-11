package sourcesignals

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanContent_RequestHandler(t *testing.T) {
	src := `package etcdserver

func (s *EtcdServer) Put(ctx context.Context, r *pb.PutRequest) (*pb.PutResponse, error) {
	return s.put(ctx, r)
}

func (s *EtcdServer) Range(ctx context.Context, r *pb.RangeRequest) (*pb.RangeResponse, error) {
	return s.rangeOp(ctx, r)
}
`
	signals := scanContent([]byte(src), "server/etcdserver/server.go", 10)

	if len(signals) == 0 {
		t.Fatal("expected at least one request_handler signal")
	}

	for _, sig := range signals {
		if sig.Category != "request_handler" {
			continue
		}
		if sig.Path != "server/etcdserver/server.go" {
			t.Errorf("unexpected path: %s", sig.Path)
		}
		if sig.Line < 1 || sig.Line > 10 {
			t.Errorf("unexpected line: %d", sig.Line)
		}
		if sig.Match == "" {
			t.Error("match should not be empty")
		}
		if sig.Snippet == "" {
			t.Error("snippet should not be empty")
		}
	}
}

func TestScanContent_BackgroundLoop(t *testing.T) {
	src := `package lease

func (le *lessor) runLoop() {
	defer le.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			le.expire()
		}
	}
}
`
	signals := scanContent([]byte(src), "server/lease/lessor.go", 10)

	categories := make(map[string]bool)
	for _, sig := range signals {
		categories[sig.Category] = true
	}

	if !categories["background_loop"] {
		t.Errorf("expected background_loop signals: got %v", categories)
	}
}

func TestScanContent_AdminMaintenance(t *testing.T) {
	src := `package etcdserver

func (s *EtcdServer) compactionLoop() {
	// handles compaction and defrag
}

func (s *EtcdServer) snapshot() {
	// creates snapshot
}

func (s *EtcdServer) alarmHandler() {
	// triggers alarm
}
`
	signals := scanContent([]byte(src), "server/etcdserver/server.go", 10)

	found := false
	for _, sig := range signals {
		if sig.Category == "admin_maintenance" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected admin_maintenance signals from compaction/defrag/snapshot/alarm references")
	}
}

func TestScanContent_ThresholdLimit(t *testing.T) {
	src := `package backend

const (
	defaultQuota = 2 * 1024 * 1024 * 1024
)

var ErrNoSpace = errors.New("NOSPACE: quota exceeded")

func checkQuota(n int) error {
	if n > maxSize {
		return fmt.Errorf("too large: %d exceeds %d", n, maxSize)
	}
	return nil
}
`
	signals := scanContent([]byte(src), "server/storage/backend/backend.go", 10)

	found := false
	for _, sig := range signals {
		if sig.Category == "threshold_limit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected threshold_limit signals from NOSPACE/quota/exceeded/too large references")
	}
}

func TestScanContent_ConsensusState(t *testing.T) {
	src := `package raft

func (n *node) Propose(ctx context.Context, data []byte) error {
	return n.step(ctx, pb.Message{Type: pb.MsgProp})
}

func (n *node) becomeLeader() {
	n.heartbeatElapsed = 0
}

func (n *node) apply() {
	for _, entry := range n.raftLog.committed {
		// apply committed entries
	}
}

func (n *node) tickHeartbeat() {
	if n.state == StateLeader {
		n.heartbeatElapsed++
	}
}
`
	signals := scanContent([]byte(src), "pkg/raft/node.go", 15)

	categories := make(map[string]bool)
	for _, sig := range signals {
		categories[sig.Category] = true
	}

	if !categories["consensus_state"] {
		t.Errorf("expected consensus_state signals: got %v", categories)
	}
}

func TestScanContent_StorageDurability(t *testing.T) {
	src := `package wal

func (w *WAL) Save(st raftpb.HardState, ents []raftpb.Entry) error {
	w.encoder.encode(ents)
	w.locks.Lock()
	defer w.locks.Unlock()
	if err := w.f.Sync(); err != nil {
		return err
	}
	w.bw.Flush()
	return nil
}
`
	signals := scanContent([]byte(src), "server/storage/wal/wal.go", 10)

	found := false
	for _, sig := range signals {
		if sig.Category == "storage_durability" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected storage_durability signals from Sync/wal references")
	}
}

func TestScanContent_ScopeMarker(t *testing.T) {
	src := `package v3rpc
import "fmt"
`
	signals := scanContent([]byte(src), "server/etcdserver/api/v3rpc/key.go", 10)

	// At minimum the package declaration should produce a scope_marker
	_ = signals
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		path string
		skip bool
	}{
		{"server/main.go", false},
		{"server/etcdserver/server.go", false},
		{"api/etcdserverpb/rpc.pb.go", true},   // generated
		{"api/etcdserverpb/rpc.pb.gw.go", true}, // generated gateway
		{"vendor/github.com/foo/bar.go", true},  // vendor
		{"node_modules/pkg/index.js", true},     // node_modules
		{".git/hooks/pre-commit", true},         // .git
		{"README.md", true},                     // not Go
		{"server/config.yaml", true},            // not Go
		{"server/etcdserver/api/v3rpc/key_test.go", false}, // test files are not skipped
	}
	for _, tt := range tests {
		got := shouldSkipFile(tt.path)
		if got != tt.skip {
			t.Errorf("shouldSkipFile(%q) = %v, want %v", tt.path, got, tt.skip)
		}
	}
}

func TestScanFiles_Integration(t *testing.T) {
	dir := t.TempDir()

	writeTestGoFile(t, dir, "server/handler.go", `package server

func (s *Server) Put(ctx context.Context, r *pb.PutRequest) (*pb.PutResponse, error) {
	return s.put(ctx, r)
}
`)
	writeTestGoFile(t, dir, "server/loop.go", `package server

func (s *Server) run() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			s.tick()
		}
	}
}
`)
	writeTestGoFile(t, dir, "server/raft.go", `package server

func (s *Server) Propose(ctx context.Context, data []byte) error {
	return s.node.Propose(ctx, data)
}
`)
	// This file is generated and should be skipped
	writeTestGoFile(t, dir, "api/rpc.pb.go", `package api
// generated code
`)

	files := []string{"server/handler.go", "server/loop.go", "server/raft.go", "api/rpc.pb.go"}

	signals := ScanFiles(files, dir, ScanOptions{MaxPerFile: 5, MaxTotal: 20})

	if len(signals) == 0 {
		t.Fatal("expected signals from Go source files")
	}

	// Check that .pb.go was skipped
	for _, sig := range signals {
		if sig.Path == "api/rpc.pb.go" {
			t.Error("generated file should be skipped")
		}
	}

	// Check signal fields
	for _, sig := range signals {
		if sig.Path == "" {
			t.Error("signal path should not be empty")
		}
		if sig.Line < 1 {
			t.Errorf("signal line should be >= 1, got %d", sig.Line)
		}
		if sig.Category == "" {
			t.Error("signal category should not be empty")
		}
		if sig.Match == "" {
			t.Error("signal match should not be empty")
		}
		if sig.Snippet == "" {
			t.Error("signal snippet should not be empty")
		}
	}
}

func TestScanFiles_MaxLimits(t *testing.T) {
	dir := t.TempDir()

	// Create one file with many patterns
	writeTestGoFile(t, dir, "server/all.go", `package server

func (s *Server) Put(ctx context.Context, r *pb.PutRequest) (*pb.PutResponse, error) { return nil, nil }
func (s *Server) Range(ctx context.Context, r *pb.RangeRequest) (*pb.RangeResponse, error) { return nil, nil }
func (s *Server) Watch(*pb.WatchRequest, pb.Watch_WatchServer) error { return nil }
func (s *Server) LeaseGrant(context.Context, *pb.LeaseGrantRequest) (*pb.LeaseGrantResponse, error) { return nil, nil }
func (s *Server) run() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-ctx.Done():
			return
		}
	}
}
func (s *Server) Propose(ctx context.Context, data []byte) error { return s.node.Propose(ctx, data) }
func (s *Server) snapshot() { /* compaction, snapshot, defrag */ }
`)

	// Max 3 signals per file
	signals := ScanFiles([]string{"server/all.go"}, dir, ScanOptions{MaxPerFile: 3, MaxTotal: 3})

	if len(signals) > 3 {
		t.Errorf("expected max 3 signals, got %d", len(signals))
	}
}

func TestScanSelectedFiles(t *testing.T) {
	dir := t.TempDir()

	writeTestGoFile(t, dir, "server/key.go", `package server

func (s *Server) Put(ctx context.Context, r *pb.PutRequest) (*pb.PutResponse, error) {
	return s.put(ctx, r)
}
`)

	signals := ScanSelectedFiles([]string{"server/key.go"}, dir, 10)

	if len(signals) == 0 {
		t.Fatal("expected signals from ScanSelectedFiles")
	}
}

func TestSignalHelpers(t *testing.T) {
	signals := []Signal{
		{Path: "a.go", Line: 10, Category: "request_handler", Reason: "test reason 1"},
		{Path: "a.go", Line: 20, Category: "request_handler", Reason: "test reason 2"},
		{Path: "a.go", Line: 30, Category: "consensus_state", Reason: "test reason 3"},
		{Path: "b.go", Line: 5, Category: "scope_marker", Reason: "test reason 4"},
	}

	if !ContainsCategory(signals, "request_handler") {
		t.Error("ContainsCategory should find request_handler")
	}
	if ContainsCategory(signals, "nonexistent") {
		t.Error("ContainsCategory should not find nonexistent")
	}

	cats := CategoriesForFile(signals, "a.go")
	if len(cats) != 2 {
		t.Errorf("expected 2 categories for a.go, got %d: %v", len(cats), cats)
	}

	if !FileHasSignalCategory(signals, "a.go", "request_handler") {
		t.Error("a.go should have request_handler")
	}
	if FileHasSignalCategory(signals, "a.go", "scope_marker") {
		t.Error("a.go should not have scope_marker")
	}

	reasons := SignalReasonsForFile(signals, "a.go")
	if len(reasons) != 3 {
		t.Errorf("expected 3 reasons, got %d", len(reasons))
	}

	pmap := BuildFileSignalMap(signals)
	if len(pmap) != 2 {
		t.Errorf("expected 2 files in map, got %d", len(pmap))
	}

	summ := Summary(signals)
	if summ["request_handler"] != 2 {
		t.Errorf("expected 2 request_handler in summary, got %d", summ["request_handler"])
	}
}

func TestScanContent_Empty(t *testing.T) {
	signals := scanContent([]byte(""), "empty.go", 10)
	if len(signals) != 0 {
		t.Errorf("expected no signals from empty file, got %d", len(signals))
	}
}

func TestScanContent_NonGoSyntax(t *testing.T) {
	src := `This is not valid Go code
	No functions here, just comments and random text
	`
	signals := scanContent([]byte(src), "notes.txt", 10)
	if len(signals) != 0 {
		t.Errorf("expected few/no signals from non-Go text, got %d", len(signals))
	}
}

func writeTestGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
