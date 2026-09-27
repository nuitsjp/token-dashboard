import { useEffect, useState } from 'react';
import { Outlet } from '@tanstack/react-router';
import { useIsMutating, useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Group, Modal, Stack, Text } from '@mantine/core';
import { appInfo, confirmQuit, ready, subscribeClose } from '../features/application/queries';
import { ErrorNotice } from '../shared/ErrorNotice';
import { ExitProvider, useExit } from '../shared/ExitContext';
import styles from './Shell.module.css';

function Content() {
  const info = useQuery(appInfo());
  const { dirty } = useExit();
  const busy = useIsMutating() > 0;
  const [closing, setClosing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    const off = subscribeClose(() => setClosing(true));
    void ready().catch(setError);
    return off;
  }, []);
  async function close() { try { await confirmQuit(); } catch (failure) { setError(failure); } }
  return <div className={styles.shell}>
    <header className={styles.header}>
      <Group gap="sm"><Text fw={700}>{info.data?.name ?? 'token-monitor-turzx'}</Text><Text size="xs" c="dimmed">v{info.data?.version ?? '—'}</Text></Group>
      {__MOCK__ && <Badge color="orange">Mock data</Badge>}
    </header>
    <main className={styles.main}>
      <ErrorNotice error={info.error || error} />
      {info.data && !info.data.diagnosticsAvailable && <Alert color="yellow" mb="lg">Diagnostic logs cannot be saved. Check the access rights and free space of the data folder.</Alert>}
      <Outlet />
    </main>
    <Modal opened={closing} onClose={() => setClosing(false)} title="Exit the application?" centered>
      <Stack>
        <Text>{busy ? 'An operation is in progress. Wait for it to finish before exiting.' : dirty ? 'Unsaved changes will be discarded.' : 'The application will exit.'}</Text>
        <ErrorNotice error={error} />
        <Group justify="flex-end"><Button variant="default" onClick={() => setClosing(false)}>Back</Button><Button disabled={busy} onClick={() => void close()}>Exit</Button></Group>
      </Stack>
    </Modal>
  </div>;
}
export function Shell() { return <ExitProvider><Content /></ExitProvider>; }
