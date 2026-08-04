package report

import (
	"errors"

	"github.com/dvordrova/repomap/internal/workspaceentrypoint"
)

var (
	errReportEntrypointJSONUnavailable = errors.New("workspace entrypoint index: saved Go facts are unavailable")
	errReportEntrypointJSONBounds      = errors.New("workspace entrypoint index: saved Go facts exceed bounds")
)

type savedEntrypointJSONBudget struct {
	scalarBytes int
	anchors     int
}

// preflightSnapshotExactEntrypoints locates and bounds only the existing
// top-level go_facts.entrypoint_packages collection. It retains no other
// go_facts subtree and completes before encoding/json allocates typed rows.
func preflightSnapshotExactEntrypoints(data []byte) (savedGraphJSONSpan, error) {
	var (
		foundGoFacts bool
		foundEntries bool
		span         savedGraphJSONSpan
	)
	budget := savedEntrypointJSONBudget{
		scalarBytes: workspaceEntrypointAggregateScalarBytes,
	}
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
			if foundGoFacts {
				return 0, errReportEntrypointJSONUnavailable
			}
			foundGoFacts = true
			return preflightSavedEntrypointGoFacts(
				data,
				valueStart,
				&budget,
				&foundEntries,
				&span,
			)
		},
	)
	if err != nil {
		return savedGraphJSONSpan{}, normalizeReportEntrypointJSONError(err)
	}
	if skipSavedGraphJSONSpace(data, end) != len(data) ||
		!foundGoFacts ||
		!foundEntries {
		return savedGraphJSONSpan{}, errReportEntrypointJSONUnavailable
	}
	return span, nil
}

const (
	workspaceEntrypointScalarBytes          = workspaceentrypoint.MaxScalarBytes
	workspaceEntrypointAggregateScalarBytes = workspaceentrypoint.MaxAggregateScalarBytes
	workspaceEntrypointRawRows              = workspaceentrypoint.MaxRawRows
)

func preflightSavedEntrypointGoFacts(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
	found *bool,
	span *savedGraphJSONSpan,
) (int, error) {
	return walkSavedGraphJSONObject(
		data,
		start,
		func(keyStart, keyEnd, valueStart int) (int, error) {
			field, err := matchSavedGraphJSONKey(
				data,
				keyStart,
				keyEnd,
				true,
				"entrypoint_packages",
			)
			if err != nil {
				return 0, err
			}
			if field == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			if *found {
				return 0, errReportEntrypointJSONUnavailable
			}
			*found = true
			shapeEnd, err := walkSavedGraphJSONArray(
				data,
				valueStart,
				workspaceEntrypointRawRows,
				func(itemStart int) (int, error) {
					return preflightSavedEntrypointShape(data, itemStart, budget)
				},
			)
			if err != nil {
				return 0, err
			}
			budget.anchors = 0
			scalarEnd, err := walkSavedGraphJSONArray(
				data,
				valueStart,
				workspaceEntrypointRawRows,
				func(itemStart int) (int, error) {
					return preflightSavedEntrypointScalars(data, itemStart, budget)
				},
			)
			if err != nil {
				return 0, err
			}
			if scalarEnd != shapeEnd {
				return 0, errReportEntrypointJSONUnavailable
			}
			*span = savedGraphJSONSpan{start: valueStart, end: shapeEnd}
			return shapeEnd, nil
		},
	)
}

func preflightSavedEntrypointShape(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
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
				"module_path",
				"import_path",
				"dir",
				"package_dir",
				"module_relative_dir",
				"module_dir",
				"kind",
				"go_files",
				"anchors",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint16(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportEntrypointJSONUnavailable
			}
			seen |= field
			if fieldIndex != 9 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			remaining := workspaceEntrypointRawRows - budget.anchors
			return walkSavedGraphJSONArray(
				data,
				valueStart,
				remaining,
				func(itemStart int) (int, error) {
					if budget.anchors >= workspaceEntrypointRawRows {
						return 0, errReportEntrypointJSONBounds
					}
					budget.anchors++
					return preflightSavedEntrypointAnchorShape(data, itemStart)
				},
			)
		},
	)
}

