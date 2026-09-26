import { createHashHistory, createRouter } from '@tanstack/react-router';
import { QueryClient } from '@tanstack/react-query';
import { routeTree } from '../routeTree.gen';
export const queryClient = new QueryClient({ defaultOptions: {
  queries: { networkMode: 'always', retry: false, refetchOnWindowFocus: false, refetchOnReconnect: false },
  mutations: { networkMode: 'always', retry: false },
} });
export const router = createRouter({ routeTree, history: createHashHistory(), context: { queryClient } });
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
