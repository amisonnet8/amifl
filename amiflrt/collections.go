// collections.go implements amifl-spec.md section 13.4's data-manipulation
// built-ins that don't map onto a single AMIVM instruction or a bare Go
// stdlib call — mostly List[T]/Array[T;N] operations needing a loop or a
// closure callback. AmiFL's own surface language has no generics
// (principle 4), but amiflrt is ordinary Go, so these are plain Go generic
// functions — codegen calls them with explicit type arguments via AMIVM's
// `CALL result : ?amiflrt.Fn<<^T,^U>> args...` (amivm_spec.md section
// 4.13), giving each shape its own monomorphic instantiation without
// AmiFL's compiler needing to duplicate this logic per concrete type
// (CLAUDE.md's step-11 "確定した設計判断").
package amiflrt

import (
	"cmp"
	"slices"
	"strings"
)

// Contains reports whether v appears in xs (amifl-spec.md section 13.4,
// List/Array's contains(x, target)).
func Contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// IndexOf finds v's first position in xs (section 13.4's index(x, target)
// for List/Array) — the (value, ok) pair codegen assembles directly into a
// Tuple2[Int,Bool] (CLAUDE.md's established "amiflrt returns a native Go
// multi-value, codegen builds the Tuple2 STTYPE" convention).
func IndexOf[T comparable](xs []T, v T) (int64, bool) {
	for i, x := range xs {
		if x == v {
			return int64(i), true
		}
	}
	return 0, false
}

// MapSlice applies f to every element of xs (section 13.4's map(xs, f)).
func MapSlice[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// FilterSlice keeps only the elements of xs for which f is true (section
// 13.4's filter(xs, f)).
func FilterSlice[T any](xs []T, f func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

// Reduce folds xs into a single U, left to right (section 13.4's
// reduce(xs, init, f)) — f takes the accumulator first, then the element
// (fold-left convention).
func Reduce[T, U any](xs []T, init U, f func(U, T) U) U {
	acc := init
	for _, x := range xs {
		acc = f(acc, x)
	}
	return acc
}

// SortSlice returns a new, ascending-sorted copy of xs (section 13.4's
// sort(xs)) — never mutates xs itself (AmiFL's List operations are
// data-in/data-out, CLAUDE.md's established convention since step 6).
func SortSlice[T cmp.Ordered](xs []T) []T {
	out := slices.Clone(xs)
	slices.Sort(out)
	return out
}

// SortBySlice is SortSlice ordered by a derived key instead of the
// elements themselves (section 13.4's sortBy(xs, opt) — opt is a key-
// extraction closure `fn(T) -> K`, K itself Ordered).
func SortBySlice[T any, K cmp.Ordered](xs []T, key func(T) K) []T {
	out := slices.Clone(xs)
	slices.SortFunc(out, func(a, b T) int {
		return cmp.Compare(key(a), key(b))
	})
	return out
}

// ReverseSlice returns a new, reversed copy of xs (section 13.4's
// reverse(xs), the List case — Array uses its own fixed-size AMIVM loop
// instead, since a Go array's value semantics don't fit this generic
// slice-returning shape; String uses ReverseString below).
func ReverseSlice[T any](xs []T) []T {
	out := slices.Clone(xs)
	slices.Reverse(out)
	return out
}

// ReverseString reverses s by Unicode code point, not by byte (section
// 13.4's reverse(xs), the String case) — a plain byte-reversal would
// corrupt any multi-byte UTF-8 character.
func ReverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// Unique returns xs with every duplicate after the first occurrence
// removed, order preserved (section 13.4's unique(xs)).
func Unique[T comparable](xs []T) []T {
	seen := make(map[T]bool, len(xs))
	var out []T
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Flatten concatenates a List[List[T]] into a single List[T] (section
// 13.4's flatten(xs)). S (the outer list's own element type — AmiFL's
// synthesized List[T] Go type, a *named* slice, step 7) is a separate type
// parameter from E rather than xss simply being `[][]E`: our inner List[T]
// values are never plain unnamed `[]T` at the Go level (SLTYPE always
// mints a defined type — CLAUDE.md's step-10 "Set[Int]とMap[Int,Bool]は…
// 別のGo型" reasoning extends here too), and `~[]E` is what lets S be that
// named type while still telling Go it's slice-shaped over E.
func Flatten[S ~[]E, E any](xss []S) []E {
	var out []E
	for _, xs := range xss {
		out = append(out, xs...)
	}
	return out
}

// Push returns a new list with v appended (section 13.4's push(xs, v)) —
// clones first so appending never silently mutates another List value
// that happens to share xs's backing array (a real risk with Go's own
// append, which reuses spare capacity when there is any).
func Push[T any](xs []T, v T) []T {
	return append(slices.Clone(xs), v)
}

// Pop returns xs without its last element, and that last element itself
// (section 13.4's pop(xs)) — the two-value return codegen assembles into
// Tuple2[List[T],T], mirroring IndexOf above. Panics on an empty xs via
// Go's own slice-index bounds check, the same "let Go's native panic
// happen" policy AGET/ASET already rely on (CLAUDE.md's established
// convention since step 7 — no bespoke bounds checking here either).
func Pop[T any](xs []T) ([]T, T) {
	last := xs[len(xs)-1]
	return xs[:len(xs)-1], last
}

// Insert returns a new list with v inserted at position i (section 13.4's
// insert(xs, i, v)).
func Insert[T any](xs []T, i int64, v T) []T {
	out := make([]T, 0, len(xs)+1)
	out = append(out, xs[:i]...)
	out = append(out, v)
	out = append(out, xs[i:]...)
	return out
}

// RemoveAt returns a new list with the element at position i removed
// (section 13.4's removeAt(xs, i)).
func RemoveAt[T any](xs []T, i int64) []T {
	out := make([]T, 0, len(xs)-1)
	out = append(out, xs[:i]...)
	out = append(out, xs[i+1:]...)
	return out
}

// ConcatSlice concatenates two lists into a new one (section 13.4's
// concat(a, b), the List case — String/Bytes use AMIVM's own CONCAT
// instruction directly instead, CLAUDE.md's established Concatenable
// pattern since step 3).
func ConcatSlice[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

// StringIndex finds sub's first byte offset in s (section 13.4's
// index(x, target), the String case) — Go's strings.Index returns -1 on
// failure; this converts that into the (value, ok) pair codegen assembles
// into Tuple2[Int,Bool], the same convention IndexOf follows for List/
// Array.
func StringIndex(s, sub string) (int64, bool) {
	i := strings.Index(s, sub)
	if i < 0 {
		return 0, false
	}
	return int64(i), true
}
