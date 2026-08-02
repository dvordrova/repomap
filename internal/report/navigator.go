package report

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"

	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func readNavigatorReportProduct(
	runDir string,
	atlas *repositoryatlas.Atlas,
) (*NavigatorReportProduct, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("navigator report: open run directory: %w", err)
	}
	defer root.Close()

	requestPresent, err := navigatorArtifactPresent(root, navigator.RequestArtifactFilename)
	if err != nil {
		return nil, err
	}
	resultPresent, err := navigatorArtifactPresent(root, navigator.RecordArtifactFilename)
	if err != nil {
		return nil, err
	}
	statusPresent, err := navigatorArtifactPresent(root, navigator.StatusArtifactFilename)
	if err != nil {
		return nil, err
	}
	if atlas == nil {
		if requestPresent || resultPresent || statusPresent {
			return nil, fmt.Errorf("navigator report: artifacts require an exact repository Atlas")
		}
		return nil, nil
	}
	if !statusPresent {
		return nil, fmt.Errorf("navigator report: Atlas-first status artifact is required")
	}
	statusJSON, err := readManifestFile(
		root,
		navigator.StatusArtifactFilename,
		navigator.MaxStatusArtifactBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("navigator report: read status: %w", err)
	}
	status, err := navigator.DecodeStatus(statusJSON)
	if err != nil {
		return nil, fmt.Errorf("navigator report: status: %w", err)
	}
	if err := navigator.ValidateStatusAgainstAtlas(status, *atlas); err != nil {
		return nil, fmt.Errorf("navigator report: status does not match repository Atlas: %w", err)
	}
	product := &NavigatorReportProduct{
		Version: navigator.ProductVersion, State: status.State,
		UnavailableCode: status.UnavailableCode,
	}

	switch status.State {
	case navigator.ProductStateEmpty:
		if requestPresent || !resultPresent {
			if requestPresent {
				return nil, fmt.Errorf("navigator report: empty local result must not have a provider request artifact")
			}
			return nil, fmt.Errorf("navigator report: empty local result requires its exact result artifact")
		}
		result, readErr := readNavigatorResult(root, *atlas)
		if readErr != nil {
			return nil, readErr
		}
		if err := navigatorResultMatchesStatus(result, status); err != nil {
			return nil, err
		}
	case navigator.ProductStateUnavailable:
		if status.UnavailableCode != navigator.UnavailableOffline {
			return nil, fmt.Errorf("navigator report: unsupported unavailable code %q", status.UnavailableCode)
		}
		if resultPresent {
			return nil, fmt.Errorf("navigator report: unavailable result must not have a recommendation artifact")
		}
		if !requestPresent {
			return nil, fmt.Errorf("navigator report: unavailable result requires its exact request artifact")
		}
		request, readErr := readNavigatorRequest(root)
		if readErr != nil {
			return nil, readErr
		}
		if err := navigatorRequestMatchesStatus(request, status); err != nil {
			return nil, err
		}
		if err := navigator.ValidateRequestRecordAgainstAtlas(request, *atlas); err != nil {
			return nil, fmt.Errorf("navigator report: request does not match repository Atlas: %w", err)
		}
	case navigator.ProductStateSelected:
		if !resultPresent {
			return nil, fmt.Errorf("navigator report: selected result requires its exact result artifact")
		}
		result, readErr := readNavigatorResult(root, *atlas)
		if readErr != nil {
			return nil, readErr
		}
		if err := navigatorResultMatchesStatus(result, status); err != nil {
			return nil, err
		}
		if !requestPresent {
			return nil, fmt.Errorf("navigator report: selected result requires its exact request artifact")
		}
		request, readErr := readNavigatorRequest(root)
		if readErr != nil {
			return nil, readErr
		}
		if err := navigatorRequestMatchesResult(request, result); err != nil {
			return nil, err
		}
		if err := navigator.ValidateRequestRecordAgainstAtlas(request, *atlas); err != nil {
			return nil, fmt.Errorf("navigator report: request does not match repository Atlas: %w", err)
		}
		selected := cloneNavigatorRecommendation(*result.Selected)
		product.Recommendation = &selected
	default:
		return nil, fmt.Errorf("navigator report: status state %q is not publishable", status.State)
	}
	return product, nil
}

