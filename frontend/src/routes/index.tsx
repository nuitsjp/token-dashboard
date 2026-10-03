import { createFileRoute } from '@tanstack/react-router';
import { Stack, Title } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { UsageLimitSelect } from '../usecases/show-usage/UsageLimitSelect';
import { DisplaySettings, StyleError, StyleSelect } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/')({ component: () => <Stack gap="md"><Title order={2}>Display settings</Title><UsagePreview title="Style" control={<StyleSelect />}><StyleError /></UsagePreview><UsageLimitSelect /><DisplaySettings /></Stack> });
