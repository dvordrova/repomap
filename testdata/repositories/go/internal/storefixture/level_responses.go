package storefixture

// GetLevelsInfoResponse describes a count, not a list of levels.
type GetLevelsInfoResponse struct {
	Count int `json:"count"`
}

// OtherLevelsInfoResponse owns a different declaration with the same name.
type OtherLevelsInfoResponse struct {
	Count string `json:"count_label"`
}

// EmbeddedLevelsInfoResponse declares one embedded field, not another Count.
type EmbeddedLevelsInfoResponse struct {
	GetLevelsInfoResponse
}

// LevelsInfoAlias introduces no new field declaration.
type LevelsInfoAlias = GetLevelsInfoResponse

// TODO: document count validation.

// NOTE: count describes a quantity, not a list of levels.
