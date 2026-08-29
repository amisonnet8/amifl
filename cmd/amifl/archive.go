package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amisonnet8/amifl/internal/modloader"
)

// runArchive implements `amifl archive` (amifl-spec.md section 16.2):
// bundles a package directory's own direct .aml files — never a
// subdirectory's, matching modloader's own "package = directory's direct
// children only" rule (section 12.1) — into a single .amlz file (a plain
// zip; readArchiveAmlFiles in internal/modloader reads it back). This is
// the write side of modloader.AmlzExt handling: build/run/emit-ir/emit-go
// already accept a .amlz wherever a package-dir is otherwise valid (as the
// top-level source argument or an import target) with no further change
// needed here, since modloader.Load branches on the extension itself.
func runArchive(args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	out := fs.String("o", "", "output .amlz path (default: <directory-name>.amlz)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: amifl archive [-o file.amlz] <package-dir>")
	}
	dir := fs.Arg(0)

	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("amifl archive: %q is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".aml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("amifl archive: no .aml files found in %s", dir)
	}

	outPath := *out
	if outPath == "" {
		outPath = filepath.Base(filepath.Clean(dir)) + modloader.AmlzExt
	}

	if err := writeArchive(outPath, dir, names); err != nil {
		return err
	}
	fmt.Println(outPath)
	return nil
}

// writeArchive writes dir/names[i] (each a direct child of dir) into a new
// zip at outPath, flat (member names only, no directory prefix) — exactly
// the shape internal/modloader's readArchiveAmlFiles expects back.
func writeArchive(outPath, dir string, names []string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			zw.Close()
			return err
		}
		w, err := zw.Create(name)
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(data); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}
