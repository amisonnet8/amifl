package sema

import "math"

// typeAliases maps AmiFL's convenience scalar-type aliases to their
// canonical name (amifl-spec.md section 2.1). Two type names denote the
// same type if and only if they canonicalize to the same string — e.g.
// "Int" and "Int64" are the same type, but "Int32" and "Int64" are not
// (principle 2: no implicit conversion between differently-sized or
// differently-signed numeric types).
var typeAliases = map[string]string{
	"Int":   "Int64",
	"UInt":  "UInt64",
	"Byte":  "UInt8",
	"Rune":  "Int32",
	"Float": "Float64",
}

// scalarTypes is the set of canonical scalar type names step 2 supports.
// Collections, struct/enum, Any, Error, and File (amifl-spec.md section
// 2.2) arrive in later steps.
var scalarTypes = map[string]bool{
	"Int8": true, "Int16": true, "Int32": true, "Int64": true,
	"UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true,
	"Float32": true, "Float64": true,
	"Bool": true, "String": true,
}

// unitType is the implicit type of a `let`/`const`/assignment/discard
// expression, and of a block whose value is discarded (amifl-spec.md
// principle 1). It intentionally isn't in scalarTypes — Unit isn't a
// type a user can write in a type annotation (there is no AmiFL syntax
// that would even parse one there), only an internal bookkeeping type.
const unitType = "Unit"

// canonicalType resolves a type-annotation identifier to its canonical
// scalar type name, or reports it as unknown.
func canonicalType(name string) (string, bool) {
	if alias, ok := typeAliases[name]; ok {
		name = alias
	}
	if !scalarTypes[name] {
		return "", false
	}
	return name, true
}

func isIntType(name string) bool {
	switch name {
	case "Int8", "Int16", "Int32", "Int64", "UInt8", "UInt16", "UInt32", "UInt64":
		return true
	}
	return false
}

func isFloatType(name string) bool {
	return name == "Float32" || name == "Float64"
}

// intLitMax is the largest value an IntLit can hold for each integer
// type. Step 2 has no unary minus yet (amifl-spec.md's arithmetic
// operators land in step 3), so every IntLit is non-negative and only an
// upper bound needs checking.
var intLitMax = map[string]uint64{
	"Int8":   math.MaxInt8,
	"Int16":  math.MaxInt16,
	"Int32":  math.MaxInt32,
	"Int64":  math.MaxInt64,
	"UInt8":  math.MaxUint8,
	"UInt16": math.MaxUint16,
	"UInt32": math.MaxUint32,
	"UInt64": math.MaxUint64,
}
