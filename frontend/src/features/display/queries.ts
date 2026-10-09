import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query';
import { Events } from '@wailsio/runtime';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';

export const previewKey = ['display', 'preview'] as const;
export const getPreview = () => queryOptions({ queryKey: previewKey, queryFn: () => Display.Preview() });
export const subscribePreview = (handler: () => void) => Events.On('display:updated', handler);

export const limitsKey = ['display', 'limits'] as const;
export const getLimits = () => queryOptions({ queryKey: limitsKey, queryFn: () => Display.Limits() });
export const servicesKey = ['display', 'services'] as const;
export const getServices = () => queryOptions({ queryKey: servicesKey, queryFn: () => Display.Services() });
export function useSetServiceContent() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ provider, enabled, showLimits, showTokens }: { provider: string; enabled: boolean; showLimits: boolean; showTokens: boolean }) => Display.SetServiceContent(provider, enabled, showLimits, showTokens),
    onSuccess: (services) => { client.setQueryData(servicesKey, services); },
  });
}
export function useSetShown() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ keys, shown }: { keys: string[]; shown: boolean }) => Display.SetShown(keys, shown),
    onSuccess: (contracts) => { client.setQueryData(limitsKey, contracts); },
  });
}
