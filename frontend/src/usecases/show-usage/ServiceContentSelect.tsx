import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Accordion, SegmentedControl, Stack, Switch, Text } from '@mantine/core';
import { getServices, servicesKey, subscribePreview, useSetServiceContent } from '../../features/display/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import styles from './ServiceContentSelect.module.css';

export function ServiceContentSelect() {
  const client = useQueryClient();
  const services = useQuery(getServices());
  const setContent = useSetServiceContent();
  useEffect(() => subscribePreview(() => void client.invalidateQueries({ queryKey: servicesKey })), [client]);
  return <Stack gap="xs" mt="lg" className={styles.root}>
    <ErrorNotice error={services.error ?? setContent.error} />
    <Accordion classNames={{ control: styles.control, content: styles.content }}>
      <Accordion.Item value="service-content">
        <Accordion.Control><Text component="span" fw={600}>Service content</Text></Accordion.Control>
        <Accordion.Panel>
          {(services.data?.length ?? 0) === 0 && !services.error && <Text size="sm" c="dimmed">Waiting for usage.</Text>}
          <div className={styles.grid}>
            {services.data?.map(service => {
              const enabled = service.enabled ?? (service.showLimits || service.showTokens);
              const value = service.showLimits ? service.showTokens ? 'Both' : 'Limits' : 'Tokens';
              return <div key={service.provider} className={styles.card} data-enabled={enabled}>
                <div className={styles.header}>
                  <Text size="sm" fw={600} className={styles.name}>{service.provider}</Text>
                  <Switch size="md" color="violet" onLabel="ON" offLabel="OFF" className={styles.toggle}
                    aria-label={`${service.provider} display enabled`} checked={enabled} disabled={setContent.isPending}
                    onChange={event => setContent.mutate({ provider: service.provider, enabled: event.currentTarget.checked,
                      showLimits: service.showLimits, showTokens: service.showTokens })} />
                </div>
                <SegmentedControl fullWidth size="sm" radius="md" color="violet"
                  aria-label={`${service.provider} display content`} data={['Both', 'Limits', 'Tokens']}
                  className={styles.contentChoice}
                  classNames={{ label: styles.choice }}
                  value={value} disabled={!enabled || setContent.isPending}
                  onChange={choice => setContent.mutate({ provider: service.provider, enabled,
                    showLimits: choice === 'Both' || choice === 'Limits', showTokens: choice === 'Both' || choice === 'Tokens' })} />
              </div>;
            })}
          </div>
        </Accordion.Panel>
      </Accordion.Item>
    </Accordion>
  </Stack>;
}
