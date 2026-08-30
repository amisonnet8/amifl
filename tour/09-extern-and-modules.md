# 9. 外部Go資産とモジュール

[← 目次](README.md) | [前へ: 8. 並列処理とファイルI/O](08-concurrency-and-io.md) | [次へ: 10. まとめ →](10-wrapup.md)

## `extern`——Go資産の利用(`amifl-spec.md` §15)

AmiFLはGoの上に実装されているため、標準ライブラリを含む任意のGoパッケージをホワイトリスト方式で取り込めます。

```amifl
extern "strings" as strs {
    bind Contains2(s: String, substr: String) -> Bool as Contains
}

fn main() -> Int {
    let hasFoo = Contains2("foobar", "foo")
    if hasFoo { 0 } else { 1 }
}
```

`extern "パス" as 名前空間 { bind ... }`でGoパッケージを取り込みます。**`名前空間`(`as`の右辺)はAmiFLソース内では一切参照されません**——AmiFLに`.`を使ったメソッド呼び出し構文は存在せず(§15.1)、`bind`した関数は`Contains2(...)`のように裸の名前でグローバルスコープから呼び出します。`as Contains`(`bind`のGo側の実際の呼び出し対象)は、AmiFL側の名前(`Contains2`)とGo側の名前(`Contains`)が異なる場合に使います。

### メソッドのバインド

Goのレシーバー付きメソッドは、レシーバーを普通の第1引数として受け取る関数として`bind`します。

```amifl
extern "time" as time {
    type Time
    bind Now() -> Time
    bind TimeUnix(t: Time) -> Int as Time.Unix
    bind TimeFormat(t: Time, layout: String) -> String as Time.Format
}

fn main() -> Int {
    let now = Now()
    let formatted = now |> TimeFormat(_, "2006")
    print(formatted)
    len(formatted)
}
```

`as Time.Unix`のような`Type.Method`形式が、Goのメソッド呼び出しをAmiFL側の普通の関数呼び出しへ変換します。`type Time`は、AmiFLからは中身の見えない不透明な外部型を1つ登録します。

### `Any`型境界

`bind`の引数・戻り値に`Any`が現れる場合があります——これは`extern`境界でのみ登場する、実行時まで具体型が確定しない真に動的な型です(2章・6章で見た`print`の`Any`とは別の使われ方です、§2.2)。

```amifl
extern "encoding/json" as json2 {
    bind Marshal2(v: Any) -> Tuple2[Bytes, Error] as Marshal
}

fn main() -> Int {
    let marshaled = Marshal2("hi")   // AnyへString値を渡す
    let bytesLen = len(marshaled.0)
    print(bytesLen)
    0
}
```

`typeName(v: Any) -> String`で、`extern`由来の値を含むあらゆる値の動的な型名を覗くことができます(§13.2)。

### 同じディレクトリの手書き`.go`ファイルを束ねる(§15.3)

単一の関数・メソッド呼び出しに収まらない複雑なGoロジックが必要な場合、専用構文は増やさず、`.aml`ファイルと同じ場所(またはその配下の任意のディレクトリ)に普通の`.go`ファイルを置き、それを`extern`で束ねます。`extern "パス"`の`パス`が`.`・`./...`・`../...`で始まると(`import alias "./x"`と同じ相対パスの慣習)、Go標準ライブラリではなく**ローカルの手書きGoファイル**として解釈されます。

```go
// mathhelpers.go(main.amlと同じディレクトリに置く)
package mathhelpers

func GCD(a, b int64) int64 {
    for b != 0 {
        a, b = b, a%b
    }
    if a < 0 {
        return -a
    }
    return a
}
```

```amifl
extern "." as mh {
    bind Gcd(a: Int, b: Int) -> Int as GCD
}

fn main() -> Int {
    print(Gcd(48, 18))   // 6
    0
}
```

そのディレクトリ直下にある`.go`ファイルが自動的にコンパイル対象になります(サブディレクトリは見ません、`amifl build`が別途`go.mod`を用意する必要もありません)。ファイル自身が宣言する`package`名はAmiFLからは一切参照されません——常に`extern`の`as`名(ここでは`mh`)で参照するためです。

