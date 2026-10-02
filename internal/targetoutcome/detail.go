package targetoutcome

import (
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FailureDetail is a failure's error text as the reader reads it, a
// display relativization of its paths, not a redaction:
//   - a repository root (an absolute form in roots) standing where a path
//     starts reads as nothing before its slash, so its paths read relative,
//     and alone as "."; the JS/TS helper's "<repository>" label likewise;
//   - the home directory reads as "~", so a module cache path keeps its
//     module@version (~/go/pkg/mod/…);
//   - any other absolute path reads as "…/" and its last element;
//   - a tab or another control character reads as a space; line breaks stay.
//
// Every other word stays as the failure wrote it. The result is its own
// FailureDetail, whatever the roots: nothing in it reads as a host path.
func FailureDetail(text, home string, roots ...string) string {
	text = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, text)
	var labels []label
	for _, root := range roots {
		if root = filepath.ToSlash(filepath.Clean(root)); strings.HasPrefix(root, "/") && root != "/" {
			labels = append(labels, label{root, "."})
		}
	}
	labels = append(labels, label{"<repository>", "."})
	if home = filepath.ToSlash(filepath.Clean(home)); strings.HasPrefix(home, "/") && home != "/" {
		labels = append(labels, label{home, "~"})
	}
	// The longest first: a root inside the home directory is the root, and
	// /private/var/… is not read as /var/….
	slices.SortStableFunc(labels, func(a, b label) int { return len(b.from) - len(a.from) })
	// A label written where a slash stood can begin a path the first
	// reading did not see ("(/repo//etc/x)"): it is read again until it
	// reads the same.
	for range 16 {
		next := relabel(text, labels)
		if next == text {
			break
		}
		text = next
	}
	return text
}

// label is what a path prefix reads as: a repository root ".", home "~".
type label struct{ from, to string }

// relabel is one reading of text: each path at a path start relabelled or
// cut to its last element, each line's trailing spaces and the text's
// surrounding space trimmed.
func relabel(text string, labels []label) string {
	var out strings.Builder
	for at := 0; at < len(text); {
		repository := strings.HasPrefix(text[at:], "<repository>") && boundaryBefore(text, at)
		if !pathStart(text, at) && !repository {
			r, size := utf8.DecodeRuneInString(text[at:])
			out.WriteRune(r)
			at += size
			continue
		}
		matched := false
		for _, l := range labels {
			if !strings.HasPrefix(text[at:], l.from) {
				continue
			}
			end := at + len(l.from)
			switch {
			case end < len(text) && text[end] == '/' && l.to == ".":
				// A repository path reads relative: src/a.c.
				at = end + 1
			case end < len(text) && text[end] == '/':
				out.WriteString(l.to)
				at = end
			case end == len(text) || !pathRune(firstRune(text[end:])):
				out.WriteString(l.to)
				at = end
			default:
				continue
			}
			matched = true
			break
		}
		if matched {
			continue
		}
		if text[at] != '/' {
			out.WriteByte(text[at])
			at++
			continue
		}
		end := at + 1
		for end < len(text) {
			r, size := utf8.DecodeRuneInString(text[end:])
			if r != '/' && !pathRune(r) {
				break
			}
			end += size
		}
		out.WriteString("…/" + path.Base(text[at:end]))
		at = end
	}
	lines := strings.Split(out.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// boundaryBefore reports text[at] standing where a word starts: the start,
// a space, a quote or bracket, or one of = , ; :.
func boundaryBefore(text string, at int) bool {
	if at == 0 {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(text[:at])
	return unicode.IsSpace(before) || strings.ContainsRune("'\"`([{=,;:", before)
}

// pathStart reports a slash beginning an absolute path at text[at]: a name
// follows it, and before it is the start, a space, a quote or bracket, one
// of = , ; : or a compiler's -I -L -F. A URL's "//" and a relative path's
// inner slash are not one; neither is "…/" or "~/".
func pathStart(text string, at int) bool {
	if text[at] != '/' || at+1 >= len(text) || !pathRune(firstRune(text[at+1:])) {
		return false
	}
	if boundaryBefore(text, at) {
		return true
	}
	if at >= 2 && text[at-2] == '-' && strings.ContainsRune("ILF", rune(text[at-1])) {
		return true
	}
	// file:///abs: the path after a URL's empty host.
	return strings.HasSuffix(text[:at], "file://")
}

// firstRune is the first character of text.
func firstRune(text string) rune {
	r, _ := utf8.DecodeRuneInString(text)
	return r
}

// pathRune is a character a path element is written with.
func pathRune(r rune) bool {
	return r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.-+@~%#", r)) || r >= utf8.RuneSelf && unicode.IsLetter(r)
}

// ValidFailureDetail reports a display detail: non-empty trimmed text whose
// FailureDetail is itself, so no host path or control character is left.
func ValidFailureDetail(detail string) bool {
	return detail != "" && utf8.ValidString(detail) && FailureDetail(detail, "") == detail
}
