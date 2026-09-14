package plugin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConfigFieldType defines the supported data types for plugin configuration fields.
type ConfigFieldType string

const (
	FieldTypeString ConfigFieldType = "string"
	FieldTypeInt    ConfigFieldType = "int"
	FieldTypeBool   ConfigFieldType = "bool"
	FieldTypeSelect ConfigFieldType = "select"

	// Alias constants matching naming variations
	ConfigFieldTypeString = FieldTypeString
	ConfigFieldTypeInt    = FieldTypeInt
	ConfigFieldTypeBool   = FieldTypeBool
	ConfigFieldTypeSelect = FieldTypeSelect
)

// ConfigField describes an individual configuration field within a plugin config schema.
type ConfigField struct {
	Name     string          `json:"name"`
	Label    string          `json:"label,omitempty"`
	Desc     string          `json:"desc,omitempty"`
	Type     ConfigFieldType `json:"type"`
	Required bool            `json:"required,omitempty"`
	Secret   bool            `json:"secret,omitempty"`
	Default  any             `json:"default,omitempty"`
	Enum     []string        `json:"enum,omitempty"`
	Min      *int            `json:"min,omitempty"`
	Max      *int            `json:"max,omitempty"`
}

// ConfigSchema defines the configuration schema for a plugin.
type ConfigSchema struct {
	Title  string        `json:"title,omitempty"`
	Fields []ConfigField `json:"fields"`
}

func normalizeFieldType(t ConfigFieldType) string {
	switch strings.ToLower(string(t)) {
	case "int", "integer":
		return "integer"
	case "bool", "boolean":
		return "boolean"
	case "select":
		return "select"
	default:
		return "string"
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		if float64(int(n)) == n {
			return int(n), true
		}
		return 0, false
	case float32:
		if float32(int(n)) == n {
			return int(n), true
		}
		return 0, false
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i), true
		}
		return 0, false
	default:
		return 0, false
	}
}

// ToJSONSchema generates a Draft-07 compliant JSON Schema representation of the ConfigSchema.
// Secret fields are tagged with the "x-secret": true extension property.
func (s ConfigSchema) ToJSONSchema() ([]byte, error) {
	properties := make(map[string]map[string]any)
	var required []string

	for _, f := range s.Fields {
		prop := make(map[string]any)
		normType := normalizeFieldType(f.Type)
		if normType == "select" {
			prop["type"] = "string"
		} else {
			prop["type"] = normType
		}

		if f.Label != "" {
			prop["title"] = f.Label
		}
		if f.Desc != "" {
			prop["description"] = f.Desc
		}
		if f.Default != nil {
			prop["default"] = f.Default
		}
		if len(f.Enum) > 0 {
			prop["enum"] = f.Enum
		}
		if f.Min != nil {
			prop["minimum"] = *f.Min
		}
		if f.Max != nil {
			prop["maximum"] = *f.Max
		}
		if f.Secret {
			prop["x-secret"] = true
		}

		properties[f.Name] = prop
		if f.Required {
			required = append(required, f.Name)
		}
	}

	schema := map[string]any{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"type":       "object",
		"properties": properties,
	}
	if s.Title != "" {
		schema["title"] = s.Title
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return json.Marshal(schema)
}

// Validate checks the provided data map against the ConfigSchema constraints (required, type, enum, min, max).
func (s ConfigSchema) Validate(data map[string]any) error {
	for _, f := range s.Fields {
		val, exists := data[f.Name]
		if !exists || val == nil {
			if f.Required {
				return fmt.Errorf("field %q is required", f.Name)
			}
			continue
		}

		if f.Required {
			if str, ok := val.(string); ok && strings.TrimSpace(str) == "" {
				return fmt.Errorf("field %q is required", f.Name)
			}
		}

		normType := normalizeFieldType(f.Type)
		switch normType {
		case "string", "select":
			strVal, ok := val.(string)
			if !ok {
				return fmt.Errorf("field %q must be a string", f.Name)
			}
			if len(f.Enum) > 0 {
				matched := false
				for _, item := range f.Enum {
					if item == strVal {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("field %q value is not in allowed enum list %v", f.Name, f.Enum)
				}
			}
		case "integer":
			intVal, ok := toInt(val)
			if !ok {
				return fmt.Errorf("field %q must be an integer", f.Name)
			}
			if f.Min != nil && intVal < *f.Min {
				return fmt.Errorf("field %q value %d is less than minimum %d", f.Name, intVal, *f.Min)
			}
			if f.Max != nil && intVal > *f.Max {
				return fmt.Errorf("field %q value %d exceeds maximum %d", f.Name, intVal, *f.Max)
			}
		case "boolean":
			_, ok := val.(bool)
			if !ok {
				return fmt.Errorf("field %q must be a boolean", f.Name)
			}
		}
	}
	return nil
}
