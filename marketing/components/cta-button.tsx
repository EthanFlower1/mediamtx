'use client';

import { useState, type ReactNode } from 'react';
import { useTranslations } from 'next-intl';
import { capture } from '../lib/posthog';
import { trackEvent } from '../lib/plausible';

type CtaIntent = 'trial' | 'demo' | 'partner';

interface CtaButtonProps {
  intent: CtaIntent;
  children: ReactNode;
}

/**
 * CTA button that POSTs to /api/lead (HubSpot stub) on click and fires
 * PostHog + Plausible events. Real lead-form modal lands in KAI-344.
 */
export function CtaButton({ intent, children }: CtaButtonProps) {
  const tCta = useTranslations('Cta');
  const [state, setState] = useState<'idle' | 'loading' | 'done'>('idle');

  async function handleClick() {
    setState('loading');
    capture('cta_click', { intent });
    trackEvent('cta_click', { intent });
    try {
      await fetch('/api/lead', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ intent, source: 'cta_button', ts: Date.now() })
      });
      setState('done');
    } catch {
      setState('idle');
    }
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={state === 'loading'}
      style={{
        background: 'var(--accent)',
        color: 'white',
        border: 'none',
        padding: '0.75rem 1.25rem',
        borderRadius: '0.375rem',
        cursor: 'pointer',
        fontWeight: 600
      }}
    >
      {state === 'loading' ? tCta('submitting') : state === 'done' ? tCta('thanks') : children}
    </button>
  );
}
