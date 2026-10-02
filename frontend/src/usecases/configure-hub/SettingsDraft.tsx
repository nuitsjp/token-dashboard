import { createContext, useContext, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import { getSettings, useSaveSettings } from '../../features/settings/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useDraftDirty } from '../../shared/ExitContext';

type Scope = 'connection' | 'display';
export const automatic = '__automatic__';

function useDraft(saved: View) {
  const save = useSaveSettings();
  const [source, setSource] = useState(saved.source || 'Local');
  const [url, setURL] = useState(saved.url);
  const [token, setToken] = useState('');
  const [display, setDisplay] = useState(saved.displayID || automatic);
  const [done, setDone] = useState<Scope | null>(null);
  const connectionDirty = source !== (saved.source || 'Local') || (source === 'Hub' && (url !== saved.url || token !== ''));
  const displayDirty = display !== (saved.displayID || automatic);
  const dirty = connectionDirty || displayDirty;
  useDraftDirty(dirty);
  const edit = <T,>(set: (value: T) => void) => (value: T) => { set(value); setDone(null); };
  // Each page saves only its own fields. The other page's unsaved input is neither sent nor reset.
  async function submit(scope: Scope) {
    setDone(null);
    const connection = scope === 'connection';
    const view = await save.mutateAsync({
      source: connection ? source : saved.source || 'Local',
      url: connection ? url : saved.url,
      token: connection ? token : '',
      displayID: connection ? saved.displayID : display === automatic ? '' : display,
    });
    if (connection) {
      setSource(view.source || 'Local');
      setURL(view.url);
      setToken('');
    } else setDisplay(view.displayID || automatic);
    setDone(scope);
  }
  return {
    saved, source, url, token, display, connectionDirty, displayDirty, done,
    setSource: edit(setSource), setURL: edit(setURL), setToken: edit(setToken), setDisplay: edit(setDisplay),
    saving: save.isPending,
    saveError: save.error,
    fields: save.error ? publicError(save.error).fieldErrors ?? {} : {},
    submit: (scope: Scope) => void submit(scope).catch(() => {}),
  };
}

type Draft = ReturnType<typeof useDraft>;
const Context = createContext<Draft | null>(null);

function Provider({ saved, children }: { saved: View; children: ReactNode }) {
  return <Context.Provider value={useDraft(saved)}>{children}</Context.Provider>;
}

// The draft spans the Display and Connection pages; each page saves only its own fields.
export function SettingsDraftProvider({ children }: { children: ReactNode }) {
  const settings = useQuery(getSettings());
  if (settings.data) return <Provider saved={settings.data}>{children}</Provider>;
  return <Context.Provider value={null}><ErrorNotice error={settings.error} />{children}</Context.Provider>;
}

export const useSettingsDraft = () => useContext(Context);
