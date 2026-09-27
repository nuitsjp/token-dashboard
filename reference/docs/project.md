# Wails Template のプロジェクト定義

## 1. 目的と範囲

Wails の共通構造を動作可能な実装として示す参照アプリです。メモの編集、CSV 一括取り込み、起動・終了、およびアプリ内更新機能を含みます。業務プロダクトや汎用 CRUD 基盤を作るものではありません。生成時には製品ルートへアプリ一式をコピーし、このディレクトリは参照用として残します。

## 2. 制約・受け入れ条件

Windows デスクトップを主対象とします（ブラウザ server build は確認専用）。Go 機能の再利用、下書きとキャッシュの分離、結果確定後の通知を確認できる構成にします。各系列の UI 受け入れと Windows 実機確認が済むまで、テンプレートを動作保証済みとは扱いません。

サンプルの上限値はタイトル100文字、本文10,000文字、CSV 1 MiB・1,000行です。大量データ対策の共通基盤は含まず、新しい製品へこれらの上限を暗黙に引き継ぎません。

<a id="usecases"></a>
## 3. ユースケース一覧

| ユースケース | 主アクター | 目的 | 実装順序 | 実現パターン | モック適用 |
| --- | --- | --- | --- | --- | --- |
| [メモを作成・編集して保存する](usecases/メモを作成・編集して保存する/README.md) | 利用者 | メモを作成・編集して保存する | 1 | [UCP-1](design/UCP-1.md) | 対象 |
| [CSVの内容を確認して一括登録する](usecases/CSVの内容を確認して一括登録する/README.md) | 利用者 | CSVの内容を確認して一括登録する | 2 | [UCP-2](design/UCP-2.md) | 対象 |
| [新版を確認してアプリを更新する](usecases/新版を確認してアプリを更新する/README.md) | 利用者 | 新版を確認してアプリを更新する | 3 | [UCP-3](design/UCP-3.md) | 対象 |

<a id="design"></a>
## 4. 確認した事実

