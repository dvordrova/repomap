package report

import (
	"path"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// programTitle is what a program is titled by when its directory does not
// say it: a script by its file (ProgramTarget ScriptFile), titled
// build_helpers/create_command_partials.py, not by its dotted module name
// or its directory ("etc" for litestream's etc/s3_mock.py); a program its
// build names once, by that name where its directory ends otherwise
// (freqtrade's client, `freqtrade-client = freqtrade_client.ft_client:main`,
// read ft_client/freqtrade_client). A program whose directory ends in its
// name (cmd/litestream, freqtrade) keeps the directory. Empty when neither
// applies.
func programTitle(target programindex.Target, root string) string {
	if script := target.ScriptFile(); script != "" {
		return script
	}
	if len(target.Executables) != 1 {
		return ""
	}
	name := target.Executables[0]
	if root := strings.Trim(root, "/"); root != "" && root != "." && path.Base(root) == name {
		return ""
	}
	return name
}

// scriptPartTitle is the title a part of a script program reads by: the
// reading names the one part of a program of one unit by the program's name
// (atlas design, "the one part takes the target's name"), and the page names
// a script program by its file, so that part reads the same. Every other
// part keeps its title.
func scriptPartTitle(target programindex.Target, title string) string {
	if script := target.ScriptFile(); script != "" && title == target.Name {
		return script
	}
	return title
}

// scriptGroups names the one part of a script program by its file
// (scriptPartTitle), for the page's view of the group graph.
func scriptGroups(index groupindex.Index, groups []groupindex.Group) {
	for i := range groups {
		groups[i].Title = scriptPartTitle(index.Target, groups[i].Title)
	}
}

// titledLabels labels the sections a program title names (programTitle),
// when no other section takes the same title; the others keep the
// directory rule of labelSections.
func titledLabels(sections []*pageSection) map[*pageSection]bool {
	count := make(map[string]int, len(sections))
	for _, section := range sections {
		if section.title != "" {
			count[section.title]++
		}
	}
	titled := make(map[*pageSection]bool)
	for _, section := range sections {
		if section.title != "" && count[section.title] == 1 {
			section.ShortLabel, section.Label = section.title, section.title
			titled[section] = true
		}
	}
	return titled
}
