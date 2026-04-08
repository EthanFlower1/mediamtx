import { useTranslation } from 'react-i18next';
import { useRuntimeContext } from '@/contexts/runtime';

export function CommandHome(): JSX.Element {
  const { t } = useTranslation();
  const ctx = useRuntimeContext();
  return (
    <main>
      <h1>{t('command.home.title')}</h1>
      <p>context: {ctx.kind}</p>
    </main>
  );
}
