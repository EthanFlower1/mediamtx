import type {
  Site, Camera, Rule, User, Role, AlertItem, AuditEntry, SavedView, SiteKind,
} from '../types';

export const SITES: Site[] = [
  { id: 'SITE-01', code: 'MRC-04', name: 'Mercer Logistics — Hub 04', client: 'Mercer Logistics', kind: 'logistics', city: 'Tacoma, WA', cameras: 24, online: 22, recording: 22, alerts: 3, status: 'online', storage: { used: 412, total: 2048 }, bandwidth: 18.4, region: 'us-west', x: 12, y: 28, lastSync: '14:32:07' },
  { id: 'SITE-02', code: 'MRC-07', name: 'Mercer Logistics — Cold Chain', client: 'Mercer Logistics', kind: 'logistics', city: 'Salem, OR', cameras: 18, online: 18, recording: 18, alerts: 1, status: 'online', storage: { used: 290, total: 1024 }, bandwidth: 12.1, region: 'us-west', x: 18, y: 41, lastSync: '14:32:07' },
  { id: 'SITE-03', code: 'PCK-01', name: 'Packwell Distribution', client: 'Packwell', kind: 'logistics', city: 'Reno, NV', cameras: 32, online: 28, recording: 28, alerts: 7, status: 'degraded', storage: { used: 698, total: 2048 }, bandwidth: 22.7, region: 'us-west', x: 24, y: 56, lastSync: '14:32:05' },
  { id: 'SITE-04', code: 'NRT-12', name: 'Northgate Retail — Pike St', client: 'Northgate Retail', kind: 'retail', city: 'Seattle, WA', cameras: 14, online: 14, recording: 14, alerts: 2, status: 'online', storage: { used: 184, total: 512 }, bandwidth: 6.3, region: 'us-west', x: 14, y: 22, lastSync: '14:32:07' },
  { id: 'SITE-05', code: 'NRT-18', name: 'Northgate Retail — Bellevue', client: 'Northgate Retail', kind: 'retail', city: 'Bellevue, WA', cameras: 14, online: 13, recording: 13, alerts: 0, status: 'online', storage: { used: 162, total: 512 }, bandwidth: 5.9, region: 'us-west', x: 16, y: 24, lastSync: '14:32:07' },
  { id: 'SITE-06', code: 'NRT-22', name: 'Northgate Retail — Capitol Hill', client: 'Northgate Retail', kind: 'retail', city: 'Seattle, WA', cameras: 12, online: 12, recording: 12, alerts: 1, status: 'online', storage: { used: 140, total: 512 }, bandwidth: 5.2, region: 'us-west', x: 15, y: 23, lastSync: '14:32:07' },
  { id: 'SITE-07', code: 'BRX-03', name: 'Bryx Outlet — Spokane', client: 'Bryx', kind: 'retail', city: 'Spokane, WA', cameras: 9, online: 0, recording: 0, alerts: 0, status: 'offline', storage: { used: 0, total: 256 }, bandwidth: 0, region: 'us-west', x: 38, y: 19, lastSync: '11:14:22' },
  { id: 'SITE-08', code: 'BRX-08', name: 'Bryx Outlet — Boise', client: 'Bryx', kind: 'retail', city: 'Boise, ID', cameras: 11, online: 11, recording: 10, alerts: 0, status: 'online', storage: { used: 122, total: 512 }, bandwidth: 4.8, region: 'us-west', x: 44, y: 38, lastSync: '14:32:06' },
  { id: 'SITE-09', code: 'HSE-21', name: 'Halverson Residence', client: 'Halverson', kind: 'residential', city: 'Mercer Island, WA', cameras: 8, online: 8, recording: 8, alerts: 0, status: 'online', storage: { used: 84, total: 256 }, bandwidth: 2.4, region: 'us-west', x: 16, y: 26, lastSync: '14:32:07' },
  { id: 'SITE-10', code: 'HSE-34', name: 'Vinland Estate', client: 'Vinland', kind: 'residential', city: 'Woodinville, WA', cameras: 12, online: 12, recording: 12, alerts: 1, status: 'online', storage: { used: 96, total: 512 }, bandwidth: 3.6, region: 'us-west', x: 17, y: 21, lastSync: '14:32:07' },
  { id: 'SITE-11', code: 'HSE-41', name: 'Tomasi Residence', client: 'Tomasi', kind: 'residential', city: 'Lake Oswego, OR', cameras: 6, online: 5, recording: 5, alerts: 0, status: 'degraded', storage: { used: 48, total: 256 }, bandwidth: 1.8, region: 'us-west', x: 19, y: 44, lastSync: '14:32:06' },
  { id: 'SITE-12', code: 'ATL-02', name: 'Atlas Steel — Yard 2', client: 'Atlas Steel', kind: 'industrial', city: 'Portland, OR', cameras: 28, online: 27, recording: 27, alerts: 4, status: 'online', storage: { used: 612, total: 2048 }, bandwidth: 19.8, region: 'us-west', x: 21, y: 46, lastSync: '14:32:07' },
  { id: 'SITE-13', code: 'ATL-05', name: 'Atlas Steel — Foundry Annex', client: 'Atlas Steel', kind: 'industrial', city: 'Portland, OR', cameras: 16, online: 16, recording: 16, alerts: 2, status: 'online', storage: { used: 304, total: 1024 }, bandwidth: 11.2, region: 'us-west', x: 22, y: 47, lastSync: '14:32:07' },
  { id: 'SITE-14', code: 'CRX-01', name: 'Corex Marine Terminal', client: 'Corex', kind: 'industrial', city: 'Long Beach, CA', cameras: 36, online: 34, recording: 33, alerts: 5, status: 'online', storage: { used: 824, total: 4096 }, bandwidth: 28.6, region: 'us-west', x: 28, y: 78, lastSync: '14:32:07' },
];

