import { createFileRoute } from '@tanstack/react-router';
import { UpdateApp } from '../usecases/update-app/UpdateApp';
export const Route = createFileRoute('/updates')({ component: UpdateApp });
