// KAI-315: Deterministic mock implementation for permissions API.
//
// 5 tenants, 8 categories, mixed states. Sub-permissions for AI Features.
// This is lazy-imported by permissions.ts via the feature-flag bootstrap.

import type {
  PermissionMatrix,
  PermissionCategory,
  PermissionCategoryId,
  PermissionState,
  PermissionAuditEntry,
  PermissionsClient,
  SavePermissionsRequest,
  SubPermission,
  TenantPermissionRow,
} from './permissions';

// ---------------------------------------------------------------------------
// Sub-permission definitions per category
// ---------------------------------------------------------------------------

const AI_SUB_PERMISSIONS: readonly SubPermission[] = [
  { id: 'object_detection', labelKey: 'permissions.sub.objectDetection', state: 'enabled' },
  { id: 'face_recognition', labelKey: 'permissions.sub.faceRecognition', state: 'disabled' },
  { id: 'lpr', labelKey: 'permissions.sub.lpr', state: 'inherited' },
  { id: 'behavioral', labelKey: 'permissions.sub.behavioral', state: 'enabled' },
  { id: 'semantic_search', labelKey: 'permissions.sub.semanticSearch', state: 'disabled' },
  { id: 'custom_models', labelKey: 'permissions.sub.customModels', state: 'disabled' },
];

const CAMERAS_SUB_PERMISSIONS: readonly SubPermission[] = [
  { id: 'add_cameras', labelKey: 'permissions.sub.addCameras', state: 'enabled' },
  { id: 'ptz_control', labelKey: 'permissions.sub.ptzControl', state: 'inherited' },
  { id: 'delete_cameras', labelKey: 'permissions.sub.deleteCameras', state: 'disabled' },
];

const RECORDINGS_SUB_PERMISSIONS: readonly SubPermission[] = [
  { id: 'view_recordings', labelKey: 'permissions.sub.viewRecordings', state: 'enabled' },
  { id: 'export_recordings', labelKey: 'permissions.sub.exportRecordings', state: 'inherited' },
  { id: 'delete_recordings', labelKey: 'permissions.sub.deleteRecordings', state: 'disabled' },
];

// ---------------------------------------------------------------------------
// Category templates
// ---------------------------------------------------------------------------

function makeCategories(tenantIndex: number): PermissionCategory[] {
  const states: PermissionState[] = ['enabled', 'disabled', 'inherited'];

  const catDefs: { id: PermissionCategoryId; labelKey: string; subs: readonly SubPermission[] }[] = [
    { id: 'cameras', labelKey: 'permissions.category.cameras', subs: CAMERAS_SUB_PERMISSIONS },
    { id: 'recordings', labelKey: 'permissions.category.recordings', subs: RECORDINGS_SUB_PERMISSIONS },
    { id: 'ai_features', labelKey: 'permissions.category.aiFeatures', subs: AI_SUB_PERMISSIONS },
    { id: 'users', labelKey: 'permissions.category.users', subs: [] },
    { id: 'billing', labelKey: 'permissions.category.billing', subs: [] },
    { id: 'api_access', labelKey: 'permissions.category.apiAccess', subs: [] },
    { id: 'white_label', labelKey: 'permissions.category.whiteLabel', subs: [] },
    { id: 'alerts', labelKey: 'permissions.category.alerts', subs: [] },
  ];

  return catDefs.map((def, catIdx) => {
    const stateIndex = (tenantIndex + catIdx) % states.length;
    const topState = states[stateIndex]!;
    // Vary sub-permission states per tenant
    const subs = def.subs.map((sub, subIdx) => ({
      ...sub,
      state: states[(tenantIndex + catIdx + subIdx) % states.length]!,
    }));
    return {
      id: def.id,
      labelKey: def.labelKey,
      state: topState,
      subPermissions: subs,
    };
  });
}

// ---------------------------------------------------------------------------
// Mock tenants
// ---------------------------------------------------------------------------

const PLAN_TIERS = ['free', 'starter', 'pro', 'enterprise', 'pro'];

function buildMockRows(integratorId: string): TenantPermissionRow[] {
  const tenantNames = [
    'Acme Retail Corp',
    'Beta Healthcare Inc',
    'Gamma Logistics LLC',
    'Delta Education Group',
    'Epsilon Finance Ltd',
  ];

  return tenantNames.map((name, i) => ({
    tenantId: `cust-${integratorId}-${String(i).padStart(3, '0')}`,
    tenantName: name,
    planTier: PLAN_TIERS[i]!,
    categories: makeCategories(i),
  }));
}

// ---------------------------------------------------------------------------
// Mock audit trail
// ---------------------------------------------------------------------------

