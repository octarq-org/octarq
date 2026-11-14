package ontology

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxStringChars is the width of a string column on Postgres and MySQL.
const maxStringChars = 255

// Value issue codes. They share the Issue shape with declaration codes so one
// error type reaches the client for both.
const (
	CodeValueUnknown  = "VALUE_UNKNOWN"
	CodeValueReserved = "VALUE_RESERVED"
	CodeValueType     = "VALUE_TYPE"
	CodeValueRequired = "VALUE_REQUIRED"
	CodeValueInvalid  = "VALUE_INVALID"
)

func fieldIndex(spec Spec) map[string]Field {
	out := make(map[string]Field, len(spec.Fields))
	for _, f := range spec.Fields {
		out[f.Name] = f
	}
	return out
}

// DecodeValues parses a create body. Every declared field is checked; a missing
// required field without a default is refused.
func DecodeValues(spec Spec, raw []byte) (map[string]any, error) {
	return decode(spec, raw, true)
}

// DecodePatch parses an update body. Omitted fields keep their current value, so
// required fields are not demanded again; a field present but null is refused
// when the declaration says required.
func DecodePatch(spec Spec, raw []byte) (map[string]any, error) {
	return decode(spec, raw, false)
}

func decode(spec Spec, raw []byte, create bool) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, Errors{issue(CodeValueRequired, "body", "a JSON object is required")}
	}
	// encoding/json keeps the last of two keys with the same name and says
	// nothing. The caller who sent both believes the first one applied, so the
	// document is refused before it is decoded.
	if key, dup := DuplicateKey(raw); dup {
		return nil, Errors{issue(CodeValueInvalid, key, "%q is sent twice; send it once", key)}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var in map[string]any
	if err := dec.Decode(&in); err != nil {
		return nil, Errors{issue(CodeValueType, "body", "body is not a JSON object: %v", err)}
	}
	// A second JSON value after the object is refused, not ignored: the operator
	// sending it believes it was applied, and this package refuses the same shape
	// in a declaration file.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, Errors{issue(CodeValueType, "body", "body must contain exactly one JSON object")}
	}
	if in == nil {
		return nil, Errors{issue(CodeValueType, "body", "body is not a JSON object")}
	}

	fields := fieldIndex(spec)
	var errs Errors
	out := make(map[string]any, len(in))
	for name, v := range in {
		f, ok := fields[name]
		if !ok {
			if ReservedColumnNames[name] {
				errs = append(errs, issue(CodeValueReserved, name, "%q is managed by the platform", name))
				continue
			}
			errs = append(errs, issue(CodeValueUnknown, name, "%q is not a declared field", name))
			continue
		}
		if v == nil {
			if f.Required {
				errs = append(errs, issue(CodeValueRequired, name, "%q is required", name))
			}
			out[name] = nil
			continue
		}
		coerced, err := coerceValue(f, v)
		if err != nil {
			errs = append(errs, issue(CodeValueType, name, "%s", err.Error()))
			continue
		}
		out[name] = coerced
	}

	if create {
		for _, f := range spec.Fields {
			if !f.Required || f.Default != nil {
				continue
			}
			if _, ok := out[f.Name]; !ok {
				errs = append(errs, issue(CodeValueRequired, f.Name, "%q is required", f.Name))
			}
		}
	}
	if len(errs) > 0 {
		return nil, normalize(errs)
	}
	return out, nil
}

// DuplicateKey reports the first key a JSON document declares twice, at any
// depth. encoding/json resolves the collision silently by keeping the last
// value, which is the one behaviour a declaration and a request body must not
// have: the sender cannot tell which value was stored.
//
// A malformed document is left to the caller's own decode, which reports it with
// a position; this scan only answers the duplicate question.
func DuplicateKey(raw []byte) (string, bool) {
	type frame struct {
		object    bool
		expectKey bool
		seen      map[string]bool
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var stack []frame

	for {
		tok, err := dec.Token()
		if err != nil {
			return "", false
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				stack = append(stack, frame{object: true, expectKey: true, seen: map[string]bool{}})
			case '[':
				stack = append(stack, frame{})
			default: // '}' or ']'
				if len(stack) == 0 {
					return "", false
				}
				stack = stack[:len(stack)-1]
				// The container was its parent's value, which is now complete.
				if n := len(stack); n > 0 && stack[n-1].object {
					stack[n-1].expectKey = true
				}
			}
			continue
		}

		if len(stack) == 0 {
			return "", false
		}
		top := &stack[len(stack)-1]
		if !top.object {
			continue
		}
		if top.expectKey {
			key, ok := tok.(string)
			if !ok {
				return "", false
			}
			if top.seen[key] {
				return key, true
			}
			top.seen[key] = true
			top.expectKey = false
			continue
		}
		// A scalar value just ended; the next token is this object's key.
		top.expectKey = true
	}
}

