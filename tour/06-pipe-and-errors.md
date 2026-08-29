# 6. パイプ演算子とエラー処理

[← 目次](README.md) | [前へ: 5. タプル・構造体・enum](05-tuples-structs-enums.md) | [次へ: 7. Set と Map →](07-sets-and-maps.md)

ここまでで学んだ基礎の上に、AmiFLらしさが最も濃く出る2つの機能——**データフロー志向のパイプ演算子**と、**統一されたエラー処理**——を見ていきます。AmiFLがAWK/Perlの系譜を名乗りながら「Perl的な乱雑さを避ける」(§0)と言っているのは、主にこの2つの設計に表れています。

## パイプ演算子`|>`(`amifl-spec.md` §9)

`a |> f`は`f(a)`と同じ意味です。`_`を明示すればその位置へ、省略時は第1引数へ左辺値が注入されます。

```amifl
fn double(x: Int) -> Int { x * 2 }
fn addN(x: Int, n: Int) -> Int { x + n }

fn main() -> Int {
    let a = 5 |> double          // double(5) = 10
    let b = 5 |> addN(_, 3)      // addN(5, 3) = 8
    let c = 5 |> addN(3, _)      // addN(3, 5) = 8

    a + b + c
}
```

複数の`|>`をつなげると、処理の流れがそのまま左から右へ読める形になります——これがAWK/Perlの系譜としてのAmiFLの核心です。

```amifl
data |> trim |> upper |> print
```

右辺にその場限りのクロージャーを直接書くこともできます(左辺値がクロージャーの唯一の引数になります)。

```amifl
fn main() -> Int {
    5 |> fn(x: Int) -> Int { x * x }   // 25
}
```

## 組み込み関数は「データが第1引数」で統一されている(§9.2、§2.4)

13節にある組み込み関数(`len`・`map`・`filter`・`sort`・`contains`など、50個ほどあります)は、ほぼ全て第1引数にデータを取るよう統一されています——`_`を省略した`|>`が自然にデータを第1引数へ注入するため、パイプラインの中で`_`を一度も書かずに済みます。

```amifl
fn main() -> Int {
    let xs: List[Int] = [5, 3, 8, 1, 9]
    let isEven = fn(x: Int) -> Bool { x % 2 == 0 }

    let result = xs |> filter(_, isEven) |> sort |> len

    print(result)   // フィルタ後にソート、その長さを求める(結果は1——偶数は8だけ)
    0
}
```

長いパイプラインは、`|>`を次の行の先頭に置くことで複数行に分けて書けます(§5)——`|>`は他のどんな式の先頭にもなり得ないため、次の行が`|>`で始まっていれば必ず前の行からの継続として読まれます:

```amifl
let result = xs
    |> filter(_, isEven)
    |> sort
    |> len
```

これらの組み込み関数は、`List`・`Array`・`String`・`Map`・`Set`など複数の型に対応する**多相**関数ですが、AmiFLにユーザー向けのオーバーロード機構はありません(原則4)。多態はコンパイラ内部限定の「能力(Capability)」というタグで実現されており(§2.4)、`len(xs)`(Listの長さ)と`len(s)`(文字列の長さ)は、呼び出し箇所ごとにコンパイル時に別々の実装へ解決されます——実行時の動的ディスパッチは一切発生しません。

## `Error`型と`Tuple2[T, Error]`(§2.2、§13.2、§13.3)

AmiFLの関数は常に単一の値しか返せません(原則6)。「値、またはエラー」という結果は、専用の例外機構ではなく**`Tuple2[T, Error]`という統一された形**で表現します。

```amifl
fn main() -> Int {
    let result = parse[Int]("42")   // Tuple2[Int, Error]
    let value = result.0
    let err = result.1

    if isError(err) {
        print("parse failed")
    } else {
        print(value)   // 42
    }
    0
}
```

`parse[T](s: String) -> Tuple2[T, Error]`は文字列を数値へ変換する組み込み関数です。`isError(v: Error) -> Bool`はエラーが実際に発生したかどうかを判定します。

## `?`演算子——エラーの短絡(§3.3)

`Tuple2[T, Error]`を返す呼び出し式の直後に`?`を書くと、エラーが非nilだった場合その場で(自分を囲む関数から)早期脱出します。これがAmiFLに唯一存在する早期脱出の形です(`return`という汎用キーワードはありません)。

```amifl
fn parseSumTry(a: String, b: String) -> Tuple2[Int, Error] {
    let x = parse[Int](a)?     // aの変換に失敗したら、ここでエラー側を持って早期脱出する
    let py = parse[Int](b)
    (x + py.0, py.1)             // 成功時は、既存のError値(nil)をそのまま使い回す
}

fn main() -> Int {
    let r1 = parseSumTry("10", "20")
    print(isError(r1.1))   // false

    let r2 = parseSumTry("not a number", "5")
    print(isError(r2.1))   // true

    r1.0
}
```

`?`が使えるのは、自分を囲む関数の戻り値が`Tuple2[U, Error]`(または`Error`単体)の場合だけです——コンパイル時に検査されます。

## `unwrap`/`okOr`——プロトタイピング向けの近道(§13.9)

`Tuple2[T,Error]`を毎回`.0`/`.1`で分解するのは煩雑なので、簡易なヘルパーもあります。

```amifl
fn main() -> Int {
    let n = unwrap(parse[Int]("42"))       // エラーならその場でクラッシュする(プロトタイピング用)
    let m = okOr(parse[Int]("bad"), 0)     // エラーならdefault値(0)を使う
    print(n + m)
    0
}
```

`unwrap`はエラー時に実際にプロセスをクラッシュさせるため、本番コードでの多用は推奨されません——`okOr`か、`?`演算子での明示的な処理を使うのが基本です。

## `cast[T]`——数値型の明示変換(§13.3)

原則2(暗黙変換の排除)により、数値型どうしは自動変換されません。明示的に変換したい場合は`cast[T](v)`を使います。

```amifl
fn main() -> Int {
    let f: Float = 3.9
    let i: Int64 = cast[Int64](f)   // 3(小数点以下切り捨て)
    i
}
```

## 演習

1. `List[String]`型の変数`words`に`["10", "abc", "30"]`を束縛し、`for...yield`と`parse[Int]`・`okOr`を組み合わせて、パースに失敗した要素は`0`として扱いつつ全て合計するプログラムを書いてください。
2. `|>`を使って、`List[Int]`の`[4, 1, 3, 9, 2]`を「偶数だけ残す→昇順にソートする→先頭の要素を取り出す(`at(_, 0)`)」という3段のパイプラインで処理し、結果を`print`してください。

<details>
<summary>解答例</summary>

```amifl
fn main() -> Int {
    let words: List[String] = ["10", "abc", "30"]
    let nums = for w in words yield okOr(parse[Int](w), 0)
    let total = 0
    for n in nums {
        total = total + n
    }
    print(total)   // 40
    0
}
```

```amifl
fn main() -> Int {
    let xs: List[Int] = [4, 1, 3, 9, 2]
    let isEven = fn(x: Int) -> Bool { x % 2 == 0 }

    let result = xs |> filter(_, isEven) |> sort |> at(_, 0)

    print(result)   // 2
    0
}
```

</details>

[← 目次](README.md) | [前へ: 5. タプル・構造体・enum](05-tuples-structs-enums.md) | [次へ: 7. Set と Map →](07-sets-and-maps.md)
