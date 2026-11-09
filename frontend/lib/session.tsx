import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { api, setApiHandlers } from './api';
import { useToast } from './toast';
import type { Me } from './types';

type Upgrade = { required?: number; remaining?: number };

interface Session {
  status: 'loading' | 'signedIn' | 'signedOut';
  me: Me | null;
  trial: boolean;
  exhausted: boolean;
  superAdmin: boolean;
  canManage: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<Me | null>;
  upgrade: Upgrade | null;
  showUpgrade: (reason?: Upgrade) => void;
  hideUpgrade: () => void;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const toast = useToast();
  const [state, setState] = useState<{ status: Session['status']; me: Me | null }>({ status: 'loading', me: null });
  const [upgrade, setUpgrade] = useState<Upgrade | null>(null);

  const refreshMe = useCallback(async () => {
    try {
      const me = await api<Me>('GET', '/api/me');
      setState({ status: 'signedIn', me });
      return me;
    } catch {
      setState({ status: 'signedOut', me: null });
      return null;
    }
  }, []);

  const login = useCallback(async (email: string, password: string) => {
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

  const value = useMemo<Session>(() => {
    const me = state.me;
    const trial = me ? me.workspace.plan === 'trial' : false;
    const exhausted = Boolean(me && trial && me.quota.trial_used >= me.quota.trial_limit);
    const superAdmin = me?.user?.role === 'super_admin';
    const canManage = superAdmin || me?.key?.scope === 'full_access';
    return {
      ...state, trial, exhausted, superAdmin, canManage, login, logout, refreshMe,
      upgrade, showUpgrade: (reason) => setUpgrade(reason || {}), hideUpgrade: () => setUpgrade(null),
    };
  }, [state, login, logout, refreshMe, upgrade]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  return useContext(SessionContext) as Session;
}

export function useMe(): Me {
  return useSession().me as Me;
}
