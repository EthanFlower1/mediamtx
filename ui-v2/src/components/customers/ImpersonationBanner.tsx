import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { useImpersonationStore } from '@/stores/impersonation';
import { terminateSession } from '@/api/impersonation';

// KAI-467: Impersonation banner.
//
// Fixed banner shown at the top of the viewport during an active
// impersonation session. Displays:
//   - Who is being impersonated (tenant name)
//   - Countdown timer to auto-terminate
//   - "End session" button
//
// Accessibility:
//   - role="alert" so screen readers announce it immediately
//   - aria-live="polite" on the countdown so SR announce changes
//   - The end button is clearly labelled and keyboard-focusable

function formatCountdown(ms: number): string {
  if (ms <= 0) return '0:00';
  const totalSeconds = Math.ceil(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

export function ImpersonationBanner(): JSX.Element | null {
  const { t } = useTranslation();
  const session = useImpersonationStore((s) => s.session);
  const endSession = useImpersonationStore((s) => s.endSession);
  const [remaining, setRemaining] = useState(0);

  useEffect(() => {
    if (!session) return;
    const expiresAt = new Date(session.expiresAtIso).getTime();

    function tick() {
      setRemaining(Math.max(0, expiresAt - Date.now()));
    }

    tick();
    const interval = setInterval(tick, 1000);
    return () => clearInterval(interval);
  }, [session]);

  const terminateMutation = useMutation({
    mutationFn: () => terminateSession(session!.sessionId),
    onSuccess: () => {
      endSession();
    },
  });

  const handleEnd = useCallback(() => {
    terminateMutation.mutate();
  }, [terminateMutation]);

  if (!session) return null;

  return (
    <div
      role="alert"
      data-testid="impersonation-banner"
      className="sticky top-0 z-50 flex items-center justify-between gap-3 bg-amber-500 px-4 py-2 text-sm font-medium text-amber-950 shadow-md"
    >
      <div className="flex items-center gap-2">
        <span aria-hidden="true" className="text-base">
          !
        </span>
        <span data-testid="impersonation-banner-text">
          {t('impersonation.banner.text', {
            customerName: session.impersonatedTenantName,
          })}
        </span>
        <span
          aria-live="polite"
          aria-atomic="true"
          data-testid="impersonation-banner-countdown"
          className="ml-2 rounded bg-amber-600/30 px-2 py-0.5 font-mono text-xs"
        >
          {t('impersonation.banner.timeRemaining', {
            time: formatCountdown(remaining),
          })}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs opacity-80">
          {t('impersonation.banner.reason', { reason: session.reason })}
        </span>
        <button
          type="button"
          data-testid="impersonation-end-button"
          onClick={handleEnd}
          disabled={terminateMutation.isPending}
          className="rounded border border-amber-800 bg-amber-700 px-3 py-1 text-xs font-semibold text-white hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-amber-900 disabled:opacity-60"
        >
          {terminateMutation.isPending
            ? t('impersonation.banner.ending')
            : t('impersonation.banner.endSession')}
        </button>
      </div>
    </div>
  );
}
