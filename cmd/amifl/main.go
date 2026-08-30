// Command amifl is the AmiFL language toolchain: it compiles .aml source
// through AMIVM-IR (via the external amivm CLI) and go build into a
// native executable.
//
// The source argument names either a single .aml file (its own
// independent one-file package, amifl-spec.md section 12.1), a directory
// (every .aml file directly inside it, merged into one package), or that
// directory's .amlz archive (step 15's `archive` subcommand output,
// section 16.2 — internal/modloader.Load treats it exactly like the
// directory it was produced from) — either way, modloader.Load resolves
// whatever `import` declarations it (transitively) reaches into the full
// package DAG (section 12.2-12.5) before compileToIR ever runs
// sema/codegen.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/amisonnet8/amifl/internal/modloader"
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
		return runArchive(rest)
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

Commands:

	build      compile to a native executable
	run        compile and immediately run, streaming its stdin/stdout/stderr
	emit-ir    compile to AMIVM-IR
	emit-go    compile to Go source (via amivm)
	archive    package a directory's .aml files into a .amlz archive
	help       show this help message

"amifl run <src> [program args...]" forwards any arguments after <src>
straight through to the compiled program as its own argv (like "go run
main.go arg1 arg2") — this is how a "fn main(args: List[String])" program
(amifl-spec.md section 14) receives them.

Flags (build, emit-ir, emit-go):

	-o <file>  output file path (default: derived from the input path)
	-v         show each pipeline stage's output as it runs

Flags (archive):

	-o <file>  output .amlz path (default: <directory-name>.amlz)

The source argument is a single .aml file (its own independent one-file
package), a directory (every .aml file directly inside it, merged into one
package), or that directory's .amlz archive (produced by "amifl archive",
usable anywhere a package-dir is) — imports it declares are resolved
automatically from other package directories or their own .amlz archives
(amifl-spec.md section 12).
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
		return "", fmt.Errorf("usage: amifl %s [-o file] [-v] <file.aml | package-dir | package.amlz>", fs.Name())
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

// runRun compiles srcPath and executes the resulting binary, forwarding any
// arguments after srcPath straight through as that program's own os.Args[1:]
// (mirroring `go run main.go arg1 arg2`) — this is how a `fn main(args:
// List[String])` program (amifl-spec.md section 14) receives them.
func runRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: amifl run <file.aml | package-dir | package.amlz> [program args...]")
	}
	srcPath, progArgs := args[0], args[1:]

	tmp, err := os.MkdirTemp("", "amifl-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	binPath := filepath.Join(tmp, "a.out")
	if err := compileToBinary(srcPath, binPath, false); err != nil {
		return err
	}

	runCmd := exec.Command(binPath, progArgs...)
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

	ir, _, err := compileToIR(srcPath)
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

// defaultOutPath derives an output path from srcPath and appending ext: for
// a single .aml file, its own base name with that extension stripped (step
// 1-13's original behavior, unchanged); for a package directory (step 14)
// or its .amlz archive (step 15), the directory/archive's own base name
// with any .amlz extension stripped — e.g. building "./myproject" (a
// directory) or "myproject.amlz" both default to "myproject" in the
// current directory, never the source path itself (which `go build -o`
// could never write an executable over, and which would otherwise leave a
// stray ".amlz" in every derived output name).
func defaultOutPath(srcPath, ext string) string {
	base := filepath.Base(filepath.Clean(srcPath))
	if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
		return base + ext
	}
	base = strings.TrimSuffix(base, modloader.AmlzExt)
	return strings.TrimSuffix(base, ".aml") + ext
}
