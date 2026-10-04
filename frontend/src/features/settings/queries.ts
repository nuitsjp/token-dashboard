import { useEffect } from 'react';
import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as Settings from '@bindings/token-monitor-turzx/internal/settings/service';
import type { ConnectionRequest, View } from '@bindings/token-monitor-turzx/internal/settings/models';

export const settingsKey = ['settings'] as const;
export const displaysKey = ['displays'] as const;
export const getSettings = () => queryOptions({ queryKey: settingsKey, queryFn: () => Settings.Get(), staleTime: Infinity });

export function useDisplays() {
  const client = useQueryClient();
  const displays = useQuery({ queryKey: displaysKey, queryFn: () => Settings.GetDisplays() });
  useEffect(() => {
    const refresh = () => { void client.invalidateQueries({ queryKey: displaysKey }); };
    window.addEventListener('focus', refresh);
    return () => window.removeEventListener('focus', refresh);
  }, [client]);
  return displays;
}

export function useSaveConnection() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (request: ConnectionRequest) => Settings.SaveConnection(request),
    onSuccess: (view) => {
      client.setQueryData<View>(settingsKey, saved => saved && ({
        ...saved, source: view.source, url: view.url, tokenSet: view.tokenSet,
      }));
    },
  });
}

export function useSetDisplay() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (display: string) => Settings.SetDisplay(display),
    onSuccess: (displayID) => {
      client.setQueryData<View>(settingsKey, saved => saved && ({ ...saved, displayID }));
      void client.invalidateQueries({ queryKey: displaysKey });
    },
  });
}

export function useSetLimitStyle() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (style: string) => Settings.SetLimitStyle(style),
    onSuccess: (limitStyle) => {
      client.setQueryData<View>(settingsKey, saved => saved && ({ ...saved, limitStyle }));
    },
  });
}