func preflightSavedEntrypointAnchorShape(data []byte, start int) (int, error) {
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
				"version",
				"kind",
				"path",
				"line",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint8(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportEntrypointJSONUnavailable
			}
			seen |= field
			return skipSavedGraphJSONValue(data, valueStart)
		},
	)
}

func preflightSavedEntrypointScalars(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
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
				"module_path",
				"import_path",
				"dir",
				"package_dir",
				"module_relative_dir",
				"module_dir",
				"kind",
				"go_files",
				"anchors",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint16(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportEntrypointJSONUnavailable
			}
			seen |= field
			switch fieldIndex {
			case 1, 2, 4, 5, 6:
				return preflightSavedEntrypointJSONString(data, valueStart, budget)
			case 9:
				remaining := workspaceEntrypointRawRows - budget.anchors
				return walkSavedGraphJSONArray(
					data,
					valueStart,
					remaining,
					func(itemStart int) (int, error) {
						if budget.anchors >= workspaceEntrypointRawRows {
							return 0, errReportEntrypointJSONBounds
						}
						budget.anchors++
						return preflightSavedEntrypointAnchorScalars(data, itemStart, budget)
					},
				)
			default:
				return skipSavedGraphJSONValue(data, valueStart)
			}
		},
	)
}

func preflightSavedEntrypointAnchorScalars(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
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
				"version",
				"kind",
				"path",
				"line",
			)
			if err != nil {
				return 0, err
			}
			if fieldIndex == 0 {
				return skipSavedGraphJSONValue(data, valueStart)
			}
			field := uint8(1 << (fieldIndex - 1))
			if seen&field != 0 {
				return 0, errReportEntrypointJSONUnavailable
			}
			seen |= field
			if field == 2 || field == 4 {
				return preflightSavedEntrypointJSONString(data, valueStart, budget)
			}
			return preflightSavedEntrypointJSONInteger(data, valueStart, budget)
		},
	)
}

func preflightSavedEntrypointJSONString(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
) (int, error) {
	end, size, err := scanSavedGraphJSONString(data, start, workspaceEntrypointScalarBytes)
	if err != nil {
		return 0, err
	}
	if size > budget.scalarBytes {
		return 0, errReportEntrypointJSONBounds
	}
	budget.scalarBytes -= size
	return end, nil
}

func preflightSavedEntrypointJSONInteger(
	data []byte,
	start int,
	budget *savedEntrypointJSONBudget,
) (int, error) {
	if savedGraphJSONLiteralAt(data, start, "null") {
		return 0, errReportEntrypointJSONUnavailable
	}
	if start >= len(data) {
		return 0, errReportEntrypointJSONUnavailable
	}
	index := start
	size := 0
	if data[index] == '-' {
		index++
		size++
		if size > workspaceEntrypointScalarBytes {
			return 0, errReportEntrypointJSONBounds
		}
		if index >= len(data) {
			return 0, errReportEntrypointJSONUnavailable
		}
	}
	if data[index] < '0' || data[index] > '9' {
		return 0, errReportEntrypointJSONUnavailable
	}
	if data[index] == '0' {
		index++
		size++
		if index < len(data) && data[index] >= '0' && data[index] <= '9' {
			return 0, errReportEntrypointJSONUnavailable
		}
	} else {
		for index < len(data) && data[index] >= '0' && data[index] <= '9' {
			index++
			size++
			if size > workspaceEntrypointScalarBytes {
				return 0, errReportEntrypointJSONBounds
			}
		}
	}
	if index >= len(data) || !savedEntrypointJSONDelimiter(data[index]) {
		return 0, errReportEntrypointJSONUnavailable
	}
	if size > budget.scalarBytes {
		return 0, errReportEntrypointJSONBounds
	}
	budget.scalarBytes -= size
	return index, nil
}

func savedEntrypointJSONDelimiter(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}

func normalizeReportEntrypointJSONError(err error) error {
	if errors.Is(err, errReportEntrypointJSONBounds) ||
		errors.Is(err, errReportGraphJSONBounds) {
		return errReportEntrypointJSONBounds
	}
	return errReportEntrypointJSONUnavailable
}
