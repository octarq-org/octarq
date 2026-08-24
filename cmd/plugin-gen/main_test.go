package main

import (
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("newRootCmd returned nil")
	}
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
