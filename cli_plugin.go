package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/octarq-org/octarq/internal/scaffold"
)

// runPluginCommand handles `octarq plugin <subcommand>`. Today the only
// subcommand is `new`, which scaffolds a plugin skeleton. It returns the
// process exit code.
func runPluginCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: octarq plugin new <name> [flags]")
		return 2
	}
	switch args[0] {
	case "new":
		return runPluginNew(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "octarq plugin: unknown subcommand %q (try: new)\n", args[0])
		return 2
	}
}

func runPluginNew(args []string) int {
	fs := flag.NewFlagSet("octarq plugin new", flag.ContinueOnError)
	var opts scaffold.Options
	fs.StringVar(&opts.Dir, "dir", "", "output directory (default: octarq-plugin-<name>)")
	fs.StringVar(&opts.Module, "module", "", "Go module path (default: github.com/you/octarq-plugin-<name>)")
	fs.StringVar(&opts.NpmName, "npm", "", "npm package name (default: octarq-plugin-<name>)")
	fs.StringVar(&opts.Version, "octarq-version", "", "octarq module version for go.mod (default: "+scaffold.DefaultOctarqVersion+")")
	fs.BoolVar(&opts.Force, "force", false, "overwrite a non-empty output directory")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: octarq plugin new <name> [flags]")
		fmt.Fprintln(os.Stderr, "\nScaffolds a new octarq plugin (Go half + web half) into a directory.")
		fmt.Fprintln(os.Stderr)
		fs.PrintDefaults()
	}
	// Accept the name either first ("new <name> [flags]") or after the flags
	// ("new [flags] <name>"). Go's flag package stops at the first positional,
	// so we route the two orderings explicitly rather than mixing them.
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fs.Usage()
			return 2
		}
	} else {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if fs.NArg() != 1 {
			fs.Usage()
			return 2
		}
		name = fs.Arg(0)
	}
	opts.Name = name

	created, err := scaffold.Generate(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "octarq plugin new: %v\n", err)
		return 1
	}
	// Recompute the resolved dir for the summary (Generate defaults it internally).
	dir := opts.Dir
	if dir == "" {
		dir = "octarq-plugin-" + opts.Name
	}
	fmt.Printf("Scaffolded plugin %q in %s/\n", opts.Name, dir)
	for _, f := range created {
		fmt.Printf("  %s\n", f)
	}
	fmt.Printf("\nNext:\n  cd %s && go mod tidy && go build ./...\n", dir)
	return 0
}
