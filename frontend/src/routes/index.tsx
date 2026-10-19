import { createFileRoute } from '@tanstack/react-router';
import { Group, Stack, Title } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { UsageLimitSelect } from '../usecases/show-usage/UsageLimitSelect';
import { DisplaySettings, ServicePageInterval, StyleError, StyleSelect } from '../usecases/configure-hub/ConfigureHub';
import { PresentationNotice, ServiceContent } from '../usecases/show-usage/ServiceContent';
import { useSettingsDraft } from '../usecases/configure-hub/SettingsDraft';
function DisplayPage() {
  const draft = useSettingsDraft();
  return <Stack gap="md" style={{ flex: draft?.serviceLayout ? '0 0 auto' : 1, minHeight: 0 }}><Group justify="space-between" align="flex-start"><Title order={2}>Display settings</Title><DisplaySettings /></Group><PresentationNotice /><UsagePreview title="Style" control={<StyleSelect />}><ServicePageInterval /><StyleError /></UsagePreview><ServiceContent /><UsageLimitSelect /></Stack>;
}
export const Route = createFileRoute('/')({ component: DisplayPage });
