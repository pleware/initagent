import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import enTranslation from './locales/en/translation.json';
import plTranslation from './locales/pl/translation.json';

const resources = {
  en: {
    translation: enTranslation,
  },
  pl: {
    translation: plTranslation,
  },
};

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: ['en', 'pl'],
    nonExplicitSupportedLngs: true,
    debug: false,

    interpolation: {
      escapeValue: false,
    },

    detection: {
      order: ['querystring', 'localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupQuerystring: 'lng',
      lookupLocalStorage: 'i18nextLng',
    },
  });

i18n.on('languageChanged', (lng) => {
  document.documentElement.lang = (lng || 'en').split('-')[0];
});

export default i18n;
