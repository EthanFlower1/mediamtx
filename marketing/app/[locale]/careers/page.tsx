import { useTranslations } from 'next-intl';

export default function CareersPage() {
  const t = useTranslations('Careers');
  return (
    <section>
      <h1>{t('title')}</h1>
      <p>{t('subtitle')}</p>
      <p>{t('noOpenings')}</p>
    </section>
  );
}
