import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Card, Group, PasswordInput, Select, Stack, Text, TextInput, Title } from '@mantine/core';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { getSettings, useDisplays } from '../../features/settings/queries';
import { publicError } from '../../shared/errors';
import { useDisplayChanges, useSettingsDraft } from './SettingsDraft';
import styles from './ConfigureHub.module.css';

function Row({ title, hint, children }: { title: ReactNode; hint?: string; children: ReactNode }) {
  return <div className={styles.row}>
    <div><Text component="div" fw={600}>{title}</Text>{hint && <Text size="xs" c="dimmed">{hint}</Text>}</div>
    <div>{children}</div>
  </div>;
}

function SaveBar() {
  const draft = useSettingsDraft();
  if (!draft) return null;
  return <>
    <ErrorNotice error={draft.error} />
    {draft.done && !draft.connectionDirty && <Alert color="green" py="xs" mt="sm">Saved.</Alert>}
    <Group justify="flex-end" mt="md" pt="md" className={styles.bar}>
      <Button loading={draft.saving} onClick={draft.submitConnection}>Save</Button>
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
    <SaveBar />
  </Card>;
}

// The style choice sits right of the Style title. It applies and saves at once.
export function StyleSelect() {
  const settings = useQuery(getSettings());
  const { style } = useDisplayChanges();
  if (!settings.data) return null;
  return <Stack gap={4}>
    <Select w={160} aria-label="Display style" data={['Gauges', 'Bars']} value={style.isPending ? style.variables : settings.data.limitStyle}
      allowDeselect={false} disabled={style.isPending} error={style.error ? publicError(style.error).fieldErrors?.limitStyle : undefined}
      onChange={value => { if (value) style.mutate(value); }} />
    <ErrorNotice error={style.error} />
  </Stack>;
}

export function DisplaySettings() {
  const settings = useQuery(getSettings());
  const devices = useDisplays();
  const { display } = useDisplayChanges();
  if (!settings.data) return null;
  const automatic = '__automatic__';
  const displays = devices.data ?? [];
  const options = [
    { value: automatic, label: 'Automatic' },
    ...displays.map(d => ({ value: d.deviceID, label: d.connected ? d.name : `${d.name} (Disconnected)` })),
  ];
  return <Stack gap={4} align="flex-end">
    <Group wrap="nowrap" gap="sm">
      <Title order={5}>Output</Title>
      <Select w={280} aria-label="Output device" data={options} value={(display.isPending ? display.variables : settings.data.displayID) || automatic}
        allowDeselect={false} disabled={display.isPending} error={display.error ? publicError(display.error).fieldErrors?.displayID : undefined}
        onDropdownOpen={() => { void devices.refetch(); }}
        onChange={value => { if (value) display.mutate(value === automatic ? '' : value); }} />
    </Group>
    <ErrorNotice error={devices.error ?? display.error} />
    {devices.data && displays.length === 0 && <Text size="xs" c="dimmed">No TURZX display is connected.</Text>}
  </Stack>;
}
