# AmiFLツアー

AmiFL言語の入門ガイドです。手を動かしながら、短い章を順番に読み進めることを想定しています。

**唯一の正確な仕様は[`../amifl-spec.md`](../amifl-spec.md)です。** このツアーと仕様書が食い違う場合は仕様書を優先してください。各章には対応する仕様書の節番号(`§13.4`のような形)を添えているので、より詳しく知りたくなったらそちらを参照してください。すぐ動かして確かめたいコード例は[`../examples/`](../examples/)にもまとまっています。

## 章立て

0. [ようこそ](00-welcome.md) — AmiFLとは何か、7つの設計原則、Hello, AmiFL!
1. [スカラー型と変数](01-scalars-and-variables.md) — 静的型、`let`/`const`、リテラル、演算子
2. [制御構文](02-control-flow.md) — `if`/`elif`/`else`、`switch`(Bool専用形)、`while`
3. [関数とクロージャー](03-functions-and-closures.md) — `fn`、`Func`型、高階関数
4. [配列・リスト・範囲](04-collections-and-for.md) — `Array[T;N]`、`List[T]`、`Range`、`for`
5. [タプル・構造体・enum](05-tuples-structs-enums.md) — `Tuple2`〜`Tuple8`、`struct`、`enum`、`switch`パターンマッチ
6. [パイプ演算子とエラー処理](06-pipe-and-errors.md) — `|>`、`?`演算子、`Tuple2[T, Error]`、組み込み関数の多相
7. [Set と Map](07-sets-and-maps.md) — `Set[T]`、`Map[K,V]`
8. [並列処理とファイルI/O](08-concurrency-and-io.md) — `Chan[T]`/`Stream[T]`、`spawn`、ファイル操作
9. [外部Go資産とモジュール](09-extern-and-modules.md) — `extern`、複数ファイル・複数パッケージ
10. [まとめ](10-wrapup.md) — 統合サンプルと、次に読むもの

## 実行環境

このツアーのコード例を実際に動かすには、`amifl`と`amivm`が必要です。セットアップは`CLAUDE.md`の「amivmのインストール・呼び出し方」節、または`amifl-spec.md`16節を参照してください。準備ができたら、コード例をファイルに保存して次のように実行できます。

```sh
amifl run hello.aml
```

## このツアーについて

各章は、前の章で学んだことを土台に少しずつ話を進めます。掲載しているコードは実際に`amifl run`(または`amifl build`)で動くことを確認済みのものなので、手元で実行しながら読むことをお勧めします。分からなくなったら、各章に添えてある`amifl-spec.md`の節番号(`§13.4`のような形)を参照してください——このツアーは仕様書の要約であり、仕様書自身がAmiFLの唯一の正確な情報源です。

準備ができたら、次の章から始めましょう。
