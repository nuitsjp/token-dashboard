import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Application as RuntimeApplication, Events } from '@wailsio/runtime';
import * as Updates from '@bindings/token-monitor-turzx/internal/updates/service';
const key = ['updates', 'status'] as const;
export function useUpdates() {
  const client = useQueryClient();
  const status = useQuery({ queryKey: key, queryFn: () => Updates.GetStatus() });
  useEffect(() => Events.On('updates:progress', ({ data }) => {
    if (data && typeof data.phase === 'string') client.setQueryData(key, data);
  }), [client]);
  const apply = useMutation({ mutationKey: ['updates'], mutationFn: async () => {
    await Updates.Apply();
    await RuntimeApplication.Quit();
  } });
  return { status, apply };
}
