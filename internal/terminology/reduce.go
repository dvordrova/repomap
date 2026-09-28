package terminology

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

const CatalogVersion = 3
const StageName = "glossary"

//go:embed reduce_prompt.md
var reducePrompt string

type Entry struct {
	ID          string      `json:"id"`
	Names       []string    `json:"names"`
	Explanation string      `json:"explanation"`
	Sources     []Source    `json:"sources"`
	Variants    []Candidate `json:"variants"`
}

type Catalog struct {
	Version           int      `json:"version"`
	Entries           []Entry  `json:"entries"`
	PartialComparison bool     `json:"partial_comparison,omitempty"`
	Requests          []string `json:"requests,omitempty"`
	SHA256            string   `json:"sha256"`
}

// Reduce optionally consolidates already accepted domain explanations.
// A refused window keeps its input entries unchanged and is not tried again.
// Within an accepted window, a group whose choice was refused keeps its
// original entry while consistent joins survive. Accepted siblings survive;
// only real envelope refusals split whole groups. PartialComparison records
// any refusal, of a window or a group, or a nonshrinking disjoint comparison.
func Reduce(ctx context.Context, executor llm.Executor, provider llm.Provider, candidates []Candidate, progress ...func(state, detail string)) (Catalog, error) {
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	notify := func(state, detail string) {
		for _, fn := range progress {
			if fn != nil {
				fn(state, detail)
			}
		}
	}
	variants, err := normalizeCandidates(candidates)
	if err != nil {
		return Catalog{}, err
	}
	result := Catalog{Version: CatalogVersion, Entries: []Entry{}}
	var current []Entry
	for _, candidate := range variants {
		entry, err := makeEntry(candidate.Explanation, []Candidate{candidate})
		if err != nil {
			return Catalog{}, err
		}
		current = append(current, entry)
	}
	// A catalogue of singletons whose names never coincide leaves the model
	// nothing to decide: every group can only be assigned to itself. Morfeu
	// 20260911-152759 sent 121 such entries in a 36 KB window and got 121
	// self-assignments back. Names that differ only by case are the
	// spellings the comparison exists to join, so they still go.
	for len(current) > 1 && !distinctSingletons(current) {
		if err := ctx.Err(); err != nil {
			return Catalog{}, err
		}
		windows, err := planReduction(ctx, provider, current)
		if err != nil {
			return Catalog{}, err
		}
		var next []Entry
		acceptedInputs, finishedWindows := 0, 0
		for len(windows) > 0 {
			var calls []llm.Call[reduction]
			for i := 0; i < len(windows); i++ {
				call, err := reductionCall(windows[i])
				if err != nil {
					return Catalog{}, err
				}
				refused, err := llm.RecallAdaptiveSplit(executor, provider, call)
				if err != nil {
					return Catalog{}, err
				}
				if refused {
					if left, right, ok := splitReduction(windows[i]); ok {
						windows = slices.Concat(windows[:i], [][]Entry{left, right}, windows[i+1:])
						i-- // Rebuild complete children through the current owner.
						continue
					}
				}
				calls = append(calls, call)
			}
			if executor.PlanNotice != nil {
				executor.PlanNotice(len(windows))
			}
			failures := make(map[string]llm.FailureKind)
			eachExecutor := executor
			eachExecutor.Observer = llm.ObserverFunc(func(event llm.Event) error {
				if event.Kind == llm.EventFailure && event.Source == llm.SourceLive {
					failures[event.RequestSHA256] = event.Failure
				}
				if executor.Observer != nil {
					return executor.Observer.Observe(event)
				}
				return nil
			})
			outcomes := llm.ExecuteJSONEach(ctx, eachExecutor, provider, calls)
			if err := ctx.Err(); err != nil {
				return Catalog{}, err
			}
			var pending [][]Entry
			for i, outcome := range outcomes {
				for _, issue := range outcome.Outcome.Issues {
					if issue.Kind != llm.IssueCacheValidate && issue.Kind != llm.IssueMetrics {
						return Catalog{}, fmt.Errorf("glossary: %w", issue)
					}
				}
				if outcome.Err == nil {
					// Groups whose own choice was refused keep their original
					// entries beside the window's accepted joins.
					next = append(next, outcome.Outcome.Value.Entries...)
					result.PartialComparison = result.PartialComparison || outcome.Outcome.Value.Refused > 0
					acceptedInputs += len(windows[i])
					finishedWindows++
					if outcome.Outcome.CacheKey != "" {
						result.Requests = append(result.Requests, outcome.Outcome.CacheKey)
					}
					continue
				}
				// A provider-local deadline is an optional refusal. Only the
				// owning run context makes cancellation terminal.
				if err := ctx.Err(); err != nil {
					return Catalog{}, err
				}
				if reductionResource(outcome.Err) {
					if left, right, ok := splitReduction(windows[i]); ok {
						if _, err := llm.RememberAdaptiveSplit(executor, provider, calls[i], outcome.Outcome, outcome.Err); err != nil {
							return Catalog{}, err
						}
						notify("partitioned", fmt.Sprintf("the provider refused %d term entries in one request by resources; the complete set continues in 2 partitions", len(windows[i])))
						for _, child := range [][]Entry{left, right} {
							parts, err := planReduction(ctx, provider, child)
							if err != nil {
								return Catalog{}, err
							}
							pending = append(pending, parts...)
						}
						continue
					}
				} else {
					switch failures[outcome.Outcome.RequestSHA256] {
					case llm.FailureProvider, llm.FailureResponse, llm.FailureValidation:
					default:
						return Catalog{}, outcome.Err
					}
				}
				// The executor already recorded this rejected response. The
				// consolidation stage's fallback is its unchanged input, not
				// a repaired grouping or authority from the failed response.
				result.Entries = append(result.Entries, windows[i]...)
				result.PartialComparison = true
				finishedWindows++
			}
			windows = pending
		}
		complete, fixed := finishedWindows == 1, len(next) >= acceptedInputs
		current = next
		if complete || fixed {
			result.PartialComparison = result.PartialComparison || !complete
			break
		}
		// Grouping retains all originals, so fewer groups is the only monotone
		// progress measure. No iteration quota or evidence truncation is used.
	}
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	result.Entries = append(result.Entries, current...)
	if err := result.Seal(); err != nil {
		return Catalog{}, err
	}
	return result, nil
}

