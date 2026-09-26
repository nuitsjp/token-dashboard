import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';

type Value = { dirty: boolean; setDirty: (value: boolean) => void };
const Context = createContext<Value | null>(null);
export function ExitProvider({ children }: { children: ReactNode }) {
  const [dirty, setDirty] = useState(false);
  return <Context.Provider value={{ dirty, setDirty }}>{children}</Context.Provider>;
}
export function useExit() {
  const context = useContext(Context);
  if (!context) throw new Error('ExitProvider is missing');
  return context;
}
// One active product dialogue owns the draft at a time. No global data store.
export function useDraftDirty(dirty: boolean) {
  const { setDirty } = useExit();
  useEffect(() => { setDirty(dirty); return () => setDirty(false); }, [dirty, setDirty]);
}
