# テーマ描画方式の調査結果

2026-10-09に、GaugesとBarsの2テーマについて、拡張性と画像生成速度を比較した。採用方式は **WebView2：HTML/CSS＋Canvas** とする。この文書は利用者の依頼により調査結果を保存したものであり、現在の実装上の責務は [UCP-1](../design/UCP-1.md) を参照する。

## 採用方式

テーマ作者はHTML/CSSとHandlebarsで静止画用のテーマを作る。CSS Grid・Flexboxで配置し、ゲージやバーはインラインSVGで描画できる。アプリは共通のAgentアイコンと表示データを提供する。

常駐するWebView2内の共通処理が、展開したHTMLにCSSと画像を内包し、SVGの`foreignObject`に収めてCanvasへ描画する。PNGと、時計回りに90度回転したJPEGを生成してアプリ本体へ返す。テーマ作者はSVGへの包装、Canvasへの描画、回転、画像圧縮を実装しない。

テーマは次のファイルで構成する。

| ファイル | 内容 |
| --- | --- |
| `theme.json` | テーマ識別子、名称、画像サイズ、テンプレートとスタイルシートの指定 |
| `template.hbs` | 表示データを使って組み立てるHTMLとインラインSVG |
| `style.css` | 配色、文字、配置、ゲージ等の見た目 |
| テーマ固有の画像 | 必要な場合だけ追加する。Agentアイコンはアプリ共通の素材を参照する |

WebView2、テンプレート、CSS、共通アイコン、Canvasは再利用する。初期実装では更新のたびにテンプレートを展開する。変更領域を管理する仕組みは設けない。

## 測定条件

| 項目 | 条件 |
| --- | --- |
| CPU | AMD Ryzen 9 7950X、16コア・32スレッド |
| OS | Windows 11 Pro、10.0.26200.0 |
| WebView2 Runtime / SDK | 154.0.4258.62 / 1.0.4258.31 |
| Go / Node.js / .NET SDK | 1.26.5 / 24.21.0 / 10.0.401 |
| resvg | 0.48.1、公式C APIをcgoで呼び出す |
| 入力とPNG | 1920×462 |
| JPEG | 時計回りに90度回転した462×1920 |
| サンプル | 各方式・形式・テーマで100回×3回。各回のウォームアップは10回。計9,600フレーム |
| 更新内容 | Claudeの1つのメーターを毎回72%と71%に切り替える |
| キャッシュ | フォント、静的素材、コンパイル済みテンプレートは再利用する。生成済みの72%・71%画像は使い回さない |

実際のWebView2で測定した。ChromeやPlaywrightの画面キャプチャは使用していない。HTML/CSS＋Canvasは既存の2テーマを変更せずに使った。SVGの画像キャッシュによる再利用を避けるため、フレームごとに異なるSVG画像URLを生成した。

時間には、表示データとテンプレートの準備、描画、画像圧縮、WebView2とのデータ受け渡しを含む。初期起動、HTTP転送、検証処理、ハッシュ計算、ディスク保存、USB転送は含まない。PNGとJPEGは別々に測定し、両方を同時に生成する時間は測定していない。

最初の6方式は実行順を変えながら3回測定した。HTML/CSS＋Canvasと、非表示ウィンドウ＋OffscreenCanvasは、その後に各3回測定した。初回のGo測定は検証プログラムの準備作業と重なったため、準備終了後に取り直した値を使用した。

## 画像生成時間

数値は1画像の生成時間の中央値。単位はミリ秒。

| 方式 | PNG Gauges | PNG Bars | JPEG Gauges | JPEG Bars |
| --- | ---: | ---: | ---: | ---: |
| 元のGo描画 | 32.5 | 17.8 | 24.8 | 11.8 |
| WebView2：全体更新＋CapturePreview | 76.0 | 74.6 | 75.2 | 60.2 |
| WebView2：メーターだけ更新＋CapturePreview | 76.6 | 74.7 | 74.8 | 60.4 |
| SVG＋resvg：全体描画 | 60.6 | 50.9 | 53.4 | 45.0 |
| SVG＋resvg：変更領域だけ描画 | 30.1 | 27.8 | 22.6 | 22.5 |
| WebView2：SVG＋Canvas | 14.5 | 15.0 | 14.0 | 14.4 |
| **WebView2：HTML/CSS＋Canvas** | **14.8** | **15.0** | **14.1** | **14.4** |
| WebView2：HTML/CSS＋OffscreenCanvas・非表示ウィンドウ | 12.7 | 11.9 | 12.6 | 11.7 |

GoとresvgのJPEGは品質85、Canvasは品質0.85を指定した。これらの保存画像の量子化テーブルは一致した。CapturePreviewのJPEG品質は指定できず、量子化テーブルも異なるため、そのJPEG時間は参考値である。描画エンジン間には文字の輪郭、色管理、細かな配置の差があり、同じ内容の画像を比較しているが全方式の画素一致を保証する比較ではない。

## 判断

### 実アプリへの組み込み後の測定

実Hubから受信した6契約のデータと保存済みの表示選択を使い、Wailsの常駐WebView2でウィンドウを非表示にして測定した。PNGとJPEGを同じ描画から生成し、各スタイルでウォームアップ10回の後に100回測定した。

