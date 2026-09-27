import { Alert, Button, Card, Group, Text, Title } from '@mantine/core';
import { useUpdates } from '../../features/updates/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';

// Shown only while a verified installer of a newer version is staged.
export function UpdateApp() {
  const update = useUpdates();
  const status = update.status.data;
  if (!status?.available || (status.phase !== 'ready' && status.phase !== 'untrusted')) return null;
  const untrusted = status.phase === 'untrusted';
  return <Card withBorder padding="lg">
    <Title order={4} mb="md">Update</Title>
    {untrusted
      ? <Alert color="red" role="alert">The update could not be verified. It will be checked again at next startup.</Alert>
      : <>
        <Text mb="md">Version {status.version} is ready to install.</Text>
        {update.apply.error && publicError(update.apply.error).code !== 'UPDATE_UNTRUSTED' && <ErrorNotice error={update.apply.error} />}
        <Group><Button loading={update.apply.isPending} onClick={() => update.apply.mutate()}>Update and restart</Button></Group>
      </>}
  </Card>;
}
