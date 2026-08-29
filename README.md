# AmiFL

[![test](https://github.com/amisonnet8/amifl/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/amifl/actions/workflows/test.yml)

A statically typed, general-purpose data-processing language in the spirit of AWK/Perl — but deliberately avoiding the syntactic clutter Perl accumulated over time — implemented in Go, compiling to Go source via AMIVM-IR.

> [日本語版 README はこちら](README_ja.md)

## Status

AmiFL's front end (lexer, parser, semantic checker, and AMIVM-IR code generator) implements the full language described in [`amifl-spec.md`](amifl-spec.md): scalar types with no implicit conversion, `let`/`const`, expression-oriented control flow (`if`/`elif`/`else`, `while`, `switch` — both the boolean-only form and the enum-pattern-matching form), functions and closures (including higher-order functions via a `Func` type), `Tuple2`–`Tuple8`, `struct`, `enum`, `Array[T;N]`/`List[T]`/`Range`/`for`, the pipe operator (`|>`, including inline-closure right-hand sides), `Tuple2[T, Error]`-based error handling with the postfix `?` operator, compiler-internal capability polymorphism for ~50 built-in functions, `Set[T]`/`Map[K,V]`, `Chan[T]`/`Stream[T]`-based concurrency and file I/O, Go-asset binding (`extern`), and multi-file/multi-package modules (including `.amlz` archive packages).

AmiFL is the fourth language built on top of [AMIVM](https://github.com/amisonnet8/amivm), following [Seed](https://github.com/amisonnet8/seed) → [Cascade](https://github.com/amisonnet8/cascade) → [Weave](https://github.com/amisonnet8/weave).

## Pipeline

```
AmiFL source (.aml)
  ↓ (AmiFL — this repository)
AMIVM-IR (.ir)
  ↓ amivm (external tool, github.com/amisonnet8/amivm)
Go source (.go)
  ↓ go build
native executable
```

AmiFL's own responsibility stops at emitting AMIVM-IR. Turning that into Go source is [amivm](https://github.com/amisonnet8/amivm)'s job, and turning that into an executable is a plain `go build` — both are separate tools `amifl` shells out to, not something this repository implements itself.

## Requirements

- Go, matching the version in [`go.mod`](go.mod).
- [`amivm`](https://github.com/amisonnet8/amivm) on your `PATH`.

## Install

```sh
go install github.com/amisonnet8/amivm/cmd/amivm@latest
go install github.com/amisonnet8/amifl/cmd/amifl@latest
```

Both land in `$GOBIN` (or `$GOPATH/bin` if unset) — make sure that directory is on your `PATH`. Since every AmiFL build ends in a plain `go build`, having Go installed already covers every dependency `amifl` needs at runtime; there's nothing else to fetch.

## Usage

```
amifl <command> [flags] <file.aml | package-dir | package.amlz>
```

A package directory (or a `.amlz` archive of one, §16.2 of the spec) compiles every `.aml` file directly inside it as one package (§12.1); a single file compiles only that file, ignoring any siblings in the same directory.

| Command | Output |
|---|---|
| `build` | a native executable |
| `run` | compiles and immediately runs, streaming its stdin/stdout/stderr |
| `emit-ir` | the AMIVM-IR |
| `emit-go` | the Go source (via amivm) |
| `archive` | packages a directory's direct `.aml` files into a `.amlz` archive |
| `help` | this command list |

`build`, `emit-ir`, and `emit-go` accept:

| Flag | Description |
|---|---|
| `-o <file>` | output file path (default: derived from the input path, e.g. `foo.aml` → `foo`/`foo.ir`/`foo.go`; a directory or `.amlz` archive → its own base name) |
| `-v` | show each pipeline stage's output as it runs (the generated IR, amivm's own `-v` trace, the final Go source) |

`archive` accepts `-o <file>` (default: `<directory-name>.amlz`).

## Example

```amifl
fn main() -> Int {
    print("Hello, AmiFL!")
    0
}
```

```sh
$ amifl run hello.aml
Hello, AmiFL!
```

A slightly larger taste of the pipe operator, capability-polymorphic built-ins, and `Tuple2[T, Error]`-based error handling:

```amifl
fn main() -> Int {
    let words: List[String] = ["10", "abc", "30", "7"]
    let nums = for w in words yield okOr(parse[Int](w), 0)
    let add = fn(acc: Int, x: Int) -> Int { acc + x }

    print(nums |> reduce(_, 0, add))   // 47 — "abc" parsed as 0
    0
}
```

More runnable examples covering scalars/operators, control flow, functions/closures, collections, tuples/structs/enums, the pipe operator/error handling, sets/maps, concurrency/file I/O, `extern`/modules, and task-oriented programs (FizzBuzz, word count, an RPN calculator, and more) live in [`examples/`](examples/).

New to AmiFL? [`tour/`](tour/) is an 11-chapter, chapter-by-chapter introduction (currently Japanese only). There's also an [interactive guide built with Gemini Notebook](https://notebook.google.com/notebook/1fc67606-7665-4325-bbb1-028e373b76a7).

## Language

**The only authoritative specification is [`amifl-spec.md`](amifl-spec.md).** If any other document (including this README) disagrees with it, `amifl-spec.md` wins. Section 17 lists what's deliberately not implemented and known limitations, so you don't have to guess whether something is missing on purpose.

## Repository layout

```
cmd/amifl/            CLI entry point (this README's `amifl` commands)
internal/lexer/       tokenizing
internal/parser/      parsing → AST
internal/ast/         AST definitions (the only vocabulary sema and codegen share —
                       they depend on this and not on each other)
internal/modloader/   resolves multi-file/multi-package `import` declarations (§12,
                       directories or .amlz archives) into the full package DAG
internal/sema/        semantic analysis: type checking, scope resolution, capability
                       resolution, pipeline type-connection checks (§9.1) — everything
                       amivm itself leaves to Go's go/types happens here first, so a
                       broken AmiFL program never reaches amivm as a confusing Go error
internal/codegen/     AST → AMIVM-IR
amiflrt/               AmiFL's Go runtime library (Stream[T]/Chan[T]/File and the
                       built-ins from §13 that don't map onto a single AMIVM
                       instruction), embedded into every amifl build
examples/              runnable .aml sample programs, one group per language feature
                       plus several task-oriented programs
tour/                  an 11-chapter introductory guide (Japanese)
amifl-spec.md          the AmiFL language specification (the only authoritative one)
amifl_implementation_notes.md
                       reusable AMIVM-IR-generation lessons learned while building this
                       frontend, for whoever implements the next one
CLAUDE.md              project conventions and the full log of design decisions made
                       while building this compiler
```

## License

MIT — see [`LICENSE`](LICENSE).