function buildMockAuditTrail(integratorId: string): PermissionAuditEntry[] {
  const entries: PermissionAuditEntry[] = [
    {
      id: 'audit-001',
      timestamp: '2026-04-07T14:30:00Z',
      actor: 'admin@integrator.example',
      tenantId: `cust-${integratorId}-000`,
      tenantName: 'Acme Retail Corp',
      categoryId: 'ai_features',
      subPermissionId: 'face_recognition',
      oldState: 'inherited',
      newState: 'disabled',
    },
    {
      id: 'audit-002',
      timestamp: '2026-04-07T13:15:00Z',
      actor: 'admin@integrator.example',
      tenantId: `cust-${integratorId}-002`,
      tenantName: 'Gamma Logistics LLC',
      categoryId: 'api_access',
      subPermissionId: null,
      oldState: 'disabled',
      newState: 'enabled',
    },
    {
      id: 'audit-003',
      timestamp: '2026-04-06T16:45:00Z',
      actor: 'ops@integrator.example',
      tenantId: `cust-${integratorId}-001`,
      tenantName: 'Beta Healthcare Inc',
      categoryId: 'recordings',
      subPermissionId: 'export_recordings',
      oldState: 'disabled',
      newState: 'inherited',
    },
    {
      id: 'audit-004',
      timestamp: '2026-04-06T10:00:00Z',
      actor: 'admin@integrator.example',
      tenantId: `cust-${integratorId}-004`,
      tenantName: 'Epsilon Finance Ltd',
      categoryId: 'white_label',
      subPermissionId: null,
      oldState: 'inherited',
      newState: 'enabled',
    },
    {
      id: 'audit-005',
      timestamp: '2026-04-05T09:20:00Z',
      actor: 'ops@integrator.example',
      tenantId: `cust-${integratorId}-003`,
      tenantName: 'Delta Education Group',
      categoryId: 'billing',
      subPermissionId: null,
      oldState: 'enabled',
      newState: 'disabled',
    },
  ];
  return entries;
}

// ---------------------------------------------------------------------------
// Mock client implementation
// ---------------------------------------------------------------------------

/** Mutable copy so saves persist within a session. */
let matrixCache: Map<string, PermissionMatrix> = new Map();

export const mockPermissionsClient: PermissionsClient = {
  async getMatrix(integratorId: string): Promise<PermissionMatrix> {
    await new Promise((r) => setTimeout(r, 0));
    if (!matrixCache.has(integratorId)) {
      matrixCache.set(integratorId, {
        integratorId,
        rows: buildMockRows(integratorId),
        auditTrail: buildMockAuditTrail(integratorId),
      });
    }
    return matrixCache.get(integratorId)!;
  },

  async savePermissions(request: SavePermissionsRequest): Promise<void> {
    await new Promise((r) => setTimeout(r, 0));
    const existing = matrixCache.get(request.integratorId);
    if (!existing) return;

    // Apply overrides to the cached matrix
    const updatedRows = existing.rows.map((row) => {
      const rowOverrides = request.overrides.filter((o) => o.tenantId === row.tenantId);
      if (rowOverrides.length === 0) return row;

      const updatedCategories = row.categories.map((cat) => {
        const catOverrides = rowOverrides.filter((o) => o.categoryId === cat.id);
        if (catOverrides.length === 0) return cat;

        let newState = cat.state;
        const topOverride = catOverrides.find((o) => o.subPermissionId === null);
        if (topOverride) newState = topOverride.newState;

        const updatedSubs = cat.subPermissions.map((sub) => {
          const subOverride = catOverrides.find((o) => o.subPermissionId === sub.id);
          return subOverride ? { ...sub, state: subOverride.newState } : sub;
        });

        return { ...cat, state: newState, subPermissions: updatedSubs };
      });

      return { ...row, categories: updatedCategories };
    });

    // Add audit entries for the overrides
    const newAuditEntries: PermissionAuditEntry[] = request.overrides.map((o, i) => ({
      id: `audit-save-${Date.now()}-${i}`,
      timestamp: new Date().toISOString(),
      actor: 'admin@integrator.example',
      tenantId: o.tenantId,
      tenantName: o.tenantName,
      categoryId: o.categoryId,
      subPermissionId: o.subPermissionId,
      oldState: o.oldState,
      newState: o.newState,
    }));

    matrixCache.set(request.integratorId, {
      ...existing,
      rows: updatedRows,
      auditTrail: [...newAuditEntries, ...existing.auditTrail],
    });
  },
};

// Test exports
export const __MOCK_TEST__ = {
  resetCache: () => { matrixCache = new Map(); },
  buildMockRows,
  buildMockAuditTrail,
};
