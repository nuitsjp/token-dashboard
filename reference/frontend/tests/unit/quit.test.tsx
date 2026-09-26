import { act, cleanup, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { confirmQuit } from '../../src/features/application/queries';
import { useUpdates } from '../../src/features/updates/queries';

const calls = vi.hoisted(() => ({
  confirm: vi.fn(), apply: vi.fn(), quit: vi.fn(),
}));
vi.mock('@wailsio/runtime', () => ({
  Application: { Quit: calls.quit },
  Events: { On: () => () => {} },
}));
vi.mock('@bindings/wailstemplate/internal/desktop/service', () => ({
  ConfirmQuit: calls.confirm,
}));
vi.mock('@bindings/wailstemplate/internal/updates/service', () => ({
  Apply: calls.apply,
  GetStatus: async () => ({ phase: 'ready' }),
}));

beforeEach(() => {
  vi.resetAllMocks();
  calls.quit.mockResolvedValue(undefined);
});
afterEach(cleanup);

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  return { promise, resolve };
}

function renderUpdates() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderHook(useUpdates, {
    wrapper: ({ children }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>,
  });
}

it('waits for Go quit approval before requesting native quit', async () => {
  const approved = deferred();
  calls.confirm.mockReturnValue(approved.promise);
  const closing = confirmQuit();
  expect(calls.confirm).toHaveBeenCalledOnce();
  expect(calls.quit).not.toHaveBeenCalled();
  approved.resolve();
  await closing;
  expect(calls.quit).toHaveBeenCalledOnce();
});

it('keeps the application open when Go rejects quit', async () => {
  const failure = new Error('BUSY');
  calls.confirm.mockRejectedValue(failure);
  await expect(confirmQuit()).rejects.toBe(failure);
  expect(calls.quit).not.toHaveBeenCalled();
});

it('waits for the update handoff response before requesting native quit', async () => {
  const handedOff = deferred();
  const started = deferred();
  calls.apply.mockImplementation(() => { started.resolve(); return handedOff.promise; });
  const { result } = renderUpdates();
  await act(async () => {
    const applying = result.current.apply.mutateAsync();
    await started.promise;
    expect(calls.quit).not.toHaveBeenCalled();
    handedOff.resolve();
    await applying;
  });
  expect(calls.quit).toHaveBeenCalledOnce();
});

it('keeps the application open when the update handoff fails', async () => {
  const failure = new Error('installer launch failed');
  calls.apply.mockRejectedValue(failure);
  const { result } = renderUpdates();
  await act(async () => {
    await expect(result.current.apply.mutateAsync()).rejects.toBe(failure);
  });
  expect(calls.quit).not.toHaveBeenCalled();
});
