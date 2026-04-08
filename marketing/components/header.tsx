'use client';

import Link from 'next/link';
import { useTranslations, useLocale } from 'next-intl';
import { useRouter, usePathname } from 'next/navigation';
import { locales, type Locale } from '../i18n';

export function Header() {
  const t = useTranslations('Nav');
  const tLang = useTranslations('LanguageSwitcher');
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();

  const switchLocale = (next: string) => {
    const stripped = pathname.replace(new RegExp(`^/(${locales.join('|')})`), '');
    const newPath = `/${next}${stripped || ''}`;
    router.push(newPath);
  };

  return (
    <header
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '1rem 1.5rem',
        maxWidth: 'var(--max-width)',
        margin: '0 auto'
      }}
    >
      <Link href={`/${locale}`} style={{ fontWeight: 700 }}>
        Kaivue
      </Link>

      <nav aria-label="Primary">
        <ul style={{ display: 'flex', gap: '1.5rem', listStyle: 'none', padding: 0, margin: 0 }}>
          <li>
            <Link href={`/${locale}/pricing`}>{t('pricing')}</Link>
          </li>
          <li>
            <Link href={`/${locale}/careers`}>{t('careers')}</Link>
          </li>
          <li>
            <Link href={`/${locale}/vs/verkada`}>{t('compare')}</Link>
          </li>
        </ul>
      </nav>

      <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
        <span className="sr-only">{tLang('label')}</span>
        <select
          aria-label={tLang('label')}
          value={locale}
          onChange={(e) => switchLocale(e.target.value)}
          style={{ background: 'transparent', color: 'inherit', border: '1px solid var(--muted)', padding: '0.25rem' }}
        >
          {locales.map((l: Locale) => (
            <option key={l} value={l}>
              {tLang(l)}
            </option>
          ))}
        </select>
      </label>
    </header>
  );
}
