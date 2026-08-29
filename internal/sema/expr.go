package sema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// checkExpr type-checks e, returning its canonical type. expected, if
// non-empty, is the type context e is used in (e.g. a `let`'s type
// annotation) — literals adapt to it (amifl-spec.md's untyped-literal
// convenience, resolved by resolveType), while every other expression
// kind reports its own fixed/inferred type regardless of expected, and
// gets checked against it here, in one place, uniformly (principle 2: no
// implicit conversion between differently-typed values).
func (fc *funcChecker) checkExpr(e ast.Expr, expected string) (string, error) {
	typ, err := fc.resolveType(e, expected)
	if err != nil {
		return "", err
	}
	if expected != "" && typ != expected {
		return "", fmt.Errorf("line %d: expected %s, got %s", e.Pos(), expected, typ)
	}
	return typ, nil
}

func (fc *funcChecker) resolveType(e ast.Expr, expected string) (string, error) {
	switch v := e.(type) {
	case *ast.IntLit:
		return fc.resolveIntLit(v, expected)
	case *ast.FloatLit:
		return fc.resolveFloatLit(v, expected)
	case *ast.BoolLit:
		return "Bool", nil
	case *ast.StringLit:
		return "String", nil
	case *ast.IdentExpr:
		return fc.resolveIdentExpr(v)
	case *ast.CallExpr:
		return fc.resolveCallExpr(v)
	case *ast.LetExpr:
		return fc.resolveLetExpr(v)
	case *ast.ConstDecl:
		return fc.resolveLocalConstDecl(v)
	case *ast.AssignExpr:
		return fc.resolveAssignExpr(v)
	case *ast.DiscardExpr:
		return fc.resolveDiscardExpr(v)
	case *ast.BinaryExpr:
		return fc.resolveBinaryExpr(v, expected)
	case *ast.UnaryExpr:
		return fc.resolveUnaryExpr(v, expected)
	case *ast.IfExpr:
		return fc.resolveIfExpr(v, expected)
	case *ast.WhileExpr:
		return fc.resolveWhileExpr(v)
	case *ast.BreakExpr:
		return fc.resolveBreakExpr(v)
	case *ast.ContinueExpr:
		return fc.resolveContinueExpr(v)
	case *ast.TupleLit:
		return fc.resolveTupleLit(v)
	case *ast.StructLit:
		return fc.resolveStructLit(v)
	case *ast.FieldExpr:
		return fc.resolveFieldExpr(v)
	case *ast.ListLit:
		return fc.resolveListLit(v, expected)
	case *ast.SetOrMapLit:
		return fc.resolveSetOrMapLit(v, expected)
	case *ast.IndexExpr:
		return fc.resolveIndexExpr(v)
	case *ast.IndexAssignExpr:
		return fc.resolveIndexAssignExpr(v)
	case *ast.SliceExpr:
		return fc.resolveSliceExpr(v)
	case *ast.ForExpr:
		return fc.resolveForExpr(v, expected)
	case *ast.SwitchExpr:
		return fc.resolveSwitchExpr(v, expected)
	case *ast.TryExpr:
		return fc.resolveTryExpr(v)
	case *ast.ClosureLit:
		// A ClosureLit only ever reaches resolveType from somewhere other
		// than resolveLetExpr's own dedicated check (which intercepts it
		// before checkExpr is ever called) — a call argument, an if/while
		// condition, a binary operand, a discard target, and so on. Step
		// 5 deliberately doesn't support any of those (see the type's doc
		// comment), so every other path lands here with a clear, specific
		// rejection instead of the generic "unsupported expression" below.
		return "", fmt.Errorf("line %d: a closure literal is only allowed as a `let`'s value in step 5 (bind it first: `let f = fn(...) -> R { ... }`, then use f)", v.Line)
	default:
		return "", fmt.Errorf("sema: unsupported expression %T", e)
	}
}

func (fc *funcChecker) resolveIntLit(v *ast.IntLit, expected string) (string, error) {
	target := expected
	if target == "" {
		target = "Int64"
	}
	switch {
	case isIntType(target):
		if v.Value > intLitMax[target] {
			return "", fmt.Errorf("line %d: %d overflows %s (max %d)", v.Line, v.Value, target, intLitMax[target])
		}
		return target, nil
	case isFloatType(target):
		// Any integer literal fits a float type (step 2 does no
		// precision-loss check for very large literals against
		// Float32's ~24-bit mantissa — a known, minor limitation).
		return target, nil
	default:
		return "", fmt.Errorf("line %d: %s is not a numeric type; cannot use an integer literal here", v.Line, target)
	}
}

func (fc *funcChecker) resolveFloatLit(v *ast.FloatLit, expected string) (string, error) {
	target := expected
	if target == "" {
		target = "Float64"
	}
	if !isFloatType(target) {
		return "", fmt.Errorf("line %d: %s is not a floating-point type; a float literal cannot implicitly narrow to it", v.Line, target)
	}
	return target, nil
}

func (fc *funcChecker) resolveIdentExpr(v *ast.IdentExpr) (string, error) {
	b, ok := fc.lookup(v.Name)
	if !ok {
		return "", fmt.Errorf("line %d: undefined name %q", v.Line, v.Name)
	}
	v.ResolvedType = b.typ
	if b.isConst {
		v.ConstValue = b.value
	} else {
		v.Token = b.token
	}
	return b.typ, nil
}

// resolveCallExpr type-checks `callee(args...)` (amifl-spec.md section 8).
// `print` is still its own hardcoded special case, unchanged since step 1
// (the general built-in function library arrives in step 11). Otherwise
// Callee is resolved in the same shadowing order as any other name: the
// current scope chain first (a local closure-valued variable — step 5's
// "ローカルクロージャー"), then top-level `fn`s (step 5's "トップレベル
// fn") — mirroring how a local `let`/`const` already shadows a top-level
// `const` of the same name (funcChecker.lookup).
func (fc *funcChecker) resolveCallExpr(v *ast.CallExpr) (string, error) {
	if v.Callee == "print" {
		if len(v.Args) != 1 {
			return "", fmt.Errorf("line %d: print expects exactly 1 argument, got %d", v.Line, len(v.Args))
		}
		if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
			return "", err
		}
		v.ResolvedType = unitType
		return unitType, nil
	}

	if typ, ok, err := fc.resolveBuiltinCall(v); ok {
		return typ, err
	}

	if b, ok := fc.lookup(v.Callee); ok {
		if !isFuncType(b.typ) {
			return "", fmt.Errorf("line %d: %q is not callable (it has type %s)", v.Line, v.Callee, b.typ)
		}
		params, ret, _ := funcTypeParts(b.typ)
		if err := fc.checkCallArgs(v, params); err != nil {
			return "", err
		}
		v.CalleeToken = b.token
		v.ResolvedType = ret
		return ret, nil
	}

	if sig, ok := fc.funcs[v.Callee]; ok {
		if err := fc.checkCallArgs(v, sig.params); err != nil {
			return "", err
		}
		v.ResolvedType = sig.ret
		return sig.ret, nil
	}

	return "", fmt.Errorf("line %d: undefined function %q", v.Line, v.Callee)
}

// resolveTryExpr type-checks the postfix `?` operator (amifl-spec.md
// section 3.3). v.Value must resolve to Tuple2[U,Error] (the common case —
// TryExpr's own type is then U, the unwrapped payload) or to bare Error
// (v.IsBareError — TryExpr's own type is then Unit, since there is no
// payload to unwrap; nothing in step 11 actually produces a bare
// Error-returning call yet, but the check is written to accept one
// uniformly rather than special-casing "unreachable for now" away — step
// 12's `close`/13.10's file I/O will start reaching it). Either way, the
// *enclosing* function/closure's own declared return type (fc.retType) —
// independent of what v.Value's type happens to be — must itself be
// Tuple2[_,Error] or bare Error, since that's the shape genTryValue's
// early-return path constructs when propagating (17.2節#1 explicitly rules
// out generalizing `?` beyond this 2-type convention).
func (fc *funcChecker) resolveTryExpr(v *ast.TryExpr) (string, error) {
	valTyp, err := fc.checkExpr(v.Value, "")
	if err != nil {
		return "", err
	}

	var resultTyp string
	if isErrorType(valTyp) {
		v.IsBareError = true
		resultTyp = unitType
	} else if payload, ok := tuple2ErrorPayload(valTyp); ok {
		v.ElemType = payload
		resultTyp = payload
	} else {
		return "", fmt.Errorf("line %d: `?` requires a Tuple2[T,Error] or Error-typed expression, got %s", v.Line, valTyp)
	}

	if !isErrorType(fc.retType) {
		if _, ok := tuple2ErrorPayload(fc.retType); !ok {
			return "", fmt.Errorf("line %d: `?` can only be used inside a function/closure returning Tuple2[U,Error] or Error, got %s", v.Line, fc.retType)
		}
	}

	return resultTyp, nil
}

