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

Step 2時点で、以下の命令だけで足りた。**実際に`amivm`→`go build`→実行まで通して動作確認済み。**

```
FUNC RET ENDFUNC
VAR SET
CALL
```

`main`のブリッジ（下記§1）で使う`CALL`のキャスト機能（`CALL %exitCode : ?int %code`、Seed/Cascade/Weave notes共通の「`CALL`はキャストにも使われる」パターン）も実証済み。`SET`はStep 2の`let`/代入で単純に「既存の`VAR`宣言済み変数へ値を書き込む」用途のみ使用（`%name value`という最も素直な形。複合代入や配列添字等の複雑な左辺は未検証）。

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

### amivmの未使用変数自己修復が、AmiFLの`_ = expr`設計をそのまま裏付けた（Step 2で実地確認）

`ignored/amivm/CLAUDE.md`に「amivmは生成コード中の未使用変数を自己修復する仕組み（`_ = 変数名`の自動挿入）を内部に持っている」という記述があったが、実際に`let inferred = 42`（以降未使用）を含むAmiFLプログラムを`emit-go`で確認したところ、生成されたGoコードには`var ...inferred int64`の直後に`_ = ...inferred`が自動的に挿入されており、`go build`が問題なく通ることを確認した。

**一般化した教訓**: 対象言語のcodegenが「このローカル変数は後で使われるだろうか」を心配する必要は無い——amivmが最終防衛線として引き受けてくれる。この事実は`_ = expr`（AmiFLの明示的な値破棄構文）の設計判断にも直接影響した：`_ = expr`はGoの未使用変数エラーを回避するための構文ではなく、**AmiFL自身の「ブロック内の最後以外の式はUnit型必須」という言語規律を満たすためだけの構文**だと判明した。そのためcodegen側は「副作用の無い値（リテラル・変数読み取り）を`_ = ...`で捨てる」ケースでは一切IRを生成しない——Goの未使用変数対策としての`_ = x`をAmiFL側で二重に生成する必要はなく、amivmに完全に委ねてよい。この整理をCascadeの同種の結論（cascade_implementation_notes.mdではなくCascade自身のCLAUDE.md）と照合し、3言語目（Cascade）・4言語目（AmiFL）で独立に同じ結論へ達したことを確認できた。

### 数値リテラルは「無型」として文脈の期待型へ適応させる設計にした（先行実装に無い、AmiFL独自の設計判断）

Seed/Cascadeはどちらも静的型付けだが、リテラルが「無型」かどうかという設計判断そのものへの直接の言及は無かった（両言語とも、リテラルの型は構文上ほぼ自明だったため、この設計判断を明示的に迫られなかったと見られる）。AmiFLは`let`/`const`の型注釈が省略可能で、かつ数値型のサイズ・符号違いが暗黙変換不可（原則2）という組み合わせのため、**リテラル自体は「まだ型が確定していない値」とし、使われる文脈（`let`の型注釈、無ければデフォルト`Int64`/`Float64`）に応じて初めて具体的なAMIVM型が決まる**という設計にした（Goの無型定数の考え方に近い）。

**この設計はコード生成側に嬉しい副産物をもたらした**: リテラルトークンをAMIVM-IRへ出力する際、常に生の数字列（`42`や`3.14`）をそのまま埋め込むだけで良く、対象の変数がどの具体的な数値型であっても、Go自身の無型定数→具体型への暗黙変換規則がそのまま働く（`var x int8; x = 200`のような範囲外だけがGo自身の`go/types`でも弾かれる——ただし「わかりやすいAmiFL自身のエラーで先に弾く」という意味検証の責任分担の原則により、AmiFL側でも同じ範囲チェックを`internal/sema/expr.go`の`resolveIntLit`で先に行っている）。

**唯一の罠**: 浮動小数点リテラルをAMIVM-IRへ出力する際、Goの`strconv.FormatFloat`が整数値ぴったりの浮動小数点数（例: `5.0`）を小数点無しの`"5"`という文字列にしてしまう。これ自体はGoのビルド結果としては無害（無型定数`5`は`float64`変数へ問題なく代入できるため）だが、IRやGoソースを目視で読んだときに「本当にfloatとして生成されているのか」が分かりにくくなる。対策として`internal/codegen/codegen.go`の`formatFloatLit`が、小数点を含まない場合は`.0`を付与するようにした（Weaveの`weave_implementation_notes.md`にある「整数形の値には強制的に`.0`を付与し、Goの無型浮動小数点定数のデフォルト型`float64`を選ばせる」という教訓と同じ発想——ただしWeaveの動機は`any`型変数がint型のデフォルトを選んでしまう問題の回避であり、AmiFLの動機は可読性のみという違いがある）。

## 2. 開発プロセスの教訓

`CLAUDE.md`の「開発の進め方」節を参照。「機能単位の縦切り+各ステップでamivm→go buildの実地検証必須」という3言語共通の最重要教訓は、AmiFLでも変わらず適用する。
