import { createContext, useContext, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { View } from '@bindings/token-monitor-turzx/internal/settings/models';
import type { Presentation, ServiceContent } from '@bindings/token-monitor-turzx/internal/display/models';
import { getSettings, useSaveSettings } from '../../features/settings/queries';
import { ErrorNotice } from '../../shared/ErrorNotice';
import { publicError } from '../../shared/errors';
import { useDraftDirty } from '../../shared/ExitContext';

type Scope = 'connection' | 'display' | 'service-interval';
type DisplayChange = { display?: string; style?: string; cycle?: boolean; cycleSeconds?: number; serviceCycleSeconds?: number; awayMinutes?: number; presentation?: Presentation };
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
      const view = await save.mutateAsync({ source, url, token, displayID: saved.displayID, limitStyle: saved.limitStyle, cycle: saved.cycle, cycleSeconds: saved.cycleSeconds, serviceCycleSeconds: saved.serviceCycleSeconds, awayMinutes: saved.awayMinutes, presentation: saved.presentation });
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
        cycle: change.cycle ?? saved.cycle, cycleSeconds: change.cycleSeconds ?? saved.cycleSeconds,
        serviceCycleSeconds: change.serviceCycleSeconds ?? saved.serviceCycleSeconds,
        awayMinutes: change.awayMinutes ?? saved.awayMinutes,
        presentation: change.presentation ?? saved.presentation,
      });
    } catch { setFailed(change.serviceCycleSeconds === undefined ? 'display' : 'service-interval'); } finally { setPending({}); }
  }
  function setPresentation(change: Partial<Presentation>) {
    if (saved.presentation) void applyDisplay({ presentation: { ...structuredClone(saved.presentation), ...change } });
  }
  function setService(id: string, change: Partial<ServiceContent>) {
    if (!saved.presentation) return;
    const presentation = structuredClone(saved.presentation);
    presentation.services ??= {};
    const current = presentation.services[saved.source] ?? {};
    const value = current[id] ?? { enabled: true, content: 'Both' };
    current[id] = { ...value, ...change };
    presentation.services[saved.source] = current;
    void applyDisplay({ presentation });
  }
  return {
    saved, source, url, token, connectionDirty, done,
    display: pending.display ?? (saved.displayID || automatic),
    style: pending.style ?? (saved.limitStyle || 'Gauges'),
    cycle: pending.cycle ?? saved.cycle,
    cycleSeconds: saved.cycleSeconds,
    serviceCycleSeconds: saved.serviceCycleSeconds,
    awayMinutes: saved.awayMinutes,
    setSource: edit(setSource), setURL: edit(setURL), setToken: edit(setToken),
    setDisplay: (display: string) => void applyDisplay({ display }),
    setStyle: (style: string) => void applyDisplay({ style }),
    setCycle: (cycle: boolean) => void applyDisplay({ cycle }),
    setCycleSeconds: (cycleSeconds: number) => void applyDisplay({ cycleSeconds }),
    setServiceCycleSeconds: (serviceCycleSeconds: number) => void applyDisplay({ serviceCycleSeconds }),
    setAwayMinutes: (awayMinutes: number) => void applyDisplay({ awayMinutes }),
    setPresentation, setService,
    serviceLayout: saved.themes?.find(theme => theme.id === (pending.style ?? saved.limitStyle))?.dataLayout.startsWith('service-') ?? false,
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
