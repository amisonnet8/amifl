# 4. 配列・リスト・範囲

[← 目次](README.md) | [前へ: 3. 関数とクロージャー](03-functions-and-closures.md) | [次へ: 5. タプル・構造体・enum →](05-tuples-structs-enums.md)

## `List[T]`と`Array[T;N]`(`amifl-spec.md` §2.2)

AmiFLは可変長と固定長のコレクションを、明確に2つの別の型として分けています。

- **`List[T]`**——可変長。既定のコレクションリテラル`[1, 2, 3]`はこちらになります。
- **`Array[T;N]`**——固定長・コンパイル時サイズ。型注釈で明示したときだけこちらになります。

```amifl
fn main() -> Int {
    let xs: List[Int] = [1, 2, 3, 4]
    let arr: Array[Int; 3] = [10, 20, 30]

    print(len(xs))   // 4
    print(xs[0])       // 1
    print(arr[2])      // 30

    0
}
```

`List`はGoのスライス(参照型)、`Array`はGoのネイティブ固定長配列(値型)としてコンパイルされます。要素は改行を挟んで複数行に書け、末尾カンマも付けられます(1章参照)。

添字アクセス`x[i]`・代入`x[i] = v`・スライス`x[a:b]`/`x[a:]`/`x[:b]`/`x[:]`はどのコレクションにも使えます(§3.2)。

```amifl
fn main() -> Int {
    let xs: List[Int] = [1, 2, 3, 4, 5]
    let middle = xs[1:3]   // [2, 3]
    print(len(middle))
    0
}
```

## 多次元配列は入れ子の糖衣(§2.2)

`Array[T; N1, N2]`は`Array[Array[T;N2]; N1]`のネストの糖衣構文にすぎません——AmiFLは「1つの仕組みで足りるものを2つ用意しない」(原則3)ため、専用の多次元配列型は作らず、添字も`x[i][j]`の連鎖で表します(`x[i,j]`という別構文は導入しません)。

```amifl
fn main() -> Int {
    let grid: Array[Array[Int;3]; 3] = [[1,2,3],[4,5,6],[7,8,9]]
    print(grid[1][1])   // 5
    0
}
```

## `for x in items { ... }`(§7.3)

```amifl
fn main() -> Int {
    let xs: List[Int] = [1, 2, 3, 4]
    let total = 0
    for x in xs {
        total = total + x
    }
    print(total)   // 10
    0
}
```

`for`の束縛変数(`x`)は再代入できません(関数引数と同じ規則、原則7)。

## `for x in items yield expr`(§7.3、§9)

各要素を変換して新しい`List`を作りたい場合は`yield`形を使います。これは`items |> map(x => expr)`に相当する糖衣構文です(パイプ演算子は6章で扱います)。

```amifl
fn main() -> Int {
    let xs: List[Int] = [1, 2, 3, 4]
    let doubled = for x in xs yield x * 2
    for d in doubled {
        print(d)   // 2, 4, 6, 8
    }
    0
}
```

## `Range`——`a..b`と`a..=b`(§3.1、§7.3)

`0..10`(半開区間)・`0..=10`(閉区間)は、数値の範囲を表す`Range`値を作ります。`for`で消費するのが主な使い道です。

```amifl
fn main() -> Int {
    let total = 0
    for i in 0..5 {
        total = total + i   // 0+1+2+3+4 = 10
    }

    let squares = for i in 0..=3 yield i * i   // [0, 1, 4, 9]

    total + len(squares)
}
```

`Range`の`From`/`To`は常に`Int64`固定です(他の整数幅・`Float`は使えません)。`Range`型を`let`や関数引数の型注釈として書くことはできません——`let r = 0..10`のように、値として使うだけです。

## 演習

1. `List[Int]`型の変数`xs`に`[3, 1, 4, 1, 5, 9]`を束縛し、`for`ループでその最大値を求めて`print`するプログラムを書いてください。
2. `0..10`という`Range`を`for...yield`で消費し、各要素を2乗したうえで3の倍数のものだけ数える(3章までで学んだ`if`を`yield`式の中で使う)プログラムを書いてください。ヒント: `yield`式自体は1つの式なので、`if...else`をそのまま書けます。

<details>
<summary>解答例</summary>

```amifl
fn main() -> Int {
    let xs: List[Int] = [3, 1, 4, 1, 5, 9]
    let maxVal = xs[0]
    for x in xs {
        if x > maxVal {
            maxVal = x
        }
    }
    print(maxVal)   // 9
    0
}
```

```amifl
fn main() -> Int {
    let squares = for i in 0..10 yield i * i
    let count = 0
    for s in squares {
        if s % 3 == 0 {
            count = count + 1
        }
    }
    print(count)
    0
}
```

</details>

[← 目次](README.md) | [前へ: 3. 関数とクロージャー](03-functions-and-closures.md) | [次へ: 5. タプル・構造体・enum →](05-tuples-structs-enums.md)
