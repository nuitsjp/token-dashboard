import { createFileRoute } from '@tanstack/react-router';
import { Stack } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { ConfigureHub } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/')({ component: () => <Stack gap="xl"><UsagePreview /><ConfigureHub /></Stack> });
