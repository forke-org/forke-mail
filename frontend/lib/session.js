import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { api, setApiHandlers } from './api';
import { useToast } from './toast';

const SessionContext = createContext(null);

export function SessionProvider({ children }) {
  const toast = useToast();
  const [state, setState] = useState({ status: 'loading', me: null });
  const [upgrade, setUpgrade] = useState(null);

  const refreshMe = useCallback(async () => {
    try {
      const me = await api('GET', '/api/me');
      setState({ status: 'signedIn', me });
      return me;
    } catch {
      setState({ status: 'signedOut', me: null });
      return null;
    }
  }, []);

  const login = useCallback(async (email, password) => {
    await api('POST', '/api/auth/login', { email, password });
    await refreshMe();
  }, [refreshMe]);

  const logout = useCallback(async () => {
    try { await api('POST', '/api/auth/logout'); } catch {}
    setState({ status: 'signedOut', me: null });
  }, []);

  useEffect(() => {
    setApiHandlers({
      unauthorized: () => setState({ status: 'signedOut', me: null }),
      rateLimited: (seconds) => toast(`Rate limited. Try again in ${seconds}s.`, 'warn'),
      quota: (body) => {
        setUpgrade(body || {});
        refreshMe();
      },
    });
    refreshMe();
  }, [refreshMe, toast]);

  const value = useMemo(() => {
    const me = state.me;
    const trial = me ? me.workspace.plan === 'trial' : false;
    const exhausted = trial && me.quota.trial_used >= me.quota.trial_limit;
    return {
      ...state, trial, exhausted, login, logout, refreshMe,
      upgrade, showUpgrade: (reason) => setUpgrade(reason || {}), hideUpgrade: () => setUpgrade(null),
    };
  }, [state, login, logout, refreshMe, upgrade]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  return useContext(SessionContext);
}
