import type { ReactNode } from 'react';
import { Alert, Badge, Button, Card, Group, PasswordInput, Select, Text, TextInput, Title } from '@mantine/core';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { automatic, useSettingsDraft } from './SettingsDraft';
import styles from './ConfigureHub.module.css';

function Row({ title, hint, children }: { title: ReactNode; hint?: string; children: ReactNode }) {
  return <div className={styles.row}>
    <div><Text component="div" fw={600}>{title}</Text>{hint && <Text size="xs" c="dimmed">{hint}</Text>}</div>
    <div>{children}</div>
  </div>;
}

function SaveBar({ scope }: { scope: 'connection' | 'display' }) {
  const draft = useSettingsDraft();
  if (!draft) return null;
  const dirty = scope === 'connection' ? draft.connectionDirty : draft.displayDirty;
  return <>
    <ErrorNotice error={draft.saveError} />
    {draft.done === scope && !dirty && <Alert color="green" py="xs" mt="sm">Saved.</Alert>}
    <Group justify="flex-end" mt="md" pt="md" className={styles.bar}>
      <Button loading={draft.saving} onClick={() => draft.submit(scope)}>Save</Button>
    </Group>
  </>;
}

export function ConnectionSettings() {
  const draft = useSettingsDraft();
  if (!draft) return null;
  const { fields, saved } = draft;
  return <Card withBorder padding="md">
    <Title order={4} mb="sm">Usage source</Title>
    <Row title="Data source" hint="Where usage is read from">
      <Select aria-label="Data source" data={['Local', 'Hub']} value={draft.source} allowDeselect={false} error={fields.source}
        onChange={value => { if (value) draft.setSource(value); }} />
    </Row>
    {draft.source === 'Hub' && <>
      <Title order={4} mt="lg" mb="sm">Hub connection</Title>
      <Row title="Hub URL" hint="Where to connect">
        <TextInput aria-label="Hub URL" placeholder="https://hub.example.com" value={draft.url} error={fields.url}
          onChange={e => draft.setURL(e.currentTarget.value)} />
      </Row>
      <Row title={<Group gap="xs" component="span">Access token<Badge size="sm" variant="light" color={saved.tokenSet ? 'green' : 'gray'}>{saved.tokenSet ? 'Set' : 'Not set'}</Badge></Group>}
        hint={saved.tokenSet ? 'Leave blank to keep the saved token.' : undefined}>
        <PasswordInput aria-label="Access token" value={draft.token} error={fields.token} autoComplete="off"
          onChange={e => draft.setToken(e.currentTarget.value)} />
      </Row>
    </>}
    <SaveBar scope="connection" />
  </Card>;
}

export function DisplaySettings() {
  const draft = useSettingsDraft();
  if (!draft) return null;
  const displays = draft.saved.displays ?? [];
  const options = [
    { value: automatic, label: 'Automatic' },
    ...displays.map(d => ({ value: d.deviceID, label: d.connected ? d.name : `${d.name} (Disconnected)` })),
  ];
  return <Card withBorder padding="md">
    <Title order={4} mb="sm">Output</Title>
    <Row title="Output device" hint={displays.length === 0 ? 'No TURZX display is connected.' : 'Where the usage is shown'}>
      <Select aria-label="Output device" data={options} value={draft.display} allowDeselect={false} error={draft.fields.displayID}
        onChange={value => { if (value) draft.setDisplay(value); }} />
    </Row>
    <SaveBar scope="display" />
  </Card>;
}
