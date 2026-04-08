import { useTranslation } from 'react-i18next';

export function NotFound(): JSX.Element {
  const { t } = useTranslation();
  return (
    <main>
      <h1>{t('errors.notFound')}</h1>
    </main>
  );
}
