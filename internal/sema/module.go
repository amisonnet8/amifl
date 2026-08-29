// module.go implements step 14's cross-package reference support
// (amifl-spec.md section 12): the Exports a package's own CheckPackage run
// hands back for another package to import, and resolveFieldExpr's own
// qualified-reference branch (expr.go) that resolves `alias.Name` against
// an already-imported package's Exports. See sema.go's CheckPackage for
// the surrounding pass structure this plugs into.
package sema

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/amisonnet8/amifl/internal/ast"
)

// ExportedFunc is one exported (capitalized-name) top-level `fn` or extern
// bind, as another package may call it via `alias.Name(args...)` (amifl-
// spec.md section 12.2).
type ExportedFunc struct {
	Params []string
	Ret    string
	// Token is the full AMIVM callname another package's own codegen must
	// use to call this declaration — already fully resolved: "!" plus this
	// package's own rename prefix plus the declared name for a plain `fn`
	// (ast.ImportDecl's doc comment on that prefix), or an extern bind's
	// own already-package-qualified Go callname ("?alias.GoName")
	// verbatim, since that one is never subject to AmiFL-level renaming at
	// all (it names a Go declaration directly, independent of which AmiFL
	// package the bind happens to be declared in). Either way, an importer
	// never needs to know or recompute which of the two shapes this is —
	// codegen's genQualifiedCallValue/genQualifiedCallStmt (module.go) just
	// use it verbatim as CALL's callname operand.
	Token string
}

// ExportedConst is one exported top-level `const`, as another package may
// reference it via `alias.NAME` — Value is this package's own already
// fully sema-resolved initializer expression, ready to inline at the
// reference site exactly the way a same-package reference already does
// (ast.IdentExpr.ConstValue's identical convention — amifl-spec.md section
// 4, "参照箇所へインライン展開される"; a const has no runtime storage
// either way, same-package or cross-package).
type ExportedConst struct {
	Typ   string
	Value ast.Expr
}

// Exports is one package's public surface — amifl-spec.md section 12.2's
// "他パッケージから参照できるのは、名前が大文字で始まるトップレベル宣言
// だけ...値の種類（fn／struct／enum／const）は問わない". Struct/enum export
// is deliberately not modeled here (CLAUDE.md's step-14 "確定した設計判断":
// no `alias.Point{...}`/`alias.Status.Variant(...)` syntax exists yet to
// reach one — a struct/enum remains fully usable, just only within its own
// declaring package, until that syntax is added) — Funcs/Consts alone
// cover every qualified reference amifl-spec.md's own worked example
// (12.2's `mathutil.clamp(15, 0, 10)`) and step 14's parser actually let
// through (FieldExpr's IsQualifiedCall/QualifiedConstValue branches).
type Exports struct {
	Funcs  map[string]ExportedFunc
	Consts map[string]ExportedConst
}

// isExported reports whether name is visible from another package —
// amifl-spec.md section 12.2's "名前が大文字で始まる" rule, deliberately
// matching Go's own visibility convention (the spec's own stated reason:
// "実行基盤がGoである以上、生成物がそのまま正規のGoパッケージ群になるため、
// Goのツールチェーンとの親和性を優先した").
func isExported(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

// buildExports gathers c's own exported funcs/consts once CheckPackage has
// finished checking every declaration — called exactly once, at the very
// end of CheckPackage, so every signature/const value it reads is already
// fully resolved.
func (c *checker) buildExports(prefix string) Exports {
	ex := Exports{Funcs: map[string]ExportedFunc{}, Consts: map[string]ExportedConst{}}
	for name, sig := range c.funcs {
		if !isExported(name) {
			continue
		}
		// A method-style extern bind (sig.externMethod != "") has no fixed
		// callname token at all — METHVAL dispatch needs the receiver
		// *value* at each call site (CallExpr.ExternMethod's own doc
		// comment), which a qualified reference has no way to supply
		// (Target only ever names the import alias, never a receiver
		// expression). Rather than exporting a token that would silently
		// misresolve, such a bind is simply omitted from Exports — a
		// cross-package reference to it reports "no exported name", the
		// same clear error an unexported name would get.
		if sig.externMethod != "" {
			continue
		}
		token := sig.externCallee
		if token == "" {
			token = "!" + prefix + name
		}
		ex.Funcs[name] = ExportedFunc{Params: sig.params, Ret: sig.ret, Token: token}
	}
	for name, b := range c.globals {
		if !b.isConst || !isExported(name) {
			continue
		}
		ex.Consts[name] = ExportedConst{Typ: b.typ, Value: b.value}
	}
	return ex
}

// resolveQualifiedReference type-checks step 14's `alias.Name` (amifl-
// spec.md section 12.2) once resolveFieldExpr has already determined
// ident.Name is a known import alias — a function call
// (`alias.Name(args...)`, v.Args != nil) or a const reference
// (`alias.NAME`, v.Args == nil), told apart exactly the way
// resolveFieldExpr's other branches already are, by presence of a trailing
// `(...)`. Every function-call argument must be positional (Name == "") —
// principle 7, "名前付き引数無し" — parseFieldCallArgs's shared grammar
// with enum variant construction (which *requires* every argument named)
// means a mixed or fully-named argument list here is a genuine user
// mistake, not an enum construction that happened to target the wrong
// name, so it's reported as its own clear error rather than falling
// through to a confusing "wrong argument count"/"no such field" message.
func (fc *funcChecker) resolveQualifiedReference(v *ast.FieldExpr, alias string, pkg Exports) (string, error) {
	if fn, ok := pkg.Funcs[v.Field]; ok {
		if v.Args == nil {
			return "", fmt.Errorf("line %d: %s.%s is a function; call it with (...)", v.Line, alias, v.Field)
		}
		if len(v.Args) != len(fn.Params) {
			return "", fmt.Errorf("line %d: %s.%s expects %d argument(s), got %d", v.Line, alias, v.Field, len(fn.Params), len(v.Args))
		}
		argTypes := make([]string, len(v.Args))
		for i := range v.Args {
			a := &v.Args[i]
			if a.Name != "" {
				return "", fmt.Errorf("line %d: qualified function calls take positional arguments only, got named argument %q", a.Line, a.Name)
			}
			t, err := fc.checkExpr(a.Value, fn.Params[i])
			if err != nil {
				return "", err
			}
			argTypes[i] = t
		}
		v.IsQualifiedCall = true
		v.QualifiedCallee = fn.Token
		v.QualifiedArgTypes = argTypes
		v.ResolvedType = fn.Ret
		return fn.Ret, nil
	}
	if c, ok := pkg.Consts[v.Field]; ok {
		if v.Args != nil {
			return "", fmt.Errorf("line %d: %s.%s is a const, not a function", v.Line, alias, v.Field)
		}
		v.QualifiedConstValue = c.Value
		v.ResolvedType = c.Typ
		return c.Typ, nil
	}
	return "", fmt.Errorf("line %d: package %q has no exported name %q", v.Line, alias, v.Field)
}
