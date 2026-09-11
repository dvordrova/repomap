package reading

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// The merge decoder reads a grouping against the pool's refs: an
// unadvertised ref is ignored, a ref in two groups stays in the first, a
// representative outside its members gives way to the first member, a ref
// no group named is its own group, a malformed group is skipped; a response
// without a groups array or naming nothing advertised is refused.
func TestLearningGroupDecoderRules(t *testing.T) {
	refs := []string{"q1", "q2", "q3", "q4", "q5"}
	for name, test := range map[string]struct {
		raw    string
		groups []learningGroup
		notes  []string
		refuse string
	}{
		"repairs": {
			raw: `{"groups":[{"representative":"q1","members":["q1","q2","q9","q2"]},{"representative":"q7","members":["q3","q2"]},{"representative":"q4","members":["q4"]}]}`,
			groups: []learningGroup{
				{Representative: "q1", Members: []string{"q1", "q2"}}, {Representative: "q3", Members: []string{"q3"}},
				{Representative: "q4", Members: []string{"q4"}}, {Representative: "q5", Members: []string{"q5"}},
			},
			notes: []string{
				`group 1 names the unadvertised ref "q9", ignored`,
				"q2 appears in groups 1 and 2, kept in group 1",
				`group 2 representative "q7" is not a member, q3 taken`,
				"q5 is in no group, its own",
			},
		},
		"malformed-group": {
			raw: `{"groups":[{"representative":"q1","members":"q1"},{"representative":"q2","members":["q2","q3"]}]}`,
			groups: []learningGroup{
				{Representative: "q2", Members: []string{"q2", "q3"}}, {Representative: "q1", Members: []string{"q1"}},
				{Representative: "q4", Members: []string{"q4"}}, {Representative: "q5", Members: []string{"q5"}},
			},
			notes: []string{"group 1 is malformed and was skipped", "q1 is in no group, its own", "q4 is in no group, its own", "q5 is in no group, its own"},
		},
		"complete": {
			raw:    `{"groups":[{"representative":"q5","members":["q1","q5"]},{"representative":"q2","members":["q2"]},{"representative":"q3","members":["q3","q4"]}]}`,
			groups: []learningGroup{{Representative: "q5", Members: []string{"q1", "q5"}}, {Representative: "q2", Members: []string{"q2"}}, {Representative: "q3", Members: []string{"q3", "q4"}}},
		},
		"no-groups":     {raw: `{}`, refuse: "learn: response needs a groups array"},
		"not-json":      {raw: `groups`, refuse: "learn: response needs a groups array"},
		"empty":         {raw: `{"groups":[]}`, refuse: "learn: response names no advertised ref"},
		"nothing-known": {raw: `{"groups":[{"representative":"x","members":["x"]}]}`, refuse: "learn: response names no advertised ref"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeLearningGroups([]byte(test.raw), refs)
			if test.refuse != "" {
				if err == nil || err.Error() != test.refuse {
					t.Fatalf("a response that decided nothing was read: %+v, %v", got, err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got.Groups, test.groups) || !reflect.DeepEqual(got.Notes, test.notes) {
				t.Fatalf("decoded %+v / %v", got, err)
			}
		})
	}
}

func learningMergeFixture(t *testing.T, reply string) (*reader, *learningProvider, []atlas.LearningQuestion) {
	t.Helper()
	provider := &learningProvider{mergeReply: []byte(reply)}
	r := isolatedLearningReader(t, t.TempDir(), provider)
	var questions []atlas.LearningQuestion
	for i, wording := range []string{"What is this service?", "What does the service do?", "What must be running?", "How is a film created?", "What does the -mode flag select?"} {
		questions = append(questions, atlas.LearningQuestion{Question: wording, Origins: []atlas.LearningOrigin{{Intent: fmt.Sprintf("intent%d", i), Question: wording, Why: fmt.Sprintf("Reason %d.", i)}}})
	}
	r.learning = &atlas.LearningPlan{State: "ready", Questions: append([]atlas.LearningQuestion{}, questions...)}
	return r, provider, questions
}

// One merge call per pool: the closed catalogue goes out as task and
// questions, the groups come back as refs. A joined question keeps every
// member's origins under its representative, a question the response
// omitted stays its own, and the journal holds the window's prompt, input,
// request, response and decoded groups beside the plan's record.
func TestLearningMergeGroupsOnePoolAndKeepsOrigins(t *testing.T) {
	r, provider, questions := learningMergeFixture(t, `{"groups":[{"representative":"q1","members":["q1","q2"]},{"representative":"q3","members":["q3"]},{"representative":"q4","members":["q4"]}]}`)
	if err := r.mergeLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []atlas.LearningQuestion{
		{Question: questions[0].Question, Origins: append(append([]atlas.LearningOrigin{}, questions[0].Origins...), questions[1].Origins...)},
		questions[2], questions[3], questions[4],
	}
	if r.learning.State != "ready" || !reflect.DeepEqual(r.learning.Questions, want) {
		t.Fatalf("grouping: state %q, %+v", r.learning.State, r.learning.Questions)
	}
	if !reflect.DeepEqual(r.learning.Groups, []atlas.LearningGroup{{Representative: questions[0].Question, Members: []string{questions[0].Question, questions[1].Question}, Source: atlas.SourceModel, Round: 1}}) {
		t.Fatalf("plan groups: %+v", r.learning.Groups)
	}
	var prompt llm.Prompt
	if len(provider.requests) != 1 || json.Unmarshal(provider.requests[0], &prompt) != nil {
		t.Fatalf("one request expected: %d", len(provider.requests))
	}
	var request struct {
		Task      string `json:"task"`
		Questions []struct {
			Ref      string `json:"ref"`
			Question string `json:"question"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil || request.Task != learningMergeContract || len(request.Questions) != 5 || request.Questions[4].Ref != "q5" || request.Questions[4].Question != questions[4].Question {
		t.Fatalf("request form: %s / %v", prompt.User, err)
	}
	if prompt.Reasoning || !prompt.ResponseFormatJSON || prompt.ResponseExample != learningMergeResponseExample || !strings.Contains(prompt.System, "Group the learning questions of one repository") {
		t.Fatalf("call shape: %+v", prompt)
	}
	journal := r.tables.String()
	for _, line := range []string{
		"## atlas_learn · round 1 · window 0 · tables/atlas_learn-r1-w0.request.ref.json",
		"source: model", "questions: 5 · groups: 4 · joined: 1",
		"- q1 «What is this service?» ← q2 «What does the service do?»",
		"- q5 «What does the -mode flag select?»",
		"- note: q5 is in no group, its own",
	} {
		if !strings.Contains(journal, line) {
			t.Fatalf("tables journal lacks %q:\n%s", line, journal)
		}
	}
	for _, name := range []string{"prompt.ref.json", "input.ref.json", "request.ref.json", "response.ref.json", "result.json"} {
		if _, err := os.Stat(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, "atlas_learn-r1-w0."+name)); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, "atlas_learn-r1-w0.result.json"))
	var result struct {
		Source string          `json:"source"`
		Groups []learningGroup `json:"groups"`
		Notes  []string        `json:"notes"`
	}
	if err != nil || json.Unmarshal(saved, &result) != nil || result.Source != atlas.SourceModel || len(result.Groups) != 4 || !reflect.DeepEqual(result.Notes, []string{"q5 is in no group, its own"}) {
		t.Fatalf("result.json: %s / %v", saved, err)
	}
	if use := r.use(stageLearn); len(r.rejected) != 0 || use.Windows != 1 || use.Rows != 5 || use.Live != 1 || use.Given != 0 {
		t.Fatalf("use: %+v, rejected %+v", use, r.rejected)
	}
}

// A refused merge window decides nothing: the pool's five questions stay
// their own groups with their origins, the plan says partial and the journal
// names the window with its response.
func TestLearningRefusedMergeWindowLeavesThePoolUnchanged(t *testing.T) {
	r, provider, questions := learningMergeFixture(t, `{"groups":[{"representative":"q9","members":["q8","q9"]}]}`)
	if err := r.mergeLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "partial" || !reflect.DeepEqual(r.learning.Questions, questions) || len(r.learning.Groups) != 0 || provider.calls != 1 {
		t.Fatalf("refused window changed the pool: state %q, %+v", r.learning.State, r.learning.Questions)
	}
	if len(r.rejected) != 1 || r.rejected[0].Kind != "window_rejected" || r.rejected[0].Count != 5 || r.rejected[0].ResponseRef != "tables/atlas_learn-r1-w0.response.ref.json" ||
		!reflect.DeepEqual(r.rejected[0].Samples, []string{"round 1 window 0"}) || !strings.Contains(r.rejected[0].Reason, "learn: response names no advertised ref") {
		t.Fatalf("journal: %+v", r.rejected)
	}
	journal := r.tables.String()
	for _, line := range []string{"source: given", "rejected: ", "names no advertised ref", "- q1 «What is this service?»", "- q5 «What does the -mode flag select?»"} {
		if !strings.Contains(journal, line) {
			t.Fatalf("tables journal lacks %q:\n%s", line, journal)
		}
	}
	if use := r.use(stageLearn); use.Windows != 1 || use.Rows != 5 || use.Live != 1 || use.Rejected != 1 || use.Given != 5 {
		t.Fatalf("use: %+v", use)
	}
}
