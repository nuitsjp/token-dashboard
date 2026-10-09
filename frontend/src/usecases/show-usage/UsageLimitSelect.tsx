import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Card, Text, Title } from '@mantine/core';
import { getLimits, limitsKey, subscribePreview, useSetShown } from '../../features/display/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import styles from './UsageLimitSelect.module.css';

// The bar colours follow the colours of the drawn image: below 25% is bad, up to 40% is a warning.
const level = (percent: number) => percent < 25 ? 'bad' : percent <= 40 ? 'warn' : 'ok';

// Chooses which contracts and windows are drawn. A choice applies at once, so the preview above shows
// the result. A contract is on when all its windows are, and its switch changes all of them.
export function UsageLimitSelect({ compact = false }: { compact?: boolean }) {
  const client = useQueryClient();
  const limits = useQuery(getLimits());
  const setShown = useSetShown();
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: limitsKey })), [client]);
  const contracts = limits.data ?? [];
  return <Card withBorder padding="md" className={styles.panel} data-compact={compact || undefined}>
    <Title order={4} mb="sm">Usage Limits</Title>
    <ErrorNotice error={limits.error ?? setShown.error} />
    {contracts.length === 0
      ? !limits.error && <Text size="sm" c="dimmed">Waiting for usage.</Text>
      : <div className={styles.scroll}><div className={styles.grid}>
        {contracts.map(contract => {
          const windows = contract.windows ?? [];
          const shown = windows.filter(w => w.shown).length;
          const state = shown === 0 ? 'none' : shown === windows.length ? 'all' : 'some';
          const label = `${contract.provider} ${contract.plan}`;
          return <div key={windows[0].key} className={styles.card} data-state={state}>
            <div className={styles.head}>
              <div className={styles.title}><Text fw={600} size="sm">{contract.provider}</Text><Text size="xs" c="dimmed">{contract.plan}</Text></div>
              {!compact && <button type="button" role="checkbox" aria-label={label} className={styles.switch}
                aria-checked={state === 'all' ? 'true' : state === 'some' ? 'mixed' : 'false'}
                onClick={() => setShown.mutate({ keys: windows.map(w => w.key), shown: state !== 'all' })} />}
            </div>
            <div className={styles.chips}>
              {windows.map(w => <button key={w.key} type="button" role="checkbox" aria-label={`${label} ${w.name}`} aria-checked={w.shown}
                className={styles.chip} onClick={() => setShown.mutate({ keys: [w.key], shown: !w.shown })}>
                <span className={styles.dot} />
                <Text span size="sm">{w.name}</Text>
                <Text span size="xs" c="dimmed">{w.remainingPercent === null ? '—' : `${Math.round(w.remainingPercent)}%`}</Text>
                <span className={styles.track}><i className={styles.fill} data-level={w.remainingPercent === null ? 'ok' : level(w.remainingPercent)} style={{ width: `${Math.min(Math.max(w.remainingPercent ?? 0, 0), 100)}%` }} /></span>
              </button>)}
            </div>
          </div>;
        })}
      </div></div>}
  </Card>;
}
