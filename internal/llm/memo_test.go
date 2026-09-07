package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoStoresValidatedValuesWithoutFictionalExchanges(t *testing.T) {
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	key := strings.Repeat("a", 64)
	value := []byte("{\n  \"value\": \"a < b & c\"\n}")
	if err := SaveMemo(executor, key, value); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadMemo(executor, key, DecodeJSON[testValue](nil))
	if err != nil || !found || got.Value != "a < b & c" {
		t.Fatalf("memo: %+v, found=%v, err=%v", got, found, err)
	}
	raw, _ := os.ReadFile(filepath.Join(executor.RootDir, CacheDirectoryName, "memo-"+key+".json"))
	if strings.Contains(string(raw), "metrics") || strings.Contains(string(raw), "finish_reason") {
		t.Fatal("memo invented a provider exchange")
	}
	executor.Enabled = false
	if _, found, err := LoadMemo(executor, key, DecodeJSON[testValue](nil)); err != nil || found {
		t.Fatalf("disabled cache read memo: %v %v", found, err)
	}
	if err := SaveMemo(executor, strings.Repeat("b", 64), value); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(executor.RootDir, CacheDirectoryName, "memo-*.json"))
	if len(files) != 1 {
		t.Fatal("disabled cache wrote a memo")
	}
}

func TestMemoIdentityChangesWithModelAndSemanticState(t *testing.T) {
	provider := &testProvider{state: []byte(`{"model":"first"}`)}
	first, err := MemoIdentity(provider, []byte(`{"entity":"x","basis":"one"}`), Prompt{User: "row"}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	provider.state = []byte(`{"model":"second"}`)
	second, err := MemoIdentity(provider, []byte(`{"entity":"x","basis":"one"}`), Prompt{User: "row"}, Limits{})
	if err != nil || first == second {
		t.Fatalf("model change reused memo: %v", err)
	}
	third, err := MemoIdentity(provider, []byte(`{"entity":"x","basis":"two"}`), Prompt{User: "row"}, Limits{})
	if err != nil || second == third || provider.completeCalls != 0 {
		t.Fatalf("basis change or preparation made a provider call: %v, calls %d", err, provider.completeCalls)
	}
	english, err := MemoIdentity(provider, []byte(`{"entity":"x","basis":"two"}`), Prompt{User: "row", ResponseLanguage: "en"}, Limits{})
	if err != nil || english != third {
		t.Fatalf("explicit English changed the default memo: %v", err)
	}
	russian, err := MemoIdentity(provider, []byte(`{"entity":"x","basis":"two"}`), Prompt{User: "row", ResponseLanguage: "ru"}, Limits{})
	if err != nil || russian == english || provider.completeCalls != 0 {
		t.Fatalf("translation language reused English memo or made a call: %v", err)
	}
}
