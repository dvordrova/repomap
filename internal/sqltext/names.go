package sqltext

import "strings"

// ObjectName reads the object name written at index: a possibly qualified
// name whose parts may be pieced together from identifiers, template holes
// ({name}, ${name}) and printf verbs (%s, %[1]s, %(name)s) with no space
// between them, as in "partitions.manifests_p_%d". A name with a hole is
// supplied at run time: it proves that a name is there, but it has no written
// text, so name is "" and runtime is true. end is the index after the name.
func ObjectName(tokens []Token, index int) (name string, end int, runtime bool) {
	var parts []string
	end = index
	for {
		start := end
		if len(parts) > 0 {
			if !sqlPunctuation(tokens, end, ".") {
				break
			}
			start = end + 1
		}
		text, next, hole := namePart(tokens, start)
		if next == start {
			break
		}
		parts = append(parts, text)
		runtime = runtime || hole
		end = next
	}
	if len(parts) == 0 || runtime {
		return "", end, len(parts) > 0
	}
	return strings.Join(parts, "."), end, false
}

// namePart reads one unqualified name made of adjacent pieces.
func namePart(tokens []Token, index int) (string, int, bool) {
	text := ""
	hole := false
	end := index
	for end < len(tokens) {
		if end > index && !adjacent(tokens, end) {
			break
		}
		if next, ok := sourceHole(tokens, end); ok {
			hole = true
			end = next
			continue
		}
		piece, _ := Identifier(tokens[end:end+1], 0)
		if piece == "" {
			break
		}
		if tokens[end].Quoted && quotedHole(tokens[end].Text) {
			hole = true
		}
		text += piece
		end++
	}
	return text, end, hole
}

// sourceHole recognizes a value the source fills in before the text reaches a
// database: a template hole token or a printf verb. The verb is a "%" followed
// directly by an optional argument index ([1] or (name)) and a verb letter; a
// "%" with a space after it is the modulo operator.
func sourceHole(tokens []Token, index int) (int, bool) {
	if index < 0 || index >= len(tokens) || tokens[index].Quoted || tokens[index].Literal {
		return index, false
	}
	text := tokens[index].Text
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "${") {
		return index + 1, true
	}
	if text != "%" {
		return index, false
	}
	j := index + 1
	switch {
	case sqlPunctuation(tokens, j, "(") && adjacent(tokens, j) && sqlPunctuation(tokens, j+2, ")"):
		j += 3
	case j < len(tokens) && tokens[j].Quoted && adjacent(tokens, j) && digits(tokens[j].Text):
		j++
	}
	if j >= len(tokens) || !adjacent(tokens, j) || tokens[j].Quoted || tokens[j].Literal {
		return index, false
	}
	if r := tokens[j].Text[0]; !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z') {
		return index, false
	}
	return j + 1, true
}

// quotedHole reports a hole inside a quoted identifier ("%v", "{table}").
func quotedHole(text string) bool {
	tokens, _ := Tokens(text, 1)
	for i := range tokens {
		if _, ok := sourceHole(tokens, i); ok {
			return true
		}
	}
	return false
}

// adjacent reports that the token at index starts where the previous one ends.
func adjacent(tokens []Token, index int) bool {
	if index <= 0 || index >= len(tokens) {
		return false
	}
	previous := tokens[index-1]
	end := previous.Offset + len(previous.Text)
	if previous.Quoted {
		end += 2
	}
	return tokens[index].Offset == end
}

func digits(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return text != ""
}
