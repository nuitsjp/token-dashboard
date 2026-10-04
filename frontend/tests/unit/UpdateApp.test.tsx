import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { UpdateApp } from '../../src/usecases/update-app/UpdateApp';

const calls = vi.hoisted(() => ({
  status: vi.fn(), apply: vi.fn(), quit: vi.fn(),
  listener: undefined as ((event: { data: unknown }) => void) | undefined,
}));
vi.mock('@wailsio/runtime', () => ({
  Application: { Quit: calls.quit },
  Events: { On: (_name: string, listener: (event: { data: unknown }) => void) => {
    calls.listener = listener;
    return () => { calls.listener = undefined; };
  } },
}));
vi.mock('@bindings/token-monitor-turzx/internal/updates/service', () => ({
  Apply: calls.apply,
  GetStatus: calls.status,
}));

const ready = { configured: true, available: true, version: '0.2.0', phase: 'ready', downloaded: 100, total: 100 };
const untrusted = {
  ...ready, phase: 'untrusted',
  applyError: { code: 'UPDATE_UNTRUSTED', message: 'Verification failed.' },
};
const warning = 'The update could not be verified. It will be checked again at next startup.';

beforeEach(() => {
  vi.resetAllMocks();
  calls.listener = undefined;
  calls.status.mockResolvedValue(ready);
});
afterEach(cleanup);

function renderUpdate() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MantineProvider><UpdateApp /></MantineProvider></QueryClientProvider>);
}

it('shows verification failure from a tray operation without a window mutation', async () => {
  renderUpdate();
  await screen.findByRole('button', { name: 'Update and restart' });
  act(() => { calls.listener?.({ data: untrusted }); });
  expect(await screen.findByRole('alert')).toHaveTextContent(warning);
  expect(screen.queryByRole('button', { name: 'Update and restart' })).not.toBeInTheDocument();
  expect(calls.apply).not.toHaveBeenCalled();
  expect(calls.quit).not.toHaveBeenCalled();
});

it('shows the same shared verification failure from a window operation', async () => {
  calls.apply.mockImplementation(async () => {
    calls.listener?.({ data: untrusted });
    throw { cause: untrusted.applyError };
  });
  renderUpdate();
  fireEvent.click(await screen.findByRole('button', { name: 'Update and restart' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(warning);
  expect(screen.queryByRole('button', { name: 'Update and restart' })).not.toBeInTheDocument();
  expect(calls.apply).toHaveBeenCalledOnce();
  expect(calls.quit).not.toHaveBeenCalled();
});

it('shows a saved launch error when the window opens and keeps retry available', async () => {
  calls.status.mockResolvedValue({ ...ready, applyError: { code: 'INTERNAL', message: 'The installer could not be started.' } });
  renderUpdate();
  expect(await screen.findByRole('alert')).toHaveTextContent('The installer could not be started.');
  expect(screen.getByRole('button', { name: 'Update and restart' })).toBeEnabled();
  expect(calls.apply).not.toHaveBeenCalled();
});

it.each([
  { ...ready, phase: 'checked', available: false },
  { ...ready, phase: 'failed' },
])('does not display an update for startup status $phase', async status => {
  calls.status.mockResolvedValue(status);
  await act(async () => { renderUpdate(); });
  expect(screen.queryByText('Update')).not.toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});