func (fc *funcChecker) checkCallArgs(v *ast.CallExpr, params []string) error {
	if len(v.Args) != len(params) {
		return fmt.Errorf("line %d: %q expects %d argument(s), got %d", v.Line, v.Callee, len(params), len(v.Args))
	}
	for i, arg := range v.Args {
		if _, err := fc.checkExpr(arg, params[i]); err != nil {
			return err
		}
	}
	return nil
}

// resolveLetExpr type-checks `let name[: Type] = value` (amifl-spec.md
// section 4). A ClosureLit value is handled entirely separately, before
// any of the usual literal-adaptation/type-annotation machinery: its type
// is always fully self-determined (every parameter and the return type
// are already explicit in the closure literal itself), so an *additional*
// type annotation on the `let` would be redundant at best — rejected here
// with a clear message rather than silently accepted or run through
// checkExpr's generic (and, for a closure, not even reachable — see
// resolveType's *ast.ClosureLit case) expected-type machinery.
func (fc *funcChecker) resolveLetExpr(v *ast.LetExpr) (string, error) {
	if clos, ok := v.Value.(*ast.ClosureLit); ok {
		if v.Type != nil {
			return "", fmt.Errorf("line %d: a closure literal's type is always inferred from its own signature; remove the type annotation on %q", v.Line, v.Name)
		}
		typ, err := fc.resolveClosureLit(clos)
		if err != nil {
			return "", err
		}
		return fc.declareLet(v, typ)
	}

	var expected string
	if v.Type != nil {
		t, err := fc.resolveTypeExpr(v.Type)
		if err != nil {
			return "", err
		}
		expected = t
	}
	typ, err := fc.checkExpr(v.Value, expected)
	if err != nil {
		return "", err
	}
	if typ == unitType {
		return "", fmt.Errorf("line %d: cannot bind %q to a Unit-typed value", v.Line, v.Name)
	}
	return fc.declareLet(v, typ)
}

// declareLet mints v's Token, declares it (reassignable, unlike a
// parameter — amifl-spec.md section 4, "再代入可"), and annotates v,
// shared by resolveLetExpr's closure and non-closure paths.
func (fc *funcChecker) declareLet(v *ast.LetExpr, typ string) (string, error) {
	token := "%" + fc.freshInternalName(v.Name)
	if err := fc.declare(v.Name, &binding{typ: typ, token: token, reassignable: true}); err != nil {
		return "", fmt.Errorf("line %d: %s", v.Line, err)
	}
	v.ResolvedType = typ
	v.Token = token
	return unitType, nil
}

func (fc *funcChecker) resolveLocalConstDecl(v *ast.ConstDecl) (string, error) {
	typ, lit, err := resolveConstDecl(fc, v)
	if err != nil {
		return "", err
	}
	if err := fc.declare(v.Name, &binding{isConst: true, typ: typ, value: lit}); err != nil {
		return "", fmt.Errorf("line %d: %s", v.Line, err)
	}
	v.ResolvedType = typ
	return unitType, nil
}

func (fc *funcChecker) resolveAssignExpr(v *ast.AssignExpr) (string, error) {
	b, ok := fc.lookup(v.Name)
	if !ok {
		return "", fmt.Errorf("line %d: undefined name %q", v.Line, v.Name)
	}
	if !b.reassignable {
		kind := "a function parameter"
		if b.isConst {
			kind = "a const"
		}
		return "", fmt.Errorf("line %d: cannot assign to %q: it is %s", v.Line, v.Name, kind)
	}
	if _, err := fc.checkExpr(v.Value, b.typ); err != nil {
		return "", err
	}
	v.Token = b.token
	return unitType, nil
}

func (fc *funcChecker) resolveDiscardExpr(v *ast.DiscardExpr) (string, error) {
	if _, err := fc.checkExpr(v.Value, ""); err != nil {
		return "", err
	}
	return unitType, nil
}

var (
	arithmeticOps = map[string]bool{"+": true, "-": true, "*": true, "/": true, "%": true}
	bitwiseOps    = map[string]bool{"&": true, "|": true, "^": true, "&^": true}
	shiftOps      = map[string]bool{"<<": true, ">>": true}
	equalityOps   = map[string]bool{"==": true, "!=": true}
	orderedOps    = map[string]bool{"<": true, "<=": true, ">": true, ">=": true}
	logicalOps    = map[string]bool{"&&": true, "||": true}
)

// resolveBinaryExpr type-checks a binary operator expression
// (amifl-spec.md section 6) against its operator's capability requirement
// (2.3節), storing the operand type on the node (BinaryExpr.ResolvedType)
// for codegen. expected propagates to the operand(s) an untyped literal
// among them can adapt to; it never determines the *result* type of a
// comparison/logical expression, which is always Bool regardless of
// context.
func (fc *funcChecker) resolveBinaryExpr(v *ast.BinaryExpr, expected string) (string, error) {
	switch {
	case logicalOps[v.Op]:
		if _, err := fc.checkExpr(v.Left, "Bool"); err != nil {
			return "", err
		}
		if _, err := fc.checkExpr(v.Right, "Bool"); err != nil {
			return "", err
		}
		v.ResolvedType = "Bool"
		return "Bool", nil

	case shiftOps[v.Op]:
		leftTyp, err := fc.checkExpr(v.Left, expected)
		if err != nil {
			return "", err
		}
		if !isIntType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s requires an integer type, got %s", v.Line, v.Op, leftTyp)
		}
		// The shift count is always UInt regardless of the left operand's
		// signedness (amifl-spec.md section 6, "右辺は`UInt`") — an untyped
		// literal there defaults to UInt64 rather than the usual Int64.
		rightTyp, err := fc.resolveType(v.Right, "UInt64")
		if err != nil {
			return "", err
		}
		if !isUIntType(rightTyp) {
			return "", fmt.Errorf("line %d: shift count must be a UInt type, got %s", v.Line, rightTyp)
		}
		v.ResolvedType = leftTyp
		return leftTyp, nil

	case arithmeticOps[v.Op]:
		leftTyp, rightTyp, err := fc.resolveOperandTypes(v.Left, v.Right, expected)
		if err != nil {
			return "", err
		}
		if leftTyp != rightTyp {
			return "", fmt.Errorf("line %d: operator %s requires both operands to have the same type, got %s and %s", v.Line, v.Op, leftTyp, rightTyp)
		}
		// `+` is Concatenable on String in addition to being Numeric
		// (amifl-spec.md section 6); Bytes/List/Array join it once those
		// types exist (step 7).
		if !(v.Op == "+" && leftTyp == "String") && !isIntType(leftTyp) && !isFloatType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s is not defined for type %s", v.Line, v.Op, leftTyp)
		}
		v.ResolvedType = leftTyp
		return leftTyp, nil

	case bitwiseOps[v.Op]:
		leftTyp, rightTyp, err := fc.resolveOperandTypes(v.Left, v.Right, expected)
		if err != nil {
			return "", err
		}
		if leftTyp != rightTyp {
			return "", fmt.Errorf("line %d: operator %s requires both operands to have the same type, got %s and %s", v.Line, v.Op, leftTyp, rightTyp)
		}
		if !isIntType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s requires an integer type, got %s", v.Line, v.Op, leftTyp)
		}
		v.ResolvedType = leftTyp
		return leftTyp, nil

	case equalityOps[v.Op]:
		// `==`/`!=` never take a type hint from the surrounding context —
		// they always produce Bool — so operand adaptation only ever runs
		// between the two operands themselves (amifl-spec.md section 6,
		// "同じ型どうしのみ").
		leftTyp, rightTyp, err := fc.resolveOperandTypes(v.Left, v.Right, "")
		if err != nil {
			return "", err
		}
		if leftTyp != rightTyp {
			return "", fmt.Errorf("line %d: operator %s requires both operands to have the same type, got %s and %s", v.Line, v.Op, leftTyp, rightTyp)
		}
		// amifl-spec.md section 8.3: "関数型どうしの==比較はできない" — every
		// other same-typed pair (Bool, String, numeric, ...) is legitimately
		// comparable, so this is the one type family equalityOps must
		// exclude explicitly rather than relying on isIntType/isFloatType/
		// etc. (which arithmetic/bitwise/ordered comparison already lean on
		// to reject Func operands implicitly, having no membership rule to
		// admit them in the first place).
		if isFuncType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s is not defined for function values", v.Line, v.Op)
		}
		// amifl-spec.md section 2.2: "バリアント判定・フィールド取り出しは
		// switchのパターンマッチでのみ行う" — an enum value's only sanctioned
		// interaction is switch, so unlike Tuple/struct (whose == step 6
		// deliberately allows, since Go's native comparison already does the
		// right thing for them) this is excluded explicitly, mirroring the
		// Func exclusion right above.
		if fc.isEnumType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s is not defined for enum values (use switch to inspect one)", v.Line, v.Op)
		}
		v.ResolvedType = leftTyp
		return "Bool", nil

	case orderedOps[v.Op]:
		leftTyp, rightTyp, err := fc.resolveOperandTypes(v.Left, v.Right, "")
		if err != nil {
			return "", err
		}
		if leftTyp != rightTyp {
			return "", fmt.Errorf("line %d: operator %s requires both operands to have the same type, got %s and %s", v.Line, v.Op, leftTyp, rightTyp)
		}
		if !isOrderedType(leftTyp) {
			return "", fmt.Errorf("line %d: operator %s requires an Ordered type (Int/UInt/Float/String), got %s", v.Line, v.Op, leftTyp)
		}
		v.ResolvedType = leftTyp
		return "Bool", nil

	default:
		return "", fmt.Errorf("line %d: unsupported operator %q", v.Line, v.Op)
	}
}

