package report

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	errReportGraphJSONUnavailable = errors.New("workspace graph: saved Go facts are unavailable")
	errReportGraphJSONBounds      = errors.New("workspace graph: saved Go facts exceed bounds")
)

type savedGraphJSONSpan struct {
	start int
	end   int
}

type savedGraphJSONBudget struct {
	scalarBytes  int
	files        int
	declarations int
}

// preflightSnapshotExactGoFacts locates the existing go_facts value without
// copying it and bounds every collection and scalar consumed by the neutral
// package-graph adapter before encoding/json may allocate typed results.
func preflightSnapshotExactGoFacts(data []byte) (savedGraphJSONSpan, error) {
	var (
		found bool
		span  savedGraphJSONSpan
	)
	budget := savedGraphJSONBudget{scalarBytes: maxReportGraphAggregateScalars}
	start := skipSavedGraphJSONSpace(data, 0)
	end, err := walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			field, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				false,
				"go_facts",
			)
			if err != nil {
				return 0, err
			}
			if field == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			if found {
				return 0, errReportGraphJSONUnavailable
			}
			found = true
			valueEnd, err := preflightSavedGraphGoFacts(data, valueStart, &budget)
			if err != nil {
				return 0, err
			}
			span = savedGraphJSONSpan{start: valueStart, end: valueEnd}
			return valueEnd, nil
		},
	)
	if err != nil || skipSavedGraphJSONSpace(data, end) != len(data) || !found {
		if errors.Is(err, errReportGraphJSONBounds) {
			return savedGraphJSONSpan{}, err
		}
		return savedGraphJSONSpan{}, errReportGraphJSONUnavailable
	}
	return span, nil
}

func preflightSavedGraphGoFacts(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	var seen uint8
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			field, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"modules",
				"packages",
				"internal_edges",
			)
			if err != nil {
				return 0, err
			}
			switch field {
			case 1:
				if seen&1 != 0 {
					return 0, errReportGraphJSONUnavailable
				}
				seen |= 1
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					maxReportGraphFactModules,
					func(itemStart int) (int, error) {
						return preflightSavedGraphModule(data, itemStart, budget)
					},
				)
			case 2:
				if seen&2 != 0 {
					return 0, errReportGraphJSONUnavailable
				}
				seen |= 2
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					maxReportGraphFactPackages,
					func(itemStart int) (int, error) {
						return preflightSavedGraphPackage(data, itemStart, budget)
					},
				)
			case 3:
				if seen&4 != 0 {
					return 0, errReportGraphJSONUnavailable
				}
				seen |= 4
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					maxReportGraphFactEdges,
					func(itemStart int) (int, error) {
						return preflightSavedGraphEdge(data, itemStart, budget)
					},
				)
			default:
				return skipSavedGraphJSONValue(data, valueStart)
			}
		},
	)
}

func preflightSavedGraphModule(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	var seen uint8
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			fieldIndex, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"id",
				"module_path",
				"module_dir",
				"go_mod",
				"main",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint8(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportGraphJSONUnavailable
			}
			seen |= field
			if field == 16 {
				return preflightSavedGraphJSONBool(data, valueStart)
			}
			return preflightSavedGraphJSONString(data, valueStart, budget)
		},
	)
}

func preflightSavedGraphPackage(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	var seen uint16
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			fieldIndex, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"canonical_package_path",
				"name",
				"owning_module_id",
				"module_path",
				"package_directory",
				"module_relative_path",
				"files",
				"declarations",
				"declarations_scanned",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint16(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportGraphJSONUnavailable
			}
			seen |= field
			if field <= 32 {
				return preflightSavedGraphJSONString(data, valueStart, budget)
			}
			switch field {
			case 64:
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					maxReportGraphFilesPerPackage,
					func(itemStart int) (int, error) {
						if budget.files >= maxReportGraphAggregateFiles {
							return 0, errReportGraphJSONBounds
						}
						budget.files++
						return preflightSavedGraphJSONString(data, itemStart, budget)
					},
				)
			case 128:
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					maxReportGraphDeclarations,
					func(itemStart int) (int, error) {
						if budget.declarations >= maxReportGraphDeclarations {
							return 0, errReportGraphJSONBounds
						}
						budget.declarations++
						return preflightSavedGraphDeclaration(data, itemStart, budget)
					},
				)
			case 256:
				return preflightSavedGraphJSONBool(data, valueStart)
			default:
				return 0, errReportGraphJSONUnavailable
			}
		},
	)
}

