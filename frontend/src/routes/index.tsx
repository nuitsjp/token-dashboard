import { createFileRoute } from '@tanstack/react-router';
import { Stack } from '@mantine/core';
import { UsagePreview } from '../usecases/show-usage/UsagePreview';
import { ConfigureHub } from '../usecases/configure-hub/ConfigureHub';
import { UpdateApp } from '../usecases/update-app/UpdateApp';
export const Route = createFileRoute('/')({ component: () => <Stack gap="xl"><UpdateApp /><UsagePreview /><ConfigureHub /></Stack> });
