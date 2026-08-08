package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)


func TestArchitectureAssetKeepsOpaqueEnumsAndModelLabelsExact(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");

function inspect(locale) {
  const document = { documentElement: { lang: locale } };
  const window = { document, __REPOMAP_LAYOUT_TEST__: {} };
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("architecture_canvas.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
  const api = window.__REPOMAP_LAYOUT_TEST__;
  return {
    unknownProofArea: api.proofAreaLabel("future_proof_area", window.RepomapUI.message),
    knownProofArea: api.proofAreaLabel("application_callable", window.RepomapUI.message),
    modelLabel: api.presentationValueLabel({ label: "model_label_with_underscores" }),
    enumValue: api.presentationValueLabel("behavior_grounded"),
  };
}

process.stdout.write(JSON.stringify({ en: inspect("en"), ru: inspect("ru") }));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-opaque-enum-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run architecture exact enum renderer: %v\n%s", err, output)
	}
	var got map[string]struct {
		UnknownProofArea string `json:"unknownProofArea"`
		KnownProofArea   string `json:"knownProofArea"`
		ModelLabel       string `json:"modelLabel"`
		EnumValue        string `json:"enumValue"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode architecture exact enum renderer: %v\n%s", err, output)
	}
	for _, locale := range []string{"en", "ru"} {
		result := got[locale]
		if result.UnknownProofArea != "future_proof_area" ||
			result.ModelLabel != "model_label_with_underscores" ||
			result.EnumValue != "behavior_grounded" {
			t.Errorf("%s architecture identities = %#v", locale, result)
		}
	}
	if got["en"].KnownProofArea != "Application callable" ||
		got["ru"].KnownProofArea != "Вызываемый объект приложения" {
		t.Errorf("known proof-area product copy did not use typed catalog: en=%q ru=%q",
			got["en"].KnownProofArea, got["ru"].KnownProofArea)
	}

	for _, name := range []string{"script.js", "architecture_canvas.js"} {
		asset := readCanvasAsset(t, name)
		for _, forbidden := range []string{
			`.replaceAll("_", " ")`, `.replaceAll('_', ' ')`,
			`.replace(/_/g, " ")`, `.replace(/_/g, ' ')`,
			`.replaceAll("_", "/")`, `.replaceAll('_', '/')`,
		} {
			if strings.Contains(asset, forbidden) {
				t.Errorf("%s still mutates underscore-bearing identity with %s", name, forbidden)
			}
		}
	}
}
