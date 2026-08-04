package flowexplain

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type FlowSeed struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	FlowType         string   `json:"flow_type,omitempty"`
	Trigger          string   `json:"trigger"`
	LikelyEntrypoint string   `json:"likely_entrypoint"`
	ValidSeedFiles   []string `json:"valid_seed_files"`
	UnverifiedSeeds  []string `json:"unverified_seed_paths"`
	Evidence         []string `json:"evidence"`
	CandidateBasis   string   `json:"candidate_basis,omitempty"`
}

type FlowBundle struct {
	FlowSeed         FlowSeed               `json:"flow_seed"`
	QueryTerms       []string               `json:"query_terms"`
	AliasTerms       []string               `json:"alias_terms"`
	SelectedFiles    []scoredFile           `json:"selected_files"`
	SelectedTests    []scoredFile           `json:"selected_tests"`
	SelectedDocs     []scoredFile           `json:"selected_docs"`
	SelectedPackages []string               `json:"selected_packages"`
	RelatedEdges     []flowEdge             `json:"related_edges"`
	SourceSignals    []sourcesignals.Signal `json:"source_signals,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
}

type scoredFile struct {
	Path         string   `json:"path"`
	Kind         string   `json:"kind"`
	Score        int      `json:"score"`
	Reasons      []string `json:"reasons"`
	MatchedTerms []string `json:"matched_terms,omitempty"`
}

type flowEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

const (
	FlowTypeRequest                     = "request"
	FlowTypeOperational                 = "operational"
	DirectionAccepted                   = "accepted"
	DirectionRejected                   = "rejected"
	CandidateBasisModelOrientation      = "model_orientation"
	CandidateBasisLocalEntrypoint       = "local_entrypoint_candidate"
	CandidateBasisSourceSignalAggregate = "local_source_signal_aggregate"
	CandidateBasisRuntimeActivity       = "local_runtime_activity"
)

type CandidateFlow struct {
	Name              string             `json:"name"`
	FlowType          string             `json:"flow_type,omitempty"`
	Trigger           string             `json:"trigger"`
	LikelyEntrypoint  string             `json:"likely_entrypoint"`
	LikelyFiles       []string           `json:"likely_files"`
	WhyInteresting    string             `json:"why_interesting"`
	Evidence          []string           `json:"evidence"`
	Confidence        float64            `json:"confidence"`
	LocalVerification *FlowVerification  `json:"local_verification,omitempty"`
	LocalProof        *flowproof.Session `json:"local_proof,omitempty"`
	Disposition       string             `json:"disposition,omitempty"`
	DispositionReason string             `json:"disposition_reason,omitempty"`
	CandidateBasis    string             `json:"candidate_basis,omitempty"`
}

// ClassifyCandidateFlow applies the local acceptance policy after grounding
// and confidence gates. Provider prose cannot decide this disposition.
func ClassifyCandidateFlow(flow *CandidateFlow) {
	if flow == nil {
		return
	}
	verified := flow.LocalVerification != nil && len(flow.LocalVerification.Verified) > 0
	if flow.LocalProof != nil || verified {
		flow.Disposition = DirectionAccepted
		flow.DispositionReason = ""
		return
	}
	flow.Disposition = DirectionRejected
	flow.DispositionReason = "no exact local verification"
}

type FlowVerification struct {
	Status        string   `json:"status"`
	ConfidenceCap float64  `json:"confidence_cap"`
	Verified      []string `json:"verified,omitempty"`
	Missing       []string `json:"missing,omitempty"`
}

var ignoredTerms = map[string]bool{
	"flow": true, "path": true, "lifecycle": true, "request": true,
	"response": true, "operation": true, "server": true, "client": true,
	"main":    true,
	"package": true, "module": true, "repo": true, "trigger": true,
	"likely": true, "entrypoint": true, "the": true, "and": true,
	"for": true, "from": true, "with": true, "this": true, "that": true,
	"what": true, "how": true, "which": true, "via": true, "using": true,
	"handles": true, "handled": true,
	// Evidence phrase junk — DeepSeek generates descriptive sentences
	"defines": true, "implements": true, "manages": true, "dispatches": true,
	"applies": true, "provides": true, "contains": true, "stores": true,
	"writes": true, "reads": true, "routes": true,
	"a": true, "an": true, "is": true, "it": true, "of": true, "to": true,
	"as": true, "by": true, "on": true, "in": true, "at": true,
	"its": true, "be": true, "or": true, "has": true, "was": true,
	"are": true, "been": true, "were": true, "had": true,
	"do": true, "does": true, "did": true, "can": true, "will": true,
	"would": true, "could": true, "should": true, "may": true, "might": true,
	"must": true, "shall": true, "not": true, "only": true, "also": true,
	"very": true, "just": true, "now": true, "into": true,
	"out": true, "each": true, "all": true, "every": true, "any": true,
	"some": true, "more": true, "most": true, "other": true, "such": true,
	"no": true, "up": true, "down": true, "off": true, "over": true,
	"about": true, "after": true, "before": true, "between": true,
	"through": true, "during": true, "above": true, "below": true,
	"etc": true, "e.g": true, "i.e": true, "v3": true,
	// Bundle metadata words that leak into evidence
	"allowed_paths": true, "candidate_file_index": true,
}

var aliasExpansions = map[string][]string{
	"put":     {"put", "kv", "txn", "key", "v3rpc", "backend", "mvcc"},
	"watch":   {"watch", "watcher", "watchable", "stream", "event"},
	"lease":   {"lease", "lessor", "ttl", "keepalive", "expire", "revoke", "grant"},
	"raft":    {"raft", "rafthttp", "propose", "proposal", "commit", "apply", "wal", "snapshot"},
	"write":   {"write", "put", "txn", "propose", "proposal", "commit", "apply", "wal", "backend", "mvcc"},
	"grpc":    {"grpc", "rpc", "v3rpc", "etcdserverpb", "proto"},
	"etcdctl": {"etcdctl", "ctlv3", "command", "cobra"},
	"stream":  {"stream", "watch", "watcher", "event"},
	"startup": {"startup", "init", "bootstrap", "config"},
	"command": {"command", "ctl", "cobra"},
}

func GenerateFlowID(name string) string {
	slug := strings.ToLower(name)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		digest := sha256.Sum256([]byte(name))
		slug = "flow-" + hex.EncodeToString(digest[:6])
	}
	return slug
}

func ExtractTerms(flowName, trigger, entrypoint string, evidence []string) (queryTerms, aliasTerms []string) {
	seen := map[string]bool{}
	var raw []string

	cleanWord := func(w string) string {
		w = strings.ToLower(strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}))
		return w
	}

	extract := func(s string) {
		for _, part := range strings.Fields(s) {
			w := cleanWord(part)
			if len(w) < 2 {
				continue
			}
			if ignoredTerms[w] {
				continue
			}
			if seen[w] {
				continue
			}
			seen[w] = true
			raw = append(raw, w)
		}
	}

	extract(flowName)

	if entrypoint != "" {
		base := strings.TrimSuffix(filepath.Base(entrypoint), filepath.Ext(entrypoint))
		base = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(base)
		extract(base)
	}

	// likely_files already enter the focused bundle as exact high-priority
	// seeds. Do not turn their directory structure or model evidence paths into
	// global search terms: generic segments such as command/ctlv3 otherwise
	// swamp the small local neighborhood with unrelated sibling files.

	allAlias := map[string]bool{}
	allQuery := map[string]bool{}

	for _, t := range raw {
		allQuery[t] = true
		if exps, ok := aliasExpansions[t]; ok {
			for _, e := range exps {
				allAlias[e] = true
			}
		} else {
			allAlias[t] = true
		}
	}

	queryTerms = sortedKeys(allQuery)
	aliasTerms = sortedKeys(allAlias)

	if queryTerms == nil {
		queryTerms = []string{}
	}
	if aliasTerms == nil {
		aliasTerms = []string{}
	}

	return
}

func sortedKeys(m map[string]bool) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

func ValidateSeedFiles(likelyFiles []string, trackedFiles []string) (valid []string, unverified []string) {
	trackedSet := make(map[string]bool, len(trackedFiles))
	for _, f := range trackedFiles {
		trackedSet[f] = true
		trackedSet[filepath.Clean(f)] = true
	}
	for _, lf := range likelyFiles {
		lf = filepath.Clean(lf)
		if trackedSet[lf] {
			valid = append(valid, lf)
		} else {
			unverified = append(unverified, lf)
		}
	}
	return
}

type fileCandidate struct {
	path         string
	score        int
	reasons      []string
	matchedTerms []string
	kind         string
	isSeed       bool
}

func SelectFlowFiles(trackedFiles []string, aliasTerms []string, validSeeds []string, facts *gofacts.Facts, maxFiles int) (files, tests, docs []scoredFile, pkgs []string, edges []flowEdge) {
	seedSet := make(map[string]bool)
	for _, s := range validSeeds {
		seedSet[filepath.Clean(s)] = true
	}

	type candidate struct {
		path         string
		score        int
		reasons      []string
		matchedTerms []string
		kind         string
		isSeed       bool
	}

	var candidates []fileCandidate

	for _, f := range trackedFiles {
		clean := filepath.Clean(f)
		isSeed := seedSet[clean]
		kind := detectKind(f)
		if !isSeed && !isFlowSourceKind(kind) {
			continue
		}

		score, reasons, matched := scoreFileLayered(f, kind, aliasTerms, isSeed)

		if score > 0 || isSeed {
			candidates = append(candidates, fileCandidate{
				path:         f,
				score:        score,
				reasons:      reasons,
				matchedTerms: matched,
				kind:         kind,
				isSeed:       isSeed,
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].isSeed != candidates[j].isSeed {
			return candidates[i].isSeed
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].path < candidates[j].path
	})

	if maxFiles <= 0 {
		maxFiles = 50
	}

	selectedCandidates := make([]fileCandidate, 0, maxFiles)
	for _, c := range candidates {
		entry := scoredFile{
			Path:         c.path,
			Kind:         c.kind,
			Score:        c.score,
			Reasons:      c.reasons,
			MatchedTerms: c.matchedTerms,
		}

		switch c.kind {
		case "test":
			tests = append(tests, entry)
		case "doc":
			docs = append(docs, entry)
		default:
			files = append(files, entry)
		}
		selectedCandidates = append(selectedCandidates, c)

		if len(files)+len(tests)+len(docs) >= maxFiles {
			break
		}
	}

	if facts != nil {
		pkgs, edges = selectPackagesAndEdges(selectedCandidates, facts, aliasTerms)
	}

	// Ensure non-nil slices for JSON serialization
	if files == nil {
		files = []scoredFile{}
	}
	if tests == nil {
		tests = []scoredFile{}
	}
	if docs == nil {
		docs = []scoredFile{}
	}
	if pkgs == nil {
		pkgs = []string{}
	}
	if edges == nil {
		edges = []flowEdge{}
	}

	return
}

func isFlowSourceKind(kind string) bool {
	switch kind {
	case "source", "test", "doc", "proto", "generated":
		return true
	default:
		return false
	}
}

func scoreFileLayered(path string, kind string, aliasTerms []string, isSeed bool) (int, []string, []string) {
	if isSeed {
		return 200, []string{"likely_file from candidate_flow"}, nil
	}

	score := 0
	var reasons []string
	var matched []string
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.Dir(path))

	add := func(s int, reason, term string) {
		score += s
		reasons = append(reasons, reason)
		if term != "" {
			matched = append(matched, term)
		}
	}

	// Penalties
	if strings.Contains(lower, "patch/") || strings.Contains(lower, "diff/") {
		score -= 20
	}
	if strings.Contains(lower, "v2store") {
		add(-100, "v2store is legacy, heavily deprioritized for v3 flow", "")
	}
	if strings.Contains(lower, "pathutil") && !isSeed {
		add(-30, "pathutil is generic utility", "")
	}

	// Exact basename term match: highest
	for _, t := range aliasTerms {
		if strings.TrimSuffix(base, filepath.Ext(base)) == t || base == t {
			add(100, "exact basename match '"+t+"'", t)
			break
		}
	}

	// Basename contains term
	for _, t := range aliasTerms {
		if len(t) >= 3 && containsPathToken(base, t) {
			add(70, "filename contains term '"+t+"'", t)
			break
		}
	}

	// Directory segment match
	for _, t := range aliasTerms {
		if len(t) >= 3 {
			segs := strings.Split(dir, "/")
			for _, seg := range segs {
				if seg == t {
					add(60, "directory segment '"+t+"'", t)
					break
				}
			}
			if score > 0 && len(reasons) > 0 {
				d := reasons[len(reasons)-1]
				if strings.Contains(d, "directory segment") {
					break
				}
			}
		}
	}

	// Path contains term
	for _, t := range aliasTerms {
		if len(t) >= 3 && containsPathToken(lower, t) {
			already := false
			for _, m := range matched {
				if m == t {
					already = true
					break
				}
			}
			if !already {
				add(50, "path contains term '"+t+"'", t)
			}
		}
	}

	// Domain-specific boosts
	hasV3RPC := strings.Contains(lower, "v3rpc")
	hasServer := strings.Contains(lower, "server/")
	hasClient := strings.Contains(lower, "client/")
	hasEtcdctl := strings.Contains(lower, "etcdctl/")
	hasAPI := strings.Contains(lower, "api/")
	hasMVCC := strings.Contains(lower, "mvcc")
	hasBackend := strings.Contains(lower, "backend")
	hasWAL := strings.Contains(lower, "wal")
	hasRaft := strings.Contains(lower, "raft")

	if hasV3RPC {
		add(40, "v3rpc handler", "")
	}
	if hasServer && !hasV3RPC {
		add(25, "server path", "")
	}
	if hasClient {
		add(30, "client path", "")
	}
	if hasEtcdctl {
		add(30, "etcdctl command path", "")
	}
	if hasAPI && kind == "proto" {
		add(45, "API proto file", "")
	}

	if hasMVCC && !strings.Contains(lower, "_test") {
		add(30, "mvcc storage component", "")
	}
	if hasBackend && !strings.Contains(lower, "_test") {
		add(30, "backend storage", "")
	}
	if hasWAL && !strings.Contains(lower, "_test") {
		add(30, "write-ahead log", "")
	}
	if hasRaft && !strings.Contains(lower, "_test") {
		add(30, "raft component", "")
	}

	if kind == "proto" {
		add(20, "protocol buffer definition", "")
	}
	if kind == "generated" {
		add(-10, "generated file (lower priority)", "")
	}

	if score == 0 {
		return 0, nil, nil
	}

	return score, reasons, unique(matched)
}

func containsPathToken(value, term string) bool {
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if token == term {
			return true
		}
	}
	return false
}

func selectPackagesAndEdges(candidates []fileCandidate, facts *gofacts.Facts, aliasTerms []string) ([]string, []flowEdge) {
	if facts == nil {
		return []string{}, []flowEdge{}
	}

	type pkgScore struct {
		importPath string
		score      int
		reason     string
	}
	pkgMap := map[string]pkgScore{}

	selectedPaths := map[string]bool{}
	for _, c := range candidates {
		selectedPaths[filepath.Clean(c.path)] = true
	}

	for _, ep := range facts.EntrypointPackages {
		if ep.PackageDir == "" {
			continue
		}
		s := 0
		reason := ""

		for _, gf := range ep.GoFiles {
			p := gf
			if ep.PackageDir != "." {
				p = ep.PackageDir + "/" + gf
			}
			if selectedPaths[filepath.Clean(p)] {
				s += 60
				reason = "contains selected files"
			}
		}

		lower := strings.ToLower(ep.ImportPath)
		for _, t := range aliasTerms {
			if len(t) >= 3 && strings.Contains(lower, t) {
				s += 50
				reason = "import path matches term '" + t + "'"
				break
			}
		}

		if s > 0 {
			pkgMap[ep.ImportPath] = pkgScore{importPath: ep.ImportPath, score: s, reason: reason}
		}
	}

	type ps struct {
		importPath string
		score      int
	}
	var scored []ps
	for _, p := range pkgMap {
		scored = append(scored, ps{p.importPath, p.score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].importPath < scored[j].importPath
	})

	pkgs := make([]string, 0, len(scored))
	for _, p := range scored {
		pkgs = append(pkgs, p.importPath)
	}
	if len(pkgs) > 15 {
		pkgs = pkgs[:15]
	}

	selectedPkgSet := map[string]bool{}
	for _, p := range pkgs {
		selectedPkgSet[p] = true
	}

	var edges []flowEdge
	for _, e := range facts.InternalEdges {
		if selectedPkgSet[e.From] && selectedPkgSet[e.To] {
			edges = append(edges, flowEdge{From: e.From, To: e.To, Reason: "selected package imports selected package"})
		}
	}
	if len(edges) == 0 {
		for _, e := range facts.InternalEdges {
			if selectedPkgSet[e.From] || selectedPkgSet[e.To] {
				edges = append(edges, flowEdge{From: e.From, To: e.To, Reason: "neighbor of selected package"})
			}
		}
	}
	if len(edges) > 20 {
		edges = edges[:20]
	}

	return pkgs, edges
}

func detectKind(path string) string {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))

	if strings.HasSuffix(base, "_test.go") ||
		(strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))) {
		return "test"
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".drawio") {
		return "doc"
	}
	if strings.HasSuffix(lower, ".proto") {
		return "proto"
	}
	if strings.HasSuffix(base, ".pb.go") {
		return "generated"
	}
	if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".pyi") {
		return "source"
	}
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml") {
		return "config"
	}
	return "unknown"
}

func unique(s []string) []string {
	seen := map[string]bool{}
	var r []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			r = append(r, v)
		}
	}
	return r
}
