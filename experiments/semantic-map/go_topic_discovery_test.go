package semanticmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
)

const (
	etcdGoTopicRevision = "58f45a9ff1c083130830eb02b0cc7d9783609095"
)

func TestGoTopicInventoryIsCallableOnlyRoundRobinAndDeterministic(t *testing.T) {
	repoPath, revision := makeGoTopicInventoryFixture(t)
	first, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("Go topic inventory is not byte-identical across two runs")
	}
	if len(firstJSON) > goTopicMaxInventoryBytes {
		t.Fatalf("inventory bytes = %d, limit %d", len(firstJSON), goTopicMaxInventoryBytes)
	}
	gotNames := make([]string, 0, len(first.Declarations))
	for _, declaration := range first.Declarations {
		gotNames = append(gotNames, declaration.Name)
		if strings.Contains(declaration.Signature, "BODY_SENTINEL") {
			t.Fatal("function body leaked into declaration signature")
		}
		if strings.HasSuffix(declaration.Path, "_test.go") ||
			declaration.Name == "GeneratedOnly" ||
			declaration.Name == "init" {
			t.Fatalf("excluded declaration survived: %#v", declaration)
		}
		if declaration.Name == "Put" {
			if declaration.Kind != "method" ||
				declaration.Receiver != "*Box[T]" ||
				declaration.Signature != "func(value T) error" {
				t.Fatalf("generic method declaration = %#v", declaration)
			}
		}
	}
	wantPrefix := []string{
		"rootOne",
		"alphaOne",
		"zetaOne",
		"rootTwo",
		"deepOne",
		"zetaTwo",
		"otherOne",
		"Put",
		"deepTwo",
		"otherTwo",
	}
	if len(gotNames) < len(wantPrefix) ||
		!reflect.DeepEqual(gotNames[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("round-robin names = %v, want prefix %v", gotNames, wantPrefix)
	}
	for _, forbidden := range []string{"RootType", "AlphaValue", repoPath} {
		if bytes.Contains(firstJSON, []byte(forbidden)) {
			t.Fatalf("inventory contains forbidden non-callable/absolute text %q", forbidden)
		}
	}
	if first.Stats.TestFilesExcluded != 1 ||
		first.Stats.NonRegularFilesExcluded != 1 ||
		first.Stats.GeneratedFilesExcluded != 1 ||
		first.Stats.RetainedDeclarations != len(first.Declarations) {
		t.Fatalf("inventory stats = %#v", first.Stats)
	}
}

func TestGoTopicRoundRobinInterleavesFilesWithinDirectory(t *testing.T) {
	candidates := []goTopicDeclarationCandidate{
		{path: "flat/alpha.go", line: 10, name: "alphaOne"},
		{path: "flat/alpha.go", line: 20, name: "alphaTwo"},
		{path: "flat/beta.go", line: 10, name: "betaOne"},
		{path: "flat/beta.go", line: 20, name: "betaTwo"},
	}
	got := goTopicRoundRobinFiles(candidates)
	gotNames := make([]string, 0, len(got))
	for _, candidate := range got {
		gotNames = append(gotNames, candidate.name)
	}
	want := []string{"alphaOne", "betaOne", "alphaTwo", "betaTwo"}
	if !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("file round-robin = %v, want %v", gotNames, want)
	}
}

