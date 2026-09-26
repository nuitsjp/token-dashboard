import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Group, Modal, Paper, Progress, Stack, Text, Title } from '@mantine/core';
import { appInfo } from '../../features/application/queries';
import { useUpdates } from '../../features/updates/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';

export function UpdateApp() {
  const info = useQuery(appInfo());
  const update = useUpdates();
  const [confirm, setConfirm] = useState(false);
  const busy = update.check.isPending || update.download.isPending || update.apply.isPending;
  const status = update.status.data;
  async function apply() { try { await update.apply.mutateAsync(); } catch { setConfirm(false); } }
  return <Stack gap="lg">
    <div><Text size="sm" c="dimmed">共通機能 / 検証・引き渡し・再起動</Text><Title order={1}>アプリの更新</Title><Text c="dimmed" mt="sm">更新情報とファイルを検証してから、NSISへ適用を引き渡します。</Text></div>
    <Paper p="xl" withBorder><Stack>
      <Group justify="space-between"><Title order={3}>{info.data?.name ?? 'Wails Template'}</Title><Badge size="lg">v{info.data?.version ?? '—'}</Badge></Group>
      {!info.data?.updateConfigured && <Alert color="gray" title="配布先の設定が必要です">build/app.json の updateSource と updatePublicKey を設定してビルドしてください。更新元や秘密鍵はサンプルに含めていません。</Alert>}
      {info.data?.server && <Alert color="yellow">Server Buildでは署名の確認と取得までを検証できます。インストーラーは実行しません。</Alert>}
      <ErrorNotice error={update.check.error || update.download.error || update.apply.error || update.status.error} />
      {status?.available && <Alert color="blue" title={`v${status.version} を利用できます`}>{status.notes || '新版が公開されています。'}</Alert>}
      {status?.phase === 'checked' && !status.available && <Text>現在の版が最新です。</Text>}
      {(status?.phase === 'downloading' || status?.phase === 'ready') && <Progress value={status.total ? 100 * status.downloaded / status.total : 0} />}
      <Group><Button variant="light" disabled={busy || !info.data?.updateConfigured} loading={update.check.isPending} onClick={() => update.check.mutate()}>更新を確認</Button>
        {status?.available && status.phase !== 'ready' && <Button loading={update.download.isPending} disabled={update.check.isPending || update.apply.isPending} onClick={() => update.download.mutate()}>更新ファイルを取得</Button>}
        {update.download.isPending && <Button variant="default" onClick={update.cancel}>取得を中止</Button>}
        {status?.phase === 'ready' && <Button disabled={info.data?.server || busy} onClick={() => setConfirm(true)}>更新して再起動</Button>}
      </Group>
    </Stack></Paper>
    <Modal opened={confirm} onClose={() => setConfirm(false)} title="更新を適用しますか？" centered><Stack>
      <Text>アプリを終了して更新します。Windowsの警告が表示される場合があります。</Text>
      <Group justify="flex-end"><Button variant="default" disabled={update.apply.isPending} onClick={() => setConfirm(false)}>戻る</Button><Button loading={update.apply.isPending} onClick={() => void apply()}>適用して再起動</Button></Group>
    </Stack></Modal>
  </Stack>;
}
