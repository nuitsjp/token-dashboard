# Token Dashboard のプロジェクト定義

プロジェクト共通の要件・制約、ユースケース一覧、確認した事実、および実行・検証手順の正本です。全体構造は [アーキテクチャ](architecture.md) を参照します。

## 1. 目的と範囲

| 項目 | 内容 |
| --- | --- |
| 解決する問題・達成したい結果 | ローカルまたは Token Monitor Hub から取得した AI ツールのトークン利用量・推定コストと利用枠の残量を、ブラウザーやアプリの画面を開かずに、PC に接続した TURZX の USB ディスプレイで常時確認できるようにする |
| 利用者・利用場面 | 自分の Windows PC に TURZX 9.2インチを接続し、作業中に横目で利用状況を確認する |
| 今回の対象 | ローカルと1つの Hub から取得元を選ぶ設定（既定はローカル）と、Hub 選択時のリアルタイム受信。TURZX への表示（Tokens の Today・Month・All のトークン数と推定コスト、Usage Limits）。サインイン時の自動起動とタスクトレイ常駐。接続中の TURZX 9.2インチから表示先を1台選ぶ設定（選んでいない場合は最初の1台）。プレビューと取得元の設定を1画面にまとめたウィンドウ。インストーラーによる導入。GitHub Releases を使った新版の確認と自動更新 |
| 今回の対象外 | 複数 Hub の同時受信と合算。利用状況の保存と履歴。Hub への書き込み。端末別・モデル別などの内訳表示。TURZX 9.2インチ以外の機種と、複数台への同時表示。動画（H.264）の送信。WinUSB ドライバーの導入。Windows 以外の OS。インストーラーのコード署名（Authenticode） |

## 2. 制約・品質要求・受け入れ条件

- **動作環境**: Windows 11（x64）と WebView2 Runtime が必要です。表示先は TURZX 9.2インチ（USB `1CBE:0092`）で、WinUSB ドライバーが設定済みであることを前提にします。表示中はメーカーのソフトを終了しておきます。
- **受信**: Hub は1つだけです。`GET /api/stats/stream` に SSE で接続し、Bearer 認証と `x-token-monitor-stream: 2` を付けます。受信が止まったときは、待ち時間を1秒から倍にしながら（上限60秒）再接続を続けます。
- **ローカル取得**: アプリに同梱する tokscale 4.17.0 の Windows 用実行ファイルから、この端末の Today・Month・All のトークン数と推定コスト、利用枠を取得します。起動時に取得し、その後は利用記録の変更、2分ごとの利用枠の取得、Cursor の同期（起動時に同期できた場合だけ）、日付・月の切り替わりを契機に取得し直します。取得に失敗しても前回の表示値を保ち、選択した取得元を維持します。tokscale の設定・解析キャッシュはアプリ専用のディレクトリに置きます。利用者による tokscale の別途インストールは不要です。
- **保存しないもの**: 受信した利用状況はメモリにだけ持ち、ファイルや DB には保存しません。起動後に最初の snapshot を受信するまでは、利用状況の値を表示しません。
- **保存するもの**: Hub の接続先 URL と認証トークンは1組にまとめ、Windows の DPAPI でユーザー単位に暗号化して、ユーザーごとの設定領域に保存します。認証トークンはログにも画面表示にも出しません。
- **Usage Limits の表示**: Hub が報告した枠のうち、メーターを表示する指定（`showMeter`）が付いたものを、残量の割合が小さい枠を持つ契約から順に、表示領域に収まる件数だけ表示します。利用者が表示しないと選んだ枠（`Display` 画面の `Usage Limits`）は、報告されなかったものとして扱います。
- **障害の独立**: 受信の失敗と USB の切断・抜き差しは、互いにもう一方を止めません。アプリ自体も終了しません。USB ディスプレイがつながったら、その時点の最新表示を送ります。
- **表示の同一性**: USB ディスプレイとウィンドウのプレビューには同じ内容を表示します。
- **配布と更新**: インストーラーは NSIS で作り、Windows へのサインイン時の自動起動を登録します。公開する GitHub Releases に、インストーラーと、自作の Ed25519 鍵で署名した更新情報（`update.json`）を置きます。アプリは起動のたびに、起動処理とは別にバックグラウンドで新版を確認し、見つかった新版の取得と、更新情報の署名・インストーラーのハッシュの検証までを自動で行います。適用（インストールと再起動）は、利用者がタスクトレイまたはウィンドウから実行したときだけ行います。リリースはタグの push で CI（`.github/workflows/release.yml`）が作成します。署名用の秘密鍵はリポジトリの外で保管し、CI 用に GitHub の Secrets（`UPDATE_SIGNING_KEY`）に登録します。インストーラーにはコード署名をしないため、初回のインストールでは SmartScreen の警告が出ます。
- **完了の条件**: 受け入れ条件は各シナリオに書きます。`node scripts/run.mjs verify` の合格を完了の条件とします。USB 実機への表示は、自動テストとは別に実機で確認します。

