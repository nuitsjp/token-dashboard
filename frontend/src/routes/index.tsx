import { createFileRoute } from '@tanstack/react-router';
import { Stack, Title } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { DisplaySettings } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/')({ component: () => <Stack gap="md"><Title order={2}>Display settings</Title><UsagePreview /><DisplaySettings /></Stack> });
