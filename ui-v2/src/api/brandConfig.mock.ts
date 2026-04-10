// KAI-310: Brand Configuration deterministic mock data.
//
// Separated from brandConfig.ts so the real client can lazy-import this
// file behind a feature flag. The main brandConfig.ts embeds the mock
// inline today (same as fleet.ts / customers.ts pattern), but this
// companion file provides a standalone export for Storybook, Playwright,
// and integration test fixtures.

import type {
  BrandConfig,
  ColorPalette,
  EmailTemplate,
  EmailTemplateType,
  DomainValidation,
} from './brandConfig';

// ---------------------------------------------------------------------------
// Deterministic fixtures
// ---------------------------------------------------------------------------

export const MOCK_INTEGRATOR_ID = 'integrator-001';

export const MOCK_COLORS: ColorPalette = {
  primary: '#1e40af',
  secondary: '#64748b',
  accent: '#f59e0b',
  danger: '#dc2626',
};

export const MOCK_DOMAIN_VALIDATION: DomainValidation = {
  domain: 'nvr.securevision.example',
  status: 'valid',
  cnameTarget: 'cname.kaivue.io',
  checkedAtIso: '2026-04-07T10:00:00.000Z',
  errorMessage: null,
};

export const MOCK_BRAND_CONFIG: BrandConfig = {
  integratorId: MOCK_INTEGRATOR_ID,
  companyName: 'SecureVision Pro',
  tagline: 'Enterprise surveillance, simplified.',
  logoUrl: null,
  iconUrl: null,
  colors: { ...MOCK_COLORS },
  customDomain: 'nvr.securevision.example',
  domainValidation: MOCK_DOMAIN_VALIDATION,
  mobileAppName: 'SecureVision',
  mobileBundleIdPrefix: 'com.securevision.nvr',
  splashScreenUrl: null,
  isDraft: false,
  lastPublishedAtIso: '2026-04-06T18:00:00.000Z',
  updatedAtIso: '2026-04-07T12:00:00.000Z',
};

const TEMPLATE_TYPES: EmailTemplateType[] = ['welcome', 'alert', 'report'];

export const MOCK_EMAIL_TEMPLATES: readonly EmailTemplate[] = TEMPLATE_TYPES.map(
  (type, i): EmailTemplate => ({
    id: `tmpl-${MOCK_INTEGRATOR_ID}-${type}`,
    integratorId: MOCK_INTEGRATOR_ID,
    type,
    subject:
      type === 'welcome'
        ? 'Welcome to {{companyName}}'
        : type === 'alert'
          ? 'Alert: {{alertTitle}}'
          : 'Monthly Report for {{month}}',
    bodyHtml: `<div style="font-family:sans-serif;color:#333"><h1 style="color:{{primaryColor}}">{{companyName}}</h1><p>${type} template body.</p></div>`,
    updatedAtIso: new Date(Date.UTC(2026, 3, 7, 12, 0, 0) - i * 3_600_000).toISOString(),
  }),
);

export const MOCK_INVALID_DOMAIN_VALIDATION: DomainValidation = {
  domain: 'invalid.example.com',
  status: 'invalid',
  cnameTarget: 'cname.kaivue.io',
  checkedAtIso: '2026-04-07T11:00:00.000Z',
  errorMessage: 'CNAME record not found for this domain.',
};
