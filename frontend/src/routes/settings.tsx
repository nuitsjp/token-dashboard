import { createFileRoute } from '@tanstack/react-router';
import { Stack, Title } from '@mantine/core';
import { EditSettings } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/settings')({ component: () => <Stack gap="md"><Title order={2}>Settings</Title><EditSettings /></Stack> });