## モジュールと複数ファイルの統合(§12)

同じディレクトリに置かれた`.aml`ファイル群は、`import`不要で1つの共有スコープとしてコンパイルされます(§12.1)。別のディレクトリのパッケージを使いたい場合だけ`import`します。

```amifl
// mathutil/mathutil.aml
const MaxClamp: Int = 100

fn Clamp(v: Int, lo: Int, hi: Int) -> Int {
    if v < lo { lo } elif v > hi { hi } else { v }
}

fn double(x: Int) -> Int { x * 2 }   // 小文字始まり -> 他パッケージから見えない
```

```amifl
// main.aml
import mathutil "./mathutil"

fn main() -> Int {
    let a = mathutil.Clamp(15, 0, 10)
    let b = mathutil.MaxClamp
    print(a + b)
    0
}
```

**他パッケージから参照できるのは、名前が大文字で始まるトップレベル宣言だけです**(Goの可視性規則、§12.2)。`fn`/`const`/`struct`/`enum`のいずれも同じ規則に従い、`struct`・`enum`のクロスパッケージ構築(`mathutil.Interval{...}`のような)も同じ`エイリアス.名前`構文で書けます。

```amifl
// mathutil.aml側
struct Interval { lo: Int, hi: Int }
```

```amifl
// main.aml側
let iv: mathutil.Interval = mathutil.Interval{lo: 0, hi: 10}
```

**既知の限界**: クロスパッケージのenum値を`switch`で直接パターンマッチすることはできません(§12.2)。あるenumの値を検査したい場合は、そのenum自身の宣言パッケージ側に検査用の関数(内部で`switch`する)を用意し、それを呼び出す形にします。

パッケージ間の依存は循環できません(§12.5)。

## 演習

1. Go標準ライブラリの`math`パッケージから`Sqrt(x float64) float64`を`extern`で取り込み(`bind Sqrt(x: Float) -> Float`)、`16.0`の平方根を`print`するプログラムを書いてください。
2. `Rectangle`という`struct{width: Int, height: Int}`と、その面積を返す`fn area(r: Rectangle) -> Int`を持つ`shapes`という別パッケージを作り、ルート側からその`struct`を構築して`area`を呼ぶプログラムを書いてください(ディレクトリ構成は`main.aml`と`shapes/shapes.aml`)。
3. `main.aml`と同じディレクトリに`double.go`(`package doubler` `func Double(x int64) int64 { return x * 2 }`)を置き、`extern "." as d { bind Double(x: Int) -> Int }`で束ねて、`Double(21)`を`print`するプログラムを書いてください。

<details>
<summary>解答例</summary>

```amifl
extern "math" as mathpkg {
    bind Sqrt(x: Float) -> Float
}

fn main() -> Int {
    print(Sqrt(16.0))   // 4
    0
}
```

```amifl
// shapes/shapes.aml
struct Rectangle { width: Int, height: Int }

fn area(r: Rectangle) -> Int {
    r.width * r.height
}
```

```amifl
// main.aml
import shapes "./shapes"

fn main() -> Int {
    let r = shapes.Rectangle{width: 3, height: 4}
    print(shapes.area(r))   // 12
    0
}
```

`area`は小文字始まりなので、実際にはこの構成では他パッケージから見えません——`fn Area(r: Rectangle) -> Int`のように大文字始まりへ直す必要があります。実際に試して、意図的にこのエラーを確認してみるのもよい練習になります。

```go
// double.go(main.amlと同じディレクトリ)
package doubler

func Double(x int64) int64 { return x * 2 }
```

```amifl
extern "." as d {
    bind Double(x: Int) -> Int
}

fn main() -> Int {
    print(Double(21))   // 42
    0
}
```

</details>

[← 目次](README.md) | [前へ: 8. 並列処理とファイルI/O](08-concurrency-and-io.md) | [次へ: 10. まとめ →](10-wrapup.md)
