import { createRootRouteWithContext } from '@tanstack/react-router';
import type { QueryClient } from '@tanstack/react-query';
import { Shell } from '../app/Shell';
import { ErrorNotice } from '../shared/ErrorNotice';
export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: Shell,
  errorComponent: ({ error }) => <ErrorNotice error={error} />,
  notFoundComponent: () => <p>画面が見つかりません。ナビゲーションから開き直してください。</p>,
});
