package ontology

import (
	"encoding/json"
	"strings"

	core "github.com/octarq-org/octarq/server/ontology"
)

const toolPrefix = "octarq_data__"

func mcpToolName(slug string, op core.Operation) string {
	return toolPrefix + string(op) + "_" + slug
}

func inputSchema(e *entity, op core.Operation) map[string]any {
	props := map[string]any{}
	var required []string

	if op == core.OpList {
		props["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 200}
		props["offset"] = map[string]any{"type": "integer", "minimum": 0}
		props["sort"] = map[string]any{
			"type":        "string",
			"description": "Comma-separated field names. Prefix with - for descending.",
		}
		return map[string]any{
			"type":                 "object",
			"properties":           props,
			"additionalProperties": false,
		}
	}

	if op == core.OpGet || op == core.OpDelete {
		props["id"] = map[string]any{"type": "string", "description": "Record id."}
		return map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             []string{"id"},
			"additionalProperties": false,
		}
	}

	if op == core.OpUpdate {
		props["id"] = map[string]any{"type": "string", "description": "Record id."}
		for _, f := range e.spec.Fields {
			props[f.Name] = fieldSchema(f)
		}
		return map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             []string{"id"},
			"additionalProperties": false,
		}
	}

	for _, f := range e.spec.Fields {
		prop := fieldSchema(f)
		props[f.Name] = prop
		if op == core.OpCreate && f.Required && f.Default == nil {
			required = append(required, f.Name)
		}
	}
	out := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func fieldSchema(f core.Field) map[string]any {
	out := map[string]any{}
	if t := jsonType(f.Type); t != "" {
		out["type"] = t
	}
	if label := fieldLabel(f); label != f.Name {
		out["description"] = label
	}
	if f.Default != nil {
		if v, ok := typedDefault(f); ok {
			out["default"] = v
		}
	}
	return out
}

func jsonType(t core.FieldType) string {
	switch t {
	case core.TypeInteger:
		return "integer"
	case core.TypeNumber:
		return "number"
	case core.TypeBoolean:
		return "boolean"
	case core.TypeJSON:
		return ""
	default:
		return "string"
	}
}

func typedDefault(f core.Field) (any, bool) {
	if f.Default == nil {
		return nil, false
	}
	raw := strings.TrimSpace(*f.Default)
	switch f.Type {
	case core.TypeInteger:
		var n int64
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			return nil, false
		}
		return n, true
	case core.TypeNumber:
		var fl float64
		if err := json.Unmarshal([]byte(raw), &fl); err != nil {
			return nil, false
		}
		return fl, true
	case core.TypeBoolean:
		return raw == "true", true
	default:
		return *f.Default, true
	}
}
