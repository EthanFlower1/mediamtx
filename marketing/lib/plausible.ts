'use client';

import Plausible from 'plausible-tracker';

type PlausibleInstance = ReturnType<typeof Plausible>;
let instance: PlausibleInstance | null = null;

/**
 * Lazy Plausible initializer. No-op when domain is REPLACE_ME.
 */
export function getPlausible(): PlausibleInstance | null {
  if (typeof window === 'undefined') return null;
  if (instance) return instance;

  const domain = process.env.NEXT_PUBLIC_PLAUSIBLE_DOMAIN;
  if (!domain || domain === 'REPLACE_ME') {
    return null;
  }

  instance = Plausible({
    domain,
    trackLocalhost: false,
    apiHost: 'https://plausible.io'
  });
  instance.enableAutoPageviews();
  return instance;
}

export function trackEvent(name: string, props?: Record<string, string | number | boolean>): void {
  const p = getPlausible();
  if (p) p.trackEvent(name, { props });
}
