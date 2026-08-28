// Package ast defines the abstract syntax tree shared across the AmiFL
// compiler: it is the only vocabulary internal/parser, internal/sema, and
// internal/codegen have in common. internal/parser depends on
// internal/lexer and internal/ast; internal/sema and internal/codegen each
// depend only on internal/ast and never on each other (see CLAUDE.md's
// "リポジトリ構成" section — this layering follows Seed/Cascade/Weave).
package ast
