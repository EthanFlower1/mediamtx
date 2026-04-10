// KAI-313: Deterministic mock implementation for Staff Management API.
//
// 6 staff members with mixed roles and statuses, deterministic data so
// tests are stable. Mutable in-memory store so mutations (invite,
// suspend, remove) are reflected within the same session.

import type {
  StaffClient,
  StaffMember,
  StaffRole,
  StaffStatus,
  InviteStaffArgs,
  UpdateStaffArgs,
  RoleChangeEntry,
} from './staff';

// ---------------------------------------------------------------------------
// Deterministic mock dataset
// ---------------------------------------------------------------------------

const INTEGRATOR_ID = 'integrator-001';

function buildMockStaff(): StaffMember[] {
  const now = Date.UTC(2026, 3, 8, 12, 0, 0);
  return [
    {
      id: 'staff-001',
      integratorId: INTEGRATOR_ID,
      name: 'Alice Chen',
      email: 'alice@integrator.example',
      role: 'owner',
      status: 'active',
      lastLoginIso: new Date(now - 1_800_000).toISOString(),
      createdAt: new Date(now - 365 * 86_400_000).toISOString(),
    },
    {
      id: 'staff-002',
      integratorId: INTEGRATOR_ID,
      name: 'Bob Martinez',
      email: 'bob@integrator.example',
      role: 'admin',
      status: 'active',
      lastLoginIso: new Date(now - 7_200_000).toISOString(),
      createdAt: new Date(now - 200 * 86_400_000).toISOString(),
    },
    {
      id: 'staff-003',
      integratorId: INTEGRATOR_ID,
      name: 'Carol Davis',
      email: 'carol@integrator.example',
      role: 'technician',
      status: 'active',
      lastLoginIso: new Date(now - 86_400_000).toISOString(),
      createdAt: new Date(now - 150 * 86_400_000).toISOString(),
    },
    {
      id: 'staff-004',
      integratorId: INTEGRATOR_ID,
      name: 'Dan Wilson',
      email: 'dan@integrator.example',
      role: 'viewer',
      status: 'invited',
      lastLoginIso: null,
      createdAt: new Date(now - 2 * 86_400_000).toISOString(),
    },
    {
      id: 'staff-005',
      integratorId: INTEGRATOR_ID,
      name: 'Eva Johnson',
      email: 'eva@integrator.example',
      role: 'technician',
      status: 'suspended',
      lastLoginIso: new Date(now - 30 * 86_400_000).toISOString(),
      createdAt: new Date(now - 100 * 86_400_000).toISOString(),
    },
    {
      id: 'staff-006',
      integratorId: INTEGRATOR_ID,
      name: 'Frank Lee',
      email: 'frank@integrator.example',
      role: 'admin',
      status: 'active',
      lastLoginIso: new Date(now - 3_600_000).toISOString(),
      createdAt: new Date(now - 90 * 86_400_000).toISOString(),
    },
  ];
}

// Mutable in-memory stores.
const staffStore: Map<string, StaffMember[]> = new Map();
const roleHistoryStore: Map<string, RoleChangeEntry[]> = new Map();

function getStaff(integratorId: string): StaffMember[] {
  if (!staffStore.has(integratorId)) {
    staffStore.set(
      integratorId,
      integratorId === INTEGRATOR_ID ? buildMockStaff() : [],
    );
  }
  return staffStore.get(integratorId)!;
}

function getRoleHistoryForStaff(staffId: string): RoleChangeEntry[] {
  if (!roleHistoryStore.has(staffId)) {
    // Seed some history for the first staff member.
    if (staffId === 'staff-002') {
      roleHistoryStore.set(staffId, [
        {
          id: 'rh-001',
          staffId: 'staff-002',
          fromRole: 'technician',
          toRole: 'admin',
          changedBy: 'Alice Chen',
          changedAtIso: new Date(Date.UTC(2026, 2, 1, 10, 0, 0)).toISOString(),
        },
      ]);
    } else {
      roleHistoryStore.set(staffId, []);
    }
  }
  return roleHistoryStore.get(staffId)!;
}

let inviteCounter = 0;

// ---------------------------------------------------------------------------
// Mock client implementation
// ---------------------------------------------------------------------------

export const mockStaffClient: StaffClient = {
  async listStaff(integratorId: string): Promise<readonly StaffMember[]> {
    await Promise.resolve();
    return getStaff(integratorId);
  },

  async inviteStaff(args: InviteStaffArgs): Promise<StaffMember> {
    await Promise.resolve();
    inviteCounter += 1;
    const member: StaffMember = {
      id: `staff-new-${inviteCounter}`,
      integratorId: args.integratorId,
      name: args.email.split('@')[0] ?? args.email,
      email: args.email,
      role: args.role,
      status: 'invited',
      lastLoginIso: null,
      createdAt: new Date().toISOString(),
    };
    getStaff(args.integratorId).push(member);
    return member;
  },

  async updateStaff(args: UpdateStaffArgs): Promise<StaffMember> {
    await Promise.resolve();
    const staff = getStaff(args.integratorId);
    const idx = staff.findIndex((s) => s.id === args.staffId);
    if (idx < 0) throw new Error(`Staff ${args.staffId} not found`);
    const old = staff[idx]!;
    const updated: StaffMember = { ...old, role: args.role };
    staff[idx] = updated;

    // Record role change.
    const history = getRoleHistoryForStaff(args.staffId);
    history.push({
      id: `rh-${Date.now()}`,
      staffId: args.staffId,
      fromRole: old.role,
      toRole: args.role,
      changedBy: 'Current User',
      changedAtIso: new Date().toISOString(),
    });

    return updated;
  },

  async suspendStaff(integratorId: string, staffId: string): Promise<void> {
    await Promise.resolve();
    const staff = getStaff(integratorId);
    const idx = staff.findIndex((s) => s.id === staffId);
    if (idx < 0) throw new Error(`Staff ${staffId} not found`);
    staff[idx] = { ...staff[idx]!, status: 'suspended' };
  },

  async reactivateStaff(integratorId: string, staffId: string): Promise<void> {
    await Promise.resolve();
    const staff = getStaff(integratorId);
    const idx = staff.findIndex((s) => s.id === staffId);
    if (idx < 0) throw new Error(`Staff ${staffId} not found`);
    staff[idx] = { ...staff[idx]!, status: 'active' };
  },

  async removeStaff(integratorId: string, staffId: string): Promise<void> {
    await Promise.resolve();
    const staff = getStaff(integratorId);
    const idx = staff.findIndex((s) => s.id === staffId);
    if (idx < 0) throw new Error(`Staff ${staffId} not found`);
    staff.splice(idx, 1);
  },

  async getRoleHistory(
    _integratorId: string,
    staffId: string,
  ): Promise<readonly RoleChangeEntry[]> {
    await Promise.resolve();
    return getRoleHistoryForStaff(staffId);
  },
};

// Test helpers
export const __TEST__ = {
  INTEGRATOR_ID,
  MOCK_STAFF: buildMockStaff(),
  resetStores: () => {
    staffStore.clear();
    roleHistoryStore.clear();
    inviteCounter = 0;
  },
};
