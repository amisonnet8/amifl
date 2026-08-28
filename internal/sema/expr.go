package sema

import (
	"fmt"

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
		if v.Type != "" {
			return "", fmt.Errorf("line %d: a closure literal's type is always inferred from its own signature; remove the type annotation on %q", v.Line, v.Name)
		}
		typ, err := fc.resolveClosureLit(clos)
		if err != nil {
			return "", err
		}
		return fc.declareLet(v, typ)
	}

	var expected string
	if v.Type != "" {
		t, ok := canonicalType(v.Type)
		if !ok {
			return "", fmt.Errorf("line %d: unknown type %q", v.Line, v.Type)
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

	typ, err := fc.checkClosureBody(v, depth)

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
		pt, ok := canonicalType(p.Type)
		if !ok {
			return "", fmt.Errorf("line %d: unknown type %q", p.Line, p.Type)
		}
		p.ResolvedType = pt
		token := fmt.Sprintf("&%d-%d", depth, i+1)
		if err := fc.declare(p.Name, &binding{typ: pt, token: token}); err != nil {
			return "", fmt.Errorf("line %d: %s", p.Line, err)
		}
		paramTypes = append(paramTypes, pt)
	}

	retType, ok := canonicalReturnType(v.ReturnType)
	if !ok {
		return "", fmt.Errorf("line %d: unknown type %q", v.Line, v.ReturnType)
	}
	v.ResolvedReturnType = retType
	if _, err := fc.checkBlock(v.Body, retType); err != nil {
		return "", err
	}

	typ := makeFuncType(paramTypes, retType)
	v.ResolvedType = typ
	return typ, nil
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
	if d.Type != "" {
		t, ok := canonicalType(d.Type)
		if !ok {
			return "", nil, fmt.Errorf("line %d: unknown type %q", d.Line, d.Type)
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
	default:
		return fmt.Errorf("must be a literal, a const reference, or an operator expression over those")
	}
}
