import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { SessionAuditTable } from '@/components/customers/SessionAuditTable';

// KAI-467: Impersonation audit page.
//
// Route: /command/impersonation-audit
// Shows the SessionAuditTable with navigation back to the portal.

export function ImpersonationAuditPage(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();

  return (
    <main
      data-testid="impersonation-audit-page"
      aria-labelledby="impersonation-audit-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <nav
        aria-label={t('impersonation.audit.breadcrumbAriaLabel')}
        className="text-xs text-slate-500"
      >
        <ol className="flex gap-1">
          <li>
            <button
              type="button"
              data-testid="breadcrumb-portal"
              onClick={() => navigate('/command')}
              className="hover:underline focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              {t('command.home.title')}
            </button>
          </li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-slate-700">
            {t('impersonation.audit.pageTitle')}
          </li>
        </ol>
      </nav>

      <header className="mt-2">
        <h1
          id="impersonation-audit-heading"
          className="text-2xl font-bold text-slate-900"
        >
          {t('impersonation.audit.pageTitle')}
        </h1>
        <p className="mt-1 text-sm text-slate-600">
          {t('impersonation.audit.pageSubtitle')}
        </p>
      </header>

      <div className="mt-4">
        <SessionAuditTable />
      </div>
    </main>
  );
}

export default ImpersonationAuditPage;
