// pipeline.go implements amifl-spec.md section 13.8's pipeline-DX helpers
// (design issue 8, step 15) — `tap`/`peek` are identity functions over an
// unconstrained T (any AmiFL value at all, including struct/tuple/List/
// Map/Set — never restricted to a single capability group the way most of
// section 13 is), so unlike collections.go's generics (called with an
// explicit type argument the sema side already resolved) they only need
// `T any`.
package amiflrt

import (
	"bufio"
	"fmt"
	"os"
)

// Tap is `tap(v, label) -> T` (section 13.8, "恒等関数＋ログ出力"): writes
// one line to stderr — never stdout, so it never mixes into a pipeline's
// own data output — and returns v unchanged. %v is Go's own generic
// formatter (fmt.Stringer-aware, reflective otherwise); it renders every
// AmiFL runtime representation reasonably (structs/tuples/slices/maps
// print their fields/elements) except a Func-typed value (prints as a
// bare pointer/address) and a Chan-typed one (prints as an opaque handle)
// — acceptable for a debug aid, not a value-formatting guarantee the way
// the (still-unimplemented, section 13.1) `format` built-in would need.
func Tap[T any](v T, label string) T {
	fmt.Fprintf(os.Stderr, "[%s] %v\n", label, v)
	return v
}

// Peek is `peek(v) -> T` (section 13.8, "開発モード限定の対話的インスペ
// クタ"): a pure passthrough — no I/O at all — unless the AMIFL_DEV
// environment variable is set to a non-empty value, in which case it
// prints v's dynamic type and value to stderr and blocks on one line of
// stdin before returning, so a developer can inspect a pipeline stage
// interactively without that inspection ever firing (or costing anything)
// in a production run that doesn't set the variable.
func Peek[T any](v T) T {
	if os.Getenv("AMIFL_DEV") == "" {
		return v
	}
	fmt.Fprintf(os.Stderr, "[peek] %T: %v\n", v, v)
	fmt.Fprint(os.Stderr, "(press Enter to continue) ")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return v
}
