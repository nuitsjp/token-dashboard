# 文書方針

適用する標準、モック駆動開発の適用範囲、正本の配置、および仕様変更の対象を定義します。

<a id="adoption"></a>
## 1. 適用する標準

導入状態: **未適用**。各項目を確定後に「適用済み」へ更新してください。

| 項目 | 内容 |
| --- | --- |
| 配布元 | [aidd-project-template](https://github.com/nuitsjp/aidd-project-template) |
| 設計・文書標準 | [project-template-design-and-documentation](standards/design-and-documentation.md) |
| モック標準 | [project-template-mock-driven-development](standards/mock-driven-development.md) |
| 採用元固定コミット | 9fd9540b0408a0c410fc667c444dce276429060d |
| プロジェクト固有の適用範囲と差分 | {{LOCAL_RULE_DIFFERENCES}} |

採用後、設計・文書標準はすべての変更に適用し、モック標準の適用範囲は第2節で定めます。配布元管理の AGENTS・標準2件・`scripts/doc_check.py`・`.agents/skills/usecase-docs/` のスキルと雛形2件は同じ固定コミットから7ファイル一組で更新し、採用元固定コミット欄には実際に採用した40桁SHAを記載します。作成済みのプロジェクト文書は採用先で管理し、雛形全文は同期しません。現在有効な固有差分のみを上表に記載します（例: `scripts/doc_check.py` を実行できない環境、Playwright CLI が使えない対象の代替確認手段と適用範囲。差分がなければ「なし」）。確認手段を替えても利用者の承認は省略しません。配布元の更新は自動適用しません。

<a id="mock-scope"></a>
## 2. モック駆動開発の適用範囲

ユースケースの主成功系列または拡張系列を新設・変更する作業に適用します。仕様文面を変更しない作業（確定済み仕様の不具合修正、振る舞いを変えない内部変更、文書修正）は対象外です（単体テスト等でのスタブ・モック利用とは区別します）。

プロジェクト固有の除外範囲: {{MOCK_SCOPE}}

<a id="sources"></a>
## 3. 文書の役割と分割

| 正本 | 扱う内容 |
| --- | --- |
| 本書 | 適用する標準、固有差分、モック適用範囲、正本の配置、仕様変更の対象 |
| `standards/` 各標準 | プロジェクト非依存の開発・文書基準（配布元からの輸入物） |
| [project.md](project.md) | プロジェクトの目的・制約、ユースケース一覧、確認した事実、実行・検証手順 |
| `usecases/<名称>/README.md` | 主アクター・目的・共通条件、シナリオ一覧、実現パターンへの参照 |
| `usecases/<名称>/scenarios/<名称>.md` | シナリオ固有の条件・手順・受け入れ条件、種別、UI確認の要否 |
| [.agents/skills/usecase-docs/](../.agents/skills/usecase-docs/SKILL.md) | ユースケース・シナリオの文書構造、作成手順と新規作成用雛形（雛形は実仕様ではない） |
| [architecture.md](architecture.md) | システム構成、共通方針、実現パターン一覧、設計上の制約 |
| `design/UCP-n.md` | 実現パターンごとの処理・役割・境界と UC 固有の差分 |
| [design/data.md](design/data.md) | DB・ファイル等の保存形式と現在のテーブル設計 |
| `reference/` | 外部システムの実測応答（取得日時・方法・対象版を記録。利用時のみ） |
| [README.md](../README.md) | プロジェクト概要と参照案内 |
| [AGENTS.md](../AGENTS.md) | AIエージェントの作業規範 |

- **新設の禁止**: 本表にない規約・方針・プロセス文書は新設しません。固有の規則は第1節の差分欄、[project.md](project.md) 第2節の制約、または実現パターン内に記述します。作業単位と再開は [モック標準第2節](standards/mock-driven-development.md#workflow) に従い、進捗・現在地・未決事項の管理文書は置きません。複数セッションにまたがる長期計画は外部の課題管理システム等で扱います。
- **文書の分割基準**: 単独参照の必要性や更新頻度の違いにより管理が困難な場合のみ分割を認めます。分割時は元の記述を参照リンクへ置き換え、本表と関連リンクを更新します。
- **図の形式**: Mermaid を使用します（コンテキスト・コンテナは flowchart または C4 構文、系列は sequenceDiagram、ER図は erDiagram）。

<a id="agreements"></a>
## 4. 仕様変更の対象

採用規則、適用範囲、正本の責務、[プロジェクト定義](project.md) と `usecases/` のユースケース・シナリオの要件・制約・仕様・完了条件、および [アーキテクチャ](architecture.md) の構成・設計上の制約と `design/` の処理・保存設計を変更・緩和する場合は [変更手続き](standards/design-and-documentation.md#agreement-changes) に従います。文書再編でも現在有効な仕様を維持し、不要になった記述は削除します。

初期記入欄、検討中の案、モック上の仮定は合意とみなしません。判断に必要な未確定事項は停止点で利用者に確認し、確定するまで仕様・設計として記録しません。
