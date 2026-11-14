package ontology

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// DefaultEntitiesFile is where the loader looks when neither the inline JSON nor
// an explicit path is configured. It is relative to the working directory, the
// same convention OCTARQ_DB_DSN's default ("octarq.db") uses.
const DefaultEntitiesFile = "octarq.entities.json"

// ErrEntitiesFileMissing is returned by Load when an explicitly named file does
// not exist. The default path is different: its absence means "this installation
// declares no entities", which is not an error.
var ErrEntitiesFileMissing = errors.New("ontology: entity file not found")

// Load resolves the declaration set the way the frontend manifest does:
//
//  1. inline JSON (OCTARQ_ENTITIES)
//  2. a file path (OCTARQ_ENTITIES_FILE)
//  3. ./octarq.entities.json
//
// It returns (nil, nil) when none of the three is present: an installation that
// declares no entities is valid. A source that IS present and cannot be parsed
// or validated is an error, never an empty set.
func Load(inline, path string) ([]Spec, error) {
	if inline = strings.TrimSpace(inline); inline != "" {
		return Decode(strings.NewReader(inline))
	}
	if path = strings.TrimSpace(path); path != "" {
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("%w: %s", ErrEntitiesFileMissing, path)
			}
			return nil, fmt.Errorf("ontology: open %s: %w", path, err)
		}
		defer f.Close()
		return Decode(f)
	}

	f, err := os.Open(DefaultEntitiesFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("ontology: open %s: %w", DefaultEntitiesFile, err)
	}
	defer f.Close()
	return Decode(f)
}

// DecodeString is Decode for a document already in memory.
func DecodeString(doc string) ([]Spec, error) { return Decode(strings.NewReader(doc)) }

// Decode parses a declaration document and validates the whole set.
func Decode(r io.Reader) ([]Spec, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("ontology: read entities: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("ontology: entities document is empty")
	}

	var doc struct {
		Entities []Spec `json:"entities"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// An unknown key is a typo the operator would otherwise never see: the
	// platform would silently ignore the field they thought they declared.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("ontology: parse entities: %w", err)
	}
	if err := decodeTrailing(dec); err != nil {
		return nil, err
	}
	if len(doc.Entities) == 0 {
		return nil, errors.New(`ontology: "entities" is empty; remove the file or declare at least one entity`)
	}
	if err := ValidateAll(doc.Entities); err != nil {
		return nil, err
	}
	return doc.Entities, nil
}

// decodeTrailing refuses a second JSON value after the document. A file whose
// second half is silently ignored is a file whose operator believes something
// that is not true.
func decodeTrailing(dec *json.Decoder) error {
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("ontology: trailing content after the entities document")
		}
		return fmt.Errorf("ontology: parse entities: %w", err)
	}
	return nil
}

// ValidateAll validates every declaration and the set as a whole.
func ValidateAll(specs []Spec) error {
	var errs Errors
	seen := make(map[string]int, len(specs))
	for i, s := range specs {
		if vErr := Validate(s); vErr != nil {
			var issues Errors
			if errors.As(vErr, &issues) {
				for _, iss := range issues {
					iss.Path = fmt.Sprintf("entities[%d].%s", i, iss.Path)
					errs = append(errs, iss)
				}
			} else {
				// Validate reports Issues today. An error it cannot express as one
				// is still a refusal, and dropping it would let a broken entity
				// through on the strength of the others being fine.
				return fmt.Errorf("ontology: validate entities[%d]: %w", i, vErr)
			}
		}
		if prev, dup := seen[s.Slug]; dup {
			errs = append(errs, issue(CodeSlugDuplicate, fmt.Sprintf("entities[%d].slug", i),
				"%q is already declared by entities[%d]", s.Slug, prev))
			continue
		}
		seen[s.Slug] = i
	}
	return normalize(errs)
}

// SortedSlugs returns the declared slugs in a stable order. Callers use it for
// menus and for error messages; map iteration order would make boot output
// differ between runs.
func SortedSlugs(specs []Spec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Slug)
	}
	sort.Strings(out)
	return out
}
