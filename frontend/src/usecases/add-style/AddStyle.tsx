import { useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Group, Stack } from '@mantine/core';
import { importStyleFolder } from '../../features/styles/catalog';
import { ErrorNotice } from '../../shared/ErrorNotice';

export function AddStyle({ heading }: { heading: ReactNode }) {
  const client = useQueryClient();
  const [error, setError] = useState<unknown>(null);
  async function pick() {
    try {
      await importStyleFolder();
      setError(null);
      await client.invalidateQueries({ queryKey: ['styles', 'catalog'] });
    } catch (failure) {
      setError(failure);
    }
  }
  return <Stack gap="sm">
    <Group justify="space-between" align="center">
      {heading}
      <Button onClick={() => void pick()}>Add style</Button>
    </Group>
    <ErrorNotice error={error} />
  </Stack>;
}
