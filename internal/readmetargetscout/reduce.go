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
)

const (
	schemaContract  = "response is exactly one JSON array of {file_ref,classifications:[{class,hypotheses}]}; strict fields and closed classes; repeated set-valued rows are permitted; no local class, hypothesis-count, or hypothesis-byte ceiling; unknown file_ref rows are ignorable-v5"
	reducerContract = "ignore rows whose FileID is outside request-local authority before class interpretation; discard valid non-documentation classifications for known prose refs before hypothesis validation without promotion; merge repeated known file and class rows and deduplicate identical hypotheses without local count or text ceilings; union every shard against aggregate authority; guidance-grounded multi-role rows without a repository-size quota; canonical path/class/hypothesis order-v9"
)

// ResolveResponse treats valid non-documentation roles for a known prose ref
// as unsupported set members and discards them before their hypotheses or
// content has authority. It never repairs such a role into documentation;
// independently valid classifications in the same response remain usable.
func ResolveResponse(compilation Compilation, raw []byte) (Result, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return nil, fmt.Errorf("README file classifier: response exceeds bounded envelope")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("README file classifier: response must be one JSON array")
	}
	var wireItems []ClassifiedFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wireItems); err != nil || wireItems == nil {
		return nil, fmt.Errorf("README file classifier: invalid JSON response")
	}
	if err := ensureResponseEOF(decoder); err != nil {
		return nil, err
	}
	type classSet map[FileClass]map[string]struct{}
	files := make(map[corpus.FileID]classSet, len(wireItems))
	for _, wireItem := range wireItems {
		filePath, known := compilation.authority[wireItem.FileRef]
		if !known {
			continue
		}
		for _, classification := range wireItem.Classifications {
			if !validFileClass(classification.Class) {
				return nil, fmt.Errorf("README file classifier: response contains unknown file class")
			}
		}
		if wireItem.Classifications == nil || len(wireItem.Classifications) == 0 {
			return nil, fmt.Errorf("README file classifier: invalid classifications array")
		}
		classes := files[wireItem.FileRef]
		for _, classification := range wireItem.Classifications {
			// Prose membership is exact negative compatibility authority. Once a
			// closed class is known to be incompatible, neither its hypotheses nor
			// its contribution to per-file bounds has authority.
			if isProseEvidencePath(filePath) && classification.Class != ClassDocumentation {
				continue
			}
			if classification.Hypotheses == nil || len(classification.Hypotheses) == 0 {
				return nil, fmt.Errorf("README file classifier: invalid classification hypotheses array")
			}
			if classes == nil {
				classes = make(classSet)
				files[wireItem.FileRef] = classes
			}
			hypotheses := classes[classification.Class]
			if hypotheses == nil {
				hypotheses = make(map[string]struct{})
				classes[classification.Class] = hypotheses
			}
			for _, hypothesis := range classification.Hypotheses {
				if !validHypothesis(hypothesis) {
					return nil, fmt.Errorf("README file classifier: invalid classification hypothesis")
				}
				hypotheses[hypothesis] = struct{}{}
			}
		}
	}

	result := make(Result, 0, len(files))
	for fileRef, classes := range files {
		classifications := make([]Classification, 0, len(classes))
		for class, hypothesisSet := range classes {
			hypotheses := make([]string, 0, len(hypothesisSet))
			for hypothesis := range hypothesisSet {
				hypotheses = append(hypotheses, hypothesis)
			}
			sort.Strings(hypotheses)
			classifications = append(classifications, Classification{
				Class: class, Hypotheses: hypotheses,
			})
		}
		sort.Slice(classifications, func(i, j int) bool {
			return classifications[i].Class < classifications[j].Class
		})
		result = append(result, ClassifiedFile{
			FileRef: fileRef, Classifications: classifications,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		leftPath := compilation.authority[result[left].FileRef]
		rightPath := compilation.authority[result[right].FileRef]
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		return result[left].FileRef < result[right].FileRef
	})
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
