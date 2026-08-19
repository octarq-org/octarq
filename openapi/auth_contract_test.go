package openapi_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/api"
	"github.com/octarq-org/octarq/openapi"
)

// These tests pin the published auth shapes to the structs they describe.
//
// They were written when the auth paths were hand-written rather than derived
// from the huma operations, so nothing stopped them from drifting: the spec
// documented a `username` login field for an API that has taken `email` since
// per-org identities landed, and anyone following the reference got a 400. The
// document is now generated from the live registrations, which makes that
// particular drift structurally impossible: the schema is read off the struct.
//
// They are kept because the failure they describe has a second route in. If an
// auth route stops being a huma operation, its generated path disappears and
// the document falls back to whatever openapi/extra_paths.go says — a hand
// entry that cannot override a live operation but does answer for a dead one.
// These fail loudly at that moment instead of publishing a stale shape.
//
// Generated schemas are emitted as components and referenced, so resolving
// $ref is part of reading them.

func specProps(t *testing.T, path, method string, steps ...string) []string {
	t.Helper()
	var buf bytes.Buffer
	if err := openapi.Generate(&buf, nil); err != nil {
		t.Fatalf("generate spec: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	node := any(doc["paths"].(map[string]any)[path].(map[string]any)[method])
	for _, step := range steps {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("%s %s: %q is not an object", method, path, step)
		}
		node, ok = m[step]
		if !ok {
			t.Fatalf("%s %s: no %q under %v", method, path, step, steps)
		}
		node = resolveRef(t, doc, node)
	}
	node = resolveRef(t, doc, node)
	props, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%s %s: %v is not a properties object", method, path, steps)
	}
	out := make([]string, 0, len(props))
	for k := range props {
		// huma stamps "$schema" onto every generated component schema as
		// JSON-Schema metadata. It is not a field the handler accepts or
		// returns, so it is not part of the contract being pinned here.
		if k == "$schema" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveRef follows a "$ref" into #/components/schemas so the assertions read
// the same shape a client would after dereferencing. A node that is not a $ref
// is returned unchanged.
func resolveRef(t *testing.T, doc map[string]any, node any) any {
	t.Helper()
	m, ok := node.(map[string]any)
	if !ok {
		return node
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return node
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		t.Fatalf("unsupported $ref %q", ref)
	}
	comps, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("$ref %q but the document has no components", ref)
	}
	schemas, ok := comps["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("$ref %q but components has no schemas", ref)
	}
	target, ok := schemas[strings.TrimPrefix(ref, prefix)]
	if !ok {
		t.Fatalf("$ref %q does not resolve", ref)
	}
	return target
}

func jsonFields(v any) []string {
	rt := reflect.TypeOf(v)
	out := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestLoginRequestSpecMatchesHandlerStruct(t *testing.T) {
	got := specProps(t, "/api/auth/login", "post", "requestBody", "content", "application/json", "schema", "properties")
	want := jsonFields(api.LoginInputBody{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("login request body documented as %v, handler accepts %v", got, want)
	}
}

func TestLoginResponseSpecMatchesHandlerStruct(t *testing.T) {
	got := specProps(t, "/api/auth/login", "post", "responses", "200", "content", "application/json", "schema", "properties")
	want := jsonFields(api.LoginOutputBody{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("login response documented as %v, handler returns %v", got, want)
	}
}

func TestMeResponseSpecMatchesHandlerStruct(t *testing.T) {
	got := specProps(t, "/api/auth/me", "get", "responses", "200", "content", "application/json", "schema", "properties")
	want := jsonFields(api.MeOutputBody{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("/api/auth/me documented as %v, handler returns %v", got, want)
	}
}