// distinctSingletons reports whether every entry holds one variant and no
// two entries share a name once case is folded.
func distinctSingletons(entries []Entry) bool {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if len(entry.Variants) != 1 {
			return false
		}
		name := strings.ToLower(strings.TrimSpace(entry.Variants[0].Name))
		if _, taken := seen[name]; taken {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}

type wireVariant struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Explanation string `json:"explanation"`
	SourceSet   string `json:"source_set"`
}

type wireGroup struct {
	Ref      string        `json:"ref"`
	Variants []wireVariant `json:"variants"`
}

type wireSource struct {
	Ref string `json:"ref"`
	Source
}

type wireSourceSet struct {
	Ref     string   `json:"ref"`
	Sources []string `json:"sources"`
	// Count is the real number of observations behind the variant; Sources
	// lists at most maxReductionSourcesPerVariant of them.
	Count int `json:"count"`
}

// maxReductionSourcesPerVariant bounds the observations a window lists per
// variant. The model chooses a representative explanation from the variants'
// text and their weight of evidence; it does not need every anchor. Freqtrade
// run 20260911-053911 sent 137 reduction windows of 1.9–3.1 MB, 114 million
// input tokens for 90 thousand output tokens, because a common term carried
// thousands of anchors into every window. The catalog entry keeps every
// source; only the wire is sampled.
const maxReductionSourcesPerVariant = 6

// Every window owns its own catalogues and no child depends on a parent
// window's ref allocation.
type reductionRequest struct {
	Sources    []wireSource    `json:"sources"`
	SourceSets []wireSourceSet `json:"source_sets"`
	Groups     []wireGroup     `json:"groups"`
}

