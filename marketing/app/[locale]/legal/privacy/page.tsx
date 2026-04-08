import { useTranslations } from 'next-intl';

export default function PrivacyPage() {
  const t = useTranslations('Legal');
  return (
    <section>
      <h1>{t('privacyTitle')}</h1>
      <p>{t('placeholder')}</p>
    </section>
  );
}
