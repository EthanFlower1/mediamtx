import { useTranslations } from 'next-intl';

export default function PricingPage() {
  const t = useTranslations('Pricing');
  return (
    <section>
      <h1>{t('title')}</h1>
      <p>{t('subtitle')}</p>
      <p>{t('comingSoon')}</p>
    </section>
  );
}
