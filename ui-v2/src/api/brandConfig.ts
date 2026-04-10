// KAI-310: Brand Configuration API client stub.
//
// Typed promise stubs for the integrator-facing brand configuration:
//   - getBrandConfig(integratorId)
//   - saveBrandDraft(integratorId, config)
//   - publishBrand(integratorId)
//   - validateDomain(integratorId, domain)
//   - listEmailTemplates(integratorId)
//   - updateEmailTemplate(integratorId, templateId, body)
//
// All data is mocked here. The real implementation will be wired to
// Connect-Go clients generated from KAI-238 protos. This file is the
// single seam; swapping transports is a one-file change.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type DomainValidationStatus = 'pending' | 'valid' | 'invalid' | 'checking';

export interface ColorPalette {
  readonly primary: string;
  readonly secondary: string;
  readonly accent: string;
  readonly danger: string;
}

export interface BrandConfig {
  readonly integratorId: string;
  readonly companyName: string;
  readonly tagline: string;
  readonly logoUrl: string | null;
  readonly iconUrl: string | null;
  readonly colors: ColorPalette;
  readonly customDomain: string | null;
  readonly domainValidation: DomainValidation | null;
  readonly mobileAppName: string;
  readonly mobileBundleIdPrefix: string;
  readonly splashScreenUrl: string | null;
  readonly isDraft: boolean;
  readonly lastPublishedAtIso: string | null;
  readonly updatedAtIso: string;
}

export interface DomainValidation {
  readonly domain: string;
  readonly status: DomainValidationStatus;
  readonly cnameTarget: string;
  readonly checkedAtIso: string | null;
  readonly errorMessage: string | null;
}

export type EmailTemplateType = 'welcome' | 'alert' | 'report';

export interface EmailTemplate {
  readonly id: string;
  readonly integratorId: string;
  readonly type: EmailTemplateType;
  readonly subject: string;
  readonly bodyHtml: string;
  readonly updatedAtIso: string;
}

export interface BrandDraft {
  readonly companyName: string;
  readonly tagline: string;
  readonly logoUrl: string | null;
  readonly iconUrl: string | null;
  readonly colors: ColorPalette;
  readonly customDomain: string | null;
  readonly mobileAppName: string;
  readonly splashScreenUrl: string | null;
}

export interface SaveBrandResult {
  readonly config: BrandConfig;
}

export interface PublishBrandResult {
  readonly config: BrandConfig;
  readonly publishedAtIso: string;
}

export interface ValidateDomainResult {
  readonly validation: DomainValidation;
}

export interface UpdateEmailTemplateResult {
  readonly template: EmailTemplate;
}

// ---------------------------------------------------------------------------
// Mock dataset
// ---------------------------------------------------------------------------

const CURRENT_INTEGRATOR_ID = 'integrator-001';

const DEFAULT_COLORS: ColorPalette = {
  primary: '#1e40af',
  secondary: '#64748b',
  accent: '#f59e0b',
  danger: '#dc2626',
};

function buildMockConfig(integratorId: string): BrandConfig {
  return {
    integratorId,
    companyName: 'SecureVision Pro',
    tagline: 'Enterprise surveillance, simplified.',
    logoUrl: null,
    iconUrl: null,
    colors: { ...DEFAULT_COLORS },
    customDomain: 'nvr.securevision.example',
    domainValidation: {
      domain: 'nvr.securevision.example',
      status: 'valid',
      cnameTarget: 'cname.kaivue.io',
      checkedAtIso: '2026-04-07T10:00:00.000Z',
      errorMessage: null,
    },
    mobileAppName: 'SecureVision',
    mobileBundleIdPrefix: 'com.securevision.nvr',
    splashScreenUrl: null,
    isDraft: false,
    lastPublishedAtIso: '2026-04-06T18:00:00.000Z',
    updatedAtIso: '2026-04-07T12:00:00.000Z',
  };
}

