// KAI-309: Customers API client stub (Integrator Portal).
//
// Typed promise stubs for the integrator-facing customer lifecycle:
//   - listCustomers(integratorId, filters)
//   - getCustomer(id)
//   - createCustomer(spec)
//   - generateSetupToken(tenantId)
//   - beginImpersonation(customerId, reason)
//
// All data is mocked here. The real implementation will be wired to
// Connect-Go clients generated from KAI-238 protos. This file is the
// single seam; swapping transports is a one-file change.
//
// Scope enforcement: listCustomers MUST filter by integratorId. Ten
// of the 40 sample customers deliberately belong to a different
// integrator so we can assert in tests that cross-integrator reads
// never leak into the UI.

import type { CustomerTier } from './fleet';

export type CustomerStatus = 'active' | 'pending' | 'suspended' | 'churned';
export type CustomerRelationship = 'direct' | 'sub_reseller';
export type BillingMode = 'direct' | 'via_integrator';
export type PlanId = 'free' | 'starter' | 'pro' | 'enterprise';

export interface CustomerSummary {
  readonly id: string;
  readonly integratorId: string;
  readonly subResellerId: string | null;
  readonly name: string;
  readonly status: CustomerStatus;
  readonly tier: CustomerTier;
  readonly relationship: CustomerRelationship;
  readonly camerasManaged: number;
  readonly monthlyRecurringRevenueCents: number;
  readonly lastActivityIso: string;
  readonly labels: readonly string[];
  readonly billingMode: BillingMode;
  readonly plan: PlanId;
  readonly contactEmail: string;
  readonly timezone: string;
  readonly country: string;
}

export interface CustomerActivityEntry {
  readonly id: string;
  readonly customerId: string;
  readonly iso: string;
  readonly kind: 'login' | 'camera_added' | 'incident' | 'payment' | 'user_invited';
  readonly actor: string;
  readonly detailKey: string;
}

export interface CustomerDetail extends CustomerSummary {
  readonly uptimePercent: number;
  readonly activeIncidents: number;
  readonly contact: {
    readonly name: string;
    readonly email: string;
    readonly phone: string;
  };
  readonly recentActivity: readonly CustomerActivityEntry[];
}

export interface CustomerFilters {
  readonly subResellerId?: string | null;
  readonly label?: string | null;
  readonly tier?: CustomerTier | null;
  readonly search?: string | null;
}

export interface CreateCustomerSpec {
  readonly name: string;
  readonly contactEmail: string;
  readonly timezone: string;
  readonly country: string;
  readonly billingMode: BillingMode;
  readonly plan: PlanId;
}

export interface CreateCustomerResult {
  readonly customer: CustomerSummary;
  readonly setupToken: SetupToken;
}

export interface SetupToken {
  readonly tenantId: string;
  readonly token: string;
  readonly expiresAtIso: string;
  readonly qrPayload: string;
}

export interface ImpersonationSession {
  readonly sessionId: string;
  readonly customerId: string;
  readonly integratorUserId: string;
  readonly reason: string;
  readonly beganAtIso: string;
  readonly auditLogId: string;
}

// Plan tier metadata. Hardcoded here — this is catalog config, not a
// customer-visible string. All display text comes from i18n keyed off
// the plan id.
export interface PlanTier {
  readonly id: PlanId;
  readonly maxCameras: number;
  readonly monthlyPriceCents: number;
}

export const PLAN_CATALOG: readonly PlanTier[] = [
  { id: 'free', maxCameras: 4, monthlyPriceCents: 0 },
  { id: 'starter', maxCameras: 16, monthlyPriceCents: 4900 },
  { id: 'pro', maxCameras: 64, monthlyPriceCents: 19900 },
  { id: 'enterprise', maxCameras: 1024, monthlyPriceCents: 99900 },
];

// ---------------------------------------------------------------------------
// Mock dataset — 40 customers; 10 belong to another integrator.
// ---------------------------------------------------------------------------

const CURRENT_INTEGRATOR_ID = 'integrator-001';
const OTHER_INTEGRATOR_ID = 'integrator-999';

