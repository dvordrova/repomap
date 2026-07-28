package semanticmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const (
	topicReviewLedgerVersion      = 1
	topicReviewLedgerMaxBytes     = 64 << 10
	topicReviewLedgerMaxNoteBytes = 512
)

type topicReviewLedger struct {
	Version               int                   `json:"version"`
	Repository            GoSelectionRepository `json:"repository"`
	ShelfResponseSHA256   string                `json:"shelf_response_sha256"`
	TopicExperimentSHA256 json.RawMessage       `json:"topic_experiment_sha256"`
	TopicReviews          []topicReview         `json:"topic_reviews"`
}

type topicReview struct {
	TopicID              string                    `json:"topic_id"`
	ObservationSymbolIDs []string                  `json:"observation_symbol_ids"`
	Status               string                    `json:"status"`
	Note                 string                    `json:"note"`
	Verifications        []topicReviewVerification `json:"verifications"`
}

type topicReviewVerification struct {
	SelectionRef string `json:"selection_ref"`
	Relation     string `json:"relation"`
	Usefulness   string `json:"usefulness"`
	Note         string `json:"note"`
}

type topicReviewFixture struct {
	Name             string
	Repository       GoSelectionRepository
	ShelfSHA256      string
	ExperimentPath   string
	ExperimentSHA256 string
}

