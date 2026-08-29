# 7. Set と Map

[← 目次](README.md) | [前へ: 6. パイプ演算子とエラー処理](06-pipe-and-errors.md) | [次へ: 8. 並列処理とファイルI/O →](08-concurrency-and-io.md)

## `Set[T]`(`amifl-spec.md` §2.2、§13.5)

重複の無い集合です。要素は比較可能な型(数値・文字列・真偽値・タプル)に限られます——`struct`/`enum`は要素にできません。

```amifl
fn main() -> Int {
    let s: Set[Int] = {1, 2, 2, 3}   // 重複は自動的に除かれる -> {1, 2, 3}
    print(len(s))   // 3

    add(s, 4)        // 破壊的(直接書き換える)
    discard(s, 1)     // 破壊的

    for x in s {
        print(x)
    }
    0
}
```

**要素の順序は不定です**(Goのmapイテレーション順序をそのまま使うため)。決定的な順序が欲しい場合は`toList(s) |> sort`と組み合わせます。

```amifl
fn main() -> Int {
    let a: Set[Int] = {1, 2, 3}
    let b: Set[Int] = {2, 3, 4}

    let u = union(a, b)         // {1,2,3,4}
    let i = intersect(a, b)     // {2,3}
    let d = difference(a, b)    // {1}

    print(len(u))
    0
}
```

`union`/`intersect`/`difference`は**非破壊**(新しい`Set`を返す)ですが、`add`/`discard`は**破壊的**(対象を直接書き換える)です——実行時表現がGoのmap(参照型)であることを素直に反映しています。

## `Map[K,V]`(§2.2、§13.6)

```amifl
fn main() -> Int {
    let m: Map[String, Int] = {"a": 1, "b": 2}

    set(m, "c", 3)     // 破壊的
    let v = get(m, "a", 0)   // キーが無ければ0(第3引数)を返す

    for k, val in m {
        print(k)
    }

    print(v)
    0
}
```

`Map`の`for`は**2変数形**(`for k, v in m { ... }`)専用です——`List`/`Array`/`Set`の1変数形とは構文レベルで分けられています(値がキー・値のペアという2つの情報を持つため)。

```amifl
fn main() -> Int {
    let m: Map[String, Int] = {"x": 1, "y": 2, "z": 3}

    let ks = keys(m)         // List[String]
    let vs = values(m)       // List[Int]
    let es = entries(m)      // List[Tuple2[String,Int]]

    print(len(ks))
    print(len(vs))
    print(len(es))
    0
}
```

## ミューテーターの非対称性(§13.6)

`Set`/`Map`のミューテーター(`add`/`discard`/`set`/`delete`)は破壊的、一方`List`のミューテーター相当(`push`/`pop`/`insert`/`removeAt`、6章では触れませんでしたが13.4節にあります)は**非破壊**です。この非対称性は意図的なもので、`List`の`append`がバッキング配列を再利用しうる(別の変数が同じ配列を静かに共有してしまう危険)ため、`List`側だけ常にコピーしてから変更する設計になっています。

## 演習

1. `List[String]`型の変数`words`に`["a", "b", "a", "c", "b", "a"]`を束縛し、`Set[String]`を使って重複を除いた単語の個数を`print`するプログラムを書いてください。
2. `Map[String, Int]`を使って、`words`(演習1と同じリスト)の中で各単語が何回出現するかを数え、最終的に`for k, v in m`で全て`print`するプログラムを書いてください(ヒント: `get(m, k, 0)`で「無ければ0」を取得してから`set`で書き込みます)。

<details>
<summary>解答例</summary>

```amifl
fn main() -> Int {
    let words: List[String] = ["a", "b", "a", "c", "b", "a"]
    let seen: Set[String] = {}
    for w in words {
        add(seen, w)
    }
    print(len(seen))   // 3
    0
}
```

```amifl
fn main() -> Int {
    let words: List[String] = ["a", "b", "a", "c", "b", "a"]
    let counts: Map[String, Int] = {}
    for w in words {
        let current = get(counts, w, 0)
        set(counts, w, current + 1)
    }
    for k, v in counts {
        print(k)
        print(v)
    }
    0
}
```

</details>

[← 目次](README.md) | [前へ: 6. パイプ演算子とエラー処理](06-pipe-and-errors.md) | [次へ: 8. 並列処理とファイルI/O →](08-concurrency-and-io.md)