const SUB_RESELLERS = [
  { id: 'sr-north', name: 'North Region Partners' },
  { id: 'sr-south', name: 'South Region Partners' },
  { id: 'sr-west', name: 'Western Security Co.' },
];

const LABELS = ['retail', 'healthcare', 'logistics', 'education', 'finance'] as const;
const TIERS: readonly CustomerTier[] = ['platinum', 'gold', 'silver', 'bronze'];
const STATUSES: readonly CustomerStatus[] = ['active', 'active', 'active', 'pending', 'suspended', 'churned'];
const PLAN_IDS: readonly PlanId[] = ['free', 'starter', 'pro', 'enterprise'];

const TIMEZONES = ['America/New_York', 'America/Los_Angeles', 'Europe/London', 'Europe/Berlin'];
const COUNTRIES = ['US', 'GB', 'DE', 'CA'];

function pseudo(index: number, salt: number): number {
  // Deterministic "hash" so the mock set is stable across runs.
  return Math.abs(Math.sin(index * 9.973 + salt) * 10_000) % 1;
}

function makeCustomer(index: number, integratorId: string): CustomerSummary {
  const tier = TIERS[index % TIERS.length]!;
  const status = STATUSES[index % STATUSES.length]!;
  const subReseller = index % 3 === 0 ? null : SUB_RESELLERS[index % SUB_RESELLERS.length]!;
  const camerasManaged = 4 + ((index * 11) % 96);
  const plan = PLAN_IDS[index % PLAN_IDS.length]!;
  const label = LABELS[index % LABELS.length]!;
  const nameLetter = String.fromCharCode(65 + (index % 26));
  return {
    id: `cust-${integratorId}-${String(index).padStart(3, '0')}`,
    integratorId,
    subResellerId: subReseller?.id ?? null,
    name: `${nameLetter}cme ${index} ${label.charAt(0).toUpperCase() + label.slice(1)}`,
    status,
    tier,
    relationship: subReseller ? 'sub_reseller' : 'direct',
    camerasManaged,
    monthlyRecurringRevenueCents: (200 + index * 43) * 100,
    lastActivityIso: new Date(
      Date.UTC(2026, 3, 7, 12, 0, 0) - index * 3_600_000 * (1 + pseudo(index, 1)),
    ).toISOString(),
    labels: [label],
    billingMode: index % 2 === 0 ? 'direct' : 'via_integrator',
    plan,
    contactEmail: `ops-${index}@customer-${nameLetter.toLowerCase()}${index}.example`,
    timezone: TIMEZONES[index % TIMEZONES.length]!,
    country: COUNTRIES[index % COUNTRIES.length]!,
  };
}

function buildMockCustomers(): CustomerSummary[] {
  const inScope = Array.from({ length: 30 }, (_, i) => makeCustomer(i, CURRENT_INTEGRATOR_ID));
  const outOfScope = Array.from({ length: 10 }, (_, i) =>
    makeCustomer(200 + i, OTHER_INTEGRATOR_ID),
  );
  return [...inScope, ...outOfScope];
}

const MOCK_CUSTOMERS: CustomerSummary[] = buildMockCustomers();

