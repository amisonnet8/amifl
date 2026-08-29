// extern.go compiles amifl-spec.md section 15's `extern`/`bind` mechanism
// (step 13) — see ast.ExternDecl/ExternBindDecl's doc comments for the
// surface grammar and sema/extern.go for how a call resolves to either a
// plain package-level function (CallExpr.CalleeToken == "?alias.GoName")
// or a method-style bind (CallExpr.ExternMethod != "", amifl-spec.md
// section 15.2). genCallValue/genCallStmt in codegen.go route here for
// either case — see isExternCall.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// isExternCall reports whether c resolves to an extern bind (either
// shape) rather than a top-level `fn` call (CalleeToken == "" and
// ExternMethod == "") or a closure call (CalleeToken is a "%"/"$"/"&"
// value token, never "?" — sema never produces one of those for c.Callee
// resolving to a bind). CalleeToken alone can't distinguish "top-level fn"
// (empty) from "closure" (non-empty, but never "?"-prefixed) from "extern
// plain function" (non-empty, "?"-prefixed) without this helper, since
// codegen.go's own calleeToken() only special-cases the empty case.
func isExternCall(c *ast.CallExpr) bool {
	if c.ExternMethod != "" {
		return true
	}
	return len(c.CalleeToken) > 0 && c.CalleeToken[0] == '?'
}

// externCallee returns the CALL/METHVAL+CALL callname and the argument
// expressions to evaluate for c — for a method-style bind, this means
// extracting c.Args[0]'s value as the METHVAL receiver and shrinking the
// argument list to c.Args[1:] (the receiver never appears twice — amivm's
// METHVAL already binds it into the returned callable value, exactly the
// way a Go method value `t.Unix` needs no further receiver argument at its
// own call site).
func (g *gen) externCallee(c *ast.CallExpr) (callee string, argExprs []ast.Expr, err error) {
	if c.ExternMethod == "" {
		return c.CalleeToken, c.Args, nil
	}
	recvVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", nil, err
	}
	mTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tMETHVAL\t%%%s\t%s\t<%s\n", mTmp, recvVal, c.ExternMethod)
	return "%" + mTmp, c.Args[1:], nil
}

// genExternArgValues is genArgValues plus boxIntLiteralForAny's guard on
// every position c.ExternParamTypes marks "Any" — paramTypes is c's own
// ExternParamTypes, already shrunk to line up with argExprs the same way
// externCallee shrinks c.Args itself for a method-style bind (both drop
// index 0, the receiver, together).
func (g *gen) genExternArgValues(argExprs []ast.Expr, paramTypes []string) ([]string, error) {
	vals := make([]string, len(argExprs))
	for i, a := range argExprs {
		v, err := g.genValue(a)
		if err != nil {
			return nil, err
		}
		if i < len(paramTypes) && paramTypes[i] == "Any" {
			v = g.boxIntLiteralForAny(v)
		}
		vals[i] = v
	}
	return vals, nil
}

