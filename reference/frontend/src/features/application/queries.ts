import { queryOptions } from '@tanstack/react-query';
import { Application as RuntimeApplication, Events } from '@wailsio/runtime';
import * as Application from '@bindings/wailstemplate/internal/desktop/service';
export const appInfo = () => queryOptions({ queryKey: ['app', 'info'], queryFn: () => Application.GetInfo(), staleTime: Infinity });
export const ready = () => Application.Ready();
export async function confirmQuit() {
  await Application.ConfirmQuit();
  await RuntimeApplication.Quit();
}
export const subscribeClose = (handler: () => void) => Events.On('app:close-requested', handler);
export function reportFrontendError(error: unknown) {
  const message = error instanceof Error ? `${error.name}: ${error.message}` : 'Unhandled frontend error';
  void Application.ReportFrontendError(message.slice(0, 2000)).catch(() => console.error('診断情報をGoへ送信できませんでした。'));
}
