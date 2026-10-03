import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Card, Checkbox, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { getLimits, limitsKey, subscribePreview, useSetShown } from '../../features/display/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';

// Chooses which contracts and windows are drawn. A choice applies at once, so the preview above shows
// the result. A contract is checked when all its windows are, and its box changes all of them.
export function UsageLimitSelect() {
  const client = useQueryClient();
  const limits = useQuery(getLimits());
  const setShown = useSetShown();
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: limitsKey })), [client]);
  const contracts = limits.data ?? [];
  return <Card withBorder padding="md">
    <Title order={4} mb="sm">Usage Limits</Title>
    <ErrorNotice error={limits.error ?? setShown.error} />
    {contracts.length === 0
      ? !limits.error && <Text size="sm" c="dimmed">Waiting for usage.</Text>
      : <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }} spacing="md">
        {contracts.map(contract => {
          const windows = contract.windows ?? [];
          const shown = windows.filter(w => w.shown).length;
          const label = `${contract.provider} ${contract.plan}`;
          return <Stack key={windows[0].key} gap={4}>
            <Checkbox fw={600} label={label} aria-label={label} checked={shown === windows.length}
              indeterminate={shown > 0 && shown < windows.length}
              onChange={e => setShown.mutate({ keys: windows.map(w => w.key), shown: e.currentTarget.checked })} />
            {windows.map(w => <Checkbox key={w.key} ml="lg" size="sm" aria-label={`${label} ${w.name}`}
              label={<>{w.name} <Text span size="xs" c="dimmed">{w.remainingPercent === null ? '—' : `${Math.round(w.remainingPercent)}%`}</Text></>}
              checked={w.shown} onChange={e => setShown.mutate({ keys: [w.key], shown: e.currentTarget.checked })} />)}
          </Stack>;
        })}
      </SimpleGrid>}
  </Card>;
}
