import React from 'react';
import { createRoot } from 'react-dom/client';
import { MantineProvider, createTheme } from '@mantine/core';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import '@mantine/core/styles.css';
import { queryClient, router } from './app/router';
import { reportFrontendError } from './features/application/queries';
import { startThemeRenderer } from './features/display/theme-renderer';
import './style.css';
void startThemeRenderer().catch(reportFrontendError);
const root = document.getElementById('root');
if (!root) throw new Error('Root element is missing');
// Dark palette shared with multi-token-monitor: 7 is the page, 6 the card surface.
const theme = createTheme({
  primaryColor: 'violet',
  defaultRadius: 'lg',
  fontFamily: 'system-ui, -apple-system, "Segoe UI", "Yu Gothic UI", sans-serif',
  // The default violet with shade 8 (the dark scheme's filled shade) replaced by the app's accent.
  colors: { violet: ['#f3f0ff', '#e5dbff', '#d0bfff', '#b197fc', '#9775fa', '#845ef7', '#7950f2', '#7048e8', '#7466e0', '#5f3dc4'], dark: ['#e4e5e9', '#b4b6bf', '#8b8e99', '#5d6070', '#3a3d48', '#2c2e36', '#1f2126', '#16171b', '#111215', '#0b0c0e'] },
});
createRoot(root, { onUncaughtError: reportFrontendError, onCaughtError: reportFrontendError }).render(
  <React.StrictMode><MantineProvider theme={theme} forceColorScheme="dark"><QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider></MantineProvider></React.StrictMode>,
);
window.addEventListener('unhandledrejection', (event) => reportFrontendError(event.reason));
