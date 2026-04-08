import { useTranslation } from 'react-i18next';

export function CommandCustomers(): JSX.Element {
  const { t } = useTranslation();
  return (
    <main>
      <h1>{t('command.customers.title')}</h1>
    </main>
  );
}
