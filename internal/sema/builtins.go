// builtins.go type-checks amifl-spec.md section 13's built-in function
// library (step 11) — everything except "print" (still resolveCallExpr's
// own step-1 special case) and the handful of syntax-level built-ins that
// already had dedicated AST nodes before step 11 (`x[i]`/`x[i]=v`/
// `x[a:b]` -> at/setAt/slice, amifl-spec.md section 3.2 — CLAUDE.md's
// "確定した設計判断" for step 7 explains why those stay on their own nodes
// rather than funneling through here).
//
// Every built-in name in the *complete* section-13 list is reserved via
// builtinFuncs below, even ones step 11's approved scope doesn't implement
// yet (Stream/Chan-facing capabilities, file I/O, typeName — see CLAUDE.md's
// design-issue-6/-1 notes) — reserving the whole namespace up front means a
// name step 12/13 later gives real behavior to can never have quietly meant
// something else (an undefined-function error) in code written today; it
// instead gives a clear "not yet implemented" error pointing at the actual
// reason, self-documenting which step will pick it up.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// builtinResolver type-checks one built-in call (v.Args and, for the four
// generic ones, v.TypeArg already parsed but not yet resolved) and returns
// its result type. Each resolver is responsible for its own arity/type
// checking and for filling v.ArgTypes (and v.ResolvedTypeArg, for a generic
// one) — codegen/builtins.go's per-capability dispatch reads those back
// rather than re-deriving them.
type builtinResolver func(fc *funcChecker, v *ast.CallExpr) (string, error)

// notYetImplemented produces a builtinResolver that always reports a clear
// "reserved for a later step" error — see the package doc comment above.
func notYetImplemented(step, reason string) builtinResolver {
	return func(fc *funcChecker, v *ast.CallExpr) (string, error) {
		return "", fmt.Errorf("line %d: %q is a reserved built-in name not implemented until %s (%s)", v.Line, v.Callee, step, reason)
	}
}

// builtinFuncs is the complete section-13 built-in name registry (minus
// "print", handled directly in resolveCallExpr — see this file's package
// doc comment). Everything still out of step 11's approved scope routes
// through notYetImplemented so the name stays reserved without behaving
// like an ordinary undefined identifier. Populated by an init() rather than
// directly in this var's own initializer: every resolver function here
// transitively calls back into resolveBuiltinCall (via checkExpr on a call
// argument that might itself be a built-in call) which reads this same map
// — a literal initializer referencing those functions would make Go's
// initialization-order analysis see that as a self-referential dependency
// cycle (it inspects referenced functions' bodies, not just direct
// variable references) even though nothing is actually read until Check()
// runs, long after package initialization finishes.
var builtinFuncs map[string]builtinResolver

func init() {
	builtinFuncs = map[string]builtinResolver{
		// 13.1 出力・終了 — "print" is resolveCallExpr's own special case
		// (unchanged since step 1); eprint/format/formatWith/exit are
		// registered by builtins_output.go's own init() (ex6) rather than
		// listed directly here, the same split builtins_pipeline.go/
		// builtins_chan.go already use for their own sections.

		// 13.2 型・値判定
		"typeName": resolveTypeName,
		"isError":  resolveIsError,

		// 13.3 変換
		"cast":  resolveCast,
		"parse": resolveParse,

		// 13.9 エラー処理 (wired up in phase 11d)
		"unwrap": notYetImplemented("this step's later phase (11d)", "amifl-spec.md section 13.9"),
		"okOr":   notYetImplemented("this step's later phase (11d)", "amifl-spec.md section 13.9"),

		// 13.8 チャネル・ストリーム・並列 (wired up in step 12; tap/peek are
		// design issue 8's pipeline-DX features, explicitly deferred to
		// step 15 by CLAUDE.md's implementation plan table despite living in
		// this same spec section)
		"chan":     resolveChan,
		"send":     resolveSend,
		"recv":     resolveRecv,
		"spawn":    resolveSpawn,
		"parallel": resolveParallel,
		"collect":  resolveCollect,
		"take":     resolveTakeSkip("take"),
		"skip":     resolveTakeSkip("skip"),
		"tap":      notYetImplemented("step 15", "amifl-spec.md design issue 8, pipeline DX"),
		"peek":     notYetImplemented("step 15", "amifl-spec.md design issue 8, pipeline DX"),

		// 13.10 ファイルI/O (wired up in step 12)
		"open":     resolveOpen,
		"close":    resolveClose,
		"read":     resolveRead,
		"readAll":  resolveReadAll,
		"readLine": resolveReadLine,
		"lines":    resolveLines,
		"write":    resolveWrite,
		"stdin":    resolveStdFile,
		"stdout":   resolveStdFile,
		"stderr":   resolveStdFile,
	}
}

// resolveBuiltinCall checks whether v.Callee names a section-13 built-in
// (print excluded — resolveCallExpr's own caller already special-cases it
// first) and, if so, type-checks it fully and returns (type, true, err).
// Returns ("", false, nil) for any other name, letting resolveCallExpr fall
// through to its usual closure-variable/top-level-`fn` lookup — built-ins
// take priority over a same-named user declaration at a call site exactly
// the way "print" already does (CLAUDE.md's established precedent; a user
// `fn`/`let` sharing a built-in's name isn't rejected at declaration time,
// only shadowed at any call to it, unchanged from how "print" already
// behaves).
func (fc *funcChecker) resolveBuiltinCall(v *ast.CallExpr) (string, bool, error) {
	resolver, ok := builtinFuncs[v.Callee]
	if !ok {
		return "", false, nil
	}
	typ, err := resolver(fc, v)
	if err != nil {
		return "", true, err
	}
	v.Builtin = v.Callee
	v.ResolvedType = typ
	return typ, true, nil
}

