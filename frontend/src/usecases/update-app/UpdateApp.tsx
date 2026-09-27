import { Button, Card, Group, Text, Title } from '@mantine/core';
import { useUpdates } from '../../features/updates/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';

// Shown only while a verified installer of a newer version is staged.
export function UpdateApp() {
  const update = useUpdates();
  const status = update.status.data;
  if (!status?.available || (status.phase !== 'ready' && status.phase !== 'untrusted')) return null;
  const untrusted = status.phase === 'untrusted';
  return <Card withBorder padding="sm" px="md">
    <Group justify="space-between" wrap="nowrap">
      <Group gap="md" wrap="nowrap"><Title order={4}>Update</Title>
        {untrusted
          ? <Text c="red" role="alert">The update could not be verified. It will be checked again at next startup.</Text>
          : <Text>Version {status.version} is ready to install.</Text>}
      </Group>
      {!untrusted && <Button loading={update.apply.isPending} onClick={() => update.apply.mutate()}>Update and restart</Button>}
    </Group>
    {update.apply.error && publicError(update.apply.error).code !== 'UPDATE_UNTRUSTED' && <ErrorNotice error={update.apply.error} />}
  </Card>;
}
