import { useEffect, useState } from 'react';
import { Link, Outlet } from '@tanstack/react-router';
import { useIsMutating, useQuery } from '@tanstack/react-query';
import { Alert, Button, Group, Modal, NavLink, Stack, Text } from '@mantine/core';
import { appInfo, confirmQuit, ready, subscribeClose } from '../features/application/queries';
import { ErrorNotice } from '../shared/ErrorNotice';
import { ExitProvider, useExit } from '../shared/ExitContext';
import { SettingsDraftProvider } from '../usecases/configure-hub/SettingsDraft';
import { UpdateApp } from '../usecases/update-app/UpdateApp';
import styles from './Shell.module.css';

const icon = (path: string) => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden><path d={path} /></svg>;

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
    <nav className={styles.side} aria-label="Menu">
      <NavLink component={Link} to="/" label="Display" leftSection={icon('M3 4h18v12H3zM8 20h8M12 16v4')} activeOptions={{ exact: true }} />
      <NavLink component={Link} to="/styles" label="Styles" leftSection={icon('M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z')} />
      <NavLink component={Link} to="/connection" label="Connection" leftSection={icon('M3 4h18v6H3zM3 14h18v6H3zM7 7h.01M7 17h.01')} />
      <Text size="xs" c="dimmed" className={styles.version}>{info.data ? `Version ${info.data.version}` : ''}</Text>
    </nav>
    <main className={styles.main}>
      <Stack gap="md" className={styles.page}>
        <UpdateApp />
        <ErrorNotice error={info.error || error} />
        {info.data && !info.data.diagnosticsAvailable && <Alert color="yellow">Diagnostic logs cannot be saved. Check the access rights and free space of the data folder.</Alert>}
        <SettingsDraftProvider><Outlet /></SettingsDraftProvider>
      </Stack>
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
