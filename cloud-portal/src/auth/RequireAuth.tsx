import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useSession } from './SessionContext';

export function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useSession();
  const location = useLocation();

  if (status === 'loading') {
    return (
      <div style={{
        flex: 1, height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
        background: 'var(--bg-primary)',
        fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 2, color: 'var(--text-muted)', textTransform: 'uppercase',
      }}>
        Connecting…
      </div>
    );
  }
  if (status === 'unauthenticated') {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <>{children}</>;
}
