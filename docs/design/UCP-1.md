# UCP-1. 受信した最新状態を表示画像にして出力する

適用条件と関与コンテナは [アーキテクチャの一覧](../architecture.md#patterns) を参照します。図は主成功系列を役割名で示し、UC ごとの逸脱は末尾に記録します。

| 役割 | 責務 | 実装パス（段階4完了時に記入） |
| --- | --- | --- |
| Hub 受信 | 保存済みの接続設定で SSE に接続し、snapshot・stats を最新状態へ渡す。止まったら1秒から倍にしながら（上限60秒）再接続する。接続設定の保存時にも接続し直す | `internal/hub/receiver.go` |
| ローカル取得 | 取得元が `Local` のとき、同梱の tokscale を専用の設定ディレクトリで実行する。起動時に探索先ファイルの探索先（なければ `clients --json` の探索先。Cursor を除く）の監視を始め、`cursor sync --json` の後に `graph --no-spinner` で3期間を集計し、並行して `usage --json` で利用枠を取得する。`clients --json` を実行したときは、その後に最初に成功した graph の `summary.clients` と合わせて探索先ファイルに保存し、graph に既知でないツールが現れたときに取り直す。変更は最初の通知から2秒待ってまとめ、graph は開始間隔10秒以上・同時に1つまでとし、Cursor の同期とは同時に実行しない。利用枠は、提供元が取得を制限するため、利用記録の変更や Cursor の同期とは切り離し、前回の取得から2分ごとに取得する（これより頻繁には取得しない）。取得結果から抜けた提供元は、前回の値を保つ。Cursor の同期は起動時の結果に応じて45秒ごと・停止・待ち時間を倍にする再試行（上限10分）とする。日付が変わると集計し直す。集計と利用枠がそろってから最新状態へ渡し、内容が同じなら渡さない。失敗した値は前回のまま保つ。取得元の変更と終了時に監視と子プロセスを止める | `internal/localusage/reader.go`、`internal/localusage/tokscale.go`、`internal/localusage/scan.go`、`internal/localusage/platform_windows.go` |
| 最新状態 | 全体集計とサービス別内訳をメモリにだけ保持し、置き換えを描画へ通知する。Hubの期間ごとの内訳を受け取り、Localでは既存のgraph取得結果からサービス別の3期間も集計する | `internal/usage/usage.go`、`internal/localusage/tokscale.go` |
| 描画要求 | 最新状態の置き換え、表示設定の保存、1分ごとの時刻で、選択したスタイルの表示データを作り、1件ずつWebView2へ描画を求める。残量順、配置上限、グループ化、色（ペースと残量の悪い方）、対象ページは本体で決める。初回ロードと再ロードでも要求を取得できる。1件の応答期限は10秒とし、期限切れの要求を終了する。要求と結果の対応は[共通の画像境界](../architecture.md#design-constraints)に従う | `internal/display/theme_data.go`、`internal/display/service_pages.go`、`internal/display/canvas.go`、`internal/display/presentation_run.go` |
| テーマのビルド | [スタイルの契約](../architecture.md#style-contract)に従い、形式、寸法、表示データの構成、参照ファイル、識別子の重複を検査し、不正な場合はビルドを失敗させる。HTMLの`img src`とCSSの`url()`にあるテーマ内画像を参照元ファイルから解決して内包し、Handlebarsをコンパイルする。共通Agentアイコンは`/assets/agents/`から参照する。プレビューにも同じ素材解決を使う | `frontend/build/theme-build.ts`、`frontend/vite.config.ts`、`themes/preview/server.mjs` |
| 画像生成 | 常駐WebView2で、コンパイル済みHandlebarsとテーマのHTML/CSSをCanvasに描く。寸法と機器用画像の変換は[共通制約](../architecture.md#design-constraints)に従い、同じ描画からプレビュー用PNGと機器用画像を返す。配置はテーマファイルで定義し、共通Agentアイコンを参照する。初回の素材取得に失敗した場合は次の要求で取り直す。画像生成処理はReactと画面の表示・非表示から独立して起動し、JavaScriptのタイマーやrequestAnimationFrameには依存しない | `frontend/src/features/display/theme-renderer.ts`、`themes/`、`internal/display/icons/` |
| TURZX 送信 | [機器ごとの出力形式](../architecture.md#design-constraints)で1台へ逐次送る。送信待ちは最新の1枚だけを保持し、再接続時に最新の対象ページを送る。3.5インチの差分転送と消灯・点灯は[共通制約](../architecture.md#design-constraints)に従い、すべての矩形と点灯命令の送信成功後に比較用画像を更新する。送信失敗時は比較用画像を更新せず接続を閉じる。表示先、寸法、スタイルが変わった後に旧設定の画像を送らない。不適合な組み合わせでは送信を止め、取得とプレビューは続ける。アプリの終了時に再起動コマンドを送る | `internal/display/output.go`、`internal/turzx/protocol.go`、`internal/turzx/conn_windows.go`、`internal/turzx/serial.go`、`internal/turzx/serial_windows.go` |
| 利用枠の選択 | `Display` 画面の `Usage Limits` で、契約と枠の表示・非表示を選ぶ。表示しない枠の識別子を設定ファイルの `hiddenLimits` に保存し、保存のたびに描画へ再生成を求める。描画は、最新状態から表示しない枠を取り除いたものに、契約の選び方・並び順の規則を適用する。保存に失敗したときは、選択も表示画像も変えずにエラーを返す。保存済みの選択を読めないときは、すべての枠を描く | `internal/display/limits.go`、`internal/display/service.go`、`internal/settings/service.go`、`frontend/src/usecases/show-usage/UsageLimitSelect.tsx`、`frontend/src/features/display/queries.ts` |
| 利用枠のページ切り替え | 9.2インチ用の`Settings`画面の`Cycle`と`Cycle time`を既存の`cycle`・`cycleSeconds`へ保存し、本体の巡回処理へ反映する。大型標準スタイルでは、収まらない契約を順に後続ページへ詰め、Tokensの位置と更新時のページ番号を保つ。サービス表示では[対象シナリオ](../usecases/利用状況をUSBディスプレイに表示する/scenarios/3.5インチでサービスごとの利用枠を順番に表示する.md#ページ構成と巡回)に従い、ページの識別と巡回期限を本体が保持する。現在ページと巡回期限は保存しない。保存に失敗した場合は巡回状態も変更しない。設定の範囲と保存形式は[データ設計](data.md#設定ファイル)に従う | `internal/display/render.go`、`internal/display/gauges.go`、`internal/display/theme_data.go`、`internal/display/service.go`、`internal/settings/service.go`、`frontend/src/usecases/configure-hub/ConfigureHub.tsx`、`frontend/src/usecases/configure-hub/SettingsDraft.tsx`、`frontend/src/shared/SwitchButton.tsx` |
| 表示位置のずらし | TURZX へ送る画像だけを、3分ごとに横・縦 -4〜+4 px の範囲の8点（(0,0)→(4,0)→(4,4)→(0,4)→(-4,4)→(-4,0)→(-4,-4)→(0,-4)）で順に平行移動して描き直し、送る。両機種とも正立画像の座標でずらし、機器用画像の変換と向きの適用は[共通制約](../architecture.md#design-constraints)に従う。プレビュー用の PNG は動かさない。空いた部分は画像の左上隅の色で埋める。ずらし量は保存せず、起動時は (0,0) から始める。更新がなくても3分ごとに送り直す。データ更新、Cycle、Style、反転、表示先の変更でずらしの状態と期限はリセットしない。離席中は進めず、復帰時に(0,0)から3分間隔を取り直す | `internal/display/presentation_run.go`、`internal/display/service.go`、`internal/display/canvas.go`、`frontend/src/features/display/theme-renderer.ts` |
| 離席の検知と消灯 | 次のいずれかを離席条件とし、1つでも成立している間は離席として、TURZX へ黒一色の画像を送って消灯する: 無操作が `Away time`（1〜240分、既定10分）に達した（`GetLastInputInfo` を1秒ごとに確認）、Windows がモニターの電源を切った、PC がスリープ・休止に入った、このセッションがロック・サインアウト・ユーザー切替・コンソールからの切断（リモート接続を含む）で非アクティブになった（Windows の電源・セッションの通知を非表示のウィンドウで受ける）。すべて解けたら最新の利用状況から表示画像を作り直して送り、ずらしは (0,0) から再開する。消灯中は通常描画と巡回を止め、選択Styleの寸法と出力形式に合う黒画像を同じ送信経路で送る。3.5インチの復帰時は離席前のページを巡回間隔1回分表示し、対象消失時は既存のページ選択規則に従う。離席遷移では進行中のCanvas要求と送信待ち画像を無効にする。プレビューは変えない。`Away time` は設定ファイルの `awayMinutes` に保存し、保存のたびに反映する。保存に失敗したときは値も消灯の状態も変えずにエラーを返す。保存値を読めないときは10分として扱う。通知の登録に失敗しても表示は通常のまま続ける | `internal/presence/presence.go`、`internal/presence/watch_windows.go`、`internal/display/away.go`、`internal/display/presentation_run.go`、`internal/display/output.go`、`internal/settings/service.go`、`main.go`、`frontend/src/usecases/configure-hub/ConfigureHub.tsx`、`frontend/src/usecases/configure-hub/SettingsDraft.tsx` |
| プレビュー配信 | 本体の Wails サービス。最新の表示画像を画面へ返し、画像の更新をイベントで通知する | `internal/display/service.go` |
| プレビュー区画 | ウィンドウで最新の表示画像を表示する。画像を描かず、状態も持たない | `frontend/src/usecases/show-usage/UsagePreview.tsx`、`frontend/src/features/display/queries.ts` |
| スタイル一覧 | ビルドに含まれるテーマ定義と、アプリ内部に取り込んだ表示スタイルを列挙する。組み込みの Gauges と Bars は常に先に出し、各スタイルで描いたプレビューを見せる。TURZX へ送る画像と、現在の表示に使っているスタイルは変えない | `frontend/src/features/styles/catalog.ts`、`frontend/src/usecases/browse-styles/StyleList.tsx`、`frontend/src/routes/styles.tsx`、`frontend/src/app/Shell.tsx`、`frontend/src/features/display/theme-renderer.ts`、`internal/styles/service.go`、`internal/styles/catalog.go` |
| スタイルの取り込み | [取り込みシナリオ](../usecases/Styleを追加する/scenarios/Styleを追加する.md)の寸法・表示データ構成を含めて検査し、合格したファイルだけを内部へコピーする。確定済みの定義をメタデータとして保持し、設定取得で素材を読み直さない。本体の共通候補と画面の描画登録へ接続し、起動時にも復元する。取消、不合格、重複、コピー失敗では現在の設定・画像を変更しない | `internal/styles/service.go`、`internal/desktop/service.go`、`frontend/src/usecases/add-style/AddStyle.tsx`、`frontend/src/features/styles/catalog.ts`、`frontend/src/features/styles/prepare.ts` |
| スタイルの削除 | 追加したスタイルの内部コピーを、`styles` 下の一時名へ移してから削除し、定義から取り除く。組み込みと選択中のスタイルは削除できない。選択の保存と削除の判定・確定は同じ設定ロックで直列化する。取消と失敗では定義も内部のファイルも変えない。TURZXへ送る画像と、現在の表示に使っているスタイルは変えない | `internal/styles/service.go`、`internal/desktop/service.go`、`frontend/src/usecases/delete-style/DeleteStyle.tsx`、`frontend/src/features/styles/catalog.ts` |

```mermaid
sequenceDiagram
  participant Hub as Hub
  participant R as Hub 受信
  participant St as 最新状態
  participant D as 描画
  participant C as WebView2画像生成
  participant T as TURZX 送信
  participant P as プレビュー配信
  participant UI as プレビュー区画
  Hub-->>R: snapshot / stats
  R->>St: 最新状態を置き換える
  St->>D: 置き換えを通知
  D->>C: 要求識別子・テーマ・表示データ・寸法・出力形式・反転・設定の世代
  C->>C: HTML/CSS・Canvasで描画
  C->>D: 要求識別子・プレビュー用PNG・機器用画像
  D->>D: 要求との対応と設定の世代・形式・寸法を確認
  D->>T: 検証済みの機器用画像
  T->>T: 選択先の通信方式で送信
  D->>P: PNG
  P-->>UI: 更新を通知
  UI->>P: 最新の表示画像を取得
```

- 整合性: 状態更新の主体は最新状態と、本体が所有する巡回状態 / 結果確定点はメモリ上の最新状態の置き換えと、画像境界の検査に合格した結果の受領 / 障害時は、受信の失敗では最後の最新状態と表示を保ったまま再接続を続け、描画の失敗・応答期限切れでは前回の表示を保ち、次の状態変更または定期更新で再生成する。TURZXの送信失敗では失敗した画像を再送せず、再接続後に最新の画像を送る。いずれも他の処理とアプリを止めない / 境界は、描画を逐次にし、送信は最新の1枚だけで追いつくようにすること。
- モックの境界: 設定と利用状況の合成点は[UCP-2](UCP-2.md)に従い、通常起動は実処理を使う。外部スタイルは実際の内部コピーを本体の定義と描画登録へ接続し、起動時に復元する。操作確認用の固定応答は残さない。実機確認は[実行手順](../project.md#commands)に従う。サービス表示では巡回ONと専用間隔を共通の`display.Configuration`へ渡し、ページ構成・巡回期限・Canvas描画は共通の実装を使う。失敗による固定値への切り替えはない。Hub経路の既存検証は接続URLを制御可能なSSEサーバーへ向ける。

UC 固有の逸脱: 「Style一覧を閲覧する」は、その時点の表示スタイル定義を列挙し、各スタイルで描いたプレビューを一覧に出す。TURZX へ送る画像と、現在の表示に使っているスタイルは変えない。「Styleを追加する」は、指定したフォルダーを検査し、確定した内部コピーを本体と画面の候補に加える。適用はDisplayで手動選択し、設定保存はUCP-2に従う。
