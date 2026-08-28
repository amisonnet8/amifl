// Package modloader resolves AmiFL packages and imports (amifl-spec.md
// section 12) before semantic checking: it merges one or more .aml files —
// a single file, a package directory, or a .amlz archive — into a single
// flat internal/ast tree, so internal/sema and internal/codegen never need
// to know that a program spanned multiple files or packages.
package modloader
