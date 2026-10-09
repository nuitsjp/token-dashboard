import { createFileRoute } from '@tanstack/react-router';
import { Stack, Title } from '@mantine/core';
import { StyleList } from '../usecases/browse-styles/StyleList';
export const Route = createFileRoute('/styles')({ component: () => <Stack gap="md"><Title order={2}>Styles</Title><StyleList /></Stack> });