func readNavigatorResult(
	root *os.Root,
	atlas repositoryatlas.Atlas,
) (navigator.RecommendationRecord, error) {
	resultJSON, err := readManifestFile(
		root,
		navigator.RecordArtifactFilename,
		repositoryatlas.MaxArtifactBytes,
	)
	if err != nil {
		return navigator.RecommendationRecord{}, fmt.Errorf("navigator report: read result: %w", err)
	}
	result, err := navigator.DecodeRecommendationRecord(resultJSON)
	if err != nil {
		return navigator.RecommendationRecord{}, fmt.Errorf("navigator report: result: %w", err)
	}
	if err := navigator.ValidateRecommendationRecordAgainstAtlas(result, atlas); err != nil {
		return navigator.RecommendationRecord{}, fmt.Errorf(
			"navigator report: result does not match repository Atlas: %w",
			err,
		)
	}
	return result, nil
}

func readNavigatorRequest(root *os.Root) (navigator.RequestRecord, error) {
	requestJSON, err := readManifestFile(
		root,
		navigator.RequestArtifactFilename,
		repositoryatlas.MaxArtifactBytes,
	)
	if err != nil {
		return navigator.RequestRecord{}, fmt.Errorf("navigator report: read request: %w", err)
	}
	request, err := navigator.DecodeRequestRecord(requestJSON)
	if err != nil {
		return navigator.RequestRecord{}, fmt.Errorf("navigator report: request: %w", err)
	}
	return request, nil
}

func navigatorRequestMatchesStatus(request navigator.RequestRecord, status navigator.Status) error {
	if request.AtlasSHA256 != status.AtlasSHA256 ||
		request.WireSHA256 != status.WireSHA256 ||
		request.CatalogSHA256 != status.CatalogSHA256 ||
		request.CatalogRef != status.CatalogRef ||
		len(request.Actions) != status.ActionCount {
		return fmt.Errorf("navigator report: request and status identities do not match")
	}
	return nil
}

func navigatorArtifactPresent(root *os.Root, name string) (bool, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("navigator report: inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("navigator report: %s is not a regular file", name)
	}
	return true, nil
}

func navigatorRequestMatchesResult(
	request navigator.RequestRecord,
	result navigator.RecommendationRecord,
) error {
	if request.AtlasSHA256 != result.AtlasSHA256 ||
		request.WireSHA256 != result.WireSHA256 ||
		request.CatalogSHA256 != result.CatalogSHA256 ||
		request.CatalogRef != result.CatalogRef ||
		request.Question != result.Question ||
		!reflect.DeepEqual(request.Actions, result.Actions) {
		return fmt.Errorf("navigator report: request and result identities do not match")
	}
	return nil
}

func navigatorResultMatchesStatus(
	result navigator.RecommendationRecord,
	status navigator.Status,
) error {
	if result.State != status.State || result.AtlasSHA256 != status.AtlasSHA256 ||
		result.WireSHA256 != status.WireSHA256 ||
		result.CatalogSHA256 != status.CatalogSHA256 ||
		result.CatalogRef != status.CatalogRef ||
		len(result.Actions) != status.ActionCount || status.FailureCode != "" {
		return fmt.Errorf("navigator report: result and status identities do not match")
	}
	selectedKey := ""
	if result.Selected != nil {
		selectedKey = result.Selected.Key
	}
	if selectedKey != status.SelectedActionKey {
		return fmt.Errorf("navigator report: result and status selections do not match")
	}
	return nil
}

func cloneNavigatorRecommendation(
	value navigator.RecommendationAction,
) navigator.RecommendationAction {
	value.EvidenceIDs = append([]string(nil), value.EvidenceIDs...)
	return value
}
