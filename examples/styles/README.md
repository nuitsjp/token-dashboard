# 外部スタイルのサンプル

3.5インチ用の独立した取り込み元フォルダーです。組み込みには含めません。
各フォルダーをStylesのAdd styleで指定し、取り込み後にDisplayのStyleで選択します。
寸法、表示対象、巡回は、[取り込みの仕様](../../docs/usecases/Styleを追加する/scenarios/Styleを追加する.md)に従います。

| フォルダー | 描画寸法 | データ構成 |
| --- | --- | --- |
| `sample-midnight-35-landscape` | 480×320 | `service-gauges` |
| `sample-paper-35-portrait` | 320×480 | `service-bars` |
| `sample-terminal-35-landscape` | 480×320 | `service-bars` |
| `sample-orbit-35-portrait` | 320×480 | `service-gauges` |

## 定義の形式

```json
{
  "formatVersion": 1,
  "id": "my-style",
  "name": "My style",
  "width": 480,
  "height": 320,
  "dataLayout": "service-gauges",
  "template": "template.hbs",
  "stylesheet": "style.css"
}
```

`id`は既存の定義と重複しないディレクトリー名にします。
3.5インチ用の`dataLayout`は`service-gauges`または`service-bars`を明示します。
9.2インチの1920×462では`gauges`または`bars`を使い、省略時は`gauges`です。
テンプレートとCSSはフォルダー直下に置きます。画像はこのフォルダー内の相対パスで参照します。
Paperは`assets/badge.svg`をHTMLの`img src`とCSSの`url()`から参照します。
共通サービスアイコンは`/assets/agents/`から取得し、未同梱のサービスには頭文字を表示します。

## テンプレートに渡るデータ

サービス表示では`service`に現在ページが入り、取得前や対象なしでは`serviceMessage`が入ります。
`service`にはサービス名`provider`、契約名`plan`、`icon`、`initial`、
`showTokens`、`showLimits`、`both`、ページ番号`number`と総数`count`があります。
Tokensの`tokens[]`には`label`、`tokensText`、`costText`、`tokensFontSize`があります。
Gaugesの`circles[]`は最大3件で、それぞれの`windows[]`に最大2枠が入ります。
Barsの`windows[]`は最大3件です。枠は`label`、`percentText`、`resetText`、
`hasRemaining`、`remainingPercent`、`tone`などの共通表示データを持ちます。
値が不明なときは渡された`—`をそのまま表示し、`hasRemaining`が偽なら残量図形を描きません。
ページ分割、表示対象、巡回はアプリが決めます。テーマは受け取ったページを描きます。
JavaScript、別フォルダーのファイル、外部URLを必要とするスタイルは定義しません。
取り込み元を後から変更しても内部コピーは変わらないため、変更を反映するには別スタイルへ切り替え、削除してから再取り込みします。