<a id="usecases"></a>
## 3. ユースケース一覧

ユースケースの共通事項は `usecases/<名称>/README.md`、シナリオと固有の受け入れ条件は同じディレクトリの `scenarios/<名称>.md` に記載します。案の検討・保存は [提示と保存の手順](standards/mock-driven-development.md#discussion) に従います（未着手のユースケースは下表に名称だけを置き、本文とリンクは作りません）。

ユースケースの単位・系列の分割・モック適用は [モック標準のユースケース分割](standards/mock-driven-development.md#discussion) に従います。

| ユースケース | 主アクター | 目的 | 実装順序 | 実現パターン | モック適用 |
| --- | --- | --- | --- | --- | --- |
| [利用状況の取得元と表示先を設定する](usecases/利用状況の取得元と表示先を設定する/README.md) | 利用者 | 利用状況の取得元と、表示先の TURZX を設定する | 1 | [UCP-2](design/UCP-2.md) | 対象 |
| [利用状況をUSBディスプレイに表示する](usecases/利用状況をUSBディスプレイに表示する/README.md) | 利用者 | 選択した取得元の最新のトークン数・推定コストと利用枠を、TURZX とプレビューに常時表示する | 2 | [UCP-1](design/UCP-1.md) | 対象 |
| [新版を確認してアプリを更新する](usecases/新版を確認してアプリを更新する/README.md) | 利用者 | 起動時にバックグラウンドで GitHub Releases の新版を検出し、利用者の確認後にアプリを更新する | 3 | [UCP-3](design/UCP-3.md) | 対象 |
| [Style一覧を閲覧する](usecases/Style一覧を閲覧する/README.md) | 利用者 | アプリに定義された表示スタイルを、現在の表示を変えずに見比べる | 4 | [UCP-1](design/UCP-1.md) | 対象 |

常駐（タスクトレイ、ウィンドウの表示・非表示、終了）は独立したユースケースにせず、「利用状況をUSBディスプレイに表示する」の操作の一部として扱います。

<a id="design"></a>
## 4. 確認した事実

### Token Monitor Hub

- 仕様の情報源は [Javis603/token-monitor](https://github.com/Javis603/token-monitor) の `docs/API.md` です。`cdab5686d2711efea2c961f26d1e2e8c3c211b40`（v0.63.0 以降、2026-09-27 に確認）を対象とし、`GET /api/stats/stream` の節は `8b6cee22eabb7bf7ce0226655b46907300e14a04` から変わっていません。
- 認証は `Authorization: Bearer <secret>` ヘッダーで行います。
- `GET /api/stats/stream` は SSE です。接続ごとに最初に全体状態の `snapshot` を送り、30秒ごとに `: hb` のコメント行を送ります。利用量・利用枠・期間の区切りなどが変わると全体状態の `stats` を送ります。要求ヘッダー `x-token-monitor-stream: 2` を付けた場合、時刻だけの変化は `limits.updatedAt` と端末ごとの時刻・鮮度切れだけを含む `freshness` で送ります。通知本文は `type`・`reason`・`stats`・`at` を持つ JSON です。
- 全体状態の `stats.periods.today`・`month`・`allTime` は Hub 全体の集計値で、`totalTokens`（整数）と `costUsd`（数値）を持ちます。Hub は期間が終わった端末の `today`・`month` を自身の集計から除き、期間の区切りが変わると `stats` を送ります。
- 利用枠は `stats.limits.providers[]` にアカウントごとに並び、各要素が `provider`・`accountLabel`・`planLabel` と `windows[]` を持ちます。`windows[]` の各要素は `kind`（`session`・`daily`・`weekly`・`billing`）、`showMeter`、`remainingPercent`、`usedPercent`、`resetsAt`、`label` を持ちます。`showMeter` が偽の枠は残量の割合を表示に使いません。
- 2026-09-27 に利用者の Hub へ接続し、最初の `snapshot` のキー名と型だけを確認しました（値は記録していません）。`periods` の3期間すべてに `totalTokens` と `costUsd` があり、`limits.providers` は21件、そのうち `showMeter` が真の枠を持つのは6件で、枠は合計14件でした。

### tokscale

- 情報源は [junhoyeo/tokscale v4.17.0](https://github.com/junhoyeo/tokscale/tree/v4.17.0) です。Windows 用のネイティブ実行ファイルを同梱し、`clients --json`・`graph --no-spinner`・`usage --json`・`cursor sync --json` を呼び出します。
- 2026-09-29 にこの PC で、`graph --no-spinner` の日別の `tokenBreakdown`（`input`・`output`・`cacheRead`・`cacheWrite`）と `totals.cost` から求めた Today・Month・All が、`--today --json`・`--month --json`・`--json` の合計と一致することを確認しました。日別の `totals.tokens` は reasoning を含むため使いません。`cursor sync --json` は失敗時も終了コード0で、`synced` と `error`（認証情報がなければ `Not authenticated`）で結果を返します。
- 2026-09-27 にこの PC で v4.17.0 の JSON 出力を確認しました。期間集計は `totalInput`・`totalOutput`・`totalCacheRead`・`totalCacheWrite`・`totalCost` を持ち、利用枠はプロバイダーごとの `metrics` に `label`・`used_percent`・`remaining_percent`・`resets_at` を持ちました。

### TURZX 9.2インチ

- 情報源は [nuitsjp/Turzx.Net](https://github.com/nuitsjp/Turzx.Net) の `7f164ba89a229db900171bc02f608c47bdbeed5e` のソースと `docs/technical-notes.md`（2026-09-27 に実機で検証済み）です。
- USB `1CBE:0092` の WinUSB デバイスです。bulk IN/OUT の各1本で通信し、読み書きのタイムアウトは2秒です。
- 1回の送信は、504バイトのヘッダーを DES-CBC（鍵と IV は `slv3tuzx`、パディングなし）で暗号化し、末尾2バイトを `A1 1A` とした512バイトの制御部に、画像のバイト列を続けたものです。ヘッダーは、先頭にコマンド、2・3バイト目に `1A 6D`、4〜7バイト目にタイムスタンプ（ミリ秒、リトルエンディアン）、8〜11バイト目に画像の長さ（ビッグエンディアン）を置きます。コマンドは同期が10、JPEG が101、PNG が102です。応答の先頭がコマンドと `C8` で、2〜5バイト目が送ったタイムスタンプと一致すれば成功です。
- 画像は 1920×462 で描き、時計回りに90度回転した 462×1920 の Baseline JPEG または PNG を1MiB以下で送ります。JPEG 品質85で毎回描画・圧縮しても約58fps、CPU 負荷は PC 全体の約0.9% でした。静止画像は送信を止めても表示が残ります。
- 抜き差しは `CM_Register_Notification` で通知を受け、`CM_Get_Device_Interface_ListW` で再列挙して検出できます。対象の列挙には、Turzx.Net の開発 PC の WinUSB ドライバーが公開するインターフェース GUID `dee824ef-729b-4a0e-9c14-b7117d33a817` を使います。このインターフェース GUID はドライバーの導入時に決まる値で、ほかの PC で同じになるかは確認していません。
- 2026-09-27 に、[turing-smart-screen-python](https://github.com/mathoudebine/turing-smart-screen-python) の `library/lcd/lcd_comm_turing_usb.py` にある再起動コマンド（11）を送ったところ、約3秒後に機器がいったん列挙から消え、約1.5秒後に再び列挙されました。再起動後の画面の表示は目視で確認していません。
- 2026-09-27 にこの PC で、すべての USB デバイスが公開する標準のインターフェース GUID `a5dcbf10-6530-11d2-901f-00c04fb951ed`（`GUID_DEVINTERFACE_USB_DEVICE`）から `vid_1cbe&pid_0092` のパスを列挙し、WinUSB で開いて同期コマンドの正常応答を得ました。上記のドライバー固有の GUID で開いた場合も同じ結果でした。

### Wails とフォント

- `go.mod` の `github.com/wailsapp/wails/v3 v3.0.0-beta.23` は、Windows のタスクトレイ（`SystemTray` のアイコン、メニュー、クリック、ウィンドウの関連付け）と多重起動の防止（`SingleInstanceOptions`）を提供します（2026-09-27 にモジュールのソースで確認）。
- この PC の Windows 11 には、日本語と英数字の両方を含む游ゴシック（Windows のフォントフォルダーの `YuGothM.ttc`、`YuGothB.ttc`）があります（2026-09-27 に確認）。

テーマの描画方式と画像生成速度の比較は、[テーマ描画方式の調査結果](research/theme-rendering.md)を参照します。

<a id="commands"></a>
<a id="execution"></a>
## 5. 実行・切り替え・検証手順

作業ディレクトリはリポジトリのルートです。Go 1.25以上、`.nvmrc` と一致する Node.js、Python 3、WebView2 Runtime を用意します。インストーラーの作成には NSIS 3.11以上も必要です。

段階5の起動環境は [エージェント行動指針](../AGENTS.md) に従います。ゲージ配置の完成系監査では、インストール済みアプリを終了してから、保存済みの実 Hub 接続設定を監査用ディレクトリへ複製し、今回の変更を含むデスクトップアプリを起動します。

```powershell
$auditDir = Join-Path (Get-Location) '.test-data/gauge-real-app'
New-Item -ItemType Directory -Path $auditDir -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $env:APPDATA 'io.github.nuitsjp.token-monitor-turzx/settings.json') -Destination (Join-Path $auditDir 'settings.json')
$env:WAILS_DATA_DIR = $auditDir
$env:WAILS_WEBVIEW_DEBUG_PORT = '9347'
try {
    node scripts/run.mjs dev
} finally {
    Remove-Item Env:WAILS_DATA_DIR, Env:WAILS_WEBVIEW_DEBUG_PORT
}
```

トレイからウィンドウを開き、`Connection` で取得元が `Hub`、接続先が利用者の実 Hub であることを確認します。`Display` の `Gauges` で受信した利用枠の選択を切り替え、契約や円の数が変わっても円の位置が7列の格子に固定されていること、円が7つを超える契約が飛ばされて収まる契約が表示されること、Tokens の左端が先頭のパネルの左端にそろうことを確認します。自動確認は `playwright-cli attach --cdp=http://127.0.0.1:9347` でアプリの WebView2 に接続して行い、操作後は元の選択状態に戻します。アプリをトレイの `Exit` で終了し、開発起動のターミナルで Ctrl+C を押して監視プロセスも止めます。元の設定ファイルは変更しません。

mise を使う場合は、`mise install` で `mise.toml` の版のツールを導入し、下表の `node scripts/run.mjs <コマンド>` の代わりに `mise run <コマンド>` を実行できます（例: `mise run dev`、`mise run verify`）。`mise run release` に続けて書いた引数は、`scripts/run.mjs release` へそのまま渡します。

| 目的 | コマンド | 期待結果 |
| --- | --- | --- |
| 環境構築 | `node scripts/run.mjs setup` | Wails CLI を `.tools/` に導入し、Go と npm の依存（同梱用 tokscale を含む）、Go バインディング、ルートツリーを生成します |
| 開発起動 | `node scripts/run.mjs dev` | 開発用の Vite はポート 9345 を使います（Wails の既定の 9245 は、ほかの Wails プロジェクトと重なりやすいため変えています）。アプリがタスクトレイに常駐します。ウィンドウは最初は表示せず、トレイのアイコンのクリックか、トレイのメニューの `Open` で開きます |
| スタイル一覧のモック | `node scripts/run.mjs dev:mock` | `dev` と同じ起動で、Styles の定義一覧だけ `frontend/mocks/style-catalog.json` を使います。`Night` は Bars のテンプレートで描きます。ファイルから `Night` を除くと、そのカードが消え、`Gauges` と `Bars` は残ります。追加するとカードが戻ります。`dev` と本番ビルドはこのファイルを読まず、ビルドに含まれるテーマ定義を一覧にします。本番ビルドで `WAILS_FRONTEND_MODE=mock` を付けると、画面のビルドを止めます |
| 終了 | トレイのメニューの `Exit`、または起動したターミナルで Ctrl+C | `Exit` でアプリが終了します。`dev` では、変更を監視する `wails3 dev` と Vite が `Exit` の後も残るため、ターミナルで Ctrl+C を押して止めます。ウィンドウの閉じるボタンではウィンドウを隠すだけです |
| 全体検証 | `node scripts/run.mjs verify` | 生成、型検査、Lint、単体テスト、Go のテストと vet、文書検査、E2E がすべて合格します。生成のあと、画面の検査と検証用サーバーのビルド、Go のテスト、vet、文書検査を並列に実行し、出力の各行に `[check:go]` のようにタスク名を付けます |
| TURZX 実機の列挙 | `$env:TURZX_DEVICE_TEST='1'; go test -run TestListConnected -v ./internal/turzx` | 接続中の TURZX が `TURZX1.0 (633A6E01)` の形式の表示名で列挙されます。TURZX を接続した PC で手動で実行します |
| 文書検査 | `python scripts/doc_check.py .` | NG が0件です |
| 更新の E2E | `node scripts/run.mjs test:desktop` | 「起動時に取得した新版で更新して再起動する」の E2E が合格します。v0.1.0 と v0.2.0 のインストーラーを作り、実際にインストールして更新し、最後にアンインストールします。動いている Token Dashboard をすべて止め、インストールしていない状態で、手元で実行します。`verify` には含めません |
| インストーラーの作成 | `node scripts/run.mjs package` | `bin\token-monitor-turzx-<版>-amd64-setup.exe` ができます。版は `build/app.json` の `version` です。リリースの版はタグで決まるため、`build/app.json` の `version` は上げません |
| リリース | `mise run release:tag`（版を指定するときは `mise run release:tag 1.2.0`） | 最新のタグ（タグがなければ `build/app.json` の `version`）の patch を1つ上げた `v<版>` のタグを、指定したときはその版のタグを push します。コミットしていない変更がない状態で、`origin/main` に含まれるコミットで実行します。push したタグで CI が検証、インストーラーの作成、`update.json` の署名、GitHub Releases の最新リリースへの公開を行い、各 PC のアプリが次の起動時に新版として取得します |

デスクトップ版のビルドとインストーラーの作成では、環境変数 `BUILD_APP_VERSION`・`BUILD_UPDATE_SOURCE`・`BUILD_UPDATE_PUBLIC_KEY` を指定すると、版番号・更新元・公開鍵を、`build/app.json` の代わりにその値にします（指定しない項目は `build/app.json` の値のまま）。値はビルド時に実行ファイルへ埋め込み、実行時の環境変数では変わりません。リリースの CI は `BUILD_APP_VERSION` だけをタグの版にし、更新の E2E は3つとも指定して、手元の更新元を使う版を作ります。

接続設定は `%APPDATA%\io.github.nuitsjp.token-monitor-turzx\settings.json` に保存します（形式は [データ設計](design/data.md)）。環境変数 `WAILS_DATA_DIR` に絶対パスを指定すると、そのディレクトリを使います。設定の保存・再起動後の復元は、`dev` で起動し、トレイのアイコンからウィンドウを開いて保存し、トレイのメニューの `Exit` で終了してから再び起動して確かめます。

E2E（`frontend/tests/e2e/`）はブラウザーから検証用サーバー（`-tags server` のビルド）を操作します。検証用サーバーにはタスクトレイがないため、ページを開くことでトレイからウィンドウを開く操作に、プロセスの停止と起動で `Exit` と再起動に代えます。実際の時間の経過を待たないよう、E2E は検証用サーバーを環境変数 `WAILS_TEST_INTERVALS=short` で起動し、表示画像の描き直しを1分ごとから1秒ごとに、ローカル取得のまとめ待ち・集計の間隔・周期・利用枠の周期を2秒・10秒・45秒・2分から0.2秒・1秒・1秒・1秒に縮めます。E2E は互いに別のポートとデータフォルダーを使い、4本まで並列に実行します。この環境変数は検証用サーバーだけが読みます。ローカル取得の E2E は、検証用サーバーの `HOME`・`USERPROFILE`・`APPDATA`・`LOCALAPPDATA` を一時フォルダーに向け、ログの置き場所を変えうる環境変数（名前が `_HOME`・`_DIR`・`_PATH` で終わるもの、`XDG_` などで始まるもの）を除いて、そこに置いた Claude Code のログを同梱の tokscale に読ませます。この PC の利用状況と Cursor のログイン情報は使いません。利用記録の変更がない間に利用枠だけを取得し直すことと、取得元の変更後に tokscale が止まることは、サーバーの子プロセスを0.25秒ごとに調べて確かめます。短い実行を見逃すことがあるため、集計の重なりと間隔、利用枠の周期、取得の失敗時に前回の値を保つこと、tokscale が起動したプロセスを待たずに停止すること、探索先ファイルを使って起動時に `clients --json` を省き、新しいツールが現れたときだけ取り直すことは、テストの実行ファイルを tokscale の代わりにしてすべての実行を記録する Go の結合テスト（`internal/localusage`）で、待ち時間を縮めて並列に検証します。表示先の選択と `(Disconnected)` の保持は機器の接続状態に依存するため、機器の一覧を差し替えられる Go の単体テスト（`internal/settings`）で検証し、タスクトレイの操作は実機で確認します。更新の E2E だけは、インストールした本番ビルドのウィンドウを Windows の UI オートメーションで操作します。アプリをもう一度起動して既存のウィンドウを表示させることで、トレイからウィンドウを開く操作に代えます。タスクトレイのメニューの更新項目は実機で確認します。署名・対象・ハッシュが一致しない場合と、適用前の再検証の失敗は、Go の単体テスト（`internal/updates`）で検証します。

開発時に Playwright CLI からウィンドウを操作する場合は、環境変数 `WAILS_WEBVIEW_DEBUG_PORT` にポート番号を指定して起動し、`playwright-cli attach --cdp=http://127.0.0.1:<ポート番号>` で接続します。この環境変数は本番ビルドでは無視します。WebView2は起動時に非表示でも初期化されるため、ウィンドウを開く前から接続して画像生成を確認できます。画面のクリック操作を確認する場合は、先にウィンドウを開きます。