| スタイル | 中央値 | 95パーセンタイル |
| --- | ---: | ---: |
| Gauges | 66.9ms | 77.0ms |
| Bars | 60.1ms | 68.7ms |

測定範囲は、描画要求を取得するWails呼び出しの開始から、PNGとJPEGを本体へ返す呼び出しの応答まで。データの準備、初期起動、設定保存、USB送信は含めない。同じ表示内容の画像はブラウザーが再利用できる。上表は二形式の生成とWailsの往復を含むため、前節の単形式・検証プログラムの値とは直接比較しない。

組み込み試験では、非表示中の`Canvas.toBlob`の完了待ちが約1秒を占めた。現在の実装は`Canvas.toDataURL`を使う。画像の仕様と障害時の動作は [UCP-1](../design/UCP-1.md) を参照する。

### 更新箇所を絞る効果

GaugesのDOM更新は、全体更新の中央値約3.0msから、メーターだけ更新する約0.36msへ短縮した。しかしCapturePreview自体が約73〜76msを占め、全体の画像生成時間は短縮しなかった。

HTML/CSS＋CanvasではCapturePreviewを使わないため、PNG生成を約80%短縮できた。Handlebarsによるテンプレート展開は約0.12〜0.16msであり、現時点では差分更新による複雑化を必要としない。

resvgの部分描画も改善したが、変更領域の抽出、背景の保持、配置変更時の扱いが必要になる。今回の測定では、HTML/CSS＋Canvasより画像生成時間が長かった。

### HTML/CSSを採用する理由

SVG＋CanvasとHTML/CSS＋Canvasの速度はほぼ同等だった。HTML/CSSを使えば、テーマ作者が一般的なレイアウト手法を使え、既存2テーマも再利用できる。純粋なSVGだけでテーマを記述する制限を加える理由はない。

GoのBarsのJPEGは11.8msで、推奨方式の14.4msより速かった。推奨方式は全条件で最速という判断ではなく、テーマの作りやすさと画像生成速度を合わせた判断である。

### 通常のCanvasを初期採用する理由

通常のCanvas構成の95パーセンタイルは、PNGがGauges 15.6ms・Bars 15.8ms、JPEGが両テーマ15.1msだった。

非表示ウィンドウ＋OffscreenCanvas構成は中央値が約12msと速い一方、95パーセンタイルはPNGが24.1ms・35.5ms、JPEGが20.6ms・29.2msだった。Canvas APIに加えネイティブウィンドウの表示条件も変わっているため、差をOffscreenCanvas固有の性質とは断定できない。今回の通常Canvas構成は十分に速く、ばらつきも小さかったため初期採用する。

## 確認できた範囲と未検証事項

- GaugesとBarsの数値、ゲージ、バー、共通アイコンを画像として生成できた。
- resvgの部分描画は、全サンプルで全体描画のRGBAとエンコード結果に一致した。
- 通常CanvasとOffscreenCanvasの保存画像8枚は、出力バイト列が一致した。
- 保存したJPEG 156枚は、Baseline JPEG、462×1920、1MiB以下を満たした。
- ネイティブウィンドウを非表示にしても生成できた。ただし`document.visibilityState`は`visible`のままであり、ページが`hidden`または休止状態になる条件は確認していない。
- 検証プログラムが内包する素材はスタイルシートと`img`の画像である。一般的なCSSの`url()`、`@import`、SVGの外部画像、Webフォントを扱う処理は未実装だった。
- 調査時点では実アプリへの組み込み、長時間のトレイ常駐、CPU・メモリ使用量、PNGとJPEGの同時生成、実機へのUSB転送は未検証だった。この表の値をアプリ全体の処理時間として扱わない。

## 再測定に使った実装

検証プログラムは [themes/benchmark](../../themes/benchmark/) に置く。生成画像、実行ファイル、生の測定JSONはリポジトリに保存しない。

| 比較対象 | ソース |
| --- | --- |
| Go描画 | [go/main.go](../../themes/benchmark/go/main.go) |
| WebView2全体・差分更新 | [webview2-incremental/Program.cs](../../themes/benchmark/webview2-incremental/Program.cs) |
| SVG全体・部分描画 | [partial-resvg/main.go](../../themes/benchmark/partial-resvg/main.go)、[partial-svg/server.mjs](../../themes/benchmark/partial-svg/server.mjs) |
| SVG＋Canvas | [webview2-svg-canvas/Program.cs](../../themes/benchmark/webview2-svg-canvas/Program.cs) |
| HTML/CSS＋Canvas | [webview2-html-canvas/Program.cs](../../themes/benchmark/webview2-html-canvas/Program.cs) |
| HTML/CSS＋OffscreenCanvas | [webview2-html-offscreen/Program.cs](../../themes/benchmark/webview2-html-offscreen/Program.cs) |

測定の基礎となるAPIは、[Canvas.toBlob](https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/toBlob)、[OffscreenCanvas.convertToBlob](https://developer.mozilla.org/en-US/docs/Web/API/OffscreenCanvas/convertToBlob)、[WebView2 CapturePreviewAsync](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2.capturepreviewasync)を参照する。
