import { Link } from '@tanstack/react-router';
import { Alert, Button, Group, Paper, Progress, Stack, Table, Text, Title } from '@mantine/core';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { useImportDialogue } from './ImportDialogue';

export function ImportConfirm() {
  const dialogue = useImportDialogue();
  const { mutation, cancel } = dialogue.execution;
  async function execute() {
    const id = crypto.randomUUID(); dialogue.setOperationID(id);
    // The mounted parent listener matches this ID through a synchronously updated ref.
    try { const result = await mutation.mutateAsync({ csv: dialogue.csv, operationID: id }); dialogue.setResult(result); }
    catch { /* Keep the input and expose the public failure. */ }
  }
  if (!dialogue.preview) return <Alert title="確認内容がありません"><Text>先にCSVを入力してください。</Text><Button component={Link} to="/import" variant="light" mt="sm">入力画面へ</Button></Alert>;
  const progress = dialogue.progress;
  // PreviewImport only succeeds with a non-empty, non-nil row slice.
  const rows = dialogue.preview.rows!;
  return <Paper p="xl" withBorder><Stack>
    <Title order={3}>2. 確認して取り込む</Title>
    <Text>{dialogue.preview.count}件を追加します。保存済みメモの件数は実行後に一覧へ反映されます。</Text>
    <Table withTableBorder><Table.Thead><Table.Tr><Table.Th>タイトル</Table.Th><Table.Th>本文</Table.Th></Table.Tr></Table.Thead><Table.Tbody>{rows.map((row, i) => <Table.Tr key={i}><Table.Td>{row.title}</Table.Td><Table.Td>{row.body}</Table.Td></Table.Tr>)}</Table.Tbody></Table>
    {mutation.isPending && <><Progress value={progress && progress.total ? 100 * progress.completed / progress.total : 0} animated /><Text size="sm">{progress?.phase ?? '開始しています…'}</Text></>}
    <ErrorNotice error={mutation.error} />
    {mutation.isError && <Text size="sm">中止と保存確定が重なった場合は、一覧で実際の保存結果を確認してください。</Text>}
    {dialogue.result && <Alert color="green">{dialogue.result.count}件を取り込みました。</Alert>}
    <Group justify="space-between"><Button component={Link} to="/import" variant="default" disabled={mutation.isPending}>入力に戻る</Button><Group>
      {mutation.isPending && <Button color="orange" variant="light" onClick={cancel}>中止を要求</Button>}
      {dialogue.result ? <Button component={Link} to="/notes">メモ一覧へ</Button> : <Button loading={mutation.isPending} onClick={() => void execute()}>取り込む</Button>}
    </Group></Group>
  </Stack></Paper>;
}
