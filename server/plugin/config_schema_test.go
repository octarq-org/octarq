package plugin_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

func intPtr(n int) *int {
	return &n
}

func TestConfigSchema_ToJSONSchema(t *testing.T) {
	schema := plugin.ConfigSchema{
		Title: "Test Plugin Settings",
		Fields: []plugin.ConfigField{
			{
				Name:     "apiKey",
				Label:    "API Key",
				Desc:     "Secret API Key for upstream service",
				Type:     plugin.FieldTypeString,
				Required: true,
				Secret:   true,
			},
			{
				Name:    "port",
				Label:   "Service Port",
				Type:    plugin.FieldTypeInt,
				Default: 8080,
				Min:     intPtr(1024),
				Max:     intPtr(65535),
			},
			{
				Name:  "mode",
				Type:  plugin.FieldTypeSelect,
				Enum:  []string{"fast", "standard", "thorough"},
				Label: "Operation Mode",
			},
		},
	}

	bytes, err := schema.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("unmarshal json schema failed: %v", err)
	}

	if parsed["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Fatalf("expected draft-07 schema, got %v", parsed["$schema"])
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object")
	}

	apiKeyProp, ok := props["apiKey"].(map[string]any)
	if !ok {
		t.Fatalf("missing apiKey property")
	}

	if secretVal, ok := apiKeyProp["x-secret"].(bool); !ok || !secretVal {
		t.Fatalf("expected apiKey x-secret to be true, got %v", apiKeyProp["x-secret"])
	}

	reqList, ok := parsed["required"].([]any)
	if !ok || len(reqList) != 1 || reqList[0] != "apiKey" {
		t.Fatalf("expected required to contain apiKey, got %v", parsed["required"])
	}
}

func TestConfigSchema_Validate_Required(t *testing.T) {
	schema := plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Name:     "serviceUrl",
				Type:     plugin.FieldTypeString,
				Required: true,
			},
			{
				Name:     "optionalDesc",
				Type:     plugin.FieldTypeString,
				Required: false,
			},
		},
	}

	// Missing serviceUrl
	err := schema.Validate(map[string]any{
		"optionalDesc": "testing",
	})
	if err == nil {
		t.Fatalf("expected error for missing required field, got nil")
		return
	}
	if !strings.Contains(err.Error(), "serviceUrl") {
		t.Fatalf("expected error to mention 'serviceUrl', got: %v", err)
	}

	// Empty string for required field
	err = schema.Validate(map[string]any{
		"serviceUrl": "   ",
	})
	if err == nil {
		t.Fatalf("expected error for empty required field, got nil")
		return
	}
	if !strings.Contains(err.Error(), "serviceUrl") {
		t.Fatalf("expected error to mention 'serviceUrl', got: %v", err)
	}

	// Present serviceUrl
	err = schema.Validate(map[string]any{
		"serviceUrl": "https://example.com",
	})
	if err != nil {
		t.Fatalf("expected valid schema to pass, got: %v", err)
	}
}

func TestConfigSchema_Validate_Range(t *testing.T) {
	schema := plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Name: "port",
				Type: plugin.FieldTypeInt,
				Min:  intPtr(1024),
				Max:  intPtr(65535),
			},
		},
	}

	// Exceed Max
	err := schema.Validate(map[string]any{
		"port": 99999,
	})
	if err == nil {
		t.Fatalf("expected error for port 99999 exceeding Max, got nil")
		return
	}
	if !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected error to mention 'port', got: %v", err)
	}

	// Below Min
	err = schema.Validate(map[string]any{
		"port": 80,
	})
	if err == nil {
		t.Fatalf("expected error for port 80 below Min, got nil")
		return
	}
	if !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected error to mention 'port', got: %v", err)
	}

	// In range
	err = schema.Validate(map[string]any{
		"port": 8080,
	})
	if err != nil {
		t.Fatalf("expected valid port to pass, got: %v", err)
	}

	// Float representation of int (e.g. from JSON decoder)
	err = schema.Validate(map[string]any{
		"port": float64(8080),
	})
	if err != nil {
		t.Fatalf("expected float64 8080 to pass, got: %v", err)
	}
}

func TestConfigSchema_Validate_Enum(t *testing.T) {
	schema := plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Name: "logLevel",
				Type: plugin.FieldTypeSelect,
				Enum: []string{"debug", "info", "warn", "error"},
			},
		},
	}

	// Invalid enum
	err := schema.Validate(map[string]any{
		"logLevel": "trace",
	})
	if err == nil {
		t.Fatalf("expected error for invalid enum 'trace', got nil")
		return
	}
	if !strings.Contains(err.Error(), "logLevel") {
		t.Fatalf("expected error to mention 'logLevel', got: %v", err)
	}

	// Valid enum
	err = schema.Validate(map[string]any{
		"logLevel": "debug",
	})
	if err != nil {
		t.Fatalf("expected valid enum to pass, got: %v", err)
	}
}

func TestConfigSchema_Validate_NilMinMax(t *testing.T) {
	schema := plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Name: "count",
				Type: plugin.FieldTypeInt,
				Min:  nil,
				Max:  nil,
			},
		},
	}

	// Should not panic with nil Min/Max
	err := schema.Validate(map[string]any{
		"count": 1000000,
	})
	if err != nil {
		t.Fatalf("expected nil Min/Max to pass, got: %v", err)
	}
}

func TestConfigSchema_Validate_AllNumericTypes(t *testing.T) {
	schema := plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Name: "count",
				Type: plugin.FieldTypeInt,
				Min:  intPtr(5),
				Max:  intPtr(20),
			},
			{
				Name: "flag",
				Type: plugin.FieldTypeBool,
			},
		},
	}

	types := []any{
		int8(10), int16(10), int32(10), int64(10),
		uint(10), uint8(10), uint16(10), uint32(10), uint64(10),
		float32(10), float64(10),
		json.Number("10"),
	}

	for _, v := range types {
		if err := schema.Validate(map[string]any{"count": v, "flag": true}); err != nil {
			t.Errorf("type %T (%v) failed validation: %v", v, v, err)
		}
	}

	// Invalid non-integer float
	if err := schema.Validate(map[string]any{"count": 10.5}); err == nil {
		t.Errorf("expected error for non-integer float 10.5")
	}

	// Invalid non-integer json.Number
	if err := schema.Validate(map[string]any{"count": json.Number("invalid")}); err == nil {
		t.Errorf("expected error for invalid json.Number")
	}

	// Invalid type (string for int field)
	if err := schema.Validate(map[string]any{"count": "not-an-int"}); err == nil {
		t.Errorf("expected error for string in int field")
	}

	// Invalid boolean
	if err := schema.Validate(map[string]any{"flag": "not-a-bool"}); err == nil {
		t.Errorf("expected error for invalid boolean")
	}
}
