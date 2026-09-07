package report

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestRussianUIExistsBeforeBrowserScripts(t *testing.T) {
	t.Parallel()
	data := reportProgramShellDataFixture(t, "fixture")
	options := reportSingleTargetRenderOptionsFixture(t, &data)
	options.Language, options.NoModel = Russian, true
	rendered, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}
	static, scripts, ok := strings.Cut(string(rendered), `<script type="application/json" id="rm-ui-vocabulary">`)
	if !ok {
		t.Fatal("report has no UI vocabulary for browser labels")
	}
	for _, want := range []string{`<html lang="ru">`, `aria-label="Репозиторий"`, `>Главная</a>`, `>Компоненты · `, `>Об этом запуске</h2>`, "JavaScript необязателен."} {
		if !strings.Contains(static, want) {
			t.Fatalf("static Russian HTML omits %q", want)
		}
	}
	for _, unwanted := range []string{`>Home</a>`, `>Components · `, `>About this run</h2>`, "JavaScript is optional here."} {
		if strings.Contains(static, unwanted) {
			t.Fatalf("static Russian HTML keeps untranslated UI %q", unwanted)
		}
	}
	encoded, _, ok := strings.Cut(scripts, "</script>")
	if !ok {
		t.Fatal("UI vocabulary script is incomplete")
	}
	var embedded map[string]string
	if err := json.Unmarshal([]byte(encoded), &embedded); err != nil {
		t.Fatal(err)
	}
	want, err := uiVocabulary(Russian)
	if err != nil || !reflect.DeepEqual(embedded, want) {
		t.Fatal("browser UI uses a different vocabulary from static HTML")
	}
}

func TestUIVocabularyKeepsEveryParameter(t *testing.T) {
	t.Parallel()
	for key, translated := range russianUI {
		original := uiParameter.FindAllString(key, -1)
		localized := uiParameter.FindAllString(translated, -1)
		sort.Strings(original)
		sort.Strings(localized)
		if !reflect.DeepEqual(original, localized) {
			t.Errorf("UI message %q changes parameters: %v -> %v", key, original, localized)
		}
		if strings.TrimSpace(translated) == "" {
			t.Errorf("UI message %q has an empty translation", key)
		}
	}
	for _, language := range []DisplayLanguage{English, Russian} {
		vocabulary, err := uiVocabulary(language)
		if err != nil {
			t.Fatal(err)
		}
		for key := range russianUI {
			if vocabulary[key] == "" {
				t.Errorf("%s vocabulary omits %q", language, key)
			}
		}
	}
}

// Role filters use the existing repository bands as identities. Translation
// changes their visible titles without changing URL or membership keys.
func TestRepositoryRoleFiltersKeepTheirKeysWhenTranslated(t *testing.T) {
	t.Parallel()
	roles := []string{"Applications", "Libraries", "Tools", "Examples", "Tests and fixtures"}
	view := &pageView{RepoMap: &pageRepoMap{}}
	for _, role := range roles {
		view.RepoMap.Lanes = append(view.RepoMap.Lanes, pageRepoLane{Title: role, Role: role})
		view.RepoMap.Nodes = append(view.RepoMap.Nodes, pageRepoNode{Role: role, NativeName: role, Root: role})
	}
	view.RepoMap.Nodes = append(view.RepoMap.Nodes, pageRepoNode{Note: "Unavailable"})
	view.RepoMap.Edges = []pageRepoEdge{{From: "a", To: "b", Label: "3 inferred integrations · 2 callback bindings · 4 imports · 5 calls"}}
	page := &PreparedPage{view: view}
	if err := page.applyUI(Russian); err != nil {
		t.Fatal(err)
	}
	for i, role := range roles {
		lane, node := view.RepoMap.Lanes[i], view.RepoMap.Nodes[i]
		if lane.Title != russianUI[role] || lane.Role != role || node.Role != role || node.NativeName != role || node.Root != role {
			t.Fatalf("translation changed %q membership: lane %+v, node role %q", role, lane, node.Role)
		}
	}
	if got := view.RepoMap.Nodes[len(roles)].Role; got != "" {
		t.Fatalf("unread component acquired an invented role %q", got)
	}
	if edge := view.RepoMap.Edges[0]; edge.From != "a" || edge.To != "b" || edge.Label != "Предполагаемых интеграций: 3 · Привязок обратных вызовов: 2 · Импортов: 4 · Вызовов: 5" {
		t.Fatalf("counted UI edge label or endpoints lost: %+v", edge)
	}
}

func TestUIParametersAreNotTranslatedOrReplacedAgain(t *testing.T) {
	t.Parallel()
	name := `Root.{1}<http.Handler>&/file.go`
	got, err := uiText(Russian, "Opened {0} in your editor.", name)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Источник "+name+" открыт в редакторе." {
		t.Fatalf("literal parameter changed: %q", got)
	}
	if got := englishUI(Russian, name); got != name {
		t.Fatalf("unknown native text changed: %q", got)
	}
	for _, params := range [][]any{nil, {"one", "extra"}} {
		if _, err := uiText(Russian, "Opened {0} in your editor.", params...); err == nil {
			t.Fatalf("accepted mismatched parameters: %#v", params)
		}
	}
	if _, err := uiText(Russian, "not in the catalogue"); err == nil {
		t.Fatal("accepted an unknown UI message")
	}
	for _, count := range []int{1, 2, 5, 11, 21} {
		got, err := uiText(Russian, "{0} symbols", count)
		if err != nil || got != "Символов: "+strconv.Itoa(count) {
			t.Fatalf("Russian count %d: %q, %v", count, got, err)
		}
	}
	if got, err := uiText(English, "{0} symbols", 1); err != nil || got != "1 symbol" {
		t.Fatalf("English singular: %q, %v", got, err)
	}
}

// This is the cross-file contract: every static template or runtime message
// must exist in the shared catalogue before a less common UI branch is opened.
func TestUIVocabularyCoversTemplateAndRuntimeMessages(t *testing.T) {
	t.Parallel()
	message := regexp.MustCompile(`\b(?:t\s+|rmT(?:\.html)?\()("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
	err := fs.WalkDir(reportTemplateFS, "templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".html") && !strings.HasSuffix(path, ".js")) || strings.HasSuffix(path, "/27-elk.js") {
			return nil
		}
		content, err := fs.ReadFile(reportTemplateFS, path)
		if err != nil {
			return err
		}
		for _, match := range message.FindAllSubmatch(content, -1) {
			literal := string(match[1])
			if literal[0] == '\'' {
				literal = `"` + strings.ReplaceAll(strings.ReplaceAll(literal[1:len(literal)-1], `\'`, `'`), `"`, `\"`) + `"`
			}
			key, err := strconv.Unquote(literal)
			if err != nil {
				t.Errorf("%s: invalid UI key %s: %v", path, literal, err)
				continue
			}
			if _, known := russianUI[key]; !known {
				t.Errorf("%s: unknown UI key %q", path, key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
