import { useEffect, useState, type ReactNode } from 'react';
import { Alert, Badge, Button, Card, Group, NumberInput, PasswordInput, Select, Stack, Switch, Text, TextInput, Title } from '@mantine/core';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { ServiceContentSelect } from '../show-usage/ServiceContentSelect';
import { automatic, useSettingsDraft } from './SettingsDraft';
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
    <ErrorNotice error={draft.errorFor('connection')} />
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
  const draft = useSettingsDraft();
  if (!draft) return null;
  return <Select w={160} aria-label="Display style" data={['Gauges', 'Bars']} value={draft.style} allowDeselect={false} error={draft.fields.limitStyle}
    onChange={value => { if (value) draft.setStyle(value); }} />;
}

export function StyleError() {
  const draft = useSettingsDraft();
  return draft ? <ErrorNotice error={draft.errorFor('display')} /> : null;
}

export function DisplaySettings() {
  const draft = useSettingsDraft();
  if (!draft) return null;
  const displays = draft.saved.displays ?? [];
  const options = [
    { value: automatic, label: 'Automatic' },
    ...displays.map(d => ({ value: d.deviceID, label: d.connected ? d.name : `${d.name} (Disconnected)` })),
  ];
  return <Stack gap={4} align="flex-end" w={draft.compact ? 350 : undefined} maw={draft.compact ? '100%' : undefined}>
    <Group wrap={draft.compact ? 'wrap' : 'nowrap'} justify={draft.compact ? 'flex-end' : undefined} gap="sm" maw={draft.compact ? '100%' : undefined}>
      <Title order={5}>Output</Title>
      <Select w={280} maw={draft.compact ? '100%' : undefined} aria-label="Output device" data={options} value={draft.display} allowDeselect={false} error={draft.fields.displayID}
        onChange={value => { if (value) draft.setDisplay(value); }} />
    </Group>
    {displays.length === 0 && <Text size="xs" c="dimmed">No TURZX display is connected.</Text>}
  </Stack>;
}

function RotationControls() {
  const draft = useSettingsDraft()!;
  const [interval, setInterval] = useState<string | number>(draft.interval);
  useEffect(() => setInterval(draft.interval), [draft.interval]);
  return <Card withBorder padding="md" className={styles.compact}>
    <Title order={4} mb="sm">3.5-inch display</Title>
    <div className={styles.rotation}>
      <Select label="Orientation" aria-label="Orientation" data={[
        { value: 'Landscape', label: 'Landscape' },
        { value: 'ReverseLandscape', label: 'Landscape (180°)' },
        { value: 'Portrait', label: 'Portrait' },
        { value: 'ReversePortrait', label: 'Portrait (180°)' },
      ]}
        value={draft.orientation} allowDeselect={false} disabled={draft.saving} error={draft.fields.orientation}
        onChange={value => { if (value) draft.setOrientation(value); }} />
      <NumberInput label="Rotation interval (seconds)" aria-label="Rotation interval (seconds)" value={interval}
        min={5} max={300} allowDecimal={false} allowNegative={false} clampBehavior="none" disabled={draft.saving}
        error={draft.fields.rotationIntervalSeconds} onChange={setInterval}
        onBlur={() => { if (interval !== draft.interval) draft.setInterval(typeof interval === 'number' ? interval : 0); }} />
    </div>
    <Switch label="Skip services with full 5h limits" checked={draft.skipFull} disabled={draft.saving}
      onChange={event => draft.setSkipFull(event.currentTarget.checked)} />
    <ServiceContentSelect />
  </Card>;
}

export function CompactSettings() {
  const draft = useSettingsDraft();
  return draft?.compact ? <RotationControls /> : null;
}
