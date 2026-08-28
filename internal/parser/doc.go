// Package parser turns the token stream produced by internal/lexer into an
// internal/ast tree. It is a hand-written recursive-descent parser
// (operator-precedence / Pratt parsing for expressions, per amifl-spec.md
// section 6's precedence table) — no parser generator is used, following
// Seed/Cascade/Weave's precedent (see CLAUDE.md).
package parser