// resolveOperandTypes resolves left and right's types for a binary
// operator whose two operands must end up the same type. Untyped literals
// (isAdaptableLiteral) adapt to whichever side has a fixed, concrete type;
// when both sides are literals, left adapts to expected (or its own
// default) and right then adapts to left. Resolving whichever side isn't
// a literal *first* is what makes `1 + count` behave the same as
// `count + 1` — resolving strictly left-to-right would instead force the
// literal to expected's (or the default's) type before ever consulting
// count's actual type, rejecting perfectly valid code whenever a literal
// happens to come first (a case with no precedent in Seed/Cascade, which
// never had adaptive literals to begin with — CLAUDE.md's design-decision
// log).
func (fc *funcChecker) resolveOperandTypes(left, right ast.Expr, expected string) (string, string, error) {
	leftIsLit := isAdaptableLiteral(left)
	rightIsLit := isAdaptableLiteral(right)

	if !leftIsLit {
		leftTyp, err := fc.checkExpr(left, expected)
		if err != nil {
			return "", "", err
		}
		rightTyp, err := fc.checkExpr(right, leftTyp)
		if err != nil {
			return "", "", err
		}
		return leftTyp, rightTyp, nil
	}
	if !rightIsLit {
		rightTyp, err := fc.checkExpr(right, expected)
		if err != nil {
			return "", "", err
		}
		leftTyp, err := fc.checkExpr(left, rightTyp)
		if err != nil {
			return "", "", err
		}
		return leftTyp, rightTyp, nil
	}
	leftTyp, err := fc.checkExpr(left, expected)
	if err != nil {
		return "", "", err
	}
	rightTyp, err := fc.checkExpr(right, leftTyp)
	if err != nil {
		return "", "", err
	}
	return leftTyp, rightTyp, nil
}

func isAdaptableLiteral(e ast.Expr) bool {
	switch e.(type) {
	case *ast.IntLit, *ast.FloatLit:
		return true
	}
	return false
}

// resolveUnaryExpr type-checks a prefix operator expression
// (amifl-spec.md section 6).
func (fc *funcChecker) resolveUnaryExpr(v *ast.UnaryExpr, expected string) (string, error) {
	switch v.Op {
	case "!":
		if _, err := fc.checkExpr(v.Operand, "Bool"); err != nil {
			return "", err
		}
		v.ResolvedType = "Bool"
		return "Bool", nil

	case "~":
		typ, err := fc.checkExpr(v.Operand, expected)
		if err != nil {
			return "", err
		}
		if !isIntType(typ) {
			return "", fmt.Errorf("line %d: operator ~ requires an integer type, got %s", v.Line, typ)
		}
		v.ResolvedType = typ
		return typ, nil

	case "-":
		// A literal directly under unary minus gets its own bound check
		// (resolveNegatedIntLit) — an Int8's minimum magnitude (128) is one
		// past its maximum (127), so plain resolveIntLit's bound alone
		// would wrongly reject a perfectly valid `-128`.
		if lit, ok := v.Operand.(*ast.IntLit); ok {
			return fc.resolveNegatedIntLit(lit, expected, v)
		}
		typ, err := fc.checkExpr(v.Operand, expected)
		if err != nil {
			return "", err
		}
		if !isIntType(typ) && !isFloatType(typ) {
			return "", fmt.Errorf("line %d: unary - requires a numeric type, got %s", v.Line, typ)
		}
		v.ResolvedType = typ
		return typ, nil

	default:
		return "", fmt.Errorf("line %d: unsupported unary operator %q", v.Line, v.Op)
	}
}

func (fc *funcChecker) resolveNegatedIntLit(lit *ast.IntLit, expected string, v *ast.UnaryExpr) (string, error) {
	target := expected
	if target == "" {
		target = "Int64"
	}
	switch {
	case isIntType(target):
		limit := intLitMax[target]
		if isSignedIntType(target) {
			limit++
		}
		if lit.Value > limit {
			return "", fmt.Errorf("line %d: -%d overflows %s", lit.Line, lit.Value, target)
		}
		v.ResolvedType = target
		return target, nil
	case isFloatType(target):
		v.ResolvedType = target
		return target, nil
	default:
		return "", fmt.Errorf("line %d: %s is not a numeric type; cannot use a negated integer literal here", lit.Line, target)
	}
}

// collectIfBranches walks v's elif chain (desugared at parse time into
// nested IfExpr.Else values — CLAUDE.md's "過去に踏まれた地雷" #2, "ELIFは
// ELSEの中に次のIFをネストする"), type-checking every condition along the
// way in the scope active where the whole if-expression appears (none of
// a branch's own bindings are visible to a sibling condition) and
// collecting each branch's body block. hasElse reports whether the chain
// ends in an explicit `else` (a *ast.Block) rather than running out
// (nil) — see resolveIfExpr for why that distinction controls the whole
// if-expression's type.
func (fc *funcChecker) collectIfBranches(v *ast.IfExpr) ([]*ast.Block, bool, error) {
	var branches []*ast.Block
	cur := v
	for {
		if _, err := fc.checkExpr(cur.Cond, "Bool"); err != nil {
			return nil, false, err
		}
		branches = append(branches, cur.Then)
		switch e := cur.Else.(type) {
		case nil:
			return branches, false, nil
		case *ast.Block:
			branches = append(branches, e)
			return branches, true, nil
		case *ast.IfExpr:
			cur = e
		default:
			return nil, false, fmt.Errorf("sema: unexpected if-else kind %T", e)
		}
	}
}

// resolveBranches type-checks a set of value-producing block branches (an
// if/elif/else chain's bodies) so they all resolve to the same type,
// returning it. A branch whose last expression is an untyped literal
// adapts to whichever sibling branch has a concrete type first — the same
// order-independence fix as resolveOperandTypes (step 3's design
// decision for binary operators), generalized from 2 operands to N
// branches, for exactly the same reason: resolving strictly in written
// order would make `if c { 1 } else { x }` behave differently from
// `if c { x } else { 1 }` even though they should be symmetric.
func (fc *funcChecker) resolveBranches(branches []*ast.Block, expected string) (string, error) {
	checkBranch := func(b *ast.Block, want string) (string, error) {
		fc.pushScope()
		defer fc.popScope()
		return fc.checkBlock(b, want)
	}

	anchor := 0
	for i, b := range branches {
		if !blockEndsInAdaptableLiteral(b) {
			anchor = i
			break
		}
	}

	target, err := checkBranch(branches[anchor], expected)
	if err != nil {
		return "", err
	}
	for i, b := range branches {
		if i == anchor {
			continue
		}
		if _, err := checkBranch(b, target); err != nil {
			return "", err
		}
	}
	return target, nil
}

func blockEndsInAdaptableLiteral(b *ast.Block) bool {
	if len(b.Exprs) == 0 {
		return false
	}
	return isAdaptableLiteral(b.Exprs[len(b.Exprs)-1])
}

// resolveIfExpr type-checks `if`/`elif`/`else` (amifl-spec.md section 7).
// Without an else, the whole expression is Unit-typed and every branch
// must be too (a value with nowhere to flow when the condition is
// false); with one, every branch (then, each elif, and else) must agree
// on a single type, which becomes the if-expression's own type. A
// `switch`'s Bool-only case list (step 4's scope) desugars into an IfExpr
// at parse time, so this same function — and this same "default required
// to produce a value" rule, since a missing default is exactly a missing
// else — handles it too without switch needing any sema code of its own.
func (fc *funcChecker) resolveIfExpr(v *ast.IfExpr, expected string) (string, error) {
	branches, hasElse, err := fc.collectIfBranches(v)
	if err != nil {
		return "", err
	}
	branchExpected := expected
	if !hasElse {
		branchExpected = unitType
	}
	typ, err := fc.resolveBranches(branches, branchExpected)
	if err != nil {
		return "", err
	}
	v.ResolvedType = typ
	return typ, nil
}

