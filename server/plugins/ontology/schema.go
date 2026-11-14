package ontology

import (
	"encoding/json"
	"strings"

	core "github.com/octarq-org/octarq/server/ontology"
)

// toolPrefix namespaces every generated tool. The protocol caps a tool name at
// 64 bytes: prefix (13) + operation + "_" + slug (at most 41) stays under it for
// the longest slug the pattern allows, which TestToolNamesFitProtocolLimit pins.
const toolPrefix = "octarq_data__"

// mcpToolName is the tool name for one operation on one entity. Underscores, not
// dots: strict MCP clients reject anything outside ^[a-zA-Z0-9_-]{1,64}$.
func mcpToolName(slug string, op core.Operation) string {
	return toolPrefix + string(op) + "_" + slug
}

// inputSchema renders one tool's arguments as a self-contained JSON Schema 2020-12
// document. Self-contained matters: MCP has no components section, so a $ref
// would dangle.
//
// The field list and the required list are real (unlike the HTTP body schema):
// an agent reads this to know what it may send, and getting that wrong is the
// whole cost of a generated tool.
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
		// An update names its target and sends only the fields that change, so
		// nothing but the id is required.
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

// fieldSchema renders one declared field. defaultValue is typed: the SDK
// validates the whole schema at AddTool and a string default on an integer
// property panics the server.
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

// jsonType maps a declared type to its JSON Schema type.
//
// A json field has no type on purpose. The column holds any JSON value — an
// object, an array, a number — and core.DecodeValues accepts all of them, so
// declaring "object" here would make the SDK refuse the array the server would
// happily store.
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

// typedDefault converts the declared literal into the JSON value the schema
// expects. A literal that does not parse is impossible to reach: load-time
// validation refuses it (core.CodeDefaultInvalid).
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
