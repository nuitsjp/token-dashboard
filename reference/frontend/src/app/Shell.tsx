import { useEffect, useState } from 'react';
import { Link, Outlet } from '@tanstack/react-router';
import { useIsMutating, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Badge, Button, Group, Modal, Stack, Text, Title } from '@mantine/core';
import { appInfo, confirmQuit, ready, subscribeClose } from '../features/application/queries';
import { subscribeNotes } from '../features/notes/queries';
import { ErrorNotice } from '../shared/ErrorNotice';
import { ExitProvider, useExit } from '../shared/ExitContext';
import styles from './Shell.module.css';

function Content() {
  const client = useQueryClient();
  const info = useQuery(appInfo());
  const { dirty } = useExit();
  const busy = useIsMutating() > 0;
  const [closing, setClosing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => subscribeNotes(client), [client]);
  useEffect(() => {
    const off = subscribeClose(() => setClosing(true));
    void ready().catch(setError);
    return off;
  }, []);
  async function close() { try { await confirmQuit(); } catch (failure) { setError(failure); } }
  return <div className={styles.shell}>
    <aside className={styles.sidebar}>
      <Text size="xs" fw={700} c="dimmed" className={styles.eyebrow}>WAILS TEMPLATE</Text>
      <Title order={3} mt="xs">ユースケースから<br />実装へ。</Title>
      <Text size="sm" c="dimmed" mt="md">対話はReact、機能はGo。境界を確認できる参照アプリ。</Text>
      <nav className={styles.navigation} aria-label="メインナビゲーション">
        <Link to="/notes" activeProps={{ className: styles.active }}>{'01　メモの編集'}</Link>
        <Link to="/import" activeProps={{ className: styles.active }}>{'02　一括取り込み'}</Link>
        <Link to="/updates" activeProps={{ className: styles.active }}>{'03　アプリの更新'}</Link>
      </nav>
      <div className={styles.footer}><Badge variant="light">{info.data?.server ? 'Server / 検証用' : 'Windows Desktop'}</Badge><Text size="xs" c="dimmed" mt="sm">v{info.data?.version ?? '—'}</Text></div>
    </aside>
    <main className={styles.main}>
      <Group justify="space-between" mb="xl"><Text size="sm" c="dimmed">REFERENCE IMPLEMENTATION</Text>{__MOCK__ && <Badge color="orange">試験用モック</Badge>}</Group>
      {__MOCK__ && <Alert color="orange" mb="lg">固定データによる試験用の再現です。保存・取り込みは実データへ反映されません。</Alert>}
      <ErrorNotice error={info.error || error} />
      {info.data && !info.data.diagnosticsAvailable && <Alert color="yellow" mb="lg">診断ログを保存できません。データ領域のアクセス権と空き容量を確認してください。</Alert>}
      <Outlet />
    </main>
    <Modal opened={closing} onClose={() => setClosing(false)} title="アプリを終了しますか？" centered>
      <Stack>
        <Text>{busy ? '処理中です。完了または中止を待ってから終了してください。' : dirty ? '未保存の入力内容は破棄されます。' : 'アプリを終了します。'}</Text>
        <ErrorNotice error={error} />
        <Group justify="flex-end"><Button variant="default" onClick={() => setClosing(false)}>戻る</Button><Button disabled={busy} onClick={() => void close()}>終了する</Button></Group>
      </Stack>
    </Modal>
  </div>;
}
export function Shell() { return <ExitProvider><Content /></ExitProvider>; }
