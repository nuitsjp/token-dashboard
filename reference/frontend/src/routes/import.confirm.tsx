import { createFileRoute } from '@tanstack/react-router';
import { ImportConfirm } from '../usecases/import-notes/ImportConfirm';
export const Route = createFileRoute('/import/confirm')({ component: ImportConfirm });
