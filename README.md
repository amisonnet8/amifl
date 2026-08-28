# AmiFL

**Status: early scaffold — the compiler pipeline is not implemented yet.** See `CLAUDE.md` for the implementation plan.

AmiFL is a statically typed, general-purpose data-processing language in the spirit of AWK/Perl — but deliberately avoiding the syntactic clutter Perl accumulated over time. It leans on a small set of expression-oriented control constructs, polymorphic built-in functions, and a pipe operator (`|>`) for readable data transformation pipelines.

AmiFL is the fourth language built on top of [AMIVM](https://github.com/amisonnet8/amivm), following [Seed](https://github.com/amisonnet8/seed) → [Cascade](https://github.com/amisonnet8/cascade) → [Weave](https://github.com/amisonnet8/weave).

```
AmiFL source (.aml)
  |  (this repository's scope)
  v
AMIVM-IR (.ir)
  |  (amivm, an external CLI tool)
  v
Go source (.go)
  |  (go build, orchestrated by this repository's build pipeline)
  v
native executable
```

## Requirements

- Go (see `go.mod` for the minimum version)
- [`amivm`](https://github.com/amisonnet8/amivm) installed and on `PATH`:

  ```sh
  go install github.com/amisonnet8/amivm/cmd/amivm@latest
  ```

## Install

```sh
go install github.com/amisonnet8/amifl/cmd/amifl@latest
```

## Usage

```sh
amifl <command> [flags] <file.aml | package-dir | package.amlz>
```

| Command | Description |
|---|---|
| `build` | compile to a native executable |
| `run` | compile and immediately run |
| `emit-ir` | compile to AMIVM-IR |
| `emit-go` | compile to Go source (via amivm) |
| `archive` | package a directory's `.aml` files into a `.amlz` archive |
| `help` | show the help message |

None of these are implemented yet — see `CLAUDE.md`'s implementation plan.

## Example

```amifl
fn main(args: List[String]) -> Int {
    print("Hello, AmiFL!")
    0
}
```

## Language spec

The full language specification lives in [`amifl-spec.md`](./amifl-spec.md).

## Repository layout

See `amifl-spec.md` section 16.1 and `CLAUDE.md`'s "リポジトリ構成" section.

## License

MIT — see [`LICENSE`](./LICENSE).
