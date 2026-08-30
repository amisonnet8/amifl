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

// externImport is one deduped extern binding the program's compiled units
// declare, resolved down to exactly one of two forms: Path is a Go import
// path to hand amivm unchanged (a standard-library/module target, e.g.
// "time"), or — when the source `extern "..."` string starts with "."
// (isLocalExternPath) — LocalDir is instead the resolved absolute
// directory of hand-written .go files compileToGo must copy into the
// scratch module before amivm can be pointed at them (amifl-spec.md
// 15.3's "同じディレクトリに手書きの.goファイルを置き...extern束ねる";
// a bare relative/absolute filesystem path isn't a valid Go import string
// on its own, so it can never be handed to `-i` as-is the way a package
// name can).
type externImport struct {
	Alias    string
	Path     string
	LocalDir string
}

// externTarget renders whichever of externImport's two forms is set, for
// error messages (the "bound to both X and Y" collision check below).
func (e externImport) externTarget() string {
	if e.LocalDir != "" {
		return e.LocalDir
	}
	return e.Path
}

// isLocalExternPath reports whether path (an `extern "path"` string) names
// a local filesystem location rather than a Go standard-library/module
// import path — exactly Go's own convention for telling "./foo" and
// "../foo" apart from "strings" or "github.com/x/y" (a real Go import
// path never starts with "."), which amifl-spec.md 15.3 deliberately
// reuses rather than inventing a separate marker (CLAUDE.md principle 3;
// it also mirrors `import alias "./x"`'s own local-vs-package convention,
// section 12.2).
func isLocalExternPath(path string) bool {
	return path == "." || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")
}

// resolveLocalExternDir resolves a local `extern` path against pkgDir —
// the AmiFL package declaring it, since amifl-spec.md 15.3's "同じディレ
// クトリ" and 12.2's "パスは参照元ファイル自身のディレクトリからの相対
// パス" are the same rule applied to two different things (a hand-written
// .go file vs. an imported AmiFL package) — and confirms it names a real
// directory before compileToGo commits to copying anything out of it.
func resolveLocalExternDir(pkgDir, path string) (string, error) {
	dir := filepath.Clean(filepath.Join(pkgDir, path))
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("extern path %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("extern path %q resolves to %s, which is not a directory", path, dir)
	}
	return dir, nil
}

// compileToIR loads and compiles srcPath down to AMIVM-IR text, plus one
// externImport per distinct extern alias declared anywhere in the program
// (codegen.ExternImportMappings, step 13, plus this file's own local-path
// resolution on top of it, see isLocalExternPath) — compileToGo turns
// these into the amivm `-i alias=path` mappings it appends to the fixed
// amiflrt one it always passes, so every generated `?alias.Xxx`/METHVAL
// callname resolves deterministically. srcPath is a directory
// (amifl-spec.md section 12.1's package) or a single .aml file (its own
// independent one-file package); modloader.Load resolves every `import`
// it (transitively) declares into the full package DAG, in dependency
// order — sema.CheckPackage then runs once per package, left to right, so
// each import's Exports (step 14) are always ready by the time its
// importer needs them, and codegen.GenerateProgram compiles every
// package's own declarations into one combined AMIVM-IR program
// (section 12.4).
func compileToIR(srcPath string) (ir string, externImports []externImport, err error) {
	_, order, err := modloader.Load(srcPath)
	if err != nil {
		return "", nil, err
	}

	exportsByKey := map[string]sema.Exports{}
	var units []codegen.Unit
	externMap := map[string]externImport{}
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
			ei := externImport{Alias: alias, Path: path}
			if isLocalExternPath(path) {
				dir, err := resolveLocalExternDir(pkg.Dir, path)
				if err != nil {
					return "", nil, err
				}
				ei = externImport{Alias: alias, LocalDir: dir}
			}
			if existing, ok := externMap[alias]; ok && existing != ei {
				return "", nil, fmt.Errorf("extern alias %q is bound to both %q and %q across different packages", alias, existing.externTarget(), ei.externTarget())
			}
			externMap[alias] = ei
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
		externImports = append(externImports, externMap[alias])
	}
	return ir, externImports, nil
}

// copyLocalExternDir copies srcDir's own direct *.go files (non-recursive
// — a Go package, like an AmiFL package, amifl-spec.md section 12.1, is
// exactly one directory's own files, never a subtree) into dstDir, so the
// scratch Go module compileToGo builds in can resolve them as a local
// package. The declared `package` clause inside those files never has to
// match the extern block's own `as alias` — the generated code always
// references them through an explicit `import alias "..."`, which
// overrides whatever package name Go itself sees, exactly like amiflrt's
// own `import amiflrt "amiflbuild/amiflrt"` doesn't care that amiflrt.go
// happens to also declare `package amiflrt`.
func copyLocalExternDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	found := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		found = true
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("extern path resolves to %s, but it contains no .go files", srcDir)
	}
	return nil
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
	// localScratchNames dedupes local extern directories that end up
	// referenced by more than one alias (two different `extern "./x" as
	// a`/`as b` blocks, or the same relative path resolved from two
	// different packages) — copied into the scratch module exactly once,
	// under a fresh externN subdirectory each, in the same deterministic
	// alias-sorted order externImports already comes in.
	localScratchNames := map[string]string{}
	for _, ei := range externImports {
		path := ei.Path
		if ei.LocalDir != "" {
			name, ok := localScratchNames[ei.LocalDir]
			if !ok {
				name = fmt.Sprintf("extern%d", len(localScratchNames))
				if err := copyLocalExternDir(ei.LocalDir, filepath.Join(workDir, name)); err != nil {
					cleanup()
					return "", "", err
				}
				localScratchNames[ei.LocalDir] = name
			}
			path = scratchModule + "/" + name
		}
		args = append(args, "-i", ei.Alias+"="+path)
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
