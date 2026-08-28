# AMIVM上で言語を実装するときのヒント(AmiFLの実装から)

> AmiFL(`github.com/amisonnet8/amifl`)の実装過程で得られた知見を、次にAMIVM上で別言語を実装するAI向けに申し送るためのメモ。
> AmiFL自体の言語仕様の話ではなく、「AMIVM-IRを生成するフロントエンドを書くときに踏む地雷・使える型」の話に絞る(Seed/Cascade/Weave各リポジトリの同名ファイルと同じ位置づけ)。
>
> 着手前に必ず`CLAUDE.md`の「AmiFL特有の設計課題」「過去に踏まれた地雷」節、および以下3つの先行実装の知見を読むこと——AmiFLはSeed/Cascadeと同じ静的型付けであり、特にこの2つは直接参考になる箇所が多い。
>
> - `ignored/seed_implementation_notes.md`(goto/VAR巻き上げ問題・スコープのフラット化・`CALL`の多義性・参照渡し/値渡しの基本方針)
> - `ignored/cascade_implementation_notes.md`(ポインタ・構造体・クロージャー・map・チャネル・ビット演算の実地検証結果。AMIVMの旧64命令をほぼ全て実証した実績があり、AmiFLが必要とする機能構成に最も近い)
> - `ignored/weave_implementation_notes.md`(動的型付け言語がAMIVMとどう相性が悪い/良いか。AmiFLは静的型なので大半は直接は関係しないが、`extern`機構の実装時は`gotype`/`gofunc`/`gomethod`の変遷が参考になる)

## 0. AmiFLが実証した命令

Step 1（ブートストラップ）時点で、以下の命令だけで足りた。**実際に`amivm`→`go build`→実行まで通して動作確認済み。**

```
FUNC RET ENDFUNC
VAR
CALL
```

`main`のブリッジ（下記§1）で使う`CALL`のキャスト機能（`CALL %exitCode : ?int %code`、Seed/Cascade/Weave notes共通の「`CALL`はキャストにも使われる」パターン）も実証済み。

## 1. 踏んだ地雷・確立したパターン

### `main`ブリッジの`os.Exit`境界で、AmiFLの`Int`（`^int64`）とGoの`int`が食い違う

`fn main() -> Int`の戻り値をamivmの`!main`（Goの`func main()`。引数・戻り値ともに無し）へ橋渡しするため、Seed/Cascade/Weaveと同じ「ユーザーの`main`を`!amifl_main`という別名でコンパイルし、`!main`は`amifl_main`を呼んで`os.Exit()`に渡す薄いラッパー」という設計にした（詳細は`CLAUDE.md`「確定した設計判断」参照）。

素朴に「`amifl_main`の戻り値`%code`（`^int64`）をそのまま`os.Exit`に渡す」IRを書くと、`amivm`のIRパース自体は通るが、生成されたGoコードの`go/types`型チェックで`cannot use ... (variable of type int64) as int value in argument to os.Exit`という形で初めて発覚する。**GoのAPI（`os.Exit`はプラットフォーム依存の`int`を取る）と対象言語の固定幅整数型（AmiFLの`Int`＝`Int64`）が食い違う境界は、IRの構文チェックだけでは検出できず、必ず`go build`まで通して確認する必要がある**——という、Seed/Cascade/Weaveのnotesが繰り返し強調する「実地検証必須」の教訓を、`main`ブリッジという最も基本的な部分でさっそく再確認した形になった。

対策は`CALL`のキャスト機能（Goの型変換`T(v)`と`ast.CallExpr`が構文的に同一であることを利用）を1回挟むだけ：

```
VAR	%code	^int64
VAR	%exitCode	^int
CALL	%code	:	!amifl_main
CALL	%exitCode	:	?int	%code
CALL	:	?os.Exit	%exitCode
```

**一般化した教訓**: AmiFLの固定幅数値型（`Int8`〜`UInt64`等）をGoの標準ライブラリ関数（`os.Exit`に限らず、`int`を要求する他のAPI全般）へ渡す境界では、常にこの「AmiFL型→Go APIが要求する型」への明示キャストが必要になる可能性を意識すること。今後`extern`機構（15節）でGo資産をバインドする際、引数・戻り値の型変換ロジックとしてこのパターンを一般化できる見込み。

### ブロック内の式区切りは改行トークン（Weave方式を採用、仕様の未決事項を解消）

`amifl-spec.md`は式の区切り方法を明記していなかった（原則1「プログラムは式の並びのみ」とあるだけ）。Weaveの`weave_implementation_notes.md`が示す「レキサーが実際の`\n`をそのまま`Newline`トークンとして生成し、パーサーがブロック内でだけそれを区切りとして消費する」という設計（Goのセミコロン自動挿入のような中間層を持たない、最も単純な形）をそのまま採用した。実装・実地検証とも問題なし。詳細は`CLAUDE.md`「確定した設計判断」参照。

## 2. 開発プロセスの教訓

`CLAUDE.md`の「開発の進め方」節を参照。「機能単位の縦切り+各ステップでamivm→go buildの実地検証必須」という3言語共通の最重要教訓は、AmiFLでも変わらず適用する。