func TestRecordedTopicReviewLedgers(t *testing.T) {
	fixtures := []topicReviewFixture{
		{
			Name: "caddy",
			Repository: GoSelectionRepository{
				Name:     "caddy",
				Revision: caddyGoSelectionRevision,
			},
			ShelfSHA256: "06015ecc80a392db44f43510bf2c6ab43530359159ef34074b1d7f785b7cf47d",
		},
		{
			Name: "etcd",
			Repository: GoSelectionRepository{
				Name:     "etcd",
				Revision: etcdGoTopicRevision,
			},
			ShelfSHA256: "82e04e9b293ad6df3652e307eea7dddb324d0a1de2237381b712424d4c712db4",
		},
		{
			Name:             "restic",
			ShelfSHA256:      "fe3bb0ba59e07ac7c5beeedfbf51581237a25d873b34dd387cd6ef1ae4dadc5d",
			ExperimentPath:   "restic.topic-experiment.json",
			ExperimentSHA256: "542fac4ea5cae24d8c40fa1de3725b00d3d20928e2d03b46498b213d26ea8e50",
		},
		{
			Name:             "go-git",
			ShelfSHA256:      "837880944203ee63dc028161d3790de6ee8c6db03ddcb6dbd4c33eea459f5241",
			ExperimentPath:   "go-git.topic-experiment.json",
			ExperimentSHA256: "2d1395f5ab01075ea450c3b616ea97067edf6451b7cf383cedc8637fbf8594a7",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			ledger, shelf, experiment, experimentJSON := loadTopicReviewFixture(t, fixture)
			if err := validateTopicReviewLedger(
				ledger,
				shelf,
				readBoundedFile(
					t,
					fixture.Name+".topic-shelf.response.json",
					goTopicMaxResponseBytes,
				),
				fixture,
				experiment,
				experimentJSON,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTopicReviewLedgerKeepsStatusSeparateFromProjectionUsefulness(t *testing.T) {
	fixture := topicReviewFixture{
		Name:             "go-git",
		ShelfSHA256:      "837880944203ee63dc028161d3790de6ee8c6db03ddcb6dbd4c33eea459f5241",
		ExperimentPath:   "go-git.topic-experiment.json",
		ExperimentSHA256: "2d1395f5ab01075ea450c3b616ea97067edf6451b7cf383cedc8637fbf8594a7",
	}
	ledger, _, _, _ := loadTopicReviewFixture(t, fixture)
	review := ledger.TopicReviews[0]
	if review.TopicID != "t1" || review.Status != "corroborated" {
		t.Fatalf("go-git t1 review = %#v, want corroborated", review)
	}
	for _, verification := range review.Verifications {
		if verification.SelectionRef == "t1/change" {
			if verification.Relation != "unresolved" ||
				verification.Usefulness != "not_useful" {
				t.Fatalf(
					"go-git t1/change = %#v, want unresolved and not useful",
					verification,
				)
			}
			return
		}
	}
	t.Fatal("go-git t1/change verification is missing")
}

func TestTopicReviewLedgerRejectsBrokenReferences(t *testing.T) {
	fixture := topicReviewFixture{
		Name:             "go-git",
		ShelfSHA256:      "837880944203ee63dc028161d3790de6ee8c6db03ddcb6dbd4c33eea459f5241",
		ExperimentPath:   "go-git.topic-experiment.json",
		ExperimentSHA256: "2d1395f5ab01075ea450c3b616ea97067edf6451b7cf383cedc8637fbf8594a7",
	}
	cases := []struct {
		name   string
		mutate func(*topicReviewLedger)
		want   string
	}{
		{
			name: "observation mismatch",
			mutate: func(ledger *topicReviewLedger) {
				ledger.TopicReviews[0].ObservationSymbolIDs[0] = "d9999"
			},
			want: "observation IDs differ",
		},
		{
			name: "missing topic",
			mutate: func(ledger *topicReviewLedger) {
				ledger.TopicReviews = ledger.TopicReviews[:len(ledger.TopicReviews)-1]
			},
			want: "topic review count",
		},
		{
			name: "unknown selection",
			mutate: func(ledger *topicReviewLedger) {
				ledger.TopicReviews[0].Verifications[0].SelectionRef = "t1/missing"
			},
			want: "unknown selection",
		},
		{
			name: "null experiment with verification",
			mutate: func(ledger *topicReviewLedger) {
				ledger.TopicExperimentSHA256 = json.RawMessage("null")
			},
			want: "null experiment",
		},
		{
			name: "recorded selection left not reviewed",
			mutate: func(ledger *topicReviewLedger) {
				ledger.TopicReviews[0].Verifications[0].Usefulness = "not_reviewed"
			},
			want: "verification",
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			ledger, shelf, experiment, experimentJSON := loadTopicReviewFixture(t, fixture)
			item.mutate(&ledger)
			err := validateTopicReviewLedger(
				ledger,
				shelf,
				readBoundedFile(
					t,
					fixture.Name+".topic-shelf.response.json",
					goTopicMaxResponseBytes,
				),
				fixture,
				experiment,
				experimentJSON,
			)
			if err == nil || !strings.Contains(err.Error(), item.want) {
				t.Fatalf("error = %v, want containing %q", err, item.want)
			}
		})
	}
}

func TestTopicReviewLedgerRequiresLiteralNullForResponseOnlyFixture(t *testing.T) {
	fixture := topicReviewFixture{
		Name: "caddy",
		Repository: GoSelectionRepository{
			Name:     "caddy",
			Revision: caddyGoSelectionRevision,
		},
		ShelfSHA256: "06015ecc80a392db44f43510bf2c6ab43530359159ef34074b1d7f785b7cf47d",
	}
	ledger, shelf, experiment, experimentJSON := loadTopicReviewFixture(t, fixture)
	ledger.TopicExperimentSHA256 = nil
	err := validateTopicReviewLedger(
		ledger,
		shelf,
		readBoundedFile(
			t,
			fixture.Name+".topic-shelf.response.json",
			goTopicMaxResponseBytes,
		),
		fixture,
		experiment,
		experimentJSON,
	)
	if err == nil || !strings.Contains(err.Error(), "must be a string or literal null") {
		t.Fatalf("error = %v, want omitted hash rejection", err)
	}
}

func loadTopicReviewFixture(
	t *testing.T,
	fixture topicReviewFixture,
) (topicReviewLedger, GoTopicShelf, *GoTopicExperiment, []byte) {
	t.Helper()
	ledger := decodeStrict[topicReviewLedger](t, readBoundedFile(
		t,
		fixture.Name+".topic-review.json",
		topicReviewLedgerMaxBytes,
	))
	shelf := decodeStrict[GoTopicShelf](t, readBoundedFile(
		t,
		fixture.Name+".topic-shelf.response.json",
		goTopicMaxResponseBytes,
	))
	if fixture.ExperimentPath == "" {
		return ledger, shelf, nil, nil
	}
	experimentJSON := readBoundedFile(t, fixture.ExperimentPath, 2<<20)
	experiment := decodeStrict[GoTopicExperiment](t, experimentJSON)
	return ledger, shelf, &experiment, experimentJSON
}

func validateTopicReviewLedger(
	ledger topicReviewLedger,
	shelf GoTopicShelf,
	shelfJSON []byte,
	fixture topicReviewFixture,
	experiment *GoTopicExperiment,
	experimentJSON []byte,
) error {
	if ledger.Version != topicReviewLedgerVersion ||
		!goTopicScalar(ledger.Repository.Name, goTopicMaxTextBytes, false) ||
		len(ledger.Repository.Revision) != 40 ||
		!lowerHex(ledger.Repository.Revision) {
		return fmt.Errorf("topic review ledger: metadata is invalid")
	}
	if len(ledger.ShelfResponseSHA256) != 64 ||
		!lowerHex(ledger.ShelfResponseSHA256) ||
		ledger.ShelfResponseSHA256 != fixture.ShelfSHA256 ||
		goTopicSHA256(shelfJSON) != fixture.ShelfSHA256 {
		return fmt.Errorf("topic review ledger: shelf response hash is invalid")
	}
	if shelf.Coverage != goTopicCoverage ||
		len(shelf.Topics) < goTopicMinShelfTopics ||
		len(shelf.Topics) > goTopicMaxShelfTopics ||
		len(ledger.TopicReviews) != len(shelf.Topics) {
		return fmt.Errorf("topic review ledger: topic review count or shelf coverage is invalid")
	}

	topics := make(map[string]GoTopic, goTopicMaxShelfTopics)
	for index, topic := range shelf.Topics {
		if topic.ID != fmt.Sprintf("t%d", index+1) {
			return fmt.Errorf("topic review ledger: shelf topic ID %q is invalid", topic.ID)
		}
		if _, duplicate := topics[topic.ID]; duplicate {
			return fmt.Errorf("topic review ledger: duplicate shelf topic %q", topic.ID)
		}
		topics[topic.ID] = topic
	}

	selections := make(map[string]GoTopicSelectionRun, goTopicSelectorRuns)
	experimentSHA256, experimentIsNull, err := topicReviewExperimentSHA256(
		ledger.TopicExperimentSHA256,
	)
	if err != nil {
		return err
	}
	if experiment == nil {
		if !experimentIsNull ||
			ledger.Repository != fixture.Repository ||
			fixture.ExperimentPath != "" ||
			fixture.ExperimentSHA256 != "" {
			return fmt.Errorf("topic review ledger: response-only metadata is invalid")
		}
	} else {
		if experimentIsNull {
			return fmt.Errorf("topic review ledger: null experiment has verification data")
		}
		if len(experimentSHA256) != 64 ||
			!lowerHex(experimentSHA256) ||
			experimentSHA256 != fixture.ExperimentSHA256 ||
			goTopicSHA256(experimentJSON) != fixture.ExperimentSHA256 {
			return fmt.Errorf("topic review ledger: experiment hash is invalid")
		}
		if err := ValidateGoTopicExperiment(*experiment); err != nil {
			return err
		}
		if ledger.Repository != experiment.Repository ||
			experiment.ResponseSHA256 != ledger.ShelfResponseSHA256 ||
			!reflect.DeepEqual(experiment.Shelf, shelf) {
			return fmt.Errorf("topic review ledger: experiment references differ")
		}
		for _, selection := range experiment.Selections {
			topic, ok := topics[selection.TopicID]
			if !ok {
				return fmt.Errorf(
					"topic review ledger: selection references unknown topic %q",
					selection.TopicID,
				)
			}
			suffix := ""
			switch selection.Question {
			case topic.HowQuestion:
				suffix = "how"
			case topic.ChangeQuestion:
				suffix = "change"
			default:
				return fmt.Errorf(
					"topic review ledger: selection question differs for %q",
					selection.TopicID,
				)
			}
			ref := selection.TopicID + "/" + suffix
			if _, duplicate := selections[ref]; duplicate {
				return fmt.Errorf("topic review ledger: duplicate selection %q", ref)
			}
			selections[ref] = selection
		}
	}

	seenTopics := make(map[string]struct{}, goTopicMaxShelfTopics)
	seenSelections := make(map[string]struct{}, goTopicSelectorRuns)
	for _, review := range ledger.TopicReviews {
		topic, ok := topics[review.TopicID]
		if !ok {
			return fmt.Errorf(
				"topic review ledger: review references unknown topic %q",
				review.TopicID,
			)
		}
		if _, duplicate := seenTopics[review.TopicID]; duplicate {
			return fmt.Errorf(
				"topic review ledger: duplicate topic review %q",
				review.TopicID,
			)
		}
		seenTopics[review.TopicID] = struct{}{}
		if !reflect.DeepEqual(
			review.ObservationSymbolIDs,
			topic.SupportSymbolIDs,
		) {
			return fmt.Errorf(
				"topic review ledger: %s observation IDs differ from shelf support IDs",
				review.TopicID,
			)
		}
		if !topicReviewStatus(review.Status) ||
			!goTopicScalar(review.Note, topicReviewLedgerMaxNoteBytes, false) ||
			len(review.Verifications) > goTopicSelectorRuns {
			return fmt.Errorf("topic review ledger: %s review is invalid", review.TopicID)
		}
		if experiment == nil {
			if review.Status != "not_reviewed" || len(review.Verifications) != 0 {
				return fmt.Errorf(
					"topic review ledger: null experiment contains review claims",
				)
			}
			continue
		}
		if review.Status == "not_reviewed" {
			return fmt.Errorf(
				"topic review ledger: complete experiment leaves %s not reviewed",
				review.TopicID,
			)
		}
		for _, verification := range review.Verifications {
			selection, ok := selections[verification.SelectionRef]
			if !ok {
				return fmt.Errorf(
					"topic review ledger: unknown selection %q",
					verification.SelectionRef,
				)
			}
			if selection.TopicID != review.TopicID {
				return fmt.Errorf(
					"topic review ledger: selection %q belongs to another topic",
					verification.SelectionRef,
				)
			}
			if _, duplicate := seenSelections[verification.SelectionRef]; duplicate {
				return fmt.Errorf(
					"topic review ledger: duplicate verification %q",
					verification.SelectionRef,
				)
			}
			seenSelections[verification.SelectionRef] = struct{}{}
			if !topicReviewRelation(verification.Relation) ||
				!topicReviewUsefulness(verification.Usefulness) ||
				verification.Usefulness == "not_reviewed" ||
				!goTopicScalar(
					verification.Note,
					topicReviewLedgerMaxNoteBytes,
					false,
				) {
				return fmt.Errorf(
					"topic review ledger: verification %q is invalid",
					verification.SelectionRef,
				)
			}
		}
	}
	if len(seenTopics) != len(topics) {
		return fmt.Errorf("topic review ledger: topic review count is incomplete")
	}
	if experiment != nil && len(seenSelections) != len(selections) {
		return fmt.Errorf("topic review ledger: recorded selections are not fully reviewed")
	}
	return nil
}

func topicReviewExperimentSHA256(
	raw json.RawMessage,
) (string, bool, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", true, nil
	}
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == "" {
		return "", false, fmt.Errorf(
			"topic review ledger: experiment hash must be a string or literal null",
		)
	}
	return value, false, nil
}

func topicReviewStatus(value string) bool {
	switch value {
	case "corroborated", "conflicted", "unknown", "not_reviewed":
		return true
	default:
		return false
	}
}

func topicReviewRelation(value string) bool {
	switch value {
	case "supports", "conflicts", "unresolved":
		return true
	default:
		return false
	}
}

func topicReviewUsefulness(value string) bool {
	switch value {
	case "useful", "not_useful", "not_reviewed":
		return true
	default:
		return false
	}
}
