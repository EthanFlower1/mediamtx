import { useTranslations } from 'next-intl';

export default function TermsPage() {
  const t = useTranslations('Legal');
  return (
    <section>
      <h1>{t('termsTitle')}</h1>
      <p>{t('placeholder')}</p>
    </section>
  );
}