func reductionCall(window []Entry) (llm.Call[reduction], error) {
	groups, variants := make(map[string]Entry), make(map[string]Candidate)
	owners := make(map[string]string)
	var request reductionRequest
	sourceRefs := make(map[Source]string)
	setRefs := make(map[string]string)
	for i, entry := range window {
		ref := fmt.Sprintf("g%d", i+1)
		groups[ref] = entry
		group := wireGroup{Ref: ref}
		for _, candidate := range entry.Variants {
			variant := fmt.Sprintf("v%d", len(variants)+1)
			variants[variant] = candidate
			owners[variant] = ref
			var sources []string
			sampled := candidate.Sources
			if len(sampled) > maxReductionSourcesPerVariant {
				sampled = sampled[:maxReductionSourcesPerVariant]
			}
			for _, source := range sampled {
				sourceRef, exists := sourceRefs[source]
				if !exists {
					sourceRef = fmt.Sprintf("s%d", len(sourceRefs)+1)
					sourceRefs[source] = sourceRef
					request.Sources = append(request.Sources, wireSource{Ref: sourceRef, Source: source})
				}
				sources = append(sources, sourceRef)
			}
			key := fmt.Sprintf("%d %s", len(candidate.Sources), strings.Join(sources, " ")) // Allocated s* refs cannot contain spaces.
			setRef, exists := setRefs[key]
			if !exists {
				setRef = fmt.Sprintf("p%d", len(setRefs)+1)
				setRefs[key] = setRef
				request.SourceSets = append(request.SourceSets, wireSourceSet{Ref: setRef, Sources: sources, Count: len(candidate.Sources)})
			}
			group.Variants = append(group.Variants, wireVariant{Ref: variant, Name: candidate.Name, Explanation: candidate.Explanation,
				SourceSet: setRef})
		}
		request.Groups = append(request.Groups, group)
	}
	user, err := json.Marshal(request)
	if err != nil {
		return llm.Call[reduction]{}, err
	}
	order := make([]string, len(request.Groups))
	for i, group := range request.Groups {
		order[i] = group.Ref
	}
	return llm.Call[reduction]{
		State:  []byte(`{"stage":"glossary","version":6}`),
		Prompt: llm.Prompt{System: reducePrompt, User: string(user), ResponseFormatJSON: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) (reduction, error) {
			return decodeReduction(raw, order, groups, variants, owners)
		},
	}, nil
}

// reduction is one accepted comparison window: its joined entries, the
// original entries of groups whose choice was refused, and those refusals.
type reduction struct {
	Entries    []Entry
	Refused    int
	Rejections []llm.ResponseRejection
}

func (value reduction) ResponseRejections() []llm.ResponseRejection { return value.Rejections }

