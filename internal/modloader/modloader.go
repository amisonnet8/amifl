// Package modloader implements amifl-spec.md section 12's package loading:
// directory-as-package (12.1), `import alias "path"` cross-package
// resolution (12.2), the "root package only" entry-point rule (12.3), and
// the compile-time DAG requirements every package's own aliases must
// satisfy (12.4's global alias-uniqueness rule, 12.5's cycle prohibition).
//
// modloader's own responsibility ends at "parse every file, resolve every
// import path, hand back packages in dependency order" — it has no
// semantic knowledge at all (visibility, signatures, exported names all
// belong to sema.CheckPackage, called once per Package in the order Load
// returns). This mirrors CLAUDE.md's リポジトリ構成 layering: modloader
// depends on ast and parser, nothing depends on modloader but cmd/amifl.
package modloader

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
	"github.com/amisonnet8/amifl/internal/parser"
)

// AmlzExt is the extension amifl-spec.md section 16.2 gives a package's
// distributable archive form (the `archive` command's own output, step 15)
// — a plain zip (ArchiveDir, cmd/amifl/archive.go) holding exactly the
// producing directory's own direct .aml files, flat, no nested paths.
const AmlzExt = ".amlz"

// Package is one loaded AmiFL package: every .aml file sharing one
// directory (amifl-spec.md section 12.1), already parsed. Key uniquely
// identifies it for dedup (a diamond dependency reached via two different
// import paths that happen to resolve to the same directory) and cycle
// detection — the absolute, cleaned directory path, except in single-file
// root mode, where it's the absolute, cleaned file path itself (12.1's
// "コンパイル対象として単一ファイルを直接指定した場合は、そのファイル1つ
// だけの独立したパッケージとして扱う" — deliberately never merged with
// sibling .aml files in the same directory, so it needs its own identity
// distinct from "the directory as a whole").
type Package struct {
	Key   string
	Dir   string
	Files []*ast.File
	// Prefix is "" for the root package (amifl-spec.md section 12.4: "ルー
	// トパッケージ自身の宣言には、この名前の書き換えを行わない") or this
	// package's own canonical rename prefix otherwise — see loader.load's
	// doc comment on how it's chosen among possibly several aliases.
	Prefix string
	// Imports maps each `import alias "path"` this package's own files
	// declare to the imported package's Key — sema.CheckPackage needs this
	// (via the caller building its own alias->Exports map from Order,
	// looked up by Key) to resolve `alias.Name` qualified references.
	Imports map[string]string
}

// Load parses and resolves the whole package DAG rooted at srcPath (a
// directory or a single .aml file — amifl-spec.md sections 12.1/12.2),
// returning the root package plus Order: every package the program needs
// (the root included, always last), in dependency order — every package a
// given entry imports appears *before* it, so sema.CheckPackage can be run
// once per entry, left to right, always having every import's Exports
// already computed by the time it's needed.
func Load(srcPath string) (root *Package, order []*Package, err error) {
	l := &loader{
		byKey:         map[string]*Package{},
		aliasOwner:    map[string]string{},
		packagePrefix: map[string]string{},
		visiting:      map[string]bool{},
	}
	root, err = l.load(srcPath, true)
	if err != nil {
		return nil, nil, err
	}
	return root, l.order, nil
}

type loader struct {
	byKey    map[string]*Package
	order    []*Package
	visiting map[string]bool
	// aliasOwner enforces amifl-spec.md 12.4's global rule: one alias
	// string maps to at most one package across the *entire* program, not
	// just within one file — "同じエイリアスを2つの異なるパッケージへ割り
	// 当てようとすると、コンパイルエラーになる...別々のパッケージがそれぞ
	// れ独立に同じエイリアス名を選んでしまった場合も...拒否される".
	aliasOwner map[string]string
	// packagePrefix records, per package Key, the first alias any importer
	// anywhere in the program used to reach it — that becomes its one
	// canonical rename prefix (12.4's "mathutil_Clamp" example). The spec
	// doesn't directly address a package being reached via two genuinely
	// *different* aliases from two unrelated importers (only forbidding
	// the reverse: one alias, two different packages) — first-seen-wins,
	// deterministic given a fixed file/import declaration order, is this
	// loader's resolution for that otherwise-unspecified case.
	packagePrefix map[string]string
}

