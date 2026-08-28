// Package sema performs semantic analysis on an internal/ast tree: type
// checking, scope resolution, capability resolution (amifl-spec.md section
// 2.3), and pipeline type-flow checking. AMIVM itself does not validate
// types or scopes — that is delegated entirely to go/types — so sema is
// the only barrier standing between a user's mistake and a confusing
// generated-Go compiler error (see CLAUDE.md's "意味検証の責任分担").
package sema
