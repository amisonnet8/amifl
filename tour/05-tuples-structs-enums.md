# 5. タプル・構造体・enum

[← 目次](README.md) | [前へ: 4. 配列・リスト・範囲](04-collections-and-for.md) | [次へ: 6. パイプ演算子とエラー処理 →](06-pipe-and-errors.md)

## タプル(`amifl-spec.md` §2.2)

`Tuple2[A,B]`〜`Tuple8[...]`は、固定長・異種混合の値をまとめる型です。

```amifl
fn main() -> Int {
    let point = (3, 4)        // Tuple2[Int, Int]
    let mixed = (1, "a", true)   // Tuple3[Int, String, Bool]

    print(point.0)   // 3
    print(point.1)   // 4

    0
}
```

`.0`・`.1`のようなフィールドアクセスは、後述する`struct`のフィールドアクセスと全く同じ内部構文です。1要素だけのタプルは`(x,)`と末尾カンマを付けて書きます(`(x)`は単なる括弧付き式になってしまうため)。タプルは`==`/`!=`比較ができ(全要素が比較可能な限り)、フィールドの書き換えはできません(常に新しいタプルを作ります)。

## `struct`(§2.2)

```amifl
struct Point { x: Float, y: Float }

fn main() -> Int {
    let p = Point{x: 1.0, y: 2.0}
    print(p.x)   // 1

    let moved = Point{x: p.x + 1.0, y: p.y}   // 書き換えではなく作り直す
    print(moved.x)   // 2

    0
}
```

**AmiFLに`struct`のフィールド代入構文(`p.x = 5`)はありません**(原則4)。書き換えたい場合は、上の例のように新しい値を明示的に作ります——これは「1つの値は一度作られたら変わらない」という設計を、`struct`にも一貫して適用した結果です。全フィールドが比較可能な型(スカラー・別の`struct`・タプル)なら、`struct`どうしも`==`/`!=`比較ができます。

## `enum`(タグ付きバリアント、§2.2)

複数の「形の異なる場合分け」を1つの型として表現します。

```amifl
enum Status {
    Ok
    Retry(delay: Int)
    Failed(reason: String)
}

fn main() -> Int {
    let s1 = Status.Ok
    let s2 = Status.Retry(delay: 5)
    let s3 = Status.Failed(reason: "timeout")

    0
}
```

値の生成は`型名.バリアント名(...)`という通常の式です(フィールドが無いバリアントは括弧無し)。**`enum`の値へアクセスする唯一の正当な手段は`switch`によるパターンマッチです**——`enum`どうしの`==`比較はできません(バリアントが違えば無関係なフィールドを比較してしまうことになるため、意図的に禁止されています)。

## `switch`(enum専用形、§7.2、§10)

```amifl
enum Status {
    Ok
    Retry(delay: Int)
    Failed(reason: String)
}

fn describe(s: Status) -> String {
    switch s {
        case Status.Ok: "success"
        case Status.Retry(delay): "retry after"
        case Status.Failed(reason): "failed: " + reason
    }
}

fn main() -> Int {
    print(describe(Status.Ok))
    print(describe(Status.Retry(delay: 3)))
    print(describe(Status.Failed(reason: "timeout")))
    0
}
```

`case Status.Retry(delay):`と書くと、`Retry`バリアントが持つフィールド`delay`がcase本体のスコープに`Int`値として束縛されます——**パターン中の識別子は、必ずそのバリアントが宣言しているフィールド名そのものでなければなりません**(別名を使いたい場合は、case本体の中で改めて`let`します)。

**全バリアントを1回ずつ網羅していれば`default`は省略できます。** コンパイル時にバリアントの集合が閉じていることが分かっているため、漏れがあれば型エラーとして検出されます。

```amifl
fn code(s: Status) -> Int {
    switch s {
        case Status.Ok: 0
        case Status.Retry(delay): delay
        // Status.Failedのcaseを書き忘れるとコンパイルエラーになる
    }
}
```

## 演習

1. `struct Line { from: Point, to: Point }`(`Point`は上で定義したもの)を宣言し、`Line`の2点間の`x`座標の差の絶対値を返す関数`dx`を書いてください(`abs`という組み込み関数は6章で扱うので、`if`で自分で絶対値を計算してかまいません)。
2. `enum Shape { Circle(radius: Float)  Rectangle(width: Float, height: Float) }`を宣言し、`switch`で面積を計算する関数`area`を書いてください(円の面積は`3.14159 * radius * radius`、長方形は`width * height`)。

<details>
<summary>解答例</summary>

```amifl
struct Point { x: Float, y: Float }
struct Line { from: Point, to: Point }

fn dx(l: Line) -> Float {
    let diff = l.to.x - l.from.x
    if diff < 0.0 { 0.0 - diff } else { diff }
}

fn main() -> Int {
    let l = Line{from: Point{x: 1.0, y: 0.0}, to: Point{x: 4.0, y: 0.0}}
    print(dx(l))   // 3
    0
}
```

```amifl
enum Shape {
    Circle(radius: Float)
    Rectangle(width: Float, height: Float)
}

fn area(s: Shape) -> Float {
    switch s {
        case Shape.Circle(radius): 3.14159 * radius * radius
        case Shape.Rectangle(width, height): width * height
    }
}

fn main() -> Int {
    print(area(Shape.Circle(radius: 2.0)))
    print(area(Shape.Rectangle(width: 3.0, height: 4.0)))
    0
}
```

</details>

[← 目次](README.md) | [前へ: 4. 配列・リスト・範囲](04-collections-and-for.md) | [次へ: 6. パイプ演算子とエラー処理 →](06-pipe-and-errors.md)
