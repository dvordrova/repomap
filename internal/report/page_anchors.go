package report

import (
	"encoding/json"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageAnchor is one path:line reference on the static page. Href is a
// permalink at the captured revision; Open is the served-mode editor spec
// that report.js posts to /api/open. With neither, the anchor is plain text
// marked NoSource (no remote, a source unavailable at the captured revision,
// or a served path with no openable ID), keyed by its place.
type pageAnchor struct {
	Path     string
	Line     int
	Href     string
	Open     string
	Text     string
	NoSource bool `json:"NoSource,omitempty"`
	// Code is, for a declaration on a static page, the link to all of its
	// lines (#L1250-L1360), where Href names its first (owner, 2026-09-28: a
	// code link covers the whole declaration).
	Code string `json:"Code,omitempty"`
	// key is, for a declaration, its identity (groupindex DeclarationKey:
	// path, line, column, kind and name), set where the declaration is named
	// (subjectDisplay); the link names no column and two declarations
	// written on one line have one link (review, 2026-10-03). Written as
	// "Key" (MarshalJSON).
	key string
	// words marks a declaration subjectDisplay names in words, a callable
	// written inline ("anonymous function in Start"), not by a code name.
	words bool
}

// Key is the identity of the declaration the anchor names, "" when it
// names none; the page's script reads a declaration by it, never by its
// link.
func (anchor pageAnchor) Key() string { return anchor.key }

// notePlace records where the declaration keyed key is written.
func (builder *pageBuilder) notePlace(key string, location *programindex.Location) {
	if key == "" || location == nil {
		return
	}
	if builder.places == nil {
		builder.places = map[string]SceneSource{}
	}
	builder.places[key] = SceneSource{Path: location.Path, Line: location.Line}
}

// MarshalJSON writes an anchor as the page's script reads it, with the
// identity of the declaration it names.
func (anchor pageAnchor) MarshalJSON() ([]byte, error) {
	type plain pageAnchor
	return json.Marshal(struct {
		plain
		Key string `json:"Key,omitempty"`
	}{plain(anchor), anchor.key})
}

// pageLinks builds anchors for one render. Static reports carry one external
// host; served reports carry the manifest source IDs of openable paths.
type pageLinks struct {
	repositoryURL string
	blobPrefix    string
	revision      string
	pathPrefix    string
	sourceIDs     map[string]string
	unavailable   map[string]bool
	// keyed opens every path, keyed by its place (sceneKey).
	keyed bool
}

func newPageLinks(data *ReportData) pageLinks {
	links := pageLinks{sourceIDs: data.SourceIDs, unavailable: make(map[string]bool, len(data.UnavailableSourcePaths)), keyed: data.sceneKeys}
	for _, sourcePath := range data.UnavailableSourcePaths {
		links.unavailable[sourcePath] = true
	}
	switch {
	case data.GitHubSourceLinks != nil:
		links.repositoryURL = data.GitHubSourceLinks.RepositoryURL
		links.blobPrefix = "/blob/"
		links.revision = data.GitHubSourceLinks.Revision
		links.pathPrefix = data.GitHubSourceLinks.PathPrefix
	case data.GitLabSourceLinks != nil:
		links.repositoryURL = data.GitLabSourceLinks.RepositoryURL
		links.blobPrefix = "/-/blob/"
		links.revision = data.GitLabSourceLinks.Revision
		links.pathPrefix = data.GitLabSourceLinks.PathPrefix
	}
	return links
}

func (links pageLinks) static() bool { return links.repositoryURL != "" }

func (links pageLinks) served() bool { return len(links.sourceIDs) > 0 }

// The reviewed root can be a subdirectory of the Git repository. Keep the
// header pointed at that recorded directory, just like its source anchors.
func (links pageLinks) rootURL() string {
	if links.pathPrefix == "" || !links.static() {
		return links.repositoryURL
	}
	href := links.permalink("", 0)
	return strings.Replace(strings.TrimSuffix(href, "/"), links.blobPrefix, strings.Replace(links.blobPrefix, "blob", "tree", 1), 1)
}

func (links pageLinks) anchor(path string, line, column int) pageAnchor {
	anchor := pageAnchor{Path: path, Line: line, Text: path}
	if line > 0 {
		anchor.Text = path + ":" + strconv.Itoa(line)
	}
	if path == "" {
		return anchor
	}
	switch {
	case links.keyed:
		anchor.Open = sceneKey(path, max(line, 0), max(column, 0))
	case links.static() && links.unavailable[path]:
		anchor.NoSource = true
	case links.static():
		anchor.Href = links.permalink(path, line)
	case links.served():
		if _, openable := links.sourceIDs[path]; openable {
			anchor.Open = path + ":" + strconv.Itoa(max(line, 0)) + ":" + strconv.Itoa(max(column, 0))
		} else {
			anchor.NoSource = true
		}
	default:
		// No remote link and no served source: the place stays plain text
		// with its "No source" explanation, and a declaration is keyed by
		// it (declarationKey), its code, explanation and navigation kept
		// (CONSTITUTION; control review, 2026-10-02: a render of a run with
		// no remote had lost every function's reading and Code search).
		anchor.NoSource = true
	}
	return anchor
}

func (links pageLinks) anchorPointer(path string, line, column int) *pageAnchor {
	if path == "" {
		return nil
	}
	anchor := links.anchor(path, line, column)
	return &anchor
}

func (links pageLinks) factAnchor(fact facts.Fact) *pageAnchor {
	if fact.Anchor == nil {
		return nil
	}
	return links.anchorPointer(fact.Anchor.Path, fact.Anchor.Line, fact.Anchor.Column)
}

// permalink mirrors the host blob URL layout: GitHub uses /blob/<rev>/<path>,
// GitLab uses /-/blob/<rev>/<path>; both accept #L<line>.
func (links pageLinks) permalink(path string, line int) string {
	segments := make([]string, 0, 8)
	if links.pathPrefix != "" {
		segments = append(segments, strings.Split(links.pathPrefix, "/")...)
	}
	segments = append(segments, strings.Split(path, "/")...)
	for position := range segments {
		segments[position] = url.PathEscape(segments[position])
	}
	href := links.repositoryURL + links.blobPrefix + url.PathEscape(links.revision) +
		"/" + strings.Join(segments, "/")
	if line > 0 {
		href += "#L" + strconv.Itoa(line)
	}
	return href
}

// rangeLink is the static link to lines line..end of a file, "" when the
// page has no static links or the range is one line: GitHub writes
// #L10-L20, GitLab #L10-20.
func (links pageLinks) rangeLink(path string, line, end int) string {
	if !links.static() || path == "" || line <= 0 || end <= line || links.unavailable[path] {
		return ""
	}
	if links.blobPrefix == "/-/blob/" {
		return links.permalink(path, line) + "-" + strconv.Itoa(end)
	}
	return links.permalink(path, line) + "-L" + strconv.Itoa(end)
}

// factLabel is the short principal of a fact shown next to its anchor.
func factLabel(fact facts.Fact) string {
	switch fact.Kind {
	case facts.KindRegistration:
		return strings.TrimSpace(fact.Method + " " + firstNonEmpty(fact.Path, strings.Join(fact.Values, " "), fact.Key))
	case facts.KindSQLQuery:
		return firstNonEmpty(fact.Key, fact.Value)
	case facts.KindEntrypoint:
		if fact.Symbol != "" {
			return fact.Symbol
		}
		return fact.Key
	case facts.KindManifest, facts.KindDependency:
		if fact.Value != "" {
			return fact.Key + " = " + fact.Value
		}
		return fact.Key
	case facts.KindTODO:
		return fact.Text
	case facts.KindDeadModule, facts.KindImport:
		return fact.Path
	case facts.KindNegative:
		return negativeSentence(fact)
	default:
		return fact.Key
	}
}

var negativeSentences = map[string]string{
	facts.NegativeNoDockerfile: "No Dockerfile or docker-compose file found",
	facts.NegativeNoCI:         "No CI configuration found",
}

// negativeStatements are sentences that say everything their negative
// knows, qualifier included, so the fact's own words do not follow them.
// "No test files found" read as "this repository has no tests": a newcomer
// writing from the Redis report told readers to skip the 204 tests of
// test-redis.tcl, a file no adapter recognizes as a test.
var negativeStatements = map[string]string{
	facts.NegativeNoTests: "No recognized test files found in the inspected paths",
}

// negativeSentence phrases a closed negative as one plain sentence.
func negativeSentence(fact facts.Fact) string {
	if statement, known := negativeStatements[fact.Key]; known {
		return statement + "."
	}
	base, known := negativeSentences[fact.Key]
	if !known {
		base = strings.ReplaceAll(fact.Key, "_", " ")
	}
	if fact.Text != "" {
		return base + " (" + fact.Text + ")."
	}
	return base + "."
}

func shortRevision(revision string) string {
	const visible = 12
	if len(revision) <= visible {
		return revision
	}
	return revision[:visible]
}

// editorOnPath reports whether this machine can open a source link in an
// editor. It names the same command internal/reportserver resolves; the page
// only decides what to promise, and the server decides what actually happens.
func editorOnPath() bool {
	_, err := exec.LookPath("code")
	return err == nil
}
