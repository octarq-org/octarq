package lab

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/octarq-org/octarq/server/agent/harness"
)

type Candidate struct {
	Name       string              `json:"name"`
	SourcePath string              `json:"source_path,omitempty"`
	Spec       harness.HarnessSpec `json:"spec"`
}

func LoadCandidateFile(path string) (Candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Candidate{}, fmt.Errorf("read candidate file %q: %w", path, err)
	}

	spec, err := harness.DecodeSpec(data)
	if err != nil {
		return Candidate{}, fmt.Errorf("candidate %q: %w", path, err)
	}

	return Candidate{
		Name:       spec.Name,
		SourcePath: path,
		Spec:       spec,
	}, nil
}

func LoadCandidates(targets []string) ([]Candidate, error) {
	if len(targets) == 0 {
		return nil, errors.New("no candidate targets specified")
	}

	var candidates []Candidate
	seenNames := make(map[string]string)

	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("candidate target %q: %w", target, err)
		}

		if info.IsDir() {
			entries, err := os.ReadDir(target)
			if err != nil {
				return nil, fmt.Errorf("read candidate dir %q: %w", target, err)
			}

			var jsonFiles []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
					jsonFiles = append(jsonFiles, filepath.Join(target, e.Name()))
				}
			}
			sort.Strings(jsonFiles)

			if len(jsonFiles) == 0 {
				return nil, fmt.Errorf("candidate dir %q contains no .json candidate files", target)
			}

			for _, f := range jsonFiles {
				cand, err := LoadCandidateFile(f)
				if err != nil {
					return nil, err
				}
				if prev, exists := seenNames[cand.Name]; exists {
					return nil, fmt.Errorf("duplicate candidate name %q in %q (previously in %q)", cand.Name, f, prev)
				}
				seenNames[cand.Name] = f
				candidates = append(candidates, cand)
			}
		} else {
			cand, err := LoadCandidateFile(target)
			if err != nil {
				return nil, err
			}
			if prev, exists := seenNames[cand.Name]; exists {
				return nil, fmt.Errorf("duplicate candidate name %q in %q (previously in %q)", cand.Name, target, prev)
			}
			seenNames[cand.Name] = target
			candidates = append(candidates, cand)
		}
	}

	if len(candidates) == 0 {
		return nil, errors.New("no candidates found in provided targets")
	}

	return candidates, nil
}

func FormatValidationError(err error) string {
	var valErr *harness.SpecValidationError
	if errors.As(err, &valErr) {
		var b strings.Builder
		b.WriteString("spec validation failed:\n")
		for _, v := range valErr.Violations {
			fmt.Fprintf(&b, "  - %s: %s\n", v.Field, v.Message)
		}
		return b.String()
	}
	return err.Error()
}