// boxIntLiteralForAny counters a Go gotcha at the one point AmiFL code can
// actually trigger it: a bare integer-literal *argument expression* (or a
// `const` reference chasing down to one — genValue's IdentExpr case
// already dereferences that to the identical literal token) passed
// directly to an extern bind's Any-typed parameter, or to typeName's own
// argument (codegen/builtins.go's genTypeNameValue, unconditionally "Any"
// at its one position). sema's own literal-defaulting (resolveIntLit,
// reached via checkExpr's "Any" bypass with no surrounding expected type)
// always resolves such a literal to Int64 — but the *value token* codegen
// actually emits for it (genValue's IntLit case) is just Go's own
// untyped integer constant syntax ("5"), and Go's rule for boxing an
// untyped constant into `any` uses its own *default* type, "int", not
// whatever concrete type the surrounding AmiFL expression settled on. So
// `fmt.Sprintf("%T", 5)` reports "int", not the "int64" sema actually
// checked the call against — a real, self-caught step-13 bug (found by
// actually running examples/extern.aml's typeName(5) through the full
// amivm -> go build -> execute pipeline, not by inspecting the IR).
//
// A float-literal token needs no equivalent guard: Go's own untyped-
// float-constant default is already float64, which is exactly
// resolveFloatLit's own no-context default — the two happen to already
// agree. Nor does a variable reference (a "%"/"$"/"&"/"@"-prefixed
// token): its Go type was already fixed by its own VAR/param declaration,
// so boxing it into `any` uses that declared type, not any default at
// all. The one-character prefix check below is what tells a bare literal
// token apart from either of those — every other value shape emits with
// a recognizable non-digit, non-"-" leading character (a name-prefix
// sigil, a quote, or "true"/"false"/"nil"'s own leading letter).
func (g *gen) boxIntLiteralForAny(val string) string {
	if val == "" {
		return val
	}
	c := val[0]
	if c != '-' && (c < '0' || c > '9') {
		return val
	}
	for _, r := range val {
		if r == '.' || r == 'e' || r == 'E' {
			return val // a float literal — no guard needed, see doc comment
		}
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", tmp)
	g.writeCall("%"+tmp, "?int64", []string{val})
	return "%" + tmp
}

// genExternCallStmt is genCallStmt's counterpart for an extern bind call
// used purely for effect (amifl-spec.md section 15's `-> Unit` binds, or a
// non-Unit bind's result explicitly discarded via DiscardExpr). A
// Unit-returning Go function always has zero native return values, so —
// unlike genExternCallValue — there's no Tuple2 multi-return case to
// consider here at all.
func (g *gen) genExternCallStmt(c *ast.CallExpr) error {
	callee, argExprs, err := g.externCallee(c)
	if err != nil {
		return err
	}
	argVals, err := g.genExternArgValues(argExprs, externParamTypesFor(c))
	if err != nil {
		return err
	}
	g.writeCall("", callee, argVals)
	return nil
}

// externParamTypesFor lines c.ExternParamTypes up with externCallee's own
// argExprs — shrunk by dropping index 0 (the receiver) for a method-style
// bind, exactly like externCallee shrinks c.Args itself, so
// genExternArgValues's positional paramTypes[i] check lines up correctly
// either way.
func externParamTypesFor(c *ast.CallExpr) []string {
	if c.ExternMethod == "" {
		return c.ExternParamTypes
	}
	if len(c.ExternParamTypes) == 0 {
		return nil
	}
	return c.ExternParamTypes[1:]
}

// genExternCallValue is genCallValue's counterpart for an extern bind call
// used as a value. Most binds return a single Go value and compile to a
// plain single-result CALL exactly like a top-level `fn` call — the one
// difference is a bind whose declared return type is a 2-element tuple
// (amifl-spec.md sections 13.3/13.9's own `Tuple2[T,Error]`/`Tuple2[T,Bool]`
// convention, e.g. 15.1's `bind Marshal(v: Any) -> Tuple2[Bytes, Error]`):
// that maps to a Go function/method genuinely returning two native values,
// exactly like `parse[T]`/`recv`/`open` already do (builtins.go's
// genParseValue, chan.go's genRecvValue, files.go's genOpenValue) — this
// reuses files.go's assembleTuple2 the same way those do, rather than
// duplicating the two-VAR-plus-two-FSET pattern a fourth time.
func (g *gen) genExternCallValue(c *ast.CallExpr) (string, error) {
	callee, argExprs, err := g.externCallee(c)
	if err != nil {
		return "", err
	}
	argVals, err := g.genExternArgValues(argExprs, externParamTypesFor(c))
	if err != nil {
		return "", err
	}

	if isTupleType(c.ResolvedType) {
		parts := tupleTypeParts(c.ResolvedType)
		if len(parts) == 2 {
			payloadGoType := g.prog.resolveGoType(parts[0])
			payloadTmp := g.newTemp()
			fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", payloadTmp, payloadGoType)
			secondGoType := g.prog.resolveGoType(parts[1])
			secondTmp := g.newTemp()
			fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", secondTmp, secondGoType)
			g.writeCallMulti([]string{"%" + payloadTmp, "%" + secondTmp}, callee, argVals)
			return g.assembleTuple2(c.ResolvedType, "%"+payloadTmp, "%"+secondTmp)
		}
	}

	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeCall("%"+tmp, callee, argVals)
	return "%" + tmp, nil
}
