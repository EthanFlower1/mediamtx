// KAI-323: Deterministic mock PlaybackClient.
//
// Every method returns promise-resolved data seeded from the tenantId +
// eventId so tests are stable across runs. Replaced wholesale by the real
// Connect-Go backed client in KAI-334.

import type {
  ExportClipArgs,
  ExportClipResult,
  ListMarkersArgs,
  MarkerKind,
  MarkerSeverity,
  PlaybackClient,
  PlaybackEvent,
  PlaybackMarker,
} from './playback';

const WINDOW_DURATION_SEC = 24 * 60 * 60; // 24h

const MARKER_KINDS: MarkerKind[] = [
  'person.detected',
  'vehicle.detected',
  'motion.detected',
  'line.crossed',
  'loitering',
  'tamper',
];

const SEVERITY_FOR_KIND: Record<MarkerKind, MarkerSeverity> = {
  'person.detected': 'info',
  'vehicle.detected': 'info',
  'motion.detected': 'info',
  'line.crossed': 'warning',
  loitering: 'warning',
  tamper: 'critical',
};

function pseudoIndex(seed: string): number {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

function buildMarkers(tenantId: string, eventId: string): PlaybackMarker[] {
  const seed = pseudoIndex(`${tenantId}:${eventId}`);
  const count = 8 + (seed % 6); // 8..13 markers
  const out: PlaybackMarker[] = [];
  for (let i = 0; i < count; i++) {
    const kind = MARKER_KINDS[(seed + i) % MARKER_KINDS.length]!;
    const atSec = ((seed * (i + 1) * 733) % WINDOW_DURATION_SEC);
    out.push({
      id: `${eventId}-marker-${i}`,
      kind,
      severity: SEVERITY_FOR_KIND[kind],
      atSec,
      label: kind,
    });
  }
  out.sort((a, b) => a.atSec - b.atSec);
  return out;
}

export function createMockPlaybackClient(): PlaybackClient {
  return {
    async getEvent(tenantId: string, eventId: string): Promise<PlaybackEvent> {
      await Promise.resolve();
      const seed = pseudoIndex(`${tenantId}:${eventId}`);
      // Window starts at today 00:00:00 for deterministic test output.
      const start = new Date();
      start.setUTCHours(0, 0, 0, 0);
      return {
        id: eventId,
        tenantId,
        cameraId: `cam-${tenantId}-${(seed % 30).toString().padStart(3, '0')}`,
        cameraName: `Camera ${((seed % 30) + 1).toString().padStart(3, '0')}`,
        windowStart: start.toISOString(),
        windowDurationSec: WINDOW_DURATION_SEC,
        seedAtSec: (seed * 17) % WINDOW_DURATION_SEC,
        posterUrl: null,
      };
    },

    async listMarkers(args: ListMarkersArgs): Promise<PlaybackMarker[]> {
      await Promise.resolve();
      const all = buildMarkers(args.tenantId, args.eventId);
      return all.filter((m) => m.atSec >= args.startSec && m.atSec <= args.endSec);
    },

    async exportClip(args: ExportClipArgs): Promise<ExportClipResult> {
      await Promise.resolve();
      return {
        clipId: `clip-${args.eventId}-${Math.floor(args.startSec)}-${Math.floor(args.durationSec)}`,
        downloadUrl: `https://stub.invalid/clips/${args.eventId}.mp4`,
        expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
      };
    },
  };
}
