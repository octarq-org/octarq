package main

import (
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()
	if cmd.Use != "plugin-gen" {
		t.Errorf("Use = %q, want plugin-gen", cmd.Use)
	}
	// Ensure gen subcommand exists
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "gen" {
			found = true
			if c.Flags().Lookup("desc") == nil {
				t.Error("gen command missing --desc flag")
			}
			if c.Flags().Lookup("dir") == nil {
				t.Error("gen command missing --dir flag")
			}
		}
	}
	if !found {
		t.Error("gen subcommand not found")
	}
}

func TestNewRootCmd_ExecuteHelp(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("Execute --help failed: %v", err)
	}
	cmd.SetArgs([]string{"gen", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("Execute gen --help failed: %v", err)
	}
}

func TestNewRootCmd_ExecuteGen(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetArgs([]string{"gen", "sample-plugin", "--dir", tmpDir, "--desc", "Sample plugin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute gen failed: %v", err)
	}

	// Error case with invalid name
	cmdErr := newRootCmd()
	cmdErr.SetArgs([]string{"gen", "invalid_name", "--dir", tmpDir})
	if err := cmdErr.Execute(); err == nil {
		t.Errorf("expected error executing gen with invalid name")
	}
}
