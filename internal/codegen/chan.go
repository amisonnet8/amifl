// chan.go compiles amifl-spec.md sections 2.2/11/13.8's Chan[T] and
// Stream[T] — step 12. Chan[T]'s own built-ins (`chan[T]`/`send`/`recv`/
// `spawn`) map directly onto AMIVM's native CHTYPE/CHMAKE/CHSEND/CHRECV/
// SPAWN instructions with no amiflrt helper at all. Stream[T]'s relay
// operations (`take`/`skip`, and files.go's `lines`) are hand-rolled AMIVM-
// IR too — a CLOS spawned as a goroutine, unconditionally DEFERring a
// `close` of its own output channel on the way out (Cascade's own
// established "DEFERの綺麗な使い道" pattern, CLAUDE.md's cascade reference
// notes) — since a CHRECV-based drain loop plus a channel make/close is all
// either one needs; no synchronization primitive beyond what AMIVM already
// offers is required. `parallel`'s true multi-goroutine fan-in *does* need
// real synchronization (a completion count across N workers) that AMIVM has
// no instruction for at all, so that one genuinely needs amiflrt
// (streams.go's ParallelStream, CLAUDE.md's "専用のGoランタイムパッケージ
// を新設しないと実現不能" criterion) — the one built-in in this file that
// isn't hand-rolled. `collect` also stays in this file (it drains a Stream
// synchronously in the calling goroutine, no CLOS/SPAWN needed at all) since
// it's Stream[T]'s built-in, even though it reuses amiflrt.Push exactly the
// way step 11's `push` already does (builtins_data.go).
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// isChanType/chanElemType and isStreamType/streamElemType are codegen's own
// copies of sema's identical helpers (types.go's makeChanType/makeStreamType
// and friends) — ast is codegen's and sema's only shared vocabulary
// (CLAUDE.md's リポジトリ構成), so these string conventions have to be
// independently understood here too, exactly like isSetType/isMapType
// (maps.go).
func isChanType(t string) bool {
	return strings.HasPrefix(t, "Chan(") && strings.HasSuffix(t, ")")
}

func chanElemType(t string) string {
	return strings.TrimSuffix(strings.TrimPrefix(t, "Chan("), ")")
}

func isStreamType(t string) bool {
	return strings.HasPrefix(t, "Stream(") && strings.HasSuffix(t, ")")
}

func streamElemType(t string) string {
	return strings.TrimSuffix(strings.TrimPrefix(t, "Stream("), ")")
}

// chanGoTypeName mints (or reuses) the synthesized Go/AMIVM channel type
// backing one Chan[T] shape, keyed by its full canonical string — mirrors
// setGoTypeName (maps.go) exactly, minus the hardcoded bool value type.
func (p *program) chanGoTypeName(canonical string) string {
	if name, ok := p.chanTypes[canonical]; ok {
		return name
	}
	elemGoType := p.resolveGoType(chanElemType(canonical))
	p.chanSeq++
	name := fmt.Sprintf("AmiflChan%d", p.chanSeq)
	if p.chanTypes == nil {
		p.chanTypes = map[string]string{}
	}
	p.chanTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "CHTYPE\t^%s\t^%s\n", name, elemGoType)
	return name
}

// streamGoTypeName is chanGoTypeName's Stream[T] counterpart — its own
// CHTYPE, separate from chanGoTypeName's even for the structurally
// identical element type, exactly like setGoTypeName/mapGoTypeName's
// step-10 precedent (ast.StreamType's doc comment explains why).
func (p *program) streamGoTypeName(canonical string) string {
	if name, ok := p.streamTypes[canonical]; ok {
		return name
	}
	elemGoType := p.resolveGoType(streamElemType(canonical))
	p.streamSeq++
	name := fmt.Sprintf("AmiflStream%d", p.streamSeq)
	if p.streamTypes == nil {
		p.streamTypes = map[string]string{}
	}
	p.streamTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "CHTYPE\t^%s\t^%s\n", name, elemGoType)
	return name
}

