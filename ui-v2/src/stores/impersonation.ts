import { create } from 'zustand';
import type { ImpersonationSessionDetail } from '@/api/impersonation';

// KAI-467: Impersonation session state.
//
// Tracks the active impersonation session for the current user.
// When `session` is non-null, the ImpersonationBanner renders at
// the top of the viewport, a countdown timer runs, and the "End
// session" action is available.
//
// The auto-terminate timeout fires client-side as a UX courtesy;
// the backend enforces the authoritative expiry independently.

export interface ImpersonationState {
  /** The active impersonation session, or null. */
  session: ImpersonationSessionDetail | null;

  /** Start an impersonation session (sets session + starts timer). */
  startSession: (session: ImpersonationSessionDetail) => void;

  /** End the current impersonation session. */
  endSession: () => void;

  /** Internal: the timeout ID for auto-terminate. */
  _timeoutId: ReturnType<typeof setTimeout> | null;
}

export const useImpersonationStore = create<ImpersonationState>((set, get) => ({
  session: null,
  _timeoutId: null,

  startSession: (session) => {
    // Clear any existing timeout.
    const prev = get()._timeoutId;
    if (prev) clearTimeout(prev);

    // Calculate remaining time until expiry.
    const expiresAt = new Date(session.expiresAtIso).getTime();
    const remaining = Math.max(0, expiresAt - Date.now());

    // Auto-terminate after timeout.
    const timeoutId = setTimeout(() => {
      set({ session: null, _timeoutId: null });
    }, remaining);

    set({ session, _timeoutId: timeoutId });
  },

  endSession: () => {
    const prev = get()._timeoutId;
    if (prev) clearTimeout(prev);
    set({ session: null, _timeoutId: null });
  },
}));
