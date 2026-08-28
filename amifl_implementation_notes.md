# AMIVM上で言語を実装するときのヒント(AmiFLの実装から)

> AmiFL(`github.com/amisonnet8/amifl`)の実装過程で得られた知見を、次にAMIVM上で別言語を実装するAI向けに申し送るためのメモ。
> AmiFL自体の言語仕様の話ではなく、「AMIVM-IRを生成するフロントエンドを書くときに踏む地雷・使える型」の話に絞る(Seed/Cascade/Weave各リポジトリの同名ファイルと同じ位置づけ)。
>
> **現時点では実装にまだ着手していないため、本ファイルは雛形のみ。** 実装が進み次第、このプロジェクト固有の発見をここへ追記していく。着手前に必ず`CLAUDE.md`の「AmiFL特有の設計課題」「過去に踏まれた地雷」節、および以下3つの先行実装の知見を読むこと——AmiFLはSeed/Cascadeと同じ静的型付けであり、特にこの2つは直接参考になる箇所が多い。
>
> - `ignored/seed_implementation_notes.md`(goto/VAR巻き上げ問題・スコープのフラット化・`CALL`の多義性・参照渡し/値渡しの基本方針)
> - `ignored/cascade_implementation_notes.md`(ポインタ・構造体・クロージャー・map・チャネル・ビット演算の実地検証結果。AMIVMの旧64命令をほぼ全て実証した実績があり、AmiFLが必要とする機能構成に最も近い)
> - `ignored/weave_implementation_notes.md`(動的型付け言語がAMIVMとどう相性が悪い/良いか。AmiFLは静的型なので大半は直接は関係しないが、`extern`機構の実装時は`gotype`/`gofunc`/`gomethod`の変遷が参考になる)

## 0. AmiFLが実証した命令

(未実装。実装が進んだら、Seed/Cascade/Weaveの各notesの§0に倣い、実際に`amivm`→`go build`まで通して動作確認済みの命令を一覧化する。)

## 1. 踏んだ地雷・確立したパターン

(未実装。ステップを進めるたびに、実地検証で発覚したバグ・確立した設計判断をここに追記する。)

## 2. 開発プロセスの教訓

`CLAUDE.md`の「開発の進め方」節を参照。「機能単位の縦切り+各ステップでamivm→go buildの実地検証必須」という3言語共通の最重要教訓は、AmiFLでも変わらず適用する。
