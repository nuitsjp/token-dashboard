import { createContext, useContext, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import { getSettings, useSaveConnection, useSetDisplay, useSetLimitStyle } from '../../features/settings/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useDraftDirty } from '../../shared/ExitContext';

function useConnectionDraft(saved: View) {
  const save = useSaveConnection();
  const [source, setSource] = useState(saved.source);
  const [url, setURL] = useState(saved.url);
  const [token, setToken] = useState('');
  const [done, setDone] = useState(false);
  const connectionDirty = source !== saved.source || (source === 'Hub' && (url !== saved.url || token !== ''));
  useDraftDirty(connectionDirty);
  const edit = <T,>(set: (value: T) => void) => (value: T) => { set(value); setDone(false); };
  async function submitConnection() {
    setDone(false);
    try {
      const view = await save.mutateAsync({ source, url, token });
      setSource(current => current === source ? view.source : current);
      setURL(current => current === url ? view.url : current);
      setToken(current => current === token ? '' : current);
      setDone(true);
    } catch { /* Keep the input and show the mutation error. */ }
  }
  return {
    saved, source, url, token, connectionDirty, done,
    setSource: edit(setSource), setURL: edit(setURL), setToken: edit(setToken),
    saving: save.isPending,
    error: save.error,
    fields: save.error ? publicError(save.error).fieldErrors ?? {} : {},
    submitConnection: () => void submitConnection(),
  };
}

type Draft = ReturnType<typeof useConnectionDraft>;
const Context = createContext<Draft | null>(null);
type DisplayChanges = { display: ReturnType<typeof useSetDisplay>; style: ReturnType<typeof useSetLimitStyle> };
const DisplayContext = createContext<DisplayChanges | null>(null);

function Provider({ saved, children }: { saved: View; children: ReactNode }) {
  return <Context.Provider value={useConnectionDraft(saved)}>{children}</Context.Provider>;
}

// Only Connection has an unsaved draft, kept while moving between the two pages.
export function SettingsDraftProvider({ children }: { children: ReactNode }) {
  const settings = useQuery(getSettings());
  const display = useSetDisplay();
  const style = useSetLimitStyle();
  // Immediate saves also span the pages, so pending work and failures survive navigation.
  return <DisplayContext.Provider value={{ display, style }}>
    {settings.data
      ? <Provider saved={settings.data}>{children}</Provider>
      : <Context.Provider value={null}><ErrorNotice error={settings.error} />{children}</Context.Provider>}
  </DisplayContext.Provider>;
}

export const useSettingsDraft = () => useContext(Context);
export const useDisplayChanges = () => useContext(DisplayContext)!;
