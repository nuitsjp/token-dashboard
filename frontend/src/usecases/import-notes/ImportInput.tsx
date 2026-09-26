import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Button, Group, Paper, Stack, Text, Textarea, Title } from '@mantine/core';
import { listNotes, usePreviewImport } from '../../features/notes/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useImportDialogue } from './ImportDialogue';

export function ImportInput() {
  const dialogue = useImportDialogue();
  const preview = usePreviewImport();
  const current = useQuery(listNotes());
  const navigate = useNavigate();
  async function next() {
    try { const value = await preview.mutateAsync(dialogue.csv); dialogue.setPreview(value); dialogue.setResult(null); await navigate({ to: '/import/confirm' }); } catch { /* Retain CSV for correction. */ }
  }
  return <Paper p="xl" withBorder><Stack>
    <Group justify="space-between"><Title order={3}>1. CSVを入力</Title><Text size="sm" c="dimmed">現在の保存件数: {current.data?.length ?? '—'}件</Text></Group>
    <Text size="sm" c="dimmed">先頭行は title,body。既存のメモは変更せず、新しいメモとして追加します。</Text>
    <ErrorNotice error={preview.error} />
    <Textarea label="CSVデータ" placeholder={'title,body\n最初のメモ,本文'} value={dialogue.csv} minRows={10} autosize
      error={preview.error ? publicError(preview.error).fieldErrors?.csv : undefined}
      onChange={event => { dialogue.setCSV(event.currentTarget.value); dialogue.setPreview(null); dialogue.setResult(null); }} disabled={preview.isPending || dialogue.execution.mutation.isPending} />
    <Group justify="flex-end"><Button loading={preview.isPending} onClick={() => void next()}>内容を確認</Button></Group>
  </Stack></Paper>;
}
