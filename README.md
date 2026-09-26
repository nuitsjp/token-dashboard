# Wails テンプレート

新規プロジェクトは配布元ルートの `mise run init:wails <新しい出力先>` で生成します。生成先ルートには Wails・React・Go の実行可能なアプリ一式が入り、メモ編集・CSV取り込み・アプリ内更新の参照実装は `reference/` に残します。製品の現行仕様はルートの `docs/` に記述し、参照実装のユースケース・シナリオと設計は `reference/docs/` に保持します。

生成先ルートで `node scripts/run.mjs setup` を実行し、`node scripts/run.mjs dev` で製品アプリを起動するか `node scripts/run.mjs verify` で検証します。サンプルは `reference/` へ移動して同じコマンドを実行します。`.github/workflows/windows.yml` は生成先ルートと `reference/` の両方で `setup`・`verify`・`build`・`package` を実行し、インストーラーを artifact として保存する CI 例です。

サンプルの構成、実行手順、配布・更新の設定と、製品固有の実装へ置き換える箇所は [reference/README.md](reference/README.md) から参照してください。共通の `AGENTS.md`、標準、文書検査スクリプトは生成時に `template/` からルートへ配置します。生成時に依存取得やビルドは行わず、既存の出力先は上書きしません。`wails-template/` は単体では実行せず、生成先で開発・検証します。
