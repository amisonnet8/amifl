# 10. まとめ

[← 目次](README.md) | [前へ: 9. 外部Go資産とモジュール](09-extern-and-modules.md)

## 統合サンプルを読む

ここまでの章で見てきた要素——`enum`/`switch`パターンマッチ(5章)・`struct`(5章)・`Map`(7章)——を1つのプログラムに組み合わせてみましょう。気温の読み取り値を分類し、集計するプログラムです。

```amifl
enum Alert {
    Normal
    Warning(delta: Float)
    Critical(delta: Float)
}

struct Reading { label: String, celsius: Float }

fn classify(threshold: Float, r: Reading) -> Alert {
    let delta = r.celsius - threshold
    if delta > 10.0 {
        Alert.Critical(delta: delta)
    } elif delta > 0.0 {
        Alert.Warning(delta: delta)
    } else {
        Alert.Normal
    }
}

fn describe(a: Alert) -> String {
    switch a {
        case Alert.Normal: "normal"
        case Alert.Warning(delta): "warning"
        case Alert.Critical(delta): "critical"
    }
}

fn main() -> Int {
    let readings: List[Reading] = [Reading{label: "room1", celsius: 22.0}, Reading{label: "room2", celsius: 35.0}, Reading{label: "room3", celsius: 50.0}]

    let counts: Map[String, Int] = {}
    for r in readings {
        let a = classify(25.0, r)
        let key = describe(a)
        let current = get(counts, key, 0)
        set(counts, key, current + 1)
    }

    for k, v in counts {
        print(k)
        print(v)
    }

    len(readings)
}
```

`classify`が値を生成し(`enum`)、`describe`がパターンマッチで場合分けし(`switch`)、`main`が`Map`で集計する——それぞれ異なる章で学んだ要素が、迷い無く1つのデータパイプラインへ組み合わさっていることに注目してください。`Tuple2[T, Error]`と`?`演算子(6章)も、ファイル・ネットワーク・パース処理が絡む実用的なプログラムでは、ここに自然に合流します。

## `amifl`コマンドとビルド(`amifl-spec.md` §16)

```
amifl <command> [flags] <file.aml | package-dir | package.amlz>
```

| コマンド | 説明 |
|---|---|
| `build` | ネイティブ実行ファイルへコンパイルする |
| `run` | コンパイルして即座に実行する |
| `emit-ir` | AMIVM-IRへコンパイルする(内部構造を覗きたいとき) |
| `emit-go` | Goソースへコンパイルする |
| `archive` | パッケージを`.amlz`(zip)アーカイブへまとめて配布する |

複数ファイルからなるパッケージ(9章)は、ディレクトリをそのまま`amifl build`に渡すだけでコンパイルできます。`.amlz`アーカイブは、`import`のターゲットとしても、コンパイル対象のトップレベル引数としても、ディレクトリの完全な代役として扱われます(§16.2)。

## 「できないこと」を知る(§17)

AmiFLには、意図的に持たせていない機能・既知の限界がいくつもあります。仕様書17節はこれを明示的に一覧化したもので、「まだ実装されていないだけで、いずれ対応する予定」というリストではなく、**現時点の設計上、構文として書けない・意味論として存在しないもの**です。たとえば:

- `Set[T]`の要素順序は不定です(7章)——決定的な順序が必要なら`toList(_) |> sort`と組み合わせます。
- タプルのフィールド(`.0`等)への代入はできません(5章)——タプルは不変値として扱う設計です(`struct`のフィールドは`p.x = 5`で代入できます)。
- クロスパッケージのenum値を`switch`で直接パターンマッチすることはできません(9章)——宣言パッケージ側に検査用の関数を用意します。
- クロージャーリテラルを`let`の直接の値・パイプ右辺以外の位置(通常の呼び出し引数など)にインラインで書くことはできません(3章・6章)。

このリストを一度読んでおくと、「これは書けるはずなのに、なぜエラーになるのだろう」と仕様書を読み返す時間を減らせます。

## 次のステップ

- **[`../amifl-spec.md`](../amifl-spec.md)** — 唯一の正確な仕様書です。このツアーで触れられなかった細部(13節の組み込み関数完全一覧、16節のプロジェクト構成・CLIなど)も含め、AmiFLの全てがここに書かれています。
- **[`../examples/`](../examples/)** — 実行可能なサンプルプログラム集です。機能ごとの網羅型サンプル(`data_ops.aml`・`sets_maps.aml`など)に加えて、実際にAmiFLで何を書くかを示すタスク志向のサンプル(`fizzbuzz.aml`・`word_count.aml`・`inventory.aml`・`rpn_calculator.aml`・`run_length_encode.aml`など)も揃っています。特に`examples/modules/`(9章のパッケージ例の完全版)を見てみてください。

お疲れさまでした。あとは実際にコードを書きながら、`amifl-spec.md`を辞書代わりに参照するのが一番の近道です。AmiFLの7原則(0章)——特に「1つの仕組みで足りるものを2つ用意しない」と「明示性 > 簡潔さ」——を思い出すと、迷ったときの道しるべになります。

[← 目次](README.md) | [前へ: 9. 外部Go資産とモジュール](09-extern-and-modules.md)
