// KAI-313: Staff Management API client (Integrator Portal).
//
// Typed promise stubs for the integrator-facing staff lifecycle:
//   - listStaff(integratorId)
//   - inviteStaff(args)
//   - updateStaff(args)
//   - suspendStaff(integratorId, staffId)
//   - reactivateStaff(integratorId, staffId)
//   - removeStaff(integratorId, staffId)
//
// All data is mocked via staff.mock.ts behind a feature-flag lazy
// import. The real implementation will be wired to Connect-Go clients
// generated from KAI-238 protos. This file is the single seam;
// swapping transports is a one-file change.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type StaffRole = 'owner' | 'admin' | 'technician' | 'viewer';
export type StaffStatus = 'active' | 'invited' | 'suspended';

export interface StaffMember {
  readonly id: string;
  readonly integratorId: string;
  readonly name: string;
  readonly email: string;
  readonly role: StaffRole;
  readonly status: StaffStatus;
  readonly lastLoginIso: string | null;
  readonly createdAt: string;
}

export interface RoleChangeEntry {
  readonly id: string;
  readonly staffId: string;
  readonly fromRole: StaffRole;
  readonly toRole: StaffRole;
  readonly changedBy: string;
  readonly changedAtIso: string;
}

export interface InviteStaffArgs {
  readonly integratorId: string;
  readonly email: string;
  readonly role: StaffRole;
  readonly message?: string;
}

export interface UpdateStaffArgs {
  readonly integratorId: string;
  readonly staffId: string;
  readonly role: StaffRole;
}

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface StaffClient {
  listStaff(integratorId: string): Promise<readonly StaffMember[]>;
  inviteStaff(args: InviteStaffArgs): Promise<StaffMember>;
  updateStaff(args: UpdateStaffArgs): Promise<StaffMember>;
  suspendStaff(integratorId: string, staffId: string): Promise<void>;
  reactivateStaff(integratorId: string, staffId: string): Promise<void>;
  removeStaff(integratorId: string, staffId: string): Promise<void>;
  getRoleHistory(integratorId: string, staffId: string): Promise<readonly RoleChangeEntry[]>;
}

// ---------------------------------------------------------------------------
// Query key factory (integrator-scoped)
// ---------------------------------------------------------------------------

export const STAFF_QUERY_KEY = 'staff' as const;

export const staffQueryKeys = {
  all: (integratorId: string) => [STAFF_QUERY_KEY, integratorId] as const,
  list: (integratorId: string) => [STAFF_QUERY_KEY, integratorId, 'list'] as const,
  roleHistory: (integratorId: string, staffId: string) =>
    [STAFF_QUERY_KEY, integratorId, 'roleHistory', staffId] as const,
};

// ---------------------------------------------------------------------------
// Feature-flag bootstrap with lazy mock import
// ---------------------------------------------------------------------------

let _client: StaffClient | null = null;

async function getClient(): Promise<StaffClient> {
  if (_client) return _client;
  // TODO(KAI-313): replace with real Connect-Go client when KAI-238
  // protos ship. For now, always load mock.
  const mock = await import('./staff.mock');
  _client = mock.mockStaffClient;
  return _client;
}

// ---------------------------------------------------------------------------
// Public API — thin wrappers that resolve the client lazily
// ---------------------------------------------------------------------------

export async function listStaff(integratorId: string): Promise<readonly StaffMember[]> {
  const client = await getClient();
  return client.listStaff(integratorId);
}

export async function inviteStaff(args: InviteStaffArgs): Promise<StaffMember> {
  const client = await getClient();
  return client.inviteStaff(args);
}

export async function updateStaff(args: UpdateStaffArgs): Promise<StaffMember> {
  const client = await getClient();
  return client.updateStaff(args);
}

export async function suspendStaff(integratorId: string, staffId: string): Promise<void> {
  const client = await getClient();
  return client.suspendStaff(integratorId, staffId);
}

export async function reactivateStaff(integratorId: string, staffId: string): Promise<void> {
  const client = await getClient();
  return client.reactivateStaff(integratorId, staffId);
}

export async function removeStaff(integratorId: string, staffId: string): Promise<void> {
  const client = await getClient();
  return client.removeStaff(integratorId, staffId);
}

export async function getRoleHistory(
  integratorId: string,
  staffId: string,
): Promise<readonly RoleChangeEntry[]> {
  const client = await getClient();
  return client.getRoleHistory(integratorId, staffId);
}

// ---------------------------------------------------------------------------
// Role metadata (predefined, not tenant-configurable)
// ---------------------------------------------------------------------------

export interface RoleDefinition {
  readonly role: StaffRole;
  readonly labelKey: string;
  readonly descriptionKey: string;
  readonly iconLabel: string; // emoji-free text icon identifier
}

export const ROLE_DEFINITIONS: readonly RoleDefinition[] = [
  {
    role: 'owner',
    labelKey: 'staff.roles.owner',
    descriptionKey: 'staff.roles.ownerDescription',
    iconLabel: 'crown',
  },
  {
    role: 'admin',
    labelKey: 'staff.roles.admin',
    descriptionKey: 'staff.roles.adminDescription',
    iconLabel: 'shield',
  },
  {
    role: 'technician',
    labelKey: 'staff.roles.technician',
    descriptionKey: 'staff.roles.technicianDescription',
    iconLabel: 'wrench',
  },
  {
    role: 'viewer',
    labelKey: 'staff.roles.viewer',
    descriptionKey: 'staff.roles.viewerDescription',
    iconLabel: 'eye',
  },
];

// Test constants
export const CURRENT_INTEGRATOR_ID = 'integrator-001';

export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
  resetClient: () => {
    _client = null;
  },
};
