import { useTranslations } from 'next-intl';
import { CtaButton } from '../../components/cta-button';

export default function HomePage() {
  const t = useTranslations('Home');
  const tCta = useTranslations('Cta');

  return (
    <section>
      <h1>{t('heroTitle')}</h1>
      <p>{t('heroSubtitle')}</p>

      <div style={{ display: 'flex', gap: '0.75rem', margin: '2rem 0' }}>
        <CtaButton intent="trial">{tCta('tryFree')}</CtaButton>
        <CtaButton intent="demo">{tCta('scheduleDemo')}</CtaButton>
        <CtaButton intent="partner">{tCta('becomePartner')}</CtaButton>
      </div>

      <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: '1.5rem' }}>
        <li>
          <h2>{t('valueProp1Title')}</h2>
          <p>{t('valueProp1Body')}</p>
        </li>
        <li>
          <h2>{t('valueProp2Title')}</h2>
          <p>{t('valueProp2Body')}</p>
        </li>
        <li>
          <h2>{t('valueProp3Title')}</h2>
          <p>{t('valueProp3Body')}</p>
        </li>
      </ul>
    </section>
  );
}
