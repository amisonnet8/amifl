package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amisonnet8/amifl/amiflrt"
	"github.com/amisonnet8/amifl/internal/ast"
	"github.com/amisonnet8/amifl/internal/codegen"
	"github.com/amisonnet8/amifl/internal/modloader"
	"github.com/amisonnet8/amifl/internal/sema"
)

// scratchModule is the module name compileToGo's scratch Go module (its
// go.mod, below) is declared under. amiflrtImportPath is the resulting
// import path generated code uses for amiflrt (a subdirectory of that same
// module — Go resolves it automatically from the directory layout alone,
// no `require` needed), and amiflrtImportMapping is the amivm `-i`/
// `--import` flag value that tells amivm to emit that import whenever
// generated code references `?amiflrt.Xxx` (CLAUDE.md's "独自のGoランタイム
// を呼ぶ").
const scratchModule = "amiflbuild"

var amiflrtImportPath = scratchModule + "/amiflrt"
var amiflrtImportMapping = "amiflrt=" + amiflrtImportPath

// copyAmiflrt writes amiflrt's embedded source (amiflrt.Files) into
// dir/amiflrt, so the scratch Go module compileToGo builds in can resolve
// `import "amiflbuild/amiflrt"` locally — no network access, no dependency
// on this machine's GOPATH/module cache (CLAUDE.md's established amiflrt
// distribution plan, mirroring Seed/Cascade/Weave's seedrt/cascadert/
// weavert).
func copyAmiflrt(dir string) error {
	dstDir := filepath.Join(dir, "amiflrt")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	entries, err := amiflrt.Files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		data, err := amiflrt.Files.ReadFile(e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// compileToIR loads and compiles srcPath down to AMIVM-IR text, plus one
// amivm `-i alias=path` mapping string per distinct extern alias declared
// anywhere in the program (codegen.ExternImportMappings, step 13) —
// compileToGo appends these to the fixed amiflrt mapping it always passes,
// so every generated `?alias.Xxx`/METHVAL callname resolves
// deterministically. srcPath is a directory (amifl-spec.md section 12.1's
// package) or a single .aml file (its own independent one-file package);
// modloader.Load resolves every `import` it (transitively) declares into
// the full package DAG, in dependency order — sema.CheckPackage then runs
// once per package, left to right, so each import's Exports (step 14) are
// always ready by the time its importer needs them, and codegen.
// GenerateProgram compiles every package's own declarations into one
// combined AMIVM-IR program (section 12.4).
func compileToIR(srcPath string) (ir string, externImports []string, err error) {
	_, order, err := modloader.Load(srcPath)
	if err != nil {
		return "", nil, err
	}

	exportsByKey := map[string]sema.Exports{}
	var units []codegen.Unit
	externMap := map[string]string{}
	for _, pkg := range order {
		imports := map[string]sema.Exports{}
		for alias, key := range pkg.Imports {
			imports[alias] = exportsByKey[key]
		}
		exports, err := sema.CheckPackage(pkg.Files, pkg.Prefix, imports)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", pkg.Dir, err)
		}
		exportsByKey[pkg.Key] = exports

		var decls []ast.TopLevelDecl
		for _, f := range pkg.Files {
			decls = append(decls, f.Decls...)
		}
		units = append(units, codegen.Unit{Prefix: pkg.Prefix, Decls: decls})

		for _, mapping := range codegen.ExternImportMappings(decls) {
			alias, path, _ := strings.Cut(mapping, "=")
			if existing, ok := externMap[alias]; ok && existing != path {
				return "", nil, fmt.Errorf("extern alias %q is bound to both %q and %q across different packages", alias, existing, path)
			}
			externMap[alias] = path
		}
	}

	ir, err = codegen.GenerateProgram(units)
	if err != nil {
		return "", nil, err
	}

	aliases := make([]string, 0, len(externMap))
	for alias := range externMap {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		externImports = append(externImports, alias+"="+externMap[alias])
	}
	return ir, externImports, nil
}

// compileToGo runs compileToIR and then the external amivm CLI, returning
// the generated Go source and the scratch Go module directory it was
// written into — the caller must remove it. amivm requires its output
// directory to be a Go module (CLAUDE.md's "amivmのインストール・呼び出し方"),
// so a minimal go.mod is written alongside the IR file.
func compileToGo(srcPath string, verbose bool) (goSrc string, workDir string, err error) {
	ir, externImports, err := compileToIR(srcPath)
	if err != nil {
		return "", "", err
	}
	if verbose {
		fmt.Println("=== AMIVM-IR ===")
		fmt.Print(ir)
	}

	workDir, err = os.MkdirTemp("", "amifl-build-*")
	if err != nil {
		return "", "", err
	}
	cleanup := func() { os.RemoveAll(workDir) }

	irPath := filepath.Join(workDir, "main.ir")
	if err := os.WriteFile(irPath, []byte(ir), 0o644); err != nil {
		cleanup()
		return "", "", err
	}
	modContent := "module " + scratchModule + "\n\ngo 1.26.5\n"
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(modContent), 0o644); err != nil {
		cleanup()
		return "", "", err
	}
	if err := copyAmiflrt(workDir); err != nil {
		cleanup()
		return "", "", err
	}

	goPath := filepath.Join(workDir, "main.go")
	args := []string{irPath, "-o", goPath, "-i", amiflrtImportMapping}
	for _, mapping := range externImports {
		args = append(args, "-i", mapping)
	}
	if verbose {
		args = append(args, "-v")
	}
	cmd := exec.Command("amivm", args...)
	cmd.Dir = workDir
	out, runErr := cmd.CombinedOutput()
	if verbose || runErr != nil {
		os.Stdout.Write(out)
	}
	if runErr != nil {
		cleanup()
		return "", "", fmt.Errorf("amivm: %w", runErr)
	}

	goSrcBytes, err := os.ReadFile(goPath)
	if err != nil {
		cleanup()
		return "", "", err
	}
	return string(goSrcBytes), workDir, nil
}

// compileToBinary compiles srcPath all the way to a native executable at
// outPath.
func compileToBinary(srcPath, outPath string, verbose bool) error {
	_, workDir, err := compileToGo(srcPath, verbose)
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-o", absOut, ".")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		os.Stdout.Write(out)
	}
	if err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	return nil
}
