// KAI-321: Stub cameras API client.
//
// Mock-only deterministic data for the Customer Admin Cameras page.
// The real Connect-Go generated client (KAI-226) will replace these
// functions. Function signatures are promise-based so migration is
// mechanical. All queries are tenant-scoped via the session store.

export type CameraStatus = 'online' | 'offline' | 'warning';

export type RetentionTier = 'short' | 'standard' | 'long' | 'forensic';

export type StreamCodec = 'h264' | 'h265' | 'mjpeg';

export interface CameraStreamProfile {
  name: string;
  resolution: string;
  fps: number;
  codec: StreamCodec;
  bitrateKbps: number;
}

export interface Camera {
  id: string;
  tenantId: string;
  name: string;
  model: string;
  vendor: string;
  ipAddress: string;
  rtspUrl: string;
  recorderId: string;
  recorderName: string;
  retentionTier: RetentionTier;
  profileName: string;
  status: CameraStatus;
  lastSeenAt: string; // ISO-8601
  enabled: boolean;
  mainProfile?: CameraStreamProfile;
  subProfile?: CameraStreamProfile;
}

export interface Recorder {
  id: string;
  name: string;
  siteName: string;
  cameraCount: number;
  capacity: number;
}

export interface OnvifCandidate {
  id: string;
  ipAddress: string;
  vendor: string;
  model: string;
  hardware: string;
  onvifUrl: string;
}

export interface CameraListFilters {
  tenantId: string;
  search?: string;
  status?: CameraStatus | 'all';
}

export interface CameraSpec {
  name: string;
  rtspUrl: string;
  username: string;
  password: string;
  modelHint?: string;
  recorderId: string;
  retentionTier: RetentionTier;
  profileName: string;
}

// ---------------------------------------------------------------------
// Deterministic mock data generators
// ---------------------------------------------------------------------

const VENDORS = ['Axis', 'Hanwha', 'Hikvision', 'Dahua', 'Bosch'];
const MODELS = ['M3066-LVE', 'XNP-8300RW', 'DS-2CD2386', 'IPC-HFW5442', 'NDI-3503-AL'];
const PROFILES = ['main', 'sub', 'low-light', 'outdoor-ptz'];
const TIERS: RetentionTier[] = ['short', 'standard', 'long', 'forensic'];
const STATUSES: CameraStatus[] = [
  'online',
  'online',
  'online',
  'online',
  'online',
  'warning',
  'offline',
];

const RECORDERS: Recorder[] = [
  { id: 'rec-hq-1', name: 'HQ Recorder 1', siteName: 'Headquarters', cameraCount: 12, capacity: 32 },
  { id: 'rec-hq-2', name: 'HQ Recorder 2', siteName: 'Headquarters', cameraCount: 8, capacity: 32 },
  { id: 'rec-br-1', name: 'Branch Recorder', siteName: 'Branch Office', cameraCount: 10, capacity: 16 },
];

function pseudoIndex(seed: string): number {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

function buildCameras(tenantId: string, count: number): Camera[] {
  const out: Camera[] = [];
  const seed = pseudoIndex(tenantId);
  const now = Date.now();
  for (let i = 0; i < count; i++) {
    const offset = (seed + i) % 1000;
    const vendor = VENDORS[i % VENDORS.length]!;
    const model = MODELS[i % MODELS.length]!;
    const recorder = RECORDERS[i % RECORDERS.length]!;
    const status = STATUSES[i % STATUSES.length]!;
    const retentionTier = TIERS[i % TIERS.length]!;
    const profileName = PROFILES[i % PROFILES.length]!;
    const octet = 10 + (i % 240);
    out.push({
      id: `cam-${tenantId}-${i.toString().padStart(3, '0')}`,
      tenantId,
      name: `Camera ${(i + 1).toString().padStart(3, '0')}`,
      model,
      vendor,
      ipAddress: `10.0.${Math.floor(i / 16)}.${octet}`,
      rtspUrl: `rtsp://10.0.${Math.floor(i / 16)}.${octet}:554/Streaming/Channels/${i % 8}`,
      recorderId: recorder.id,
      recorderName: recorder.name,
      retentionTier,
      profileName,
      status,
      lastSeenAt: new Date(now - offset * 60_000).toISOString(),
      enabled: status !== 'offline',
      mainProfile: {
        name: 'main',
        resolution: '3840x2160',
        fps: 30,
        codec: 'h265',
        bitrateKbps: 8000,
      },
      subProfile: {
        name: 'sub',
        resolution: '1280x720',
        fps: 15,
        codec: 'h264',
        bitrateKbps: 1200,
      },
    });
  }
  return out;
}

function buildOnvifCandidates(): OnvifCandidate[] {
  return [
    {
      id: 'onvif-192.168.10.21',
      ipAddress: '192.168.10.21',
      vendor: 'Axis',
      model: 'M3066-LVE',
      hardware: '1.0',
      onvifUrl: 'http://192.168.10.21/onvif/device_service',
    },
    {
      id: 'onvif-192.168.10.22',
      ipAddress: '192.168.10.22',
      vendor: 'Hanwha',
      model: 'XNP-8300RW',
      hardware: '1.2',
      onvifUrl: 'http://192.168.10.22/onvif/device_service',
    },
    {
      id: 'onvif-192.168.10.23',
      ipAddress: '192.168.10.23',
      vendor: 'Hikvision',
      model: 'DS-2CD2386',
      hardware: '2.1',
      onvifUrl: 'http://192.168.10.23/onvif/device_service',
    },
  ];
}

// ---------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------

export async function listCameras(filters: CameraListFilters): Promise<Camera[]> {
  await Promise.resolve();
  const cameras = buildCameras(filters.tenantId, 30);
  let filtered = cameras;
  if (filters.status && filters.status !== 'all') {
    filtered = filtered.filter((c) => c.status === filters.status);
  }
  if (filters.search && filters.search.trim().length > 0) {
    const q = filters.search.trim().toLowerCase();
    filtered = filtered.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.model.toLowerCase().includes(q) ||
        c.ipAddress.toLowerCase().includes(q),
    );
  }
  return filtered;
}