// resolveTypeName type-checks `typeName(v: Any) -> String` (amifl-spec.md
// section 13.2, step 13) — v may be any value at all, not only one whose
// own static type already reads "Any": passing a concrete AmiFL value
// boxes it into Go's `any` at the call boundary exactly like an extern
// bind's own Any-typed parameter would (checkExpr's "Any" bypass), and
// codegen's %T-based implementation (codegen/extern.go's
// genTypeNameValue) reveals the underlying Go runtime type name either
// way — a genuinely `extern`-derived Any value's real dynamic type in the
// case the spec text describes, or just a concrete AmiFL type's own Go
// representation name (e.g. "int64") when called on an ordinary value.
func resolveTypeName(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", fmt.Errorf("line %d: typeName expects exactly 1 argument, got %d", v.Line, len(v.Args))
	}
	argTyp, err := fc.checkExpr(v.Args[0], "Any")
	if err != nil {
		return "", err
	}
	v.ArgTypes = []string{argTyp}
	return "String", nil
}

// resolveIsError type-checks `isError(v: Error) -> Bool` (amifl-spec.md
// section 13.2). The spec writes this as `(v: Any) -> Bool`, but — exactly
// like `print`'s own step-1 "確定した設計判断" — that's the "無制約な多相
// 引数, monomorphized at each call site" sense of Any, not the true dynamic
// `extern`-boundary Any (section 2.2); since Error is already its own
// concrete type, the natural monomorphic instantiation here is simply
// Error itself, not a genuinely unconstrained parameter.
func resolveIsError(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", fmt.Errorf("line %d: isError expects exactly 1 argument, got %d", v.Line, len(v.Args))
	}
	argTyp, err := fc.checkExpr(v.Args[0], "Error")
	if err != nil {
		return "", err
	}
	v.ArgTypes = []string{argTyp}
	return "Bool", nil
}

// resolveCast type-checks `cast[T](v: Numeric) -> T` (amifl-spec.md section
// 13.3) — T restricted to the Numeric capability (2.3節: Int系/UInt系/
// Float系), matching Go's own numeric-conversion rule this compiles
// directly to (codegen/builtins.go).
func resolveCast(fc *funcChecker, v *ast.CallExpr) (string, error) {
	targetTyp, err := fc.requireNumericTypeArg(v, "cast")
	if err != nil {
		return "", err
	}
	if len(v.Args) != 1 {
		return "", fmt.Errorf("line %d: cast expects exactly 1 argument, got %d", v.Line, len(v.Args))
	}
	argTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if !isIntType(argTyp) && !isFloatType(argTyp) {
		return "", fmt.Errorf("line %d: cast: argument must be a numeric type, got %s", v.Line, argTyp)
	}
	v.ArgTypes = []string{argTyp}
	return targetTyp, nil
}

// resolveParse type-checks `parse[T](s: String) -> Tuple2[T, Error]`
// (amifl-spec.md section 13.3) — T restricted to Numeric or Bool ("文字列
// →数値/真偽値へのパース").
func resolveParse(fc *funcChecker, v *ast.CallExpr) (string, error) {
	targetTyp, err := fc.requireTypeArg(v, "parse")
	if err != nil {
		return "", err
	}
	if !isIntType(targetTyp) && !isFloatType(targetTyp) && targetTyp != "Bool" {
		return "", fmt.Errorf("line %d: parse[%s]: T must be a numeric or Bool type", v.Line, targetTyp)
	}
	if len(v.Args) != 1 {
		return "", fmt.Errorf("line %d: parse expects exactly 1 argument, got %d", v.Line, len(v.Args))
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"String"}
	return makeTupleType([]string{targetTyp, "Error"}), nil
}

// requireTypeArg resolves v.TypeArg (already parsed, since v.Callee is one
// of parser.genericBuiltinNames) to its canonical type and records it on
// v.ResolvedTypeArg, erroring if the bracket was omitted entirely.
func (fc *funcChecker) requireTypeArg(v *ast.CallExpr, name string) (string, error) {
	if v.TypeArg == nil {
		return "", fmt.Errorf("line %d: %s requires an explicit type argument: %s[T](...)", v.Line, name, name)
	}
	typ, err := fc.resolveTypeExpr(v.TypeArg)
	if err != nil {
		return "", err
	}
	v.ResolvedTypeArg = typ
	return typ, nil
}

// requireNumericTypeArg is requireTypeArg plus the Numeric-capability check
// shared by cast[T] and any other built-in restricted to Int系/UInt系/
// Float系 (amifl-spec.md section 2.3).
func (fc *funcChecker) requireNumericTypeArg(v *ast.CallExpr, name string) (string, error) {
	typ, err := fc.requireTypeArg(v, name)
	if err != nil {
		return "", err
	}
	if !isIntType(typ) && !isFloatType(typ) {
		return "", fmt.Errorf("line %d: %s[%s]: T must be a numeric type", v.Line, name, typ)
	}
	return typ, nil
}
