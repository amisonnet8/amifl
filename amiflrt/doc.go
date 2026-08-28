// Package amiflrt is AmiFL's own Go runtime: the implementation behind
// built-in functions and types that don't map onto a single AMIVM
// instruction (Stream[T]/Chan[T] helpers, File I/O, string/collection
// helpers not covered by native instructions, etc. — see amifl-spec.md
// section 13). It is distributed via go:embed — the amifl build pipeline
// copies its own source into a scratch Go module at build time so
// generated code can import it without any network access, the same
// pattern seed/seedrt, cascade/cascadert, and weave/weavert used (see
// CLAUDE.md's "独自のGoランタイムを呼ぶ").
package amiflrt