func preflightSavedGraphDeclaration(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	var seen uint8
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			fieldIndex, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"kind",
				"name",
				"receiver",
				"path",
				"line",
				"column",
				"executable_body",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint8(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportGraphJSONUnavailable
			}
			seen |= field
			if field <= 8 {
				return preflightSavedGraphJSONString(data, valueStart, budget)
			}
			if field <= 32 {
				return preflightSavedGraphJSONNonnegativeInt(data, valueStart)
			}
			return preflightSavedGraphJSONBool(data, valueStart)
		},
	)
}

func preflightSavedGraphEdge(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	var seen uint8
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			fieldIndex, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"from",
				"to",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint8(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportGraphJSONUnavailable
			}
			seen |= field
			return preflightSavedGraphJSONString(data, valueStart, budget)
		},
	)
}

func preflightSavedGraphJSONString(
	data []byte,
	start int,
	budget *savedGraphJSONBudget,
) (int, error) {
	end, size, err := scanSavedGraphJSONString(data, start, maxReportGraphScalarBytes)
	if err != nil {
		return 0, err
	}
	if size > budget.scalarBytes {
		return 0, errReportGraphJSONBounds
	}
	budget.scalarBytes -= size
	return end, nil
}

func preflightSavedGraphJSONBool(data []byte, start int) (int, error) {
	for _, literal := range []string{"true", "false", "null"} {
		if savedGraphJSONLiteralAt(data, start, literal) {
			return start + len(literal), nil
		}
	}
	return 0, errReportGraphJSONUnavailable
}

func preflightSavedGraphJSONNonnegativeInt(data []byte, start int) (int, error) {
	if start >= len(data) || data[start] < '0' || data[start] > '9' {
		return 0, errReportGraphJSONUnavailable
	}
	index := start
	for index < len(data) && data[index] >= '0' && data[index] <= '9' {
		if index-start >= 10 {
			return 0, errReportGraphJSONBounds
		}
		index++
	}
	if index-start > 1 && data[start] == '0' {
		return 0, errReportGraphJSONUnavailable
	}
	return index, nil
}

func walkSavedGraphJSONObject(
	data []byte,
	start int,
	visit func(keyStart, keyEnd, valueStart int) (int, error),
) (int, error) {
	start = skipSavedGraphJSONSpace(data, start)
	if start >= len(data) || data[start] != '{' {
		return 0, errReportGraphJSONUnavailable
	}
	index := skipSavedGraphJSONSpace(data, start+1)
	if index < len(data) && data[index] == '}' {
		return index + 1, nil
	}
	for {
		keyStart, keyEnd, next, err := scanSavedGraphJSONKey(data, index)
		if err != nil {
			return 0, err
		}
		index = skipSavedGraphJSONSpace(data, next)
		if index >= len(data) || data[index] != ':' {
			return 0, errReportGraphJSONUnavailable
		}
		valueStart := skipSavedGraphJSONSpace(data, index+1)
		valueEnd, err := visit(keyStart, keyEnd, valueStart)
		if err != nil {
			return 0, err
		}
		index = skipSavedGraphJSONSpace(data, valueEnd)
		if index >= len(data) {
			return 0, errReportGraphJSONUnavailable
		}
		switch data[index] {
		case '}':
			return index + 1, nil
		case ',':
			index = skipSavedGraphJSONSpace(data, index+1)
		default:
			return 0, errReportGraphJSONUnavailable
		}
	}
}

func walkSavedGraphJSONArray(
	data []byte,
	start,
	limit int,
	visit func(itemStart int) (int, error),
) (int, error) {
	start = skipSavedGraphJSONSpace(data, start)
	if savedGraphJSONLiteralAt(data, start, "null") {
		return start + len("null"), nil
	}
	if start >= len(data) || data[start] != '[' {
		return 0, errReportGraphJSONUnavailable
	}
	index := skipSavedGraphJSONSpace(data, start+1)
	if index < len(data) && data[index] == ']' {
		return index + 1, nil
	}
	count := 0
	for {
		if count >= limit {
			return 0, errReportGraphJSONBounds
		}
		itemEnd, err := visit(index)
		if err != nil {
			return 0, err
		}
		count++
		index = skipSavedGraphJSONSpace(data, itemEnd)
		if index >= len(data) {
			return 0, errReportGraphJSONUnavailable
		}
		switch data[index] {
		case ']':
			return index + 1, nil
		case ',':
			index = skipSavedGraphJSONSpace(data, index+1)
		default:
			return 0, errReportGraphJSONUnavailable
		}
	}
}

