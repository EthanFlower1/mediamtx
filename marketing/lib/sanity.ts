import { createClient, type SanityClient } from '@sanity/client';

/**
 * Sanity client for fetching marketing CMS content.
 *
 * NOTE: projectId is intentionally a placeholder. Provision a real Sanity
 * project (KAI-343 follow-up) and replace via NEXT_PUBLIC_SANITY_PROJECT_ID.
 * Do NOT commit a real projectId — load from env at runtime.
 */
export const sanityClient: SanityClient = createClient({
  projectId: process.env.NEXT_PUBLIC_SANITY_PROJECT_ID || 'REPLACE_ME',
  dataset: process.env.NEXT_PUBLIC_SANITY_DATASET || 'production',
  apiVersion: process.env.NEXT_PUBLIC_SANITY_API_VERSION || '2024-01-01',
  useCdn: true
});

/**
 * Stub fetcher used by pages until Sanity content models exist.
 * Returns null when projectId is not yet configured so pages can render
 * placeholder copy from i18n messages.
 */
export async function fetchSanityDocument<T = unknown>(
  query: string,
  params: Record<string, unknown> = {}
): Promise<T | null> {
  const projectId = process.env.NEXT_PUBLIC_SANITY_PROJECT_ID;
  if (!projectId || projectId === 'REPLACE_ME') {
    return null;
  }
  try {
    return await sanityClient.fetch<T>(query, params);
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[sanity] fetch failed', err);
    return null;
  }
}