export async function listRecorders(tenantId: string): Promise<Recorder[]> {
  await Promise.resolve();
  void tenantId;
  return RECORDERS;
}

export interface AddCameraArgs {
  tenantId: string;
  spec: CameraSpec;
}

export async function addCamera(args: AddCameraArgs): Promise<Camera> {
  await Promise.resolve();
  const recorder = RECORDERS.find((r) => r.id === args.spec.recorderId) ?? RECORDERS[0]!;
  return {
    id: `cam-${args.tenantId}-new-${Date.now()}`,
    tenantId: args.tenantId,
    name: args.spec.name,
    model: args.spec.modelHint ?? 'Unknown',
    vendor: 'Unknown',
    ipAddress: '0.0.0.0',
    rtspUrl: args.spec.rtspUrl,
    recorderId: recorder.id,
    recorderName: recorder.name,
    retentionTier: args.spec.retentionTier,
    profileName: args.spec.profileName,
    status: 'online',
    lastSeenAt: new Date().toISOString(),
    enabled: true,
  };
}

export interface UpdateCameraArgs {
  tenantId: string;
  id: string;
  patch: Partial<CameraSpec>;
}

export async function updateCamera(args: UpdateCameraArgs): Promise<void> {
  await Promise.resolve();
  void args;
}

export interface MoveCameraArgs {
  tenantId: string;
  id: string;
  targetRecorderId: string;
}

export async function moveCamera(args: MoveCameraArgs): Promise<void> {
  await Promise.resolve();
  void args;
}

export interface DeleteCameraArgs {
  tenantId: string;
  id: string;
}

export async function deleteCamera(args: DeleteCameraArgs): Promise<void> {
  await Promise.resolve();
  void args;
}

export async function scanOnvif(): Promise<OnvifCandidate[]> {
  await Promise.resolve();
  return buildOnvifCandidates();
}

export interface ProbeProfileArgs {
  rtspUrl: string;
  username: string;
  password: string;
}

export interface ProbeProfileResult {
  profiles: CameraStreamProfile[];
}

export async function probeProfile(args: ProbeProfileArgs): Promise<ProbeProfileResult> {
  await Promise.resolve();
  void args;
  return {
    profiles: [
      { name: 'main', resolution: '3840x2160', fps: 30, codec: 'h265', bitrateKbps: 8000 },
      { name: 'sub', resolution: '1280x720', fps: 15, codec: 'h264', bitrateKbps: 1200 },
    ],
  };
}

export const camerasQueryKeys = {
  all: (tenantId: string) => ['cameras', tenantId] as const,
  list: (tenantId: string, filters: Omit<CameraListFilters, 'tenantId'>) =>
    ['cameras', tenantId, 'list', filters] as const,
  recorders: (tenantId: string) => ['cameras', tenantId, 'recorders'] as const,
  onvifScan: (tenantId: string) => ['cameras', tenantId, 'onvif-scan'] as const,
};

// Validate an RTSP URL string. Exported for reuse in the wizard.
export function isValidRtspUrl(url: string): boolean {
  if (!url) return false;
  try {
    const u = new URL(url);
    return u.protocol === 'rtsp:' || u.protocol === 'rtsps:';
  } catch {
    return false;
  }
}
