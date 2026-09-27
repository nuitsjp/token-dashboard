# UCP-2. 入力・確認・実行・結果確認

関連: [アーキテクチャ](../architecture.md)、[ユースケース](../usecases/CSVの内容を確認して一括登録する/README.md)、[データ設計](data.md)。

「CSVの内容を確認して一括登録する」 に適用します。`ImportDialogue` が複数ルート間の状態を所有し、`ImportInput` と `ImportConfirm` を切り替えます。取得には 「メモを作成・編集して保存する」 と同一の `listNotes` を使用します。

```mermaid
sequenceDiagram
  actor User as 利用者
  participant UI as ImportDialogue
  participant Q as 機能アクセス
  participant Go as notes.Service
  User->>UI: CSV入力・確認指示
  UI->>Q: PreviewImport
  Q->>Go: CSV解析
  Go-->>UI: 取り込み候補
  User->>UI: 実行指示
  UI->>Q: Import
  Q->>Go: CSV・処理ID
  Go-->>UI: 進捗イベント
  Go->>Go: 全件を検証して一括確定
  Go-->>Q: 結果・変更通知
  Q-->>UI: 結果表示・一覧更新
```

確定前の失敗や中止では一部行のみの保存は行いません。中止と確定の競合は、一覧の再取得で実状態を確認します。最後の進捗は `GetImportProgress` でも取得できます。モック合成点は [UCP-1](UCP-1.md) と同様です。

UC 固有の逸脱: 「CSVの内容を確認して一括登録する」 はなし。
