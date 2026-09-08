package report

import (
	"encoding/json"
	"fmt"
)

// rebindDisplayTranslations changes request-local refs only. A presentation
// can encounter the same saved text in another order without needing another
// translation, but every role, text and protected span must still match.
func rebindDisplayTranslations(saved, current DisplayTextCatalog, translations DisplayTranslations) (DisplayTranslations, error) {
	if err := translations.Validate(saved); err != nil {
		return DisplayTranslations{}, err
	}
	if err := current.Validate(); err != nil {
		return DisplayTranslations{}, err
	}
	if len(saved.Entries) != len(current.Entries) {
		return DisplayTranslations{}, fmt.Errorf("report: saved translations require the same complete display text catalogue")
	}
	key := func(entry DisplayTextEntry) string {
		entry.Ref = ""
		encoded, _ := json.Marshal(entry)
		return string(encoded)
	}
	byRef := make(map[string]DisplayTranslationEntry, len(translations.Entries))
	for _, entry := range translations.Entries {
		byRef[entry.Ref] = entry
	}
	byIdentity := make(map[string]DisplayTranslationEntry, len(saved.Entries))
	for _, entry := range saved.Entries {
		identity := key(entry)
		if _, exists := byIdentity[identity]; exists {
			return DisplayTranslations{}, fmt.Errorf("report: saved display text catalogue has an ambiguous entry identity")
		}
		byIdentity[identity] = byRef[entry.Ref]
	}
	result := DisplayTranslations{Version: translations.Version, Language: translations.Language, CatalogSHA256: current.SHA256, Entries: make([]DisplayTranslationEntry, 0, len(current.Entries))}
	for _, entry := range current.Entries {
		identity := key(entry)
		translated, exists := byIdentity[identity]
		if !exists {
			return DisplayTranslations{}, fmt.Errorf("report: current display text %s does not exactly match the saved catalogue", entry.Ref)
		}
		delete(byIdentity, identity)
		translated.Ref = entry.Ref
		result.Entries = append(result.Entries, translated)
	}
	if err := result.Validate(current); err != nil {
		return DisplayTranslations{}, err
	}
	return result, nil
}