func TestGoTopicPromptAndShelfContract(t *testing.T) {
	repoPath, revision := makeGoTopicInventoryFixture(t)
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	systemPrompt, userPrompt, err := BuildGoTopicPrompt(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if systemPrompt != goTopicSystemPrompt ||
		!strings.Contains(strings.ToLower(userPrompt), "json") ||
		strings.Contains(userPrompt, goTopicInventoryPlaceholder) ||
		strings.Contains(userPrompt, "BODY_SENTINEL") ||
		len(userPrompt) > goTopicMaxPromptBytes {
		t.Fatal("topic prompt does not preserve the bounded declaration-only contract")
	}

	shelf := makeValidGoTopicShelf(inventory)
	response, err := json.Marshal(shelf)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeGoTopicShelf(response, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, shelf) {
		t.Fatal("strict topic shelf decoding changed the response")
	}

	invalid := shelf
	invalid.Topics = append([]GoTopic(nil), shelf.Topics...)
	invalid.Topics[0] = shelf.Topics[0]
	invalid.Topics[0].SupportSymbolIDs = []string{
		inventory.Declarations[0].ID,
		"d9999",
	}
	invalidJSON, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeGoTopicShelf(invalidJSON, inventory); err == nil ||
		!strings.Contains(err.Error(), "unknown declaration") {
		t.Fatalf("unknown declaration error = %v", err)
	}

	invalid = shelf
	invalid.Topics = append([]GoTopic(nil), shelf.Topics...)
	invalid.Topics[0] = shelf.Topics[0]
	invalid.Topics[0].Label = "server package"
	invalidJSON, err = json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeGoTopicShelf(invalidJSON, inventory); err == nil ||
		!strings.Contains(err.Error(), "package/file") {
		t.Fatalf("package-label error = %v", err)
	}

	for _, mutation := range []struct {
		name  string
		apply func(*GoTopic)
	}{
		{
			name: "how prefix",
			apply: func(topic *GoTopic) {
				topic.HowQuestion = "Where does this behavior run?"
			},
		},
		{
			name: "change prefix",
			apply: func(topic *GoTopic) {
				topic.ChangeQuestion = "How should this behavior change?"
			},
		},
		{
			name: "path leak",
			apply: func(topic *GoTopic) {
				topic.Why = "Start with alpha/alpha.go before exploring the behavior."
			},
		},
		{
			name: "absolute path leak",
			apply: func(topic *GoTopic) {
				topic.Why = "Start with /workspace/alpha before exploring the behavior."
			},
		},
		{
			name: "declaration ID leak",
			apply: func(topic *GoTopic) {
				topic.Label = "Lifecycle around d0001"
			},
		},
		{
			name: "file line leak",
			apply: func(topic *GoTopic) {
				topic.Why = "The behavior begins at entry:42."
			},
		},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			invalid := makeValidGoTopicShelf(inventory)
			mutation.apply(&invalid.Topics[0])
			invalidJSON, err := json.Marshal(invalid)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeGoTopicShelf(invalidJSON, inventory); err == nil {
				t.Fatal("invalid topic shelf was accepted")
			}
		})
	}

	validWithNaturalSlash := makeValidGoTopicShelf(inventory)
	validWithNaturalSlash.Topics[0].Why = "Useful for debugging startup/shutdown behavior."
	validWithNaturalSlash.Topics[0].Label = "Startup/shutdown lifecycle"
	validJSON, err := json.Marshal(validWithNaturalSlash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeGoTopicShelf(validJSON, inventory); err != nil {
		t.Fatalf("natural slash wording was rejected: %v", err)
	}
}

func TestGoTopicProviderRequestIsOneShotJSONWithThinkingDisabled(t *testing.T) {
	repoPath, revision := makeGoTopicInventoryFixture(t)
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	systemPrompt, userPrompt, err := BuildGoTopicPrompt(inventory)
	if err != nil {
		t.Fatal(err)
	}
	client := &deepseek.Client{
		Endpoint:  "https://api.deepseek.com/chat/completions",
		Model:     "deepseek-v4-flash",
		MaxTokens: 7891,
	}
	request, err := client.ComponentSynthesisPromptJSON(goTopicSynthesisPrompt(
		systemPrompt,
		userPrompt,
	))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Model          string `json:"model"`
		MaxTokens      int    `json:"max_tokens"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Thinking struct {
			Type string `json:"type"`
		} `json:"thinking"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != "deepseek-v4-flash" ||
		decoded.MaxTokens != client.MaxTokens ||
		decoded.ResponseFormat.Type != "json_object" ||
		decoded.Thinking.Type != "disabled" ||
		len(decoded.Messages) != 2 ||
		!strings.HasPrefix(
			decoded.Messages[0].Content,
			systemPrompt+"\n\n",
		) ||
		!strings.Contains(
			decoded.Messages[0].Content,
			deepseek.SemanticOutputLanguageContractVersion,
		) ||
		!strings.HasSuffix(
			decoded.Messages[1].Content,
			"\n\n"+userPrompt,
		) ||
		!strings.Contains(
			decoded.Messages[1].Content,
			"The response is the canonical semantic result.",
		) {
		t.Fatalf("provider request = %#v", decoded)
	}
}

