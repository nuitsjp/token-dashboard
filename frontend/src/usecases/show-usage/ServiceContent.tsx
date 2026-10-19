import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Card, Group, SegmentedControl, Stack, Text } from '@mantine/core';
import { getServices, servicesKey, subscribePreview } from '../../features/display/queries';
import { useSettingsDraft } from '../configure-hub/SettingsDraft';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { SwitchButton } from '../../shared/SwitchButton';
import styles from './ServiceContent.module.css';

export function PresentationNotice() {
  const draft = useSettingsDraft();
  if (!draft) return null;
  return <>
    {draft.saved.outputWarning && <Alert color="yellow">{draft.saved.outputWarning}</Alert>}
  </>;
}

export function ServiceContent() {
  const draft = useSettingsDraft();
  const client = useQueryClient();
  const services = useQuery({ ...getServices(), enabled: draft?.serviceLayout === true });
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: servicesKey })), [client]);
  if (!draft?.serviceLayout || !draft.saved.presentation) return null;
  const presentation = draft.saved.presentation;
  return <Card withBorder padding="md" className={styles.section}>
    <Stack gap="md">
      <Group gap="xl">
        <Group gap="sm"><Text size="sm">Rotate 180°</Text><SwitchButton role="switch" label="Rotate 180°" checked={presentation.reverse180 ? 'true' : 'false'} disabled={draft.saving} onClick={() => draft.setPresentation({ reverse180: !presentation.reverse180 })} /></Group>
        <Group gap="sm"><Text size="sm">Skip full 5h or missing limits</Text><SwitchButton role="switch" label="Skip full 5h or missing limits" checked={presentation.skipFull5h ? 'true' : 'false'} disabled={draft.saving} onClick={() => draft.setPresentation({ skipFull5h: !presentation.skipFull5h })} /></Group>
      </Group>
      <ErrorNotice error={services.error} />
      <details>
        <summary className={styles.summary}>Service content</summary>
        {services.data?.length ? <div className={styles.grid}>{services.data.map(service => <article key={service.id} className={styles.card} data-enabled={service.enabled} aria-label={`${service.id} content`}>
          <Group justify="space-between" wrap="nowrap" gap="md">
            <Text fw={600} className={styles.name}>{service.id}</Text>
            <SwitchButton role="switch" label={`Show ${service.id}`} checked={service.enabled ? 'true' : 'false'} disabled={draft.saving} onClick={() => draft.setService(service.id, { enabled: !service.enabled })} />
          </Group>
          <SegmentedControl fullWidth aria-label={`${service.id} display content`} value={service.content} data={['Both', 'Limits', 'Tokens']} disabled={!service.enabled || draft.saving}
            onChange={content => draft.setService(service.id, { content })} />
        </article>)}</div> : <Text size="sm" c="dimmed" mt="sm">Waiting for services.</Text>}
      </details>
    </Stack>
  </Card>;
}
