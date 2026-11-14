package ontology_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/ontology"
)

const oneEntity = `{"entities":[{"slug":"tickets","label":"Ticket",
	"fields":[{"name":"title","type":"string","required":true}],
	"operations":{"list":{"roles":["owner"]},"get":{"roles":["owner"]},
	"create":{"roles":["owner"]},"update":{"roles":["owner"]},"delete":{"roles":["owner"]}}}]}`

func TestLoadPrefersInlineOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entities.json")
	if err := os.WriteFile(path, []byte(`{"entities":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	specs, err := ontology.Load(oneEntity, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 || specs[0].Slug != "tickets" {
		t.Fatalf("specs = %+v, want the inline document", specs)
	}
}

func TestLoadReadsExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entities.json")
	if err := os.WriteFile(path, []byte(oneEntity), 0o600); err != nil {
		t.Fatal(err)
	}
	specs, err := ontology.Load("", path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len = %d, want 1", len(specs))
	}
}

func TestLoadMissingDefaultFileMeansNoEntities(t *testing.T) {
	t.Chdir(t.TempDir())
	specs, err := ontology.Load("", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if specs != nil {
		t.Fatalf("specs = %+v, want nil", specs)
	}
}

func TestLoadMissingExplicitFileFails(t *testing.T) {
	_, err := ontology.Load("", filepath.Join(t.TempDir(), "absent.json"))
	if !errors.Is(err, ontology.ErrEntitiesFileMissing) {
		t.Fatalf("err = %v, want ErrEntitiesFileMissing", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := ontology.DecodeString(`{"entities":[{"slug":"tickets","label":"T","labl":"typo",
		"fields":[{"name":"a","type":"string"}],
		"operations":{"list":{"roles":["owner"]},"get":{"roles":["owner"]},
		"create":{"roles":["owner"]},"update":{"roles":["owner"]},"delete":{"roles":["owner"]}}}]}`)
	if err == nil {
		t.Fatal("expected an unknown-field error")
	}
}

func TestDecodeRejectsDuplicateSlug(t *testing.T) {
	_, err := ontology.DecodeString(`{"entities":[
		{"slug":"tickets","label":"T","fields":[{"name":"a","type":"string"}],"operations":{"list":{"roles":["owner"]},"get":{"roles":["owner"]},"create":{"roles":["owner"]},"update":{"roles":["owner"]},"delete":{"roles":["owner"]}}},
		{"slug":"tickets","label":"T2","fields":[{"name":"b","type":"string"}],"operations":{"list":{"roles":["owner"]},"get":{"roles":["owner"]},"create":{"roles":["owner"]},"update":{"roles":["owner"]},"delete":{"roles":["owner"]}}}]}`)
	assertIssues(t, err, ontology.CodeSlugDuplicate)
}

func TestDecodeRejectsRequireApproval(t *testing.T) {
	_, err := ontology.DecodeString(`{"entities":[{"slug":"tickets","label":"T",
		"fields":[{"name":"a","type":"string"}],
		"operations":{"list":{"roles":["owner"]},"get":{"roles":["owner"]},
		"create":{"roles":["owner"]},"update":{"roles":["owner"]},
		"delete":{"roles":["owner"],"requireApproval":true}}}]}`)
	assertIssues(t, err, ontology.CodeApprovalUnsupported)
}

func TestDecodeRejectsTrailingContent(t *testing.T) {
	if _, err := ontology.DecodeString(oneEntity + oneEntity); err == nil {
		t.Fatal("expected an error for trailing JSON")
	}
}

func TestDecodeRejectsEmptyEntityList(t *testing.T) {
	if _, err := ontology.DecodeString(`{"entities":[]}`); err == nil {
		t.Fatal("expected an error for an empty list; remove the file instead")
	}
}

// TestLoadAcceptsAnInlineEmptyDeclaration pins the one difference between the
// sources: a file that declares nothing is a mistake, while an inline document
// saying "nothing" is the operator's answer. openapi generation relies on it to
// keep runtime routes out of the published spec.
func TestLoadAcceptsAnInlineEmptyDeclaration(t *testing.T) {
	specs, err := ontology.Load(`{"entities":[]}`, "")
	if err != nil {
		t.Fatalf("Load = %v, want an empty declaration accepted", err)
	}
	if len(specs) != 0 {
		t.Fatalf("specs = %v, want none", specs)
	}
}

func TestDecodeRejectsDuplicateKey(t *testing.T) {
	_, err := ontology.DecodeString(`{"entities":[],"entities":[]}`)
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("Decode = %v, want the duplicate key refused", err)
	}
}
