// extern.go type-checks `extern "path" as alias { type Name ... bind
// Name(params) -> Ret [as GoTarget] ... }` declarations (amifl-spec.md
// section 15) — step 13, resolving CLAUDE.md's design issue 1 (the Any/
// extern value boundary). See ast.ExternDecl's doc comment for the overall
// shape and ast.ExternBindDecl's for GoTarget's two forms.
package sema

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// reservedExternAliases are Go identifiers a user-chosen `as alias` can
// never claim: every package alias codegen's own generated file already
// relies on being able to reference bare (os/fmt/strconv/strings/math/
// amiflrt), plus every Go predeclared identifier (bool/int64/len/panic/
// close/delete/append/...). Passing `-i alias=path` to amivm adds an
// *explicit* `import alias "path"` to the top of the generated file
// (CLAUDE.md's "amivmのインストール・呼び出し方") — if alias shadowed one
// of these, every one of codegen's own unrelated `?os.Exit`/`?len`/
// `?int64`-style references elsewhere in the *same* file would silently
// resolve to the user's extern import (or, for a predeclared identifier,
// simply stop being the builtin) instead of what codegen actually meant,
// since Go import aliases and predeclared identifiers share one file-level
// namespace. This is a purely defensive, conservative list — reserving a
// name AmiFL doesn't even use today (e.g. "complex128") costs nothing and
// guards against a future codegen extension quietly reintroducing this
// exact bug.
var reservedExternAliases = map[string]bool{
	"os": true, "fmt": true, "strconv": true, "strings": true, "math": true, "amiflrt": true,
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"true": true, "false": true, "iota": true, "nil": true,
	"append": true, "cap": true, "clear": true, "close": true, "complex": true,
	"copy": true, "delete": true, "imag": true, "len": true, "make": true,
	"max": true, "min": true, "new": true, "panic": true, "print": true,
	"println": true, "real": true, "recover": true,
	"any": true, "comparable": true,
}

// registerExternTypes validates d's own `as alias` (Check's pass 0a,
// alongside registerStructName/registerEnumName) and registers every
// `type Name` entry it declares. Binds are deliberately not touched here —
// registerExternBind runs later (Check's pass 1, alongside
// registerFuncSig) since a bind's parameter/return types may reference a
// struct/enum declared anywhere in the file, which aren't fully known
// until pass 0 finishes.
func (c *checker) registerExternTypes(d *ast.ExternDecl) error {
	if d.Path == "" {
		return fmt.Errorf("line %d: extern block needs a non-empty package path", d.Line)
	}
	if reservedExternAliases[d.Alias] {
		return fmt.Errorf("line %d: %q can't be used as an extern alias (reserved — collides with a name AmiFL's generated code relies on)", d.Line, d.Alias)
	}
	if _, exists := c.externAliases[d.Alias]; exists {
		return fmt.Errorf("line %d: extern alias %q is already declared", d.Line, d.Alias)
	}
	c.externAliases[d.Alias] = d.Path

	for ti := range d.Types {
		t := &d.Types[ti]
		if t.Name == reservedMainName {
			return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", t.Line, t.Name)
		}
		if _, exists := c.structs[t.Name]; exists {
			return fmt.Errorf("line %d: %q is already declared as a struct", t.Line, t.Name)
		}
		if _, exists := c.enums[t.Name]; exists {
			return fmt.Errorf("line %d: %q is already declared as an enum", t.Line, t.Name)
		}
		if _, exists := c.externTypes[t.Name]; exists {
			return fmt.Errorf("line %d: duplicate extern type %q", t.Line, t.Name)
		}
		c.externTypes[t.Name] = true
	}
	return nil
}

// resolveExternTypeExpr resolves a type annotation appearing inside an
// extern bind's parameter list — identical to funcChecker.resolveTypeExpr
// except it also accepts the bare name "Any" (amifl-spec.md section 2.2:
// "Anyは...externの経由のみ登場"), which is nowhere else in the
// type-annotation grammar a valid type (canonicalType never recognizes
// it — see checkExpr's own "Any" bypass in expr.go for the other half of
// how a genuinely dynamic Any value flows through the type system without
// ever needing a general-purpose annotation grammar for it).
func (fc *funcChecker) resolveExternTypeExpr(t ast.TypeExpr) (string, error) {
	if nt, ok := t.(*ast.NamedType); ok && nt.Name == "Any" {
		return "Any", nil
	}
	return fc.resolveTypeExpr(t)
}

// resolveExternReturnTypeExpr is resolveExternTypeExpr plus
// canonicalReturnType's extra "Unit" case (amifl-spec.md section 8.3),
// for a bind's own return-type position — a bind wrapping a Go function
// with no return value needs to be writable as `-> Unit` exactly like an
// ordinary `fn` can.
func (fc *funcChecker) resolveExternReturnTypeExpr(t ast.TypeExpr) (string, error) {
	if nt, ok := t.(*ast.NamedType); ok {
		if nt.Name == "Any" {
			return "Any", nil
		}
		if nt.Name == "Unit" {
			return unitType, nil
		}
	}
	return fc.resolveTypeExpr(t)
}

// registerExternBind resolves and records one `bind` entry's signature
// (Check's pass 1, run for every extern block's every bind alongside
// registerFuncSig) — bind names share the exact same no-overloading
// namespace a top-level `fn` occupies (c.funcs), so a bind and a `fn`
// (or another bind) can't collide, mirroring registerFuncSig's own checks
// almost verbatim.
func (c *checker) registerExternBind(alias string, b *ast.ExternBindDecl) error {
	if b.Name == reservedMainName {
		return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", b.Line, b.Name)
	}
	if _, exists := c.funcs[b.Name]; exists {
		return fmt.Errorf("line %d: duplicate function %q", b.Line, b.Name)
	}
	if _, exists := c.globals[b.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as a const", b.Line, b.Name)
	}

	fc := newFuncChecker(c)
	seen := map[string]bool{}
	var params []string
	for i := range b.Params {
		p := &b.Params[i]
		if seen[p.Name] {
			return fmt.Errorf("line %d: duplicate parameter %q", p.Line, p.Name)
		}
		seen[p.Name] = true
		pt, err := fc.resolveExternTypeExpr(p.Type)
		if err != nil {
			return err
		}
		p.ResolvedType = pt
		params = append(params, pt)
	}

	retType, err := fc.resolveExternReturnTypeExpr(b.ReturnType)
	if err != nil {
		return err
	}
	b.ResolvedReturnType = retType

	goTarget := b.GoTarget
	if goTarget == "" {
		goTarget = b.Name
	}

	sig := funcSig{params: params, ret: retType}
	if dotIdx := strings.IndexByte(goTarget, '.'); dotIdx >= 0 {
		recvType := goTarget[:dotIdx]
		methodName := goTarget[dotIdx+1:]
		if len(params) == 0 {
			return fmt.Errorf("line %d: method-style bind %q (as %s) needs at least one parameter to serve as the receiver", b.Line, b.Name, goTarget)
		}
		if params[0] != recvType {
			return fmt.Errorf("line %d: bind %q targets method %s.%s, but its first parameter has type %s, not %s", b.Line, b.Name, recvType, methodName, params[0], recvType)
		}
		sig.externMethod = methodName
	} else {
		sig.externCallee = "?" + alias + "." + goTarget
	}
	c.funcs[b.Name] = sig
	return nil
}
