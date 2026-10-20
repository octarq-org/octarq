package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/agent/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_Success(t *testing.T) {
	t.Parallel()

	tsPath := filepath.Join("..", "..", "agent", "lab", "testdata", "tasks.example.json")
	candDir := filepath.Join("..", "..", "agent", "lab", "testdata", "candidates")
	outDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-tasks", tsPath,
		"-candidates", candDir,
		"-out", outDir,
	}, &stdout, &stderr)

	assert.Equal(t, 0, code, "CLI must exit with code 0 on success")
	assert.Contains(t, stdout.String(), "Agent Lab Evaluation Completed Successfully")
	assert.Contains(t, stdout.String(), "Recommended Candidate: candidate-optimized")
	assert.FileExists(t, filepath.Join(outDir, "report.json"))
	assert.FileExists(t, filepath.Join(outDir, "report.md"))
}

func TestCLI_MissingFlagsFailClosed(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	// Missing -tasks
	code := run([]string{"-candidates", "some/cand", "-out", "/tmp/out"}, &stdout, &stderr)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "missing required flag: -tasks")

	// Missing -candidates
	stderr.Reset()
	code = run([]string{"-tasks", "some/tasks.json", "-out", "/tmp/out"}, &stdout, &stderr)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "missing required flag: -candidates")

	// Missing -out
	stderr.Reset()
	code = run([]string{"-tasks", "some/tasks.json", "-candidates", "some/cand"}, &stdout, &stderr)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "missing required flag: -out")
}

func TestCLI_InvalidTaskSetFile(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-tasks", "/non/existent/tasks.json",
		"-candidates", filepath.Join("..", "..", "agent", "lab", "testdata", "candidates"),
		"-out", t.TempDir(),
	}, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "failed to load task set")
}

func TestCLI_InvalidCandidateFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	badCand := filepath.Join(tmpDir, "bad-cand.json")
	require.NoError(t, os.WriteFile(badCand, []byte(`{"version": 99, "name": "bad"}`), 0644))

	tsPath := filepath.Join("..", "..", "agent", "lab", "testdata", "tasks.example.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-tasks", tsPath,
		"-candidates", badCand,
		"-out", t.TempDir(),
	}, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "spec validation failed")
	assert.Contains(t, stderr.String(), "version: must be 1, got 99")
}

func TestCLI_Propose_Success(t *testing.T) {
	t.Parallel()

	tsPath := filepath.Join("..", "..", "agent", "lab", "testdata", "tasks.example.json")
	candDir := filepath.Join("..", "..", "agent", "lab", "testdata", "candidates")
	outDir := t.TempDir()
	propPath := filepath.Join(t.TempDir(), "proposal.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-tasks", tsPath,
		"-candidates", candDir,
		"-out", outDir,
		"-propose", propPath,
	}, &stdout, &stderr)

	assert.Equal(t, 0, code, "CLI must exit with code 0 on success")
	assert.Contains(t, stdout.String(), "Agent Lab Evaluation Completed Successfully")
	assert.Contains(t, stdout.String(), "Proposal: "+propPath)
	assert.FileExists(t, propPath)

	data, err := os.ReadFile(propPath)
	require.NoError(t, err)

	var doc struct {
		Name     string          `json:"name"`
		Spec     json.RawMessage `json:"spec"`
		Evidence struct {
			TaskSet   string `json:"taskSet"`
			Candidate string `json:"candidate"`
		} `json:"evidence"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "candidate-optimized", doc.Name)
	assert.Equal(t, "candidate-optimized", doc.Evidence.Candidate)
	assert.Equal(t, "example-ops-tasks", doc.Evidence.TaskSet)

	// Contract verification: spec inside proposal can be decoded by core
	decoded, err := harness.DecodeSpec(doc.Spec)
	require.NoError(t, err)
	assert.Equal(t, "candidate-optimized", decoded.Name)
	assert.Equal(t, 4, decoded.MaxSteps)
}

func TestCLI_Propose_DoesNotAffectReport(t *testing.T) {
	t.Parallel()

	tsPath := filepath.Join("..", "..", "agent", "lab", "testdata", "tasks.example.json")
	candDir := filepath.Join("..", "..", "agent", "lab", "testdata", "candidates")
	outDirWithoutProp := t.TempDir()
	outDirWithProp := t.TempDir()
	propPath := filepath.Join(t.TempDir(), "proposal.json")

	var stdout1, stderr1 bytes.Buffer
	code1 := run([]string{
		"-tasks", tsPath,
		"-candidates", candDir,
		"-out", outDirWithoutProp,
	}, &stdout1, &stderr1)
	require.Equal(t, 0, code1)

	var stdout2, stderr2 bytes.Buffer
	code2 := run([]string{
		"-tasks", tsPath,
		"-candidates", candDir,
		"-out", outDirWithProp,
		"-propose", propPath,
	}, &stdout2, &stderr2)
	require.Equal(t, 0, code2)

	// Compare report.json byte-for-byte
	json1, err := os.ReadFile(filepath.Join(outDirWithoutProp, "report.json"))
	require.NoError(t, err)
	json2, err := os.ReadFile(filepath.Join(outDirWithProp, "report.json"))
	require.NoError(t, err)
	assert.Equal(t, string(json1), string(json2), "report.json must be identical whether -propose is given or not")

	// Compare report.md byte-for-byte
	md1, err := os.ReadFile(filepath.Join(outDirWithoutProp, "report.md"))
	require.NoError(t, err)
	md2, err := os.ReadFile(filepath.Join(outDirWithProp, "report.md"))
	require.NoError(t, err)
	assert.Equal(t, string(md1), string(md2), "report.md must be identical whether -propose is given or not")

	assert.FileExists(t, propPath)
}

func TestCLI_Propose_NoRecommendedCandidate_FailsClosed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	badCand := filepath.Join(tmpDir, "bad-cand.json")
	require.NoError(t, os.WriteFile(badCand, []byte(`{"version": 99, "name": "bad"}`), 0644))

	tsPath := filepath.Join("..", "..", "agent", "lab", "testdata", "tasks.example.json")
	propPath := filepath.Join(tmpDir, "should-not-exist.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-tasks", tsPath,
		"-candidates", badCand,
		"-out", filepath.Join(tmpDir, "out"),
		"-propose", propPath,
	}, &stdout, &stderr)

	assert.NotEqual(t, 0, code, "CLI must exit with non-zero exit code on failure")
	assert.NoFileExists(t, propPath, "proposal file must not be created when candidates are invalid")
}
