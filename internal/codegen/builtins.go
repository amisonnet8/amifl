// builtins.go compiles amifl-spec.md section 13's built-in function
// library (step 11) — the codegen half of sema/builtins.go's dispatch
// (c.Builtin, c.ArgTypes, c.ResolvedTypeArg are all filled there; this file
// only ever reads them, never re-derives a type from the AST itself, same
// as every other sema/codegen split in this codebase).
package codegen

import (
	"fmt"
	"strconv"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genBuiltinValue dispatches a built-in call used as a value (its result
// type is non-Unit) to its own codegen. Reached only via genCallValue,
// which already checked c.Builtin != "".
func (g *gen) genBuiltinValue(c *ast.CallExpr) (string, error) {
	switch c.Builtin {
	case "isError":
		return g.genIsErrorValue(c)
	case "cast":
		return g.genCastValue(c)
	case "parse":
		return g.genParseValue(c)
	case "len":
		return g.genLenValue(c)
	case "slice":
		return g.genSliceBuiltinValue(c)
	case "at":
		return g.genAtValue(c)
	case "contains":
		return g.genContainsValue(c)
	case "index":
		return g.genIndexBuiltinValue(c)
	case "split":
		return g.genSplitValue(c)
	case "join":
		return g.genJoinValue(c)
	case "replace":
		return g.genReplaceValue(c)
	case "trim":
		return g.genStringUnaryValue(c, "?strings.TrimSpace")
	case "upper":
		return g.genStringUnaryValue(c, "?strings.ToUpper")
	case "lower":
		return g.genStringUnaryValue(c, "?strings.ToLower")
	case "startsWith":
		return g.genStringPredicateValue(c, "?strings.HasPrefix")
	case "endsWith":
		return g.genStringPredicateValue(c, "?strings.HasSuffix")
	case "map":
		return g.genMapValue(c)
	case "filter":
		return g.genFilterValue(c)
	case "reduce":
		return g.genReduceValue(c)
	case "sort":
		return g.genSortValue(c)
	case "sortBy":
		return g.genSortByValue(c)
	case "reverse":
		return g.genReverseValue(c)
	case "unique":
		return g.genUniqueValue(c)
	case "flatten":
		return g.genFlattenValue(c)
	case "zip":
		return g.genZipValue(c)
	case "push":
		return g.genPushValue(c)
	case "pop":
		return g.genPopValue(c)
	case "insert":
		return g.genInsertValue(c)
	case "removeAt":
		return g.genRemoveAtValue(c)
	case "concat":
		return g.genConcatValue(c)
	case "union":
		return g.genSetBinaryOpValue(c, "?amiflrt.UnionSet")
	case "intersect":
		return g.genSetBinaryOpValue(c, "?amiflrt.IntersectSet")
	case "difference":
		return g.genSetBinaryOpValue(c, "?amiflrt.DifferenceSet")
	case "toList":
		return g.genSetToListValue(c)
	case "keys":
		return g.genMapKeysValue(c)
	case "values":
		return g.genMapValuesValue(c)
	case "entries":
		return g.genMapEntriesValue(c)
	case "get":
		return g.genMapGetValue(c)
	case "min":
		return g.genMinMaxValue(c, "LT")
	case "max":
		return g.genMinMaxValue(c, "GT")
	case "abs":
		return g.genAbsValue(c)
	case "clamp":
		return g.genClampValue(c)
	case "round":
		return g.genFloatUnaryValue(c, "?math.Round")
	case "floor":
		return g.genFloatUnaryValue(c, "?math.Floor")
	case "ceil":
		return g.genFloatUnaryValue(c, "?math.Ceil")
	case "sqrt":
		return g.genFloatUnaryValue(c, "?math.Sqrt")
	case "pow":
		return g.genPowValue(c)
	case "unwrap":
		return g.genUnwrapValue(c)
	case "okOr":
		return g.genOkOrValue(c)
	case "chan":
		return g.genChanValue(c)
	case "recv":
		return g.genRecvValue(c)
	case "parallel":
		return g.genParallelValue(c)
	case "collect":
		return g.genCollectValue(c)
	case "take":
		return g.genTakeValue(c)
	case "skip":
		return g.genSkipValue(c)
	case "open":
		return g.genOpenValue(c)
	case "close":
		return g.genCloseValue(c)
	case "read":
		return g.genReadValue(c)
	case "readAll":
		return g.genReadAllValue(c)
	case "readLine":
		return g.genReadLineValue(c)
	case "lines":
		return g.genLinesValue(c)
	case "write":
		return g.genWriteValue(c)
	case "stdin", "stdout", "stderr":
		return g.genStdFileValue(c)
	default:
		return "", fmt.Errorf("codegen: built-in %q has no value-position codegen yet", c.Builtin)
	}
}

// genBuiltinStmt dispatches a built-in call used purely for effect — a
// Unit-returning built-in (setAt, and, since phase 11c, Set's add/discard
// and Map's set/delete — all mutate their first argument in place rather
// than producing a value), or a non-Unit one reached through a discard,
// which falls through to genBuiltinValue and discards the token
// (mirroring genStmt's own established "generate as a value, discard the
// result" pattern for every other non-Unit kind that can appear discarded
// — codegen.go's genStmt, the TupleLit/StructLit/... bucket).
func (g *gen) genBuiltinStmt(c *ast.CallExpr) error {
	switch c.Builtin {
	case "setAt":
		return g.genSetAtStmt(c)
	case "add":
		return g.genSetAddStmt(c)
	case "discard":
		return g.genSetDiscardStmt(c)
	case "set":
		return g.genMapSetStmt(c)
	case "delete":
		return g.genMapDeleteStmt(c)
	case "send":
		return g.genSendStmt(c)
	case "spawn":
		return g.genSpawnStmt(c)
	}
	_, err := g.genBuiltinValue(c)
	return err
}

// genIsErrorValue emits `isError(v)` (amifl-spec.md section 13.2): a bare
// Go `!= nil` check, since Error already compiles to Go's own `error`
// interface (codegen.go's goTypeNames) whose nil value *is* "no error" —
// no amiflrt helper needed.
func (g *gen) genIsErrorValue(c *ast.CallExpr) (string, error) {
	argVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", tmp)
	fmt.Fprintf(g.b, "\tNEQ\t%%%s\t%s\tnil\n", tmp, argVal)
	return "%" + tmp, nil
}

// genCastValue emits `cast[T](v)` (amifl-spec.md section 13.3) as a plain
// Go type conversion via CALL — the same `CALL %dest : ?<GoType> %src`
// pattern step 1's main/amifl_main bridge already established for the
// os.Exit boundary (CLAUDE.md's "確定した設計判断"): AMIVM's CALL is
// syntactically identical to a Go type conversion when the callname is a
// bare type name (CLAUDE.md's 過去に踏まれた地雷 #5), so no dedicated
// instruction or amiflrt helper is needed for a plain numeric conversion.
func (g *gen) genCastValue(c *ast.CallExpr) (string, error) {
	argVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeCall("%"+tmp, "?"+goType, []string{argVal})
	return "%" + tmp, nil
}

// parseTargetInfo maps parse[T]'s target type to the Go stdlib strconv
// function to call, the raw Go type that function itself returns (always
// int64/uint64/float64/bool — strconv has no narrower-width entry points),
// and the numeric base/bit-size argument(s) that call needs (amifl-spec.md
// section 13.3, "文字列→数値/真偽値へのパース"). sema's resolveParse has
// already restricted targetTyp to Numeric or Bool, so the default case
// here is unreachable in practice — it only guards against that invariant
// ever silently breaking.
func parseTargetInfo(targetTyp string) (parseFn, rawGoType string, bitSize int) {
	switch {
	case targetTyp == "Bool":
		return "?strconv.ParseBool", "bool", 0
	case isFloatType(targetTyp):
		bits := 64
		if targetTyp == "Float32" {
			bits = 32
		}
		return "?strconv.ParseFloat", "float64", bits
	case isUIntType(targetTyp):
		return "?strconv.ParseUint", "uint64", intBitSize(targetTyp)
	default: // signed int family
		return "?strconv.ParseInt", "int64", intBitSize(targetTyp)
	}
}

// isFloatType/isUIntType are codegen's own minimal copies of sema's
// identical classifiers (types.go) — ast is codegen's and sema's only
// shared vocabulary (CLAUDE.md's リポジトリ構成), so this classification
// has to be independently available here too, exactly like isTupleType
// above (structs.go) or isSetType/isMapType (maps.go).
func isFloatType(t string) bool {
	return t == "Float32" || t == "Float64"
}

func isUIntType(t string) bool {
	switch t {
	case "UInt8", "UInt16", "UInt32", "UInt64":
		return true
	}
	return false
}

func intBitSize(t string) int {
	switch t {
	case "Int8", "UInt8":
		return 8
	case "Int16", "UInt16":
		return 16
	case "Int32", "UInt32":
		return 32
	default: // Int64/UInt64
		return 64
	}
}

// genParseValue emits `parse[T](s)` (amifl-spec.md section 13.3): calls the
// matching strconv.ParseXxx function (parseTargetInfo), narrowing its
// always-64-bit (or bool) raw result down to T with one more CALL-as-
// conversion when T itself is narrower, then assembles the Tuple2[T,Error]
// result the same way errors.go's genTryValue's early-return path does —
// a VAR (F0 already the payload, no separate zero-value step needed since
// FSET writes it directly) plus one FSET for the error field.
func (g *gen) genParseValue(c *ast.CallExpr) (string, error) {
	argVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}

	targetTyp := c.ResolvedTypeArg
	parseFn, rawGoType, bitSize := parseTargetInfo(targetTyp)

	rawTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", rawTmp, rawGoType)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)

	var callArgs []string
	switch parseFn {
	case "?strconv.ParseBool":
		callArgs = []string{argVal}
	case "?strconv.ParseFloat":
		callArgs = []string{argVal, strconv.Itoa(bitSize)}
	default: // ParseInt/ParseUint
		callArgs = []string{argVal, "10", strconv.Itoa(bitSize)}
	}
	g.writeCallMulti([]string{"%" + rawTmp, "%" + errTmp}, parseFn, callArgs)

	payloadGoType := g.prog.resolveGoType(targetTyp)
	payloadVal := "%" + rawTmp
	if payloadGoType != rawGoType {
		castTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", castTmp, payloadGoType)
		g.writeCall("%"+castTmp, "?"+payloadGoType, []string{"%" + rawTmp})
		payloadVal = "%" + castTmp
	}

	tupleGoType := g.prog.resolveGoType(c.ResolvedType)
	tupleTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tupleTmp, tupleGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%s\n", tupleTmp, payloadVal)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%%%s\n", tupleTmp, errTmp)
	return "%" + tupleTmp, nil
}
