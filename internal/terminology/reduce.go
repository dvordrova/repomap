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
// Accepted siblings survive; only real envelope refusals split whole groups.
// PartialComparison records any refusal or nonshrinking disjoint comparison.
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
	for len(current) > 1 {
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
			var calls []llm.Call[[]Entry]
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
					if issue.Kind != llm.IssueCacheValidate {
						return Catalog{}, fmt.Errorf("glossary: %w", issue)
					}
				}
				if outcome.Err == nil {
					next = append(next, outcome.Outcome.Value...)
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
}

// Every window owns complete catalogues. Repeated provenance is referenced,
// not sampled, and no child depends on a parent window's ref allocation.
type reductionRequest struct {
	Sources    []wireSource    `json:"sources"`
	SourceSets []wireSourceSet `json:"source_sets"`
	Groups     []wireGroup     `json:"groups"`
}

func reductionCall(window []Entry) (llm.Call[[]Entry], error) {
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
			for _, source := range candidate.Sources {
				sourceRef, exists := sourceRefs[source]
				if !exists {
					sourceRef = fmt.Sprintf("s%d", len(sourceRefs)+1)
					sourceRefs[source] = sourceRef
					request.Sources = append(request.Sources, wireSource{Ref: sourceRef, Source: source})
				}
				sources = append(sources, sourceRef)
			}
			key := strings.Join(sources, " ") // Allocated s* refs cannot contain spaces.
			setRef, exists := setRefs[key]
			if !exists {
				setRef = fmt.Sprintf("p%d", len(setRefs)+1)
				setRefs[key] = setRef
				request.SourceSets = append(request.SourceSets, wireSourceSet{Ref: setRef, Sources: sources})
			}
			group.Variants = append(group.Variants, wireVariant{Ref: variant, Name: candidate.Name, Explanation: candidate.Explanation,
				SourceSet: setRef})
		}
		request.Groups = append(request.Groups, group)
	}
	user, err := json.Marshal(request)
	if err != nil {
		return llm.Call[[]Entry]{}, err
	}
	return llm.Call[[]Entry]{
		State:  []byte(`{"stage":"glossary","version":5}`),
		Prompt: llm.Prompt{System: reducePrompt, User: string(user), ResponseFormatJSON: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) ([]Entry, error) {
			var response struct {
				Assignments []struct {
					Ref            string `json:"ref"`
					Representative string `json:"representative"`
				} `json:"assignments"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				return nil, err
			}
			assigned := make(map[string]string)
			for _, row := range response.Assignments {
				if _, known := groups[row.Ref]; !known {
					continue
				}
				if _, known := variants[row.Representative]; !known {
					return nil, fmt.Errorf("glossary: missing representative")
				}
				if previous, exists := assigned[row.Ref]; exists && previous != row.Representative {
					return nil, fmt.Errorf("glossary: conflicting group assignment")
				}
				assigned[row.Ref] = row.Representative
			}
			if len(assigned) != len(groups) {
				return nil, fmt.Errorf("glossary: response omits original groups")
			}
			joined := make(map[string][]Candidate)
			for _, group := range request.Groups {
				representative := assigned[group.Ref]
				if assigned[owners[representative]] != representative {
					return nil, fmt.Errorf("glossary: representative is outside its group")
				}
				joined[representative] = append(joined[representative], groups[group.Ref].Variants...)
			}
			var result []Entry
			for representative, originals := range joined {
				entry, err := makeEntry(variants[representative].Explanation, originals)
				if err != nil {
					return nil, err
				}
				result = append(result, entry)
			}
			sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
			return result, nil
		},
	}, nil
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

func normalizeCandidates(values []Candidate) ([]Candidate, error) {
	byValue := make(map[string]Candidate)
	for _, value := range values {
		if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Explanation) == "" || len(value.Sources) == 0 {
			return nil, fmt.Errorf("glossary: invalid original candidate")
		}
		value.Sources = append([]Source(nil), value.Sources...)
		for _, source := range value.Sources {
			if !canonicalPath(source.Path) || source.Line < 0 {
				return nil, fmt.Errorf("glossary: invalid original source")
			}
		}
		sort.Slice(value.Sources, func(i, j int) bool {
			if value.Sources[i].Path != value.Sources[j].Path {
				return value.Sources[i].Path < value.Sources[j].Path
			}
			return value.Sources[i].Line < value.Sources[j].Line
		})
		value.Sources = slices.Compact(value.Sources)
		origins := append([]Origin(nil), value.Origins...)
		value.Origins = nil
		key, _ := json.Marshal(value)
		if previous, exists := byValue[string(key)]; exists {
			origins = append(origins, previous.Origins...)
		}
		value.Origins = normalizeOrigins(origins)
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