func TestGoTopicOutputsAreExclusiveAndDoNotFollowSymlinks(t *testing.T) {
	outputDir := t.TempDir()
	responsePath := filepath.Join(outputDir, "response.json")
	experimentPath := filepath.Join(outputDir, "experiment.json")
	if _, _, err := goTopicOutputPaths(responsePath, responsePath); err == nil ||
		!strings.Contains(err.Error(), "must differ") {
		t.Fatalf("same output path error = %v", err)
	}
	if err := writeGoTopicExclusive(responsePath, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeGoTopicExclusive(responsePath, []byte("second")); err == nil {
		t.Fatal("exclusive writer overwrote an existing output")
	}
	content, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first" {
		t.Fatalf("existing output = %q, want first", content)
	}
	if _, _, err := goTopicOutputPaths(responsePath, experimentPath); err == nil ||
		!strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("existing output preflight error = %v", err)
	}

	danglingPath := filepath.Join(outputDir, "dangling.json")
	danglingTarget := filepath.Join(outputDir, "missing-target.json")
	if err := os.Symlink(danglingTarget, danglingPath); err != nil {
		t.Fatal(err)
	}
	if err := writeGoTopicExclusive(danglingPath, []byte("secret")); err == nil {
		t.Fatal("exclusive writer followed a dangling symlink")
	}
	if _, err := os.Stat(danglingTarget); !os.IsNotExist(err) {
		t.Fatalf("dangling symlink target stat error = %v, want not exist", err)
	}
	if _, _, err := goTopicOutputPaths(danglingPath, experimentPath); err == nil ||
		!strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("dangling output preflight error = %v", err)
	}
}

func TestRecordedGoTopicExperiments(t *testing.T) {
	type recordedCase struct {
		name     string
		revision string
	}
	candidates := []recordedCase{
		{name: "caddy", revision: caddyGoSelectionRevision},
		{name: "etcd", revision: etcdGoTopicRevision},
	}
	var cases []recordedCase
	for _, item := range candidates {
		if _, err := os.Stat(item.name + ".topic-experiment.json"); err == nil {
			cases = append(cases, item)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if len(cases) == 0 {
		// Capture persists the unedited provider response before downstream
		// decode and source selection. A response without an experiment is
		// therefore a legitimate failed-capture artifact, not a complete
		// replay fixture.
		t.Skip("complete topic experiments are created only by explicit live capture")
	}

	var promptHash string
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			response := readBoundedFile(
				t,
				item.name+".topic-shelf.response.json",
				goTopicMaxResponseBytes,
			)
			experimentJSON := readBoundedFile(
				t,
				item.name+".topic-experiment.json",
				2<<20,
			)
			experiment := decodeStrict[GoTopicExperiment](t, experimentJSON)
			if err := ValidateGoTopicExperiment(experiment); err != nil {
				t.Fatal(err)
			}
			if experiment.Repository.Revision != item.revision {
				t.Fatalf(
					"repository revision = %s, want %s",
					experiment.Repository.Revision,
					item.revision,
				)
			}
			if got := goTopicSHA256(response); got != experiment.ResponseSHA256 {
				t.Fatalf("response SHA-256 = %s, want %s", got, experiment.ResponseSHA256)
			}
			shelf, err := DecodeGoTopicShelf(response, experiment.Inventory)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(shelf, experiment.Shelf) {
				t.Fatal("recorded unedited response and experiment shelf differ")
			}
			encoded, err := EncodeGoTopicExperiment(experiment)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, experimentJSON) {
				t.Fatal("recorded topic experiment is not canonically encoded")
			}
			if promptHash == "" {
				promptHash = experiment.PromptTemplateSHA256
			} else if experiment.PromptTemplateSHA256 != promptHash {
				t.Fatal("Caddy and etcd used different topic prompt templates")
			}
		})
	}
}

func TestLiveGoTopicInventory(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_TOPIC_REPO")
	revision := os.Getenv("REPOMAP_TOPIC_REVISION")
	if repoPath == "" && revision == "" {
		t.Skip("set REPOMAP_TOPIC_REPO/REVISION for a pinned inventory replay")
	}
	if repoPath == "" || revision == "" {
		t.Fatal("REPOMAP_TOPIC_REPO and REPOMAP_TOPIC_REVISION must be set together")
	}
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	inventoryJSON, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, min(16, len(inventory.Declarations)))
	for _, declaration := range inventory.Declarations[:min(16, len(inventory.Declarations))] {
		names = append(names, declaration.Name)
	}
	t.Logf(
		"%s@%s: %d/%d declarations, %d bytes, first names=%v",
		inventory.Repository.Name,
		inventory.Repository.Revision,
		len(inventory.Declarations),
		inventory.Stats.CandidateDeclarations,
		len(inventoryJSON),
		names,
	)
}

