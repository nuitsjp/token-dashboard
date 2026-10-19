import { useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Group, Stack } from '@mantine/core';
import { importStyleFolder } from '../../features/styles/catalog';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { reportFrontendError } from '../../features/application/queries';

export function AddStyle({ heading }: { heading: ReactNode }) {
  const client = useQueryClient();
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  async function pick() {
    if (busy) return;
    setBusy(true);
    try {
      await importStyleFolder();
      setError(null);
      await client.invalidateQueries({ queryKey: ['styles', 'catalog'] });
      await client.invalidateQueries({ queryKey: ['settings'] });
    } catch (failure) {
      if (publicError(failure).code === 'UNEXPECTED') reportFrontendError(failure);
      setError(failure);
    } finally { setBusy(false); }
  }
  return <Stack gap="sm">
    <Group justify="space-between" align="center">
      {heading}
      <Button loading={busy} onClick={() => void pick()}>Add style</Button>
    </Group>
    <ErrorNotice error={error} />
  </Stack>;
}
