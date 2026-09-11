package readmetargetscout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	schemaContract  = "response is one JSON object {files:[{file_ref,hypotheses}]}; extra fields have no authority; independently validate file refs and hypotheses; repeated set-valued rows are permitted; no local hypothesis-count or hypothesis-byte ceiling; unknown file_ref rows are ignorable-v7"
	reducerContract = "ignore rows whose FileID is outside request-local candidate authority before their hypotheses are interpreted; retain every valid hypothesis beside rejected siblings and record reasons; normalize short hypothesis whitespace; merge repeated known file rows and deduplicate identical hypotheses without local count or text ceilings; every accepted file is one target_entry classification; union accepted shards against aggregate authority; canonical path/hypothesis order-v11"
)

// responseResult keeps the accepted catalog beside the exact local
// rejections. A row citing an unknown ref is dropped before its hypotheses
// have authority; independently valid rows in the same response remain usable.
type responseResult struct {
	Result   Result
	rejected []llm.ResponseRejection
	accepted []string
}

func (result responseResult) ResponseRejections() []llm.ResponseRejection { return result.rejected }
func (result responseResult) AcceptedRowKeys() []string {
	if len(result.rejected) > 0 {
		return result.accepted
	}
	return nil
}

func ResolveResponse(compilation Compilation, raw []byte) (Result, error) {
	result, err := resolveResponse(compilation, raw)
	return result.Result, err
}

func resolveResponse(compilation Compilation, raw []byte) (responseResult, error) {
	result := responseResult{Result: Result{}, accepted: []string{}}
	if err := validateReadyCompilation(compilation); err != nil {
		return result, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return result, fmt.Errorf("README file classifier: response exceeds bounded envelope")
	}
	var wire struct {
		Files []json.RawMessage `json:"files"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&wire); err != nil || wire.Files == nil {
		return result, fmt.Errorf("README file classifier: response must be one JSON object with a files array")
	}
	if err := ensureResponseEOF(decoder); err != nil {
		return result, err
	}
	badFiles := make(map[corpus.FileID]bool)
	var currentFile corpus.FileID
	reject := func(position, reason string) {
		if currentFile != "" {
			badFiles[currentFile] = true
		}
		result.rejected = append(result.rejected, llm.ResponseRejection{Kind: "classification_rejected", Count: 1, Samples: []string{position}, Reason: reason})
	}
	files := make(map[corpus.FileID]map[string]struct{})
	invalid := false
	for i, rawFile := range wire.Files {
		currentFile = ""
		position := fmt.Sprintf("files[%d]", i)
		var file struct {
			FileRef    corpus.FileID   `json:"file_ref"`
			Hypotheses json.RawMessage `json:"hypotheses"`
		}
		if json.Unmarshal(rawFile, &file) != nil || file.FileRef == "" {
			invalid = true
			reject(position, "file row has no string file_ref")
			continue
		}
		currentFile = file.FileRef
		if _, known := compilation.authority[file.FileRef]; !known {
			reject(position, "file_ref was not advertised")
			continue
		}
		var hypotheses []json.RawMessage
		if json.Unmarshal(file.Hypotheses, &hypotheses) != nil || len(hypotheses) == 0 {
			invalid = true
			reject(position, "hypotheses must be a non-empty array")
			continue
		}
		for k, rawHypothesis := range hypotheses {
			at := fmt.Sprintf("%s.hypotheses[%d]", position, k)
			var hypothesis string
			if json.Unmarshal(rawHypothesis, &hypothesis) != nil {
				invalid = true
				reject(at, "hypothesis must be text")
				continue
			}
			hypothesis = strings.Join(strings.Fields(hypothesis), " ")
			if !validHypothesis(hypothesis) {
				invalid = true
				reject(at, "hypothesis must be non-empty text")
				continue
			}
			if files[file.FileRef] == nil {
				files[file.FileRef] = make(map[string]struct{})
			}
			files[file.FileRef][hypothesis] = struct{}{}
		}
	}
	for fileRef, hypotheses := range files {
		result.Result = append(result.Result, entryFile(fileRef, hypotheses))
	}
	sortByPath(result.Result, compilation.authority)
	for _, file := range result.Result {
		if !badFiles[file.FileRef] {
			result.accepted = append(result.accepted, string(file.FileRef))
		}
	}
	if invalid && len(result.Result) == 0 {
		return result, fmt.Errorf("README file classifier: no usable classifications: %s", result.rejected[0].Reason)
	}
	return result, nil
}

// entryFile builds the canonical single-role row of one accepted file.
func entryFile(fileRef corpus.FileID, hypothesisSet map[string]struct{}) ClassifiedFile {
	hypotheses := make([]string, 0, len(hypothesisSet))
	for hypothesis := range hypothesisSet {
		hypotheses = append(hypotheses, hypothesis)
	}
	sort.Strings(hypotheses)
	return ClassifiedFile{
		FileRef:         fileRef,
		Classifications: []Classification{{Class: ClassTargetEntry, Hypotheses: hypotheses}},
	}
}

func sortByPath(result Result, authority map[corpus.FileID]string) {
	sort.Slice(result, func(i, j int) bool {
		left, right := authority[result[i].FileRef], authority[result[j].FileRef]
		if left != right {
			return left < right
		}
		return result[i].FileRef < result[j].FileRef
	})
}

func validHypothesis(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !containsControl(value)
}

func validFileClass(value FileClass) bool {
	return value == ClassTargetEntry
}

func ensureResponseEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("README file classifier: trailing JSON value")
		}
		return fmt.Errorf("README file classifier: invalid trailing response data")
	}
	return nil
}
