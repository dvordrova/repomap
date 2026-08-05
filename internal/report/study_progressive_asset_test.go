package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudyProgressiveAssetKeepsOverviewCompactAndDocumentsReadable(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
function snippet(path, symbol, lines) {
  return {
    path, enclosing_symbol: symbol, start_line: 1, end_line: lines.length,
    highlight_ranges: [], content_sha256: "a".repeat(64),
    presentation_sha256: path === "README.mdx" ? "b".repeat(64) : "c".repeat(64),
    lines: lines.map((line, index) => ({ line: index + 1, text: line })),
  };
}
const codeSource = snippet("serve.go", "Server.Run", [
  "func (s *Server) Run() error {", "  return s.ListenAndServe()", "}",
]);
const tick = String.fromCharCode(96);
const documentSource = snippet("README.mdx", "", [
  "import Callout from './Callout'",
  "# Quick start",
  "",
  "Use " + tick + "Server.Run" + tick + " and [the guide](https://example.test/guide).",
	"Keep [unsafe](javascript:evil) as inert text.",
  "- Create a server",
  "- Register a route",
  "1. Build",
  "2. Run",
  "> Saved repository guidance.",
  tick.repeat(3) + "go",
  "go run ./cmd/server",
  tick.repeat(3),
  "<Callout>",
  "Plain <strong>safe</strong> text.",
  "</Callout>",
]);
const reading = {
  label: "Start here",
  symbol: "Server.Run",
  what_to_look_for: "Run() starts the HTTP server.",
  location: { path: "serve.go", line: 2 },
  source: codeSource,
};
const documentReference = {
  label: "README",
  location: { path: "README.mdx" },
  source: documentSource,
};
const report = {
  repo_name: "fixture", user_mechanisms: [], user_sources: [],
  openable_paths: ["README.mdx", "serve.go"], source_ids: {},
  study_map: { brief: {}, shape: [], directions: [] },
};
const window = {
  location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
  __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {},
};
const document = {
  createElement(tag) { return new Element(tag); },
  getElementById(id) {
    if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
    return null;
  },
  querySelectorAll() { return []; },
};
document.documentElement = { lang: "en" };
window.document = document;
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, document, URLSearchParams, Set, Map, AbortController, Promise,
});
const api = window.__REPOMAP_WORKSPACE_TEST__;
function walk(root) {
  const result = [];
  (function visit(node) {
    result.push(node);
    (node.children || []).forEach(visit);
  })(root);
  return result;
}
function text(node) {
  return String(node.textContent || "") + (node.children || []).map(text).join("");
}
const anchorCard = api.renderStudyReadingAnchor(reading, 0);
const anchorNodes = walk(anchorCard);
const readable = api.renderReadableDocument(documentSource);
const readableNodes = walk(readable);
const link = readableNodes.find((node) => String(node.className).includes("rm-readable-document__link"));
const unsafeLink = readableNodes.find((node) => text(node) === "unsafe");
const documentCard = api.renderReadableDocumentCard(
  documentSource,
  documentReference.location,
  documentReference,
);
const rawButton = walk(documentCard).find((node) => text(node) === "Show raw exact source");
const rawLabelBefore = rawButton && rawButton.textContent;
const rawSourceBefore = walk(documentCard).some((node) => node.attributes["data-source-content"] === "true");
rawButton.onclick();
const rawSourceAfter = walk(documentCard).some((node) => node.attributes["data-source-content"] === "true");