function buildRecentActivity(customerId: string): CustomerActivityEntry[] {
  const kinds: CustomerActivityEntry['kind'][] = [
    'login',
    'camera_added',
    'incident',
    'payment',
    'user_invited',
  ];
  return kinds.map((kind, i) => ({
    id: `${customerId}-act-${i}`,
    customerId,
    iso: new Date(Date.UTC(2026, 3, 7, 11, 0, 0) - i * 5_400_000).toISOString(),
    kind,
    actor: i % 2 === 0 ? 'system' : 'admin@customer.example',
    detailKey: `customers.activity.kind.${kind}`,
  }));
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * listCustomers returns customers scoped to the given integrator.
 *
 * Scope enforcement: any customer whose integratorId does not match
 * the requested integratorId is dropped here. The real RPC will do
 * the same server-side.
 */
export async function listCustomers(
  integratorId: string,
  filters: CustomerFilters = {},
): Promise<readonly CustomerSummary[]> {
  await new Promise((r) => setTimeout(r, 0));
  const scoped = MOCK_CUSTOMERS.filter((c) => c.integratorId === integratorId);
  const needle = filters.search?.trim().toLowerCase() ?? '';
  return scoped.filter((c) => {
    if (filters.subResellerId && c.subResellerId !== filters.subResellerId) return false;
    if (filters.label && !c.labels.includes(filters.label)) return false;
    if (filters.tier && c.tier !== filters.tier) return false;
    if (needle && !c.name.toLowerCase().includes(needle)) return false;
    return true;
  });
}

export async function getCustomer(id: string): Promise<CustomerDetail | null> {
  await new Promise((r) => setTimeout(r, 0));
  const summary = MOCK_CUSTOMERS.find((c) => c.id === id);
  if (!summary) return null;
  return {
    ...summary,
    uptimePercent: 99.1 + (pseudo(summary.camerasManaged, 3) * 0.8),
    activeIncidents: summary.status === 'active' ? Math.round(pseudo(summary.camerasManaged, 4) * 3) : 0,
    contact: {
      name: `Primary Contact ${summary.name}`,
      email: summary.contactEmail,
      phone: '+1-555-0100',
    },
    recentActivity: buildRecentActivity(summary.id),
  };
}

let createCounter = 0;

export async function createCustomer(
  spec: CreateCustomerSpec,
  integratorId: string = CURRENT_INTEGRATOR_ID,
): Promise<CreateCustomerResult> {
  await new Promise((r) => setTimeout(r, 0));
  createCounter += 1;
  const id = `cust-${integratorId}-new-${createCounter}`;
  const customer: CustomerSummary = {
    id,
    integratorId,
    subResellerId: null,
    name: spec.name,
    status: 'pending',
    tier: 'bronze',
    relationship: 'direct',
    camerasManaged: 0,
    monthlyRecurringRevenueCents: 0,
    lastActivityIso: new Date().toISOString(),
    labels: [],
    billingMode: spec.billingMode,
    plan: spec.plan,
    contactEmail: spec.contactEmail,
    timezone: spec.timezone,
    country: spec.country,
  };
  MOCK_CUSTOMERS.push(customer);
  const token = await generateSetupToken(id);
  return { customer, setupToken: token };
}

export async function generateSetupToken(tenantId: string): Promise<SetupToken> {
  await new Promise((r) => setTimeout(r, 0));
  const token = `kv_setup_${tenantId}_${Math.floor(Math.random() * 1e9).toString(36)}`;
  const expiresAtIso = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString();
  return {
    tenantId,
    token,
    expiresAtIso,
    qrPayload: `kaivue://setup?tenant=${encodeURIComponent(tenantId)}&token=${encodeURIComponent(token)}`,
  };
}

export async function beginImpersonation(
  customerId: string,
  reason: string,
): Promise<ImpersonationSession> {
  await new Promise((r) => setTimeout(r, 0));
  if (!reason || !reason.trim()) {
    throw new Error('reason is required');
  }
  return {
    sessionId: `imp-${customerId}-${Date.now()}`,
    customerId,
    integratorUserId: 'integrator-user-self',
    reason: reason.trim(),
    beganAtIso: new Date().toISOString(),
    auditLogId: `audit-${customerId}-${Date.now()}`,
  };
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const CUSTOMERS_QUERY_KEY = 'customers' as const;

export function customersQueryKey(integratorId: string, filters: CustomerFilters = {}) {
  return [CUSTOMERS_QUERY_KEY, integratorId, filters] as const;
}

export function customerQueryKey(id: string) {
  return [CUSTOMERS_QUERY_KEY, 'detail', id] as const;
}

// Test/fixture exports.
export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
  OTHER_INTEGRATOR_ID,
  SUB_RESELLERS,
  LABELS,
  TIERS,
  PLAN_IDS,
  MOCK_CUSTOMERS,
};
