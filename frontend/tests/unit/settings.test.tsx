import { act, cleanup, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import { settingsKey, useSaveConnection, useSetDisplay, useSetLimitStyle } from '../../src/features/settings/queries';
import { ExitProvider } from '../../src/shared/ExitContext';
import { SettingsDraftProvider, useSettingsDraft } from '../../src/usecases/configure-hub/SettingsDraft';
import { ConnectionSettings, DisplaySettings, StyleSelect } from '../../src/usecases/configure-hub/ConfigureHub';

const calls = vi.hoisted(() => ({ get: vi.fn(), displays: vi.fn(), connection: vi.fn(), display: vi.fn(), style: vi.fn() }));
vi.mock('@bindings/token-monitor-turzx/internal/settings/service', () => ({
  Get: calls.get, GetDisplays: calls.displays, SaveConnection: calls.connection, SetDisplay: calls.display, SetLimitStyle: calls.style,
}));

const initial: View = { source: 'Local', url: '', tokenSet: false, displayID: '', limitStyle: 'Gauges' };
const connected = { deviceID: 'usb-first', name: 'TURZX1.0 (633A6E01)', connected: true };
const clients: QueryClient[] = [];

function client() {
  const value = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false }, mutations: { retry: false } } });
  value.setQueryData(settingsKey, initial);
  clients.push(value);
  return value;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.resetAllMocks();
  calls.get.mockResolvedValue(initial);
  calls.displays.mockResolvedValue([]);
  calls.display.mockImplementation(async (value: string) => value);
  calls.style.mockImplementation(async (value: string) => value);
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach(value => value.clear());
  vi.unstubAllGlobals();
  Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView');
});

