import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Group, Modal, Stack, Text } from '@mantine/core';
import { deleteStyle, type StyleDefinition } from '../../features/styles/catalog';
import { ErrorNotice } from '../../shared/ErrorNotice';

export function DeleteStyle({ style, active = false }: { style: StyleDefinition; active?: boolean }) {
  const client = useQueryClient();
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  async function confirm() {
    if (busy) return;
    setBusy(true);
    try {
      await deleteStyle(style.id);
      setError(null);
      setAsking(false);
      await client.invalidateQueries({ queryKey: ['styles', 'catalog'] });
      await client.invalidateQueries({ queryKey: ['settings'] });
    } catch (failure) {
      setError(failure);
      setAsking(false);
    } finally { setBusy(false); }
  }
  return <>
    <Stack gap={4} align="flex-end"><Button variant="default" size="xs" color="red" disabled={active || busy} onClick={() => setAsking(true)} aria-label={`Delete ${style.name}`}>Delete</Button>
      {active && <Text size="xs" c="dimmed">Switch to another style before deleting.</Text>}</Stack>
    <Modal opened={asking} onClose={() => setAsking(false)} title="Delete this style?" centered>
      <Stack gap="md">
        <Text>{style.name}</Text>
        <Group justify="flex-end">
          <Button variant="default" disabled={busy} onClick={() => setAsking(false)}>Cancel</Button>
          <Button color="red" loading={busy} onClick={() => void confirm()}>Delete</Button>
        </Group>
      </Stack>
    </Modal>
    <ErrorNotice error={error} />
  </>;
}
