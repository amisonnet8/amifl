# 3. 関数とクロージャー

[← 目次](README.md) | [前へ: 2. 制御構文](02-control-flow.md) | [次へ: 4. 配列・リスト・範囲 →](04-collections-and-for.md)

## トップレベル関数(`amifl-spec.md` §8.1、§8.5)

```amifl
fn add(a: Int, b: Int) -> Int { a + b }
```

- `fn`はトップレベルにしか書けません(§8.5)——関数の中に別の`fn`を入れ子にすることはできません。ローカルに関数が欲しい場合はクロージャー(後述)を使います。
- 本体の最後の式が戻り値です。途中で早期に抜けたい場合は`return`が使えます(次項)。
- **多引数がそのまま多引数です**——カリー化はしません(原則4)。パイプ演算子(6章)と組み合わせれば部分適用相当のことができるため、カリー化を言語機能として持つ必要が無いという判断です。
- **可変長引数・名前付き引数はありません**(原則7)。複数の値をまとめて渡したい場合は`struct`(5章)にまとめます。
- 関数引数は**再代入できません**(1章の`let`との違い)。書き換えたい場合は`let x2 = x`で明示的にコピーします。

## `return`——早期脱出(§5)

`fn`本体は最後の式が戻り値になりますが、途中で早期に抜けたいこともあります。そんなときは`return`/`return expr`が使えます。

```amifl
fn earlyIfNegative(x: Int) -> Int {
    if x < 0 {
        return 0
    }
    x * 2
}
```

`return`は`let`/`const`と同じ「文の位置」でのみ書けます——呼び出し引数や二項演算子の右辺のような、より大きな式の内側には埋め込めません(`f(return 1)`は構文エラーです)。ただしブロックの末尾(if/switchの分岐の末尾を含む)としてなら自由に書けるので、他の分岐と型の違う値を混ぜることもできます:

```amifl
fn describeSign(x: Int) -> String {
    if x < 0 { return "negative" } else { "non-negative" }
}
```

`return 5`と`"non-negative"`は一見すると型が違いますが、`return`はどこに現れても制御がそこで発散する(関数の外へ抜ける)ため、周囲がどんな型を期待していても矛盾しません——AmiFLはこれをコンパイラ内部限定の`Never`型として扱います(§2.2)。`break`/`continue`(2章)も同じ扱いなので、ループの中でif分岐の末尾として使えます(`if done { break } else { 5 }`)。`return`はクロージャーの中に書くと、そのクロージャー自身から早期脱出します(囲む`fn`からではありません)。

## 再帰(§8.6)

トップレベルの`fn`は自分自身を再帰呼び出しできます。相互再帰も可能で、宣言順序に依存しません。

```amifl
fn factorial(n: Int) -> Int {
    if n <= 1 {
        1
    } else {
        n * factorial(n - 1)
    }
}

fn isEven(n: Int) -> Bool {
    if n == 0 { true } else { isOdd(n - 1) }
}

fn isOdd(n: Int) -> Bool {
    if n == 0 { false } else { isEven(n - 1) }
}
```

`isEven`は(まだ宣言されていない)`isOdd`を呼び出していますが、コンパイルは通ります——AmiFLはまず全ての`fn`のシグネチャを集めてから本体を検査するため、宣言の前後関係を気にする必要がありません。

## クロージャー(§8.1、§8.3)

`let`の初期化式には、その場で書いたクロージャーリテラルを直接束縛できます。

```amifl
fn main() -> Int {
    let square = fn(x: Int) -> Int { x * x }
    let sq = square(5)   // 25

    let base = 50
    let addBase = fn(x: Int) -> Int { x + base }   // baseを捕捉する
    let ab = addBase(3)   // 53

    sq + ab
}
```

`addBase`は外側の`base`を**レキシカルスコープに従って捕捉**します——特別な構文は要りません。**クロージャーリテラルは`let`の直接の値としてのみ書けます**(呼び出しの引数へインラインで渡す、といった用途は使えません——ただし6章で扱う`|>`の右辺は例外です)。

## `Func`型と高階関数(§8.3、§8.8)

クロージャーの型は`fn(T1, T2, ...) -> R`という記法で書けます。この型注釈は`let`・`fn`の引数/戻り値・`struct`のフィールドなど、通常の型注釈が書ける場所ならどこでも使えます。

```amifl
fn applyTwo(f: fn(Int, Int) -> Int, a: Int, b: Int) -> Int {
    f(a, b)
}

fn addNums(a: Int, b: Int) -> Int { a + b }
fn mulNums(a: Int, b: Int) -> Int { a * b }

fn main() -> Int {
    let sum = applyTwo(addNums, 3, 4)    // 7 (トップレベルfnを値として渡せる)
    let prod = applyTwo(mulNums, 3, 4)   // 12
    sum + prod
}
```

`applyTwo`のような、関数を引数として受け取り呼び出す関数(高階関数、HOF)が書けます。渡す側は**トップレベル`fn`をその名前でそのまま値として渡す**(`addNums`)ことも、`let`束縛したクロージャーを渡すこともできます——どちらも同じ`Func`型として扱われます。

クロージャーを戻り値にすることもできます。

```amifl
fn makeAdder(n: Int) -> fn(Int) -> Int {
    let f = fn(x: Int) -> Int { x + n }
    f
}

fn main() -> Int {
    let addThree = makeAdder(3)
    addThree(5)   // 8
}
```

`makeAdder(3)`が返すクロージャーは、呼び出された時点の`n`(=3)を捕捉したまま持ち歩きます。

## 関数値の比較はできない

`Func`型どうしの`==`比較はサポートされていません(§8.3)。「同じ関数かどうか」を判定する手段はAmiFLにはありません。

## 演習

1. 引数`n: Int`を取り、`n`以下の全ての正の整数の合計を返す関数`sumUpTo`を、`while`を使って書いてください。
2. `fn(x: Int) -> Int`という型のクロージャーを引数に取り、それを`10`に適用した結果を返す関数`applyTo10`を書いてください。`let double = fn(x: Int) -> Int { x * 2 }`を定義し、`applyTo10(double)`を呼んで結果を`print`してください。

<details>
<summary>解答例</summary>

```amifl
fn sumUpTo(n: Int) -> Int {
    let total = 0
    let i = 1
    while i <= n {
        total = total + i
        i = i + 1
    }
    total
}

fn main() -> Int {
    print(sumUpTo(5))   // 15
    0
}
```

```amifl
fn applyTo10(f: fn(Int) -> Int) -> Int {
    f(10)
}

fn main() -> Int {
    let double = fn(x: Int) -> Int { x * 2 }
    print(applyTo10(double))   // 20
    0
}
```

</details>

[← 目次](README.md) | [前へ: 2. 制御構文](02-control-flow.md) | [次へ: 4. 配列・リスト・範囲 →](04-collections-and-for.md)
