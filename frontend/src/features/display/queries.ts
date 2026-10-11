import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query';
import { Events } from '@wailsio/runtime';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';

export const previewKey = ['display', 'preview'] as const;
export const getPreview = () => queryOptions({ queryKey: previewKey, queryFn: () => Display.Preview() });
export const subscribePreview = (handler: () => void) => Events.On('display:updated', handler);

export const limitsKey = ['display', 'limits'] as const;
export const getLimits = () => queryOptions({ queryKey: limitsKey, queryFn: () => Display.Limits() });
export function useSetShown() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ keys, shown }: { keys: string[]; shown: boolean }) => Display.SetShown(keys, shown),
    onSuccess: (contracts) => { client.setQueryData(limitsKey, contracts); },
  });
}

// Mock: the cycle switch is held in memory by the app until it is saved with the settings.
export const cycleKey = ['display', 'cycle'] as const;
export const getCycle = () => queryOptions({ queryKey: cycleKey, queryFn: () => Display.Cycle() });
export function useSetCycle() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (on: boolean) => Display.SetCycle(on),
    onSuccess: (on) => { client.setQueryData(cycleKey, on); },
  });
}

export const cycleSecondsKey = ['display', 'cycleSeconds'] as const;
export const getCycleSeconds = () => queryOptions({ queryKey: cycleSecondsKey, queryFn: () => Display.CycleSeconds() });
export function useSetCycleSeconds() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (seconds: number) => Display.SetCycleSeconds(seconds),
    onSuccess: (seconds) => { client.setQueryData(cycleSecondsKey, seconds); },
  });
}
