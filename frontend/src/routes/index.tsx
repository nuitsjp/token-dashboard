import { createFileRoute } from '@tanstack/react-router';
import { Group, Stack, Title } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { UsageLimitSelect } from '../usecases/show-usage/UsageLimitSelect';
import { CompactSettings, DisplaySettings, StyleError, StyleSelect } from '../usecases/configure-hub/ConfigureHub';
import { useSettingsDraft } from '../usecases/configure-hub/SettingsDraft';
import styles from './Display.module.css';

function DisplayPage() {
  const draft = useSettingsDraft();
  return <Stack gap="md" className={styles.page} data-compact={draft?.compact || undefined}>
    <Group justify="space-between" align="flex-start"><Title order={2}>Display settings</Title><DisplaySettings /></Group>
    <UsagePreview title="Style" control={<StyleSelect />}><StyleError /></UsagePreview>
    <CompactSettings />
    <UsageLimitSelect compact={draft?.compact} />
  </Stack>;
}

export const Route = createFileRoute('/')({ component: DisplayPage });
