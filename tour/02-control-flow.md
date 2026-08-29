# 2. 制御構文

[← 目次](README.md) | [前へ: 1. スカラー型と変数](01-scalars-and-variables.md) | [次へ: 3. 関数とクロージャー →](03-functions-and-closures.md)

AmiFLの制御構文はすべて**式**です(原則1)。`if`も`switch`も`while`も、値を返します——他の多くの言語のように「文」として実行されるだけの構文ではありません。

## `if`/`elif`/`else`(`amifl-spec.md` §7.1)

```amifl
fn main() -> Int {
    let score = 82
    let grade = if score >= 90 {
        "A"
    } elif score >= 70 {
        "B"
    } else {
        "C"
    }
    print(grade)
    0
}
```

`if`は値を返す式なので、`let grade = if ... { ... } else { ... }`のように直接束縛できます。`else`を省略した場合、その`if`式全体は`Unit`型に限定されます(値を返す用途には使えません)。**`else`を書く場合、全ての分岐は同じ型を返さなければなりません。**

**`elif`/`else`は直前の`}`と同じ行に書く必要があります。** 改行を挟むと構文エラーになります——AmiFLはブロック内の改行を式の区切りとして扱う(§5)ため、この制約が曖昧さを防ぎます。

```amifl
// これは構文エラー(elifが新しい行から始まっている)
if a {
    1
}
elif b {
    2
}
```

## `switch`(Bool専用形、§7.2)

`switch`には2つの構文形がありますが、`enum`(5章で扱います)が無い段階では、**サブジェクト無しのBool専用形**だけを使えます。これは意味的に`if`/`elif`/`else`の連鎖と完全に同一です——AmiFLは「1つの仕組みで足りるものを2つ用意しない」(原則3)ため、専用の`match`構文を新設せず、`switch`をこの形の糖衣構文として実装しています。

```amifl
fn describe(n: Int) -> String {
    switch {
        case n < 0: "negative"
        case n == 0: "zero"
        default: "positive"
    }
}
```

`default`は`if`の`else`と同じ規則に従います——`switch`式が値を返す用途で使われている場合、`default`は省略できません。

## `while`(§7.4)

```amifl
fn main() -> Int {
    let sum = 0
    let i = 0
    while i < 5 {
        sum = sum + i
        i = i + 1
    }
    print(sum)   // 0+1+2+3+4 = 10
    0
}
```

`while`は常に`Unit`型を返します。`break`/`continue`は最も内側のループにのみ作用し、クロージャーの境界を越えられません(3章で詳しく扱います)。

## `&&`/`||`は短絡評価ではない、という注意点(§6)

1章でも触れましたが、制御構文の条件式を書くときにこれが特に効いてきます。

```amifl
// 危険な例: jがxsの範囲外のとき、xs[j]の評価自体は無条件に行われる
// (xsがList[Int]で、jがxsの長さと同じ場合、xs[j]は範囲外アクセスになる)
if j < len(xs) && xs[j] == target { ... }
```

このような場面は、短絡評価に頼らずネストした`if`で書き直します。

```amifl
if j < len(xs) {
    if xs[j] == target { ... }
}
```

## 演習

1. `Int`型の引数`n`を受け取り、`n`が3の倍数かつ5の倍数なら`"FizzBuzz"`、3の倍数だけなら`"Fizz"`、5の倍数だけなら`"Buzz"`、それ以外なら`n`自身を文字列化した値を返す関数`fizzbuzz`を、`switch`(Bool専用形)を使って書いてください(文字列への変換は一旦保留し、`n`を返す代わりに`"other"`を返す形で構いません)。
2. `while`を使って、`1`から`10`までの整数の合計を計算し`print`するプログラムを書いてください。

<details>
<summary>解答例</summary>

```amifl
fn fizzbuzz(n: Int) -> String {
    switch {
        case n % 15 == 0: "FizzBuzz"
        case n % 3 == 0: "Fizz"
        case n % 5 == 0: "Buzz"
        default: "other"
    }
}

fn main() -> Int {
    print(fizzbuzz(15))
    print(fizzbuzz(9))
    print(fizzbuzz(10))
    print(fizzbuzz(7))
    0
}
```

```amifl
fn main() -> Int {
    let total = 0
    let i = 1
    while i <= 10 {
        total = total + i
        i = i + 1
    }
    print(total)   // 55
    0
}
```

数値を文字列へ変換して返す完全な`fizzbuzz`は、`format`という組み込み関数を使うと書けます——6章で扱います。

</details>

[← 目次](README.md) | [前へ: 1. スカラー型と変数](01-scalars-and-variables.md) | [次へ: 3. 関数とクロージャー →](03-functions-and-closures.md)
