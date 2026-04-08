import { useTranslation } from 'react-i18next';

export function AdminSettings(): JSX.Element {
  const { t } = useTranslation();
  return (
    <main>
      <h1>{t('admin.settings.title')}</h1>
    </main>
  );
}
