import { queryOptions } from '@tanstack/react-query';
import { Events } from '@wailsio/runtime';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';

export const previewKey = ['display', 'preview'] as const;
export const getPreview = () => queryOptions({ queryKey: previewKey, queryFn: () => Display.Preview() });
export const subscribePreview = (handler: () => void) => Events.On('display:updated', handler);
