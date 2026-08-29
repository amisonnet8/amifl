// args.go implements the `!main` bridge's argv plumbing for
// amifl-spec.md section 14's `fn main(args: List[String]) -> Int` form —
// codegen.go's GenerateProgram calls Args() from the generated `!main`
// wrapper (ahead of the CALL into the user's own amifl_main) whenever the
// user's `fn main` declares the one-parameter form; the zero-parameter
// form never emits this call at all.
package amiflrt

import "os"

// Args returns the process's command-line arguments, excluding the
// program name itself (Go's os.Args[0]) — matching the usual C/Go/etc.
// convention that a program's own argv[0] isn't data the program's logic
// should have to filter out itself. Never nil (an AmiFL List[String]'s Go
// representation is a plain slice, and a nil slice both ranges and
// len()s identically to an empty one, but returning a real empty slice
// here — rather than relying on that — keeps this independent of exactly
// how AMIVM's own List[T] machinery happens to treat nil).
func Args() []string {
	if len(os.Args) <= 1 {
		return []string{}
	}
	return os.Args[1:]
}