const CAM_NAMES: Record<SiteKind, string[]> = {
  logistics: ['Dock A','Dock B','Dock C','Receiving','Aisle 12','Aisle 24','Loading Bay','Yard North','Yard South','Office','Server Room','Perimeter East','Perimeter West','Gate'],
  retail: ['Storefront','Register','Aisle 1','Aisle 3','Stockroom','Rear Exit','Office','Manager Desk'],
  residential: ['Front Door','Driveway','Backyard','Side Gate','Garage','Pool','Front Walk','Patio'],
  industrial: ['Furnace Bay','Crane North','Crane South','Pour Floor','Shipping','Yard','Gate A','Gate B','Control Room','Catwalk','Conveyor'],
};

// Deterministic pseudo-random keyed by camera index so renders stay stable.
function det(seed: number, mod: number): number {
  const x = Math.sin(seed * 9301 + 49297) * 233280;
  return Math.floor((x - Math.floor(x)) * mod);
}

function buildCameras(): Camera[] {
  const all: Camera[] = [];
  let idx = 1;
  for (const site of SITES) {
    const pool = CAM_NAMES[site.kind];
    for (let i = 0; i < site.cameras; i++) {
      const camId = `CAM-${String(idx).padStart(3, '0')}`;
      const offline = i >= site.online;
      const recording = i < site.recording;
      const motion = !offline && det(idx, 100) < 18;
      const caps = ['REC',
        ...(det(idx + 1, 100) < 60 ? ['AI'] : []),
        ...(det(idx + 2, 100) < 30 ? ['PTZ'] : []),
        ...(det(idx + 3, 100) < 25 ? ['AUDIO'] : []),
      ];
      all.push({
        id: camId,
        name: pool[i % pool.length] + (i >= pool.length ? ` ${Math.floor(i / pool.length) + 1}` : ''),
        siteId: site.id,
        siteCode: site.code,
        status: offline ? 'offline' : (det(idx + 4, 100) < 4 ? 'degraded' : 'online'),
        recording,
        motion,
        caps,
        resolution: ['1920×1080','2560×1440','3840×2160'][det(idx + 5, 3)],
        fps: [15, 24, 30][det(idx + 6, 3)],
        timestamp: '14:32:07',
        retention: [7, 14, 30, 90][det(idx + 7, 4)],
      });
      idx++;
    }
  }
  return all;
}

export const CAMERAS: Camera[] = buildCameras();

