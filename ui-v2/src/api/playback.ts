// KAI-323: Playback API — types, client interface, query-key factory,
// and feature-flag bootstrap.
//
// Split pattern (matches KAI-149 events API split):
//   - playback.ts      (this file) — types + PlaybackClient interface
//                                    + query-key factory + pure helpers
//                                    + registerPlaybackClient / reset*
//                                    + thin wrapper functions
//   - playback.mock.ts             — deterministic mock PlaybackClient
//
// Real WebRTC/HLS/RTSP streaming + clip export land with KAI-334. This
// file is the seam: pages, tests, and KAI-334 production impl all talk
// to the same PlaybackClient interface.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type MarkerSeverity = 'info' | 'warning' | 'critical';

export type MarkerKind =
  | 'person.detected'
  | 'vehicle.detected'
  | 'motion.detected'
  | 'line.crossed'
  | 'loitering'
  | 'tamper';

export interface PlaybackMarker {
  id: string;
  kind: MarkerKind;
  severity: MarkerSeverity;
  /** Seconds since windowStart (0..windowDurationSec). */
  atSec: number;
  label: string;
}

export interface PlaybackEvent {
  id: string;
  tenantId: string;
  cameraId: string;
  cameraName: string;
  /** ISO-8601 start of the 24-hour playback window. */
  windowStart: string;
  /** Duration of the playback window in seconds. Default 86_400. */
  windowDurationSec: number;
  /** Seed timestamp (seconds since windowStart) the scrubber opens on. */
  seedAtSec: number;
  posterUrl: string | null;
}

export interface ExportClipArgs {
  tenantId: string;
  eventId: string;
  startSec: number;
  durationSec: number;
}

export interface ExportClipResult {
  clipId: string;
  downloadUrl: string;
  expiresAt: string;
}

export interface ListMarkersArgs {
  tenantId: string;
  eventId: string;
  startSec: number;
  endSec: number;
}

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface PlaybackClient {
  getEvent(tenantId: string, eventId: string): Promise<PlaybackEvent>;
  listMarkers(args: ListMarkersArgs): Promise<PlaybackMarker[]>;
  exportClip(args: ExportClipArgs): Promise<ExportClipResult>;
}

// ---------------------------------------------------------------------------
// Query key factory (tenant-scoped)
// ---------------------------------------------------------------------------

export const playbackQueryKeys = {
  all: (tenantId: string) => ['playback', tenantId] as const,
  event: (tenantId: string, eventId: string) =>
    ['playback', tenantId, 'event', eventId] as const,
  markers: (
    tenantId: string,
    eventId: string,
    startSec: number,
    endSec: number,
  ) => ['playback', tenantId, 'event', eventId, 'markers', startSec, endSec] as const,
};

// ---------------------------------------------------------------------------
// Pure helpers (exported for reuse in components + tests)
// ---------------------------------------------------------------------------

/** Format seconds (0..86399) as HH:MM:SS for the timeline tick labels. */
export function formatClockSec(totalSec: number): string {
  const s = Math.max(0, Math.floor(totalSec));
  const hh = Math.floor(s / 3600).toString().padStart(2, '0');
  const mm = Math.floor((s % 3600) / 60).toString().padStart(2, '0');
  const ss = (s % 60).toString().padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

/** Clamp atSec into [0, windowDurationSec]. */
export function clampSeek(atSec: number, windowDurationSec: number): number {
  if (Number.isNaN(atSec)) return 0;
  if (atSec < 0) return 0;
  if (atSec > windowDurationSec) return windowDurationSec;
  return atSec;
}

// ---------------------------------------------------------------------------
// Feature-flag bootstrap
// ---------------------------------------------------------------------------

// Module-level client ref. Defaults to the deterministic mock until
// KAI-334 wires in the real Connect-Go backed implementation.
let activeClient: PlaybackClient | null = null;

/** Register the production PlaybackClient (called from KAI-334 bootstrap). */
export function registerPlaybackClient(client: PlaybackClient): void {
  activeClient = client;
}

/** Reset back to the mock. Used from test setup to isolate tests. */
export function resetPlaybackClientForTests(): void {
  activeClient = null;
}

/** Lazily load the mock so test/dev bundles don't pay for it until used. */
async function getClient(): Promise<PlaybackClient> {
  if (activeClient) return activeClient;
  const mod = await import('./playback.mock');
  activeClient = mod.createMockPlaybackClient();
  return activeClient;
}

// ---------------------------------------------------------------------------
// Thin wrapper functions — what pages + tests actually import
// ---------------------------------------------------------------------------

export async function getPlaybackEvent(
  tenantId: string,
  eventId: string,
): Promise<PlaybackEvent> {
  const client = await getClient();
  return client.getEvent(tenantId, eventId);
}

export async function listMarkersForWindow(
  args: ListMarkersArgs,
): Promise<PlaybackMarker[]> {
  const client = await getClient();
  return client.listMarkers(args);
}

export async function exportClip(
  args: ExportClipArgs,
): Promise<ExportClipResult> {
  const client = await getClient();
  return client.exportClip(args);
}
