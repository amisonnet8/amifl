package modloader

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates each name->content pair as a file under dir (making
// parent directories as needed), returning dir itself for convenience.
func writeFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_SingleFileIsItsOwnPackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.aml":  "fn main() -> Int {\n    0\n}\n",
		"other.aml": "fn unused() -> Int {\n    1\n}\n",
	})

	root, order, err := Load(filepath.Join(dir, "main.aml"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(root.Files) != 1 {
		t.Fatalf("got %d files, want 1 (a directly-named file is its own package, ignoring siblings)", len(root.Files))
	}
	if len(order) != 1 {
		t.Fatalf("got %d packages in order, want 1", len(order))
	}
	if root.Prefix != "" {
		t.Fatalf("got Prefix %q, want \"\" for the root package", root.Prefix)
	}
}

func TestLoad_DirectoryMergesEveryAmlFileIntoOnePackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.aml":   "fn main() -> Int {\n    helper()\n}\n",
		"helper.aml": "fn helper() -> Int {\n    1\n}\n",
		"notes.txt":  "not an AmiFL file, must be ignored",
	})

	root, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(root.Files) != 2 {
		t.Fatalf("got %d files, want 2 (main.aml + helper.aml, notes.txt ignored)", len(root.Files))
	}
}

func TestLoad_ResolvesImportRelativeToImportingPackagesDirectory(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"main.aml":       "import mathutil \"./mathutil\"\nfn main() -> Int {\n    0\n}\n",
		"mathutil/u.aml": "fn Clamp(v: Int) -> Int {\n    v\n}\n",
	})

	rootPkg, order, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("got %d packages in order, want 2 (mathutil, then root)", len(order))
	}
	// Dependency order: the imported package must appear before its
	// importer, so sema.CheckPackage can process left to right.
	if order[len(order)-1] != rootPkg {
		t.Fatalf("expected the root package to be last in dependency order")
	}
	mathutilKey := rootPkg.Imports["mathutil"]
	if order[0].Key != mathutilKey {
		t.Fatalf("expected mathutil to be loaded before root")
	}
	if order[0].Prefix != "mathutil_" {
		t.Fatalf("got Prefix %q, want \"mathutil_\"", order[0].Prefix)
	}
}

func TestLoad_ImportCycleIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"root/main.aml": "import a \"../a\"\nfn main() -> Int {\n    0\n}\n",
		"a/a.aml":       "import b \"../b\"\nfn Foo() -> Int {\n    0\n}\n",
		"b/b.aml":       "import a \"../a\"\nfn Bar() -> Int {\n    0\n}\n",
	})

	if _, _, err := Load(filepath.Join(dir, "root")); err == nil {
		t.Fatal("expected an import-cycle error")
	}
}

func TestLoad_SameAliasBoundToTwoDifferentPackagesIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"root/main.aml": "import u \"../x\"\nimport u \"../y\"\nfn main() -> Int {\n    0\n}\n",
		"x/x.aml":       "fn Foo() -> Int {\n    1\n}\n",
		"y/y.aml":       "fn Bar() -> Int {\n    2\n}\n",
	})

	if _, _, err := Load(filepath.Join(dir, "root")); err == nil {
		t.Fatal("expected an error: alias \"u\" bound to two different packages")
	}
}

func TestLoad_DiamondDependencyReusesTheSamePackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"root/main.aml": "import a \"../a\"\nimport b \"../b\"\nfn main() -> Int {\n    0\n}\n",
		"a/a.aml":       "import shared \"../shared\"\nfn Foo() -> Int {\n    0\n}\n",
		"b/b.aml":       "import shared \"../shared\"\nfn Bar() -> Int {\n    0\n}\n",
		"shared/s.aml":  "fn Baz() -> Int {\n    0\n}\n",
	})

	_, order, err := Load(filepath.Join(dir, "root"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	seen := map[string]int{}
	for _, pkg := range order {
		seen[pkg.Key]++
	}
	for key, n := range seen {
		if n != 1 {
			t.Fatalf("package %s appears %d times in Order, want exactly once", key, n)
		}
	}
	if len(order) != 4 {
		t.Fatalf("got %d packages, want 4 (shared, a, b, root)", len(order))
	}
}
