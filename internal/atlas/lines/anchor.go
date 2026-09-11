package lines

// AnchorEvidence preserves the selected original evidence for later reading
// decisions. A model's reason is stored separately and never becomes a fact.
// Refs are local to one request row: a member a row named by ref is restored
// to its complete record, so answers, learning and the report read the
// preserved evidence on its own.
func AnchorEvidence(chunk QuestionChunk, ref string) map[string]any {
	result := make(map[string]any)
	context := map[string]any{"path": chunk.Place.Path}
	for _, field := range chunk.Row.Fields {
		switch field.Name {
		case "anchor_options", "chunk", "chunks", "place_kind", "path":
			continue
		case "evidence":
			items := field.Value.([]map[string]any)
			units := make(map[string]map[string]any, len(items))
			for _, item := range items {
				if id, ok := item["ref"].(string); ok {
					units[id] = item
				}
			}
			var facts []map[string]any
			for _, item := range items {
				if ref != "file" && item["ref"] != ref {
					continue
				}
				copy := make(map[string]any)
				for key, value := range item {
					switch key {
					case "ref":
						continue
					case "owned_declarations":
						value = restoredOwnedDeclarations(value, units, chunk.Anchors)
					}
					copy[key] = value
				}
				facts = append(facts, copy)
			}
			result[field.Name] = facts
		default:
			context[field.Name] = field.Value
		}
	}
	result["context"] = context
	return result
}

func restoredOwnedDeclarations(value any, units map[string]map[string]any, anchors map[string]QuestionAnchor) any {
	records, ok := value.([]map[string]any)
	if !ok {
		return value
	}
	restored := make([]map[string]any, len(records))
	for i, record := range records {
		id, named := record["ref"].(string)
		if !named {
			restored[i] = record
			continue
		}
		unit, anchor := units[id], anchors[id]
		restored[i] = map[string]any{"path": anchor.Path, "line": anchor.Line, "name": unit["name"], "kind": unit["kind"], "signature": unit["signature"], "author_doc": unit["author_doc"]}
	}
	return restored
}
