import { createFileRoute } from '@tanstack/react-router';
import { Stack, Title } from '@mantine/core';
import { ConnectionSettings } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/connection')({ component: () => <Stack gap="md"><Title order={2}>Connection settings</Title><ConnectionSettings /></Stack> });
