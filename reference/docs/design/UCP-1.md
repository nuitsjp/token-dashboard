# UCP-1. 取得・編集・保存

関連: [アーキテクチャ](../architecture.md)、[ユースケース](../usecases/メモを作成・編集して保存する/README.md)、[データ設計](data.md)。

「メモを作成・編集して保存する」 に適用します。役割は `EditNotes/Editor`（下書き）、`features/notes/queries.ts`（取得・更新）、`notes.Service`（検証・保存）です。

```mermaid
sequenceDiagram
  actor User as 利用者
  participant UI as 対話制御
  participant Q as 機能アクセス
  participant Go as notes.Service
  User->>UI: メモを選択
  UI->>Q: 取得
  Q->>Go: Get
  Go-->>UI: 保存済みメモ
  User->>UI: 入力・保存指示
  UI->>Q: 下書きを保存
  Q->>Go: Save
  Go->>Go: 検証・ファイル置換による確定
  Go-->>Q: 保存結果 / 変更通知
  Q-->>UI: 関連Queryの更新
```

Go のファイル置換成功を結果確定点とします。失敗時は既存データと下書きを保持し、取得結果の再取得で下書きを上書きしません。モック合成点は `frontend/vite.config.ts` の `@notes-service` です。

UC 固有の逸脱: 「メモを作成・編集して保存する」 はなし。