共通資材の配布元と採用元固定コミットは [ルートの文書方針](../../docs/document-policy.md#adoption) に従います。

Wails 本体・CLI・npm ランタイムは `v3.0.0-beta.23` / `3.0.0-beta.23`、Go 1.25以上を前提とします。Node.js は `.nvmrc` と `mise.toml` で固定し、生成 API・Service 登録・ライフサイクル・runtime Vite plugin は [固定版ソース](https://github.com/wailsapp/wails/tree/v3.0.0-beta.23/v3) および [CLI資料](https://v3.wails.io/guides/cli/) に従います。

生成先では依存取得、Wails バインディング生成、TypeScript 型検査、実 Go サービスを使う E2E を実行します。Headless E2E は導入済み Playwright に対応する Chromium Headless Shell を使用します。

<a id="commands"></a>
## 5. 実行・切り替え・検証手順

生成したプロジェクトの `reference/` を作業ディレクトリとします。製品ルートのアプリも同じ手順で実行します。Go 1.25以上、`.nvmrc` と完全一致する Node.js 24.21.0、Python 3.9以上（mise の指定は3.14系）、WebView2 Evergreen Runtime を導入し、`go`・`node`・`npm`・`python` を PATH 上で使用できるようにします。NSIS 3.11以上はインストーラー作成時のみ必要です。Go の自動ツールチェーン取得を禁止する環境では、依存モジュールが要求する Go 版も事前に導入します。

nvm-windows を使う場合は次の手順で指定版を選択します。mise を使う場合は `mise install` で指定ツールを導入し、`mise exec -- node scripts/run.mjs setup` と `mise exec -- node scripts/run.mjs dev` を実行します（nvm の操作は不要）。

```powershell
$nodeVersion = (Get-Content .nvmrc -Raw).Trim()
nvm install $nodeVersion
nvm use $nodeVersion
node scripts/run.mjs setup
node scripts/run.mjs dev
```

`setup` は指定版の Wails CLI をローカルの `.tools/` に導入し、Go/npm 依存、実際の Go バインディング、ルートツリーを生成します。初回は外部ネットワークが必要です。`go.sum` と `frontend/package-lock.json` は配布物に含まれ、フロントエンド依存は `frontend/` で `npm ci` を使用します。直接依存の版を変更したときは両ファイルを更新します。

Wails 本体・JavaScript Runtime は固定版の組合せで使用し、`setup` で Go 依存に対応する CLI と生成コードを更新します。採用後の依存定義・ロックは採用先で管理します。Node.js が `.nvmrc` と異なる場合は `scripts/run.mjs` が処理開始前に停止します。CI も `.nvmrc` を使用します。

| コマンド（先頭に `node scripts/run.mjs`） | 内容 |
| --- | --- |
| `dev` | Windows アプリを起動し、Go・React の変更を監視 |
| `dev:mock` | 試験用固定データで起動（実データは変更しない） |
| `build` | 本番実行ファイルを `bin/` に生成 |
| `package` | ビルド後、ユーザー単位の未署名 NSIS インストーラーを `bin/` に生成 |
| `server` | Go 実処理を使うブラウザ確認用サーバーを localhost:34115 で起動 |
| `verify` | 生成・型検査・Lint・テスト・文書検査・server E2E を一括実行 |
| `test:core` | Go の機能・保存・更新検証を実行 |

初回の E2E 実行前に `cd frontend; npx playwright install --only-shell chromium; cd ..` を実行します。`server` は開発・確認用であり、LAN へ公開しません。終了は Ctrl+C とします。`dev:mock` は Wails を起動したままメモ機能のみを固定データへ差し替えます（画面に「試験用モック」が表示されることを確認）。本番ビルドでモック設定を検出した場合はビルドを中止します。

ブラウザー取得にプロキシが必要な端末では、その環境で指定されたプロキシURLを `HTTPS_PROXY` に設定してから導入コマンドを実行します。端末固有のURLを配布物へ固定せず、ブラウザーやNode.jsを自動的に切り替える処理は設けません。

通常の Chrome / Chromium を明示して検証する場合は、`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` に実行ファイルの絶対パスを設定します。その環境で既定引数 `--disable-extensions` による起動失敗を確認した場合だけ、`PLAYWRIGHT_IGNORE_DISABLE_EXTENSIONS=1` も設定して当該引数を除外します。指定例は次のとおりです。自動的なブラウザー切り替えは行いません。

```powershell
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = Join-Path $env:ProgramFiles 'Google/Chrome/Application/chrome.exe'
$env:PLAYWRIGHT_IGNORE_DISABLE_EXTENSIONS = '1'
node scripts/run.mjs verify
```

E2E は実 Go サービスと専用の一時データ領域を使用します。このディレクトリで `python ../scripts/doc_check.py .` を実行すると文書リンク・UC 対応・標準ハッシュ等を確認できます。Windows 実機確認では、保存後の再起動、未保存状態からの終了、多重起動、CSV 処理中の終了、インストール・更新・アンインストール後のデータ保持を確認します。変更後は `node scripts/run.mjs verify` を実行し、生成・型検査・Lint・テスト・文書・server E2E が合格した状態を維持します。

<a id="release"></a>
### 更新元と署名

`build/app.json` の変更後は再ビルドします。公開鍵と更新元は初回インストーラーに埋め込みます。開発者は秘密鍵をリポジトリ外で生成します。

```powershell
node scripts/run.mjs release keygen -out "$env:USERPROFILE/wails-release-private-key.txt"
```

表示された公開鍵を `build/app.json` の `updatePublicKey` に設定します。`updateSource` には共有フォルダの絶対パス、または公開 GitHub Releases の `https://github.com/OWNER/REPO/releases/latest/download/update.json` を設定します。JSON で Windows パスを記述する場合はバックスラッシュをエスケープします。private リポジトリの認証は対象外です。

初回版を `package` で作成します。新版発行時は `build/app.json` の版だけを増やし、同じ ID・公開鍵・更新元で `package` を実行します。

```powershell
node scripts/run.mjs package
node scripts/run.mjs release manifest -key "$env:USERPROFILE/wails-release-private-key.txt" -installer bin/wails-template-0.2.0-amd64-setup.exe -app-id io.github.nuitsjp.wails-template -version 0.2.0 -arch amd64 -out bin -notes "更新内容"
```

生成した `update.json` と対象インストーラーを同じ共有フォルダまたは Release assets へ配置します。共有フォルダではインストーラーを先に完全配置し、更新情報を最後に置き換えます。GitHub Releases は Draft へ両方を配置してから公開します。ZIP や別形式を `setup.exe` として指定しません。

旧版の更新画面で確認・取得・適用します。NSIS の実行待ちは旧アプリ PID を対象とします。署名・ハッシュ違反、適用失敗、組織ポリシーの拒否を成功扱いにしません。復旧時は同じ信頼できるインストーラーを手動実行します。

**この参照版の制約:** Runtime がない端末は WebView2 を事前導入します。署名鍵の自動交換、private Releases、古いステージファイルの自動清掃、インストーラーのトランザクション復旧は実装していません。インストーラー自体は未署名であり、実行許可は環境に依存します。