func scanSavedGraphJSONKey(data []byte, start int) (int, int, int, error) {
	end, _, err := scanSavedGraphJSONString(data, start, maxReportGraphScalarBytes)
	if err != nil {
		return 0, 0, 0, err
	}
	return start + 1, end - 1, end, nil
}

func scanSavedGraphJSONString(data []byte, start, limit int) (int, int, error) {
	if start >= len(data) || data[start] != '"' {
		return 0, 0, errReportGraphJSONUnavailable
	}
	size := 0
	for index := start + 1; index < len(data); {
		switch data[index] {
		case '"':
			return index + 1, size, nil
		case '\\':
			if index+1 >= len(data) {
				return 0, 0, errReportGraphJSONUnavailable
			}
			size += 2
			index += 2
		default:
			if data[index] < 0x20 {
				return 0, 0, errReportGraphJSONUnavailable
			}
			runeSize := 1
			if data[index] >= utf8.RuneSelf {
				_, runeSize = utf8.DecodeRune(data[index:])
				if runeSize == 1 {
					return 0, 0, errReportGraphJSONUnavailable
				}
			}
			size += runeSize
			index += runeSize
		}
		if limit >= 0 && size > limit {
			return 0, 0, errReportGraphJSONBounds
		}
	}
	return 0, 0, errReportGraphJSONUnavailable
}

// skipSavedGraphJSONValue performs only structural scanning. It retains no
// caller-controlled strings or collection-sized state.
func skipSavedGraphJSONValue(data []byte, start int) (int, error) {
	start = skipSavedGraphJSONSpace(data, start)
	if start >= len(data) {
		return 0, errReportGraphJSONUnavailable
	}
	if data[start] == '"' {
		end, _, err := scanSavedGraphJSONString(data, start, -1)
		return end, err
	}
	if data[start] != '{' && data[start] != '[' {
		index := start
		for index < len(data) {
			switch data[index] {
			case ' ', '\t', '\r', '\n', ',', '}', ']':
				if index == start {
					return 0, errReportGraphJSONUnavailable
				}
				return index, nil
			default:
				index++
			}
		}
		if index == start {
			return 0, errReportGraphJSONUnavailable
		}
		return index, nil
	}

	depth := 1
	for index := start + 1; index < len(data); {
		switch data[index] {
		case '"':
			end, _, err := scanSavedGraphJSONString(data, index, -1)
			if err != nil {
				return 0, err
			}
			index = end
		case '{', '[':
			depth++
			index++
		case '}', ']':
			depth--
			index++
			if depth == 0 {
				return index, nil
			}
		default:
			index++
		}
	}
	return 0, errReportGraphJSONUnavailable
}

func skipSavedGraphJSONSpace(data []byte, start int) int {
	for start < len(data) {
		switch data[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			return start
		}
	}
	return start
}

func savedGraphJSONKeyEqual(data []byte, start, end int, want string) bool {
	if end-start != len(want) {
		return false
	}
	for index := range want {
		if data[start+index] != want[index] {
			return false
		}
	}
	return true
}

// matchSavedGraphJSONKey returns a one-based canonical field index. Known
// aliases are rejected before encoding/json can apply its unescaping and
// case-folding rules. Within the consumed go_facts subtree, any escaped object
// key fails closed; unknown ordinary keys retain their legacy skip behavior.
func matchSavedGraphJSONKey(
	data []byte,
	start,
	end int,
	rejectEscaped bool,
	wants ...string,
) (int, error) {
	for index, want := range wants {
		if savedGraphJSONKeyEqual(data, start, end, want) {
			return index + 1, nil
		}
	}

	escaped := false
	for index := start; index < end; index++ {
		if data[index] == '\\' {
			escaped = true
			break
		}
	}
	if escaped {
		if rejectEscaped || start == 0 || end >= len(data) {
			return 0, errReportGraphJSONUnavailable
		}
		key, err := strconv.Unquote(string(data[start-1 : end+1]))
		if err != nil {
			return 0, errReportGraphJSONUnavailable
		}
		for _, want := range wants {
			if strings.EqualFold(key, want) {
				return 0, errReportGraphJSONUnavailable
			}
		}
		return 0, nil
	}

	key := string(data[start:end])
	for _, want := range wants {
		if strings.EqualFold(key, want) {
			return 0, errReportGraphJSONUnavailable
		}
	}
	return 0, nil
}

func savedGraphJSONLiteralAt(data []byte, start int, literal string) bool {
	if start < 0 || len(data)-start < len(literal) {
		return false
	}
	for index := range literal {
		if data[start+index] != literal[index] {
			return false
		}
	}
	end := start + len(literal)
	if end == len(data) {
		return true
	}
	switch data[end] {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}
