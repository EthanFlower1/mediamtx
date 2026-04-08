// KAI-308: Fleet API client stub.
//
// Returns typed mock data for the Integrator Portal fleet dashboard.
// The real implementation will use Connect-Go clients generated from
// the protos defined in KAI-238. Until those land, this file is the
// single seam where the fleet dashboard fetches data so swapping to
// real transport is a one-file change.
//
// Scope enforcement is modelled explicitly: `listFleet` MUST filter
// to customers whose `integratorId` matches the caller. This mirrors
// how the real RPC will reject cross-integrator reads.

export type HealthStatus = 'healthy' | 'degraded' | 'critical';
export type CustomerTier = 'platinum' | 'gold' | 'silver' | 'bronze';

export interface FleetCustomer {
  readonly id: string;
  readonly integratorId: string;
  readonly subResellerId: string | null;
  readonly name: string;
  readonly tier: CustomerTier;
  readonly labels: readonly string[];
  readonly cameraCount: number;
  readonly onlineCameraCount: number;
  readonly monthlyRecurringRevenueCents: number;
  readonly uptimePercent: number;
  readonly openIncidents: number;
  readonly health: HealthStatus;
  readonly lastContactIso: string;
}

export type AlertSeverity = 'info' | 'warning' | 'critical';

export interface FleetAlert {
  readonly id: string;
  readonly customerId: string;
  readonly customerName: string;
  readonly severity: AlertSeverity;
  readonly messageKey: string; // i18n key; payload values are inlined by caller
  readonly createdAtIso: string;
}

export interface FleetKpis {
  readonly totalCustomers: number;
  readonly totalCameras: number;
  readonly totalMrrCents: number;
  readonly activeIncidents: number;
  readonly uptimePercent: number;
}

export interface FleetFilters {
  readonly subResellerId?: string | null;
  readonly label?: string | null;
  readonly tier?: CustomerTier | null;
}

export interface FleetSnapshot {
  readonly integratorId: string;
  readonly kpis: FleetKpis;
  readonly customers: readonly FleetCustomer[];
  readonly alerts: readonly FleetAlert[];
  readonly availableSubResellers: readonly { id: string; name: string }[];
  readonly availableLabels: readonly string[];
}

// ---------------------------------------------------------------------------
// Mock dataset
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

function makeCustomer(
  index: number,
  integratorId: string,
  overrides: Partial<FleetCustomer> = {},
): FleetCustomer {
  const tier = TIERS[index % TIERS.length]!;
  const healthRoll = index % 5;
  const health: HealthStatus =
    healthRoll === 0 ? 'critical' : healthRoll === 1 || healthRoll === 2 ? 'degraded' : 'healthy';
  const cameraCount = 8 + ((index * 7) % 64);
  const offline = health === 'critical' ? Math.ceil(cameraCount * 0.35) : health === 'degraded' ? 2 : 0;
  const subReseller = SUB_RESELLERS[index % SUB_RESELLERS.length]!;
  return {
    id: `cust-${integratorId}-${String(index).padStart(3, '0')}`,
    integratorId,
    subResellerId: subReseller.id,
    name: `Customer ${String.fromCharCode(65 + (index % 26))}${index}`,
    tier,
    labels: [LABELS[index % LABELS.length]!],
    cameraCount,
    onlineCameraCount: cameraCount - offline,
    monthlyRecurringRevenueCents: (250 + index * 37) * 100,
    uptimePercent: health === 'critical' ? 91.2 : health === 'degraded' ? 97.4 : 99.8,
    openIncidents: health === 'critical' ? 3 : health === 'degraded' ? 1 : 0,
    health,
    lastContactIso: new Date(Date.UTC(2026, 3, 7, 12, 0, 0) - index * 60_000).toISOString(),
    ...overrides,
  };
}

function buildMockDataset(): FleetCustomer[] {
  const inScope = Array.from({ length: 20 }, (_, i) => makeCustomer(i, CURRENT_INTEGRATOR_ID));
  const outOfScope = Array.from({ length: 10 }, (_, i) => makeCustomer(100 + i, OTHER_INTEGRATOR_ID));
  return [...inScope, ...outOfScope];
}

const MOCK_CUSTOMERS: readonly FleetCustomer[] = buildMockDataset();

const MOCK_ALERTS: readonly FleetAlert[] = MOCK_CUSTOMERS.filter(
  (c) => c.integratorId === CURRENT_INTEGRATOR_ID && c.health !== 'healthy',
).flatMap((c, i): FleetAlert[] => {
  const base: FleetAlert = {
    id: `alert-${c.id}-1`,
    customerId: c.id,
    customerName: c.name,
    severity: c.health === 'critical' ? 'critical' : 'warning',
    messageKey: c.health === 'critical' ? 'fleet.alerts.cameraOffline' : 'fleet.alerts.degradedLink',
    createdAtIso: new Date(Date.UTC(2026, 3, 7, 11, 30, 0) - i * 300_000).toISOString(),
  };
  return [base];
});

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export interface ListFleetRequest {
  readonly integratorId: string;
  readonly filters?: FleetFilters;
}

/**
 * listFleet returns a {@link FleetSnapshot} scoped to the given integrator.
 *
 * Scope enforcement: customers whose `integratorId` does not match the
 * requested `integratorId` are dropped here. Production Connect-Go will
 * enforce the same invariant server-side; this client copy is a belt-and-
 * braces guard for the mock path.
 */
export async function listFleet(
  integratorId: string,
  filters: FleetFilters = {},
): Promise<FleetSnapshot> {
  // Simulate network latency in dev.
  await new Promise((r) => setTimeout(r, 0));

  const scoped = MOCK_CUSTOMERS.filter((c) => c.integratorId === integratorId);

  const filtered = scoped.filter((c) => {
    if (filters.subResellerId && c.subResellerId !== filters.subResellerId) return false;
    if (filters.label && !c.labels.includes(filters.label)) return false;
    if (filters.tier && c.tier !== filters.tier) return false;
    return true;
  });

  const kpis: FleetKpis = {
    totalCustomers: filtered.length,
    totalCameras: filtered.reduce((acc, c) => acc + c.cameraCount, 0),
    totalMrrCents: filtered.reduce((acc, c) => acc + c.monthlyRecurringRevenueCents, 0),
    activeIncidents: filtered.reduce((acc, c) => acc + c.openIncidents, 0),
    uptimePercent:
      filtered.length === 0
        ? 100
        : filtered.reduce((acc, c) => acc + c.uptimePercent, 0) / filtered.length,
  };

  const filteredIds = new Set(filtered.map((c) => c.id));
  const alerts = MOCK_ALERTS.filter((a) => filteredIds.has(a.customerId));

  return {
    integratorId,
    kpis,
    customers: filtered,
    alerts,
    availableSubResellers: SUB_RESELLERS,
    availableLabels: [...LABELS],
  };
}

export const FLEET_QUERY_KEY = 'fleet' as const;

export function fleetQueryKey(integratorId: string, filters: FleetFilters = {}) {
  return [FLEET_QUERY_KEY, integratorId, filters] as const;
}

// Test/fixture exports -----------------------------------------------------

export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
  OTHER_INTEGRATOR_ID,
  SUB_RESELLERS,
  LABELS,
  TIERS,
  MOCK_CUSTOMERS,
  MOCK_ALERTS,
};
