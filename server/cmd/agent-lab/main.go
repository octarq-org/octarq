package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/octarq-org/octarq/server/agent/lab"
)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(val string) error {
	for _, part := range strings.Split(val, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent-lab", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var tasksPath string
	var candidateTargets stringSlice
	var outDir string
	var baselineName string

	fs.StringVar(&tasksPath, "tasks", "", "Path to the task set JSON file (required)")
	fs.Var(&candidateTargets, "candidates", "Path(s) to candidate HarnessSpec files or directory (required)")
	fs.StringVar(&outDir, "out", "", "Output directory to store report.json and report.md (required)")
	fs.StringVar(&baselineName, "baseline", "", "Name of the baseline candidate (optional, defaults to 'baseline' or 'default')")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: flag parsing failed: %v\n", err)
		return 1
	}

	// Capture any trailing positional arguments as candidate targets
	for _, arg := range fs.Args() {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			candidateTargets = append(candidateTargets, arg)
		}
	}

	// Fail closed: enforce mandatory inputs
	if strings.TrimSpace(tasksPath) == "" {
		fmt.Fprintln(stderr, "error: missing required flag: -tasks <task-set.json>")
		return 1
	}
	if len(candidateTargets) == 0 {
		fmt.Fprintln(stderr, "error: missing required flag: -candidates <dir-or-file>...")
		return 1
	}
	if strings.TrimSpace(outDir) == "" {
		fmt.Fprintln(stderr, "error: missing required flag: -out <report-dir>")
		return 1
	}

	// Load and validate task set
	ts, err := lab.LoadTaskSet(tasksPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to load task set: %v\n", err)
		return 2
	}

	// Load and validate candidate specs
	candidates, err := lab.LoadCandidates(candidateTargets)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to load candidates:\n%s\n", lab.FormatValidationError(err))
		return 3
	}

	// Execute offline evaluation
	ctx := context.Background()
	engine := lab.NewEngine(baselineName)
	report, err := engine.Run(ctx, ts, candidates, outDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: lab execution failed: %v\n", err)
		return 4
	}

	fmt.Fprintf(stdout, "Agent Lab Evaluation Completed Successfully.\n")
	fmt.Fprintf(stdout, "Report Directory: %s\n", outDir)
	fmt.Fprintf(stdout, "  JSON: %s\n", filepath.Join(outDir, "report.json"))
	fmt.Fprintf(stdout, "  Markdown: %s\n", filepath.Join(outDir, "report.md"))
	fmt.Fprintf(stdout, "Tasks: %d, Candidates: %d\n", report.TotalTasks, len(candidates))
	fmt.Fprintf(stdout, "Recommended Candidate: %s (%s)\n", report.RecommendedCandidate, report.RecommendationReason)

	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
