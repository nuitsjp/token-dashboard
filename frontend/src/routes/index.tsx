import { createFileRoute } from '@tanstack/react-router';
import { Group, Stack, Title } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { UsageLimitSelect } from '../usecases/show-usage/UsageLimitSelect';
import { DisplaySettings, StyleError, StyleSelect } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/')({ component: () => <Stack gap="md" style={{ flex: 1, minHeight: 0 }}><Group justify="space-between" align="flex-start"><Title order={2}>Display settings</Title><DisplaySettings /></Group><UsagePreview title="Style" control={<StyleSelect />}><StyleError /></UsagePreview><UsageLimitSelect /></Stack> });
