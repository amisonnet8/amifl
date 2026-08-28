// Command amifl is the AmiFL language toolchain: it will compile .aml
// source through AMIVM-IR (via the external amivm CLI) and go build into a
// native executable.
//
// This is the initial project scaffold (see CLAUDE.md's implementation
// plan) — the compiler pipeline (lexer/parser/ast/sema/codegen) has not
// been implemented yet, so every subcommand below just reports that.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		printUsage(os.Stderr)
		return fmt.Errorf("no command given")
	}

	cmd := args[0]
	switch cmd {
	case "build", "run", "emit-ir", "emit-go", "archive":
		return fmt.Errorf("amifl %s: not implemented yet (see CLAUDE.md implementation plan)", cmd)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `AmiFL is a compiler for the AmiFL programming language.

Usage:

	amifl <command> [flags] <file.aml | package-dir | package.amlz>

A package-dir (or a .amlz archive of one, amifl-spec.md 12 section) compiles
every .aml file in that package as one program; a single file compiles
only that file, ignoring any siblings in the same directory.

Commands:

	build      compile to a native executable
	run        compile and immediately run, streaming its stdin/stdout/stderr
	emit-ir    compile to AMIVM-IR
	emit-go    compile to Go source (via amivm)
	archive    package a directory's .aml files into a .amlz archive
	help       show this help message

None of the commands above are implemented yet — this is the initial
project scaffold. See CLAUDE.md for the implementation plan.
`)
}
