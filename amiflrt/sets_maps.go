// sets_maps.go implements amifl-spec.md section 13.5 (Set[T]) and 13.6
// (Map[K,V])'s built-ins that don't map onto a single AMIVM instruction —
// mostly set-algebra operations on Go's `map[T]bool` (step 10's
// established Set representation) and Map's values() (Go's own
// maps.Values needs no wrapper for keys() — MPKEYS already does exactly
// that, CLAUDE.md's step-10 note — but there's no MVALUES equivalent).
package amiflrt

// UnionSet returns a new set containing every element of a or b (amifl-
// spec.md section 13.5's union(a, b)).
func UnionSet[T comparable](a, b map[T]bool) map[T]bool {
	out := make(map[T]bool, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

// IntersectSet returns a new set containing only elements present in both
// a and b (amifl-spec.md section 13.5's intersect(a, b)).
func IntersectSet[T comparable](a, b map[T]bool) map[T]bool {
	out := make(map[T]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// DifferenceSet returns a new set containing a's elements that aren't in b
// (amifl-spec.md section 13.5's difference(a, b)).
func DifferenceSet[T comparable](a, b map[T]bool) map[T]bool {
	out := make(map[T]bool)
	for k := range a {
		if !b[k] {
			out[k] = true
		}
	}
	return out
}

// MapValues collects m's values into a slice, order unspecified (amifl-
// spec.md section 13.6's values(m)) — MPKEYS's own generated Go
// (slices.Collect(maps.Keys(m))) covers keys() directly as an AMIVM
// instruction; this is its values()-shaped counterpart, kept in amiflrt
// since there's no equivalent native instruction for it.
func MapValues[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