function buildMockTemplates(integratorId: string): EmailTemplate[] {
  const types: EmailTemplateType[] = ['welcome', 'alert', 'report'];
  return types.map((type, i) => ({
    id: `tmpl-${integratorId}-${type}`,
    integratorId,
    type,
    subject:
      type === 'welcome'
        ? 'Welcome to {{companyName}}'
        : type === 'alert'
          ? 'Alert: {{alertTitle}}'
          : 'Monthly Report for {{month}}',
    bodyHtml: `<div style="font-family:sans-serif;color:#333"><h1 style="color:{{primaryColor}}">{{companyName}}</h1><p>${type} template body.</p></div>`,
    updatedAtIso: new Date(Date.UTC(2026, 3, 7, 12, 0, 0) - i * 3_600_000).toISOString(),
  }));
}

let mockConfig: BrandConfig = buildMockConfig(CURRENT_INTEGRATOR_ID);
const mockTemplates: EmailTemplate[] = buildMockTemplates(CURRENT_INTEGRATOR_ID);

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export async function getBrandConfig(integratorId: string): Promise<BrandConfig> {
  await new Promise((r) => setTimeout(r, 0));
  if (integratorId !== mockConfig.integratorId) {
    return buildMockConfig(integratorId);
  }
  return mockConfig;
}

export async function saveBrandDraft(
  integratorId: string,
  draft: BrandDraft,
): Promise<SaveBrandResult> {
  await new Promise((r) => setTimeout(r, 0));
  mockConfig = {
    ...mockConfig,
    integratorId,
    companyName: draft.companyName,
    tagline: draft.tagline,
    logoUrl: draft.logoUrl,
    iconUrl: draft.iconUrl,
    colors: { ...draft.colors },
    customDomain: draft.customDomain,
    mobileAppName: draft.mobileAppName,
    splashScreenUrl: draft.splashScreenUrl,
    isDraft: true,
    updatedAtIso: new Date().toISOString(),
  };
  return { config: mockConfig };
}

export async function publishBrand(integratorId: string): Promise<PublishBrandResult> {
  await new Promise((r) => setTimeout(r, 0));
  const now = new Date().toISOString();
  mockConfig = {
    ...mockConfig,
    integratorId,
    isDraft: false,
    lastPublishedAtIso: now,
    updatedAtIso: now,
  };
  return { config: mockConfig, publishedAtIso: now };
}

export async function validateDomain(
  _integratorId: string,
  domain: string,
): Promise<ValidateDomainResult> {
  await new Promise((r) => setTimeout(r, 0));
  // Deterministic mock: domains containing "invalid" fail.
  const isValid = !domain.toLowerCase().includes('invalid');
  const validation: DomainValidation = {
    domain,
    status: isValid ? 'valid' : 'invalid',
    cnameTarget: 'cname.kaivue.io',
    checkedAtIso: new Date().toISOString(),
    errorMessage: isValid ? null : 'CNAME record not found for this domain.',
  };
  mockConfig = {
    ...mockConfig,
    domainValidation: validation,
  };
  return { validation };
}

export async function listEmailTemplates(
  integratorId: string,
): Promise<readonly EmailTemplate[]> {
  await new Promise((r) => setTimeout(r, 0));
  return mockTemplates.filter((t) => t.integratorId === integratorId);
}

export async function updateEmailTemplate(
  _integratorId: string,
  templateId: string,
  body: { subject: string; bodyHtml: string },
): Promise<UpdateEmailTemplateResult> {
  await new Promise((r) => setTimeout(r, 0));
  const idx = mockTemplates.findIndex((t) => t.id === templateId);
  if (idx === -1) {
    throw new Error(`Template ${templateId} not found`);
  }
  const updated: EmailTemplate = {
    ...mockTemplates[idx]!,
    subject: body.subject,
    bodyHtml: body.bodyHtml,
    updatedAtIso: new Date().toISOString(),
  };
  mockTemplates[idx] = updated;
  return { template: updated };
}

// ---------------------------------------------------------------------------
// Query keys — integrator-scoped
// ---------------------------------------------------------------------------

export const BRAND_QUERY_KEY = 'brand' as const;

export function brandConfigQueryKey(integratorId: string) {
  return [BRAND_QUERY_KEY, integratorId, 'config'] as const;
}

export function emailTemplatesQueryKey(integratorId: string) {
  return [BRAND_QUERY_KEY, integratorId, 'emailTemplates'] as const;
}

// Test/fixture exports.
export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
  DEFAULT_COLORS,
  buildMockConfig,
  buildMockTemplates,
};
