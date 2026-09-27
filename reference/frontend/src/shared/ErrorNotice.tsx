import { Alert } from '@mantine/core';
import { publicError } from './errors';
export function ErrorNotice({ error }: { error: unknown }) {
  if (!error) return null;
  const info = publicError(error);
  return <Alert color={info.code === 'CANCELLED' ? 'gray' : 'red'} title={info.code} role="alert">{info.message}</Alert>;
}
