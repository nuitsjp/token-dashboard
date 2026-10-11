import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Card, Group, NumberInput, PasswordInput, Select, Stack, Text, TextInput, Title } from '@mantine/core';
import { getCycle, getCycleSeconds, useSetCycle, useSetCycleSeconds } from '../../features/display/queries';
import { SwitchButton } from '../../shared/SwitchButton';
import { ErrorNotice } from '../../shared/ErrorNotice';
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

// The style choice sits right of the Style title; the cycle switch and the time each page stays are
// at the right end. Every choice applies at once.
export function StyleSelect() {
  const draft = useSettingsDraft();
  const cycle = useQuery(getCycle());
  const seconds = useQuery(getCycleSeconds());
  const setCycle = useSetCycle();
  const setSeconds = useSetCycleSeconds();
  const [text, setText] = useState<number | string>('');
  useEffect(() => { if (seconds.data !== undefined) setText(seconds.data); }, [seconds.data]);
  if (!draft) return null;
  const commit = () => { if (typeof text === 'number' && text !== seconds.data) setSeconds.mutate(text, { onError: () => setText(seconds.data ?? '') }); else setText(seconds.data ?? ''); };
  return <Group gap="md" wrap="nowrap" justify="space-between" style={{ flex: 1 }}>
    <Select w={160} aria-label="Display style" data={['Gauges', 'Bars']} value={draft.style} allowDeselect={false} error={draft.fields.limitStyle}
      onChange={value => { if (value) draft.setStyle(value); }} />
    <Group gap="md" wrap="nowrap">
      <Group gap="xs" wrap="nowrap"><Text size="sm">Cycle</Text>
        <SwitchButton role="switch" label="Cycle" checked={cycle.data ? 'true' : 'false'} disabled={cycle.data === undefined || setCycle.isPending}
          onClick={() => setCycle.mutate(!cycle.data)} /></Group>
      <NumberInput w={110} aria-label="Cycle time" suffix=" s" min={5} max={3600} step={1} allowDecimal={false} clampBehavior="strict" hideControls styles={{ input: { textAlign: 'right' } }}
        value={text} onChange={setText} onBlur={commit} onKeyDown={e => { if (e.key === 'Enter') commit(); }} />
    </Group>
  </Group>;
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
  return <Stack gap={4} align="flex-end">
    <Group wrap="nowrap" gap="sm">
      <Title order={5}>Output</Title>
      <Select w={280} aria-label="Output device" data={options} value={draft.display} allowDeselect={false} error={draft.fields.displayID}
        onChange={value => { if (value) draft.setDisplay(value); }} />
    </Group>
    {displays.length === 0 && <Text size="xs" c="dimmed">No TURZX display is connected.</Text>}
  </Stack>;
}
