'use client';

import Link from 'next/link';
import { useTranslations, useLocale } from 'next-intl';

export function Footer() {
  const t = useTranslations('Footer');
  const locale = useLocale();
  const year = new Date().getFullYear();

  return (
    <footer
      style={{
        borderTop: '1px solid #1f2937',
        marginTop: '4rem',
        padding: '2rem 1.5rem',
        maxWidth: 'var(--max-width)',
        margin: '4rem auto 0'
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: '1rem' }}>
        <small>{t('copyright', { year })}</small>
        <ul style={{ display: 'flex', gap: '1rem', listStyle: 'none', padding: 0, margin: 0 }}>
          <li>
            <Link href={`/${locale}/legal/terms`}>{t('terms')}</Link>
          </li>
          <li>
            <Link href={`/${locale}/legal/privacy`}>{t('privacy')}</Link>
          </li>
          <li>
            <Link href={`/${locale}/legal/cookie`}>{t('cookies')}</Link>
          </li>
        </ul>
      </div>
    </footer>
  );
}
