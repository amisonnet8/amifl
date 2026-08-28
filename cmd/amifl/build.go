package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/amisonnet8/amifl/internal/codegen"
	"github.com/amisonnet8/amifl/internal/parser"
	"github.com/amisonnet8/amifl/internal/sema"
)

// compileToIR reads and compiles srcPath down to AMIVM-IR text. Step 1
// only accepts a single .aml file — package directories and .amlz
// archives arrive with modules (see CLAUDE.md's implementation step
// plan).
func compileToIR(srcPath string) (string, error) {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", srcPath, err)
	}
	file, err := parser.Parse(string(src))
	if err != nil {
		return "", fmt.Errorf("%s: %w", srcPath, err)
	}
	if err := sema.Check(file); err != nil {
		return "", fmt.Errorf("%s: %w", srcPath, err)
	}
	ir, err := codegen.Generate(file)
	if err != nil {
		return "", fmt.Errorf("%s: %w", srcPath, err)
	}
	return ir, nil
}

// compileToGo runs compileToIR and then the external amivm CLI, returning
// the generated Go source and the scratch Go module directory it was
// written into — the caller must remove it. amivm requires its output
// directory to be a Go module (CLAUDE.md's "amivmのインストール・呼び出し方"),
// so a minimal go.mod is written alongside the IR file.
func compileToGo(srcPath string, verbose bool) (goSrc string, workDir string, err error) {
	ir, err := compileToIR(srcPath)
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
	modContent := "module amiflbuild\n\ngo 1.26.5\n"
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(modContent), 0o644); err != nil {
		cleanup()
		return "", "", err
	}

	goPath := filepath.Join(workDir, "main.go")
	args := []string{irPath, "-o", goPath}
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
