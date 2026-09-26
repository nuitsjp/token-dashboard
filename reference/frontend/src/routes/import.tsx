import { createFileRoute } from '@tanstack/react-router';
import { ImportDialogue } from '../usecases/import-notes/ImportDialogue';
export const Route = createFileRoute('/import')({ component: ImportDialogue });
