package report

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
)

// pageAnchor is one path:line reference on the static page. Href is a
// permalink at the captured revision; Open is the served-mode editor spec
// that report.js posts to /api/open. With neither, the anchor is plain text.
type pageAnchor struct {
	Path string
	Line int
	Href string
	Open string
	Text string
}

// pageLinks builds anchors for one render. Static reports carry one external
// host; served reports carry the manifest source IDs of openable paths.
type pageLinks struct {
	repositoryURL string
	blobPrefix    string
	revision      string
	pathPrefix    string
	sourceIDs     map[string]string
}

func newPageLinks(data *ReportData) pageLinks {
	links := pageLinks{sourceIDs: data.SourceIDs}
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

func (links pageLinks) anchor(path string, line, column int) pageAnchor {
	anchor := pageAnchor{Path: path, Line: line, Text: path}
	if line > 0 {
		anchor.Text = path + ":" + strconv.Itoa(line)
	}
	if path == "" {
		return anchor
	}
	switch {
	case links.static():
		anchor.Href = links.permalink(path, line)
	case links.served():
		if _, openable := links.sourceIDs[path]; openable {
			anchor.Open = path + ":" + strconv.Itoa(max(line, 0)) + ":" + strconv.Itoa(max(column, 0))
		}
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

// factLabel is the short principal of a fact shown next to its anchor.
func factLabel(fact facts.Fact) string {
	switch fact.Kind {
	case facts.KindHTTPRoute, facts.KindHTTPCall, facts.KindPortal:
		return fact.Method + " " + fact.Path
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
	facts.NegativeNoTests:      "No test files found",
	facts.NegativeNoDockerfile: "No Dockerfile or docker-compose file found",
	facts.NegativeNoCI:         "No CI configuration found",
}

// negativeSentence phrases a closed negative as one plain sentence.
func negativeSentence(fact facts.Fact) string {
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
