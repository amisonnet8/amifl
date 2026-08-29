// builtins_chan.go type-checks amifl-spec.md sections 11/13.8's Chan[T]/
// Stream[T] built-ins and section 13.10's File built-ins — step 12. See
// CLAUDE.md's step-12 "確定した設計判断" for the runtime representation
// decisions (Chan[T]/Stream[T] as two distinct AMIVM channel types, File as
// an opaque *amiflrt.FileHandle) this file's resolvers assume.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// resolveChan type-checks `chan[T](buffer: Int) -> Chan[T]` (amifl-spec.md
// section 13.8). T has no argument to infer it from (buffer is just an
// Int), so — like cast[T]/parse[T] — it's one of the reserved names the
// parser accepts a bracketed type argument for (parser.genericBuiltinNames).
func resolveChan(fc *funcChecker, v *ast.CallExpr) (string, error) {
	elemTyp, err := fc.requireTypeArg(v, "chan")
	if err != nil {
		return "", err
	}
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"Int64"}
	return makeChanType(elemTyp), nil
}

// resolveSend type-checks `send(ch, v) -> Unit` (amifl-spec.md section
// 13.8).
func resolveSend(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	chTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := chanElemType(chTyp)
	if !ok {
		return "", fmt.Errorf("line %d: send: first argument must be a Chan[T], got %s", v.Line, chTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], elemTyp); err != nil {
		return "", err
	}
	v.ArgTypes = []string{chTyp, elemTyp}
	return unitType, nil
}

// resolveRecv type-checks `recv(ch) -> Tuple2[T, Bool]` (amifl-spec.md
// section 13.8).
func resolveRecv(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	chTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := chanElemType(chTyp)
	if !ok {
		return "", fmt.Errorf("line %d: recv: argument must be a Chan[T], got %s", v.Line, chTyp)
	}
	v.ArgTypes = []string{chTyp}
	return makeTupleType([]string{elemTyp, "Bool"}), nil
}

// resolveSpawn type-checks `spawn(f: fn() -> Unit) -> Unit` (amifl-spec.md
// section 11/13.8) — mirrors resolveMap/resolveFilter's own closure-typed-
// argument check (builtins_data.go), just against a fixed fn()->Unit shape
// instead of one derived from a collection's element type.
func resolveSpawn(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	fTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	params, ret, ok := funcTypeParts(fTyp)
	if !ok || len(params) != 0 || ret != unitType {
		return "", fmt.Errorf("line %d: spawn: f must be fn() -> Unit, got %s", v.Line, fTyp)
	}
	v.ArgTypes = []string{fTyp}
	return unitType, nil
}

// resolveParallel type-checks `parallel(s: Stream[T], workers: Int) ->
// Stream[T]` (amifl-spec.md section 11/13.8).
func resolveParallel(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	sTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if _, ok := streamElemType(sTyp); !ok {
		return "", fmt.Errorf("line %d: parallel: first argument must be a Stream[T], got %s", v.Line, sTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{sTyp, "Int64"}
	return sTyp, nil
}

// resolveCollect type-checks `collect(s: Stream[T]) -> List[T]` (amifl-
// spec.md section 13.8).
func resolveCollect(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	sTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := streamElemType(sTyp)
	if !ok {
		return "", fmt.Errorf("line %d: collect: argument must be a Stream[T], got %s", v.Line, sTyp)
	}
	v.ArgTypes = []string{sTyp}
	return makeListType(elemTyp), nil
}

// resolveTakeSkip type-checks `take`/`skip`'s shared shape, `(s: Stream[T],
// n: Int) -> Stream[T]` (amifl-spec.md section 13.8) — name is only used
// for the error message (v.Callee/v.Builtin already tell codegen's
// genBuiltinValue dispatch which of the two it is).
func resolveTakeSkip(name string) builtinResolver {
	return func(fc *funcChecker, v *ast.CallExpr) (string, error) {
		if len(v.Args) != 2 {
			return "", arityError(v, 2)
		}
		sTyp, err := fc.checkExpr(v.Args[0], "")
		if err != nil {
			return "", err
		}
		if _, ok := streamElemType(sTyp); !ok {
			return "", fmt.Errorf("line %d: %s: first argument must be a Stream[T], got %s", v.Line, name, sTyp)
		}
		if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
			return "", err
		}
		v.ArgTypes = []string{sTyp, "Int64"}
		return sTyp, nil
	}
}

// resolveOpen type-checks `open(path, mode) -> Tuple2[File, Error]`
// (amifl-spec.md section 13.10).
func resolveOpen(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"String", "String"}
	return makeTupleType([]string{"File", "Error"}), nil
}

// resolveClose type-checks `close(f) -> Error` (amifl-spec.md section
// 13.10).
func resolveClose(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File"}
	return "Error", nil
}

// resolveRead type-checks `read(f, n) -> Tuple2[Bytes, Error]` (amifl-
// spec.md section 13.10) — n is how many bytes to read at most.
func resolveRead(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File", "Int64"}
	return makeTupleType([]string{makeListType("UInt8"), "Error"}), nil
}

// resolveReadAll type-checks `readAll(f) -> Tuple2[Bytes, Error]` (amifl-
// spec.md section 13.10).
func resolveReadAll(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File"}
	return makeTupleType([]string{makeListType("UInt8"), "Error"}), nil
}

// resolveReadLine type-checks `readLine(f) -> Tuple2[String, Error]`
// (amifl-spec.md section 13.10).
func resolveReadLine(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File"}
	return makeTupleType([]string{"String", "Error"}), nil
}

// resolveLines type-checks `lines(f) -> Stream[String]` (amifl-spec.md
// section 13.10).
func resolveLines(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File"}
	return makeStreamType("String"), nil
}

// resolveWrite type-checks `write(f, data: Bytes) -> Tuple2[Int, Error]`
// (amifl-spec.md section 13.10) — Bytes canonicalizes to List(UInt8)
// (types.go's canonicalType), so the expected type here is spelled that way
// directly rather than via the "Bytes" surface name (which is never a
// canonical type string itself).
func resolveWrite(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], "File"); err != nil {
		return "", err
	}
	bytesTyp := makeListType("UInt8")
	if _, err := fc.checkExpr(v.Args[1], bytesTyp); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"File", bytesTyp}
	return makeTupleType([]string{"Int64", "Error"}), nil
}

// resolveStdFile type-checks `stdin`/`stdout`/`stderr` () -> File (amifl-
// spec.md section 13.10) — all three share this one resolver (v.Builtin,
// filled in by resolveBuiltinCall from v.Callee, is what tells codegen's
// genStdFileValue which of the three it's compiling).
func resolveStdFile(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 0 {
		return "", arityError(v, 0)
	}
	return "File", nil
}
