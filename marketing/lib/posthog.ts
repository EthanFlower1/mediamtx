'use client';

import posthog from 'posthog-js';

let initialized = false;

/**
 * Lazy PostHog initializer. Safe to call multiple times.
 * No-op when key is the REPLACE_ME placeholder so dev/test never reports.
 */
export function initPostHog(): typeof posthog | null {
  if (typeof window === 'undefined') return null;
  if (initialized) return posthog;

  const key = process.env.NEXT_PUBLIC_POSTHOG_KEY;
  if (!key || key === 'REPLACE_ME') {
    return null;
  }

  posthog.init(key, {
    api_host: process.env.NEXT_PUBLIC_POSTHOG_HOST || 'https://us.i.posthog.com',
    capture_pageview: true,
    person_profiles: 'identified_only'
  });
  initialized = true;
  return posthog;
}

export function capture(event: string, properties?: Record<string, unknown>): void {
  const ph = initPostHog();
  if (ph) ph.capture(event, properties);
}
