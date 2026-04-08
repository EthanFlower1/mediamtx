import { useTranslations } from 'next-intl';

export default function CookiePage() {
  const t = useTranslations('Legal');
  return (
    <section>
      <h1>{t('cookieTitle')}</h1>
      <p>{t('placeholder')}</p>
    </section>
  );
}
