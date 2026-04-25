import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import type {
  AlertItem, AuditEntry, Camera, Role, Rule, SavedView, Site, User,
} from '../types';
import {
  ALERTS, AUDIT, CAMERAS, ROLES, RULES, SAVED_VIEWS, SITES, USERS,
} from './portalData';

// These hooks are the seam between mock-data demo mode and real API calls.
// Phase 1 swaps the queryFn body for `apiGet('/api/v1/sites')` etc.
// Until then we resolve to the existing mock arrays so screens look right.

const MOCK_LATENCY_MS = 0;

function mock<T>(value: T): Promise<T> {
  return MOCK_LATENCY_MS > 0
    ? new Promise((r) => setTimeout(() => r(value), MOCK_LATENCY_MS))
    : Promise.resolve(value);
}

export const queryKeys = {
  sites: ['sites'] as const,
  cameras: ['cameras'] as const,
  alerts: ['alerts'] as const,
  rules: ['rules'] as const,
  users: ['users'] as const,
  roles: ['roles'] as const,
  audit: ['audit'] as const,
  savedViews: ['saved-views'] as const,
};

export function useSites(): UseQueryResult<Site[]> {
  return useQuery({ queryKey: queryKeys.sites, queryFn: () => mock(SITES) });
}

export function useCameras(): UseQueryResult<Camera[]> {
  return useQuery({ queryKey: queryKeys.cameras, queryFn: () => mock(CAMERAS) });
}

export function useAlerts(): UseQueryResult<AlertItem[]> {
  return useQuery({ queryKey: queryKeys.alerts, queryFn: () => mock(ALERTS) });
}

export function useRules(): UseQueryResult<Rule[]> {
  return useQuery({ queryKey: queryKeys.rules, queryFn: () => mock(RULES) });
}

export function useUsersList(): UseQueryResult<User[]> {
  return useQuery({ queryKey: queryKeys.users, queryFn: () => mock(USERS) });
}

export function useRoles(): UseQueryResult<Role[]> {
  return useQuery({ queryKey: queryKeys.roles, queryFn: () => mock(ROLES) });
}

export function useAudit(): UseQueryResult<AuditEntry[]> {
  return useQuery({ queryKey: queryKeys.audit, queryFn: () => mock(AUDIT) });
}

export function useSavedViews(): UseQueryResult<SavedView[]> {
  return useQuery({ queryKey: queryKeys.savedViews, queryFn: () => mock(SAVED_VIEWS) });
}
