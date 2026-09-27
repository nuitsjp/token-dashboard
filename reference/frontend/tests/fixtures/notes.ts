// Test-only UI reproduction. This is not the specification-approval mock and
// never enters a production build. Wails still supplies its real event runtime.
import { CancellablePromise, Events } from '@wailsio/runtime';
import type { Note, SaveRequest, ImportRequest, ImportPreview, ImportResult, ImportProgress } from '@bindings/wailstemplate/internal/notes/models';

const initial: Note[] = [{ id: 'fixture-1', title: '試験用メモ', body: '固定データです。実データは変更しません。', updatedAt: '2026-09-20T00:00:00Z' }];
const saved: Note = { id: 'fixture-1', title: '保存後の試験用メモ', body: '固定データの保存後状態です。', updatedAt: '2026-09-20T00:01:00Z' };
let afterSave = false;
export function List() { return CancellablePromise.resolve(afterSave ? [saved] : [...initial]); }
export function Get(_id: string) { return CancellablePromise.resolve(afterSave ? saved : initial[0]); }
export function Save(_request: SaveRequest) { afterSave = true; return CancellablePromise.resolve(saved); }
export function PreviewImport(_csv: string) { return CancellablePromise.resolve<ImportPreview>({ rows: [{ title: '試験用取り込み', body: '固定データ' }], count: 1 }); }
export function Import(request: ImportRequest) {
  void Events.Emit('notes:import-progress', { operationID: request.operationID, completed: 1, total: 1, phase: 'completed' });
  return CancellablePromise.resolve<ImportResult>({ ids: ['fixture-import'], count: 1 });
}
export function GetImportProgress(operationID: string) { return CancellablePromise.resolve<ImportProgress>({ operationID, completed: 1, total: 1, phase: 'completed' }); }
