# UCP-1. 受信した最新状態を表示画像にして出力する

適用条件と関与コンテナは [アーキテクチャの一覧](../architecture.md#patterns) を参照します。図は主成功系列を役割名で示し、UC ごとの逸脱は末尾に記録します。

| 役割 | 責務 | 実装パス |
| --- | --- | --- |
| Hub 受信 | 保存済みの接続設定で SSE に接続し、snapshot・stats を最新状態へ渡す。止まったら1秒から倍にしながら（上限60秒）再接続する。接続設定の保存時にも接続し直す | `internal/hub/receiver.go` |
| ローカル取得 | 取得元が `Local` のとき、同梱の tokscale を専用の設定ディレクトリで実行する。起動時に探索先ファイルの探索先（なければ `clients --json` の探索先。Cursor を除く）の監視を始め、`cursor sync --json` の後に `graph --no-spinner` の日別合計から大型版用の3期間を集計し、同じ結果の `clients[]` の `client`・`tokens`・`cost` から3.5インチ用のツール別3期間を集計する。ツールの ID をそのまま使い、モデル名や提供元名から帰属を推測しない。並行して `usage --json` で利用枠を取得する。`clients --json` を実行したときは、その後に最初に成功した graph の `summary.clients` と合わせて探索先ファイルに保存し、graph に既知でないツールが現れたときに取り直す。変更は最初の通知から2秒待ってまとめ、graph は開始間隔10秒以上・同時に1つまでとし、Cursor の同期とは同時に実行しない。利用枠は、提供元が取得を制限するため、利用記録の変更や Cursor の同期とは切り離し、前回の取得から2分ごとに取得する（これより頻繁には取得しない）。取得結果から抜けた提供元は、前回の値を保つ。Cursor の同期は起動時の結果に応じて45秒ごと・停止・待ち時間を倍にする再試行（上限10分）とする。日付が変わると集計し直す。集計と利用枠がそろってから最新状態へ渡し、内容が同じなら渡さない。失敗した値は前回のまま保つ。取得元の変更と終了時に監視と子プロセスを止める | `internal/localusage/reader.go`、`internal/localusage/tokscale.go`、`internal/localusage/scan.go`、`internal/localusage/platform_windows.go` |
| 最新状態 | 受信した利用状況をメモリにだけ保持し、置き換えを描画へ通知する | `internal/usage/usage.go` |
| 描画 | 最新状態の置き換え・表示設定の変更・1分ごとの時刻で、機種に合う表示画像を游ゴシックで描く（Go 標準の `image` と `golang.org/x/image`）。9.2インチの配置は既存の各シナリオに従い、3.5インチではサービス別の Tokens と利用枠を選択したページを構成して巡回期限にも描き直す。数値の書式は9.2インチと共用し、Tokens の変更だけでも現在のページと巡回期限を維持して描き直す。表示スタイルは `Gauges`（既定）または `Bars` とし、提供元のアイコンと枠の色・ラベル・残り時間の規則を共用する。ページ構成・スキップ・設定変更時の動作は [3.5インチのシナリオ](../usecases/利用状況をUSBディスプレイに表示する/scenarios/3.5インチでサービスごとの利用枠を順番に表示する.md) に従う | `internal/display/render.go`、`internal/display/gauges.go`、`internal/display/style.go`、`internal/display/compact.go`、`internal/display/content.go`、`internal/display/compact_render.go`、`internal/display/service.go`、`internal/display/icons/` |
| TURZX 送信 | 表示画像に対応する表示先・機種・向きに従って変換し、逐次送る。機種別の画像変換と通信方式は [アーキテクチャ](../architecture.md) に従う。送信中に届いた画像は最新の1枚だけ残し、表示先変更時には別機種用の画像を送らない。再接続時に最新の画像を送る。アプリの終了時に機種に応じたリセットを指示する | `internal/display/output.go`、`internal/turzx/protocol.go`、`internal/turzx/serial.go`、`internal/turzx/serial_windows.go`、`internal/turzx/conn_windows.go` |
| 利用枠の選択 | `Display` 画面の `Usage Limits` で枠の表示・非表示を選ぶ。契約単位の一括Toggleは9.2インチで表示し、3.5インチではサービス全体のON/OFFを `Service content` で選ぶ。表示しない枠の識別子を設定ファイルの `hiddenLimits` に保存し、保存のたびに描画へ再生成を求める。描画は、最新状態から表示しない枠を取り除いたものに、契約の選び方・並び順の規則を適用する。保存に失敗したときは、選択も表示画像も変えずにエラーを返す。保存済みの選択を読めないときは、すべての枠を描く | `internal/display/limits.go`、`internal/display/service.go`、`internal/settings/service.go`、`frontend/src/usecases/show-usage/UsageLimitSelect.tsx`、`frontend/src/features/display/queries.ts` |
| サービス別の表示選択 | 利用枠と期間別のツール内訳に報告された ID から一覧を作り、3.5インチ用の初期状態で閉じた `Service content` を開き、サービスカードのToggleでサービス全体のON/OFFを選び、3択で両方・利用枠のみ・Tokensのみを選ぶ。OFF中も最後の3択を保持し、ONで再開する。個別枠の選択は保持し、表示内容の変更で巡回を先頭へ戻す。Localの3.5インチでは、対象シナリオの対応表で利用枠名とTokensのツールIDを対応付けて一覧・ページ・設定を共用し、Tokensは対応する取得IDから読む。Hubと未知のIDは完全一致を維持し、未報告を全体集計で補わない。保存の責務は UCP-2 に従う | `internal/display/content.go`、`internal/display/identity.go`、`frontend/src/usecases/show-usage/ServiceContentSelect.tsx`、`frontend/src/features/display/queries.ts` |
| プレビュー配信 | 本体の Wails サービス。最新の表示画像を画面へ返し、画像の更新をイベントで通知する | `internal/display/service.go` |
| プレビュー区画 | ウィンドウで最新の表示画像を表示する。画像を描かず、状態も持たない | `frontend/src/usecases/show-usage/UsagePreview.tsx`、`frontend/src/features/display/queries.ts` |

```mermaid
sequenceDiagram
  participant Hub as Hub
  participant R as Hub 受信
  participant St as 最新状態
  participant D as 描画
  participant T as TURZX 送信
  participant P as プレビュー配信
  participant UI as プレビュー区画
  Hub-->>R: snapshot / stats
  R->>St: 最新状態を置き換える
  St->>D: 置き換えを通知
  D->>D: 表示画像を生成
  D->>T: 表示画像
  T->>T: 機種に応じた変換・送信
  D->>P: 表示画像
  P-->>UI: 更新を通知
  UI->>P: 最新の表示画像を取得
```

- 整合性: 状態更新の主体は最新状態 / 結果確定点はメモリ上の最新状態の置き換え / 障害時は、受信の失敗では最後の最新状態と表示を保ったまま再接続を続け、TURZX の送信失敗では画像を捨てて再接続後に最新の画像を送る。どちらも他方とアプリを止めない / 境界は、描画と送信を最新の1枚だけで追いつくようにすること。
- モックに置き換える境界と合成点: 本番の実行経路は Hub 受信とローカル取得の実処理だけを使い、最新状態へ仕様合意用の固定データを入れる分岐は置かない。Hub 経路の検証時は、接続設定の URL を制御可能な SSE サーバーへ向け、受信・描画・プレビュー配信は本番の処理を使う。LocalのID対応付けの検証では、既存のlocalusage検証用実行ファイルにtokscaleの各コマンドの固定応答を返させ、ローカル取得・集計・設定保存・描画を本番処理で実行する。実アプリの取得データと個別枠の識別子は書き換えない。起動・終了と検証の手順は [プロジェクト定義](../project.md#execution) を参照する。

UC 固有の逸脱: なし
