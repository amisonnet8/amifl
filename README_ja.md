# AmiFL

[![test](https://github.com/amisonnet8/amifl/actions/workflows/test.yml/badge.svg)](https://github.com/amisonnet8/amifl/actions/workflows/test.yml)

AWK/Perlの系譜に連なる、静的型付けの汎用データ処理特化言語です。Perlが積み重ねてきた文法の乱雑さを意図的に避けています。Go実装で、AMIVM-IRを経由してGoソースコードへコンパイルします。

> [English README is here](README.md)

## ステータス

AmiFLのフロントエンド(字句解析・構文解析・意味検査・AMIVM-IRコード生成)は、[`amifl-spec.md`](amifl-spec.md)に記載された言語仕様を全て実装済みです: 暗黙変換の無いスカラー型、`let`/`const`、式指向の制御構文(`if`/`elif`/`else`・`while`・`switch`——Bool専用形とenumパターンマッチ形の両方)、関数とクロージャー(`Func`型による高階関数含む)、`Tuple2`〜`Tuple8`、`struct`、`enum`、`Array[T;N]`/`List[T]`/`Range`/`for`、パイプ演算子(`|>`、右辺のインラインクロージャー含む)、`Tuple2[T, Error]`と後置`?`演算子による統一されたエラー処理、コンパイラ内部限定の「能力(Capability)」による約50個の組み込み関数の多相、`Set[T]`/`Map[K,V]`、`Chan[T]`/`Stream[T]`ベースの並列処理・ファイルI/O、Go資産バインド(`extern`)、複数ファイル・複数パッケージのモジュール機構(`.amlz`アーカイブパッケージ含む)。

AmiFLは[AMIVM](https://github.com/amisonnet8/amivm)上に実装する4つ目の言語で、[Seed](https://github.com/amisonnet8/seed) → [Cascade](https://github.com/amisonnet8/cascade) → [Weave](https://github.com/amisonnet8/weave)の系譜に続きます。

## パイプライン

```
AmiFLソース (.aml)
  ↓ (AmiFL — 本リポジトリ)
AMIVM-IR (.ir)
  ↓ amivm (外部ツール。github.com/amisonnet8/amivm)
Goソースコード (.go)
  ↓ go build
実行ファイル
```

AmiFL自身の責務はAMIVM-IRを出力するところまでです。それをGoソースへ変換するのは[amivm](https://github.com/amisonnet8/amivm)の仕事、実行ファイルにする単純な`go build`はさらに別工程で、どちらも`amifl`が呼び出す外部ツールであり、本リポジトリ自体が実装しているものではありません。

## 動作要件

- Go([`go.mod`](go.mod)記載のバージョン)
- `PATH`の通った場所にインストールされた[`amivm`](https://github.com/amisonnet8/amivm)

## インストール

```sh
go install github.com/amisonnet8/amivm/cmd/amivm@latest
go install github.com/amisonnet8/amifl/cmd/amifl@latest
```

どちらも`$GOBIN`(未設定なら`$GOPATH/bin`)に配置されるので、そのディレクトリが`PATH`に通っていることを確認してください。AmiFLのビルドは最終的に必ず素の`go build`で終わるため、Goさえインストールされていれば`amifl`が実行時に必要とするものは全て揃います(それ以外に取得すべきものはありません)。

## 使い方

```
amifl <コマンド> [フラグ] <file.aml | package-dir | package.amlz>
```

ディレクトリ(またはそれを圧縮した`.amlz`アーカイブ、仕様16.2節)を指定した場合、その直下の全`.aml`ファイルを1つのパッケージとしてコンパイルします(仕様12.1節)。単一ファイルを指定した場合はそのファイルだけをコンパイルし、同じディレクトリの他のファイルは無視します。

| コマンド | 出力 |
|---|---|
| `build` | 実行ファイル |
| `run` | コンパイルして即座に実行(stdin/stdout/stderrをそのまま引き継ぐ。ソースパスの後に続けた引数は`go run`同様プログラム自身へそのまま渡される) |
| `emit-ir` | AMIVM-IR |
| `emit-go` | Goソースコード(amivm経由) |
| `archive` | ディレクトリ直下の`.aml`ファイル群を`.amlz`アーカイブへまとめる |
| `help` | このコマンド一覧 |

`build`・`emit-ir`・`emit-go`は以下のフラグを受け付けます。

| フラグ | 説明 |
|---|---|
| `-o <file>` | 出力ファイルパス(省略時は入力パスから導出。例: `foo.aml` → `foo`/`foo.ir`/`foo.go`。ディレクトリ・`.amlz`アーカイブの場合はその名前自体) |
| `-v` | 各パイプライン段階の出力を実行しながら表示(生成されたIR、amivm自身の`-v`トレース、最終的なGoソース) |

`archive`は`-o <file>`(省略時は`<ディレクトリ名>.amlz`)を受け付けます。

## 例

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

パイプ演算子・能力多相の組み込み関数・`Tuple2[T, Error]`によるエラー処理を少しだけ味わえる例:

```amifl
fn main() -> Int {
    let words: List[String] = ["10", "abc", "30", "7"]
    let nums = for w in words yield okOr(parse[Int](w), 0)
    let add = fn(acc: Int, x: Int) -> Int { acc + x }

    print(nums |> reduce(_, 0, add))   // 47 — "abc"は0として扱われる
    0
}
```

スカラー・演算子・制御構文・関数/クロージャー・コレクション・タプル/構造体/enum・パイプ演算子/エラー処理・Set/Map・並列処理/ファイルI/O・`extern`/モジュールを一通り網羅した実行可能なサンプルに加え、タスク志向のサンプル(FizzBuzz・単語頻度カウント・逆ポーランド記法電卓など)も[`examples/`](examples/)に置いています。

AmiFLが初めての方は、11章立ての入門ガイド[`tour/`](tour/)からどうぞ(現在は日本語版のみ)。Gemini Notebookで作成した[インタラクティブガイド](https://notebook.google.com/notebook/1fc67606-7665-4325-bbb1-028e373b76a7)もあります。

## 言語仕様

**唯一の正確な仕様は[`amifl-spec.md`](amifl-spec.md)です。** 本READMEを含む他のドキュメントと矛盾する場合は`amifl-spec.md`を優先してください。17節に「意図的に実装していない機能・既知の限界」が一覧化されているので、書かれていないだけで実は使えるのでは、と迷う必要はありません。

## リポジトリ構成

```
cmd/amifl/            CLIエントリポイント(本READMEの`amifl`コマンド群)
internal/lexer/       字句解析
internal/parser/      構文解析 → AST
internal/ast/         AST定義(semaとcodegenが共有する唯一の語彙——両者はこれにのみ依存し、
                       互いには依存しない)
internal/modloader/   複数ファイル・複数パッケージのimport宣言(12節、ディレクトリまたは
                       .amlzアーカイブ)を、完全なパッケージDAGへ解決する
internal/sema/        意味検査: 型チェック・スコープ解決・capability解決・パイプライン型接続
                       検査(9.1節)——amivm自身がgo/typesへ委ねている検証を全てここで
                       先に済ませるため、壊れたAmiFLプログラムが分かりにくいGoのエラーとして
                       amivmから返ってくることはない
internal/codegen/     AST → AMIVM-IR生成
amiflrt/               AmiFL独自ランタイム(Stream[T]/Chan[T]/File、および13節の組み込み関数の
                       うち単一のAMIVM命令に対応しないもの。go:embedでamiflビルドのたびに
                       埋め込まれる)
examples/              実行可能な.amlサンプル(言語機能ごとにグループ化、加えて
                       タスク志向のサンプルも複数)
tour/                  11章立ての入門ガイド(日本語)
amifl-spec.md          AmiFL言語仕様(唯一の正確な仕様)
amifl_implementation_notes.md
                      このフロントエンドの実装で得た、AMIVM-IR生成に関する再利用可能な知見
                      (次にAMIVM上で言語を実装する人向け)
CLAUDE.md              プロジェクト規約、およびこのコンパイラを作る過程で下した設計判断の全記録
```

## ライセンス

MIT — [`LICENSE`](LICENSE)を参照してください。
