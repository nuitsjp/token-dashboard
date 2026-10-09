import { createContext, useContext, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import { getSettings, useSaveSettings } from '../../features/settings/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useDraftDirty } from '../../shared/ExitContext';

type Scope = 'connection' | 'display';
type DisplayChange = { display?: string; style?: string; orientation?: string; interval?: number; skipFull?: boolean };
export const automatic = '__automatic__';

function useDraft(saved: View) {
  const save = useSaveSettings();
  const [source, setSource] = useState(saved.source || 'Local');
  const [url, setURL] = useState(saved.url);
  const [token, setToken] = useState('');
  const [pending, setPending] = useState<DisplayChange>({});
  const [done, setDone] = useState(false);
  const [failed, setFailed] = useState<Scope | null>(null);
  const connectionDirty = source !== (saved.source || 'Local') || (source === 'Hub' && (url !== saved.url || token !== ''));
  useDraftDirty(connectionDirty);
  const edit = <T,>(set: (value: T) => void) => (value: T) => { set(value); setDone(false); };
  // The Connection page saves with its Save button. The other page's saved values are sent as they are.
  async function submitConnection() {
    setDone(false);
    setFailed(null);
    try {
      const view = await save.mutateAsync({ source, url, token, displayID: saved.displayID, limitStyle: saved.limitStyle });
      setSource(view.source || 'Local');
      setURL(view.url);
      setToken('');
      setDone(true);
    } catch { setFailed('connection'); }
  }
  // The Display page applies and saves a choice at once. A failed save leaves the saved value shown.
  async function applyDisplay(change: DisplayChange) {
    setPending(change);
    setFailed(null);
    try {
      await save.mutateAsync({
        source: saved.source || 'Local', url: saved.url, token: '',
        displayID: change.display === undefined ? saved.displayID : change.display === automatic ? '' : change.display,
        limitStyle: change.style ?? saved.limitStyle,
        orientation: change.orientation,
        rotationIntervalSeconds: change.interval,
        skipFull5hServices: change.skipFull,
      });
    } catch { setFailed('display'); } finally { setPending({}); }
  }
  return {
    saved, source, url, token, connectionDirty, done,
    display: pending.display ?? (saved.displayID || automatic),
    style: pending.style ?? (saved.limitStyle || 'Gauges'),
    orientation: pending.orientation ?? saved.orientation,
    interval: pending.interval ?? saved.rotationIntervalSeconds,
    skipFull: pending.skipFull ?? saved.skipFull5hServices,
    compact: /vid_1a86&pid_5722/i.test((pending.display ?? saved.displayID) === automatic || !(pending.display ?? saved.displayID)
      ? saved.displays?.find(d => d.connected)?.deviceID ?? '' : pending.display ?? saved.displayID),
    setSource: edit(setSource), setURL: edit(setURL), setToken: edit(setToken),
    setDisplay: (display: string) => void applyDisplay({ display }),
    setStyle: (style: string) => void applyDisplay({ style }),
    setOrientation: (orientation: string) => void applyDisplay({ orientation }),
    setInterval: (interval: number) => void applyDisplay({ interval }),
    setSkipFull: (skipFull: boolean) => void applyDisplay({ skipFull }),
    saving: save.isPending,
    errorFor: (scope: Scope) => failed === scope ? save.error : null,
    fields: save.error ? publicError(save.error).fieldErrors ?? {} : {},
    submitConnection: () => void submitConnection(),
  };
}

type Draft = ReturnType<typeof useDraft>;
const Context = createContext<Draft | null>(null);

function Provider({ saved, children }: { saved: View; children: ReactNode }) {
  return <Context.Provider value={useDraft(saved)}>{children}</Context.Provider>;
}

// The draft spans the Display and Connection pages. Display choices save at once; Connection saves with its button.
export function SettingsDraftProvider({ children }: { children: ReactNode }) {
  const settings = useQuery(getSettings());
  if (settings.data) return <Provider saved={settings.data}>{children}</Provider>;
  return <Context.Provider value={null}><ErrorNotice error={settings.error} />{children}</Context.Provider>;
}

export const useSettingsDraft = () => useContext(Context);
