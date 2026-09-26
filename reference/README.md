# Wails アプリの参照実装

Windows用のユースケース駆動参照アプリです。対話制御を React、機能と保存を Go に配置し、メモ編集・CSV取り込み・アプリ内更新を実装しています。生成時にこのアプリ一式を製品ルートへコピーします。製品開発を開始しても、このディレクトリを参照用として残します。

## Windowsで開始

生成後の環境構築、起動、モック切り替え、ビルド、E2E の手順は [プロジェクト定義の実行手順](docs/project.md#commands) に集約しています。生成先の `reference/` で実行します。

## 確認できる実装

メモの作成・選択・編集・保存、入力エラーと未保存確認、CSV 入力と確認画面をまたぐ下書き、Go による一括保存・進捗通知・中止要求を含みます。CSV は `title,body` 列形式です。

```csv
title,body
設計メモ,対話はフロントエンドで制御する
実装メモ,保存はGoで確定する
```

進捗は実処理から通知し、見せるための待ち時間は入れていません。

初回起動時のメモは空です。データは OS のユーザー設定領域（アプリ ID 配下）に保存されます。別の試験データを使う場合だけ、環境変数 `WAILS_DATA_DIR` に絶対パスを指定します。更新やアンインストールでデータは削除されません。

## 配布と更新の設定

アプリ名・識別子・版・実行ファイル名・更新元・更新用公開鍵は [`build/app.json`](build/app.json) で設定します。初期状態の更新元と鍵は空で、存在しない公開リリースや共通秘密鍵は同梱しません。更新コードと画面は実装済みで、配布者が設定すると利用できます。

[更新元と署名の設定手順](docs/project.md#release) に従い、初回インストーラー作成前に公開鍵と更新元を設定してください。共有フォルダと公開 GitHub Releases に対応し、private リポジトリの認証・鍵ローテーション・差分更新は含めません。

NSIS はアプリ本体、スタートメニュー、アンインストール情報を管理します。アプリ内更新では、取得物の検証と利用者の確認後に NSIS へ引き渡し、旧 PID の終了を最大60秒待って適用・再起動します。適用失敗時の自動ロールバックはありません。

この版の NSIS は WebView2 の既存導入を検査し、未導入なら案内して停止します。Runtime のオンライン自動取得は組み込んでいません。[Microsoft公式のEvergreen Installer](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution) を先に導入してください。未署名による実行制限は、更新用 Ed25519 署名では解除されません。

## 設計・規則・採用

- [アーキテクチャ](docs/architecture.md)
- 実現パターンの設計: [UCP-1](docs/design/UCP-1.md)、[UCP-2](docs/design/UCP-2.md)、[UCP-3](docs/design/UCP-3.md)
- [データ設計](docs/design/data.md)
- [Wails補足](docs/architecture-wails.md)
- [プロジェクト定義](docs/project.md)

サンプルの UI やデータ設計は参照用です。製品開発時は製品ルートにある下表の「サンプル」を製品固有の実装へ置き換え、製品固有の UC・データ設計・現在の仕様を親ディレクトリの `docs/` に定義してください。参照実装の仕様を製品へそのまま転用しません。

| 区分 | 対象 |
| --- | --- |
| 残す基盤 | `main.go`（Service 登録の骨格）、`internal/desktop`・`appstate`・`fault`・`diagnostics`・`updates`、`cmd/release`、`frontend/src/app`・`shared`・`features/application`・`features/updates`、`usecases/update-app`、`routes/__root.tsx`・`routes/updates.tsx`、`build/`、`scripts/`、`Taskfile.yml` |
| 置き換えるサンプル | `internal/notes`、`frontend/src/features/notes`、`usecases/edit-notes`・`import-notes`、`routes/notes.tsx`・`routes/import*.tsx`・`routes/index.tsx` の遷移先、`Shell.tsx` のナビゲーションと `subscribeNotes` の購読、`tests/fixtures/notes.ts`、`tests/e2e/usecases/`、`vite.config.ts`・`tsconfig.json`・`eslint.config.mjs` の `@notes-service` 設定 |

新しい機能領域のモック合成点の作り方は [Wails補足第4節](docs/architecture-wails.md#4-モックと検証境界) を参照します。

採用時は [導入開始手順](https://github.com/nuitsjp/aidd-project-template#3-初期セットアップと最初のユースケース) を確認し、製品固有の内容を定義します。

上表の「残す基盤」も生成後は採用先が管理するコードです。継続同期する対象ではありません。配布元から一組で更新する対象は `AGENTS.md`・標準2件・文書検査器・`.agents/skills/usecase-docs/` とし、製品文書・実装・設定・DB移行履歴は上書きしません。採用元コミットと固有差分の扱いは [ルートの文書方針](../docs/document-policy.md#adoption) に従います。

ライセンスは [MIT](../LICENSE) です。
