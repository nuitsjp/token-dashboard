import { createContext, useContext, useEffect, useRef, useState } from 'react';
import { Outlet, useBlocker } from '@tanstack/react-router';
import { Button, Group, Modal, Stack, Text, Title } from '@mantine/core';
import type { ImportPreview, ImportProgress, ImportResult } from '@bindings/wailstemplate/internal/notes/models';
import { subscribeImport, useImportNotes } from '../../features/notes/queries';
import { useDraftDirty } from '../../shared/ExitContext';

type Value = {
  csv: string; setCSV: (value: string) => void;
  preview: ImportPreview | null; setPreview: (value: ImportPreview | null) => void;
  operationID: string; setOperationID: (id: string) => void;
  progress: ImportProgress | null;
  result: ImportResult | null; setResult: (value: ImportResult | null) => void;
  execution: ReturnType<typeof useImportNotes>;
};
const Context = createContext<Value | null>(null);
export function useImportDialogue() { const value = useContext(Context); if (!value) throw new Error('ImportDialogue is missing'); return value; }
export function ImportDialogue() {
  const [csv, setCSV] = useState('');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [operationID, updateOperationID] = useState('');
  const operationIDRef = useRef('');
  const setOperationID = (id: string) => { operationIDRef.current = id; updateOperationID(id); setProgress(null); };
  const [progress, setProgress] = useState<ImportProgress | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const execution = useImportNotes();
  const dirty = csv.length > 0 && result === null;
  useDraftDirty(dirty);
  useEffect(() => subscribeImport(value => { if (value.operationID === operationIDRef.current) setProgress(value); }), []);
  const blocker = useBlocker({
    shouldBlockFn: ({ next }) => execution.mutation.isPending || (dirty && !next.fullPath.startsWith('/import')),
    enableBeforeUnload: dirty || execution.mutation.isPending, withResolver: true,
  });
  return <Context.Provider value={{ csv, setCSV, preview, setPreview, operationID, setOperationID, progress, result, setResult, execution }}>
    <Stack gap="lg"><div><Text c="dimmed" size="sm">UCP-2 / 入力・確認・一括実行</Text><Title order={1}>CSVから取り込む</Title><Text mt="sm" c="dimmed">画面をまたいで下書きを維持し、Go側で全件を一括保存します。</Text></div><Outlet /></Stack>
    <Modal opened={blocker.status === 'blocked'} onClose={() => blocker.reset?.()} title="取り込みを終了しますか？" centered>
      <Text>{execution.mutation.isPending ? '処理の完了または中止を待ってください。' : '入力したCSVと確認内容を破棄します。'}</Text>
      <Group justify="flex-end" mt="lg"><Button variant="default" onClick={() => blocker.reset?.()}>戻る</Button><Button disabled={execution.mutation.isPending} color="red" onClick={() => blocker.proceed?.()}>破棄して移動</Button></Group>
    </Modal>
  </Context.Provider>;
}
