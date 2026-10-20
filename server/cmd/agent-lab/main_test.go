package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