func TestLiveGoTopicDiscovery(t *testing.T) {
	label := os.Getenv("REPOMAP_TOPIC_LABEL")
	repoPath := os.Getenv("REPOMAP_TOPIC_REPO")
	revision := os.Getenv("REPOMAP_TOPIC_REVISION")
	outputDir := os.Getenv("REPOMAP_TOPIC_OUTPUT_DIR")
	if label == "" && repoPath == "" && revision == "" && outputDir == "" {
		t.Skip("set REPOMAP_TOPIC_LABEL/REPO/REVISION/OUTPUT_DIR for one explicit live capture")
	}
	if label != "caddy" && label != "etcd" {
		t.Fatalf("REPOMAP_TOPIC_LABEL = %q, want caddy or etcd", label)
	}
	if repoPath == "" || revision == "" || outputDir == "" {
		t.Fatal("all REPOMAP_TOPIC_* live capture settings are required")
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	responsePath := filepath.Join(outputDir, label+".topic-shelf.response.json")
	experimentPath := filepath.Join(outputDir, label+".topic-experiment.json")
	experiment, err := CaptureGoTopicExperiment(
		ctx,
		repoPath,
		revision,
		os.Getenv("REPOMAP_GOPLS_BINARY"),
		client,
		responsePath,
		experimentPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"%s: %d topics, %d declarations, %d selector runs, %d/%d tokens",
		label,
		len(experiment.Shelf.Topics),
		len(experiment.Inventory.Declarations),
		len(experiment.Selections),
		experiment.Provider.InputTokens,
		experiment.Provider.OutputTokens,
	)
}

func TestLiveGoTopicResponseReplay(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_TOPIC_REPO")
	revision := os.Getenv("REPOMAP_TOPIC_REVISION")
	responsePath := os.Getenv("REPOMAP_TOPIC_REPLAY_RESPONSE")
	if repoPath == "" && revision == "" && responsePath == "" {
		t.Skip("set REPOMAP_TOPIC_REPO/REVISION/REPLAY_RESPONSE for a saved response replay")
	}
	if repoPath == "" || revision == "" || responsePath == "" {
		t.Fatal("REPOMAP_TOPIC_REPO/REVISION/REPLAY_RESPONSE must be set together")
	}
	response := readBoundedFile(t, responsePath, goTopicMaxResponseBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	shelf, err := DecodeGoTopicShelf(response, inventory)
	if err != nil {
		t.Fatal(err)
	}
	type selectionReview struct {
		TopicID  string                   `json:"topic_id"`
		Question string                   `json:"question"`
		Error    string                   `json:"error,omitempty"`
		Trace    *GoSourceSelectionTrace  `json:"trace,omitempty"`
		Packet   *GoSourceSelectionPacket `json:"packet,omitempty"`
	}
	reviews := make([]selectionReview, 0, goTopicSelectorRuns)
	for _, spec := range goTopicSelectionSpecs(shelf) {
		var topic GoTopic
		for _, candidate := range shelf.Topics {
			if candidate.ID == spec.topicID {
				topic = candidate
				break
			}
		}
		anchors, err := goTopicAnchorDeclarations(inventory, topic)
		if err != nil {
			t.Fatal(err)
		}
		trace, packet, err := SelectGoAnchoredQuestionSources(
			ctx,
			GoSourceSelectionOptions{
				RepositoryPath:   repoPath,
				ExpectedRevision: revision,
				Question:         spec.question,
				GoplsBinary:      os.Getenv("REPOMAP_GOPLS_BINARY"),
			},
			anchors,
		)
		review := selectionReview{
			TopicID:  spec.topicID,
			Question: spec.question,
		}
		if err != nil {
			review.Error = err.Error()
			t.Logf("%s %q: ERROR %v", spec.topicID, spec.question, err)
		} else {
			review.Trace = &trace
			review.Packet = &packet
			t.Logf(
				"%s %q: %d symbols, %d calls, %d slices",
				spec.topicID,
				spec.question,
				len(trace.SelectedSymbols),
				len(trace.ExactCalls),
				len(packet.SourceSlices),
			)
		}
		reviews = append(reviews, review)
	}
	outputPath := os.Getenv("REPOMAP_TOPIC_REPLAY_OUTPUT")
	if outputPath == "" {
		return
	}
	review := struct {
		Repository GoSelectionRepository `json:"repository"`
		Inventory  GoTopicInventoryStats `json:"inventory_stats"`
		Shelf      GoTopicShelf          `json:"shelf"`
		Selections []selectionReview     `json:"selections"`
	}{
		Repository: inventory.Repository,
		Inventory:  inventory.Stats,
		Shelf:      shelf,
		Selections: reviews,
	}
	encoded, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := writeGoTopicExclusive(outputPath, encoded); err != nil {
		t.Fatal(err)
	}
}

func TestLiveGoTopicShelfAnchors(t *testing.T) {
	repoPath := os.Getenv("REPOMAP_TOPIC_REPO")
	revision := os.Getenv("REPOMAP_TOPIC_REVISION")
	responsePath := os.Getenv("REPOMAP_TOPIC_REPLAY_RESPONSE")
	if repoPath == "" && revision == "" && responsePath == "" {
		t.Skip("set REPOMAP_TOPIC_REPO/REVISION/REPLAY_RESPONSE for saved shelf anchors")
	}
	if repoPath == "" || revision == "" || responsePath == "" {
		t.Fatal("REPOMAP_TOPIC_REPO/REVISION/REPLAY_RESPONSE must be set together")
	}
	inventory, err := BuildGoTopicInventory(repoPath, revision)
	if err != nil {
		t.Fatal(err)
	}
	shelf, err := DecodeGoTopicShelf(
		readBoundedFile(t, responsePath, goTopicMaxResponseBytes),
		inventory,
	)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]GoTopicDeclaration, len(inventory.Declarations))
	for _, declaration := range inventory.Declarations {
		byID[declaration.ID] = declaration
	}
	for _, topic := range shelf.Topics {
		anchors := make([]string, 0, len(topic.SupportSymbolIDs))
		for _, id := range topic.SupportSymbolIDs {
			declaration := byID[id]
			anchors = append(
				anchors,
				fmt.Sprintf("%s=%s:%s", id, declaration.Path, declaration.Name),
			)
		}
		t.Logf("%s %q anchors: %s", topic.ID, topic.Label, strings.Join(anchors, ", "))
	}
}

