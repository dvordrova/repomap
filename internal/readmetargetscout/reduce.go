package readmetargetscout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	schemaContract  = "response is one JSON array of {file_ref,classifications:[{class,hypotheses}]}; extra fields have no authority; independently validate files, closed classes and hypotheses; repeated set-valued rows are permitted; no local class, hypothesis-count, or hypothesis-byte ceiling; unknown file_ref rows are ignorable-v6"
	reducerContract = "ignore rows whose FileID is outside request-local authority before class interpretation; discard incompatible prose roles without promotion; retain every valid hypothesis beside rejected siblings and record reasons; normalize short hypothesis whitespace; merge repeated known file and class rows and deduplicate identical hypotheses without local count or text ceilings; union accepted shards against aggregate authority; canonical path/class/hypothesis order-v10"
)

// ResolveResponse treats valid non-documentation roles for a known prose ref
// as unsupported set members and discards them before their hypotheses or
// content has authority. It never repairs such a role into documentation;
// independently valid classifications in the same response remain usable.
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
	var wireItems []json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&wireItems); err != nil || wireItems == nil {
		return result, fmt.Errorf("README file classifier: response must be one JSON array")
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
	type classSet map[FileClass]map[string]struct{}
	files := make(map[corpus.FileID]classSet)
	invalid := false
	for i, rawFile := range wireItems {
		currentFile = ""
		position := fmt.Sprintf("files[%d]", i)
		var file struct {
			FileRef         corpus.FileID   `json:"file_ref"`
			Classifications json.RawMessage `json:"classifications"`
		}
		if json.Unmarshal(rawFile, &file) != nil || file.FileRef == "" {
			invalid = true
			reject(position, "file row has no string file_ref")
			continue
		}
		currentFile = file.FileRef
		filePath, known := compilation.authority[file.FileRef]
		if !known {
			reject(position, "file_ref was not advertised")
			continue
		}
		var classes []json.RawMessage
		if json.Unmarshal(file.Classifications, &classes) != nil || len(classes) == 0 {
			invalid = true
			reject(position, "classifications must be a non-empty array")
			continue
		}
		for j, rawClass := range classes {
			at := fmt.Sprintf("%s.classifications[%d]", position, j)
			var class struct {
				Class      FileClass       `json:"class"`
				Hypotheses json.RawMessage `json:"hypotheses"`
			}
			if json.Unmarshal(rawClass, &class) != nil || !validFileClass(class.Class) {
				invalid = true
				reject(at, "classification has no supported class")
				continue
			}
			if isProseEvidencePath(filePath) && class.Class != ClassDocumentation {
				reject(at, "class is incompatible with this prose file")
				continue
			}
			var hypotheses []json.RawMessage
			if json.Unmarshal(class.Hypotheses, &hypotheses) != nil || len(hypotheses) == 0 {
				invalid = true
				reject(at, "hypotheses must be a non-empty array")
				continue
			}
			for k, rawHypothesis := range hypotheses {
				var hypothesis string
				if json.Unmarshal(rawHypothesis, &hypothesis) != nil {
					invalid = true
					reject(fmt.Sprintf("%s.hypotheses[%d]", at, k), "hypothesis must be text")
					continue
				}
				hypothesis = strings.Join(strings.Fields(hypothesis), " ")
				if !validHypothesis(hypothesis) {
					invalid = true
					reject(fmt.Sprintf("%s.hypotheses[%d]", at, k), "hypothesis must be non-empty text")
					continue
				}
				if files[file.FileRef] == nil {
					files[file.FileRef] = make(classSet)
				}
				if files[file.FileRef][class.Class] == nil {
					files[file.FileRef][class.Class] = make(map[string]struct{})
				}
				files[file.FileRef][class.Class][hypothesis] = struct{}{}
			}
		}
	}
	for fileRef, classes := range files {
		var classifications []Classification
		for class, hypothesisSet := range classes {
			hypotheses := make([]string, 0, len(hypothesisSet))
			for hypothesis := range hypothesisSet {
				hypotheses = append(hypotheses, hypothesis)
			}
			sort.Strings(hypotheses)
			classifications = append(classifications, Classification{Class: class, Hypotheses: hypotheses})
		}
		sort.Slice(classifications, func(i, j int) bool { return classifications[i].Class < classifications[j].Class })
		result.Result = append(result.Result, ClassifiedFile{FileRef: fileRef, Classifications: classifications})
	}
	sort.Slice(result.Result, func(i, j int) bool {
		return compilation.authority[result.Result[i].FileRef] < compilation.authority[result.Result[j].FileRef]
	})
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

func validHypothesis(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !containsControl(value)
}

func validFileClass(value FileClass) bool {
	switch value {
	case ClassTargetEntry, ClassExampleEntry, ClassTestEntry,
		ClassSupportToolEntry, ClassConfiguration, ClassDatabaseAsset,
		ClassClientEntry, ClassDocumentation, ClassDeployment,
		ClassInterfaceContract:
		return true
	default:
		return false
	}
}

func isProseEvidencePath(value string) bool {
	if isReadmePath(value) {
		return true
	}
	name := strings.ToLower(path.Base(value))
	for _, prefix := range []string{
		"license", "copying", "changelog", "changes", "contributing", "code_of_conduct",
	} {
		if name == prefix || strings.HasPrefix(name, prefix+".") {
			return true
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn",
		".rst", ".rest", ".txt", ".textile", ".rdoc", ".org",
		".creole", ".mediawiki", ".wiki", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
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
