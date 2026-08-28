# AmiFL

**ステータス: 初期スケルトン段階 — コンパイラ本体はまだ実装されていません。** 実装計画は`CLAUDE.md`を参照してください。

AmiFLは、AWK/Perlの系譜に連なる汎用データ処理特化言語です。Perlが積み重ねてきた文法の乱雑さを意図的に避け、最小限の式指向制御構文・多相な組み込み関数・読みやすいデータ変換パイプラインを実現するパイプ演算子（`|>`）を軸に設計されています。静的型付けで、実行時の動的ディスパッチはありません。

AmiFLは[AMIVM](https://github.com/amisonnet8/amivm)上に実装する4つ目の言語で、[Seed](https://github.com/amisonnet8/seed) → [Cascade](https://github.com/amisonnet8/cascade) → [Weave](https://github.com/amisonnet8/weave)の系譜に続きます。

```
AmiFLソース (.aml)
  |  (このリポジトリの担当範囲)
  v
AMIVM-IR (.ir)
  |  (amivm。外部CLIツール)
  v
Goソース (.go)
  |  (go build。このリポジトリのビルドパイプラインが実行)
  v
実行ファイル
```

## 要件

- Go（最低バージョンは`go.mod`参照）
- [`amivm`](https://github.com/amisonnet8/amivm)が`PATH`にインストール済みであること

  ```sh
  go install github.com/amisonnet8/amivm/cmd/amivm@latest
  ```

## インストール

```sh
go install github.com/amisonnet8/amifl/cmd/amifl@latest
```

## 使い方

```sh
amifl <command> [flags] <file.aml | package-dir | package.amlz>
```

| コマンド | 説明 |
|---|---|
| `build` | ネイティブ実行ファイルへコンパイルする |
| `run` | コンパイルして即座に実行する |
| `emit-ir` | AMIVM-IRへコンパイルする |
| `emit-go` | Goソースへコンパイルする（amivm経由） |
| `archive` | ディレクトリの`.aml`ファイル群を`.amlz`アーカイブへまとめる |
| `help` | ヘルプメッセージを表示する |

いずれのコマンドもまだ実装されていません——実装計画は`CLAUDE.md`を参照してください。

## サンプル

```amifl
fn main(args: List[String]) -> Int {
    print("Hello, AmiFL!")
    0
}
```

## 言語仕様

完全な言語仕様は[`amifl-spec.md`](./amifl-spec.md)にあります。

## リポジトリ構成

`amifl-spec.md` 16.1節、および`CLAUDE.md`の「リポジトリ構成」節を参照してください。

## ライセンス

MIT — [`LICENSE`](./LICENSE)を参照してください。