// resolveWhileExpr type-checks `while cond { ... }` (amifl-spec.md section
// 7): always Unit-typed, so its body is checked the same way a function
// body's non-final statements are — every expression in it must itself be
// Unit-typed.
func (fc *funcChecker) resolveWhileExpr(v *ast.WhileExpr) (string, error) {
	if _, err := fc.checkExpr(v.Cond, "Bool"); err != nil {
		return "", err
	}
	fc.loopDepth++
	fc.pushScope()
	_, err := fc.checkBlock(v.Body, unitType)
	fc.popScope()
	fc.loopDepth--
	if err != nil {
		return "", err
	}
	return unitType, nil
}

// resolveBreakExpr and resolveContinueExpr just enforce amifl-spec.md
// section 7's "break/continueは最も内側のループのみに作用" by rejecting
// one found with no enclosing `while` at all — loopDepth is a simple
// counter rather than a stack because AmiFL has no labeled break/continue
// (ignored/amivm/amivm_spec.md section 4.11 confirms AMIVM's BREAK/
// CONTINUE are unlabeled too), so which specific loop is "innermost"
// never needs to be named, only whether one exists.
//
// Both are Unit-typed rather than the Never type amifl-spec.md gives
// `return` (section 5) — a deliberate, narrower scope for step 4: Never's
// "unifies with any type" behavior isn't implemented anywhere yet (no
// `return` *keyword* exists yet — step 5 adds early function values via
// `fn`/closures but not early-exit `return`; a function/closure body's
// tail expression remains the only way to produce its value), and
// break/continue's only real use case here is as a bare Unit-typed
// statement inside a loop body, which this already covers. Using break/
// continue as a value-producing if/switch branch (`if done { break } else
// { 5 }`) is consequently rejected for now — revisit once `return`/
// Never's design is settled for real.
func (fc *funcChecker) resolveBreakExpr(v *ast.BreakExpr) (string, error) {
	if fc.loopDepth == 0 {
		return "", fmt.Errorf("line %d: break outside of a loop", v.Line)
	}
	return unitType, nil
}

func (fc *funcChecker) resolveContinueExpr(v *ast.ContinueExpr) (string, error) {
	if fc.loopDepth == 0 {
		return "", fmt.Errorf("line %d: continue outside of a loop", v.Line)
	}
	return unitType, nil
}

// resolveClosureLit type-checks `fn(params) -> R { body }` used as a
// value (amifl-spec.md section 8.1), called only from resolveLetExpr (see
// ast.ClosureLit's doc comment for why every other position is rejected).
//
// It reuses fc itself (not a fresh funcChecker) for the body, pushing
// only a child scope — the same "a closure body is just a child scope of
// wherever the literal appears" trick Cascade's implementation notes
// record (github.com/amisonnet8/cascade's closure.go): a captured outer
// binding's Token was already computed once, at its own declaration site,
// and fc.lookup walking the scope chain hands that exact same token back
// unchanged, however many closures deep the reference turns out to sit —
// no copying, no dedicated capture instruction, no per-depth adjustment
// needed. loopDepth is saved and reset to 0 for the duration (break/
// continue never cross a closure boundary — amifl-spec.md section 7,
// "クロージャー境界を越えられない" — matching how a `while` isn't
// "inside" a closure literal nested in its body either way, this simply
// also covers the reverse: a loop outside the closure isn't reachable by
// a break/continue written inside it). closureDepth increments for the
// body so closure parameters get the right "&L-N" token (funcChecker's
// doc comment); CLAUDE.md's 地雷#9 warning against ever storing a bare
// "&N" is why this is computed once, fully qualified, right here at
// declaration time, rather than left for a reference site to work out.
func (fc *funcChecker) resolveClosureLit(v *ast.ClosureLit) (string, error) {
	fc.closureDepth++
	depth := fc.closureDepth
	fc.pushScope()
	savedLoopDepth := fc.loopDepth
	fc.loopDepth = 0
	savedRetType := fc.retType

	typ, err := fc.checkClosureBody(v, depth)

	fc.retType = savedRetType
	fc.loopDepth = savedLoopDepth
	fc.popScope()
	fc.closureDepth--

	if err != nil {
		return "", err
	}
	return typ, nil
}

func (fc *funcChecker) checkClosureBody(v *ast.ClosureLit, depth int) (string, error) {
	seen := map[string]bool{}
	var paramTypes []string
	for i := range v.Params {
		p := &v.Params[i]
		if seen[p.Name] {
			return "", fmt.Errorf("line %d: duplicate parameter %q", p.Line, p.Name)
		}
		seen[p.Name] = true
		pt, err := fc.resolveTypeExpr(p.Type)
		if err != nil {
			return "", err
		}
		p.ResolvedType = pt
		token := fmt.Sprintf("&%d-%d", depth, i+1)
		if err := fc.declare(p.Name, &binding{typ: pt, token: token}); err != nil {
			return "", fmt.Errorf("line %d: %s", p.Line, err)
		}
		paramTypes = append(paramTypes, pt)
	}

	retType, err := fc.resolveReturnTypeExpr(v.ReturnType)
	if err != nil {
		return "", err
	}
	v.ResolvedReturnType = retType
	fc.retType = retType
	if _, err := fc.checkBlock(v.Body, retType); err != nil {
		return "", err
	}

	typ := makeFuncType(paramTypes, retType)
	v.ResolvedType = typ
	return typ, nil
}

// resolveTupleLit type-checks `(v1, v2, ...)` (amifl-spec.md section 2.2).
// Every element resolves its own type independent of any surrounding
// `expected` context (like a CallExpr or ClosureLit's own type — nothing
// here for an untyped literal element to adapt *to* beyond its own
// default, since a tuple's type is entirely determined by its contents).
// Elements whose own type is itself a Tuple or a Func are rejected — see
// ast.TupleLit's doc comment for why (keeps makeTupleType's encoding a
// flat, unambiguous comma-join).
func (fc *funcChecker) resolveTupleLit(v *ast.TupleLit) (string, error) {
	if len(v.Elems) < 2 || len(v.Elems) > 8 {
		return "", fmt.Errorf("line %d: a tuple must have 2 to 8 elements (amifl-spec.md section 2.2, Tuple2~Tuple8), got %d", v.Line, len(v.Elems))
	}
	elemTypes := make([]string, len(v.Elems))
	for i, e := range v.Elems {
		t, err := fc.checkExpr(e, "")
		if err != nil {
			return "", err
		}
		if t == unitType {
			return "", fmt.Errorf("line %d: a tuple element cannot be Unit-typed", e.Pos())
		}
		if isTupleType(t) || isFuncType(t) {
			return "", fmt.Errorf("line %d: a tuple element cannot itself be a tuple or a function value (nested tuples aren't supported yet)", e.Pos())
		}
		elemTypes[i] = t
	}
	typ := makeTupleType(elemTypes)
	v.ResolvedType = typ
	return typ, nil
}

// resolveStructLit type-checks `TypeName{field1: v1, ...}` (amifl-spec.md
// section 2.2/8.4): TypeName must be a declared struct, every one of its
// fields must be given exactly once (in any order — matched by name, not
// position), and each value is checked against its field's declared type
// (so an untyped literal value adapts to that field's type exactly like a
// `let`'s initializer would).
func (fc *funcChecker) resolveStructLit(v *ast.StructLit) (string, error) {
	info, ok := fc.structs[v.TypeName]
	if !ok {
		return "", fmt.Errorf("line %d: undefined struct type %q", v.Line, v.TypeName)
	}
	seen := map[string]bool{}
	for i := range v.Fields {
		f := &v.Fields[i]
		if seen[f.Name] {
			return "", fmt.Errorf("line %d: duplicate field %q in %s literal", f.Line, f.Name, v.TypeName)
		}
		seen[f.Name] = true
		fieldTyp, ok := info.fieldType(f.Name)
		if !ok {
			return "", fmt.Errorf("line %d: struct %s has no field %q", f.Line, v.TypeName, f.Name)
		}
		if _, err := fc.checkExpr(f.Value, fieldTyp); err != nil {
			return "", err
		}
	}
	if len(seen) != len(info.Fields) {
		var missing []string
		for _, fld := range info.Fields {
			if !seen[fld.Name] {
				missing = append(missing, fld.Name)
			}
		}
		return "", fmt.Errorf("line %d: %s literal is missing field(s): %s", v.Line, v.TypeName, strings.Join(missing, ", "))
	}
	v.ResolvedType = v.TypeName
	return v.TypeName, nil
}

