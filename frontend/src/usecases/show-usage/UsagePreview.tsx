import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Card, Image, Text, Title } from '@mantine/core';
import { getPreview, previewKey, subscribePreview } from '../../features/display/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';

// Shows the image the app sends to the TURZX. The window never draws it.
export function UsagePreview() {
  const client = useQueryClient();
  const preview = useQuery(getPreview());
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: previewKey })), [client]);
  return <Card withBorder padding="lg">
    <Title order={4} mb="md">Preview</Title>
    <ErrorNotice error={preview.error} />
    {preview.data
      ? <Image src={preview.data} alt="Display preview" radius="sm" style={{ aspectRatio: '1920 / 462' }} />
      : !preview.error && <Text size="sm" c="dimmed">No image yet.</Text>}
  </Card>;
}
