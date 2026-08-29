// output.go implements the remainder of amifl-spec.md section 13.1
// (出力・終了) that ex6 adds — `print` itself stays codegen's own direct
// `?fmt.Println` call (no amiflrt helper needed, since fmt.Println's
// `...any` parameter already accepts any concrete Go value with no
// boxing/generics machinery of its own). eprint/format/formatWith are all
// plain `v any` (not `T any`) since none of them need to *return* v's own
// type the way pipeline.go's Tap/Peek do — they only ever read it once via
// %v, matching print/typeName's own precedent that no amiflrt generics are
// needed just to accept an Any-typed argument.
package amiflrt

import (
	"fmt"
	"os"
	"strings"
)

// Eprint is `eprint(v: Any) -> Unit` — print's own stderr counterpart:
// writes v's %v-formatted text plus a trailing newline to stderr instead
// of stdout, otherwise identical.
func Eprint(v any) {
	fmt.Fprintln(os.Stderr, v)
}

// Format is `format(v: Any) -> String` — the same %v rendering print/
// eprint write directly, returned as a value instead.
func Format(v any) string {
	return fmt.Sprintf("%v", v)
}

// FormatWith is `formatWith(template: String, v: Any) -> String`: replaces
// the first "{}" placeholder in template with v's %v-formatted text.
// Returns template unchanged if it contains no "{}" — never panics or
// errors over a missing placeholder, since a formatting helper misusing
// its own template shouldn't crash the program (mirroring
// FormatWith/Push's own defensive stance elsewhere in this package). Fill
// more than one placeholder by chaining calls (typically via `|>`), each
// one consuming the leftmost remaining "{}" — see sema/builtins_output.go's
// resolveFormatWith doc comment for why a single-placeholder-per-call
// design was chosen over a variadic/List[Any]-style multi-value form.
func FormatWith(template string, v any) string {
	idx := strings.Index(template, "{}")
	if idx < 0 {
		return template
	}
	return template[:idx] + fmt.Sprintf("%v", v) + template[idx+2:]
}
