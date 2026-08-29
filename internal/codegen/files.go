// files.go compiles amifl-spec.md section 13.10's file I/O built-ins —
// step 12. Every one but `lines` is a thin CALL/CALL-multi wrapper around
// amiflrt/file.go's low-level primitives, assembling their (payload, error)
// pair into a Tuple2 STTYPE exactly like builtins.go's genParseValue
// already does (the established "amiflrt returns Go native multi-value,
// codegen assembles the Tuple2" convention, CLAUDE.md's step-11 note).
// `lines` is the one exception: it hand-rolls a CLOS+DEFER+SPAWN relay
// (chan.go's beginRelayClosure/endRelayClosure) around a loop of
// amiflrt.ReadLineFile calls, turning a File into a Stream[String] the same
// way chan.go's take/skip hand-roll their own relay closures.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genOpenValue emits `open(path, mode) -> Tuple2[File, Error]`.
func (g *gen) genOpenValue(c *ast.CallExpr) (string, error) {
	pathVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	modeVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}

	fileGoType := g.prog.resolveGoType("File")
	fileTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", fileTmp, fileGoType)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCallMulti([]string{"%" + fileTmp, "%" + errTmp}, "?amiflrt.OpenFile", []string{pathVal, modeVal})

	return g.assembleTuple2(c.ResolvedType, "%"+fileTmp, "%"+errTmp)
}

// assembleTuple2 emits a fresh Tuple2[.,.]-typed temp, FSET from payloadVal/
// errVal into its F0/F1 fields — shared by every File built-in below (and
// chan.go's genRecvValue), mirroring builtins.go's genParseValue/errors.go's
// genTryValue, which each inline this same two-FSET pattern once for their
// own single use; File I/O has five call sites for it, so it's worth
// factoring out here.
func (g *gen) assembleTuple2(resolvedType, payloadVal, errVal string) (string, error) {
	tupleGoType := g.prog.resolveGoType(resolvedType)
	tupleTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tupleTmp, tupleGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%s\n", tupleTmp, payloadVal)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%s\n", tupleTmp, errVal)
	return "%" + tupleTmp, nil
}

// genCloseValue emits `close(f) -> Error`.
func (g *gen) genCloseValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCall("%"+errTmp, "?amiflrt.CloseFile", []string{fVal})
	return "%" + errTmp, nil
}

// genReadValue emits `read(f, n) -> Tuple2[Bytes, Error]`.
func (g *gen) genReadValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	nVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	bytesGoType := g.prog.resolveGoType(makeListType("UInt8"))
	bytesTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", bytesTmp, bytesGoType)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCallMulti([]string{"%" + bytesTmp, "%" + errTmp}, "?amiflrt.ReadFile", []string{fVal, nVal})

	return g.assembleTuple2(c.ResolvedType, "%"+bytesTmp, "%"+errTmp)
}

// genReadAllValue emits `readAll(f) -> Tuple2[Bytes, Error]`.
func (g *gen) genReadAllValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	bytesGoType := g.prog.resolveGoType(makeListType("UInt8"))
	bytesTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", bytesTmp, bytesGoType)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCallMulti([]string{"%" + bytesTmp, "%" + errTmp}, "?amiflrt.ReadAllFile", []string{fVal})

	return g.assembleTuple2(c.ResolvedType, "%"+bytesTmp, "%"+errTmp)
}

// genReadLineValue emits `readLine(f) -> Tuple2[String, Error]`.
func (g *gen) genReadLineValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	strTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", strTmp)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCallMulti([]string{"%" + strTmp, "%" + errTmp}, "?amiflrt.ReadLineFile", []string{fVal})

	return g.assembleTuple2(c.ResolvedType, "%"+strTmp, "%"+errTmp)
}

// genWriteValue emits `write(f, data) -> Tuple2[Int, Error]`.
func (g *gen) genWriteValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	dataVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	nTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", nTmp)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	g.writeCallMulti([]string{"%" + nTmp, "%" + errTmp}, "?amiflrt.WriteFile", []string{fVal, dataVal})

	return g.assembleTuple2(c.ResolvedType, "%"+nTmp, "%"+errTmp)
}

// stdFileFuncs maps stdin/stdout/stderr's builtin names to the amiflrt
// function each calls — the three share one codegen (genStdFileValue).
var stdFileFuncs = map[string]string{
	"stdin":  "?amiflrt.Stdin",
	"stdout": "?amiflrt.Stdout",
	"stderr": "?amiflrt.Stderr",
}

// genStdFileValue emits `stdin`/`stdout`/`stderr` () -> File.
func (g *gen) genStdFileValue(c *ast.CallExpr) (string, error) {
	fileGoType := g.prog.resolveGoType("File")
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, fileGoType)
	g.writeCall("%"+tmp, stdFileFuncs[c.Builtin], nil)
	return "%" + tmp, nil
}

// genLinesValue emits `lines(f) -> Stream[String]` (amifl-spec.md section
// 13.10/2.2, "Stream[T]は内部的にChan[T]+goroutineの糖衣"): a relay closure
// (chan.go's beginRelayClosure/endRelayClosure) that calls
// amiflrt.ReadLineFile in a loop, CHSENDing each successful line and
// stopping (letting the DEFER close the stream) the first time it returns a
// non-nil error — end-of-file included, since ReadLineFile's own contract
// (file.go) reports io.EOF exactly like any other read error here, with no
// special-casing needed to tell "done" apart from "failed".
func (g *gen) genLinesValue(c *ast.CallExpr) (string, error) {
	fVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}

	streamGoType := g.prog.resolveGoType(c.ResolvedType)
	streamTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", streamTmp, streamGoType)
	fmt.Fprintf(g.b, "\tCHMAKE\t%%%s\t^%s\t0\n", streamTmp, streamGoType)
	streamVal := "%" + streamTmp

	closTok := g.beginRelayClosure(streamVal)

	lineTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", lineTmp)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)

	g.b.WriteString("\tLOOP\n")
	g.writeCallMulti([]string{"%" + lineTmp, "%" + errTmp}, "?amiflrt.ReadLineFile", []string{fVal})
	hasErrTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", hasErrTmp)
	fmt.Fprintf(g.b, "\tNEQ\t%%%s\t%%%s\tnil\n", hasErrTmp, errTmp)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", hasErrTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	fmt.Fprintf(g.b, "\tCHSEND\t%s\t%%%s\n", streamVal, lineTmp)
	g.b.WriteString("\tENDLOOP\n")

	g.endRelayClosure(closTok)
	return streamVal, nil
}
