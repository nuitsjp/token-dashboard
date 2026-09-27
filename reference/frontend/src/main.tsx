import React from 'react';
import { createRoot } from 'react-dom/client';
import { MantineProvider, createTheme } from '@mantine/core';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import '@mantine/core/styles.css';
import { queryClient, router } from './app/router';
import { reportFrontendError } from './features/application/queries';
import './style.css';
const root = document.getElementById('root');
if (!root) throw new Error('Root element is missing');
const theme = createTheme({ primaryColor: 'blue', defaultRadius: 'md', fontFamily: 'system-ui, sans-serif' });
createRoot(root, { onUncaughtError: reportFrontendError, onCaughtError: reportFrontendError }).render(
  <React.StrictMode><MantineProvider theme={theme}><QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider></MantineProvider></React.StrictMode>,
);
window.addEventListener('unhandledrejection', (event) => reportFrontendError(event.reason));
