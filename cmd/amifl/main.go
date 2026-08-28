// Command amifl is the AmiFL language toolchain: it compiles .aml source
// through AMIVM-IR (via the external amivm CLI) and go build into a
// native executable.
//
// This is step 1 of CLAUDE.md's implementation plan (bootstrap): only a
// single parameter-less `fn main() -> Int { ... }`, whose body is
// print(String literal) calls followed by an Int literal, is supported.
// Package directories, .amlz archives, and the `archive` subcommand arrive
// with modules (step 14).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "build":
		return runBuild(rest)
	case "run":
		return runRun(rest)
	case "emit-ir":
		return runEmitIR(rest)
	case "emit-go":
		return runEmitGo(rest)
	case "archive":
		return fmt.Errorf("amifl archive: not implemented yet (modules land in a later step; see CLAUDE.md)")
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

	amifl <command> [flags] <file.aml>

Commands:

	build      compile to a native executable
	run        compile and immediately run, streaming its stdin/stdout/stderr
	emit-ir    compile to AMIVM-IR
	emit-go    compile to Go source (via amivm)
	archive    package a directory's .aml files into a .amlz archive (not implemented yet)
	help       show this help message

Flags (build, emit-ir, emit-go):

	-o <file>  output file path (default: derived from the input path)
	-v         show each pipeline stage's output as it runs

This is step 1 of CLAUDE.md's implementation plan (bootstrap): only a
single parameter-less "fn main() -> Int { ... }" whose body is
print(String literal) calls followed by an Int literal is supported.
`)
}

// outputFlags are the -o/-v flags shared by build, emit-ir, and emit-go.
func outputFlags(name string) (fs *flag.FlagSet, out *string, verbose *bool) {
	fs = flag.NewFlagSet(name, flag.ContinueOnError)
	out = fs.String("o", "", "output file path")
	verbose = fs.Bool("v", false, "show each pipeline stage's output as it runs")
	return fs, out, verbose
}

// parseOneSrcArg parses fs against args and returns the single required
// <file.aml> positional argument.
func parseOneSrcArg(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 1 {
		return "", fmt.Errorf("usage: amifl %s [-o file] [-v] <file.aml>", fs.Name())
	}
	return fs.Arg(0), nil
}

func runBuild(args []string) error {
	fs, out, verbose := outputFlags("build")
	srcPath, err := parseOneSrcArg(fs, args)
	if err != nil {
		return err
	}
	outPath := *out
	if outPath == "" {
		outPath = defaultOutPath(srcPath, "")
	}
	if err := compileToBinary(srcPath, outPath, *verbose); err != nil {
		return err
	}
	fmt.Println(outPath)
	return nil
}

func runRun(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: amifl run <file.aml>")
	}
	srcPath := args[0]

	tmp, err := os.MkdirTemp("", "amifl-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	binPath := filepath.Join(tmp, "a.out")
	if err := compileToBinary(srcPath, binPath, false); err != nil {
		return err
	}

	runCmd := exec.Command(binPath)
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func runEmitIR(args []string) error {
	fs, out, verbose := outputFlags("emit-ir")
	srcPath, err := parseOneSrcArg(fs, args)
	if err != nil {
		return err
	}

	ir, err := compileToIR(srcPath)
	if err != nil {
		return err
	}
	if *verbose {
		fmt.Println("=== AMIVM-IR ===")
		fmt.Print(ir)
	}

	outPath := *out
	if outPath == "" {
		outPath = defaultOutPath(srcPath, ".ir")
	}
	if err := os.WriteFile(outPath, []byte(ir), 0o644); err != nil {
		return err
	}
	fmt.Println(outPath)
	return nil
}

func runEmitGo(args []string) error {
	fs, out, verbose := outputFlags("emit-go")
	srcPath, err := parseOneSrcArg(fs, args)
	if err != nil {
		return err
	}

	goSrc, workDir, err := compileToGo(srcPath, *verbose)
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	outPath := *out
	if outPath == "" {
		outPath = defaultOutPath(srcPath, ".go")
	}
	if err := os.WriteFile(outPath, []byte(goSrc), 0o644); err != nil {
		return err
	}
	fmt.Println(outPath)
	return nil
}

// defaultOutPath derives an output path from srcPath by stripping its
// .aml extension and appending ext.
func defaultOutPath(srcPath, ext string) string {
	return strings.TrimSuffix(srcPath, ".aml") + ext
}
