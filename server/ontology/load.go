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

const DefaultEntitiesFile = "octarq.entities.json"

var ErrEntitiesFileMissing = errors.New("ontology: entity file not found")

func Load(inline, path string) ([]Spec, error) {
	if inline = strings.TrimSpace(inline); inline != "" {
		return decodeDocument(strings.NewReader(inline), true)
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

func DecodeString(doc string) ([]Spec, error) { return Decode(strings.NewReader(doc)) }

func Decode(r io.Reader) ([]Spec, error) { return decodeDocument(r, false) }

func decodeDocument(r io.Reader, allowEmpty bool) ([]Spec, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("ontology: read entities: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("ontology: entities document is empty")
	}
	if key, dup := DuplicateKey(raw); dup {
		return nil, fmt.Errorf("ontology: parse entities: %q is declared twice", key)
	}

	var doc struct {
		Entities []Spec `json:"entities"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("ontology: parse entities: %w", err)
	}
	if err := decodeTrailing(dec); err != nil {
		return nil, err
	}
	if len(doc.Entities) == 0 {
		if allowEmpty {
			return nil, nil
		}
		return nil, errors.New(`ontology: "entities" is empty; remove the file or declare at least one entity`)
	}
	if err := ValidateAll(doc.Entities); err != nil {
		return nil, err
	}
	return doc.Entities, nil
}

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

func SortedSlugs(specs []Spec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Slug)
	}
	sort.Strings(out)
	return out
}