process.stdout.write(JSON.stringify({
  anchorText: text(anchorCard),
  anchorHasSourceDOM: anchorNodes.some((node) => node.attributes["data-source-content"] === "true"),
  readableText: text(readable),
  readableTags: readableNodes.map((node) => node.tagName),
  linkTarget: link && link.attributes.title,
	linkHref: link && link.attributes.href,
	unsafeLinkHref: unsafeLink && unsafeLink.attributes.href,
  rawLabelBefore,
  rawLabelAfter: rawButton && rawButton.textContent,
  rawSourceBefore,
  rawSourceAfter,
}));
`
	runnerPath := filepath.Join(t.TempDir(), "study-progressive-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run progressive Study asset: %v\n%s", err, output)
	}
	var got struct {
		AnchorText         string   `json:"anchorText"`
		AnchorHasSourceDOM bool     `json:"anchorHasSourceDOM"`
		ReadableText       string   `json:"readableText"`
		ReadableTags       []string `json:"readableTags"`
		LinkTarget         string   `json:"linkTarget"`
		LinkHref           string   `json:"linkHref"`
		UnsafeLinkHref     string   `json:"unsafeLinkHref"`
		RawLabelBefore     string   `json:"rawLabelBefore"`
		RawLabelAfter      string   `json:"rawLabelAfter"`
		RawSourceBefore    bool     `json:"rawSourceBefore"`
		RawSourceAfter     bool     `json:"rawSourceAfter"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode progressive Study asset: %v\n%s", err, output)
	}
	for _, token := range []string{
		"Run",
		"serve.go:2",
		"Run() starts the HTTP server.",
	} {
		if !strings.Contains(got.AnchorText, token) {
			t.Errorf("compact anchor is missing %q: %q", token, got.AnchorText)
		}
	}
	for _, generic := range []string{"Start here", "Open exact source"} {
		if strings.Contains(got.AnchorText, generic) {
			t.Errorf("compact anchor retained generic copy %q: %q", generic, got.AnchorText)
		}
	}
	if got.AnchorHasSourceDOM || strings.Contains(got.AnchorText, "ListenAndServe") {
		t.Fatalf("compact anchor rendered source before a click: %q", got.AnchorText)
	}
	for _, token := range []string{
		"Quick start",
		"Server.Run",
		"the guide",
		"Create a server",
		"Build",
		"Saved repository guidance.",
		"go run ./cmd/server",
		"Plain safe text.",
	} {
		if !strings.Contains(got.ReadableText, token) {
			t.Errorf("readable document is missing %q: %q", token, got.ReadableText)
		}
	}
	if strings.Contains(got.ReadableText, "import Callout") || strings.Contains(got.ReadableText, "<strong>") ||
		strings.Contains(got.ReadableText, "<Callout>") {
		t.Fatalf("readable document exposed MDX/HTML syntax: %q", got.ReadableText)
	}
	for _, tag := range []string{"h3", "p", "ul", "ol", "blockquote", "pre", "code"} {
		if !containsString(got.ReadableTags, tag) {
			t.Errorf("readable document did not render <%s>: %#v", tag, got.ReadableTags)
		}
	}
	if got.LinkTarget != "https://example.test/guide" || got.LinkHref != got.LinkTarget ||
		got.UnsafeLinkHref != "" {
		t.Fatalf("text-safe links = target %q, href %q, unsafe href %q", got.LinkTarget, got.LinkHref, got.UnsafeLinkHref)
	}
	if got.RawLabelBefore != "Show raw exact source" || got.RawLabelAfter != "Show readable document" ||
		got.RawSourceBefore || !got.RawSourceAfter {
		t.Fatalf("raw source disclosure = %#v", got)
	}
}

func TestStudyProgressiveAssetContract(t *testing.T) {
	t.Parallel()

	script, err := os.ReadFile(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	style, err := os.ReadFile(filepath.Join("templates", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"function renderStudySourceAction(",
		"function renderStudyReadingAnchor(",
		"function renderReadableDocument(",
		"function renderReadableDocumentCard(",
		"'main.show.raw.exact.source'",
		"documentReference && isMarkdownDocumentSource(snippet)",
	} {
		if !strings.Contains(string(script), token) {
			t.Errorf("Study progressive asset is missing %q", token)
		}
	}
	for _, forbidden := range []string{
		"var start = studyStartReference(direction);",
		"rm-study-order-note",
		"rm-study-start-actions",
	} {
		if strings.Contains(string(script), forbidden) {
			t.Errorf("Study detail retains obsolete presentation %q", forbidden)
		}
	}
	for _, token := range []string{
		".rm-study-reading-anchor__copy",
		".rm-study-reading-anchor__open",
		".rm-readable-document",
		".rm-readable-document-card__raw[hidden]",
	} {
		if !strings.Contains(string(style), token) {
			t.Errorf("Study progressive style is missing %q", token)
		}
	}
}
