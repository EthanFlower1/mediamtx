import { useTranslation } from 'react-i18next';
import { Camera, UserPlus, LifeBuoy, ScrollText } from 'lucide-react';

// KAI-320: Quick actions row.
//
// Icon-only buttons would fail WCAG 2.1 — every button carries both
// a visible label and an aria-label so screen readers announce intent
// consistently with sighted users.

export interface QuickAction {
  key: string;
  onSelect: () => void;
}

export interface QuickActionsRowProps {
  onAddCamera?: () => void;
  onInviteUser?: () => void;
  onDownloadSupportBundle?: () => void;
  onViewAuditLog?: () => void;
}

export function QuickActionsRow(props: QuickActionsRowProps): JSX.Element {
  const { t } = useTranslation();

  const actions = [
    {
      key: 'addCamera',
      label: t('dashboard.quickActions.addCamera'),
      icon: <Camera aria-hidden="true" />,
      onSelect: props.onAddCamera ?? (() => undefined),
    },
    {
      key: 'inviteUser',
      label: t('dashboard.quickActions.inviteUser'),
      icon: <UserPlus aria-hidden="true" />,
      onSelect: props.onInviteUser ?? (() => undefined),
    },
    {
      key: 'downloadSupportBundle',
      label: t('dashboard.quickActions.downloadSupportBundle'),
      icon: <LifeBuoy aria-hidden="true" />,
      onSelect: props.onDownloadSupportBundle ?? (() => undefined),
    },
    {
      key: 'viewAuditLog',
      label: t('dashboard.quickActions.viewAuditLog'),
      icon: <ScrollText aria-hidden="true" />,
      onSelect: props.onViewAuditLog ?? (() => undefined),
    },
  ];

  return (
    <nav
      aria-label={t('dashboard.quickActions.sectionLabel')}
      className="admin-dashboard__quick-actions"
    >
      <ul role="list">
        {actions.map((action) => (
          <li key={action.key}>
            <button
              type="button"
              onClick={action.onSelect}
              aria-label={action.label}
              data-testid={`quick-action-${action.key}`}
            >
              {action.icon}
              <span>{action.label}</span>
            </button>
          </li>
        ))}
      </ul>
    </nav>
  );
}
