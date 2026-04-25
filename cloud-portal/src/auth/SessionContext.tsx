import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ApiError, apiGet, apiPost, setUnauthenticatedHandler } from '../api/client';

export interface SessionUser {
  id: string;
  tenant_id: string;
  email: string;
  name: string;
  created_at: string;
}

export interface SessionTenant {
  id: string;
  name: string;
  email: string;
  created_at: string;
}

interface SessionResponse {
  user: SessionUser;
  tenant: SessionTenant;
}

interface SessionState {
  status: 'loading' | 'authenticated' | 'unauthenticated';
  user: SessionUser | null;
  tenant: SessionTenant | null;
  error: string | null;
}

interface SessionContextValue extends SessionState {
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  refresh: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>({ status: 'loading', user: null, tenant: null, error: null });

  const refresh = useCallback(async () => {
    try {
      const r = await apiGet<SessionResponse>('/api/v1/session', { skipAuthRetry: true });
      setState({ status: 'authenticated', user: r.user, tenant: r.tenant, error: null });
    } catch {
      setState({ status: 'unauthenticated', user: null, tenant: null, error: null });
    }
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    try {
      const r = await apiPost<SessionResponse>('/api/v1/login', { email, password }, { skipAuthRetry: true });
      setState({ status: 'authenticated', user: r.user, tenant: r.tenant, error: null });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'login failed';
      setState((s) => ({ ...s, status: 'unauthenticated', error: msg }));
      throw err;
    }
  }, []);

  const signOut = useCallback(async () => {
    try {
      await apiPost('/api/v1/logout', undefined, { skipAuthRetry: true });
    } finally {
      setState({ status: 'unauthenticated', user: null, tenant: null, error: null });
    }
  }, []);

  // Wire the api client's 401 handler to clear session state.
  useEffect(() => {
    setUnauthenticatedHandler(() => {
      setState({ status: 'unauthenticated', user: null, tenant: null, error: null });
    });
    return () => setUnauthenticatedHandler(null);
  }, []);

  // Probe the session on mount.
  useEffect(() => {
    refresh();
  }, [refresh]);

  const value = useMemo<SessionContextValue>(
    () => ({ ...state, signIn, signOut, refresh }),
    [state, signIn, signOut, refresh],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession must be used inside <SessionProvider>');
  return ctx;
}