it('keeps changes to different settings when their responses arrive out of order', async () => {
  const queryClient = client();
  const connection = deferred<View>();
  const display = deferred<string>();
  const style = deferred<string>();
  calls.connection.mockReturnValue(connection.promise);
  calls.display.mockReturnValue(display.promise);
  calls.style.mockReturnValue(style.promise);
  const { result } = renderHook(() => ({ connection: useSaveConnection(), display: useSetDisplay(), style: useSetLimitStyle() }), {
    wrapper: ({ children }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
  });
  let savingConnection!: Promise<View>;
  let savingDisplay!: Promise<string>;
  let savingStyle!: Promise<string>;
  act(() => {
    savingConnection = result.current.connection.mutateAsync({ source: 'Hub', url: 'https://saved.example.com', token: 'secret' });
    savingDisplay = result.current.display.mutateAsync(connected.deviceID);
    savingStyle = result.current.style.mutateAsync('Bars');
  });
  await waitFor(() => expect(calls.connection).toHaveBeenCalledOnce());
  await act(async () => {
    style.resolve('Bars');
    display.resolve(connected.deviceID);
    await Promise.all([savingDisplay, savingStyle]);
  });
  // This reply was made before the display changes, but arrived after them.
  await act(async () => {
    connection.resolve({ ...initial, source: 'Hub', url: 'https://saved.example.com', tokenSet: true });
    await savingConnection;
  });
  expect(queryClient.getQueryData(settingsKey)).toEqual({ source: 'Hub', url: 'https://saved.example.com', tokenSet: true, displayID: connected.deviceID, limitStyle: 'Bars' });
  expect(calls.display).toHaveBeenCalledWith(connected.deviceID);
  expect(calls.style).toHaveBeenCalledWith('Bars');
});

it('keeps the unsaved connection through display saves and settings refetches', async () => {
  const queryClient = client();
  const { result } = renderHook(() => ({ draft: useSettingsDraft(), display: useSetDisplay(), style: useSetLimitStyle() }), {
    wrapper: ({ children }) => <QueryClientProvider client={queryClient}><ExitProvider><SettingsDraftProvider>{children}</SettingsDraftProvider></ExitProvider></QueryClientProvider>,
  });
  act(() => {
    result.current.draft!.setSource('Hub');
    result.current.draft!.setURL('https://unsaved.example.com');
    result.current.draft!.setToken('unsaved-token');
  });
  await act(async () => {
    await Promise.all([result.current.display.mutateAsync(connected.deviceID), result.current.style.mutateAsync('Bars')]);
  });
  calls.get.mockResolvedValue({ ...initial, displayID: connected.deviceID, limitStyle: 'Bars' });
  await act(async () => { await queryClient.invalidateQueries({ queryKey: settingsKey }); });
  await waitFor(() => expect(result.current.draft!.saved).toMatchObject({ source: 'Local', displayID: connected.deviceID, limitStyle: 'Bars' }));
  expect(result.current.draft).toMatchObject({ source: 'Hub', url: 'https://unsaved.example.com', token: 'unsaved-token', connectionDirty: true });
  expect(calls.connection).not.toHaveBeenCalled();
});

it('refreshes devices when the window gains focus and when the output list opens', async () => {
  const queryClient = client();
  queryClient.setQueryData(settingsKey, { ...initial, displayID: connected.deviceID });
  calls.displays.mockResolvedValue([connected]);
  render(<MantineProvider env="test"><QueryClientProvider client={queryClient}><ExitProvider><SettingsDraftProvider><DisplaySettings /></SettingsDraftProvider></ExitProvider></QueryClientProvider></MantineProvider>);
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Output device' })).toHaveValue(connected.name));
  calls.displays.mockResolvedValue([{ ...connected, connected: false }]);
  fireEvent(window, new Event('focus'));
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Output device' })).toHaveValue(`${connected.name} (Disconnected)`));
  const second = { deviceID: 'usb-second', name: 'TURZX1.0 (8F21C4D0)', connected: true };
  calls.displays.mockResolvedValue([{ ...connected, connected: false }, second]);
  fireEvent.click(screen.getByRole('textbox', { name: 'Output device' }));
  expect(await screen.findByRole('option', { name: second.name })).toBeVisible();
  expect(calls.connection).not.toHaveBeenCalled();
});

it('keeps pending display changes and their failures when navigating away and back', async () => {
  const queryClient = client();
  const display = deferred<string>();
  const style = deferred<string>();
  calls.display.mockReturnValue(display.promise);
  calls.style.mockReturnValue(style.promise);
  calls.displays.mockResolvedValue([connected]);
  const page = (showDisplay: boolean) => <MantineProvider env="test"><QueryClientProvider client={queryClient}><ExitProvider><SettingsDraftProvider>
    {showDisplay ? <><StyleSelect /><DisplaySettings /></> : <ConnectionSettings />}
  </SettingsDraftProvider></ExitProvider></QueryClientProvider></MantineProvider>;
  const view = render(page(true));
  fireEvent.click(screen.getByRole('textbox', { name: 'Display style' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Bars' }));
  fireEvent.click(screen.getByRole('textbox', { name: 'Output device' }));
  fireEvent.click(await screen.findByRole('option', { name: connected.name }));
  await waitFor(() => expect(queryClient.isMutating()).toBe(2));

  view.rerender(page(false));
  expect(screen.getByRole('textbox', { name: 'Data source' })).toBeVisible();
  view.rerender(page(true));
  expect(screen.getByRole('textbox', { name: 'Display style' })).toBeDisabled();
  expect(screen.getByRole('textbox', { name: 'Display style' })).toHaveValue('Bars');
  expect(screen.getByRole('textbox', { name: 'Output device' })).toBeDisabled();
  expect(screen.getByRole('textbox', { name: 'Output device' })).toHaveValue(connected.name);

  view.rerender(page(false));
  await act(async () => {
    style.reject({ code: 'STYLE_FAILED', message: 'The display style could not be saved.' });
    display.reject({ code: 'DISPLAY_FAILED', message: 'The output device could not be saved.' });
  });
  await waitFor(() => expect(queryClient.isMutating()).toBe(0));
  view.rerender(page(true));
  expect(screen.getByRole('textbox', { name: 'Display style' })).toHaveValue('Gauges');
  expect(screen.getByRole('textbox', { name: 'Output device' })).toHaveValue('Automatic');
  expect(screen.getByText('The display style could not be saved.')).toBeVisible();
  expect(screen.getByText('The output device could not be saved.')).toBeVisible();
});
