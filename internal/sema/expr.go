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
	}
	return b.typ, nil
}

func (fc *funcChecker) resolveCallExpr(v *ast.CallExpr) (string, error) {
	if v.Callee != "print" {
		return "", fmt.Errorf("line %d: step 2 only supports calling the built-in `print` (general function calls arrive in step 5; the built-in function library arrives in step 11)", v.Line)
	}
	if len(v.Args) != 1 {
		return "", fmt.Errorf("line %d: print expects exactly 1 argument, got %d", v.Line, len(v.Args))
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	return unitType, nil
}

func (fc *funcChecker) resolveLetExpr(v *ast.LetExpr) (string, error) {
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
	if err := fc.declare(v.Name, &binding{typ: typ}); err != nil {
		return "", fmt.Errorf("line %d: %s", v.Line, err)
	}
	v.ResolvedType = typ
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
	if b.isConst {
		return "", fmt.Errorf("line %d: cannot assign to %q: it is a const", v.Line, v.Name)
	}
	if _, err := fc.checkExpr(v.Value, b.typ); err != nil {
		return "", err
	}
	return unitType, nil
}

func (fc *funcChecker) resolveDiscardExpr(v *ast.DiscardExpr) (string, error) {
	if _, err := fc.checkExpr(v.Value, ""); err != nil {
		return "", err
	}
	return unitType, nil
}

// resolveConstDecl type-checks a const declaration's initializer and
// returns its canonical type together with the literal to inline at its
// use sites. Step 2 requires the initializer to resolve to a literal —
// either directly, or transitively through references to earlier consts
// (amifl-spec.md section 4's "リテラルまたはconstどうしの演算のみ"; the
// "演算" half — arithmetic between consts — arrives with operators in
// step 3, so for now only the "リテラル" half, plus bare const-to-const
// references, is reachable).
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

	lit, err := literalValueOf(d.Value)
	if err != nil {
		return "", nil, fmt.Errorf("line %d: const %q: %s", d.Line, d.Name, err)
	}
	return typ, lit, nil
}

// literalValueOf resolves e to the literal expression it ultimately
// stands for: e itself if it already is one, or — since checkExpr has
// already run on e and so populated ConstValue for any const-referencing
// IdentExpr (resolveIdentExpr) — the const's own (already-flattened)
// literal.
func literalValueOf(e ast.Expr) (ast.Expr, error) {
	switch v := e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.BoolLit, *ast.StringLit:
		return e, nil
	case *ast.IdentExpr:
		if v.ConstValue != nil {
			return v.ConstValue, nil
		}
	}
	return nil, fmt.Errorf("initializer must be a literal or a reference to another const (step 2 limitation until operators land in step 3)")
}
