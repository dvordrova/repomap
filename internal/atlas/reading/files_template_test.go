package reading

import "testing"

// A template's part the walk could not resolve reads by its plainest word:
// its last field or variable, or a key it is looked up by; a setting's part
// stays; a part giving no word leaves the path not established (owner's
// review, 2026-09-30: litestream's Files list read an internal expression
// and a diagnostic phrase among nine variants of one file).
func TestATemplatesUnresolvedPartReadsByItsPlainestWord(t *testing.T) {
	for template, want := range map[string]string{
		"{Clone[[]*github.com/benbjohnson/litestream.DB *github.com/benbjohnson/litestream.DB]()[?].metaPath}.tmp": "{metaPath}.tmp",
		"{write not established before read.metaPath}.tmp":                                                         "{metaPath}.tmp",
		"{multiple writes.metaPath}.tmp":                                                                           "{metaPath}.tmp",
		"{DBs()[?].metaPath}.tmp":                                                                                  "{metaPath}.tmp",
		"{?.Path}.tmp":                                                                                             "{Path}.tmp",
		"{c.path}.tmp":                                                                                             "{path}.tmp",
		`{config["user_data_dir"]}/data`:                                                                           "{user_data_dir}/data",
		`{strategy_name.split()["0"]}.py`:                                                                          "{strategy_name}.py",
		"strategy_subtemplates/buy_trend_{subtemplate}.j2":                                                         "strategy_subtemplates/buy_trend_{subtemplate}.j2",
		"{--config}/{env:API}.yml":                                                                                 "{--config}/{env:API}.yml",
		"{()}.tmp":                                                                                                 "",
		"subtemplates/exchange_{get()}.j2":                                                                         "",
	} {
		if got := plainTemplate(template); got != want {
			t.Errorf("%s reads %q, want %q", template, got, want)
		}
	}
}
