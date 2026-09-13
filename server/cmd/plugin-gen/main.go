package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "plugin-gen",
		Short: "Octarq plugin scaffolding generator",
		Long:  "plugin-gen generates new Octarq plugin boilerplate files conforming to the plugin contract.",
	}

	var desc string
	var dir string

	genCmd := &cobra.Command{
		Use:   "gen [name]",
		Short: "Generate a new Octarq plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := Scaffold(dir, name, desc); err != nil {
				return err
			}
			targetDir := name
			if dir != "" && dir != "." {
				targetDir = dir + "/" + name
			}
			fmt.Printf("Successfully scaffolded plugin %q in %s\n", name, targetDir)
			return nil
		},
	}

	genCmd.Flags().StringVarP(&desc, "desc", "d", "", "Plugin description")
	genCmd.Flags().StringVar(&dir, "dir", ".", "Output directory root")

	rootCmd.AddCommand(genCmd)
	return rootCmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
