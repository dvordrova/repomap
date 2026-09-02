package claims

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// commitRecord is one non-merge commit reachable from the captured revision.
type commitRecord struct {
	Hash    string
	Date    string
	Subject string
}

// revisionDate is the commit date of the captured revision; ages are
// measured from it so a re-run days later yields the same artifact.
func revisionDate(ctx context.Context, repoPath, revision string) (string, error) {
	out, err := gitOutput(ctx, repoPath, "log", "-n", "1", "--pretty=format:%cI", revision, "--")
	if err != nil {
		return "", err
	}
	date, ok := isoDate(strings.TrimSpace(string(out)))
	if !ok {
		return "", fmt.Errorf("claims: git log returned no date for %s", revision)
	}
	return date, nil
}

func listCommits(ctx context.Context, repoPath, revision string, limit int) ([]commitRecord, error) {
	out, err := gitOutput(ctx, repoPath,
		"log", "-n", strconv.Itoa(limit), "--no-merges", "-z",
		"--date=iso-strict", "--pretty=format:%H%x00%cI%x00%s", revision, "--",
	)
	if err != nil {
		return nil, err
	}
	return parseCommitLog(out)
}

// parseCommitLog reads the flat NUL-separated hash/date/subject triples.
func parseCommitLog(out []byte) ([]commitRecord, error) {
	fields := splitNull(out)
	if len(fields)%3 != 0 {
		return nil, fmt.Errorf("claims: git log returned a malformed commit record")
	}
	records := make([]commitRecord, 0, len(fields)/3)
	for index := 0; index < len(fields); index += 3 {
		hash := fields[index]
		if len(hash) < shortCommitLength || !isHex(hash) {
			return nil, fmt.Errorf("claims: git log returned a malformed commit hash")
		}
		date, ok := isoDate(fields[index+1])
		if !ok {
			return nil, fmt.Errorf("claims: git log returned a malformed commit date")
		}
		subject := cutAtWord(collapseSpace(fields[index+2]))
		if subject == "" {
			continue
		}
		records = append(records, commitRecord{Hash: hash, Date: date, Subject: subject})
	}
	return records, nil
}

// fileCommitDates walks the history once and keeps the newest commit date
// per path, as recorded at that commit.
func fileCommitDates(ctx context.Context, repoPath, revision string) (map[string]string, error) {
	out, err := gitOutput(ctx, repoPath,
		"log", "--no-merges", "-z", "--pretty=format:%x01%cI", "--name-only", revision, "--",
	)
	if err != nil {
		return nil, err
	}
	return parseFileDates(out), nil
}

// parseFileDates reads fields of the form "\x01<date>\n<path>" (commit header
// with its first path) or "<path>"; the walk is newest first.
func parseFileDates(out []byte) map[string]string {
	dates := make(map[string]string)
	current := ""
	for _, field := range splitNull(out) {
		filePath := field
		if strings.HasPrefix(field, "\x01") {
			header, rest, _ := strings.Cut(field[1:], "\n")
			current, _ = isoDate(header)
			filePath = rest
		}
		filePath = strings.TrimSpace(filePath)
		if filePath == "" || current == "" {
			continue
		}
		if _, seen := dates[filePath]; !seen {
			dates[filePath] = current
		}
	}
	return dates
}

func isoDate(value string) (string, bool) {
	if len(value) < isoDateLength || !validDate(value[:isoDateLength]) {
		return "", false
	}
	return value[:isoDateLength], true
}

func isHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func splitNull(data []byte) []string {
	data = bytes.TrimSuffix(data, []byte{0})
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{0})
	fields := make([]string, len(parts))
	for index, part := range parts {
		fields[index] = string(part)
	}
	return fields
}

// gitOutput mirrors the isolation used for the corpus listing: no pager,
// no hooks, no caller-supplied GIT_* redirection of repository or config.
func gitOutput(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	cmd := exec.CommandContext(ctx, "git", append(commandArgs, args...)...)
	cmd.Env = isolatedEnvironment(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("claims: git %s failed: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("claims: git %s failed: %w", args[0], err)
	}
	return out, nil
}

func isolatedEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+6)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
}
