// Package mathhelpers is a hand-written Go source file living in the same
// directory as main.aml — amifl-spec.md 15.3's "同じディレクトリに手書き
// の.goファイルを置き、そこで定義した関数をexternで束ねる" path, for Go
// logic too involved for a single stdlib bind (15.1/15.2's scope). The
// package name here is irrelevant to AmiFL: main.aml references this
// file's functions through its own extern block's `as` alias, not through
// this declared package name.
package mathhelpers

// GCD returns the greatest common divisor of a and b via the Euclidean
// algorithm.
func GCD(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}
