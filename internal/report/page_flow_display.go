package report

import "encoding/json"

// componentFlow is the already prepared reading, not another walk of the
// program. Its complete model path, native start list and independent work
// enter the page's shared value table; only the selected reading builds DOM.
type componentFlow struct {
	Flow      *pageFlow       `json:"Flow,omitempty"`
	Start     []pageStart     `json:"Start,omitempty"`
	Elsewhere []pageElsewhere `json:"Elsewhere,omitempty"`
	Own       []pageOwnWork   `json:"Own,omitempty"`
}

func (section *pageSection) FlowJSON() (string, error) {
	if section.Flow == nil && len(section.Start) == 0 && len(section.OwnWork) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(componentFlow{Flow: section.Flow, Start: section.Start,
		Elsewhere: section.StartElsewhere, Own: section.OwnWork})
	return string(encoded), err
}
