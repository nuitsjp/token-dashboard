# Token Dashboard

ローカルまたは Token Monitor Hub から取得した AI ツールの利用状況（トークン数・推定コストと利用枠）を、TURZX 9.2インチ USB ディスプレイへリアルタイムに表示する Windows 常駐アプリです。Windows へのサインイン時に自動で起動してタスクトレイに常駐し、ウィンドウを開くと USB ディスプレイと同じ表示のプレビューを確認し、取得元と表示先を設定できます。Wails（Go・React）で構成し、GitHub Releases で公開するインストーラーから導入・自動更新します。

## Local と Token Monitor Hub の使い分け

既定の取得元 `Local` は、同梱の tokscale でこの端末の利用状況を取得する簡易版です。Token Monitor の別途インストールは不要ですが、取得できる提供元・アカウント・利用枠や、表示される契約名は Token Monitor と異なる場合があります。Token Monitor に登録した複数アカウントの情報は、Local には自動で共有されません。

より詳細な利用状況や複数アカウントの情報を取得したい場合は、[Token Monitor](https://github.com/Javis603/token-monitor) との併用を推奨します。Token Monitor の Hub を有効にし、本アプリの `Connection` 画面で取得元を `Hub` に切り替えて、Hub の URL と認証トークンを設定してください。どちらの取得元でも、利用枠は TURZX の表示領域に収まる件数だけ表示します。

## 実行・確認手順

環境構築、起動、モック再現、実処理切り替え、および検証の手順は [プロジェクト定義の実行手順](docs/project.md#commands) を参照します。

文書方針が未適用の場合は、[採用後の導入開始手順](https://github.com/nuitsjp/aidd-project-template#3-初期セットアップと最初のユースケース) を確認して開始します。

## 関連文書

文書の役割と正本の配置は [文書方針第3節](docs/document-policy.md#sources) を参照します。