// resolveFieldExpr type-checks postfix `target.field` (amifl-spec.md
// section 3.2/2.2) — tuple index sugar when Target's type is a Tuple,
// ordinary struct field access when it's a struct, or (step 8) enum
// variant construction when Target is a bare identifier naming a declared
// enum type (checked *first*, before Target is ever resolved as a value —
// an enum type name was never a valid variable reference to begin with, so
// there's nothing lost by not trying checkExpr(Target) in that case, and
// trying it first would just fail with a confusing "undefined name"
// instead of resolving correctly). Every other case computes and stores
// AmivmField, the exact string codegen writes after FGET's `>` prefix
// (ast.FieldExpr's doc comment) — a synthesized "F0"/"F1"/... for a tuple
// index (Go struct fields can't be named with a bare digit) or Field
// verbatim for a struct, since codegen has no vocabulary of its own to
// tell a Tuple's encoded ResolvedType apart from a struct's (see
// makeTupleType's doc comment on why that stays sema-internal).
func (fc *funcChecker) resolveFieldExpr(v *ast.FieldExpr) (string, error) {
	if ident, ok := v.Target.(*ast.IdentExpr); ok {
		if info, isEnum := fc.enums[ident.Name]; isEnum {
			return fc.resolveEnumVariantConstruction(v, ident.Name, info)
		}
	}
	if v.Args != nil {
		return "", fmt.Errorf("line %d: '.'-call syntax (`X.Y(...)`) is only valid for enum variant construction (`EnumType.Variant(...)`)", v.Line)
	}

	targetTyp, err := fc.checkExpr(v.Target, "")
	if err != nil {
		return "", err
	}
	if isTupleType(targetTyp) {
		elems, _ := tupleTypeParts(targetTyp)
		idx, convErr := strconv.Atoi(v.Field)
		if convErr != nil || idx < 0 || idx >= len(elems) {
			return "", fmt.Errorf("line %d: tuple has no field .%s (it has %d element(s), .0 to .%d)", v.Line, v.Field, len(elems), len(elems)-1)
		}
		v.ResolvedType = elems[idx]
		v.AmivmField = fmt.Sprintf("F%d", idx)
		return v.ResolvedType, nil
	}
	if info, ok := fc.structs[targetTyp]; ok {
		fieldTyp, ok := info.fieldType(v.Field)
		if !ok {
			return "", fmt.Errorf("line %d: struct %s has no field %q", v.Line, targetTyp, v.Field)
		}
		v.ResolvedType = fieldTyp
		v.AmivmField = v.Field
		return fieldTyp, nil
	}
	return "", fmt.Errorf("line %d: type %s has no fields to access with '.'", v.Line, targetTyp)
}

// resolveEnumVariantConstruction type-checks `EnumType.Variant` (v.Args ==
// nil) or `EnumType.Variant(field: v, ...)` (v.Args != nil) — amifl-spec.md
// section 2.2's "値生成は型名.バリアント名(...)というリテラルではない通常の
// 式". Every one of the variant's declared fields must be given exactly
// once, by name (v.Args uses the identical named-field convention
// resolveStructLit already enforces for struct literals) — a zero-field
// variant naturally satisfies this with v.Args empty (nil or a
// zero-length slice both compare equal-length to a zero-field variant's
// own Fields, so no separate "bare variant" code path is needed here).
func (fc *funcChecker) resolveEnumVariantConstruction(v *ast.FieldExpr, enumName string, info *enumInfo) (string, error) {
	vi, ok := info.variantIndex(v.Field)
	if !ok {
		return "", fmt.Errorf("line %d: enum %s has no variant %q", v.Line, enumName, v.Field)
	}
	variant := info.Variants[vi]
	if len(v.Args) != len(variant.Fields) {
		return "", fmt.Errorf("line %d: %s.%s expects %d field value(s), got %d", v.Line, enumName, v.Field, len(variant.Fields), len(v.Args))
	}
	seen := map[string]bool{}
	for i := range v.Args {
		a := &v.Args[i]
		if seen[a.Name] {
			return "", fmt.Errorf("line %d: duplicate field %q in %s.%s construction", a.Line, a.Name, enumName, v.Field)
		}
		seen[a.Name] = true
		fieldTyp, ok := variant.fieldType(a.Name)
		if !ok {
			return "", fmt.Errorf("line %d: variant %s.%s has no field %q", a.Line, enumName, v.Field, a.Name)
		}
		if _, err := fc.checkExpr(a.Value, fieldTyp); err != nil {
			return "", err
		}
	}
	if len(seen) != len(variant.Fields) {
		var missing []string
		for _, fld := range variant.Fields {
			if !seen[fld.Name] {
				missing = append(missing, fld.Name)
			}
		}
		return "", fmt.Errorf("line %d: %s.%s construction is missing field(s): %s", v.Line, enumName, v.Field, strings.Join(missing, ", "))
	}
	v.IsEnumVariant = true
	v.VariantIndex = vi
	v.ResolvedType = enumName
	return enumName, nil
}

// resolveSwitchExpr type-checks step 8's subject-carrying `switch`
// (amifl-spec.md section 10) — see ast.SwitchExpr's doc comment for the
// step-8 scope cut (Subject must be a static enum type; `is`/`in` aren't
// supported here). Each case's variant/binding names are validated and
// its field bindings declared in the case's own child scope before its
// body is checked; every case body (plus Default, if present) is then
// resolved to one common type via the same order-independent
// "anchor on the first non-literal branch" trick resolveBranches uses for
// if/elif/else (step 4) and resolveListElemTypes uses for list elements
// (step 7) — a case whose body is a bare literal must still adapt to
// whatever concrete type a sibling case settles on, regardless of case
// order.
func (fc *funcChecker) resolveSwitchExpr(v *ast.SwitchExpr, expected string) (string, error) {
	subjectTyp, err := fc.checkExpr(v.Subject, "")
	if err != nil {
		return "", err
	}
	info, ok := fc.enums[subjectTyp]
	if !ok {
		return "", fmt.Errorf("line %d: switch with a subject requires an enum type, got %s (amifl-spec.md section 10's `is Type`/`in [...]` case forms aren't supported yet)", v.Subject.Pos(), subjectTyp)
	}
	v.EnumName = subjectTyp

	seen := map[string]int{} // variant name -> index into v.Cases
	for ci := range v.Cases {
		c := &v.Cases[ci]
		if c.EnumName != subjectTyp {
			return "", fmt.Errorf("line %d: case names enum %s, but this switch's subject is %s", c.Line, c.EnumName, subjectTyp)
		}
		vi, ok := info.variantIndex(c.Variant)
		if !ok {
			return "", fmt.Errorf("line %d: enum %s has no variant %q", c.Line, subjectTyp, c.Variant)
		}
		if prevCi, dup := seen[c.Variant]; dup {
			return "", fmt.Errorf("line %d: duplicate case for %s.%s (already handled at line %d)", c.Line, subjectTyp, c.Variant, v.Cases[prevCi].Line)
		}
		seen[c.Variant] = ci
		c.VariantIndex = vi

		variant := info.Variants[vi]
		if len(c.Bindings) != len(variant.Fields) {
			return "", fmt.Errorf("line %d: case %s.%s expects %d binding(s), got %d", c.Line, subjectTyp, c.Variant, len(variant.Fields), len(c.Bindings))
		}
		c.BindingTypes = make([]string, len(c.Bindings))
		for bi, bname := range c.Bindings {
			fld := variant.Fields[bi]
			if bname != fld.Name {
				return "", fmt.Errorf("line %d: binding %q in case %s.%s must be named %q (its declared field name, in order; bind a different local name inside the case body with `let` if you want one)", c.Line, bname, subjectTyp, c.Variant, fld.Name)
			}
			c.BindingTypes[bi] = fld.Typ
		}
	}

	exhaustive := len(seen) == len(info.Variants)
	if v.Default == nil && !exhaustive {
		var missing []string
		for _, variant := range info.Variants {
			if _, ok := seen[variant.Name]; !ok {
				missing = append(missing, variant.Name)
			}
		}
		return "", fmt.Errorf("line %d: switch over %s is not exhaustive and has no default (missing: %s)", v.Line, subjectTyp, strings.Join(missing, ", "))
	}

	// Resolve every case body (plus Default, if present) to one common
	// type, treating Default as one more branch appended after every case
	// for the purposes of anchor selection and the final check loop —
	// exactly resolveBranches' "anchor on the first non-literal branch"
	// trick (step 4), generalized here to N cases plus an optional
	// default, so a switch's overall type doesn't depend on case order or
	// on whether the concrete type happens to come from a case or from
	// default.
	checkCase := func(c *ast.SwitchCase, want string) (string, error) {
		fc.pushScope()
		defer fc.popScope()
		c.BindingTokens = make([]string, len(c.Bindings))
		for bi, bname := range c.Bindings {
			token := "%" + fc.freshInternalName(bname)
			if err := fc.declare(bname, &binding{typ: c.BindingTypes[bi], token: token}); err != nil {
				return "", fmt.Errorf("line %d: %s", c.Line, err)
			}
			c.BindingTokens[bi] = token
		}
		return fc.checkBlock(c.Body, want)
	}
	checkDefault := func(want string) (string, error) {
		fc.pushScope()
		defer fc.popScope()
		return fc.checkBlock(v.Default, want)
	}
	branchCount := len(v.Cases)
	if v.Default != nil {
		branchCount++
	}
	blockAt := func(i int) *ast.Block {
		if i < len(v.Cases) {
			return v.Cases[i].Body
		}
		return v.Default
	}
	checkAt := func(i int, want string) (string, error) {
		if i < len(v.Cases) {
			return checkCase(&v.Cases[i], want)
		}
		return checkDefault(want)
	}

	anchor := 0
	for i := 0; i < branchCount; i++ {
		if !blockEndsInAdaptableLiteral(blockAt(i)) {
			anchor = i
			break
		}
	}
	target, err := checkAt(anchor, expected)
	if err != nil {
		return "", err
	}
	for i := 0; i < branchCount; i++ {
		if i == anchor {
			continue
		}
		if _, err := checkAt(i, target); err != nil {
			return "", err
		}
	}

	v.ResolvedType = target
	return target, nil
}

