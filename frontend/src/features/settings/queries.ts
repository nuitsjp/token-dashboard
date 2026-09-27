import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query';
import * as Settings from '@bindings/token-monitor-turzx/internal/settings/service';
import type { SaveRequest } from '@bindings/token-monitor-turzx/internal/settings/models';

export const settingsKey = ['settings'] as const;
export const getSettings = () => queryOptions({ queryKey: settingsKey, queryFn: () => Settings.Get() });
export function useSaveSettings() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (request: SaveRequest) => Settings.Save(request),
    onSuccess: (view) => { client.setQueryData(settingsKey, view); },
  });
}
