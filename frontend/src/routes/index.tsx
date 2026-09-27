import { createFileRoute } from '@tanstack/react-router';
import { ConfigureHub } from '../usecases/configure-hub/ConfigureHub';
export const Route = createFileRoute('/')({ component: ConfigureHub });
