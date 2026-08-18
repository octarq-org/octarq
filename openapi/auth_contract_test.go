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

// The auth paths in the published spec are hand-written rather than derived
// from the huma operations, so nothing stopped them from drifting: the spec
// documented a `username` login field for an API that has taken `email` since
// per-org identities landed, and anyone following the reference got a 400.
// These tests pin the hand-written shapes to the structs they describe.

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
	}
	props, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%s %s: %v is not a properties object", method, path, steps)
	}
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
