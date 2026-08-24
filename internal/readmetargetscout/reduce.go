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
)

const (
	schemaContract  = "response is exactly one JSON array of {file_ref,classifications:[{class,hypotheses}]}; strict fields and closed classes-v2"
	reducerContract = "known complete-corpus FileIDs only; guidance-grounded multi-role rows without a repository-size quota; non-documentation prose roles reject the complete response; canonical path/class/hypothesis order-v5"
)

// ResolveResponse rejects the complete model response when it assigns a
// non-documentation role to a prose file. The reducer never repairs an invalid
// class into documentation or accepts the remaining prefix/subset.
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
	seenFiles := make(map[string]struct{}, len(wireItems))
	result := make(Result, 0, len(wireItems))
	for _, wireItem := range wireItems {
		filePath, known := compilation.authority[wireItem.FileRef]
		if !known {
			return nil, fmt.Errorf("README file classifier: response cites unknown file_ref")
		}
		if _, duplicate := seenFiles[string(wireItem.FileRef)]; duplicate {
			return nil, fmt.Errorf("README file classifier: response contains duplicate file_ref")
		}
		seenFiles[string(wireItem.FileRef)] = struct{}{}
		if wireItem.Classifications == nil || len(wireItem.Classifications) == 0 ||
			len(wireItem.Classifications) > MaxClassificationsPerFile {
			return nil, fmt.Errorf("README file classifier: invalid classifications array")
		}

		seenClasses := make(map[FileClass]struct{}, len(wireItem.Classifications))
		classifications := make([]Classification, 0, len(wireItem.Classifications))
		for _, classification := range wireItem.Classifications {
			if !validFileClass(classification.Class) {
				return nil, fmt.Errorf("README file classifier: response contains unknown file class")
			}
			if _, duplicate := seenClasses[classification.Class]; duplicate {
				return nil, fmt.Errorf("README file classifier: response classifies one file twice with the same class")
			}
			seenClasses[classification.Class] = struct{}{}
			if classification.Hypotheses == nil || len(classification.Hypotheses) == 0 ||
				len(classification.Hypotheses) > MaxHypothesesPerClassification {
				return nil, fmt.Errorf("README file classifier: invalid classification hypotheses array")
			}
			hypotheses := append([]string(nil), classification.Hypotheses...)
			seenHypotheses := make(map[string]struct{}, len(hypotheses))
			for _, hypothesis := range hypotheses {
				if hypothesis == "" || hypothesis != strings.TrimSpace(hypothesis) ||
					len(hypothesis) > MaxHypothesisBytes || !utf8.ValidString(hypothesis) || containsControl(hypothesis) {
					return nil, fmt.Errorf("README file classifier: invalid classification hypothesis")
				}
				if _, duplicate := seenHypotheses[hypothesis]; duplicate {
					return nil, fmt.Errorf("README file classifier: duplicate classification hypothesis")
				}
				seenHypotheses[hypothesis] = struct{}{}
			}
			sort.Strings(hypotheses)
			if isProseEvidencePath(filePath) && classification.Class != ClassDocumentation {
				return nil, fmt.Errorf(
					"README file classifier: prose file %q cannot have class %q; classify it as documentation or omit it",
					filePath,
					classification.Class,
				)
			}
			classifications = append(classifications, Classification{
				Class: classification.Class, Hypotheses: hypotheses,
			})
		}
		sort.Slice(classifications, func(i, j int) bool {
			return classifications[i].Class < classifications[j].Class
		})
		if len(classifications) > 0 {
			result = append(result, ClassifiedFile{
				FileRef: wireItem.FileRef, Classifications: classifications,
			})
		}
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
