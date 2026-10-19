import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Card, Group, NumberInput, PasswordInput, Select, Stack, Text, TextInput, Title } from '@mantine/core';
import { SwitchButton } from '../../shared/SwitchButton';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { automatic, useSettingsDraft } from './SettingsDraft';
import { getSettings } from '../../features/settings/queries';
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
  const settings = useQuery({ ...getSettings(), enabled: !!draft && !draft.saving, refetchInterval: 2000, retry: false });
  if (!draft) return null;
  const themed = !!draft.saved.themes?.length;
  const compatible = new Set(draft.saved.compatibleThemeIDs ?? []);
  const choices = themed ? draft.saved.themes!.filter(theme => compatible.has(theme.id)).map(theme => ({ value: theme.id, label: theme.name })) : ['Gauges', 'Bars'];
  const value = !themed || compatible.has(draft.style) ? draft.style : null;
  return <Select w={themed ? 360 : 160} maw="100%" miw={0} classNames={{ input: styles.styleInput }} disabled={draft.saving || settings.isError || choices.length === 0} aria-label="Display style" data={choices} value={value} allowDeselect={false} error={draft.fields.limitStyle}
    placeholder={choices.length ? 'Select a compatible style' : 'No compatible styles'}
    onChange={value => { if (value) draft.setStyle(value); }} />;
}

// The 3.5-inch page interval stays next to the style and its preview.
export function ServicePageInterval() {
  const draft = useSettingsDraft();
  const [text, setText] = useState<number | string>('');
  const seconds = draft?.serviceCycleSeconds;
  const saving = draft?.saving;
  useEffect(() => { if (!saving && seconds !== undefined) setText(seconds); }, [seconds, saving]);
  if (!draft?.serviceLayout) return null;
  const commit = (raw: string) => {
    const value = raw.replace(/\s*s$/, '').trim();
    const seconds = /^\d+$/.test(value) ? Number(value) : NaN;
    if (!draft.saving && seconds >= 5 && seconds <= 3600 && seconds !== draft.serviceCycleSeconds) draft.setServiceCycleSeconds(seconds);
    else setText(draft.serviceCycleSeconds);
  };
  return <Stack gap="xs" mb="md">
    <Row title="Page interval" hint="Pages switch automatically when there is more than one page.">
      <NumberInput w={110} aria-label="Page interval" suffix=" s" min={5} max={3600} step={1} clampBehavior="none" hideControls disabled={draft.saving}
        styles={{ input: { textAlign: 'right' } }} error={draft.fields.serviceCycleSeconds}
        value={text} onChange={setText} onBlur={e => commit(e.currentTarget.value)} onKeyDown={e => { if (e.key === 'Enter') commit(e.currentTarget.value); }} />
    </Row>
    <ErrorNotice error={draft.errorFor('service-interval')} />
  </Stack>;
}

// Settings keeps the 9.2-inch cycle separate from the shared away timeout.
export function EditSettings() {
  const draft = useSettingsDraft();
  const [cycleText, setCycleText] = useState<number | string>('');
  const [awayText, setAwayText] = useState<number | string>('');
  const seconds = draft?.cycleSeconds;
  const minutes = draft?.awayMinutes;
  const saving = draft?.saving;
  useEffect(() => { if (!saving && seconds !== undefined) setCycleText(seconds); }, [seconds, saving]);
  useEffect(() => { if (!saving && minutes !== undefined) setAwayText(minutes); }, [minutes, saving]);
  if (!draft) return null;
  // The entry is read from the input itself, which has it before the page has taken the change. An
  // unchanged or invalid entry shows the saved value again; the server refuses a value out of range.
  const number = (raw: string) => /\d/.test(raw) ? Number(raw.replace(/\D/g, '')) : NaN;
  const commitCycle = (raw: string) => {
    const value = number(raw);
    if (value >= 5 && value <= 3600 && value !== draft.cycleSeconds) draft.setCycleSeconds(value);
    else setCycleText(draft.cycleSeconds);
  };
  const commitAway = (raw: string) => {
    const value = number(raw);
    if (value >= 1 && value <= 240 && value !== draft.awayMinutes) draft.setAwayMinutes(value);
    else setAwayText(draft.awayMinutes);
  };
  return <Stack gap="md">
    <Card component="section" aria-label="9.2-inch settings" withBorder padding="md">
      <Title order={4} mb="sm">9.2-inch only</Title>
      <Row title="Cycle" hint="Show the contracts that do not fit in turn">
        <Group gap="md" wrap="nowrap">
          <SwitchButton role="switch" label="Cycle" checked={draft.cycle ? 'true' : 'false'} disabled={draft.saving} onClick={() => draft.setCycle(!draft.cycle)} />
          <NumberInput w={110} aria-label="Cycle time" suffix=" s" min={5} max={3600} step={1} allowDecimal={false} clampBehavior="strict" hideControls disabled={draft.saving}
            styles={{ input: { textAlign: 'right' } }} error={draft.fields.cycleSeconds}
            value={cycleText} onChange={setCycleText} onBlur={e => commitCycle(e.currentTarget.value)} onKeyDown={e => { if (e.key === 'Enter') commitCycle(e.currentTarget.value); }} />
        </Group>
      </Row>
    </Card>
    <Card component="section" aria-label="Common settings" withBorder padding="md">
      <Title order={4} mb="sm">All displays</Title>
      <Row title="Away time" hint="No input for this long switches the TURZX off">
        <NumberInput w={110} aria-label="Away time" suffix=" min" min={1} max={240} step={1} allowDecimal={false} clampBehavior="strict" hideControls disabled={draft.saving}
          styles={{ input: { textAlign: 'right' } }} error={draft.fields.awayMinutes}
          value={awayText} onChange={setAwayText} onBlur={e => commitAway(e.currentTarget.value)} onKeyDown={e => { if (e.key === 'Enter') commitAway(e.currentTarget.value); }} />
      </Row>
    </Card>
    <ErrorNotice error={draft.errorFor('display')} />
  </Stack>;
}

export function StyleError() {
  const draft = useSettingsDraft();
  const settings = useQuery(getSettings());
  return draft ? <ErrorNotice error={draft.errorFor('display') ?? settings.error} /> : null;
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
    <Group wrap="wrap" gap="sm" maw="100%">
      <Title order={5}>Output</Title>
      <Select w={280} maw="100%" disabled={draft.saving} aria-label="Output device" data={options} value={draft.display} allowDeselect={false} error={draft.fields.displayID}
        onChange={value => { if (value) draft.setDisplay(value); }} />
    </Group>
    {displays.length === 0 && <Text size="xs" c="dimmed">No TURZX display is connected.</Text>}
  </Stack>;
}
