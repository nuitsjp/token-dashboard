import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query';
import { useRef } from 'react';
import { Events } from '@wailsio/runtime';
import * as Notes from '@notes-service';
import type { ImportRequest, SaveRequest } from '@bindings/wailstemplate/internal/notes/models';

export const notesKey = ['notes'] as const;
export const noteMutationKey = ['notes', 'write'] as const;
function withSignal<T>(call: Promise<T> & { cancel(): void }, signal: AbortSignal): Promise<T> {
  const abort = () => call.cancel();
  if (signal.aborted) abort();
  else signal.addEventListener('abort', abort, { once: true });
  return call.finally(() => signal.removeEventListener('abort', abort));
}
export const listNotes = () => queryOptions({ queryKey: [...notesKey, 'list'], queryFn: ({ signal }) => withSignal(Notes.List(), signal) });
export const getNote = (id: string) => queryOptions({ queryKey: [...notesKey, 'detail', id], queryFn: ({ signal }) => withSignal(Notes.Get(id), signal), enabled: !!id });

export function subscribeNotes(client: ReturnType<typeof useQueryClient>) {
  return Events.On('notes:changed', () => {
    // Local mutations invalidate on settlement. Avoid duplicate refreshes while
    // they are pending; external changes still refresh a displayed collection.
    if (!client.isMutating({ mutationKey: noteMutationKey })) void client.invalidateQueries({ queryKey: notesKey });
  });
}
export function useSaveNote() {
  const client = useQueryClient();
  return useMutation({
    mutationKey: noteMutationKey,
    mutationFn: (request: SaveRequest) => Notes.Save(request),
    onSuccess: (note) => { client.setQueryData(getNote(note.id).queryKey, note); },
    onSettled: () => { void client.invalidateQueries({ queryKey: notesKey }); },
  });
}
export function usePreviewImport() {
  return useMutation({ mutationFn: (csv: string) => Notes.PreviewImport(csv) });
}
export function useImportNotes() {
  const client = useQueryClient();
  const call = useRef<ReturnType<typeof Notes.Import> | null>(null);
  const mutation = useMutation({
    mutationKey: noteMutationKey,
    mutationFn: (request: ImportRequest) => {
      const pending = Notes.Import(request); call.current = pending;
      return pending.finally(() => { call.current = null; });
    },
    // A cancel request racing the commit does not prove that nothing was saved.
    onSettled: () => { void client.invalidateQueries({ queryKey: notesKey }); },
  });
  return { mutation, cancel: () => call.current?.cancel() };
}
export function subscribeImport(callback: (value: { operationID: string; completed: number; total: number; phase: string }) => void) {
  return Events.On('notes:import-progress', ({ data }) => {
    if (data && typeof data.operationID === 'string' && typeof data.phase === 'string') callback(data);
  });
}
