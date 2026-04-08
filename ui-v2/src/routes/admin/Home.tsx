import { useTranslation } from 'react-i18next';
import { useRuntimeContext } from '@/contexts/runtime';

export function AdminHome(): JSX.Element {
  const { t } = useTranslation();
  const ctx = useRuntimeContext();
  return (
    <main>
      <h1>{t('admin.home.title')}</h1>
      <p>context: {ctx.kind}</p>
    </main>
  );
}
