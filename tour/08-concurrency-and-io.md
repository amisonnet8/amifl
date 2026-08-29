# 8. 並列処理とファイルI/O

[← 目次](README.md) | [前へ: 7. Set と Map](07-sets-and-maps.md) | [次へ: 9. 外部Go資産とモジュール →](09-extern-and-modules.md)

AmiFLは並列処理・ファイルI/Oのために専用の構文を増やしません(原則3)。`Chan[T]`/`Stream[T]`という**型**と、既存の`for`・組み込み関数の組み合わせだけで表現します(`amifl-spec.md` §11)。

## `Chan[T]`——チャネル(§2.2、§11)

```amifl
fn main() -> Int {
    let ch = chan[Int](0)   // バッファサイズ0のチャネル
    let worker = fn() -> Unit {
        send(ch, 1)
        send(ch, 2)
        send(ch, 3)
    }
    spawn(worker)   // goroutineとして起動

    let r1 = recv(ch)   // Tuple2[Int, Bool] (値、成功フラグ)
    let r2 = recv(ch)
    let r3 = recv(ch)

    print(r1.0 + r2.0 + r3.0)   // 6
    0
}
```

`chan[T](buffer)`・`send(ch, v)`・`recv(ch) -> Tuple2[T, Bool]`・`spawn(f)`が並列処理の全てです——`<-`のような専用記号は導入せず、いずれも普通の組み込み関数として提供されています。生の`Chan`は明示的に閉じる手段が無い点に注意してください——`recv`の成否フラグによる終了検出は、コンパイラが内部で管理する`Stream[T]`(次項)でのみ成立します。

## `Stream[T]`——遅延・並列処理向けの型(§2.2、§13.8)

`Stream[T]`は実行時表現こそ`Chan[T]`と同じ(Goのネイティブチャネル)ですが、AmiFLの型としては最後まで別物として扱われ、相互の暗黙変換はありません。`take`/`skip`/`collect`/`parallel`は`Stream[T]`専用です。

```amifl
fn main() -> Int {
    let ch = chan[Int](0)
    let worker = fn() -> Unit {
        send(ch, 1)
        send(ch, 2)
        send(ch, 3)
        send(ch, 4)
        send(ch, 5)
    }
    spawn(worker)

    0
}
```

`for x in stream { ... }`で`Stream`を1件ずつ消費できます(`Unit`版の`for`のみ、`yield`形は使えません——静的な長さを持たないためです)。

## `tap`/`peek`——パイプラインの開発体験(§9.1、§13.8)

パイプラインの途中経過を覗き見るための組み込み関数です。

```amifl
fn double(x: Int) -> Int { x * 2 }

fn main() -> Int {
    let result = 5 |> tap(_, "input") |> double |> tap(_, "doubled")
    result
}
```

`tap(v, label)`は恒等関数で、常に`stderr`へ`[label] value`という形式で1行出力してから、値をそのまま次へ流します。`peek(v)`は環境変数`AMIFL_DEV`が設定されているときだけ対話的に値を表示する、開発モード限定のインスペクタです。

## ファイルI/O(§13.10)

```amifl
fn main() -> Int {
    let path = "/tmp/amifl_tour_example.txt"
    let content: Bytes = [104, 101, 108, 108, 111, 10]   // "hello\n"のバイト列

    let wf = open(path, "w")
    let writeResult = write(wf.0, content)
    _ = close(wf.0)

    let rf = open(path, "r")
    let line = readLine(rf.0)
    print(line.0)   // "hello"
    _ = close(rf.0)

    writeResult.0
}
```

`open(path, mode) -> Tuple2[File, Error]`はファイルを開き、`write`/`readLine`/`readAll`はいずれも`Tuple2[..., Error]`を返します——6章で見た「値+エラー」パターンがここでも一貫しています。`File`は不透明なハンドル型で、フィールド・メソッドはありません。

**AmiFLには`String`から`Bytes`への変換関数がありません**(既知の限界)——ファイルへ文字列を書きたい場合、現状はこのように数値のバイト列として明示的に綴る必要があります。

`lines(f) -> Stream[String]`はファイルを1行ずつの`Stream[String]`として読みます。

```amifl
fn main() -> Int {
    let path = "/tmp/amifl_tour_lines.txt"
    let content: Bytes = [97, 10, 98, 10, 99, 10]   // "a\nb\nc\n"
    let wf = open(path, "w")
    _ = write(wf.0, content)
    _ = close(wf.0)

    let rf = open(path, "r")
    let s = lines(rf.0)
    let firstTwo = collect(take(s, 2))   // 先頭2行だけをListへ
    _ = close(rf.0)

    print(len(firstTwo))   // 2
    0
}
```

`stdin()`/`stdout()`/`stderr()`は標準入出力の`File`を返します。

## 演習

1. `chan[Int](0)`を作り、`spawn`した別のgoroutineから`0..5`の各値を`send`し、`main`側で5回`recv`して合計を`print`するプログラムを書いてください。
2. 上の「ファイルI/O」の例を参考に、`"1\n2\n3\n"`という内容のファイルを書き、`lines`と`collect`で全行を`List[String]`として読み戻し、`for...yield`と`parse[Int]`(前章)を組み合わせて数値の合計を`print`するプログラムを書いてください(パース失敗は考えなくてかまいません——`unwrap`を使うと簡単です)。

<details>
<summary>解答例</summary>

```amifl
fn main() -> Int {
    let ch = chan[Int](0)
    let worker = fn() -> Unit {
        for i in 0..5 {
            send(ch, i)
        }
    }
    spawn(worker)

    let total = 0
    let i = 0
    while i < 5 {
        let r = recv(ch)
        total = total + r.0
        i = i + 1
    }
    print(total)   // 0+1+2+3+4 = 10
    0
}
```

```amifl
fn main() -> Int {
    let path = "/tmp/amifl_tour_ex.txt"
    let content: Bytes = [49, 10, 50, 10, 51, 10]   // "1\n2\n3\n"
    let wf = open(path, "w")
    _ = write(wf.0, content)
    _ = close(wf.0)

    let rf = open(path, "r")
    let allLines = collect(lines(rf.0))
    _ = close(rf.0)

    let nums = for line in allLines yield unwrap(parse[Int](line))
    let total = 0
    for n in nums {
        total = total + n
    }
    print(total)   // 6
    0
}
```

</details>

[← 目次](README.md) | [前へ: 7. Set と Map](07-sets-and-maps.md) | [次へ: 9. 外部Go資産とモジュール →](09-extern-and-modules.md)
