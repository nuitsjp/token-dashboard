import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Card, Group, PasswordInput, Select, SimpleGrid, Stack, Text, TextInput, Title } from '@mantine/core';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import { getSettings, useSaveSettings } from '../../features/settings/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useDraftDirty } from '../../shared/ExitContext';

const automatic = '__automatic__';

function Editor({ saved }: { saved: View }) {
  const save = useSaveSettings();
  const [source, setSource] = useState(saved.source || 'Local');
  const [url, setURL] = useState(saved.url);
  const [token, setToken] = useState('');
  const [display, setDisplay] = useState(saved.displayID || automatic);
  const [done, setDone] = useState(false);
  const dirty = source !== (saved.source || 'Local') || (source === 'Hub' && (url !== saved.url || token !== '')) || display !== (saved.displayID || automatic);
  useDraftDirty(dirty);
  const displays = saved.displays ?? [];
  const fields = save.error ? publicError(save.error).fieldErrors ?? {} : {};
  const options = [
    { value: automatic, label: 'Automatic' },
    ...displays.map(d => ({ value: d.deviceID, label: d.connected ? d.name : `${d.name} (Disconnected)` })),
  ];
  async function submit() {
    setDone(false);
    const view = await save.mutateAsync({ source, url, token, displayID: display === automatic ? '' : display });
    setSource(view.source || 'Local');
    setURL(view.url);
    setToken('');
    setDisplay(view.displayID || automatic);
    setDone(true);
  }
  return <Stack gap="sm">
    <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="xl">
      <Stack gap="sm">
        <Title order={5}>Usage source</Title>
        <Select label="Data source" data={['Local', 'Hub']} value={source} allowDeselect={false}
          onChange={value => { if (value) { setSource(value); setDone(false); } }} />
      </Stack>
      <Stack gap="sm">
        <Title order={5}>Display</Title>
        <Select label="Output device" data={options} value={display} allowDeselect={false} error={fields.displayID}
          onChange={value => { if (value) { setDisplay(value); setDone(false); } }} />
        {displays.length === 0 && <Text size="sm" c="dimmed">No TURZX display is connected.</Text>}
      </Stack>
    </SimpleGrid>
    {source === 'Hub' && <Stack gap="sm">
      <Title order={5}>Hub connection</Title>
      <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="xl">
        <TextInput label="Hub URL" placeholder="https://hub.example.com" value={url} error={fields.url}
          onChange={e => { setURL(e.currentTarget.value); setDone(false); }} />
        <PasswordInput label={<Group gap="xs" component="span">Access token<Badge size="sm" variant="light" color={saved.tokenSet ? 'green' : 'gray'}>{saved.tokenSet ? 'Set' : 'Not set'}</Badge></Group>}
          description={saved.tokenSet ? 'Leave blank to keep the saved token.' : undefined}
          value={token} error={fields.token} autoComplete="off"
          onChange={e => { setToken(e.currentTarget.value); setDone(false); }} />
      </SimpleGrid>
    </Stack>}
    <ErrorNotice error={save.error} />
    {done && !dirty && <Alert color="green" py="xs">Saved.</Alert>}
    <Group justify="flex-end"><Button loading={save.isPending} onClick={() => void submit().catch(() => {})}>Save</Button></Group>
  </Stack>;
}

export function ConfigureHub() {
  const settings = useQuery(getSettings());
  return <Card withBorder padding="md">
    <Title order={4} mb="sm">Settings</Title>
    <ErrorNotice error={settings.error} />
    {settings.data && <Editor saved={settings.data} />}
  </Card>;
}
