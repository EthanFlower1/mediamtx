/* eslint-disable react-refresh/only-export-components -- this module
   intentionally exports both the provider component and the useRuntimeContext
   hook from the same file. Splitting them would require circular imports since
   the hook depends on the private RuntimeContext created here. */
import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { useLocation } from 'react-router-dom';

// KAI-307: Runtime context detection.
//
// Two runtime contexts ship from one React codebase:
//   - "admin"   -> Customer Admin Web App (/admin/*)
//   - "command" -> Integrator Portal     (/command/*)
//
// Detection precedence (per spec):
//   1. URL path prefix (this file).
//   2. Auth token claim (added in KAI-308 auth ticket).
//   3. /api/v1/discover probe (added in KAI-309 discover ticket).
//
// This scaffold implements (1) only. Subsequent tickets will layer
// (2) and (3) onto this same hook.

export type RuntimeContextKind = 'admin' | 'command' | 'unknown';

export interface RuntimeContextValue {
  kind: RuntimeContextKind;
  basePath: '/admin' | '/command' | '/';
}

const RuntimeContext = createContext<RuntimeContextValue | null>(null);

function detectFromPath(pathname: string): RuntimeContextValue {
  if (pathname.startsWith('/command')) {
    return { kind: 'command', basePath: '/command' };
  }
  if (pathname.startsWith('/admin')) {
    return { kind: 'admin', basePath: '/admin' };
  }
  return { kind: 'unknown', basePath: '/' };
}

export function RuntimeContextProvider({ children }: { children: ReactNode }): JSX.Element {
  const location = useLocation();
  const value = useMemo(() => detectFromPath(location.pathname), [location.pathname]);
  return <RuntimeContext.Provider value={value}>{children}</RuntimeContext.Provider>;
}

export function useRuntimeContext(): RuntimeContextValue {
  const ctx = useContext(RuntimeContext);
  if (!ctx) {
    throw new Error('useRuntimeContext must be used inside <RuntimeContextProvider>');
  }
  return ctx;
}
