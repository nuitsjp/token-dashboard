import { createFileRoute } from '@tanstack/react-router';
import { ImportInput } from '../usecases/import-notes/ImportInput';
export const Route = createFileRoute('/import/')({ component: ImportInput });
