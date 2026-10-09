import { useEffect, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Card, Group, Image, Text, Title } from '@mantine/core';
import { getPreview, previewKey, subscribePreview } from '../../features/display/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';

// Shows the image the app sends to the TURZX. The window never draws it. The control sits right of
// the title and the children go above the image.
export function UsagePreview({ title, control, children }: { title: string; control?: ReactNode; children?: ReactNode }) {
  const client = useQueryClient();
  const preview = useQuery(getPreview());
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: previewKey })), [client]);
  return <Card withBorder padding="md">
    <Group gap="md" mb="sm"><Title order={4}>{title}</Title>{control}</Group>
    {children}
    <ErrorNotice error={preview.error} />
    {preview.data
      ? <Image src={preview.data} alt="Display preview" radius="sm" fit="contain" style={{ width: 'auto', maxWidth: '100%', maxHeight: 360, marginInline: 'auto' }} />
      : !preview.error && <Text size="sm" c="dimmed">No image yet.</Text>}
  </Card>;
}