// resolveListLit type-checks `[v1, v2, ...]` (amifl-spec.md sections
// 2.2/3.1): a List[T] by default, or an Array[T;N] when expected says so
// — the same untyped-literal-adapts-to-context pattern step 2 established
// for IntLit/FloatLit, generalized to a collection literal's element type
// (and, for Array, its declared size). expected values other than a List/
// Array type (including "") fall through to the List-and-infer path,
// exactly like every other resolveXxx here — checkExpr's own generic
// post-check is what reports a genuine mismatch (e.g. `let x: Int =
// [1,2]`), not a special case in here. Nested list literals (multi-
// dimensional data) get the outer collection's own element type as their
// own expected, recursively, purely as a byproduct of checkExpr already
// threading expected down to each element — amifl-spec.md section 2.2's
// "コレクションリテラルの型注釈は...ネストの各階層へ再帰的に伝播する"; with
// no expected at all, every level independently defaults to List ("無注釈
// 時は各階層独立に既定（List）が適用される").
func (fc *funcChecker) resolveListLit(v *ast.ListLit, expected string) (string, error) {
	if isArrayType(expected) {
		elemTyp, size, _ := arrayParts(expected)
		if uint64(len(v.Elems)) != size {
			return "", fmt.Errorf("line %d: array literal has %d element(s), expected %d", v.Line, len(v.Elems), size)
		}
		for _, e := range v.Elems {
			if _, err := fc.checkExpr(e, elemTyp); err != nil {
				return "", err
			}
		}
		v.ResolvedType = expected
		return expected, nil
	}
	if isListType(expected) {
		elemTyp, _ := listElemType(expected)
		for _, e := range v.Elems {
			if _, err := fc.checkExpr(e, elemTyp); err != nil {
				return "", err
			}
		}
		v.ResolvedType = expected
		return expected, nil
	}

	if len(v.Elems) == 0 {
		return "", fmt.Errorf("line %d: cannot infer the element type of an empty list literal without a type annotation", v.Line)
	}
	elemTyp, err := fc.resolveListElemTypes(v.Elems)
	if err != nil {
		return "", err
	}
	typ := makeListType(elemTyp)
	v.ResolvedType = typ
	return typ, nil
}

// resolveListElemTypes resolves every element of an un-annotated list
// literal to one shared type, returning it — the same "resolve whichever
// element isn't an untyped literal first" order-independence trick step 3
// introduced for binary operators (resolveOperandTypes) and step 4
// generalized to N if/elif/else branches (resolveBranches), applied here
// to N list elements so `[x, 1]` and `[1, x]` behave symmetrically. Step
// 10 reuses this verbatim for an un-annotated Set literal's elements and a
// Map literal's keys/values independently (resolveSetLit/resolveMapLit) —
// the anchor-selection logic has nothing List-specific about it, it only
// needs "a slice of same-kind expressions that must share one type".
func (fc *funcChecker) resolveListElemTypes(elems []ast.Expr) (string, error) {
	anchor := 0
	for i, e := range elems {
		if !isAdaptableLiteral(e) {
			anchor = i
			break
		}
	}
	target, err := fc.checkExpr(elems[anchor], "")
	if err != nil {
		return "", err
	}
	for i, e := range elems {
		if i == anchor {
			continue
		}
		if _, err := fc.checkExpr(e, target); err != nil {
			return "", err
		}
	}
	return target, nil
}

// resolveSetOrMapLit type-checks `{v1, v2, ...}` (Set[T]) or `{k1: v1, ...}`
// (Map[K,V]) (amifl-spec.md sections 2.2/3.1) — step 10. Which form this
// is was already decided by the parser (ast.SetOrMapLit's doc comment):
// Entries set means Map, Elems set means Set, both nil means a bare `{}`
// whose kind can't be told from syntax alone — only that last case needs
// expected at all, mirroring resolveListLit's identical "an empty [] needs
// a type annotation" fallback.
func (fc *funcChecker) resolveSetOrMapLit(v *ast.SetOrMapLit, expected string) (string, error) {
	switch {
	case v.Entries != nil:
		return fc.resolveMapLit(v, expected)
	case v.Elems != nil:
		return fc.resolveSetLit(v, expected)
	default:
		return fc.resolveEmptySetOrMapLit(v, expected)
	}
}

func (fc *funcChecker) resolveEmptySetOrMapLit(v *ast.SetOrMapLit, expected string) (string, error) {
	if isSetType(expected) || isMapType(expected) {
		v.ResolvedType = expected
		return expected, nil
	}
	return "", fmt.Errorf("line %d: cannot tell whether an empty `{}` is a Set or a Map without a type annotation", v.Line)
}

// resolveSetLit type-checks the Set form of a SetOrMapLit: every element
// resolves to one shared comparable type (isComparableKeyType — Set[T]'s
// own restriction, amifl-spec.md section 2.2), adapting to expected's own
// element type when given (`let s: Set[Int8] = {1, 2}`) or inferred the
// same order-independent way an un-annotated List literal's elements are
// (resolveListElemTypes) otherwise.
func (fc *funcChecker) resolveSetLit(v *ast.SetOrMapLit, expected string) (string, error) {
	var elemTyp string
	if e, ok := setElemType(expected); ok {
		for _, el := range v.Elems {
			if _, err := fc.checkExpr(el, e); err != nil {
				return "", err
			}
		}
		elemTyp = e
	} else {
		t, err := fc.resolveListElemTypes(v.Elems)
		if err != nil {
			return "", err
		}
		elemTyp = t
	}
	if !isComparableKeyType(elemTyp) {
		return "", fmt.Errorf("line %d: Set[T] requires a comparable element type (numeric, String, Bool, or Tuple), got %s", v.Line, elemTyp)
	}
	typ := makeSetType(elemTyp)
	v.ResolvedType = typ
	return typ, nil
}

// resolveMapLit type-checks the Map form of a SetOrMapLit: keys and values
// each independently resolve to one shared type (keys/values may adapt to
// expected's own Map[K,V] when given, or infer the same order-independent
// way resolveListElemTypes already does for a plain list, applied here to
// the keys and the values as two separate same-length slices). The key
// type is further restricted to isComparableKeyType, exactly like Set —
// mapKeyValueTypes/makeMapType, unlike Set's simpler single-element
// encoding, requires depth-aware decoding on the codegen side too (see
// types.go's own doc comment on why), but that's invisible here.
func (fc *funcChecker) resolveMapLit(v *ast.SetOrMapLit, expected string) (string, error) {
	var keyTyp, valTyp string
	if ek, ev, ok := mapKeyValueTypes(expected); ok {
		for i := range v.Entries {
			e := &v.Entries[i]
			if _, err := fc.checkExpr(e.Key, ek); err != nil {
				return "", err
			}
			if _, err := fc.checkExpr(e.Value, ev); err != nil {
				return "", err
			}
		}
		keyTyp, valTyp = ek, ev
	} else {
		keys := make([]ast.Expr, len(v.Entries))
		vals := make([]ast.Expr, len(v.Entries))
		for i, e := range v.Entries {
			keys[i] = e.Key
			vals[i] = e.Value
		}
		kt, err := fc.resolveListElemTypes(keys)
		if err != nil {
			return "", err
		}
		vt, err := fc.resolveListElemTypes(vals)
		if err != nil {
			return "", err
		}
		keyTyp, valTyp = kt, vt
	}
	if !isComparableKeyType(keyTyp) {
		return "", fmt.Errorf("line %d: Map[K,V] requires a comparable key type (numeric, String, Bool, or Tuple), got %s", v.Line, keyTyp)
	}
	if valTyp == unitType {
		return "", fmt.Errorf("line %d: a Map value cannot be Unit-typed", v.Line)
	}
	typ := makeMapType(keyTyp, valTyp)
	v.ResolvedType = typ
	return typ, nil
}

