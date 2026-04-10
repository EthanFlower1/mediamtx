import { create } from 'zustand';

// KAI-320: Session store (minimal for dashboard wiring).
//
// Real auth session lands in KAI-223 / KAI-308. This ticket only needs
// the current tenant ID for TanStack Query key scoping and the display
// name for the dashboard page header. Providing a tiny, stable shape
// now lets the dashboard be refactored to the real session zero-touch.
export interface SessionState {
  tenantId: string;
  tenantName: string;
  userId: string;
  userDisplayName: string;
  /** Tenant entitlements keyed by feature slug. Real data lands with KAI-363. */
  entitlements: Record<string, boolean>;
  setTenant: (tenantId: string, tenantName: string) => void;
}

// Default mock session — replaced by real auth flow later.
export const useSessionStore = create<SessionState>((set) => ({
  tenantId: 'tenant-sample-001',
  tenantName: 'Sample Customer',
  userId: 'user-admin-001',
  userDisplayName: 'Admin User',
  entitlements: { 'ai.semantic_search': true },
  setTenant: (tenantId, tenantName) => set({ tenantId, tenantName }),
}));