func makeGoTopicInventoryFixture(t *testing.T) (string, string) {
	t.Helper()
	repoPath := t.TempDir()
	files := map[string]string{
		"root.go": `package sample

type RootType struct{}
const RootValue = 1

func rootOne(value int) int {
	_ = "BODY_SENTINEL"
	return value
}

func rootTwo() {}
`,
		"alpha/alpha.go": `package alpha

var AlphaValue = 1

func alphaOne() {}

type Box[T any] struct{}

func (box *Box[T]) Put(value T) error {
	return nil
}
`,
		"alpha/deep/deep.go": `package deep

func deepOne() {}
func deepTwo() {}
`,
		"alpha/other/other.go": `package other

func otherOne() {}
func otherTwo() {}
`,
		"zeta/zeta.go": `package zeta

func zetaOne() {}
func zetaTwo() {}
`,
		"ignored_test.go": `package sample
func TestOnly() {}
`,
		"generated/generated.go": `// Code generated by fixture. DO NOT EDIT.
package generated
func GeneratedOnly() {}
`,
	}
	names := make([]string, 0, len(files))
	for name, content := range files {
		fullPath := filepath.Join(repoPath, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.FromSlash(name))
	}
	symlinkPath := filepath.Join(repoPath, "alias.go")
	if err := os.Symlink("root.go", symlinkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	names = append(names, "alias.go")
	revision := commitGoSelectionFixture(t, repoPath, names...)
	return repoPath, revision
}

func makeValidGoTopicShelf(inventory GoTopicInventory) GoTopicShelf {
	topics := make([]GoTopic, goTopicMinShelfTopics)
	pairs := make([][2]string, 0, goTopicMinShelfTopics)
	for left := 0; left < len(inventory.Declarations); left++ {
		for right := left + 1; right < len(inventory.Declarations); right++ {
			pairs = append(pairs, [2]string{
				inventory.Declarations[left].ID,
				inventory.Declarations[right].ID,
			})
		}
	}
	for index := range topics {
		topics[index] = GoTopic{
			ID:             fmt.Sprintf("t%d", index+1),
			Label:          fmt.Sprintf("Lifecycle behavior %d", index+1),
			Why:            fmt.Sprintf("This is a distinct bounded investigation choice %d.", index+1),
			HowQuestion:    fmt.Sprintf("How does lifecycle behavior %d reach its effect?", index+1),
			ChangeQuestion: fmt.Sprintf("Where should lifecycle behavior %d be changed safely?", index+1),
			SupportSymbolIDs: []string{
				pairs[index][0],
				pairs[index][1],
			},
		}
	}
	return GoTopicShelf{
		Coverage: goTopicCoverage,
		Topics:   topics,
	}
}