// resolveIndexExpr type-checks `target[index]` (amifl-spec.md section
// 3.2, step 7 — see ast.IndexExpr's doc comment for why this compiles
// directly to AGET rather than through a named `at` function).
func (fc *funcChecker) resolveIndexExpr(v *ast.IndexExpr) (string, error) {
	targetTyp, err := fc.checkExpr(v.Target, "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := elementType(targetTyp)
	if !ok {
		return "", fmt.Errorf("line %d: cannot index into type %s (must be a List or Array)", v.Line, targetTyp)
	}
	if _, err := fc.checkExpr(v.Index, "Int64"); err != nil {
		return "", err
	}
	v.ResolvedType = elemTyp
	return elemTyp, nil
}

// resolveIndexAssignExpr type-checks `target[index] = value` (amifl-
// spec.md section 3.2, "x[i] = v"). Target must be a plain identifier or
// a chain of IndexExprs bottoming out in one — never a struct field or
// other compound expression — a deliberate scope cut mirroring step 6's
// "no field assignment" one, for the identical underlying reason:
// codegen's write-back (collections.go's emitIndexAssign) only has to
// unwind through IndexExpr layers, never worry about whether an
// intermediate FGET-read copy aliases its original storage (a struct
// field holding an Array would silently not, exactly the hazard step 6
// sidestepped altogether by not letting `p.x = v` exist at all). Note
// Target's own binding doesn't need to be *reassignable* (unlike
// AssignExpr) — mutating an element never rebinds the variable itself,
// so even a non-reassignable List/Array-typed parameter can have its
// elements written through this, exactly as Go itself allows.
func (fc *funcChecker) resolveIndexAssignExpr(v *ast.IndexAssignExpr) (string, error) {
	if !isAssignableIndexTarget(v.Target) {
		return "", fmt.Errorf("line %d: assignment target must be a plain variable or a chain of index expressions over one (not a struct field or other compound expression)", v.Line)
	}
	targetTyp, err := fc.checkExpr(v.Target, "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := elementType(targetTyp)
	if !ok {
		return "", fmt.Errorf("line %d: cannot index-assign into type %s (must be a List or Array)", v.Line, targetTyp)
	}
	if _, err := fc.checkExpr(v.Index, "Int64"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Value, elemTyp); err != nil {
		return "", err
	}
	return unitType, nil
}

func isAssignableIndexTarget(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.IdentExpr:
		return true
	case *ast.IndexExpr:
		return isAssignableIndexTarget(v.Target)
	default:
		return false
	}
}

// resolveSliceExpr type-checks `target[from:to]`/`target[from:]`/
// `target[:to]`/`target[:]` (amifl-spec.md section 3.2) — always resolves
// to a List[T] of Target's own element type, regardless of whether Target
// itself was a List or an Array (see ast.SliceExpr's doc comment for why).
func (fc *funcChecker) resolveSliceExpr(v *ast.SliceExpr) (string, error) {
	targetTyp, err := fc.checkExpr(v.Target, "")
	if err != nil {
		return "", err
	}
	elemTyp, ok := elementType(targetTyp)
	if !ok {
		return "", fmt.Errorf("line %d: cannot slice type %s (must be a List or Array)", v.Line, targetTyp)
	}
	if v.From != nil {
		if _, err := fc.checkExpr(v.From, "Int64"); err != nil {
			return "", err
		}
	}
	if v.To != nil {
		if _, err := fc.checkExpr(v.To, "Int64"); err != nil {
			return "", err
		}
	}
	typ := makeListType(elemTyp)
	v.ResolvedType = typ
	return typ, nil
}

// resolveForExpr type-checks `for x in items { ... }` (amifl-spec.md
// section 7, always Unit-typed), since step 9, `for x in items yield
// expr` (ast.ForExpr.Yield set instead of Body — always List(Yield's own
// type)), and, since step 10, the two-variable `for k, v in m { ... }`
// form (ast.ForExpr.Var2 set — Map[K,V] iteration, Body only; see Var2's
// doc comment for why Yield is never combined with it — the parser
// already rejects that combination outright, so it's never seen here).
// Single-variable Items must be a List, Array, (step 10) Set, or (step 12)
// Stream[T] — a Stream is Body-form only, Yield form rejected below since it
// has no statically-known length to preallocate a resulting List with
// (forIterableElemType); two-variable Items must be a Map[K,V]
// (mapKeyValueTypes) — Var binds the key, Var2 the value. Var/Var2 are
// declared as non-reassignable bindings in their own child scope,
// mirroring a function parameter rather than a `let`
// (ast.ForExpr.VarToken's doc comment).
//
// expected propagates into the Yield form only, so `let xs: List[String] =
// for x in nums yield toString(x)` can adapt an untyped-literal-producing
// Yield expression to List[String]'s own element type (List[T]'s already-
// established `expected`-threading precedent, resolveListLit) — the Body
// form ignores it entirely (always Unit, exactly like WhileExpr).
func (fc *funcChecker) resolveForExpr(v *ast.ForExpr, expected string) (string, error) {
	itemsTyp, err := fc.checkExpr(v.Items, "")
	if err != nil {
		return "", err
	}
	v.ItemsType = itemsTyp

	fc.pushScope()

	if v.Var2 != "" {
		keyTyp, valTyp, ok := mapKeyValueTypes(itemsTyp)
		if !ok {
			fc.popScope()
			return "", fmt.Errorf("line %d: `for %s, %s in ...` requires a Map[K,V], got %s", v.Line, v.Var, v.Var2, itemsTyp)
		}
		token := "%" + fc.freshInternalName(v.Var)
		if err := fc.declare(v.Var, &binding{typ: keyTyp, token: token}); err != nil {
			fc.popScope()
			return "", fmt.Errorf("line %d: %s", v.Line, err)
		}
		v.ElemType = keyTyp
		v.VarToken = token
		token2 := "%" + fc.freshInternalName(v.Var2)
		if err := fc.declare(v.Var2, &binding{typ: valTyp, token: token2}); err != nil {
			fc.popScope()
			return "", fmt.Errorf("line %d: %s", v.Line, err)
		}
		v.Var2Type = valTyp
		v.Var2Token = token2
	} else {
		var elemTyp string
		if e, ok := streamElemType(itemsTyp); ok {
			// Stream[T] (step 12) is Body-form only — see the Yield check
			// just below, which rejects it before ever reaching
			// genForYieldValue's List-preallocating codegen (a Stream has no
			// statically-known length to preallocate with, and no `?len`
			// equivalent to even compute one at runtime — CLAUDE.md's
			// step-12 "確定した設計判断").
			elemTyp = e
		} else {
			var ok bool
			elemTyp, ok = forIterableElemType(itemsTyp)
			if !ok {
				fc.popScope()
				return "", fmt.Errorf("line %d: `for` requires a List, Array, Set, or Stream, got %s", v.Line, itemsTyp)
			}
		}
		token := "%" + fc.freshInternalName(v.Var)
		if err := fc.declare(v.Var, &binding{typ: elemTyp, token: token}); err != nil {
			fc.popScope()
			return "", fmt.Errorf("line %d: %s", v.Line, err)
		}
		v.ElemType = elemTyp
		v.VarToken = token
	}

	if v.Yield != nil {
		if isStreamType(itemsTyp) {
			fc.popScope()
			return "", fmt.Errorf("line %d: `for ... yield ...` doesn't support Stream[T] (forward-only, no known length to preallocate) — use the Body form, or collect(...)/take/skip instead", v.Line)
		}
		// break/continue are legal only in the Body form (amifl-spec.md
		// section 7, "break/continueはyield無し形のみで使用可") — suppressing
		// loopDepth here, exactly like a closure body does (resolveClosureLit),
		// makes one found inside Yield hit the ordinary "outside of a loop"
		// rejection, regardless of whether this ForExpr itself happens to be
		// lexically nested inside some other loop.
		savedLoopDepth := fc.loopDepth
		fc.loopDepth = 0
		var yieldExpected string
		if e, ok := listElemType(expected); ok {
			yieldExpected = e
		}
		yieldTyp, yieldErr := fc.checkExpr(v.Yield, yieldExpected)
		fc.loopDepth = savedLoopDepth
		fc.popScope()
		if yieldErr != nil {
			return "", yieldErr
		}
		// Unlike an ordinary List[T] literal element (resolveListLit has no
		// such restriction, since a struct-shaped Unit-adjacent element
		// can't arise there), a Unit-typed Yield has no Go value to collect
		// at all — Unit compiles to no runtime representation whatsoever
		// (amifl-spec.md section 2.2), so genForYieldValue's genValue(Yield)
		// would have nothing to ASET. Caught here, at sema, rather than
		// surfacing as a codegen-internal error or (worse) a `go build`
		// failure over a synthesized `^Unit` Go type that doesn't exist —
		// CLAUDE.md's "意味検査の責任分担" principle.
		if yieldTyp == unitType {
			return "", fmt.Errorf("line %d: a `yield` value cannot be Unit-typed", v.Yield.Pos())
		}
		resultTyp := makeListType(yieldTyp)
		v.ResolvedType = resultTyp
		return resultTyp, nil
	}

	fc.loopDepth++
	_, err = fc.checkBlock(v.Body, unitType)
	fc.loopDepth--
	fc.popScope()
	if err != nil {
		return "", err
	}
	return unitType, nil
}

// resolveTypeExpr turns a parsed type annotation (ast.TypeExpr) into its
// canonical string form. NamedType defers to canonicalType (scalars/
// structs, unchanged since step 2/6); ListType/ArrayType are new in step
// 7, recursively resolving their own element type the same way and, for
// ArrayType, reducing Size to a concrete literal (evalConstArraySize)
// since AMIVM's ARTYPE instruction takes a literal immediate, never an
// identifier or expression. This is a *funcChecker* method (not a
// *checker* one, unlike canonicalType) because ArrayType's Size may
// reference a function-local `const` — every caller either already has a
// real funcChecker on hand (resolveLetExpr, checkClosureBody) or mints a
// throwaway one just for this resolution (registerStructFields/
// registerFuncSig in sema.go), the same pattern checkTopLevelConst already
// uses to resolve a top-level const's own initializer.
func (fc *funcChecker) resolveTypeExpr(te ast.TypeExpr) (string, error) {
	switch t := te.(type) {
	case *ast.NamedType:
		canon, ok := fc.canonicalType(t.Name)
		if !ok {
			return "", fmt.Errorf("line %d: unknown type %q", t.Line, t.Name)
		}
		return canon, nil
	case *ast.ListType:
		elem, err := fc.resolveTypeExpr(t.Elem)
		if err != nil {
			return "", err
		}
		return makeListType(elem), nil
	case *ast.ArrayType:
		elem, err := fc.resolveTypeExpr(t.Elem)
		if err != nil {
			return "", err
		}
		if _, err := fc.checkExpr(t.Size, "Int64"); err != nil {
			return "", err
		}
		n, err := evalConstArraySize(t.Size)
		if err != nil {
			return "", fmt.Errorf("line %d: array size %s", t.Size.Pos(), err)
		}
		return makeArrayType(elem, strconv.FormatUint(n, 10)), nil
	case *ast.SetType:
		elem, err := fc.resolveTypeExpr(t.Elem)
		if err != nil {
			return "", err
		}
		if !isComparableKeyType(elem) {
			return "", fmt.Errorf("line %d: Set[T] requires a comparable element type (numeric, String, Bool, or Tuple), got %s", t.Line, elem)
		}
		return makeSetType(elem), nil
	case *ast.MapType:
		key, err := fc.resolveTypeExpr(t.Key)
		if err != nil {
			return "", err
		}
		if !isComparableKeyType(key) {
			return "", fmt.Errorf("line %d: Map[K,V] requires a comparable key type (numeric, String, Bool, or Tuple), got %s", t.Line, key)
		}
		val, err := fc.resolveTypeExpr(t.Value)
		if err != nil {
			return "", err
		}
		return makeMapType(key, val), nil
	case *ast.TupleType:
		elemTypes := make([]string, len(t.Elems))
		for i, e := range t.Elems {
			et, err := fc.resolveTypeExpr(e)
			if err != nil {
				return "", err
			}
			if isTupleType(et) || isFuncType(et) {
				return "", fmt.Errorf("line %d: a tuple element cannot itself be a tuple or a function value (nested tuples aren't supported yet)", e.Pos())
			}
			elemTypes[i] = et
		}
		return makeTupleType(elemTypes), nil
	case *ast.ChanType:
		elem, err := fc.resolveTypeExpr(t.Elem)
		if err != nil {
			return "", err
		}
		return makeChanType(elem), nil
	case *ast.StreamType:
		elem, err := fc.resolveTypeExpr(t.Elem)
		if err != nil {
			return "", err
		}
		return makeStreamType(elem), nil
	default:
		return "", fmt.Errorf("sema: unsupported type expression %T", te)
	}
}

// resolveReturnTypeExpr is resolveTypeExpr plus one extra case usable
// only in a function's own return-type position (amifl-spec.md section
// 8.3, "戻り値無しはfn(T1, ...) -> Unit") — mirrors canonicalReturnType's
// identical relationship to canonicalType.
func (fc *funcChecker) resolveReturnTypeExpr(te ast.TypeExpr) (string, error) {
	if n, ok := te.(*ast.NamedType); ok && n.Name == "Unit" {
		return unitType, nil
	}
	return fc.resolveTypeExpr(te)
}

// evalConstArraySize reduces e to a concrete non-negative integer,
// walking a plain IntLit or an IdentExpr's ConstValue chain. Deliberately
// doesn't evaluate arithmetic (a `const N = 3 + 3` used as an array size)
// — AMIVM's ARTYPE instruction requires a literal immediate, so something
// has to fully reduce the expression at compile time, and doing that
// generally would mean re-implementing Go's typed arithmetic semantics in
// sema (exactly what step 3's design decision for `const` initializers
// chose *not* to do — see CLAUDE.md's "確定した設計判断" for step 3's
// ConstDecl handling, "この畳み込みをsema内に実装するとGoの算術演算の意味論
// をAmiFL側で二重に持つことになる"). Scope-limited to what's actually
// needed for an array size: a literal, or a chain of const references
// down to one — a documented step 7 limitation, not an oversight.
func evalConstArraySize(e ast.Expr) (uint64, error) {
	switch v := e.(type) {
	case *ast.IntLit:
		return v.Value, nil
	case *ast.IdentExpr:
		if v.ConstValue != nil {
			return evalConstArraySize(v.ConstValue)
		}
	}
	return 0, fmt.Errorf("must be a literal integer or a reference to a const holding one (arithmetic array-size expressions aren't supported yet)")
}

// resolveConstDecl type-checks a const declaration's initializer and
// returns its canonical type together with the expression to inline at
// its use sites (amifl-spec.md section 4: "初期化式はリテラルまたは const
// どうしの演算のみ...参照箇所へインライン展開される"). Unlike step 2, this
// no longer needs to fold the initializer down to a single literal value
// at declaration time: as of step 3, codegen's genValue can already
// regenerate any literal/operator/const-reference expression inline at
// every reference site (BinaryExpr/UnaryExpr included), so the checked
// expression tree itself — d.Value, exactly as checkExpr left it — is a
// perfectly good "value to inline". This sidesteps needing a second,
// compile-time arithmetic evaluator inside sema that would have to
// duplicate Go's own per-type wraparound semantics; requireConstExpr below
// still walks the tree to reject anything that isn't compile-time-constant
// (a `let` reference, a call, ...), which is the actual job amifl-spec.md
// requires here.
func resolveConstDecl(fc *funcChecker, d *ast.ConstDecl) (string, ast.Expr, error) {
	var expected string
	if d.Type != nil {
		t, err := fc.resolveTypeExpr(d.Type)
		if err != nil {
			return "", nil, err
		}
		expected = t
	}

	typ, err := fc.checkExpr(d.Value, expected)
	if err != nil {
		return "", nil, err
	}
	if typ == unitType {
		return "", nil, fmt.Errorf("line %d: cannot bind const %q to a Unit-typed value", d.Line, d.Name)
	}
	if err := requireConstExpr(d.Value); err != nil {
		return "", nil, fmt.Errorf("line %d: const %q initializer %s", d.Line, d.Name, err)
	}
	return typ, d.Value, nil
}

// requireConstExpr rejects any part of e that isn't a literal, a reference
// to another const (checkExpr already populates ConstValue for those, via
// resolveIdentExpr — a `let`/parameter reference never gets one), or an
// operator expression built from such values (amifl-spec.md section 4).
func requireConstExpr(e ast.Expr) error {
	switch v := e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.BoolLit, *ast.StringLit:
		return nil
	case *ast.IdentExpr:
		if v.ConstValue == nil {
			return fmt.Errorf("references %q, which is not a const", v.Name)
		}
		return nil
	case *ast.BinaryExpr:
		if err := requireConstExpr(v.Left); err != nil {
			return err
		}
		return requireConstExpr(v.Right)
	case *ast.UnaryExpr:
		return requireConstExpr(v.Operand)
	case *ast.TupleLit:
		for _, e := range v.Elems {
			if err := requireConstExpr(e); err != nil {
				return err
			}
		}
		return nil
	case *ast.StructLit:
		for _, f := range v.Fields {
			if err := requireConstExpr(f.Value); err != nil {
				return err
			}
		}
		return nil
	case *ast.FieldExpr:
		return requireConstExpr(v.Target)
	default:
		return fmt.Errorf("must be a literal, a const reference, or an operator expression over those")
	}
}
