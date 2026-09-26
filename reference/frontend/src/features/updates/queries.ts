import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Application as RuntimeApplication, Events } from '@wailsio/runtime';
import * as Updates from '@bindings/wailstemplate/internal/updates/service';
const key = ['updates', 'status'] as const;
export function useUpdates() {
  const client = useQueryClient();
  const active = useRef<ReturnType<typeof Updates.Download> | null>(null);
  const status = useQuery({ queryKey: key, queryFn: () => Updates.GetStatus() });
  useEffect(() => Events.On('updates:progress', ({ data }) => {
    if (data && typeof data.phase === 'string') client.setQueryData(key, data);
  }), [client]);
  const check = useMutation({ mutationKey: ['updates'], mutationFn: () => Updates.Check(), onSuccess: value => { client.setQueryData(key, value); } });
  const download = useMutation({ mutationKey: ['updates'], mutationFn: () => {
    const call = Updates.Download(); active.current = call;
    return call.finally(() => { active.current = null; });
  }, onSuccess: value => { client.setQueryData(key, value); } });
  const apply = useMutation({ mutationKey: ['updates'], mutationFn: async () => {
    await Updates.Apply();
    await RuntimeApplication.Quit();
  } });
  return { status, check, download, apply, cancel: () => active.current?.cancel() };
}
