import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import en from './locales/en/common.json';
import es from './locales/es/common.json';
import fr from './locales/fr/common.json';
import de from './locales/de/common.json';

// KAI-307 + KAI-358: i18n bootstrap.
//
// Launch languages: EN, ES, FR, DE. NO hardcoded customer-visible
// strings anywhere in the codebase — every label, error, and aria
// description must come through `t()`.
//
// Per-integrator string overrides will be layered on top of these
// defaults via the i18next backend added in KAI-358.
export const SUPPORTED_LANGUAGES = ['en', 'es', 'fr', 'de'] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { common: en },
      es: { common: es },
      fr: { common: fr },
      de: { common: de },
    },
    fallbackLng: 'en',
    supportedLngs: SUPPORTED_LANGUAGES,
    defaultNS: 'common',
    interpolation: { escapeValue: false },
  });

export { i18n };