// emitChanRecv emits a fresh VAR pair and a CHRECV of chanVal into them,
// returning both tokens (already "%"-prefixed) — the value+ok pair shared
// by every consumer of a channel/stream (recv, `for x in stream`,
// take/skip/collect's own hand-rolled drain loops).
func (g *gen) emitChanRecv(chanVal, elemGoType string) (valTok, okTok string) {
	valTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", valTmp, elemGoType)
	okTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", okTmp)
	fmt.Fprintf(g.b, "\tCHRECV\t%%%s\t%%%s\t%s\n", valTmp, okTmp, chanVal)
	return "%" + valTmp, "%" + okTmp
}

// emitBreakUnlessOk emits `IF NOT okTok { BREAK }` — the "channel closed,
// stop looping" guard every CHRECV-based drain loop in this file and
// collections.go's genForStreamStmt needs right after emitChanRecv.
func (g *gen) emitBreakUnlessOk(okTok string) {
	notTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", notTmp)
	fmt.Fprintf(g.b, "\tNOT\t%%%s\t%s\n", notTmp, okTok)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", notTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
}

// beginRelayClosure opens a 0-arg, Unit-returning CLOS whose first
// instruction unconditionally DEFERs closing outChanVal (Go's builtin
// `close`, called the same raw-Go-function-name way `?len`/`?panic`/
// `?delete` already are elsewhere in this codebase — an internal codegen
// detail, not the user-facing File `close` built-in files.go implements
// separately) — every caller (genTakeValue/genSkipValue/genLinesValue)
// fills in its own loop body between this and endRelayClosure, guaranteeing
// the output stream gets closed no matter which BREAK/return path the body
// takes, exactly the "後始末が...保証できる" property Cascade's own
// identical pattern established (CLAUDE.md's cascade reference notes).
func (g *gen) beginRelayClosure(outChanVal string) string {
	typeName := g.prog.newFuncTypeDecl(nil, "")
	tok := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tok, typeName)
	fmt.Fprintf(g.b, "\tCLOS\t%%%s\t:\n", tok)
	fmt.Fprintf(g.b, "\tDEFER\t?close\t%s\n", outChanVal)
	return "%" + tok
}

// endRelayClosure closes out the CLOS beginRelayClosure opened and SPAWNs
// it — mirroring genClosureLitInto's own Unit-body RET-before-ENDCLOS
// convention (closure.go).
func (g *gen) endRelayClosure(closTok string) {
	g.b.WriteString("\tRET\n")
	g.b.WriteString("\tENDCLOS\n")
	g.b.WriteString("\tSPAWN\t" + closTok + "\n")
}

// genChanValue emits `chan[T](buffer)` (amifl-spec.md section 13.8): CHMAKE
// straight into a fresh CHTYPE-declared variable.
func (g *gen) genChanValue(c *ast.CallExpr) (string, error) {
	bufVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tCHMAKE\t%%%s\t^%s\t%s\n", tmp, goType, bufVal)
	return "%" + tmp, nil
}

// genSendStmt emits `send(ch, v) -> Unit` (amifl-spec.md section 13.8) as a
// bare CHSEND.
func (g *gen) genSendStmt(c *ast.CallExpr) error {
	chVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	vVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tCHSEND\t%s\t%s\n", chVal, vVal)
	return nil
}

// genRecvValue emits `recv(ch) -> Tuple2[T, Bool]` (amifl-spec.md section
// 13.8): CHRECV's own native value+ok pair assembled into the Tuple2 STTYPE
// exactly like errors.go/builtins.go's other Go-multi-value-to-Tuple2
// conversions (parse[T], `?`'s own early-return path).
func (g *gen) genRecvValue(c *ast.CallExpr) (string, error) {
	chVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	elemGoType := g.prog.resolveGoType(chanElemType(c.ArgTypes[0]))
	valTok, okTok := g.emitChanRecv(chVal, elemGoType)
	return g.assembleTuple2(c.ResolvedType, valTok, okTok)
}

