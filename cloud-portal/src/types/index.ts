export type SiteStatus = 'online' | 'degraded' | 'offline';
export type SiteKind = 'logistics' | 'retail' | 'residential' | 'industrial';

export interface Site {
  id: string;
  code: string;
  name: string;
  client: string;
  kind: SiteKind;
  city: string;
  cameras: number;
  online: number;
  recording: number;
  alerts: number;
  status: SiteStatus;
  storage: { used: number; total: number };
  bandwidth: number;
  region: string;
  x: number;
  y: number;
  lastSync: string;
}

export type CamStatus = 'online' | 'degraded' | 'offline';

export interface Camera {
  id: string;
  name: string;
  siteId: string;
  siteCode: string;
  status: CamStatus;
  recording: boolean;
  motion: boolean;
  caps: string[];
  resolution: string;
  fps: number;
  timestamp: string;
  retention: number;
  detection?: string;
  feed?: string;
}

export interface RuleAction { kind: string; detail: string; }
export interface RuleCondition { kind: string; detail: string; }
export interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  scope: { sites: string | string[]; cameras: string };
  trigger: string;
  triggerDetail: string;
  conditions: RuleCondition[];
  actions: RuleAction[];
  cameras: number;
}

export interface User {
  id: string;
  name: string;
  email: string;
  role: string;
  sites: 'all' | string[];
  mfa: boolean;
  lastSeen: string;
  status: 'active' | 'invited';
  avatar: string;
}

export interface Role {
  id: string;
  name: string;
  description: string;
  perms: string[];
  count: number;
}

export type AlertSeverity = 'info' | 'warning' | 'danger';

export interface AlertItem {
  id: string;
  kind: string;
  severity: AlertSeverity;
  site: string;
  siteName: string;
  cam: string;
  camName: string;
  text: string;
  time: string;
  ack: boolean;
}

export interface AuditEntry {
  time: string;
  user: string;
  action: string;
  target: string;
  detail: string;
  site: string;
}

export interface SavedView {
  id: string;
  name: string;
  cams: number;
  sites: number;
  lastUsed: string;
}

export type Scope =
  | { kind: 'all' }
  | { kind: 'site'; id: string }
  | { kind: 'group'; label: string; ids: string[] };

export type Route =
  | 'overview' | 'live' | 'playback' | 'search'
  | 'sites' | 'devices' | 'rules' | 'users'
  | 'remote' | 'alerts' | 'audit' | 'billing' | 'onboarding';
