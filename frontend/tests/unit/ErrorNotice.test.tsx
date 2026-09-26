import { it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ErrorNotice } from '../../src/shared/ErrorNotice';
it('shows the public explanation accessibly', () => {
  render(<MantineProvider><ErrorNotice error={{ cause: { code: 'CONFLICT', message: '再取得してください。' } }} /></MantineProvider>);
  expect(screen.getByRole('alert')).toHaveTextContent('再取得してください。');
});