// genSpawnStmt emits `spawn(f) -> Unit` (amifl-spec.md section 11/13.8) as
// a bare SPAWN of f's own token (f is always a `let`-bound closure
// reference by the time it reaches here — sema's resolveSpawn requires
// fn()->Unit, and step 5 never lets a bare ClosureLit appear anywhere but a
// `let`'s direct value, so genValue always resolves it to a plain
// "%f_N"/"$N"/"&L-N" token, never something needing its own evaluation).
func (g *gen) genSpawnStmt(c *ast.CallExpr) error {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	g.b.WriteString("\tSPAWN\t" + fVal + "\n")
	return nil
}

// genTakeValue emits `take(s, n) -> Stream[T]` (amifl-spec.md section
// 13.8).
func (g *gen) genTakeValue(c *ast.CallExpr) (string, error) {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	nVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	return g.genTakeStream(sVal, nVal, c.ArgTypes[0], c.ResolvedType)
}

// genTakeStream is genTakeValue's core, factored out so
// genSliceBuiltinValue (builtins_data.go) can compose `slice(s, from, to)`
// on a Stream as skip(s, from) |> take(_, to-from) without needing a fake
// *ast.CallExpr to drive genTakeValue's own arg-evaluation — it emits a
// relay closure that forwards up to n elements from sVal (a Stream[T] whose
// canonical type is streamTyp) into a fresh output stream, then stops
// (letting beginRelayClosure/endRelayClosure's DEFER close the output
// either way — whether n elements were forwarded or sVal closed first).
func (g *gen) genTakeStream(sVal, nVal, streamTyp, resultTyp string) (string, error) {
	elemGoType := g.prog.resolveGoType(streamElemType(streamTyp))
	outGoType := g.prog.resolveGoType(resultTyp)
	outTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", outTmp, outGoType)
	fmt.Fprintf(g.b, "\tCHMAKE\t%%%s\t^%s\t0\n", outTmp, outGoType)
	outVal := "%" + outTmp

	closTok := g.beginRelayClosure(outVal)

	iTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", iTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t0\n", iTmp)
	g.b.WriteString("\tLOOP\n")
	doneTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", doneTmp)
	fmt.Fprintf(g.b, "\tGTE\t%%%s\t%%%s\t%s\n", doneTmp, iTmp, nVal)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", doneTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	valTok, okTok := g.emitChanRecv(sVal, elemGoType)
	g.emitBreakUnlessOk(okTok)
	fmt.Fprintf(g.b, "\tCHSEND\t%s\t%s\n", outVal, valTok)
	fmt.Fprintf(g.b, "\tADD\t%%%s\t%%%s\t1\n", iTmp, iTmp)
	g.b.WriteString("\tENDLOOP\n")

	g.endRelayClosure(closTok)
	return outVal, nil
}

// genSkipValue emits `skip(s, n) -> Stream[T]` (amifl-spec.md section
// 13.8).
func (g *gen) genSkipValue(c *ast.CallExpr) (string, error) {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	nVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	return g.genSkipStream(sVal, nVal, c.ArgTypes[0], c.ResolvedType)
}