// coerceValue turns a decoded JSON value into the Go value the column expects.
// encoding/json yields json.Number for numbers and string for everything with a
// textual column; the database drivers are far less forgiving than they look —
// Postgres rejects a float for BIGINT, and the SQLite driver rejects a map for
// a JSON column.
func coerceValue(f Field, v any) (any, error) {
	switch f.Type {
	case TypeString, TypeText:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%q must be a string", f.Name)
		}
		// A string column is VARCHAR(255) on Postgres and MySQL and TEXT on
		// SQLite. Without this check the same body is accepted on one dialect and
		// refused by the driver on the other two.
		if f.Type == TypeString && utf8.RuneCountInString(s) > maxStringChars {
			return nil, fmt.Errorf("%q is longer than %d characters", f.Name, maxStringChars)
		}
		return s, nil
	case TypeUUID:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%q must be a UUID string", f.Name)
		}
		parsed, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid UUID", f.Name)
		}
		// Store the canonical form. uuid.Parse accepts a 32-character hex string,
		// braces and upper case; storing the input verbatim would make two
		// spellings of one id two different keys, so a unique index would not see
		// the duplicate and the same record would differ across dialects.
		return parsed.String(), nil
	case TypeInteger:
		return toInt64(f, v)
	case TypeNumber:
		return toFloat64(f, v)
	case TypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%q must be true or false", f.Name)
		}
		return b, nil
	case TypeTimestamp:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%q must be an RFC 3339 timestamp", f.Name)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("%q must be an RFC 3339 timestamp", f.Name)
		}
		return t.UTC(), nil
	case TypeJSON:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("%q is not valid JSON", f.Name)
		}
		return string(raw), nil
	default:
		return nil, fmt.Errorf("%q has an unknown type", f.Name)
	}
}

func toInt64(f Field, v any) (any, error) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("%q must be a whole number", f.Name)
		}
		return i, nil
	case float64: // MCP tool arguments arrive as decoded JSON, not json.Number.
		if n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return nil, fmt.Errorf("%q must be a whole number", f.Name)
		}
		// A float64 holds every integer up to 2^53 and stops being able to tell
		// them apart after that. Accepting the value would store a number the
		// caller never sent.
		if math.Abs(n) > 1<<53 {
			return nil, fmt.Errorf("%q is out of range", f.Name)
		}
		return int64(n), nil
	case int64:
		return n, nil
	default:
		return nil, fmt.Errorf("%q must be a whole number", f.Name)
	}
}

func toFloat64(f Field, v any) (any, error) {
	switch n := v.(type) {
	case json.Number:
		fl, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("%q must be a number", f.Name)
		}
		return fl, nil
	case float64:
		return n, nil
	default:
		return nil, fmt.Errorf("%q must be a number", f.Name)
	}
}

// FromRow renders a stored row for the API. The reserved columns are exposed as
// id/created_at/updated_at; org_id is deliberately omitted, because the caller
// is already scoped to one tenant and echoing it invites a client to send it
// back.
func FromRow(spec Spec, row map[string]any) map[string]any {
	out := make(map[string]any, len(spec.Fields)+3)
	if v, ok := row[ColumnID]; ok {
		out[ColumnID] = asString(v)
	}
	for _, col := range []string{ColumnCreatedAt, ColumnUpdatedAt} {
		if v, ok := row[col]; ok {
			out[col] = asTimeString(v)
		}
	}
	for _, f := range spec.Fields {
		v, ok := row[f.Name]
		if !ok {
			out[f.Name] = nil
			continue
		}
		out[f.Name] = fromColumn(f, v)
	}
	return out
}

func fromColumn(f Field, v any) any {
	if v == nil {
		return nil
	}
	switch f.Type {
	case TypeBoolean:
		switch b := v.(type) {
		case bool:
			return b
		case int64:
			return b != 0
		case string:
			return strings.EqualFold(b, "true") || b == "1"
		}
		return v
	case TypeInteger:
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case []byte:
			if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
				return i
			}
		case string:
			if i, err := strconv.ParseInt(n, 10, 64); err == nil {
				return i
			}
		}
		return v
	case TypeNumber:
		switch n := v.(type) {
		case float64:
			return n
		case []byte:
			if fl, err := strconv.ParseFloat(string(n), 64); err == nil {
				return fl
			}
		}
		return v
	case TypeTimestamp:
		return asTimeString(v)
	case TypeJSON:
		switch raw := v.(type) {
		case []byte:
			return json.RawMessage(raw)
		case string:
			return json.RawMessage(raw)
		}
		return v
	default:
		return asString(v)
	}
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

func asTimeString(v any) any {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case string:
		return t
	case []byte:
		return string(t)
	}
	return v
}
