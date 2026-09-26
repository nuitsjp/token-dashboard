import { createFileRoute } from '@tanstack/react-router';
import { EditNotes } from '../usecases/edit-notes/EditNotes';
export const Route = createFileRoute('/notes')({
  validateSearch: (value: Record<string, unknown>): { id?: string } => ({ id: typeof value.id === 'string' ? value.id : undefined }),
  component: Page,
});
function Page() { const { id } = Route.useSearch(); return <EditNotes selectedID={id} />; }