// decodeReduction reads the assignments row by row. A group whose choice is
// malformed, unknown, empty, conflicting or omitted keeps its original entry,
// and so does every group that chose one of its variants. Groups connected by
// their choices join only when they all chose the same representative and its
// owning group chose it too; a chain or cycle leaves exactly its own groups
// separate. Nothing is joined transitively and nothing fills a missing choice.
// Consistent joins elsewhere in the window survive. A window with no accepted
// choice at all is refused whole.
func decodeReduction(raw []byte, order []string, groups map[string]Entry, variants map[string]Candidate, owners map[string]string) (reduction, error) {
	var response struct {
		Assignments []json.RawMessage `json:"assignments"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return reduction{}, err
	}
	var value reduction
	journal := make(map[string]int)
	record := func(kind, reason, sample string) {
		key := kind + "\x00" + reason
		index, found := journal[key]
		if !found {
			index = len(value.Rejections)
			journal[key] = index
			value.Rejections = append(value.Rejections, llm.ResponseRejection{Kind: kind, Reason: reason})
		}
		value.Rejections[index].Count++
		if len(value.Rejections[index].Samples) < 5 {
			value.Rejections[index].Samples = append(value.Rejections[index].Samples, sample)
		}
	}
	chosen := make(map[string]string)
	refused := make(map[string]string)
	for index, row := range response.Assignments {
		position := fmt.Sprintf("assignments[%d]", index)
		var fields map[string]json.RawMessage
		var ref string
		if json.Unmarshal(row, &fields) != nil || json.Unmarshal(fields["ref"], &ref) != nil {
			record("glossary_assignment_discarded", "assignment names no group", position)
			continue
		}
		if _, known := groups[ref]; !known {
			record("glossary_assignment_discarded", "unknown group ref", position)
			continue
		}
		if refused[ref] != "" {
			continue
		}
		var representative string
		if json.Unmarshal(fields["representative"], &representative) != nil || representative == "" {
			refused[ref] = "malformed or empty representative"
		} else if _, known := variants[representative]; !known {
			refused[ref] = "unknown representative"
		} else if previous, answered := chosen[ref]; answered && previous != representative {
			refused[ref] = "conflicting group assignment"
		} else {
			chosen[ref] = representative // An identical repeat is one answer.
			continue
		}
		delete(chosen, ref)
	}
	for _, ref := range order {
		if _, answered := chosen[ref]; !answered && refused[ref] == "" {
			refused[ref] = "group omitted"
		}
	}
	// Components follow each accepted choice to the group owning its variant.
	parent := make(map[string]string, len(order))
	var find func(string) string
	find = func(ref string) string {
		if parent[ref] == "" || parent[ref] == ref {
			return ref
		}
		parent[ref] = find(parent[ref])
		return parent[ref]
	}
	for _, ref := range order {
		if representative, ok := chosen[ref]; ok {
			if left, right := find(ref), find(owners[representative]); left != right {
				parent[left] = right
			}
		}
	}
	members := make(map[string][]string)
	var roots []string
	for _, ref := range order {
		root := find(ref)
		if members[root] == nil {
			roots = append(roots, root)
		}
		members[root] = append(members[root], ref)
	}
	accepted, firstReason := 0, ""
	for _, root := range roots {
		component := members[root]
		representative, consistent := chosen[component[0]], true
		for _, ref := range component {
			consistent = consistent && refused[ref] == "" && chosen[ref] == representative
		}
		if consistent {
			var originals []Candidate
			for _, ref := range component {
				originals = append(originals, groups[ref].Variants...)
			}
			entry, err := makeEntry(variants[representative].Explanation, originals)
			if err != nil {
				return reduction{}, err
			}
			value.Entries = append(value.Entries, entry)
			accepted++
			continue
		}
		for _, ref := range component {
			reason := refused[ref]
			switch {
			case reason != "":
			case len(component) > 1 && slices.ContainsFunc(component, func(other string) bool { return refused[other] != "" }):
				reason = "choice joins a group whose own choice was refused"
			default:
				reason = "representative chain or cycle"
			}
			record("glossary_group_unjoined", reason, ref)
			if firstReason == "" {
				firstReason = reason
			}
			value.Entries = append(value.Entries, groups[ref])
			value.Refused++
		}
	}
	if accepted == 0 {
		return reduction{}, fmt.Errorf("glossary: no group assignment accepted: %s", firstReason)
	}
	sort.Slice(value.Entries, func(i, j int) bool { return value.Entries[i].ID < value.Entries[j].ID })
	return value, nil
}

func planReduction(ctx context.Context, provider llm.Provider, entries []Entry) ([][]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call, err := reductionCall(entries)
	if err != nil {
		return nil, err
	}
	prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
	if err == nil && prepared.Len() > call.Limits.MaxRequestBytes {
		err = llm.NewResourceLimitError(llm.ResourceLimitError{Stage: StageName, Kind: llm.ResourceLimitRequestBytes, Limit: call.Limits.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true})
	}
	if err == nil {
		return [][]Entry{entries}, nil
	}
	var resource *llm.ResourceLimitError
	if !errors.As(err, &resource) || (resource.Kind != llm.ResourceLimitRequestBytes && resource.Kind != llm.ResourceLimitContextTokens) {
		return nil, err
	}
	left, right, ok := splitReduction(entries)
	if !ok {
		// Let ExecuteJSONEach record an indivisible envelope refusal. Its
		// fallback still preserves this whole accepted input entry.
		return [][]Entry{entries}, nil
	}
	a, err := planReduction(ctx, provider, left)
	if err != nil {
		return nil, err
	}
	b, err := planReduction(ctx, provider, right)
	return append(a, b...), err
}

func reductionResource(err error) bool {
	var resource *llm.ResourceLimitError
	if !errors.As(err, &resource) {
		return false
	}
	switch resource.Kind {
	case llm.ResourceLimitRequestBytes, llm.ResourceLimitContextTokens, llm.ResourceLimitResponseBytes, llm.ResourceLimitOutputTokens:
		return true
	}
	return false
}

// Split complete groups by their actual encoded original evidence weight.
func splitReduction(entries []Entry) ([]Entry, []Entry, bool) {
	if len(entries) < 2 {
		return nil, nil, false
	}
	weights := make([]int, len(entries))
	var total int
	for i, entry := range entries {
		call, _ := reductionCall([]Entry{entry})
		weights[i] = len(call.Prompt.User)
		total += weights[i]
	}
	middle, prefix := 1, weights[0]
	for middle < len(entries)-1 && prefix < total/2 {
		prefix += weights[middle]
		middle++
	}
	return entries[:middle], entries[middle:], true
}

// The same name with the same explanation is one definition: its sources
// and origins are the union of every observation of it.
func normalizeCandidates(values []Candidate) ([]Candidate, error) {
	byValue := make(map[string]Candidate)
	for _, value := range values {
		if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Explanation) == "" || len(value.Sources) == 0 {
			return nil, fmt.Errorf("glossary: invalid original candidate")
		}
		for _, source := range value.Sources {
			if !canonicalPath(source.Path) || source.Line < 0 {
				return nil, fmt.Errorf("glossary: invalid original source")
			}
		}
		key, _ := json.Marshal([]string{value.Name, value.Explanation})
		sources := append([]Source(nil), value.Sources...)
		origins := append([]Origin(nil), value.Origins...)
		if previous, exists := byValue[string(key)]; exists {
			sources = append(sources, previous.Sources...)
			origins = append(origins, previous.Origins...)
		}
		value.Sources, value.Origins = normalizeSources(sources), normalizeOrigins(origins)
		byValue[string(key)] = value
	}
	keys := make([]string, 0, len(byValue))
	for key := range byValue {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Candidate, 0, len(keys))
	for _, key := range keys {
		result = append(result, byValue[key])
	}
	return result, nil
}

func makeEntry(explanation string, candidates []Candidate) (Entry, error) {
	variants, err := normalizeCandidates(candidates)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Explanation: explanation, Variants: variants}
	for _, candidate := range variants {
		entry.Names = append(entry.Names, candidate.Name)
		entry.Sources = append(entry.Sources, candidate.Sources...)
	}
	if !slices.ContainsFunc(variants, func(value Candidate) bool { return value.Explanation == explanation }) {
		return Entry{}, fmt.Errorf("glossary: explanation is not an original variant")
	}
	sort.Strings(entry.Names)
	entry.Names = slices.Compact(entry.Names)
	sort.Slice(entry.Sources, func(i, j int) bool {
		if entry.Sources[i].Path != entry.Sources[j].Path {
			return entry.Sources[i].Path < entry.Sources[j].Path
		}
		return entry.Sources[i].Line < entry.Sources[j].Line
	})
	entry.Sources = slices.Compact(entry.Sources)
	identity := entry
	identity.Variants = append([]Candidate(nil), variants...)
	for i := range identity.Variants {
		identity.Variants[i].Origins = nil
	}
	encoded, _ := json.Marshal(identity)
	entry.ID = fmt.Sprintf("glossary-%x", sha256.Sum256(encoded))
	return entry, nil
}

// Seal binds the canonical catalogue, including local request provenance.
func (catalog *Catalog) Seal() error {
	if catalog.Version != CatalogVersion {
		return fmt.Errorf("glossary: unsupported catalogue version")
	}
	for i, entry := range catalog.Entries {
		canonical, err := makeEntry(entry.Explanation, entry.Variants)
		if err != nil {
			return err
		}
		catalog.Entries[i] = canonical
	}
	if catalog.Entries == nil {
		catalog.Entries = []Entry{}
	}
	sort.Slice(catalog.Entries, func(i, j int) bool { return catalog.Entries[i].ID < catalog.Entries[j].ID })
	catalog.Requests = append([]string(nil), catalog.Requests...)
	sort.Strings(catalog.Requests)
	catalog.Requests = slices.Compact(catalog.Requests)
	catalog.SHA256 = ""
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	catalog.SHA256 = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return catalog.Validate()
}

func (catalog Catalog) Validate() error {
	if catalog.Version != CatalogVersion || catalog.SHA256 == "" {
		return fmt.Errorf("glossary: invalid catalogue identity")
	}
	seen := make(map[string]bool)
	for i, entry := range catalog.Entries {
		canonical, err := makeEntry(entry.Explanation, entry.Variants)
		if err != nil || !reflect.DeepEqual(entry, canonical) || (i > 0 && catalog.Entries[i-1].ID >= entry.ID) {
			return fmt.Errorf("glossary: noncanonical entry")
		}
		for _, variant := range entry.Variants {
			variant.Origins = nil
			encoded, _ := json.Marshal(variant)
			if seen[string(encoded)] {
				return fmt.Errorf("glossary: original variant belongs to multiple entries")
			}
			seen[string(encoded)] = true
		}
	}
	want := catalog.SHA256
	catalog.SHA256 = ""
	encoded, err := json.Marshal(catalog)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(encoded)) != want {
		return fmt.Errorf("glossary: catalogue digest mismatch")
	}
	return nil
}

func (catalog Catalog) CanonicalJSON() ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(catalog)
}