// genSkipStream is genSkipValue's core (factored out for
// genSliceBuiltinValue exactly like genTakeStream above): a relay closure
// that first drains (without forwarding) up to n elements from sVal, then
// forwards everything after that — a two-phase loop, since AMIVM's LOOP/
// BREAK can't express "skip n, then forward the rest" as a single pass.
// exhaustedTmp records whether sVal closed *during* the skip phase, so the
// forwarding phase (a second LOOP) is skipped entirely rather than
// spuriously CHRECVing an already-closed channel again — that would still
// be correct (a closed channel always immediately returns ok=false), but
// the flag avoids an extra no-op iteration and keeps the two phases
// symmetric with genTakeStream's single-phase loop.
func (g *gen) genSkipStream(sVal, nVal, streamTyp, resultTyp string) (string, error) {
	elemGoType := g.prog.resolveGoType(streamElemType(streamTyp))
	outGoType := g.prog.resolveGoType(resultTyp)
	outTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", outTmp, outGoType)
	fmt.Fprintf(g.b, "\tCHMAKE\t%%%s\t^%s\t0\n", outTmp, outGoType)
	outVal := "%" + outTmp

	closTok := g.beginRelayClosure(outVal)

	exhaustedTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", exhaustedTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\tfalse\n", exhaustedTmp)

	iTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", iTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t0\n", iTmp)
	g.b.WriteString("\tLOOP\n")
	doneTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", doneTmp)
	fmt.Fprintf(g.b, "\tGTE\t%%%s\t%%%s\t%s\n", doneTmp, iTmp, nVal)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", doneTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	_, okTok := g.emitChanRecv(sVal, elemGoType)
	notOkTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", notOkTmp)
	fmt.Fprintf(g.b, "\tNOT\t%%%s\t%s\n", notOkTmp, okTok)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", notOkTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\ttrue\n", exhaustedTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	fmt.Fprintf(g.b, "\tADD\t%%%s\t%%%s\t1\n", iTmp, iTmp)
	g.b.WriteString("\tENDLOOP\n")

	notExhaustedTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", notExhaustedTmp)
	fmt.Fprintf(g.b, "\tNOT\t%%%s\t%%%s\n", notExhaustedTmp, exhaustedTmp)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", notExhaustedTmp)
	g.b.WriteString("\tLOOP\n")
	valTok, okTok2 := g.emitChanRecv(sVal, elemGoType)
	g.emitBreakUnlessOk(okTok2)
	fmt.Fprintf(g.b, "\tCHSEND\t%s\t%s\n", outVal, valTok)
	g.b.WriteString("\tENDLOOP\n")
	g.b.WriteString("\tENDIF\n")

	g.endRelayClosure(closTok)
	return outVal, nil
}

// genCollectValue emits `collect(s) -> List[T]` (amifl-spec.md section
// 13.8): drains s synchronously (in the calling goroutine — collect blocks
// until s closes, by design) into a growable List, one amiflrt.Push per
// element (step 11's own established non-destructive-append helper,
// builtins_data.go's genPushValue) — AMIVM has no native "append" any more
// here than it did for step 11's `push`, and Stream's final size is
// unknown up front (unlike genForYieldValue's List/Array `for`, which can
// `?len`-preallocate), so a Push-per-element loop is the only option.
func (g *gen) genCollectValue(c *ast.CallExpr) (string, error) {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	elemGoType := g.prog.resolveGoType(streamElemType(c.ArgTypes[0]))
	listGoType := g.prog.resolveGoType(c.ResolvedType)
	listTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", listTmp, listGoType)
	fmt.Fprintf(g.b, "\tSLMAKE\t%%%s\t^%s\t0\n", listTmp, listGoType)
	listVal := "%" + listTmp

	g.b.WriteString("\tLOOP\n")
	valTok, okTok := g.emitChanRecv(sVal, elemGoType)
	g.emitBreakUnlessOk(okTok)
	g.writeGenericCall([]string{listVal}, "?amiflrt.Push", []string{elemGoType}, []string{listVal, valTok})
	g.b.WriteString("\tENDLOOP\n")
	return listVal, nil
}

// genParallelValue emits `parallel(s, workers) -> Stream[T]` (amifl-spec.md
// section 11/13.8) via amiflrt.ParallelStream[T] (streams.go) — the one
// Stream operation in this file needing genuine cross-goroutine
// synchronization (a completion count across N workers) AMIVM has no
// instruction for, so, unlike take/skip/collect above, this delegates to
// amiflrt entirely rather than hand-rolling IR.
func (g *gen) genParallelValue(c *ast.CallExpr) (string, error) {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	workersVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	elemGoType := g.prog.resolveGoType(streamElemType(c.ArgTypes[0]))
	outGoType := g.prog.resolveGoType(c.ResolvedType)
	outTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", outTmp, outGoType)
	g.writeGenericCall([]string{"%" + outTmp}, "?amiflrt.ParallelStream", []string{elemGoType}, []string{sVal, workersVal})
	return "%" + outTmp, nil
}