export const RULES: Rule[] = [
  { id: 'RULE-01', name: 'Business hours — full record', enabled: true, scope: { sites: 'retail', cameras: 'all' }, trigger: 'schedule', triggerDetail: 'Mon–Sat 08:00–22:00', conditions: [], actions: [{ kind: 'record', detail: 'Continuous · 30 fps' }, { kind: 'retain', detail: '30 days' }], cameras: 60 },
  { id: 'RULE-02', name: 'Loitering — perimeter alert', enabled: true, scope: { sites: 'all', cameras: 'tagged:perimeter' }, trigger: 'ai-detection', triggerDetail: 'Person dwelling > 60s', conditions: [{ kind: 'time', detail: 'Outside hours' }], actions: [{ kind: 'alert', detail: 'Push + email to ops' }, { kind: 'record', detail: 'Pre/post 30s' }, { kind: 'retain', detail: '90 days' }], cameras: 38 },
  { id: 'RULE-03', name: 'After-hours motion — log only', enabled: true, scope: { sites: ['SITE-04','SITE-05','SITE-06'], cameras: 'all' }, trigger: 'motion', triggerDetail: 'Any motion', conditions: [{ kind: 'time', detail: '22:00–08:00' }], actions: [{ kind: 'record', detail: 'Event clip 60s' }, { kind: 'retain', detail: '14 days' }], cameras: 40 },
  { id: 'RULE-04', name: 'Vehicle in loading zone', enabled: true, scope: { sites: 'logistics', cameras: 'tagged:dock' }, trigger: 'ai-detection', triggerDetail: 'Vehicle classified', conditions: [], actions: [{ kind: 'record', detail: 'Continuous while present' }, { kind: 'log', detail: 'Plate + timestamp' }], cameras: 22 },
  { id: 'RULE-05', name: 'Storage cap enforcement', enabled: true, scope: { sites: 'all', cameras: 'all' }, trigger: 'storage-threshold', triggerDetail: 'Site storage > 80%', conditions: [], actions: [{ kind: 'compress', detail: 'Re-encode > 14d to 720p' }], cameras: 270 },
  { id: 'RULE-06', name: 'Weapon detection — escalate', enabled: false, scope: { sites: 'retail', cameras: 'all' }, trigger: 'ai-detection', triggerDetail: 'Weapon class', conditions: [], actions: [{ kind: 'alert', detail: 'SMS + page on-call' }, { kind: 'record', detail: 'Pre/post 60s' }, { kind: 'retain', detail: '180 days' }], cameras: 60 },
];

export const USERS: User[] = [
  { id: 'U-001', name: 'Avery Chen', email: 'avery@northwall.security', role: 'Owner', sites: 'all', mfa: true, lastSeen: 'now', status: 'active', avatar: 'AC' },
  { id: 'U-002', name: 'Marcus Reyes', email: 'marcus@northwall.security', role: 'Operator', sites: 'all', mfa: true, lastSeen: '12 min', status: 'active', avatar: 'MR' },
  { id: 'U-003', name: 'Priya Singh', email: 'priya@northwall.security', role: 'Operator', sites: ['SITE-01','SITE-02','SITE-03'], mfa: true, lastSeen: '2 hr', status: 'active', avatar: 'PS' },
  { id: 'U-004', name: 'Devon Walsh', email: 'devon@mercerlogistics.com', role: 'Site Admin', sites: ['SITE-01','SITE-02'], mfa: true, lastSeen: '1 hr', status: 'active', avatar: 'DW' },
  { id: 'U-005', name: 'Hana Mori', email: 'hana@northgate-retail.com', role: 'Site Admin', sites: ['SITE-04','SITE-05','SITE-06'], mfa: false, lastSeen: '4 hr', status: 'active', avatar: 'HM' },
  { id: 'U-006', name: 'Eli Tomasi', email: 'eli@tomasi.family', role: 'Viewer', sites: ['SITE-11'], mfa: true, lastSeen: '1 day', status: 'active', avatar: 'ET' },
  { id: 'U-007', name: 'Riley Patel', email: 'riley@northwall.security', role: 'Operator', sites: 'all', mfa: false, lastSeen: '6 hr', status: 'invited', avatar: 'RP' },
  { id: 'U-008', name: 'Kai Whitford', email: 'kai@atlassteel.com', role: 'Site Admin', sites: ['SITE-12','SITE-13'], mfa: true, lastSeen: '20 min', status: 'active', avatar: 'KW' },
];

export const ROLES: Role[] = [
  { id: 'owner', name: 'Owner', description: 'Full control over org, billing, all sites and users.', perms: ['org.*','billing.*','sites.*','users.*','rules.*','playback.*','live.*'], count: 1 },
  { id: 'operator', name: 'Operator', description: 'View live, playback, search, manage alerts across assigned sites.', perms: ['live.view','playback.view','search.*','alerts.manage','rules.read'], count: 4 },
  { id: 'site-admin', name: 'Site Admin', description: 'Configure devices, rules, and users for a specific site.', perms: ['site.config','devices.*','rules.write','users.invite','live.view','playback.view'], count: 3 },
  { id: 'viewer', name: 'Viewer', description: 'Read-only live view and playback of assigned cameras.', perms: ['live.view','playback.view'], count: 1 },
];

