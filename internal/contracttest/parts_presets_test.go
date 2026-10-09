package contracttest

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// sourcePartName puts each file in a part of its own, named by its path.
func sourcePartName(file string) string { return file }

func echoPartName(file string) string {
	switch {
	case strings.Contains(file, "/handler/"):
		return "Request handling"
	case strings.Contains(file, "/database/"):
		return "Stored users"
	default:
		return "Program setup"
	}
}

// presetGroupsAnswer answers the grouping's proposal: the smaller boxes of
// a box are the distinct names nameOf gives its files, so a box whose files
// share one name is read as it is. handled is false for any other request.
// These presets test native reading contracts, not architectural model
// quality.
func presetGroupsAnswer(raw []byte, nameOf func(string) string) (any, bool, error) {
	var request struct {
		Task  string `json:"task"`
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
		Directories []struct {
			Dir   string `json:"dir"`
			Files []struct {
				Name string `json:"name"`
			} `json:"files"`
		} `json:"directories"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, false, err
	}
	if request.Task != "repomap.atlas.group_propose.v1" {
		return nil, false, nil
	}
	var paths []string
	for _, file := range request.Files {
		paths = append(paths, file.Path)
	}
	for _, dir := range request.Directories {
		for _, file := range dir.Files {
			paths = append(paths, path.Join(dir.Dir, file.Name))
		}
	}
	if len(paths) == 0 {
		return nil, true, fmt.Errorf("parts preset: a proposal without files")
	}
	var names []string
	var boxes []map[string]string
	for _, file := range paths {
		if name := nameOf(file); !slices.Contains(names, name) {
			names = append(names, name)
			boxes = append(boxes, map[string]string{"name": name, "holds": "Preset responsibility: " + name + "."})
		}
	}
	return map[string]any{"boxes": boxes}, true, nil
}

// presetGrouping answers the grouping's closed questions: every box asked
// about needs smaller boxes, and each declaration goes in the box nameOf
// names by its file. ok is false for any other column.
func presetGrouping(column string, question llm.Question, nameOf func(string) string) (llm.Verdict, bool) {
	switch column {
	case "grouping":
		return typesafetest.Choose(lines.GroupNeedsSmaller), true
	case "box":
		file, _ := question.Item["file"].(string)
		want := nameOf(file)
		for _, option := range question.Options {
			if option.Name == want {
				return typesafetest.Choose(want), true
			}
		}
	}
	return llm.Verdict{}, false
}