// load loads (or returns the already-cached) package rooted at fsPath — a
// directory always, except when isRoot and fsPath names a single .aml file
// directly (amifl-spec.md 12.1). Every import inside it is then resolved
// recursively, relative to fsPath's own directory (12.2's "パスは参照元
// ファイル自身のディレクトリからの相対パス" — every file in one package
// shares that same directory, so resolving against the package's directory
// once covers every one of its files identically).
func (l *loader) load(fsPath string, isRoot bool) (*Package, error) {
	absPath, err := filepath.Abs(fsPath)
	if err != nil {
		return nil, err
	}
	absPath = filepath.Clean(absPath)

	isArchive := strings.HasSuffix(absPath, AmlzExt)

	var dir string
	var singleFile string
	if !isArchive {
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			dir = absPath
		} else {
			if !isRoot {
				return nil, fmt.Errorf("import path %q must name a directory (package), not a file", fsPath)
			}
			dir = filepath.Dir(absPath)
			singleFile = absPath
		}
	} else {
		// A .amlz archive stands in for the directory it was produced from
		// (amifl-spec.md section 16.2, "package-dir（またはその.amlzアーカ
		// イブ）") — never the single-file branch above, even if it happens
		// to hold just one .aml file, since it's always the *complete*
		// curated set of a directory's own children (cmd/amifl/archive.go),
		// not one file selected out of a directory with ignored siblings.
		// Any `import "./x"` its own files declare resolves relative to the
		// directory the .amlz itself sits in, exactly as if it had been
		// unpacked there.
		dir = filepath.Dir(absPath)
	}

	key := dir
	if singleFile != "" {
		key = singleFile
	}
	if isArchive {
		key = absPath
	}

	// The cycle check must run *before* the cache-hit check below: a
	// package mid-load is already present in l.byKey (registered early,
	// right after its own directory is read — see below, needed so a
	// legitimate diamond dependency reusing an *already-finished* package
	// still hits the cache), so checking byKey first would let a cycle
	// silently "succeed" by handing back that same still-being-built
	// Package instead of erroring (amifl-spec.md section 12.5).
	if l.visiting[key] {
		return nil, fmt.Errorf("import cycle detected involving %s", key)
	}
	if pkg, ok := l.byKey[key]; ok {
		return pkg, nil
	}
	l.visiting[key] = true
	defer delete(l.visiting, key)

	// sources holds every file this package merges (amifl-spec.md section
	// 12.1) as (display label, already-read bytes) pairs — read up front and
	// uniformly across all three source kinds (single file / directory /
	// .amlz archive) so the parse loop below never needs to know which one
	// it came from.
	var sources []sourceFile
	switch {
	case singleFile != "":
		src, err := os.ReadFile(singleFile)
		if err != nil {
			return nil, err
		}
		sources = []sourceFile{{label: singleFile, data: src}}
	case isArchive:
		var err error
		sources, err = readArchiveAmlFiles(absPath)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", absPath, err)
		}
	default:
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".aml") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			p := filepath.Join(dir, name)
			src, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			sources = append(sources, sourceFile{label: p, data: src})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no .aml files found in %s", absPath)
	}

	pkg := &Package{Key: key, Dir: dir, Imports: map[string]string{}}
	l.byKey[key] = pkg

	var importDecls []*ast.ImportDecl
	for _, sf := range sources {
		f, err := parser.Parse(string(sf.data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sf.label, err)
		}
		pkg.Files = append(pkg.Files, f)
		for _, decl := range f.Decls {
			if imp, ok := decl.(*ast.ImportDecl); ok {
				importDecls = append(importDecls, imp)
			}
		}
	}

	for _, imp := range importDecls {
		targetPath := filepath.Join(dir, imp.Path)
		importedPkg, err := l.load(targetPath, false)
		if err != nil {
			return nil, fmt.Errorf("line %d: import %q: %w", imp.Line, imp.Path, err)
		}
		if owner, exists := l.aliasOwner[imp.Alias]; exists && owner != importedPkg.Key {
			return nil, fmt.Errorf("line %d: import alias %q is already bound to a different package (%s)", imp.Line, imp.Alias, owner)
		}
		l.aliasOwner[imp.Alias] = importedPkg.Key
		if _, has := l.packagePrefix[importedPkg.Key]; !has {
			l.packagePrefix[importedPkg.Key] = imp.Alias + "_"
			importedPkg.Prefix = imp.Alias + "_"
		}
		pkg.Imports[imp.Alias] = importedPkg.Key
	}

	if isRoot {
		pkg.Prefix = ""
	}
	l.order = append(l.order, pkg)
	return pkg, nil
}

// sourceFile is one already-read .aml file, with a label suitable for a
// parse-error message — a filesystem path for a directory/single-file
// package, or "archive.amlz:member.aml" for one read out of a .amlz.
type sourceFile struct {
	label string
	data  []byte
}

// readArchiveAmlFiles opens the .amlz (zip) file at absPath and reads back
// every top-level *.aml member, sorted by name for the same deterministic
// ordering os.ReadDir's own sort.Strings pass gives a real directory
// (CLAUDE.md's established "file exploration order is fixed and
// reproducible" requirement, section 12.4's canonical-prefix
// first-alias-wins rule depends on it). A member under a subdirectory
// inside the zip is ignored rather than rejected — cmd/amifl/archive.go
// never writes one, but silently accepting a stray nested entry (rather
// than erroring on an archive some other tool produced) costs nothing.
func readArchiveAmlFiles(absPath string) ([]sourceFile, error) {
	r, err := zip.OpenReader(absPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var names []string
	byName := map[string]*zip.File{}
	for _, f := range r.File {
		if f.FileInfo().IsDir() || strings.Contains(f.Name, "/") || !strings.HasSuffix(f.Name, ".aml") {
			continue
		}
		names = append(names, f.Name)
		byName[f.Name] = f
	}
	sort.Strings(names)

	sources := make([]sourceFile, 0, len(names))
	for _, name := range names {
		rc, err := byName[name].Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		sources = append(sources, sourceFile{label: absPath + ":" + name, data: data})
	}
	return sources, nil
}