export const ALERTS: AlertItem[] = [
  { id: 'A-1481', kind: 'motion', severity: 'info', site: 'MRC-04', siteName: 'Mercer Logistics — Hub 04', cam: 'CAM-008', camName: 'Dock C', text: 'Person detected · dwelling 18s', time: '14:31:42', ack: false },
  { id: 'A-1480', kind: 'tamper', severity: 'danger', site: 'PCK-01', siteName: 'Packwell Distribution', cam: 'CAM-049', camName: 'Yard North', text: 'Camera tilt detected', time: '14:28:09', ack: false },
  { id: 'A-1479', kind: 'offline', severity: 'danger', site: 'BRX-03', siteName: 'Bryx Outlet — Spokane', cam: 'SITE', camName: 'All cameras', text: 'NVR unreachable for 3h 18m', time: '11:14:22', ack: false },
  { id: 'A-1478', kind: 'motion', severity: 'warning', site: 'CRX-01', siteName: 'Corex Marine Terminal', cam: 'CAM-241', camName: 'Gate A', text: 'Vehicle outside schedule window', time: '14:14:07', ack: true },
  { id: 'A-1477', kind: 'storage', severity: 'warning', site: 'PCK-01', siteName: 'Packwell Distribution', cam: 'NVR', camName: 'Primary NVR', text: 'Storage at 87% — compression scheduled', time: '13:58:00', ack: true },
  { id: 'A-1476', kind: 'motion', severity: 'info', site: 'NRT-12', siteName: 'Northgate Retail — Pike St', cam: 'CAM-046', camName: 'Rear Exit', text: 'Person detected', time: '13:44:13', ack: true },
  { id: 'A-1475', kind: 'ai', severity: 'warning', site: 'ATL-02', siteName: 'Atlas Steel — Yard 2', cam: 'CAM-186', camName: 'Crane South', text: 'Worker in restricted zone · 0.94', time: '13:31:55', ack: false },
  { id: 'A-1474', kind: 'motion', severity: 'info', site: 'MRC-04', siteName: 'Mercer Logistics — Hub 04', cam: 'CAM-002', camName: 'Loading Bay', text: 'Vehicle classified · truck', time: '13:21:00', ack: true },
];

export const AUDIT: AuditEntry[] = [
  { time: '14:31:08', user: 'Avery Chen', action: 'rule.toggle', target: 'RULE-06 · Weapon detection — escalate', detail: 'Disabled', site: 'global' },
  { time: '14:18:42', user: 'Marcus Reyes', action: 'playback.export', target: 'CAM-008 · 14:00–14:15', detail: 'MP4 · 192 MB', site: 'MRC-04' },
  { time: '14:11:27', user: 'Hana Mori', action: 'user.invite', target: 'jordan@northgate-retail.com', detail: 'Role: Viewer', site: 'NRT-12' },
  { time: '13:58:00', user: 'system', action: 'rule.fire', target: 'RULE-05 · Storage cap enforcement', detail: 'PCK-01 storage at 87%', site: 'PCK-01' },
  { time: '13:44:13', user: 'system', action: 'alert.created', target: 'A-1476', detail: 'Motion · Rear Exit', site: 'NRT-12' },
  { time: '13:32:01', user: 'Devon Walsh', action: 'device.add', target: 'CAM-049 · Yard North', detail: 'RTSP · 1080p · 30fps', site: 'PCK-01' },
  { time: '13:14:55', user: 'Avery Chen', action: 'remote-access.token', target: 'SITE-09 · 7-day token', detail: 'Issued to eli@tomasi.family', site: 'HSE-21' },
  { time: '12:48:12', user: 'Priya Singh', action: 'live.view', target: 'Wall: NIGHT-OPS', detail: '24 cameras · 4 sites', site: 'global' },
  { time: '12:31:08', user: 'Marcus Reyes', action: 'rule.edit', target: 'RULE-02 · Loitering — perimeter alert', detail: 'Dwell threshold 45s → 60s', site: 'global' },
  { time: '11:14:22', user: 'system', action: 'site.offline', target: 'SITE-07 · BRX-03', detail: 'Last contact 11:11:04', site: 'BRX-03' },
  { time: '10:42:00', user: 'Avery Chen', action: 'billing.plan', target: 'Subscription', detail: 'Upgraded to Integrator (270 cams)', site: 'org' },
  { time: '09:58:31', user: 'Kai Whitford', action: 'rule.create', target: 'Worker in restricted zone', detail: 'AI · zone polygon', site: 'ATL-02' },
];

export const SAVED_VIEWS: SavedView[] = [
  { id: 'view-night', name: 'NIGHT-OPS', cams: 12, sites: 4, lastUsed: '6h ago' },
  { id: 'view-perim', name: 'HQ-PERIMETER', cams: 8, sites: 1, lastUsed: '2h ago' },
  { id: 'view-retail', name: 'RETAIL-FLOORS', cams: 16, sites: 6, lastUsed: 'yesterday' },
  { id: 'view-cranes', name: 'CRANE-DECK', cams: 6, sites: 2, lastUsed: '4d ago' },
];
